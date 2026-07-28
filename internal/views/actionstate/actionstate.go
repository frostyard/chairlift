// Package actionstate owns the pure state transitions used by asynchronous
// Applications- and Updates-page actions. It has no GTK or puregotk
// dependency, so command completion, refresh ordering, and repeated-click
// behavior can be tested headlessly.
package actionstate

import (
	"fmt"
	"sync/atomic"
)

const (
	gateIdle uint32 = iota
	gateRunning
	gateComplete
)

// Gate prevents overlapping invocations of one action. Its zero value is
// ready for use.
type Gate struct {
	state atomic.Uint32
}

// RefreshGate gives refresh requests monotonically increasing generations so
// only the newest request may publish its result. Its zero value is ready.
type RefreshGate struct {
	generation atomic.Uint64
}

// Begin returns the generation assigned to a new refresh request.
func (g *RefreshGate) Begin() uint64 {
	return g.generation.Add(1)
}

// IsCurrent reports whether generation still belongs to the newest request.
func (g *RefreshGate) IsCurrent(generation uint64) bool {
	return generation != 0 && generation == g.generation.Load()
}

// TryStart moves an idle action to running. It reports false while the action
// is already running or after it has completed permanently.
func (g *Gate) TryStart() bool {
	return g.state.CompareAndSwap(gateIdle, gateRunning)
}

// Reset makes a running action available again after a failure, preview, or
// refresh completion.
func (g *Gate) Reset() {
	g.state.CompareAndSwap(gateRunning, gateIdle)
}

// Complete permanently closes a successfully completed row action.
func (g *Gate) Complete() {
	g.state.CompareAndSwap(gateRunning, gateComplete)
}

// Decision describes the UI work following one command attempt.
type Decision struct {
	Refresh         bool
	RemoveRow       bool
	RestoreControl  bool
	CompleteControl bool
}

// RefreshDecision describes whether a completed outdated-package query has
// authoritative replacement rows and which count must accompany them.
type RefreshDecision struct {
	ReplaceRows bool
	Count       int
}

// Presentation is the outdated-packages expander state for a known count.
type Presentation struct {
	Subtitle   string
	Expandable bool
}

// OutdatedPresentation derives singular/plural text and expansion state from
// the current authoritative outdated-package count.
func OutdatedPresentation(count int) Presentation {
	noun := "packages"
	if count == 1 {
		noun = "package"
	}
	return Presentation{
		Subtitle:   fmt.Sprintf("%d %s available", count, noun),
		Expandable: count > 0,
	}
}

// OutdatedRefresh preserves the last known count when a query fails and
// replaces it with the discovered count only after a successful query.
func OutdatedRefresh(succeeded bool, currentCount, discoveredCount int) RefreshDecision {
	if !succeeded {
		return RefreshDecision{Count: currentCount}
	}
	return RefreshDecision{ReplaceRows: true, Count: discoveredCount}
}

// PackageUpgrade distinguishes a command failure, a successful dry-run
// preview, and a successful live package upgrade.
func PackageUpgrade(succeeded, dryRun bool) Decision {
	switch {
	case !succeeded:
		return Decision{RestoreControl: true}
	case dryRun:
		return Decision{RestoreControl: true}
	default:
		return Decision{Refresh: true, RemoveRow: true}
	}
}

// PackageInstall distinguishes a command failure, a successful dry-run
// preview, and a successful live install from a search result. Only the live
// install permanently completes the row control and refreshes installed
// packages.
func PackageInstall(succeeded, dryRun bool) Decision {
	return completedPackageMutation(succeeded, dryRun)
}

// PackageUninstall decides whether an installed-package uninstall control
// resets or completes and whether the installed inventory must refresh.
func PackageUninstall(succeeded, dryRun bool) Decision {
	return completedPackageMutation(succeeded, dryRun)
}

// PackagePin decides whether a formula pin/unpin control resets or completes
// and whether the installed inventory must refresh.
func PackagePin(succeeded, dryRun bool) Decision {
	return completedPackageMutation(succeeded, dryRun)
}

func completedPackageMutation(succeeded, dryRun bool) Decision {
	switch {
	case !succeeded:
		return Decision{RestoreControl: true}
	case dryRun:
		return Decision{RestoreControl: true}
	default:
		return Decision{Refresh: true, CompleteControl: true}
	}
}

// MetadataUpdate distinguishes a command failure, a successful dry-run
// preview, and a successful live Homebrew metadata update. A live success
// keeps its control busy until the requested refresh finishes.
func MetadataUpdate(succeeded, dryRun bool) Decision {
	switch {
	case !succeeded:
		return Decision{RestoreControl: true}
	case dryRun:
		return Decision{RestoreControl: true}
	default:
		return Decision{Refresh: true}
	}
}
