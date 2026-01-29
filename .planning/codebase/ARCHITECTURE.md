# Architecture

**Analysis Date:** 2026-01-26

## Pattern Overview

**Overall:** Layered GTK4/Libadwaita Desktop Application

**Key Characteristics:**
- Pure Go implementation using puregotk bindings (no CGO required)
- Clean separation between UI, business logic, and external tool wrappers
- YAML-driven configuration for page/feature enablement
- Dry-run mode propagated through all state-changing operations
- Async operations with GTK main-thread callback pattern

## Layers

**Entry Point (cmd):**
- Purpose: Application bootstrap and version injection
- Location: `cmd/chairlift/`
- Contains: `main.go` - build version setup, app initialization
- Depends on: `internal/app`, `internal/version`
- Used by: N/A (entry point)

**Application Layer (app):**
- Purpose: GTK Application lifecycle, keyboard shortcuts, activation
- Location: `internal/app/`
- Contains: `app.go` - Application struct wrapping adw.Application
- Depends on: `internal/window`, `internal/nbc`, `internal/updex`, `internal/instex`, `internal/pm`
- Used by: `cmd/chairlift/main.go`

**Window Layer (window):**
- Purpose: Main window UI composition, navigation, dialogs
- Location: `internal/window/`
- Contains: `window.go` - NavigationSplitView layout, sidebar, content stack
- Depends on: `internal/views`, `internal/config`, `internal/version`
- Used by: `internal/app`

**Views Layer (views):**
- Purpose: Page content, UI actions, async data loading
- Location: `internal/views/`
- Contains: `userhome.go` - All page builders and event handlers
- Depends on: `internal/config`, `internal/nbc`, `internal/updex`, `internal/instex`, `internal/pm`
- Used by: `internal/window`

**Configuration Layer (config):**
- Purpose: YAML config loading, feature enablement
- Location: `internal/config/`
- Contains: `config.go` - Config struct, loading, defaults
- Depends on: None (uses gopkg.in/yaml.v3)
- Used by: `internal/window`, `internal/views`

**External Tool Wrappers:**
- Purpose: Interface with system tools (nbc, updex, instex, brew, flatpak, snap)
- Location: `internal/nbc/`, `internal/updex/`, `internal/instex/`, `internal/pm/`
- Contains: Command execution, JSON parsing, dry-run support
- Depends on: External CLI tools via exec
- Used by: `internal/views`

**Version Package:**
- Purpose: Build-time version info storage
- Location: `internal/version/`
- Contains: `version.go` - Version/Commit/Date vars
- Depends on: None
- Used by: `cmd/chairlift`, `internal/window`

## Data Flow

**Application Startup:**

1. `main.go` sets version info and creates Application via `app.New()`
2. `app.New()` initializes package managers (Flatpak, Snap, Homebrew) and checks dry-run flag
3. On activate signal, `window.New()` creates main window with navigation split view
4. Window creates `views.UserHome` which asynchronously builds page content
5. Each page loads data via goroutines, updating UI via `runOnMainThread()`

**User Action Flow:**

1. User clicks button (e.g., "Update" on NBC)
2. Button handler disables widget, shows progress state
3. Goroutine executes operation via wrapper package (e.g., `nbc.Update()`)
4. Progress events stream through channel, marshaled to main thread
5. On completion, UI updated via `runOnMainThread()` with success/error toast

**State Management:**
- No global state store; UI state is local to widgets
- Config loaded once at window creation, stored in `UserHome.config`
- Package manager instances are module-level singletons with mutex protection
- Dry-run mode is a module-level flag propagated to all wrappers

## Key Abstractions

**ToastAdder Interface:**
- Purpose: Decouple views from window for toast notifications
- Examples: `internal/views/userhome.go:57-62`
- Pattern: Dependency injection via constructor

**Package Manager Wrapper Pattern:**
- Purpose: Consistent interface to diverse package managers
- Examples: `internal/pm/wrapper.go` wraps `github.com/frostyard/pm`
- Pattern: Facade with dry-run support, availability caching

**Progress Event Channels:**
- Purpose: Stream async operation progress to UI
- Examples: `internal/nbc/nbc.go:247` (runNbcCommandStreaming), `internal/views/userhome.go:1529`
- Pattern: Goroutine produces, main thread consumes via channel

**Configuration Groups:**
- Purpose: Feature flags for page sections
- Examples: `internal/config/config.go:22-34`
- Pattern: Hierarchical map lookup with defaults

## Entry Points

**Binary Entry:**
- Location: `cmd/chairlift/main.go`
- Triggers: OS process launch
- Responsibilities: Version injection, app creation, GTK run loop

**GTK Activate:**
- Location: `internal/app/app.go:82` (onActivate)
- Triggers: GTK application activate signal
- Responsibilities: Window creation, present to user

**Page Builders:**
- Location: `internal/views/userhome.go` (buildSystemPage, buildUpdatesPage, etc.)
- Triggers: Called from goroutine in `views.New()`
- Responsibilities: Construct page UI, initiate async data loading

## Error Handling

**Strategy:** Return errors up, display via toast notifications

**Patterns:**
- Wrapper packages return custom Error types (e.g., `nbc.Error`, `nbc.NotFoundError`)
- Views catch errors in goroutine callbacks and show via `toastAdder.ShowErrorToast()`
- Progress streams include EventTypeError for inline error display

## Cross-Cutting Concerns

**Logging:** Go standard library `log` package with `log.SetFlags(log.LstdFlags | log.Lshortfile)`

**Validation:** Minimal; relies on config YAML schema and external tool validation

**Authentication:** None internal; privileged operations use pkexec for PolicyKit auth

**Thread Safety:** Mutex-protected singletons; `runOnMainThread()` for GTK thread marshaling

---

*Architecture analysis: 2026-01-26*
