// Example: basic
//
// Demonstrates basic adwutil usage: RunOnMain, widgets, and error handling.
//
// Run with: go run main.go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/frostyard/chairlift/pkg/adwutil"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

func main() {
	app := adw.NewApplication("com.example.AdwutilBasic", gio.GApplicationFlagsNoneValue)

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
	window.SetTitle("adwutil Basic Example")
	window.SetDefaultSize(400, 300)

	// Create main content
	box := gtk.NewBox(gtk.OrientationVerticalValue, 12)
	box.SetMarginTop(24)
	box.SetMarginBottom(24)
	box.SetMarginStart(24)
	box.SetMarginEnd(24)

	// Status label
	statusLabel := gtk.NewLabel("Ready")
	statusLabel.AddCssClass("title-2")
	box.Append(&statusLabel.Widget)

	// Preferences group with widget examples
	group := adw.NewPreferencesGroup()
	group.SetTitle("Widget Examples")

	// Button row - demonstrates NewButtonRow
	row1 := adwutil.NewButtonRow(
		"Async Operation",
		"Click to start a background task",
		"Start",
		func() {
			statusLabel.SetText("Starting...")
			go func() {
				// Simulate work
				time.Sleep(2 * time.Second)

				// Update UI safely from goroutine
				adwutil.RunOnMain(func() {
					statusLabel.SetText("Completed!")
				})
			}()
		},
	)
	group.Add(&row1.Widget)

	// Link row - demonstrates NewLinkRow
	row2 := adwutil.NewLinkRow(
		"Documentation",
		"View adwutil docs on GitHub",
		func() {
			fmt.Println("Would open: https://github.com/frostyard/chairlift/tree/main/pkg/adwutil")
		},
	)
	group.Add(&row2.Widget)

	// Info row - demonstrates NewInfoRow
	row3 := adwutil.NewInfoRow("Version", "1.0.0")
	group.Add(&row3.Widget)

	// Icon row - demonstrates NewIconRow
	row4 := adwutil.NewIconRow("Status", "All systems operational", "object-select-symbolic")
	group.Add(&row4.Widget)

	box.Append(&group.Widget)

	// Error handling example
	errGroup := adw.NewPreferencesGroup()
	errGroup.SetTitle("Error Handling")

	errRow := adwutil.NewButtonRow(
		"Simulate Error",
		"Demonstrates UserError formatting",
		"Trigger",
		func() {
			// Create a user-friendly error
			err := adwutil.NewUserErrorWithHint(
				"Couldn't complete operation",
				"Try again in a few moments",
				fmt.Errorf("network timeout after 30s"),
			)

			// Show user message
			statusLabel.SetText(err.FormatForUser())

			// Technical details available for logging
			fmt.Println("Technical:", err.FormatWithDetails())
		},
	)
	errGroup.Add(&errRow.Widget)

	box.Append(&errGroup.Widget)

	// Empty state example
	emptyGroup := adw.NewPreferencesGroup()
	emptyGroup.SetTitle("Empty State")

	emptyState := adwutil.NewEmptyState(adwutil.EmptyStateConfig{
		Title:       "No Items",
		Description: "Items will appear here when added",
		IconName:    "folder-symbolic",
		Compact:     true,
	})
	emptyGroup.Add(&emptyState.Widget)

	box.Append(&emptyGroup.Widget)

	// Wrap in scrolled window
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetChild(&box.Widget)

	// Wrap in toolbar view
	toolbarView := adw.NewToolbarView()
	headerBar := adw.NewHeaderBar()
	toolbarView.AddTopBar(&headerBar.Widget)
	toolbarView.SetContent(&scrolled.Widget)

	window.SetContent(&toolbarView.Widget)
	window.Present()
}
