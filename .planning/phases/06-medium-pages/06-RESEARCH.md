# Phase 6: Medium Pages - Research

**Researched:** 2026-01-27
**Domain:** Go page extraction, library integration, sysext management
**Confidence:** HIGH

## Summary

This phase extracts the Maintenance and Extensions pages from the monolithic `userhome.go` into their own packages following the pattern established in Phase 4 (Help/System pages). The key transformation is evolving `internal/updex` from a CLI wrapper to a direct library integration with `github.com/frostyard/updex/updex`.

The codebase has well-established patterns from Phase 4 that directly apply:
- **Logic/UI separation**: Pure Go logic layer (no GTK dependencies) + UI layer
- **Dependency injection**: Pages receive `pages.Deps` containing config and toaster
- **Context-based lifecycle**: Pages use `context.WithCancel` for goroutine management
- **Testability**: Logic layers are tested without GTK; mock interfaces enable test isolation

The updex library (`github.com/frostyard/updex/updex`) is a complete SDK that uses the same `pm/progress` interface as Flatpak and Homebrew in the existing codebase. This makes integration straightforward - we can follow the exact same progress reporting pattern.

**Primary recommendation:** Extract pages following Phase 4's logic/UI separation pattern; replace CLI subprocess calls with direct `updex.Client` method calls using `progress.ProgressReporter` for progress callbacks.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/frostyard/updex/updex` | v1.0.0 | sysext management SDK | First-party library, same pm/progress interface as other PMs |
| `github.com/frostyard/pm/progress` | v0.1.0 | Progress reporting | Already used by pm wrapper for Flatpak/Homebrew |
| `github.com/jwijenbergh/puregotk/v4/adw` | latest | Libadwaita bindings | Project standard for UI widgets |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `context` | stdlib | Goroutine lifecycle | All async operations |
| `os/exec` | stdlib | Script execution | Maintenance page script runner |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| updex SDK | CLI subprocess | CLI adds latency, no structured progress, harder error handling |

**Integration:**
```go
// Add to go.mod
require github.com/frostyard/updex v1.0.0
```

## Architecture Patterns

### Recommended Project Structure
```
internal/pages/
├── maintenance/
│   ├── logic.go       # Pure Go: ActionConfig parsing, executor interface
│   ├── logic_test.go  # Tests for logic layer
│   └── page.go        # GTK UI layer
├── extensions/
│   ├── logic.go       # Pure Go: updex client wrapper, result types
│   ├── logic_test.go  # Tests for logic layer (mock updex.Client)
│   └── page.go        # GTK UI layer
└── page.go            # Shared Deps interface (already exists)
```

### Pattern 1: Logic/UI Layer Separation
**What:** Logic layer contains pure Go with no GTK imports; UI layer handles all widget creation
**When to use:** All page packages
**Example:**
```go
// logic.go - Pure Go, no GTK imports
package maintenance

// Action represents a maintenance action to display/execute
type Action struct {
    Title       string
    Description string
    Script      string
    Sudo        bool
}

// ScriptExecutor abstracts script execution for testability
type ScriptExecutor interface {
    Execute(ctx context.Context, script string, sudo bool) error
}

// ParseActions extracts actions from config
func ParseActions(cfg *config.GroupConfig) []Action {
    if cfg == nil {
        return nil
    }
    var actions []Action
    for _, a := range cfg.Actions {
        actions = append(actions, Action{
            Title:       a.Title,
            Script:      a.Script,
            Sudo:        a.Sudo,
        })
    }
    return actions
}
```

### Pattern 2: updex Library Integration
**What:** Direct updex.Client usage with pm/progress interface
**When to use:** Extensions page for all sysext operations
**Example:**
```go
// logic.go - Extensions page
package extensions

import (
    "context"
    "github.com/frostyard/pm/progress"
    "github.com/frostyard/updex/updex"
)

// Client wraps updex.Client for the extensions page
type Client struct {
    client *updex.Client
}

// NewClient creates a client with optional progress reporting
func NewClient(reporter progress.ProgressReporter) *Client {
    cfg := updex.ClientConfig{
        Progress: reporter,
    }
    return &Client{
        client: updex.NewClient(cfg),
    }
}

// ListInstalled returns installed extensions
func (c *Client) ListInstalled(ctx context.Context) ([]updex.VersionInfo, error) {
    return c.client.List(ctx, updex.ListOptions{})
}

// Discover finds available extensions from a repository
func (c *Client) Discover(ctx context.Context, url string) (*updex.DiscoverResult, error) {
    return c.client.Discover(ctx, url)
}
```

### Pattern 3: Operations Integration with Retry
**What:** Track operations in registry, wire RetryFunc after Start()
**When to use:** All long-running operations
**Example:**
```go
// page.go
func (p *Page) onActionClicked(button *gtk.Button, action *Action) {
    button.SetSensitive(false)
    button.SetLabel("Running...")
    
    op := operations.Start(action.Title, operations.CategoryMaintenance, false)
    
    // Wire retry AFTER Start() - enables retry button in popover
    op.RetryFunc = func() {
        p.onActionClicked(button, action)
    }
    
    go func() {
        err := p.executor.Execute(p.ctx, action.Script, action.Sudo)
        
        async.RunOnMain(func() {
            button.SetSensitive(true)
            button.SetLabel("Run")
            op.Complete(err)
            
            if err != nil {
                p.toaster.ShowErrorToast(async.NewUserError(
                    fmt.Sprintf("Couldn't run %s", action.Title), err).FormatForUser())
            } else {
                p.toaster.ShowToast(fmt.Sprintf("%s completed", action.Title))
            }
        })
    }()
}
```

### Pattern 4: Progress Callback Integration
**What:** Use same ProgressCallback pattern as pm/wrapper.go
**When to use:** Extensions page updex operations
**Example:**
```go
// page.go - progress reporter for updex
type extensionsProgressReporter struct {
    callback ProgressCallback
    mu       sync.Mutex
}

func (pr *extensionsProgressReporter) OnAction(action progress.ProgressAction) {
    pr.mu.Lock()
    defer pr.mu.Unlock()
    if pr.callback != nil {
        actionCopy := action
        async.RunOnMain(func() {
            pr.callback(&actionCopy, nil, nil, nil)
        })
    }
}

// Similar implementations for OnTask, OnStep, OnMessage
```

### Anti-Patterns to Avoid
- **Importing GTK in logic layer:** Makes testing require GTK, breaks CI
- **Subprocess calls when library available:** Adds latency, loses structured data
- **Global updex client:** Use page-scoped client with lifecycle management
- **Ignoring context cancellation:** Always check `p.ctx.Err()` before UI updates

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| sysext management | Custom CLI parsing | `updex.Client` | SDK provides typed results, progress callbacks |
| Progress reporting | Custom callback system | `pm/progress.ProgressReporter` | Already integrated, consistent with Flatpak/Homebrew |
| User errors | Format strings | `async.UserError` | Consistent tone, expandable details |
| Operation tracking | Ad-hoc state | `operations.Registry` | Unified popover display, retry support |
| Script execution | Direct `os/exec` | `ScriptExecutor` interface | Testability via mock injection |

**Key insight:** The codebase already has well-established patterns from Phase 4 and 5. The goal is to apply them consistently, not invent new approaches.

## Common Pitfalls

### Pitfall 1: GTK Imports in Logic Layer
**What goes wrong:** Logic tests fail in headless CI because GTK can't initialize
**Why it happens:** Subtle dependencies like `log.Printf` with GTK types, or importing packages that transitively import GTK
**How to avoid:** 
- Logic files should ONLY import: stdlib, config package, updex library
- Run `go build ./internal/pages/extensions/...` with `-tags test` to verify no GTK deps
**Warning signs:** Import of `github.com/jwijenbergh/puregotk`, `internal/widgets`, or `internal/async` in logic.go

### Pitfall 2: Missing Context Cancellation Check
**What goes wrong:** Goroutine updates destroyed widget, causing panic or memory corruption
**Why it happens:** Async operation completes after page navigated away
**How to avoid:** 
```go
// Before EVERY async.RunOnMain in goroutine:
select {
case <-p.ctx.Done():
    return // Page destroyed
default:
}
```
**Warning signs:** Crashes when rapidly switching pages during operations

### Pitfall 3: CLI Wrapper Left as Fallback
**What goes wrong:** Code paths that still shell out to updex CLI instead of using library
**Why it happens:** Incremental refactoring leaves dead code paths
**How to avoid:** Remove `internal/updex/updex.go` CLI wrapper entirely; compile errors will surface any missed call sites
**Warning signs:** `os/exec` calls to "updex" command in extensions page

### Pitfall 4: Progress Callback Thread Safety
**What goes wrong:** Race conditions when progress callbacks update shared state
**Why it happens:** updex library may call OnAction/OnTask/OnStep from goroutine
**How to avoid:** 
- Always use `async.RunOnMain` for UI updates in callbacks
- Use mutex when updating callback-specific state
**Warning signs:** Sporadic crashes, inconsistent UI state

### Pitfall 5: Script Sudo Without pkexec
**What goes wrong:** Maintenance scripts requiring sudo silently fail
**Why it happens:** Direct sudo call in GUI app has no TTY for password
**How to avoid:** Use `pkexec` for scripts with `Sudo: true` flag (matches existing instex pattern)
**Warning signs:** Scripts requiring elevation fail with permission errors

### Pitfall 6: Closure Variable Capture in Loops
**What goes wrong:** All rows trigger action for last item in list
**Why it happens:** Loop variable captured by reference in closure
**How to avoid:**
```go
for _, action := range actions {
    action := action // Capture in local variable
    clickedCb := func(btn gtk.Button) {
        p.runAction(&action) // Safe - uses local copy
    }
}
```
**Warning signs:** Existing codebase has comments like "// IMPORTANT: Capture link.URL in local variable"

## Code Examples

Verified patterns from the codebase:

### Creating a Page Package (from Phase 4)
```go
// Source: internal/pages/system/page.go
package system

type Page struct {
    toolbarView *adw.ToolbarView
    prefsPage   *adw.PreferencesPage
    
    config    *config.Config
    toaster   pages.Toaster
    
    ctx    context.Context
    cancel context.CancelFunc
}

func New(deps pages.Deps, launchApp, openURL func(string)) *Page {
    ctx, cancel := context.WithCancel(context.Background())
    
    p := &Page{
        config:  deps.Config,
        toaster: deps.Toaster,
        ctx:     ctx,
        cancel:  cancel,
    }
    
    p.buildUI()
    return p
}

func (p *Page) Widget() *adw.ToolbarView {
    return p.toolbarView
}

func (p *Page) Destroy() {
    if p.cancel != nil {
        p.cancel()
    }
}
```

### UserError Pattern (from async package)
```go
// Source: internal/async/errors.go
userErr := async.NewUserErrorWithHint(
    "Couldn't install extension",           // Summary - "Couldn't" not "Failed to"
    "Check the repository URL and try again", // Hint - actionable suggestion
    err,                                     // Technical error for logging
)
p.toaster.ShowErrorToast(userErr.FormatForUser())
log.Printf("Extension install error details: %v", err)
```

### updex SDK Client Configuration
```go
// Source: frostyard/updex/updex/updex.go
client := updex.NewClient(updex.ClientConfig{
    Progress: reporter,  // pm/progress.ProgressReporter
})

results, err := client.List(ctx, updex.ListOptions{})
// results is []updex.VersionInfo

discovered, err := client.Discover(ctx, repoURL)
// discovered is *updex.DiscoverResult
```

### Progress Reporter Implementation Pattern
```go
// Source: internal/pm/wrapper.go (adapted for updex)
type progressReporter struct {
    callback ProgressCallback
    mu       sync.Mutex
}

func (pr *progressReporter) OnAction(action progress.ProgressAction) {
    pr.mu.Lock()
    defer pr.mu.Unlock()
    if pr.callback != nil {
        actionCopy := action
        async.RunOnMain(func() {
            pr.callback(&actionCopy, nil, nil, nil)
        })
    }
}

func (pr *progressReporter) OnTask(task progress.ProgressTask) {
    // Similar pattern...
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| CLI wrapper (`exec.Command("updex"...)`) | Direct library import (`updex.NewClient`) | This phase | Structured results, progress callbacks, no subprocess overhead |
| Monolithic userhome.go | Page packages with logic/UI split | Phase 4 | Testability, maintainability |

**Deprecated/outdated:**
- `internal/updex/updex.go`: CLI wrapper - remove after migration
- Direct page builds in `userhome.go`: Move to page packages

## Open Questions

Things that couldn't be fully resolved:

1. **sysext availability detection approach**
   - What we know: `updex.IsInstalled()` currently checks CLI, need library equivalent
   - What's unclear: Whether to check systemd-sysext availability or just library
   - Recommendation: Check for `/var/lib/extensions` directory existence or wrap library availability check

2. **instex library integration**
   - What we know: `internal/instex` is CLI wrapper for extension discovery/install from remote repos
   - What's unclear: Whether instex has/will have a library equivalent like updex
   - Recommendation: Keep instex as CLI wrapper for now; document for future library migration

3. **Card grid layout for discovered extensions**
   - What we know: Context.md mentions "Card grid display"
   - What's unclear: libadwaita card grid widget availability
   - Recommendation: Use `adw.PreferencesGroup` with `adw.ActionRow` (current pattern); card grid can be future enhancement

## Sources

### Primary (HIGH confidence)
- `internal/pages/help/` and `internal/pages/system/` - Phase 4 established patterns
- `internal/pm/wrapper.go` - Progress reporter integration pattern
- `internal/operations/` - Operations registry and popover
- `github.com/frostyard/updex/updex/` - SDK API and types
- `github.com/frostyard/pm/progress/` - ProgressReporter interface

### Secondary (MEDIUM confidence)
- `internal/views/userhome.go` lines 628-1020 - Current Maintenance/Extensions implementation
- `internal/updex/updex.go` - CLI wrapper to be replaced
- `internal/instex/instex.go` - CLI wrapper to remain (no library available)

### Tertiary (LOW confidence)
- Card grid alternatives - needs UI exploration

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All libraries are first-party or already in use
- Architecture: HIGH - Following established Phase 4 patterns exactly
- Pitfalls: HIGH - Derived from existing codebase patterns and prior decisions
- updex SDK integration: HIGH - Examined actual library source code

**Research date:** 2026-01-27
**Valid until:** 2026-02-27 (stable libraries, established patterns)
