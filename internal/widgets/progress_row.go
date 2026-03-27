package widgets

import (
	"time"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// ProgressRow displays operation progress with spinner or progress bar.
//
// Short operations show a spinner, operations running longer than 30 seconds
// transition to a progress bar for better feedback on long-running tasks.
//
// If the operation is cancellable, a cancel button is shown. The cancel button
// triggers a callback that should show a confirmation dialog before actually
// cancelling the operation.
//
// All methods that modify the UI must be called from the GTK main thread.
// When updating from goroutines, use [async.RunOnMain].
//
// Example:
//
//	row := widgets.NewProgressRow("Installing packages", true, func() {
//	    // Show confirmation dialog, then cancel if confirmed
//	    showConfirmDialog("Cancel installation?", op.Cancel)
//	})
//	group.Add(&row.Row.Widget)
//
//	go func() {
//	    for progress := range progressChan {
//	        async.RunOnMain(func() {
//	            row.UpdateProgress(progress.Fraction, progress.Message)
//	        })
//	    }
//	    async.RunOnMain(func() {
//	        row.Stop()
//	        group.Remove(&row.Row.Widget)
//	    })
//	}()
type ProgressRow struct {
	// Row is the underlying adw.ActionRow widget.
	// Callers can use this directly to add to containers.
	Row *adw.ActionRow

	spinner     *gtk.Spinner
	progressBar *gtk.ProgressBar
	cancelBtn   *gtk.Button

	startedAt  time.Time
	showingBar bool // true if progress bar is visible (after 30s)
}

// spinnerToProgressThreshold is the duration after which we switch from
// spinner to progress bar for operations with known progress.
const spinnerToProgressThreshold = 30 * time.Second

// NewProgressRow creates a row showing operation progress.
//
// Parameters:
//   - title: The operation name (e.g., "Installing packages", "Updating system")
//   - cancellable: Whether to show a cancel button
//   - onCancel: Callback when cancel is clicked (should show confirmation dialog)
//
// The spinner starts automatically. If cancellable is true, a cancel button
// is shown as a suffix.
//
// Must be called from the GTK main thread.
func NewProgressRow(title string, cancellable bool, onCancel func()) *ProgressRow {
	row := adw.NewActionRow()
	row.SetTitle(title)

	// Create and start spinner
	spinner := gtk.NewSpinner()
	spinner.Start()
	row.AddPrefix(&spinner.Widget)

	// Create progress bar (hidden initially)
	progressBar := gtk.NewProgressBar()
	progressBar.SetHexpand(true)
	progressBar.SetShowText(true)
	progressBar.SetVisible(false)
	row.AddSuffix(&progressBar.Widget)

	pr := &ProgressRow{
		Row:         row,
		spinner:     spinner,
		progressBar: progressBar,
		startedAt:   time.Now(),
		showingBar:  false,
	}

	// Add cancel button if cancellable
	if cancellable && onCancel != nil {
		cancelBtn := gtk.NewButtonWithLabel("Cancel")
		cancelBtn.SetValign(gtk.AlignCenterValue)
		cancelBtn.AddCssClass("flat")

		cb := func(_ gtk.Button) {
			onCancel()
		}
		cancelBtn.ConnectClicked(&cb)

		row.AddSuffix(&cancelBtn.Widget)
		pr.cancelBtn = cancelBtn
	}

	return pr
}

// UpdateProgress updates the operation's progress and message.
//
// Parameters:
//   - progress: Value between 0.0 and 1.0, or -1 for indeterminate
//   - message: Status message to show as subtitle
//
// Behavior:
//   - If progress >= 0 and elapsed time > 30s (or already showing bar):
//     hides spinner and shows progress bar with the given fraction
//   - If progress < 0 and showing bar: uses pulse animation
//   - Otherwise: continues showing spinner
//
// Must be called from the GTK main thread.
func (pr *ProgressRow) UpdateProgress(progress float64, message string) {
	pr.Row.SetSubtitle(message)

	elapsed := time.Since(pr.startedAt)
	shouldShowBar := progress >= 0 && (elapsed > spinnerToProgressThreshold || pr.showingBar)

	if shouldShowBar && !pr.showingBar {
		// Transition from spinner to progress bar
		pr.spinner.Stop()
		pr.spinner.SetVisible(false)
		pr.progressBar.SetVisible(true)
		pr.showingBar = true
	}

	if pr.showingBar {
		if progress < 0 {
			// Indeterminate mode - pulse the bar
			pr.progressBar.Pulse()
		} else {
			pr.progressBar.SetFraction(progress)
		}
	}
}

// Stop stops the spinner and hides progress indicators.
//
// Call this when the operation completes (success, failure, or cancellation).
// This method:
//   - Stops the spinner animation
//   - Hides both spinner and progress bar
//   - Hides the cancel button if present
//
// After calling Stop(), the caller should typically remove the row from
// its parent container.
//
// Must be called from the GTK main thread.
func (pr *ProgressRow) Stop() {
	pr.spinner.Stop()
	pr.spinner.SetVisible(false)
	pr.progressBar.SetVisible(false)

	if pr.cancelBtn != nil {
		pr.cancelBtn.SetVisible(false)
	}
}
