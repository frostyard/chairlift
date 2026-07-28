// Package bundleview owns the pure presentation and action-state decisions
// used by the Applications page's Brew bundle group.
package bundleview

import (
	"fmt"
	"strings"
	"sync/atomic"
)

const (
	gateIdle uint32 = iota
	gateRunning
	gateComplete
)

// Presentation describes the group-level text after bundle discovery.
// Placeholder fields are non-empty only when there are no bundle rows.
type Presentation struct {
	Description         string
	PlaceholderTitle    string
	PlaceholderSubtitle string
}

// Present derives the bundle group's complete loaded state. warning is empty
// when every existing configured path was read successfully.
func Present(count int, warning string, homebrewAvailable bool) Presentation {
	var result Presentation
	switch {
	case count == 0 && warning == "":
		result = Presentation{
			Description:         "No Brew bundles found",
			PlaceholderTitle:    "No bundles available",
			PlaceholderSubtitle: "Check the configured bundles_paths directories",
		}
	case count == 0:
		result = Presentation{
			Description:         "Brew bundles could not be loaded",
			PlaceholderTitle:    "Bundles unavailable",
			PlaceholderSubtitle: warning,
		}
	default:
		noun := "bundles"
		if count == 1 {
			noun = "bundle"
		}
		result.Description = fmt.Sprintf("%d Brew %s available", count, noun)
		if warning != "" {
			result.Description += "; some configured paths could not be read: " + warning
		}
	}

	if !homebrewAvailable {
		result.Description = strings.TrimSuffix(result.Description, ".") +
			". Homebrew is not installed; install actions are disabled."
	}
	return result
}

// InstallGate prevents overlapping invocations of one bundle row's install
// action. Its zero value is ready for use.
type InstallGate struct {
	state atomic.Uint32
}

// TryStart starts an idle action and reports whether this caller acquired it.
func (g *InstallGate) TryStart() bool {
	return g.state.CompareAndSwap(gateIdle, gateRunning)
}

// Reset makes a failed or dry-run action available again.
func (g *InstallGate) Reset() {
	g.state.CompareAndSwap(gateRunning, gateIdle)
}

// Complete permanently closes a successfully installed bundle action.
func (g *InstallGate) Complete() {
	g.state.CompareAndSwap(gateRunning, gateComplete)
}
