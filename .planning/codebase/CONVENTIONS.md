# Coding Conventions

**Analysis Date:** 2026-01-26

## Naming Patterns

**Files:**
- Lowercase with underscores for multi-word: Not applicable (single-word files used)
- Package name matches directory name: `internal/nbc/nbc.go`, `internal/config/config.go`
- Main entry point: `cmd/<appname>/main.go` pattern

**Functions:**
- CamelCase with first letter indicating visibility
- Public functions: `GetStatus()`, `LoadConfig()`, `IsInstalled()`
- Private functions: `runCommand()`, `loadFromPath()`, `isStateChanging()`
- Event handlers use `on` prefix: `onActivate()`, `onShowAbout()`, `onNBCUpdateClicked()`
- Builder methods use `build` prefix: `buildUI()`, `buildSidebar()`, `buildSystemPage()`

**Variables:**
- Local variables: camelCase (`dryRun`, `progressCh`, `toastAdder`)
- Package-level vars: camelCase (`dryRun`, `flatpakManager`, `idleCallbacks`)
- Mutex naming: append `Mu` suffix (`dryRunMu`, `flatpakMu`, `updateCountMu`)

**Types:**
- Structs: PascalCase (`Application`, `UserHome`, `Config`, `Extension`)
- Interfaces: PascalCase with descriptive names (`ToastAdder`, `ProgressCallback`)
- Type aliases: Match original type name (`StatusOutput = types.StatusOutput`)

**Constants:**
- Package-level constants: camelCase for internal, PascalCase for exported
- Command constants: descriptive names (`nbcCommand`, `pkexecCommand`, `DefaultTimeout`)

## Code Style

**Formatting:**
- Tool: `gofmt -s -w .`
- Run via: `make fmt`
- Standard Go formatting applied automatically

**Linting:**
- Tool: `golangci-lint`
- Run via: `make lint`
- Required before committing changes

## Import Organization

**Order:**
1. Standard library imports
2. External dependencies (third-party)
3. Internal project packages

**Example from `internal/app/app.go`:**
```go
import (
    "log"
    "os"

    "github.com/frostyard/chairlift/internal/instex"
    "github.com/frostyard/chairlift/internal/nbc"
    "github.com/frostyard/chairlift/internal/pm"
    "github.com/frostyard/chairlift/internal/updex"
    "github.com/frostyard/chairlift/internal/window"

    "github.com/jwijenbergh/puregotk/v4/adw"
    "github.com/jwijenbergh/puregotk/v4/gio"
    "github.com/jwijenbergh/puregotk/v4/glib"
    "github.com/jwijenbergh/puregotk/v4/gtk"
)
```

**Path Aliases:**
- None used; full import paths preferred

## Error Handling

**Patterns:**
- Return error as last value: `func GetStatus(ctx context.Context) (*StatusOutput, error)`
- Wrap errors with context: `fmt.Errorf("failed to list Homebrew formulae: %w", err)`
- Custom error types for specific cases: `Error{}`, `NotFoundError{}`
- Log errors before returning when helpful: `log.Printf("Error: %v", err)`

**Custom Error Types (from `internal/nbc/nbc.go`):**
```go
type Error struct {
    Message string
}

func (e *Error) Error() string {
    return e.Message
}

type NotFoundError struct {
    Message string
}

func (e *NotFoundError) Error() string {
    return e.Message
}
```

**Error Checking:**
- Always check errors immediately after call
- Use type assertions for specific error handling:
```go
if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
    return "", stderr.String(), &NotFoundError{Message: "nbc not found"}
}
```

## Logging

**Framework:** Standard library `log` package

**Patterns:**
- Set log flags at startup: `log.SetFlags(log.LstdFlags | log.Lshortfile)`
- Log important state changes: `log.Printf("nbc dry-run mode: %v", mode)`
- Log errors with context: `log.Printf("Error: Failed to load NBC status: %v", err)`
- Use `[DRY-RUN]` prefix for dry-run mode logging
- Use `[Progress]` prefix for progress-related logging
- Use `[PM]` prefix for package manager operations

**Dry-Run Logging:**
```go
if dryRun {
    log.Printf("[DRY-RUN] nbc update with options: %+v", opts)
}
```

## Comments

**When to Comment:**
- Package-level documentation required on all packages
- Exported function documentation required
- Complex logic or non-obvious code sections
- TODO/FIXME for known issues

**Package Documentation:**
```go
// Package nbc provides an interface to the nbc bootc container installer
package nbc
```

**Function Documentation:**
```go
// SetDryRun enables/disables dry-run mode
func SetDryRun(mode bool) {
```

**nolint Directives:**
- Used sparingly with justification:
```go
//nolint:unused // Reserved for manual cleanup or batch operations
func (uh *UserHome) cleanupProgressUI() {
```

## Function Design

**Size:**
- Keep functions focused on single responsibility
- Large builder functions acceptable for UI construction (`buildSystemPage`, `buildUpdatesPage`)
- Extract helper functions for repeated patterns

**Parameters:**
- Use struct for 3+ related parameters (e.g., `UpdateOptions`, `InstallOptions`)
- Use `context.Context` as first parameter for operations with timeouts/cancellation
- Use pointer receivers for methods that modify state: `func (uh *UserHome) buildSystemPage()`

**Return Values:**
- Return pointer for structs: `func GetStatus() (*StatusOutput, error)`
- Return slices directly (not pointers): `func ListInstalledSnaps() ([]SnapApplication, error)`
- Return empty collection instead of nil when no error: `return []HomebrewPackage{}, nil`

## Module Design

**Exports:**
- Minimize exported symbols
- Only export what's needed by other packages
- Use unexported helper functions

**Package Organization:**
- One package per `internal/` subdirectory
- Related functionality grouped together
- CLI tool packages in `cmd/<toolname>/`

**Interface Design:**
```go
// ToastAdder is an interface for adding toasts and notifying about updates
type ToastAdder interface {
    ShowToast(message string)
    ShowErrorToast(message string)
    SetUpdateBadge(count int)
}
```

## Async/Concurrency Patterns

**Goroutines for Long Operations:**
```go
go func() {
    ctx, cancel := nbc.DefaultContext()
    defer cancel()
    
    result, err := nbc.GetStatus(ctx)
    
    runOnMainThread(func() {
        // Update UI safely on main thread
        if err != nil {
            uh.toastAdder.ShowErrorToast(fmt.Sprintf("Error: %v", err))
            return
        }
        // Handle success
    })
}()
```

**Main Thread UI Updates:**
- Always use `runOnMainThread()` for UI updates from goroutines
- Uses `glib.IdleAdd()` internally
- Required for GTK thread safety

**Mutexes:**
- Use `sync.RWMutex` for read-heavy data: `dryRunMu`, `flatpakMu`
- Lock order: acquire mutex before modifying shared state
- Always unlock via defer: `defer mu.Unlock()`

**Channels for Progress:**
```go
progressCh := make(chan nbc.ProgressEvent)
go func() {
    defer close(progressCh)
    // Stream events to channel
}()

for event := range progressCh {
    // Process events
}
```

## Configuration Pattern

**Config Loading Priority:**
1. `/etc/chairlift/config.yml` (system-wide - highest priority)
2. `/usr/share/chairlift/config.yml` (package maintainer defaults)
3. `config.yml` (development/source directory)

**Config-Driven UI:**
```go
if uh.config.IsGroupEnabled("system_page", "health_group") {
    // Build and add the group
}
```

## Dry-Run Support

**All State-Changing Operations Must Support Dry-Run:**
```go
if IsDryRun() {
    log.Printf("[DRY-RUN] Would install Flatpak: %s (user=%v)", appID, userScope)
    return nil
}
```

**Package-Level Dry-Run Variable:**
```go
var dryRun = false

func SetDryRun(mode bool) {
    dryRun = mode
}

func IsDryRun() bool {
    return dryRun
}
```

## Commit Message Convention

**Format:** Conventional commits

**Types:**
- `feat:` - New features
- `fix:` - Bug fixes
- `docs:` - Documentation changes
- `chore:` - Maintenance tasks
- `refactor:` - Code refactoring

**Signing:**
- Always sign commits with `-s` flag

---

*Convention analysis: 2026-01-26*
