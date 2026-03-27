// Package widgets provides reusable GTK4/Libadwaita widget patterns.

package widgets

import (
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// AsyncExpanderRow wraps an [adw.ExpanderRow] with async loading state support.
//
// It manages loading indicators, error display, and content population, reducing
// the boilerplate needed for asynchronous data loading patterns.
//
// All methods that modify the UI must be called from the GTK main thread.
// When updating from goroutines, use [async.RunOnMain].
//
// Example:
//
//	expander := widgets.NewAsyncExpanderRow("NBC Status", "Loading...")
//	expander.StartLoading("Fetching status")
//
//	go func() {
//	    status, err := fetchStatus()
//	    async.RunOnMain(func() {
//	        if err != nil {
//	            expander.SetError(err.Error())
//	            return
//	        }
//	        expander.SetContent("Loaded")
//	        row := adw.NewActionRow()
//	        row.SetTitle("Image")
//	        row.SetSubtitle(status.Image)
//	        expander.Expander.AddRow(&row.Widget)
//	    })
//	}()
type AsyncExpanderRow struct {
	// Expander is the underlying adw.ExpanderRow widget.
	// Callers can use this directly to add rows, set properties, etc.
	Expander *adw.ExpanderRow

	// loadingRow holds the current loading indicator row (nil when not loading)
	loadingRow *adw.ActionRow

	// spinner holds the current spinner (nil when not loading)
	spinner *gtk.Spinner
}

// NewAsyncExpanderRow creates an expander row configured for async data loading.
//
// Parameters:
//   - title: The expander row title (e.g., "NBC Status")
//   - subtitle: Initial subtitle (e.g., "Loading..." or "")
//
// The returned AsyncExpanderRow is ready to use. Call StartLoading before
// initiating async work, then SetContent or SetError when complete.
//
// Must be called from the GTK main thread.
func NewAsyncExpanderRow(title, subtitle string) *AsyncExpanderRow {
	expander := adw.NewExpanderRow()
	expander.SetTitle(title)
	expander.SetSubtitle(subtitle)

	return &AsyncExpanderRow{
		Expander: expander,
	}
}

// StartLoading shows a loading row with spinner inside the expander.
//
// Call this before starting async data fetching. The loading row displays
// the provided message with "Please wait..." as the subtitle and a spinning
// indicator as the prefix.
//
// Must be called from the GTK main thread.
//
// Example:
//
//	expander.StartLoading("Fetching NBC status")
//	go func() {
//	    data := fetchData()
//	    async.RunOnMain(func() {
//	        expander.SetContent("Loaded")
//	        // populate data
//	    })
//	}()
func (a *AsyncExpanderRow) StartLoading(message string) {
	a.loadingRow = adw.NewActionRow()
	a.loadingRow.SetTitle(message)
	a.loadingRow.SetSubtitle("Please wait...")

	a.spinner = gtk.NewSpinner()
	a.spinner.Start()
	a.loadingRow.AddPrefix(&a.spinner.Widget)

	a.Expander.AddRow(&a.loadingRow.Widget)
}

// StopLoading removes the loading row if present.
//
// This is called automatically by SetContent and SetError, but can also
// be called directly if needed. It is safe to call multiple times
// (idempotent) - calling on an already-stopped row is a no-op.
//
// Must be called from the GTK main thread.
func (a *AsyncExpanderRow) StopLoading() {
	if a.loadingRow != nil {
		a.Expander.Remove(&a.loadingRow.Widget)
		a.loadingRow = nil
		a.spinner = nil
	}
}

// SetError displays an error state in the expander.
//
// This method:
//   - Removes any loading indicator (calls StopLoading)
//   - Sets the expander subtitle to "Failed to load"
//   - Adds an error row with dialog-error-symbolic icon and the message
//
// The message should be user-friendly. Consider using [async.UserError.FormatForUser]
// for consistent error formatting.
//
// Must be called from the GTK main thread.
//
// Example:
//
//	if err != nil {
//	    userErr := async.NewUserError("Couldn't load NBC status", err)
//	    expander.SetError(userErr.FormatForUser())
//	    return
//	}
func (a *AsyncExpanderRow) SetError(message string) {
	a.StopLoading()
	a.Expander.SetSubtitle("Failed to load")

	errorRow := adw.NewActionRow()
	errorRow.SetTitle("Error")
	errorRow.SetSubtitle(message)

	errorIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
	errorRow.AddPrefix(&errorIcon.Widget)

	a.Expander.AddRow(&errorRow.Widget)
}

// SetContent marks loading as complete and updates the subtitle.
//
// This method:
//   - Removes any loading indicator (calls StopLoading)
//   - Sets the expander subtitle to the provided value
//
// After calling SetContent, the caller should add content rows directly
// to the Expander field using AddRow.
//
// Must be called from the GTK main thread.
//
// Example:
//
//	expander.SetContent("3 items loaded")
//	for _, item := range items {
//	    row := adw.NewActionRow()
//	    row.SetTitle(item.Name)
//	    expander.Expander.AddRow(&row.Widget)
//	}
func (a *AsyncExpanderRow) SetContent(subtitle string) {
	a.StopLoading()
	a.Expander.SetSubtitle(subtitle)
}
