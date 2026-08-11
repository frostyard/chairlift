package sysupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// osReleasePath is where snosi tooling reads the image identity; unlike
// /etc/os-release it is guaranteed present on the read-only root.
const osReleasePath = "/usr/lib/os-release"

const lsblkCommand = "lsblk"

// imageIdentity extracts IMAGE_ID and IMAGE_VERSION from os-release
// contents. Values may be optionally double-quoted (both forms ship).
func imageIdentity(data []byte) (imageID, version string) {
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.Trim(value, `"`)
		switch key {
		case "IMAGE_ID":
			imageID = value
		case "IMAGE_VERSION":
			version = value
		}
	}
	return imageID, version
}

// lsblkNode is the subset of `lsblk -J` output needed for slot discovery.
// lsblk nests partitions under each disk's "children" array, so the walk
// must recurse rather than scan only the top-level list.
type lsblkNode struct {
	PartLabel string      `json:"partlabel"`
	Children  []lsblkNode `json:"children"`
}

type lsblkOutput struct {
	BlockDevices []lsblkNode `json:"blockdevices"`
}

// otherSlotFromLsblk returns the version label of the inactive root slot:
// the partition labeled "<imageID>_<version>_r" that is not the running
// slot. The running slot is excluded by its PARTLABEL, never by comparing
// mount sources: on a dm-verity root the source is /dev/mapper/root, which
// never equals the partition's /dev path. A fresh install's unused slot is
// labeled "_empty", which the "<imageID>_" prefix filter excludes.
func otherSlotFromLsblk(data []byte, imageID, runningVersion string) (string, bool) {
	if imageID == "" {
		return "", false
	}
	var parsed lsblkOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", false
	}
	runningLabel := imageID + "_" + runningVersion + "_r"
	prefix := imageID + "_"
	var walk func(nodes []lsblkNode) (string, bool)
	walk = func(nodes []lsblkNode) (string, bool) {
		for _, node := range nodes {
			label := node.PartLabel
			if label != runningLabel && strings.HasPrefix(label, prefix) && strings.HasSuffix(label, "_r") {
				return strings.TrimSuffix(strings.TrimPrefix(label, prefix), "_r"), true
			}
			if version, ok := walk(node.Children); ok {
				return version, ok
			}
		}
		return "", false
	}
	return walk(parsed.BlockDevices)
}

// RollbackCandidate reports whether the inactive slot's version is a
// rollback target. Only an older version qualifies: after a stage the
// inactive slot holds the newer pending version, which is not a rollback.
// The 14-digit grammar makes lexicographic comparison numeric.
func RollbackCandidate(other, running string) (string, bool) {
	if !ValidVersion(other) || !ValidVersion(running) {
		return "", false
	}
	if other >= running {
		return "", false
	}
	return other, true
}

// RollbackVersion returns the version in the inactive root slot when it is
// older than the running version (a genuine rollback target), or ("",
// false) when the other slot is empty, holds a staged newer version, or
// discovery fails. Runs unprivileged: partition labels come from lsblk and
// the running identity from os-release.
func RollbackVersion(ctx context.Context) (string, bool) {
	osRelease, err := os.ReadFile(osReleasePath)
	if err != nil {
		return "", false
	}
	imageID, runningVersion := imageIdentity(osRelease)
	output, err := runLsblk(ctx)
	if err != nil {
		return "", false
	}
	other, ok := otherSlotFromLsblk(output, imageID, runningVersion)
	if !ok {
		return "", false
	}
	return RollbackCandidate(other, runningVersion)
}

// runLsblk captures `lsblk -J -o PATH,PARTLABEL`. Separated so tests can
// exercise the parse pipeline without a real block layout.
func runLsblk(ctx context.Context) ([]byte, error) {
	output, err := exec.CommandContext(ctx, lsblkCommand, "-J", "-o", "PATH,PARTLABEL").Output()
	if err != nil {
		return nil, &Error{Message: fmt.Sprintf("lsblk failed: %v", err), Err: err}
	}
	return output, nil
}
