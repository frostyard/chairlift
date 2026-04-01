# Puregotk Pattern Alignment Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Align chairlift with official puregotk patterns — GObject subclass registration, value embedding, file splitting, and activate guard.

**Architecture:** Register Application and Window as proper GObject subtypes via `gobject.TypeRegisterStaticSimple`. Use `runtime.Pinner` + `SetDataFull` for memory management. Split the 2128-line `userhome.go` into per-page files. Add activate guard to prevent duplicate windows.

**Tech Stack:** Go 1.24, puregotk v0.0.0-20260226083027, `gobject`, `unsafe`, `runtime`

---

### Task 1: Split userhome.go — Create views.go with shared code

Pure mechanical refactor. Extract the shared infrastructure into `views.go`.

**Files:**
- Create: `internal/views/views.go`
- Modify: `internal/views/userhome.go` (remove lines moved to views.go)

**Step 1: Create `internal/views/views.go`**

Extract these sections from `userhome.go` into `views.go`:
- Package declaration and shared imports (`sync`, `glib`, `adw`, `gtk`, `config`)
- `idleCallbackRegistry` vars (lines 28-33)
- `runOnMainThread` function (lines 35-55)
- `ToastAdder` interface (lines 57-62)
- `UserHome` struct (lines 64-115)
- `New()` constructor (lines 117-141)
- `updateBadgeCount()` (lines 143-152)
- `GetPage()` (lines 154-172)
- `createPage()` (lines 174-193)

The file should have only the imports it actually uses. Each page file will add its own imports.

**Step 2: Build to verify it compiles**

Run: `cd /home/bjk/projects/frostyard/chairlift && CGO_ENABLED=0 go build ./...`
Expected: Success (no errors)

**Step 3: Commit**

```bash
git add internal/views/views.go internal/views/userhome.go
git commit -m "refactor: extract shared views infrastructure into views.go"
```

---

### Task 2: Split userhome.go — Extract system_page.go

**Files:**
- Create: `internal/views/system_page.go`
- Modify: `internal/views/userhome.go` (remove extracted functions)

**Step 1: Create `internal/views/system_page.go`**

Extract these functions from `userhome.go`:
- `buildSystemPage` (starts at line 196)
- `loadOSRelease`
- `loadNBCStatus`
- `onSystemUpdateClicked`

Add package declaration and only the imports these functions need (`fmt`, `log`, `strings`, `config`, `nbc`, `adw`, `glib`, `gtk`).

**Step 2: Build to verify**

Run: `CGO_ENABLED=0 go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add internal/views/system_page.go internal/views/userhome.go
git commit -m "refactor: extract system page into system_page.go"
```

---

### Task 3: Split userhome.go — Extract updates_page.go

**Files:**
- Create: `internal/views/updates_page.go`
- Modify: `internal/views/userhome.go` (remove extracted functions)

**Step 1: Create `internal/views/updates_page.go`**

Extract these functions:
- `buildUpdatesPage`
- `loadOutdatedPackages`
- `loadFlatpakUpdates`
- `onNBCCheckUpdateClicked`
- `checkNBCUpdateAvailability`
- `onNBCUpdateClicked`
- `onNBCDownloadClicked`
- `onUpdateHomebrewClicked`

**Step 2: Build to verify**

Run: `CGO_ENABLED=0 go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add internal/views/updates_page.go internal/views/userhome.go
git commit -m "refactor: extract updates page into updates_page.go"
```

---

### Task 4: Split userhome.go — Extract applications_page.go

**Files:**
- Create: `internal/views/applications_page.go`
- Modify: `internal/views/userhome.go` (remove extracted functions)

**Step 1: Create `internal/views/applications_page.go`**

Extract these functions:
- `buildApplicationsPage`
- `loadHomebrewPackages`
- `loadFlatpakApplications`
- `loadSnapApplications`
- `onInstallSnapStoreClicked`
- `onHomebrewSearch`
- `launchApp`

**Step 2: Build to verify**

Run: `CGO_ENABLED=0 go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add internal/views/applications_page.go internal/views/userhome.go
git commit -m "refactor: extract applications page into applications_page.go"
```

---

### Task 5: Split userhome.go — Extract maintenance_page.go

**Files:**
- Create: `internal/views/maintenance_page.go`
- Modify: `internal/views/userhome.go` (remove extracted functions)

**Step 1: Create `internal/views/maintenance_page.go`**

Extract these functions:
- `buildMaintenancePage`
- `onBrewCleanupClicked`
- `onFlatpakCleanupClicked`
- `onBrewBundleDumpClicked`
- `runMaintenanceAction`

**Step 2: Build to verify**

Run: `CGO_ENABLED=0 go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add internal/views/maintenance_page.go internal/views/userhome.go
git commit -m "refactor: extract maintenance page into maintenance_page.go"
```

---

### Task 6: Split userhome.go — Extract features_page.go

**Files:**
- Create: `internal/views/features_page.go`
- Modify: `internal/views/userhome.go` (remove extracted functions)

**Step 1: Create `internal/views/features_page.go`**

Extract these functions:
- `buildFeaturesPage`
- `loadFeatures`
- `checkFeatureUpdates`
- `onFeatureToggled`
- `onUpdateFeaturesClicked`

**Step 2: Build to verify**

Run: `CGO_ENABLED=0 go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add internal/views/features_page.go internal/views/userhome.go
git commit -m "refactor: extract features page into features_page.go"
```

---

### Task 7: Split userhome.go — Extract help_page.go and delete userhome.go

**Files:**
- Create: `internal/views/help_page.go`
- Delete: `internal/views/userhome.go` (should be empty after all extractions)

**Step 1: Create `internal/views/help_page.go`**

Extract these functions:
- `buildHelpPage`
- `openURL`

**Step 2: Delete userhome.go**

After extracting all functions, `userhome.go` should be empty (just package declaration and unused imports). Delete it.

**Step 3: Build to verify**

Run: `CGO_ENABLED=0 go build ./...`
Expected: Success

**Step 4: Commit**

```bash
git add internal/views/help_page.go
git rm internal/views/userhome.go
git commit -m "refactor: extract help page, remove userhome.go"
```

---

### Task 8: GObject subclass registration — Application

Rewrite `internal/app/app.go` to register Application as a proper GObject subtype with value embedding.

**Files:**
- Modify: `internal/app/app.go`
- Modify: `cmd/chairlift/main.go`

**Step 1: Rewrite app.go**

Replace the entire file with the GObject subclass pattern:

```go
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
	AppID            = "org.frostyard.ChairLift"
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
			myApp := (*Application)(unsafe.Pointer(a.GetData(dataKeyGoInstance)))
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
	obj := gobject.NewObject(gTypeApplication, "application_id", AppID, "flags", gio.GApplicationDefaultFlagsValue)
	if obj == nil {
		log.Fatal("Failed to create application")
	}

	app := (*Application)(unsafe.Pointer(obj.GetData(dataKeyGoInstance)))

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

```

Key changes from current code:
- `adw.Application` embedded by value (was `*adw.Application`)
- GObject subclass registered in `init()` with `TypeRegisterStaticSimple`
- `OverrideConstructed` allocates Go struct, pins with `runtime.Pinner`, attaches via `SetDataFull`
- `OverrideActivate` replaces `ConnectActivate`
- Activate guard: checks `a.window != nil` before creating new window
- `New()` uses `gobject.NewObject` + `GetData` to retrieve pinned Go struct
- New imports: `runtime`, `unsafe`, `gobject`
- Removed: `SetDryRun`/`IsDryRun` methods (unused externally)
- Exported `AppID` constant (needed by main.go)

**Step 2: Build to check for compile errors**

Run: `CGO_ENABLED=0 go build ./...`
Expected: May fail due to window.New signature change needed in Task 9. If so, note the error and proceed to Task 9.

**Step 3: Commit (if compiles) or continue to Task 9**

```bash
git add internal/app/app.go
git commit -m "feat: register Application as GObject subtype with activate guard"
```

---

### Task 9: GObject subclass registration — Window

Rewrite `internal/window/window.go` to register Window as a proper GObject subtype with value embedding.

**Files:**
- Modify: `internal/window/window.go`

**Step 1: Update Window struct and constructor**

Change the Window struct to use value embedding:

```go
type Window struct {
	adw.ApplicationWindow  // was *adw.ApplicationWindow

	splitView    *adw.NavigationSplitView
	sidebarList  *gtk.ListBox
	contentStack *gtk.Stack
	contentPage  *adw.NavigationPage
	toasts       *adw.ToastOverlay

	pages       map[string]*adw.ToolbarView
	navRows     map[string]*adw.ActionRow
	config      *config.Config
	views       *views.UserHome
	updateBadge *gtk.Button
}
```

Add GObject subclass registration:

```go
const dataKeyGoInstance = "go_instance"

var gTypeWindow gobject.Type

func init() {
	var windowClassInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))
		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.ApplicationWindow
			o.Cast(&parent)

			w := &Window{
				ApplicationWindow: parent,
				pages:             make(map[string]*adw.ToolbarView),
				navRows:           make(map[string]*adw.ActionRow),
				config:            config.Load(),
			}

			var pinner runtime.Pinner
			pinner.Pin(w)
			var cleanup glib.DestroyNotify = func(data uintptr) {
				pinner.Unpin()
			}
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(w)), &cleanup)

			w.SetDefaultSize(900, 700)
			w.SetTitle("ChairLift")
			w.buildUI()
			w.setupActions()
		})
	}

	var windowInstanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	var windowParentQuery gobject.TypeQuery
	gobject.NewTypeQuery(adw.ApplicationWindowGLibType(), &windowParentQuery)

	gTypeWindow = gobject.TypeRegisterStaticSimple(
		windowParentQuery.Type,
		"ChairLiftWindow",
		windowParentQuery.ClassSize,
		&windowClassInit,
		windowParentQuery.InstanceSize,
		&windowInstanceInit,
		0,
	)
}
```

Update the constructor:

```go
// New creates a new main window
func New(app adw.Application) *Window {
	obj := gobject.NewObject(gTypeWindow, "application", &app.Application)
	if obj == nil {
		log.Fatal("Failed to create window")
	}
	return (*Window)(unsafe.Pointer(obj.GetData(dataKeyGoInstance)))
}
```

Key changes:
- `adw.ApplicationWindow` embedded by value (was `*adw.ApplicationWindow`)
- `New()` takes `adw.Application` by value (was `*adw.Application`)
- GObject subclass registered in `init()`
- All initialization moved into `OverrideConstructed`
- `New()` uses `gobject.NewObject` + `GetData`
- New imports: `runtime`, `unsafe`, `gobject`, `log`

**Step 2: Fix any call site issues**

Check if `window.New` is called anywhere else and update signatures. The main caller is `app.go` which was updated in Task 8 to pass `a.Application` (value).

Also check all places where `w.Window` or `w.ApplicationWindow` is referenced — with value embedding, `w.Window` still works as a field access on the embedded struct.

**Step 3: Build to verify**

Run: `CGO_ENABLED=0 go build ./...`
Expected: Success

**Step 4: Commit**

```bash
git add internal/window/window.go
git commit -m "feat: register Window as GObject subtype with value embedding"
```

---

### Task 10: Update main.go

**Files:**
- Modify: `cmd/chairlift/main.go`

**Step 1: Simplify main.go**

The main change is removing `defer application.Unref()` since the GObject subclass handles its own lifecycle. The `New()` function signature hasn't changed (still returns `*Application`), so the call site should be the same.

Verify the current main.go still works with the new `app.New()`. The `Run` method and `Unref` should still be available through the embedded `adw.Application`.

Check if `Unref()` is still needed — with GObject subclass registration, the GObject system manages the lifecycle. However, `Unref` is still valid to call as a courtesy. Keep it if it compiles, remove if not.

**Step 2: Build and run**

Run: `CGO_ENABLED=0 go build ./cmd/chairlift && echo "Build OK"`
Expected: "Build OK"

Run: `./build/chairlift --dry-run` (manual test — verify window appears and navigation works)
Expected: Application launches, all pages accessible, navigation works, about dialog shows version

**Step 3: Commit (if any changes were needed)**

```bash
git add cmd/chairlift/main.go
git commit -m "chore: update main.go for GObject subclass changes"
```

---

### Task 11: Final verification and cleanup

**Files:**
- Check: All files in `internal/app/`, `internal/window/`, `internal/views/`

**Step 1: Run linter**

Run: `make lint`
Expected: No new lint errors

**Step 2: Run formatter**

Run: `make fmt`
Expected: No formatting changes (or fix any that appear)

**Step 3: Run full build**

Run: `make build`
Expected: Success

**Step 4: Manual smoke test**

Run: `make run` (launches with --dry-run)
Expected:
- Application window appears
- All 6 navigation pages are accessible via sidebar
- Keyboard shortcuts work (Alt+1 through Alt+6, Ctrl+Q)
- About dialog shows correct version
- Re-launching while running reuses existing window (activate guard)

**Step 5: Final commit if any cleanup was needed**

```bash
git add -A
git commit -m "chore: cleanup after puregotk alignment"
```

---

## Notes for the implementer

### API signatures reference (puregotk v0.0.0-20260226083027)

```go
// Type registration
gobject.TypeRegisterStaticSimple(ParentType types.GType, TypeName string, ClassSize uint32, ClassInit *ClassInitFunc, InstanceSize uint32, InstanceInit *InstanceInitFunc, Flags TypeFlags) types.GType
gobject.NewTypeQuery(Type types.GType, Query *TypeQuery)  // fills Query in place

// Callback types
type ClassInitFunc    func(*TypeClass, uintptr)
type InstanceInitFunc func(*TypeInstance, *TypeClass)

// Object methods
func (x *Object) SetDataFull(Key string, Data uintptr, Destroy *glib.DestroyNotify)
func (x *Object) GetData(Key string) uintptr
func (o Object) Cast(v Ptr)  // sets v.SetGoPointer(o.GoPointer())
func (x *TypeClass) PeekParent() *TypeClass

// GType accessors
adw.ApplicationGLibType() types.GType
adw.ApplicationWindowGLibType() types.GType

// Class overrides (cast TypeClass via unsafe.Pointer)
(*gobject.ObjectClass).OverrideConstructed(cb func(*Object))
(*gobject.ObjectClass).GetConstructed() func(*Object)
(*gio.ApplicationClass).OverrideActivate(cb func(*Application))
```

### C struct inheritance — why unsafe.Pointer casts work

All GObject class structs embed their parent at offset 0:
```
AdwApplicationClass → GtkApplicationClass → GApplicationClass → GObjectClass
```
So `(*gio.ApplicationClass)(unsafe.Pointer(tc))` is valid when `tc` points to an `AdwApplicationClass`.

### The `gobject.Type` type

This is `types.GType` which is `uintptr`. The `gTypeApplication` and `gTypeWindow` variables declared at package level hold the registered type IDs.
