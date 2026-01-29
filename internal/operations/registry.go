// Package operations provides async operation tracking for the UI.
//
// This package re-exports functionality from pkg/adwutil for backward compatibility.
package operations

import (
	"context"

	"github.com/frostyard/chairlift/pkg/adwutil"
)

// Listener is an alias for [adwutil.Listener].
type Listener = adwutil.Listener

// Registry is an alias for [adwutil.Registry].
type Registry = adwutil.Registry

// Start creates a new operation.
func Start(name string, category Category, cancellable bool) *Operation {
	return adwutil.Start(name, category, cancellable)
}

// StartWithContext creates a cancellable operation with a derived context.
func StartWithContext(ctx context.Context, name string, category Category) (*Operation, context.Context) {
	return adwutil.StartWithContext(ctx, name, category)
}

// Get returns the operation with the given ID.
func Get(id uint64) *Operation {
	return adwutil.Get(id)
}

// ActiveCount returns the number of active operations.
func ActiveCount() int {
	return adwutil.ActiveCount()
}

// Active returns a copy of all active operations.
func Active() []*Operation {
	return adwutil.Active()
}

// History returns a copy of the completed operations history.
func History() []*Operation {
	return adwutil.History()
}

// AddListener adds a listener.
func AddListener(fn Listener) {
	adwutil.AddListener(fn)
}
