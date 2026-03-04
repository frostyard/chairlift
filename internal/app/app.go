// Package app provides the main ChairLift application
package app

import (
	"log"
	"os"
	"runtime"
	"unsafe"

	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/nbc"
	"github.com/frostyard/chairlift/internal/updex"
	"github.com/frostyard/chairlift/internal/window"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

const (
	appID             = "org.frostyard.ChairLift"
	dataKeyGoInstance = "go_instance"
)

var gTypeApplication gobject.Type

// Application wraps the Adwaita Application as a proper GObject subtype
type Application struct {
	adw.Application
	window *window.Window
	dryRun bool
}

func init() {
	var appClassInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		// Override Constructed to initialize the Go struct
		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))
		objClass.OverrideConstructed(func(o *gobject.Object) {
			// Chain up to parent
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			// Cast to adw.Application
			var parent adw.Application
			o.Cast(&parent)

			// Allocate Go struct and pin it
			app := &Application{Application: parent}
			var pinner runtime.Pinner
			pinner.Pin(app)

			// Attach Go pointer to GObject lifetime
			var cleanup glib.DestroyNotify = func(data uintptr) {
				pinner.Unpin()
			}
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(app)), &cleanup)
		})

		// Override Activate for app lifecycle
		appClass := (*gio.ApplicationClass)(unsafe.Pointer(tc))
		appClass.OverrideActivate(func(a *gio.Application) {
			myApp := (*Application)(unsafe.Pointer(a.GetData(dataKeyGoInstance))) //nolint:govet // puregotk GObject pattern: retrieve pinned Go struct from GObject data
			myApp.onActivate()
		})
	}

	var appInstanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	var appParentQuery gobject.TypeQuery
	gobject.NewTypeQuery(adw.ApplicationGLibType(), &appParentQuery)

	gTypeApplication = gobject.TypeRegisterStaticSimple(
		appParentQuery.Type,
		"ChairLiftApplication",
		appParentQuery.ClassSize,
		&appClassInit,
		appParentQuery.InstanceSize,
		&appInstanceInit,
		0,
	)
}

// New creates a new ChairLift application
func New() *Application {
	obj := gobject.NewObject(gTypeApplication, "application_id", appID, "flags", gio.GApplicationFlagsNoneValue)
	if obj == nil {
		log.Fatal("Failed to create application")
	}

	app := (*Application)(unsafe.Pointer(obj.GetData(dataKeyGoInstance))) //nolint:govet // puregotk GObject pattern: retrieve pinned Go struct from GObject data

	// Check for --dry-run flag before GTK processes args
	for _, arg := range os.Args[1:] {
		if arg == "--dry-run" || arg == "-d" {
			log.Println("Running in dry-run mode")
			app.dryRun = true
			flatpak.SetDryRun(true)
			homebrew.SetDryRun(true)
			nbc.SetDryRun(true)
			updex.SetDryRun(true)
			break
		}
	}

	// Set up keyboard shortcuts
	app.setupKeyboardShortcuts()

	// Register command line options
	app.registerOptions()

	return app
}

// onActivate is called when the application is activated
func (a *Application) onActivate() {
	log.Println("ChairLift activated")

	// Guard: reuse existing window if already created
	if a.window != nil {
		a.window.Present()
		return
	}

	// Create and present the main window
	win := window.New(a.Application)
	a.window = win
	a.AddWindow(&win.Window)
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
	a.SetAccelsForAction("win.navigate-features", []string{"<Alt>5"})
	a.SetAccelsForAction("win.navigate-help", []string{"<Alt>6"})
}

// registerOptions registers command line options
func (a *Application) registerOptions() {
	a.AddMainOption(
		"dry-run",
		'd',
		glib.GOptionFlagNoneValue,
		glib.GOptionArgNoneValue,
		"Don't make any changes to the system.",
		"",
	)
}

// GetGtkApplication returns the underlying GTK Application
func (a *Application) GetGtkApplication() *gtk.Application {
	return &a.Application.Application
}
