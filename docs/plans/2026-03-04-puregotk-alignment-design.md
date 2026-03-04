# Puregotk Pattern Alignment Design

Date: 2026-03-04

## Goal

Align chairlift with official puregotk patterns from the `examples/myapp-gnome-gomod/` reference implementation. Adopt GObject subclass registration, value embedding, per-page file splitting, and activate guard — without adopting Blueprint UI files.

## Decisions

- **GObject subclassing**: Yes — Application and Window both become proper GObject subtypes via `TypeRegisterStaticSimple`
- **Blueprint/GResource**: No — pages are too data-driven to benefit; keep pure-Go UI construction
- **Value embedding**: Yes — match official pattern (`adw.Application` not `*adw.Application`)
- **File splitting**: One file per page in `internal/views/`
- **Sequencing**: Bottom-up — structural split first, then embedding, then GObject subclassing

## Changes

### 1. Split userhome.go into per-page files

Pure mechanical refactor. No behavior changes.

| File | Contents |
|------|----------|
| `views.go` | `UserHome` struct, `New()`, `GetPage()`, `createPage()`, `ToastAdder` interface, `runOnMainThread`, `updateBadgeCount()` |
| `system_page.go` | `buildSystemPage`, `loadOSRelease`, `loadNBCStatus`, `onSystemUpdateClicked` |
| `updates_page.go` | `buildUpdatesPage`, `loadOutdatedPackages`, `loadFlatpakUpdates`, `onNBCCheckUpdateClicked`, `checkNBCUpdateAvailability`, `onNBCUpdateClicked`, `onNBCDownloadClicked`, `onUpdateHomebrewClicked` |
| `applications_page.go` | `buildApplicationsPage`, `loadHomebrewPackages`, `loadFlatpakApplications`, `loadSnapApplications`, `onInstallSnapStoreClicked`, `onHomebrewSearch`, `launchApp` |
| `maintenance_page.go` | `buildMaintenancePage`, `onBrewCleanupClicked`, `onFlatpakCleanupClicked`, `onBrewBundleDumpClicked`, `runMaintenanceAction` |
| `features_page.go` | `buildFeaturesPage`, `loadFeatures`, `checkFeatureUpdates`, `onFeatureToggled`, `onUpdateFeaturesClicked` |
| `help_page.go` | `buildHelpPage`, `openURL` |

### 2. Value embedding

Change pointer embedding to value embedding:

```go
// app.go
type Application struct {
    adw.Application  // was *adw.Application
    dryRun bool
    window *window.Window
}

// window.go
type Window struct {
    adw.ApplicationWindow  // was *adw.ApplicationWindow
    ...
}
```

Update all call sites accordingly.

### 3. GObject subclass registration — Application

Register `Application` as a GObject subtype in `init()`:

- Use `gobject.TypeRegisterStaticSimple` with `adw.ApplicationGLibType()` as parent
- Override `Constructed` to allocate Go struct, pin with `runtime.Pinner`, attach via `SetDataFull`
- Override `Activate` to call `onActivate()` via `GetData(dataKeyGoInstance)` cast
- Constructor uses `gobject.NewObject(gTypeApplication, ...)` instead of `adw.NewApplication()`

Memory management:
- `runtime.Pinner` pins the Go struct to prevent GC
- `glib.DestroyNotify` callback unpins when GObject is destroyed
- `SetDataFull` attaches Go pointer to GObject lifetime

### 4. GObject subclass registration — Window

Same pattern for Window:

- Use `gobject.TypeRegisterStaticSimple` with `adw.ApplicationWindowGLibType()` as parent
- Override `Constructed` to allocate Go struct, pin, attach, then call `buildUI()` and `setupActions()`
- Constructor uses `gobject.NewObject(gTypeWindow, "application", app)`

### 5. Activate guard

`onActivate()` checks if window already exists:

```go
func (a *Application) onActivate() {
    if a.window != nil {
        a.window.Present()
        return
    }
    win := window.New(a.Application)
    a.window = win
    a.AddWindow(&win.Window)
    win.Present()
}
```

### 6. main.go update

Use the new `NewApplication()` constructor that calls `gobject.NewObject` internally:

```go
application := app.NewApplication(
    "application_id", app.AppID,
    "flags", gio.GApplicationDefaultFlagsValue,
)
os.Exit(int(application.Run(int32(len(os.Args)), os.Args)))
```

## What stays the same

- Signal connection pattern (`&namedVar`) — already correct
- `runOnMainThread` via `glib.IdleAdd` — actually better than official examples
- Property setting via typed methods — already correct
- Async goroutine + `runOnMainThread` pattern — already correct
- All page UI construction in Go code — intentional choice
- Dry-run support pattern — unchanged
- `ToastAdder` interface — unchanged

## Risk areas

- GObject subclass registration involves `unsafe.Pointer` casts — must match C struct layouts exactly
- `runtime.Pinner` requires Go 1.21+ (chairlift already uses 1.23+)
- Value embedding changes ripple through many call sites
- Need to verify `OverrideConstructed` and `OverrideActivate` are available in the puregotk version chairlift uses
