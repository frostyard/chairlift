// Package rowset holds the clear-then-repopulate bookkeeping for rows a view
// adds to an expander, so a list that is reloaded never accumulates stale rows
// from a previous load.
//
// It is deliberately free of any puregotk/GTK import — in fact it imports
// nothing outside the standard library — so its logic can be unit-tested on a
// headless host. A test binary for a package that imports puregotk panics while
// resolving GTK/graphene shared libraries at package init, before any test
// function runs, so logic that must be tested cannot live in the view packages.
// See docs/agents/skills/gtk-headless-tests.md.
//
// The tracker never names a widget type: it is generic over the row type, and
// removal is performed by a caller-supplied callback, which is what keeps the
// package dependency-free.
package rowset

// Tracker records the rows a view has added to a container so they can all be
// removed again before the container is repopulated.
//
// The zero value is ready to use. Tracker is a plain value type with no
// locking: main-thread safety is a property of the call site, which must keep
// the clear-and-repopulate sequence inside a single main-thread closure.
type Tracker[T any] struct {
	rows []T
}

// Add records a row that has been added to the container.
func (t *Tracker[T]) Add(row T) {
	t.rows = append(t.rows, row)
}

// Len reports how many rows are currently tracked.
func (t *Tracker[T]) Len() int {
	return len(t.rows)
}

// Clear invokes remove once for every tracked row, in insertion order, then
// forgets them all. It is a no-op on an empty or zero-value Tracker.
func (t *Tracker[T]) Clear(remove func(T)) {
	for _, row := range t.rows {
		remove(row)
	}
	t.rows = nil
}
