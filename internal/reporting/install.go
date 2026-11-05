package reporting

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/frostyard/chairlift/internal/bootc"
)

// statFile reads a file's metadata. Tests replace it, so no test depends on
// the host's /etc/machine-id.
var statFile = os.Stat

// MachineIDPath is the file whose modification time dates the installation.
// systemd writes it once, on the first boot. Only its time is read: its
// content never leaves this computer.
const MachineIDPath = "/etc/machine-id"

// InstallAge buckets the age of an installation into schema v1's
// install_age enum: under 7 days is "<1w", under 30 days is "<1m", under
// 182 days is "<6m", and else ">=6m". A zero time, or one after now (a
// clock that runs behind), gives false: there is no count to send.
func InstallAge(modTime time.Time, now time.Time) (string, bool) {
	if modTime.IsZero() || modTime.After(now) {
		return "", false
	}
	age := now.Sub(modTime)
	switch {
	case age < 7*24*time.Hour:
		return "<1w", true
	case age < 30*24*time.Hour:
		return "<1m", true
	case age < 182*24*time.Hour:
		return "<6m", true
	default:
		return ">=6m", true
	}
}

// MachineIDTime returns the modification time of path, read through stat so
// a test never touches the real file.
func MachineIDTime(stat func(string) (fs.FileInfo, error)) (time.Time, bool) {
	info, err := stat(MachineIDPath)
	if err != nil || info.IsDir() {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

// Channel maps a bootc image reference to schema v1's channel enum:
// "latest" when the reference's tag is latest, and "other" otherwise
// (another tag, no tag, or a digest).
func Channel(imageRef string) string {
	ref := imageRef
	if i := strings.Index(ref, "@"); i >= 0 {
		return "other"
	}
	slash := strings.LastIndex(ref, "/")
	colon := strings.LastIndex(ref, ":")
	if colon < 0 || colon < slash {
		return "other"
	}
	if ref[colon+1:] == "latest" {
		return "latest"
	}
	return "other"
}

// DefaultInstallInfo returns this host's install age bucket and update
// channel for the weekly count. It reads /etc/machine-id's time and runs
// the unprivileged `bootc status`; without bootc, or when its output cannot
// be read, the channel is "other". ok is false when the age is unknown.
func DefaultInstallInfo(ctx context.Context) (age, channel string, ok bool) {
	modTime, known := MachineIDTime(statFile)
	if !known {
		return "", "", false
	}
	age, ok = InstallAge(modTime, time.Now())
	if !ok {
		return "", "", false
	}
	channel = "other"
	if status, err := bootc.GetStatus(ctx); err == nil && status.Booted() {
		channel = Channel(status.Status.Booted.ImageRef())
	}
	return age, channel, true
}
