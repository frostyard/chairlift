// Example: operations
//
// Demonstrates adwutil operations tracking: starting, progress, completion, cancellation.
//
// Run with: go run main.go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/frostyard/chairlift/pkg/adwutil"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

var (
	activeLabel *gtk.Label
	historyBox  *gtk.Box
)

func main() {
	app := adw.NewApplication("com.example.AdwutilOperations", gio.GApplicationFlagsNoneValue)

	onActivate := func(_ gio.Application) {
		activate(app)
	}
	app.ConnectActivate(&onActivate)

	if code := app.Run(len(os.Args), os.Args); code > 0 {
		os.Exit(code)
	}
}

func activate(app *adw.Application) {
	window := adw.NewApplicationWindow(&app.Application)
	window.SetTitle("adwutil Operations Example")
	window.SetDefaultSize(500, 400)

	// Listen for operation updates
	adwutil.AddListener(onOperationUpdate)

	// Create main layout
	mainBox := gtk.NewBox(gtk.OrientationVerticalValue, 12)
	mainBox.SetMarginTop(24)
	mainBox.SetMarginBottom(24)
	mainBox.SetMarginStart(24)
	mainBox.SetMarginEnd(24)

	// Active operations counter
	activeLabel = gtk.NewLabel("Active operations: 0")
	activeLabel.AddCssClass("title-3")
	mainBox.Append(&activeLabel.Widget)

	// Actions group
	actionsGroup := adw.NewPreferencesGroup()
	actionsGroup.SetTitle("Start Operations")

	// Simple operation
	row1 := adwutil.NewButtonRow(
		"Quick Task",
		"Completes in 2 seconds",
		"Start",
		func() {
			startSimpleOperation("Quick Task", 2*time.Second)
		},
	)
	actionsGroup.Add(&row1.Widget)

	// Operation with progress
	row2 := adwutil.NewButtonRow(
		"Download Simulation",
		"Shows progress updates",
		"Start",
		func() {
			startProgressOperation()
		},
	)
	actionsGroup.Add(&row2.Widget)

	// Cancellable operation
	row3 := adwutil.NewButtonRow(
		"Long Task (Cancellable)",
		"Can be cancelled after 5 seconds",
		"Start",
		func() {
			startCancellableOperation()
		},
	)
	actionsGroup.Add(&row3.Widget)

	// Failing operation
	row4 := adwutil.NewButtonRowWithClass(
		"Failing Task",
		"Will fail after 1 second",
		"Start",
		"destructive-action",
		func() {
			startFailingOperation()
		},
	)
	actionsGroup.Add(&row4.Widget)

	mainBox.Append(&actionsGroup.Widget)

	// History group
	historyGroup := adw.NewPreferencesGroup()
	historyGroup.SetTitle("Operation History")

	historyBox = gtk.NewBox(gtk.OrientationVerticalValue, 6)
	historyLabel := gtk.NewLabel("Completed operations will appear here")
	historyLabel.AddCssClass("dim-label")
	historyBox.Append(&historyLabel.Widget)

	historyGroup.Add(&historyBox.Widget)
	mainBox.Append(&historyGroup.Widget)

	// Wrap in scrolled window
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetChild(&mainBox.Widget)

	// Wrap in toolbar view
	toolbarView := adw.NewToolbarView()
	headerBar := adw.NewHeaderBar()
	toolbarView.AddTopBar(&headerBar.Widget)
	toolbarView.SetContent(&scrolled.Widget)

	window.SetContent(&toolbarView.Widget)
	window.Present()
}

func startSimpleOperation(name string, duration time.Duration) {
	op := adwutil.Start(name, adwutil.CategoryMaintenance, false)

	go func() {
		time.Sleep(duration)
		adwutil.RunOnMain(func() {
			op.Complete(nil)
		})
	}()
}

func startProgressOperation() {
	op := adwutil.Start("Downloading...", adwutil.CategoryInstall, false)

	go func() {
		for i := 0; i <= 10; i++ {
			progress := float64(i) / 10.0
			message := fmt.Sprintf("Downloaded %d%%", i*10)

			adwutil.RunOnMain(func() {
				op.UpdateProgress(progress, message)
			})

			time.Sleep(300 * time.Millisecond)
		}

		adwutil.RunOnMain(func() {
			op.Complete(nil)
		})
	}()
}

func startCancellableOperation() {
	op, ctx := adwutil.StartWithContext(context.Background(), "Long Task", adwutil.CategoryUpdate)

	go func() {
		select {
		case <-time.After(30 * time.Second):
			adwutil.RunOnMain(func() {
				op.Complete(nil)
			})
		case <-ctx.Done():
			// Operation was cancelled
			fmt.Println("Operation cancelled!")
		}
	}()

	// After 6 seconds, cancel it (demonstrating cancellation)
	go func() {
		time.Sleep(6 * time.Second)
		adwutil.RunOnMain(func() {
			if op.IsCancellable() {
				fmt.Println("Cancelling operation...")
				op.Cancel()
			}
		})
	}()
}

func startFailingOperation() {
	op := adwutil.Start("Failing Task", adwutil.CategoryInstall, false)

	go func() {
		time.Sleep(1 * time.Second)

		err := adwutil.NewUserErrorWithHint(
			"Couldn't complete task",
			"This is a simulated failure",
			fmt.Errorf("simulated error"),
		)

		adwutil.RunOnMain(func() {
			op.Complete(err)
		})
	}()
}

func onOperationUpdate(op *adwutil.Operation) {
	// Update active count
	count := adwutil.ActiveCount()
	activeLabel.SetText(fmt.Sprintf("Active operations: %d", count))

	// Add to history if completed/failed/cancelled
	if op.State != adwutil.StateActive {
		addToHistory(op)
	}
}

func addToHistory(op *adwutil.Operation) {
	stateStr := "?"
	iconName := "object-select-symbolic"

	switch op.State {
	case adwutil.StateCompleted:
		stateStr = "Completed"
		iconName = "object-select-symbolic"
	case adwutil.StateFailed:
		stateStr = "Failed"
		iconName = "dialog-error-symbolic"
	case adwutil.StateCancelled:
		stateStr = "Cancelled"
		iconName = "process-stop-symbolic"
	}

	row := adwutil.NewIconRow(
		op.Name,
		fmt.Sprintf("%s in %v", stateStr, op.Duration().Round(time.Millisecond)),
		iconName,
	)

	historyBox.Prepend(&row.Widget)
}
