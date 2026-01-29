# Architecture Patterns: GTK4/Go Application Refactoring

**Domain:** GTK4/Go desktop application refactoring
**Researched:** 2026-01-26
**Confidence:** MEDIUM — Based on codebase analysis and GTK4/Go patterns; limited public GTK4+Go examples exist

## Executive Summary

Chairlift's 2500-line `userhome.go` monolith needs decomposition into well-bounded components. This document provides a concrete architecture based on analysis of the existing codebase, GTK4/Libadwaita patterns, and Go package organization best practices.

The key insight: GTK4/Libadwaita already provides the component model (widgets, composite templates). The refactoring challenge is **organizing Go code to match GTK's natural boundaries** while **extracting reusable patterns into a separate library**.

## Current Architecture Analysis

### Component Map (As-Is)

```
cmd/chairlift/main.go (37 lines)
    └── internal/app/app.go (129 lines)
            └── internal/window/window.go (444 lines)
                    └── internal/views/userhome.go (2499 lines) ← MONOLITH
                            ├── internal/config/config.go (207 lines)
                            ├── internal/pm/wrapper.go (1048 lines) ← Also large
                            ├── internal/nbc/nbc.go (515 lines)
                            ├── internal/updex/updex.go (139 lines)
                            └── internal/instex/instex.go (169 lines)
```

### Problems Identified

| Problem | Location | Impact |
|---------|----------|--------|
| God object | `userhome.go` UserHome struct | All 6 pages, all state, all handlers in one struct |
| Mixed concerns | `userhome.go` | Page building, async loading, event handling, progress UI all interleaved |
| Widget reference sprawl | `userhome.go` lines 66-131 | 40+ widget references on single struct |
| Duplicate patterns | Throughout `userhome.go` | Same loading/error/button patterns repeated 20+ times |
| Callback spaghetti | Event handlers | Closure captures with complex mutex coordination |
| No component reuse | N/A | Every ActionRow, ExpanderRow, progress UI built from scratch |
| runOnMainThread duplication | `userhome.go`, `pm/wrapper.go` | Same idle callback pattern implemented twice |

### Current Data Flow

```
User Action → GTK Signal → Callback Closure → Go Goroutine → Backend Call
                                                   ↓
                                           runOnMainThread()
                                                   ↓
                                            Update Widgets
```

This pattern is **correct** but **ad-hoc**. Each handler reimplements it.

## Recommended Architecture

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| `internal/app` | Application lifecycle, keyboard shortcuts | window |
| `internal/window` | Window shell, navigation, toast overlay | app, pages |
| `internal/pages/*` | Page-specific UI and logic | window (via interface), widgets, backends |
| `internal/widgets/` | Reusable composite widgets | glib/gtk only |
| `internal/async/` | Async operation framework | glib/gtk, backends |
| `internal/backends/` | Wrappers for pm, nbc, updex, instex | External libraries |
| `internal/config` | Configuration loading | Filesystem |

### Package Organization (Target)

```
internal/
├── app/
│   └── app.go                    # Application lifecycle (existing, keep)
│
├── window/
│   └── window.go                 # Main window, navigation (existing, refactor)
│
├── pages/                        # NEW: One package per page
│   ├── system/
│   │   └── page.go               # System page
│   ├── updates/
│   │   └── page.go               # Updates page
│   ├── applications/
│   │   └── page.go               # Applications page
│   ├── maintenance/
│   │   └── page.go               # Maintenance page
│   ├── extensions/
│   │   └── page.go               # Extensions page
│   └── help/
│       └── page.go               # Help page
│
├── widgets/                      # NEW: Reusable components
│   ├── actionrow.go              # Enhanced action rows (with buttons, badges)
│   ├── expanderlist.go           # Expander with async loading pattern
│   ├── progresssheet.go          # Bottom sheet progress UI
│   └── loader.go                 # Async loading wrapper widget
│
├── async/                        # NEW: Async operation framework
│   ├── scheduler.go              # runOnMainThread, idle callbacks
│   ├── operation.go              # Cancellable async operation wrapper
│   └── progress.go               # Progress reporting infrastructure
│
├── backends/                     # Renamed from individual wrappers
│   ├── pm.go                     # Package manager (existing pm/wrapper.go, refactored)
│   ├── nbc.go                    # NBC (existing, move)
│   ├── updex.go                  # Updex (existing, move)
│   └── instex.go                 # Instex (existing, move)
│
└── config/
    └── config.go                 # Configuration (existing, keep)
```

### Why This Structure

1. **Pages are natural boundaries.** Each page has distinct:
   - State (widget references, loaded data)
   - Backends (updates page uses nbc+pm, extensions page uses updex+instex)
   - Configuration groups

2. **Widgets package enables reuse.** Patterns that repeat in current code:
   - "ActionRow with async-load subtitle and button" (~15 instances)
   - "ExpanderRow with loading spinner → list of items" (~10 instances)
   - "Button that disables during operation, shows progress" (~20 instances)

3. **Async package centralizes goroutine-GTK coordination.** Current code has:
   - `runOnMainThread` duplicated in `userhome.go` and `pm/wrapper.go`
   - Inconsistent error handling in goroutines
   - No cancellation support

4. **Backends package consolidates external tool wrappers.** Already well-structured, just reorganize.

## Page Interface Contract

Each page implements this interface for the window to manage:

```go
package pages

import "github.com/jwijenbergh/puregotk/v4/adw"

// Page defines the contract between window and page implementations
type Page interface {
    // Widget returns the page's top-level widget (ToolbarView)
    Widget() *adw.ToolbarView
    
    // Name returns the page identifier (e.g., "updates", "system")
    Name() string
    
    // Title returns the human-readable page title
    Title() string
    
    // Icon returns the icon name for sidebar navigation
    Icon() string
    
    // OnActivate is called when the page becomes visible
    // Use for lazy loading, refresh, etc.
    OnActivate()
    
    // OnDeactivate is called when navigating away from the page
    // Use for cleanup, pause ongoing operations
    OnDeactivate()
}

// ToastAdder is the interface pages use to show notifications
type ToastAdder interface {
    ShowToast(message string)
    ShowErrorToast(message string)
    SetUpdateBadge(count int)
}
```

This interface is **already implicit** in the current code. Formalizing it enables:
- Lazy page initialization
- Page-level lifecycle hooks
- Clean separation between window and page concerns

## Widget Extraction Candidates

Based on pattern analysis in `userhome.go`, these widgets should be extracted:

### 1. AsyncExpanderRow

Current pattern (repeated ~10 times):

```go
expander := adw.NewExpanderRow()
expander.SetTitle("...")
expander.SetSubtitle("Loading...")

go func() {
    data, err := loadSomething()
    runOnMainThread(func() {
        if err != nil {
            expander.SetSubtitle(fmt.Sprintf("Error: %v", err))
            return
        }
        expander.SetSubtitle(fmt.Sprintf("%d items", len(data)))
        for _, item := range data {
            row := adw.NewActionRow()
            row.SetTitle(item.Name)
            expander.AddRow(&row.Widget)
        }
    })
}()
```

**Extracted widget:**

```go
package widgets

type AsyncExpanderRow struct {
    *adw.ExpanderRow
}

func NewAsyncExpanderRow(title string) *AsyncExpanderRow { ... }

// Load starts async loading with automatic UI updates
func (r *AsyncExpanderRow) Load(
    loader func() ([]RowData, error),
    rowBuilder func(RowData) *adw.ActionRow,
)
```

### 2. ActionButton

Current pattern (repeated ~20 times):

```go
btn := gtk.NewButtonWithLabel("Install")
btn.SetValign(gtk.AlignCenterValue)
btn.AddCssClass("suggested-action")

clickedCb := func(_ gtk.Button) {
    btn.SetSensitive(false)
    btn.SetLabel("Installing...")
    go func() {
        err := doSomething()
        runOnMainThread(func() {
            btn.SetSensitive(true)
            btn.SetLabel("Install")
            if err != nil {
                toastAdder.ShowErrorToast(...)
            } else {
                toastAdder.ShowToast(...)
            }
        })
    }()
}
btn.ConnectClicked(&clickedCb)
```

**Extracted widget:**

```go
package widgets

type ActionButton struct {
    *gtk.Button
    defaultLabel string
    busyLabel    string
    toastAdder   ToastAdder
}

func NewActionButton(
    label, busyLabel string,
    style string, // "suggested", "destructive", ""
    action func() error,
    toastAdder ToastAdder,
) *ActionButton
```

### 3. LoadingRow

Current pattern (used during loading):

```go
loadingRow := adw.NewActionRow()
loadingRow.SetTitle("Loading...")
spinner := gtk.NewSpinner()
spinner.Start()
loadingRow.AddPrefix(&spinner.Widget)
expander.AddRow(&loadingRow.Widget)

// Later:
expander.Remove(&loadingRow.Widget)
```

**Extracted widget:**

```go
package widgets

type LoadingRow struct {
    *adw.ActionRow
    spinner *gtk.Spinner
}

func NewLoadingRow(title, subtitle string) *LoadingRow
func (r *LoadingRow) Stop()
```

## Async Framework Design

### Problem

Current code has scattered goroutine management:

```go
// Pattern 1: Direct goroutine
go func() {
    result, err := backend.DoThing()
    runOnMainThread(func() { /* update UI */ })
}()

// Pattern 2: Channel-based (NBC progress)
progressCh := make(chan nbc.ProgressEvent)
go func() {
    err := nbc.Update(ctx, opts, progressCh)
}()
for event := range progressCh {
    runOnMainThread(func() { /* update progress */ })
}

// Pattern 3: Callback-based (pm library)
pm.InitializeFlatpak(func(action, task, step, msg) {
    runOnMainThread(func() { /* update progress */ })
})
```

### Solution: Unified Operation Type

```go
package async

// Operation represents a cancellable async operation with progress
type Operation struct {
    ctx    context.Context
    cancel context.CancelFunc
    done   chan struct{}
}

// Run starts an async operation with automatic main-thread marshaling
func Run(fn func(ctx context.Context) error) *Operation

// RunWithProgress starts an operation that reports progress
func RunWithProgress(
    fn func(ctx context.Context, progress chan<- Progress) error,
    onProgress func(Progress),
) *Operation

// Cancel cancels the operation
func (op *Operation) Cancel()

// Wait blocks until operation completes
func (op *Operation) Wait() error
```

### runOnMainThread Consolidation

Current state: Duplicated in `userhome.go` and `pm/wrapper.go`

**Consolidate to:**

```go
package async

var (
    callbackMu sync.Mutex
    callbacks  = make(map[uintptr]func())
    callbackID uintptr
)

// RunOnMain schedules a function to run on the GTK main thread
func RunOnMain(fn func()) {
    callbackMu.Lock()
    callbackID++
    id := callbackID
    callbacks[id] = fn
    callbackMu.Unlock()

    cb := glib.SourceFunc(func(data uintptr) bool {
        callbackMu.Lock()
        callback, ok := callbacks[data]
        delete(callbacks, data)
        callbackMu.Unlock()
        if ok {
            callback()
        }
        return false
    })
    glib.IdleAdd(&cb, id)
}
```

## Refactoring Order (Dependencies)

Refactoring must respect dependencies. Wrong order = painful conflicts.

```
Phase Order (dependency-driven):

1. async/scheduler.go        ← No dependencies, enables everything else
   - Extract runOnMainThread from userhome.go
   - Update pm/wrapper.go to use it
   
2. widgets/                   ← Depends on async
   - Extract AsyncExpanderRow, ActionButton, LoadingRow
   - These are pure UI components
   
3. pages/system/              ← Smallest page, proves the pattern
   - ~100 lines of userhome.go
   - Depends on: async, widgets, backends/nbc, config
   
4. pages/help/                ← Even simpler, no async loading
   - ~70 lines of userhome.go
   - Depends on: config only
   
5. pages/extensions/          ← Medium complexity
   - ~200 lines of userhome.go
   - Depends on: async, widgets, backends/updex, backends/instex
   
6. pages/maintenance/         ← Medium complexity
   - ~150 lines of userhome.go
   - Depends on: async, widgets, backends/pm
   
7. pages/applications/        ← High complexity
   - ~600 lines of userhome.go
   - Depends on: async, widgets, backends/pm (flatpak, snap, brew)
   
8. pages/updates/             ← Highest complexity
   - ~700 lines of userhome.go
   - Depends on: async, widgets, backends/nbc, backends/pm
   - Has complex progress tracking, badge updates
   
9. async/operation.go         ← After pages stabilize
   - Unified operation framework
   - Refactor pages to use it
   
10. Library extraction        ← After patterns proven
    - async/, widgets/ become candidate library code
```

### Why This Order

1. **Infrastructure first** (`async/scheduler.go`) — Everything needs main-thread scheduling
2. **Building blocks second** (`widgets/`) — Pages are built from widgets
3. **Simplest pages first** (`system`, `help`) — Validate the pattern with low risk
4. **Complexity gradient** — Build confidence before tackling applications/updates
5. **Consolidation last** — See real patterns before abstracting `async/operation.go`
6. **Library extraction last** — Know what's reusable after seeing it used

## Anti-Patterns to Avoid

### Anti-Pattern 1: Premature Abstraction

**What:** Creating complex generic systems before understanding use cases
**Why bad:** Ends up not fitting actual needs, requires rework
**Instead:** Extract patterns **after** you have 3+ concrete examples

The current `onPMProgressUpdate` callback (~200 lines) is already over-abstracted. It handles action/task/step/message with complex ID mapping that's hard to understand.

**Better approach:** Each page handles its own progress simply, extract common patterns later.

### Anti-Pattern 2: Widget Reference Accumulation

**What:** Storing every widget reference on the page struct
**Why bad:** UserHome currently has 40+ widget fields, hard to track what's active
**Instead:** 
- Pass widgets to builders, don't store unless needed for updates
- Use `SetName()` + lookup instead of storing references
- Consider builder patterns that return closures for updates

### Anti-Pattern 3: Goroutine Leaks

**What:** Starting goroutines without cancellation or lifecycle management
**Why bad:** Goroutines continue after page deactivation, race conditions
**Instead:**
- Every async operation gets a context
- Pages cancel operations in `OnDeactivate()`
- Use the `async.Operation` type

### Anti-Pattern 4: Callback Capture Bugs

**What:** Loop variable capture in closures (common Go mistake)
**Example from userhome.go (correct, but fragile):**
```go
for _, item := range items {
    name := item.Name // Capture for closure
    clickedCb := func(_ gtk.Button) {
        doSomething(name) // Uses captured name
    }
}
```
**Instead:** The explicit capture is correct but error-prone. Consider helper functions that accept the value:
```go
func makeClickHandler(name string) func(gtk.Button) {
    return func(_ gtk.Button) {
        doSomething(name)
    }
}
```

## Library Extraction Strategy

The goal is extracting reusable GTK4/Go patterns into a separate library. Strategy:

### Phase 1: Identify During Refactoring

As each page is extracted, mark patterns that are:
- Generic (not Chairlift-specific)
- Used in 3+ places
- Stable (not changing frequently)

Candidates from current analysis:
- `async.RunOnMain()` — Universal
- `widgets.AsyncExpanderRow` — Generic loading pattern
- `widgets.ActionButton` — Generic action button pattern
- `async.Operation` — Generic async wrapper

### Phase 2: Separate Within Same Repo

Create `pkg/` directory for library-candidate code:

```
pkg/
├── gtkutil/                  # GTK utilities
│   └── scheduler.go          # RunOnMain, idle callbacks
├── adwutil/                  # Libadwaita utilities  
│   ├── asyncexpander.go
│   └── actionbutton.go
└── async/                    # Async patterns
    └── operation.go
```

Import as `github.com/frostyard/chairlift/pkg/gtkutil` initially.

### Phase 3: Evaluate Extraction

After patterns stabilize (2-3 milestones), decide:
- **Separate repo** if other projects will use it
- **Keep in chairlift** if only this project uses it

**Decision criteria:**
- Are we building other GTK4/Go apps?
- Is the API stable enough to version separately?
- Is the maintenance burden of a separate repo worth it?

## Testing Architecture

### Problem

Current code has zero tests. Refactoring is high-risk.

### Testing Strategy for GTK4/Go

GTK4 widget testing is hard because:
- Widgets require GTK initialization
- GTK is not thread-safe
- Visual testing is limited

**Recommended approach:**

1. **Logic tests (easy, do first)**
   - Test backend wrappers (pm, nbc, updex, instex)
   - Test config loading
   - Test data transformation functions
   - Use mocks/fakes for external tools

2. **Integration tests (medium)**
   - Test page building with GTK initialized
   - Verify widget creation doesn't crash
   - Don't test visual appearance

3. **Manual testing protocol (required)**
   - Create test scenarios document
   - Run through scenarios before each release
   - Screenshot comparison for visual regression

### Example Test Structure

```go
// internal/backends/pm_test.go
func TestListHomebrewFormulae(t *testing.T) {
    // Mock the brew command output
    // Verify parsing logic
}

// internal/pages/system/page_test.go
func TestSystemPageBuilds(t *testing.T) {
    gtk.Init()
    config := config.DefaultConfig()
    page := system.New(config, &mockToastAdder{})
    
    // Verify page.Widget() returns non-nil
    // Verify page.Name() == "system"
}
```

## Sources and Confidence

| Topic | Source | Confidence |
|-------|--------|------------|
| Current architecture | Direct codebase analysis | HIGH |
| GTK4/Libadwaita patterns | Training data + puregotk source | MEDIUM |
| Go package organization | Go standard practices | HIGH |
| Refactoring order | Dependency analysis of current code | HIGH |
| Widget extraction patterns | Pattern matching in userhome.go | HIGH |
| Library extraction strategy | Best practices, not validated | MEDIUM |
| Testing strategy | GTK testing is inherently limited | MEDIUM |

**Note:** There are limited public examples of large GTK4+Go applications. This architecture is synthesized from GTK4 patterns (any language), Go idioms, and the specific needs of this codebase. Phase-specific validation is recommended.
