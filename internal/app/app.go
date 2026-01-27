// Package app provides the main ChairLift application
package app

import (
	"log"
	"os"

	"github.com/frostyard/chairlift/internal/instex"
	"github.com/frostyard/chairlift/internal/nbc"
	"github.com/frostyard/chairlift/internal/pm"
	"github.com/frostyard/chairlift/internal/window"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

const (
	appID   = "org.frostyard.ChairLift"
	appName = "ChairLift"
)

// Application wraps the Adwaita Application
type Application struct {
	*adw.Application
	dryRun bool
}

// New creates a new ChairLift application
func New() *Application {
	app := &Application{
		Application: adw.NewApplication(appID, gio.GApplicationFlagsNoneValue),
		dryRun:      false,
	}

	// Check for --dry-run flag in command line args before GTK processes them
	// This is simpler and more reliable than trying to wrap GVariantDict
	for _, arg := range os.Args[1:] {
		if arg == "--dry-run" || arg == "-d" {
			log.Println("Running in dry-run mode")
			app.dryRun = true
			instex.SetDryRun(true)
			nbc.SetDryRun(true)
			pm.SetDryRun(true)
			break
		}
	}

	// Initialize pm managers (without progress reporter initially)
	// Flatpak and Homebrew will be re-initialized with progress callback after window is created
	if err := pm.InitializeFlatpak(nil); err != nil {
		log.Printf("Warning: Failed to initialize Flatpak manager: %v", err)
	}

	if err := pm.InitializeSnap(); err != nil {
		log.Printf("Warning: Failed to initialize Snap manager: %v", err)
	}

	if err := pm.InitializeHomebrew(nil); err != nil {
		log.Printf("Warning: Failed to initialize Homebrew manager: %v", err)
	}

	// Connect activate signal
	activateCb := func(_ gio.Application) {
		app.onActivate()
	}
	app.ConnectActivate(&activateCb)

	// Set up keyboard shortcuts
	app.setupKeyboardShortcuts()

	// Register command line options (for --help display)
	app.registerOptions()

	return app
}

// onActivate is called when the application is activated
func (a *Application) onActivate() {
	log.Println("ChairLift activated")

	// Create and present the main window
	win := window.New(a.Application)
	win.Present()
}

// setupKeyboardShortcuts sets up application-wide keyboard shortcuts
func (a *Application) setupKeyboardShortcuts() {
	a.SetAccelsForAction("app.quit", []string{"<Primary>q"})
	a.SetAccelsForAction("win.show-shortcuts", []string{"<Primary>question"})
	a.SetAccelsForAction("win.navigate-applications", []string{"<Alt>1"})
	a.SetAccelsForAction("win.navigate-maintenance", []string{"<Alt>2"})
	a.SetAccelsForAction("win.navigate-updates", []string{"<Alt>3"})
	a.SetAccelsForAction("win.navigate-system", []string{"<Alt>4"})
	a.SetAccelsForAction("win.navigate-extensions", []string{"<Alt>5"})
	a.SetAccelsForAction("win.navigate-help", []string{"<Alt>6"})
}

// registerOptions registers command line options
func (a *Application) registerOptions() {
	// Add --dry-run option using the simpler AddMainOption API
	a.AddMainOption(
		"dry-run",                               // long name
		'd',                                     // short name
		glib.GOptionFlagNoneValue,               // flags
		glib.GOptionArgNoneValue,                // arg type
		"Don't make any changes to the system.", // description
		"",                                      // arg description
	)
}

// SetDryRun sets whether the application is in dry-run mode
func (a *Application) SetDryRun(dryRun bool) {
	a.dryRun = dryRun
}

// IsDryRun returns whether the application is in dry-run mode
func (a *Application) IsDryRun() bool {
	return a.dryRun
}

// GetGtkApplication returns the underlying GTK Application
func (a *Application) GetGtkApplication() *gtk.Application {
	return &a.Application.Application
}
