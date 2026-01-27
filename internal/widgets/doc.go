// Package widgets provides reusable GTK4/Libadwaita widget patterns for Go/puregotk.
//
// This package extracts common UI patterns from chairlift into composable, reusable
// components. It uses a composition-based approach rather than inheritance because
// puregotk doesn't support Go-level GObject subclassing.
//
// # Design Approach
//
// Each "widget" in this package is actually a struct that:
//   - Holds references to one or more GTK/Libadwaita widgets
//   - Provides a factory function (NewXxx) that creates and configures the widgets
//   - Provides helper methods for common operations on those widgets
//
// This composition pattern is idiomatic Go and works naturally with puregotk's
// wrapper types. The underlying GTK widgets are exposed as public fields so callers
// can perform any GTK operations directly when needed.
//
// # Thread Safety
//
// IMPORTANT: GTK is not thread-safe. All methods in this package that touch GTK
// widgets must be called from the GTK main thread. When updating widgets from
// goroutines, use [async.RunOnMain]:
//
//	go func() {
//	    data, err := fetchData()
//	    async.RunOnMain(func() {
//	        expander.SetContent("Loaded")
//	        // Add content rows here
//	    })
//	}()
//
// # Available Widgets
//
// AsyncExpanderRow: An [adw.ExpanderRow] wrapper with loading state management.
// Handles showing a spinner while data loads, displaying errors, and populating
// content when data is ready.
//
//	expander := widgets.NewAsyncExpanderRow("NBC Status", "Loading...")
//	expander.StartLoading("Fetching status")
//	go func() {
//	    status, err := fetchStatus()
//	    async.RunOnMain(func() {
//	        if err != nil {
//	            expander.SetError(err.Error())
//	            return
//	        }
//	        expander.SetContent("Loaded")
//	        // Add content rows to expander.Expander
//	    })
//	}()
//
// ActionButton: A [gtk.Button] wrapper that self-disables during operations.
// (Coming in 02-02-PLAN.md)
//
// LoadingRow: A pre-configured [adw.ActionRow] with spinner for loading states.
// (Coming in 02-02-PLAN.md)
//
// Row builders: Factory functions for common ActionRow configurations.
// (Coming in 02-02-PLAN.md)
//
// # References
//
// For threading utilities, see the [async] package.
// For error handling, see [async.UserError].
package widgets
