package widgets

import (
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// LoadingRow represents an [adw.ActionRow] with a spinner indicating loading state.
//
// It provides a consistent loading indicator pattern for use in expanders and
// preference groups while async data is being fetched.
//
// All methods that modify the UI must be called from the GTK main thread.
// When updating from goroutines, use [async.RunOnMain].
//
// Example:
//
//	loading := widgets.NewLoadingRow("Loading packages...", "Please wait")
//	expander.AddRow(&loading.Row.Widget)
//
//	go func() {
//	    packages, err := fetchPackages()
//	    async.RunOnMain(func() {
//	        loading.Stop()
//	        expander.Remove(&loading.Row.Widget)
//	        if err != nil {
//	            showError(err)
//	            return
//	        }
//	        // Add package rows
//	    })
//	}()
type LoadingRow struct {
	// Row is the underlying adw.ActionRow widget.
	// Callers can use this directly to add to containers.
	Row *adw.ActionRow

	// Spinner is the spinning indicator shown as the row prefix.
	Spinner *gtk.Spinner
}

// NewLoadingRow creates a row with a spinning indicator.
//
// Parameters:
//   - title: The main text (e.g., "Loading packages...", "Fetching status")
//   - subtitle: Secondary text (e.g., "Please wait...", "This may take a moment")
//
// The spinner starts automatically. The caller is responsible for removing
// the row from its parent when loading completes.
//
// Must be called from the GTK main thread.
func NewLoadingRow(title, subtitle string) *LoadingRow {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitle(subtitle)

	spinner := gtk.NewSpinner()
	spinner.Start()
	row.AddPrefix(&spinner.Widget)

	return &LoadingRow{
		Row:     row,
		Spinner: spinner,
	}
}

// Stop stops the spinner animation.
//
// Call this when loading completes, before removing the row from its parent.
// This method only stops the spinner animation; it does NOT remove the row
// from its parent container - the caller is responsible for that.
//
// Must be called from the GTK main thread.
func (lr *LoadingRow) Stop() {
	lr.Spinner.Stop()
}
