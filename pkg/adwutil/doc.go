/*
Package adwutil provides reusable GTK4/Libadwaita patterns for Go applications.

This package extracts common patterns from ChairLift into a reusable library,
including async utilities, error handling, and widget helpers.

# Thread Safety

GTK is not thread-safe. All widget operations must occur on the GTK main thread.
Use [RunOnMain] to schedule UI updates from goroutines:

	go func() {
	    result := doWork()
	    adwutil.RunOnMain(func() {
	        label.SetText(result)
	    })
	}()

# Error Handling

The [UserError] type provides user-friendly error messages while preserving
technical details for logging:

	err := adwutil.NewUserErrorWithHint(
	    "Couldn't install Firefox",
	    "Check your internet connection",
	    technicalErr,
	)
	// err.FormatForUser() returns "Couldn't install Firefox: Check your internet connection"
	// err.FormatWithDetails() includes technical error for debugging

# Tone Guidelines

Error messages follow GNOME HIG tone guidelines:
  - Use "Couldn't" not "Failed to" or "Error:"
  - Keep summaries short and action-oriented
  - Include hints only when actionable

# Widgets (coming soon)

The package will provide helper functions for common widget patterns:
  - Empty state displays
  - Action rows with buttons
  - Loading indicators
  - Progress tracking
*/
package adwutil
