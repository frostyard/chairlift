// Package app provides the main ChairLift application
package app

import (
	"log"
	"os"
	"unsafe"

	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/nbc"
	"github.com/frostyard/chairlift/internal/updex"
	"github.com/frostyard/chairlift/internal/window"

	"github.com/frostyard/snowkit/gobj"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
)

const appID = "org.frostyard.ChairLift"

var (
	gTypeApplication gobject.Type
	appRegistry      *gobj.InstanceRegistry
)

// Application wraps the Adwaita Application as a proper GObject subtype
type Application struct {
	adw.Application
	window *window.Window
	dryRun bool
}

func init() {
	gTypeApplication, appRegistry = gobj.RegisterType(gobj.TypeDef{
		ParentGLibType: adw.ApplicationGLibType,
		ClassName:      "ChairLiftApplication",
		ClassInit: func(tc *gobject.TypeClass, reg *gobj.InstanceRegistry) {
			objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))
			objClass.OverrideConstructed(func(o *gobject.Object) {
				parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
				parentObjClass.GetConstructed()(o)

				var parent adw.Application
				o.Cast(&parent)

				app := &Application{Application: parent}
				reg.Pin(o, unsafe.Pointer(app))
			})

			appClass := (*gio.ApplicationClass)(unsafe.Pointer(tc))
			appClass.OverrideActivate(func(a *gio.Application) {
				ptr := reg.Get(a.GoPointer())
				if ptr == nil {
					log.Fatal("Application instance not found")
				}
				(*Application)(ptr).onActivate()
			})
		},
	})
}

// New creates a new ChairLift application
func New() *Application {
	obj := gobject.NewObject(gTypeApplication, "application_id", appID, "flags", gio.GApplicationFlagsNoneValue)
	if obj == nil {
		log.Fatal("Failed to create application")
	}

	app := (*Application)(appRegistry.Get(obj.GoPointer()))

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
