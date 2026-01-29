package widgets

import (
	"github.com/frostyard/chairlift/internal/operations"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// ActionButton wraps a [gtk.Button] with self-disabling behavior during operations.
//
// It automatically disables itself when an operation starts and re-enables when
// the operation completes, preventing double-clicks and showing visual feedback
// that something is happening.
//
// All methods that modify the UI must be called from the GTK main thread.
// When updating from goroutines, use [async.RunOnMain].
//
// Example:
//
//	btn := widgets.NewActionButtonWithClass("Install", "suggested-action")
//	btn.OnClicked(func(done func()) {
//	    go func() {
//	        err := installPackage()
//	        async.RunOnMain(func() {
//	            done() // Re-enable button
//	            if err != nil {
//	                showError(err)
//	            }
//	        })
//	    }()
//	})
type ActionButton struct {
	// Button is the underlying gtk.Button widget.
	// Callers can use this directly to add to containers, set properties, etc.
	Button *gtk.Button

	// originalLabel stores the button's label to restore after operation completes
	originalLabel string
}

// NewActionButton creates a button that tracks operation state.
//
// Parameters:
//   - label: The button text (e.g., "Install", "Update", "Clean Up")
//
// The button is configured with vertical center alignment for use as a suffix
// in ActionRow widgets.
//
// Must be called from the GTK main thread.
func NewActionButton(label string) *ActionButton {
	btn := gtk.NewButtonWithLabel(label)
	btn.SetValign(gtk.AlignCenterValue)

	return &ActionButton{
		Button:        btn,
		originalLabel: label,
	}
}

// NewActionButtonWithClass creates a button with a CSS class.
//
// Parameters:
//   - label: The button text
//   - cssClass: CSS class to add (e.g., "suggested-action", "destructive-action")
//
// Common CSS classes:
//   - "suggested-action": Blue/accent colored button for primary actions
//   - "destructive-action": Red colored button for dangerous actions
//
// Must be called from the GTK main thread.
func NewActionButtonWithClass(label, cssClass string) *ActionButton {
	ab := NewActionButton(label)
	ab.Button.AddCssClass(cssClass)
	return ab
}

// StartOperation disables the button and shows a working state.
//
// This method:
//   - Sets the button insensitive (disabled, grayed out)
//   - Changes the label to workingLabel (e.g., "Installing...", "Updating...")
//
// Returns a done function that restores the button to its original state.
// The caller MUST call done() when the operation completes, regardless of
// success or failure.
//
// Must be called from the GTK main thread. The returned done function must
// also be called from the main thread.
//
// Example:
//
//	done := btn.StartOperation("Installing...")
//	go func() {
//	    err := doWork()
//	    async.RunOnMain(func() {
//	        done() // Always call done, even on error
//	        if err != nil {
//	            showError(err)
//	        }
//	    })
//	}()
func (ab *ActionButton) StartOperation(workingLabel string) (done func()) {
	ab.Button.SetSensitive(false)
	ab.Button.SetLabel(workingLabel)

	return func() {
		ab.Button.SetSensitive(true)
		ab.Button.SetLabel(ab.originalLabel)
	}
}

// OnClicked connects a click handler that automatically manages button state.
//
// When the button is clicked:
//  1. Button is disabled with "Working..." label
//  2. Handler is called with a done callback
//  3. Handler must call done() when operation completes
//
// The handler receives a done function that re-enables the button and restores
// the original label. The handler should use goroutines for async work and
// call done() via [async.RunOnMain] when complete.
//
// Must be called from the GTK main thread.
//
// Example:
//
//	btn.OnClicked(func(done func()) {
//	    go func() {
//	        result, err := performAsyncWork()
//	        async.RunOnMain(func() {
//	            done()
//	            if err != nil {
//	                showError(err)
//	            } else {
//	                showResult(result)
//	            }
//	        })
//	    }()
//	})
func (ab *ActionButton) OnClicked(handler func(done func())) {
	cb := func(_ gtk.Button) {
		done := ab.StartOperation("Working...")
		handler(done)
	}
	ab.Button.ConnectClicked(&cb)
}

// StartTrackedOperation disables the button and registers the operation with the central registry.
//
// This is like StartOperation but also tracks the operation in the global registry,
// making it visible in the operations popover and enabling cancellation.
//
// Parameters:
//   - workingLabel: Label shown while operation runs (e.g., "Installing...")
//   - name: Operation name for registry (e.g., "Install Firefox")
//   - category: Operation category (operations.CategoryInstall, etc.)
//   - cancellable: Whether operation can be cancelled
//
// Returns:
//   - op: The registered operation (caller can call op.UpdateProgress, etc.)
//   - done: Function to call when operation completes (restores button state)
//
// The caller should:
//  1. Call op.UpdateProgress() to update progress
//  2. Call done() when operation completes (always, even on error)
//  3. Call op.Complete(err) to mark operation finished
//
// Must be called from the GTK main thread.
//
// Example:
//
//	op, done := btn.StartTrackedOperation("Installing...", "Install Firefox", operations.CategoryInstall, true)
//	go func() {
//	    err := performInstall(op) // op.UpdateProgress() can be called here
//	    async.RunOnMain(func() {
//	        op.Complete(err)
//	        done()
//	    })
//	}()
func (ab *ActionButton) StartTrackedOperation(workingLabel, name string, category operations.Category, cancellable bool) (op *operations.Operation, done func()) {
	// Disable button and change label
	ab.Button.SetSensitive(false)
	ab.Button.SetLabel(workingLabel)

	// Register with operations registry
	op = operations.Start(name, category, cancellable)

	// Create done function that restores button state
	return op, func() {
		ab.Button.SetSensitive(true)
		ab.Button.SetLabel(ab.originalLabel)
	}
}
