// Package firstrun holds every decision of ChairLift's first-run mode
// (ADR-0014): the done markers and their one writer, the pure-Go gate of
// cmd/chairlift-first-run, the step flow, the application list, the actions
// of the Progress step, the color-scheme writer, the connectivity check and
// the completion decision.
//
// The package is puregotk-free (ADR-0007), so it builds into the gate binary
// without loading GTK and is unit-tested headlessly. internal/window's
// first-run window only draws what this package decides.
package firstrun

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// markerDirName is the directory under the config home that holds the
	// ChairLift first-run marker.
	markerDirName = "chairlift"
	// markerName is the ChairLift first-run marker:
	// <config home>/chairlift/first-run.done.
	markerName = "first-run.done"
	// legacyMarkerName is first-setup's marker:
	// <config home>/snow-first-setup.done. ChairLift always reads it.
	legacyMarkerName = "snow-first-setup.done"
	// legacyAutostartName is the per-user autostart copy that first-setup
	// releases before 0.2.16 placed through /etc/skel. Completion removes it,
	// as first-setup's complete-setup script does.
	legacyAutostartName = "org.frostyard.FirstSetup.autostart.desktop"
)

// writeLegacyMarker makes MarkDone also write first-setup's marker during the
// transition, so a bootc rollback to an image that still ships first-setup
// does not start first-setup again. ADR-0014 records the end rule: the write
// ends no earlier than 90 days after the first snosi main image without
// first-setup publishes, and the change that removes it records that date and
// the ChairLift release in docs/design/overview.md. Reading the legacy marker
// (IsDone) never ends.
const writeLegacyMarker = true

// Paths are the per-user files of the first run.
type Paths struct {
	// ConfigHome is $XDG_CONFIG_HOME when it is not empty, else $HOME/.config.
	ConfigHome string
	// Marker is <ConfigHome>/chairlift/first-run.done.
	Marker string
	// LegacyMarker is <ConfigHome>/snow-first-setup.done.
	LegacyMarker string
	// LegacyAutostart is <ConfigHome>/autostart/org.frostyard.FirstSetup.autostart.desktop.
	LegacyAutostart string
}

// ResolvePaths applies first-setup's rule for the config home
// (${XDG_CONFIG_HOME:-$HOME/.config}): $XDG_CONFIG_HOME when it is not empty,
// else home/.config. home is the user's home directory ($HOME). It is an
// error when both are empty.
func ResolvePaths(getenv func(string) string, home string) (Paths, error) {
	configHome := getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		if home == "" {
			return Paths{}, errors.New("neither XDG_CONFIG_HOME nor HOME is set")
		}
		configHome = filepath.Join(home, ".config")
	}
	return Paths{
		ConfigHome:      configHome,
		Marker:          filepath.Join(configHome, markerDirName, markerName),
		LegacyMarker:    filepath.Join(configHome, legacyMarkerName),
		LegacyAutostart: filepath.Join(configHome, "autostart", legacyAutostartName),
	}, nil
}

// MarkerPaths returns the markers MarkDone writes, in write order.
func (p Paths) MarkerPaths() []string {
	if writeLegacyMarker {
		return []string{p.Marker, p.LegacyMarker}
	}
	return []string{p.Marker}
}

// IsDone reports whether the user already finished a first run: true when
// the ChairLift marker or first-setup's marker exists.
func IsDone(p Paths) bool {
	for _, path := range []string{p.Marker, p.LegacyMarker} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// DoneAt is the gate state for the window's paths: false when the paths are
// unknown (pathsErr is set), else IsDone. The Done step uses it to tell
// whether the next login starts the first run again.
func DoneAt(p Paths, pathsErr error) bool {
	return pathsErr == nil && IsDone(p)
}

// MarkDone is the one function that records a finished first run. The Done
// step (through Complete) and `chairlift-first-run --mark-done` both reach
// it. It creates <ConfigHome>/chairlift (mode 0700 for each directory it must
// create), writes the markers of MarkerPaths as empty files (mode 0644; an
// existing marker is kept, so a second call changes nothing) and removes the
// legacy per-user autostart copy (a missing copy is not an error). It tries
// every step and returns the joined errors.
func MarkDone(p Paths) error {
	var errs []error
	if err := os.MkdirAll(filepath.Dir(p.Marker), 0o700); err != nil {
		errs = append(errs, fmt.Errorf("create %s: %w", filepath.Dir(p.Marker), err))
	}
	for _, path := range p.MarkerPaths() {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			errs = append(errs, fmt.Errorf("write %s: %w", path, err))
			continue
		}
		if err := file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", path, err))
		}
	}
	if err := os.Remove(p.LegacyAutostart); err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, fmt.Errorf("remove %s: %w", p.LegacyAutostart, err))
	}
	return errors.Join(errs...)
}
