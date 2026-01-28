// Package updates implements the Updates page for managing NBC system updates
// and package manager updates (Flatpak, Homebrew).
package updates

import (
	"context"
	"os"

	"github.com/frostyard/chairlift/internal/nbc"
	"github.com/frostyard/chairlift/internal/pm"
)

// NBCUpdateStatus holds the result of an NBC update check.
type NBCUpdateStatus struct {
	UpdateNeeded  bool
	NewDigest     string
	CurrentDigest string
}

// UpdateCounts tracks the number of available updates by source.
type UpdateCounts struct {
	NBC      int
	Flatpak  int
	Homebrew int
}

// Total returns the sum of all update counts.
func (c UpdateCounts) Total() int {
	return c.NBC + c.Flatpak + c.Homebrew
}

// IsNBCAvailable checks if the system is running on NBC by checking
// for the /run/nbc-booted marker file.
func IsNBCAvailable() bool {
	_, err := os.Stat("/run/nbc-booted")
	return err == nil
}

// CheckNBCUpdate checks for available NBC system updates.
// Returns the update status or an error if the check fails.
func CheckNBCUpdate(ctx context.Context) (*NBCUpdateStatus, error) {
	result, err := nbc.CheckUpdate(ctx)
	if err != nil {
		return nil, err
	}

	return &NBCUpdateStatus{
		UpdateNeeded:  result.UpdateNeeded,
		NewDigest:     result.NewDigest,
		CurrentDigest: result.CurrentDigest,
	}, nil
}

// CountFlatpakUpdates returns the number of available Flatpak updates.
// Returns 0 if Flatpak is not installed or if there's an error.
func CountFlatpakUpdates() int {
	if !pm.FlatpakIsInstalled() {
		return 0
	}

	updates, err := pm.ListFlatpakUpdates()
	if err != nil {
		return 0
	}

	return len(updates)
}

// CountHomebrewOutdated returns the number of outdated Homebrew packages.
// Returns 0 if Homebrew is not installed or if there's an error.
func CountHomebrewOutdated() int {
	if !pm.HomebrewIsInstalled() {
		return 0
	}

	packages, err := pm.ListHomebrewOutdated()
	if err != nil {
		return 0
	}

	return len(packages)
}
