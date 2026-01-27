# Technology Stack: GTK4/Go Refactoring Patterns

**Project:** Chairlift Refactoring Milestone
**Researched:** 2026-01-26
**Focus:** Patterns for structuring, testing, and refactoring GTK4/Go applications with puregotk

---

## Executive Summary

This research identifies patterns for refactoring a 2500-line monolithic GTK4/Go UI into a maintainable, testable architecture. The key insight: **puregotk enables standard Go patterns** because there's no CGO boundary complicating testing or structure. Use Go's native strengths (interfaces, composition, channels) rather than fighting GTK's C-oriented patterns.

**Core recommendations:**
1. Feature-based package organization with interface boundaries
2. Callback registry pattern already in use—extend it systematically
3. Unified async/progress framework using typed channels
4. Extract pure Go logic from UI code for testability
5. Test UI logic indirectly through interfaces; accept GTK code is hard to unit test

---

## Pattern 1: Feature-Based Package Structure

**Confidence:** HIGH (standard Go practice, verified applicable to this codebase)

### Current Problem

`userhome.go` (2498 lines) contains all view logic:
- System page builder
- Updates page builder (NBC, Flatpak, Homebrew, Snap)
- Applications page builder
- Extensions page builder
- All async operations mixed with UI code
- All progress handling mixed with business logic

### Recommended Structure

```
internal/
├── app/                    # Application lifecycle (exists)
├── window/                 # Main window, navigation (exists)
├── views/                  # REFACTOR: Split by feature
│   ├── system/             # System info page
│   │   ├── page.go         # Page builder
│   │   └── types.go        # Types for this page
│   ├── updates/            # Updates page (NBC, packages)
│   │   ├── page.go         # Page builder  
│   │   ├── nbc.go          # NBC-specific update UI
│   │   └── packages.go     # Package manager update UI
│   ├── applications/       # Applications page
│   │   ├── page.go
│   │   ├── flatpak.go
│   │   ├── snap.go
│   │   └── homebrew.go
│   ├── extensions/         # Extensions page
│   │   └── page.go
│   └── shared/             # EXTRACT: Reusable components
│       ├── progress.go     # Progress bar component
│       ├── actionrow.go    # Action row builders
│       ├── expander.go     # Expander row builders
│       └── async.go        # runOnMainThread, async helpers
├── ui/                     # NEW: UI component library (extractable later)
│   ├── progress/           # Progress display components
│   ├── list/               # List components
│   └── dialog/             # Dialog patterns
├── config/                 # Configuration (exists)
├── nbc/                    # NBC wrapper (exists)
├── pm/                     # Package manager wrapper (exists)
├── updex/                  # Updex wrapper (exists)
└── instex/                 # Instex wrapper (exists)
```

### Why This Structure

1. **Feature cohesion:** All code for a feature lives together
2. **Clear dependencies:** Each view package imports only what it needs
3. **Testable boundaries:** Interfaces between packages enable testing
4. **Parallel development:** Different features can be refactored independently
5. **Extractable patterns:** `shared/` and `ui/` become the reusable library

### Implementation Pattern

```go
// views/updates/page.go
package updates

import (
    "github.com/frostyard/chairlift/internal/views/shared"
    "github.com/frostyard/chairlift/internal/nbc"
    "github.com/jwijenbergh/puregotk/v4/adw"
)

// PageController manages the updates page
type PageController struct {
    config     Config           // Configuration for what to show
    toaster    shared.Toaster   // Interface for toast notifications
    progress   shared.Progress  // Progress tracking interface
    
    // Widget references
    page       *adw.ToolbarView
    nbcSection *NBCSection
    // ...
}

// NewPage creates the updates page
func NewPage(cfg Config, toaster shared.Toaster, progress shared.Progress) *PageController {
    pc := &PageController{
        config:   cfg,
        toaster:  toaster,
        progress: progress,
    }
    pc.build()
    return pc
}

// Widget returns the page widget for embedding
func (pc *PageController) Widget() *adw.ToolbarView {
    return pc.page
}
```

---

## Pattern 2: Unified Async/Progress Framework

**Confidence:** HIGH (based on frostyard/pm progress pattern, already partially implemented)

### Current Problem

Progress handling is inconsistent across the codebase:
- NBC uses channel-based progress events
- Package managers use callback-based progress
- UI update patterns duplicated everywhere
- `runOnMainThread` scattered throughout

### Recommended Pattern

Create a unified async operation framework that handles:
1. GTK main thread marshaling
2. Progress reporting
3. Cancellation
4. Error handling

```go
// views/shared/async.go
package shared

import (
    "context"
    "github.com/jwijenbergh/puregotk/v4/glib"
)

// Operation represents an async operation with progress
type Operation struct {
    Name       string
    Progress   chan ProgressEvent
    Done       chan error
    Cancel     context.CancelFunc
}

// ProgressEvent is a unified progress event type
type ProgressEvent struct {
    Type     ProgressEventType
    Message  string
    Percent  int
    Step     int
    Total    int
    StepName string
    Error    error
}

type ProgressEventType int

const (
    EventTypeProgress ProgressEventType = iota
    EventTypeStep
    EventTypeMessage
    EventTypeWarning
    EventTypeError
    EventTypeComplete
)

// RunAsync executes a function on a goroutine with proper cleanup
func RunAsync(ctx context.Context, fn func(ctx context.Context) error, onProgress func(ProgressEvent), onComplete func(error)) {
    go func() {
        err := fn(ctx)
        RunOnMainThread(func() {
            onComplete(err)
        })
    }()
}

// RunOnMainThread schedules work on GTK main thread
// Uses callback registry pattern to prevent GC collection
func RunOnMainThread(fn func()) {
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

var (
    callbackMu sync.Mutex
    callbacks  = make(map[uintptr]func())
    callbackID uintptr
)
```

### Progress UI Component

```go
// views/shared/progress.go
package shared

import (
    "github.com/jwijenbergh/puregotk/v4/adw"
    "github.com/jwijenbergh/puregotk/v4/gtk"
)

// ProgressDisplay provides a reusable progress UI component
type ProgressDisplay struct {
    expander    *adw.ExpanderRow
    progressBar *gtk.ProgressBar
    progressRow *adw.ActionRow
    logExpander *adw.ExpanderRow
}

// NewProgressDisplay creates a progress UI in the given expander
func NewProgressDisplay(parent *adw.ExpanderRow) *ProgressDisplay {
    pd := &ProgressDisplay{expander: parent}
    pd.build()
    return pd
}

// HandleEvent processes a progress event and updates UI
func (pd *ProgressDisplay) HandleEvent(event ProgressEvent) {
    RunOnMainThread(func() {
        switch event.Type {
        case EventTypeProgress:
            pd.progressBar.SetFraction(float64(event.Percent) / 100.0)
            pd.progressRow.SetSubtitle(event.Message)
        case EventTypeStep:
            progress := float64(event.Step) / float64(event.Total)
            pd.progressBar.SetFraction(progress)
            pd.expander.SetSubtitle(fmt.Sprintf("[%d/%d] %s", event.Step, event.Total, event.StepName))
        case EventTypeError:
            pd.addLogRow(event.Message, "Error", "dialog-error-symbolic")
            pd.logExpander.SetExpanded(true)
        case EventTypeComplete:
            pd.progressBar.SetFraction(1.0)
            pd.addLogRow(event.Message, "Complete", "object-select-symbolic")
        }
    })
}
```

### Why This Pattern

1. **Consistency:** All async operations look the same
2. **Testability:** Progress events are data, testable without GTK
3. **Reusability:** Same component works for NBC, Flatpak, Homebrew
4. **Memory safety:** Callback registry prevents GC issues (already proven pattern)

---

## Pattern 3: Interface Boundaries for Testability

**Confidence:** HIGH (standard Go practice)

### Current Problem

Views directly depend on:
- GTK widgets (can't test without GTK runtime)
- Global package manager singletons
- Window/toast notification system

### Recommended Pattern

Define interfaces at package boundaries:

```go
// views/shared/interfaces.go
package shared

// Toaster provides toast notification capability
type Toaster interface {
    ShowToast(message string)
    ShowErrorToast(message string)
}

// UpdateBadger provides update badge capability
type UpdateBadger interface {
    SetUpdateBadge(count int)
}

// WindowServices combines window-level services
type WindowServices interface {
    Toaster
    UpdateBadger
}

// PackageManager abstracts package manager operations for UI
type PackageManager interface {
    IsAvailable() bool
    ListInstalled(ctx context.Context) ([]Package, error)
    Install(ctx context.Context, pkg Package, progress func(ProgressEvent)) error
    Uninstall(ctx context.Context, pkg Package, progress func(ProgressEvent)) error
}

// Package represents a package for UI display
type Package struct {
    ID          string
    Name        string
    Version     string
    Description string
    Installed   bool
    UpdateAvail bool
}
```

### Mock for Testing

```go
// views/updates/nbc_test.go
package updates_test

type mockToaster struct {
    toasts      []string
    errorToasts []string
}

func (m *mockToaster) ShowToast(msg string) {
    m.toasts = append(m.toasts, msg)
}

func (m *mockToaster) ShowErrorToast(msg string) {
    m.errorToasts = append(m.errorToasts, msg)
}

func TestNBCUpdateHandler(t *testing.T) {
    toaster := &mockToaster{}
    handler := updates.NewNBCHandler(toaster)
    
    // Simulate progress events
    handler.HandleProgress(shared.ProgressEvent{
        Type: shared.EventTypeComplete,
        Message: "Update complete",
    })
    
    // Verify toast was shown
    if len(toaster.toasts) != 1 {
        t.Errorf("expected 1 toast, got %d", len(toaster.toasts))
    }
}
```

---

## Pattern 4: GTK Widget Builders

**Confidence:** MEDIUM (derived from puregotk examples and current codebase patterns)

### Current Problem

Widget creation code is verbose and repetitive:
```go
row := adw.NewActionRow()
row.SetTitle("Something")
row.SetSubtitle("Details")
icon := gtk.NewImageFromIconName("dialog-warning-symbolic")
row.AddPrefix(&icon.Widget)
```

### Recommended Pattern: Builder Functions

```go
// views/shared/widgets.go
package shared

import (
    "github.com/jwijenbergh/puregotk/v4/adw"
    "github.com/jwijenbergh/puregotk/v4/gtk"
)

// ActionRowConfig configures an action row
type ActionRowConfig struct {
    Title      string
    Subtitle   string
    Icon       string   // Prefix icon name
    IconSuffix string   // Suffix icon name
    Activatable bool
    OnActivate func()
}

// NewActionRow creates an action row with common configuration
func NewActionRow(cfg ActionRowConfig) *adw.ActionRow {
    row := adw.NewActionRow()
    row.SetTitle(cfg.Title)
    if cfg.Subtitle != "" {
        row.SetSubtitle(cfg.Subtitle)
    }
    if cfg.Icon != "" {
        icon := gtk.NewImageFromIconName(cfg.Icon)
        row.AddPrefix(&icon.Widget)
    }
    if cfg.IconSuffix != "" {
        icon := gtk.NewImageFromIconName(cfg.IconSuffix)
        row.AddSuffix(&icon.Widget)
    }
    if cfg.OnActivate != nil {
        row.SetActivatable(true)
        cb := func() {
            cfg.OnActivate()
        }
        row.ConnectActivated(&cb)
    }
    return row
}

// ExpanderRowConfig configures an expander row
type ExpanderRowConfig struct {
    Title    string
    Subtitle string
    Icon     string
    Expanded bool
}

// NewExpanderRow creates an expander row with common configuration  
func NewExpanderRow(cfg ExpanderRowConfig) *adw.ExpanderRow {
    exp := adw.NewExpanderRow()
    exp.SetTitle(cfg.Title)
    if cfg.Subtitle != "" {
        exp.SetSubtitle(cfg.Subtitle)
    }
    if cfg.Icon != "" {
        icon := gtk.NewImageFromIconName(cfg.Icon)
        exp.AddPrefix(&icon.Widget)
    }
    exp.SetExpanded(cfg.Expanded)
    return exp
}

// ButtonConfig configures a button
type ButtonConfig struct {
    Label     string
    Icon      string
    CSSClass  string
    Sensitive bool
    OnClick   func()
}

// NewButton creates a button with common configuration
func NewButton(cfg ButtonConfig) *gtk.Button {
    var btn *gtk.Button
    if cfg.Icon != "" {
        btn = gtk.NewButtonFromIconName(cfg.Icon)
    } else {
        btn = gtk.NewButtonWithLabel(cfg.Label)
    }
    if cfg.CSSClass != "" {
        btn.AddCssClass(cfg.CSSClass)
    }
    btn.SetSensitive(cfg.Sensitive)
    if cfg.OnClick != nil {
        cb := func() {
            cfg.OnClick()
        }
        btn.ConnectClicked(&cb)
    }
    return btn
}
```

### Why This Pattern

1. **Reduces boilerplate:** Common patterns become one-liners
2. **Consistency:** All buttons/rows configured the same way
3. **Extractable:** These become the core of a reusable library
4. **Type safety:** Config structs provide documentation

---

## Pattern 5: Testing Strategy for GTK4/Go

**Confidence:** MEDIUM (based on Go testing best practices; GTK-specific testing is inherently limited)

### Reality Check

**What's testable:**
- Pure business logic
- Data transformations
- Progress event handling
- Interface interactions (via mocks)
- State management

**What's NOT easily testable:**
- GTK widget creation and layout
- Visual appearance
- User interactions
- GTK signal handling

### Testing Strategy

#### Layer 1: Pure Logic Tests

```go
// nbc/status_test.go
package nbc_test

func TestParseNBCStatus(t *testing.T) {
    jsonData := `{"staged": true, "version": "1.2.3"}`
    
    status, err := nbc.ParseStatus([]byte(jsonData))
    
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !status.Staged {
        t.Error("expected staged to be true")
    }
    if status.Version != "1.2.3" {
        t.Errorf("expected version 1.2.3, got %s", status.Version)
    }
}
```

#### Layer 2: Interface Mock Tests

```go
// views/updates/handler_test.go
package updates_test

type mockNBC struct {
    status   *nbc.Status
    statusErr error
    updateCalled bool
}

func (m *mockNBC) GetStatus(ctx context.Context) (*nbc.Status, error) {
    return m.status, m.statusErr
}

func (m *mockNBC) Update(ctx context.Context, opts nbc.UpdateOptions) error {
    m.updateCalled = true
    return nil
}

func TestUpdateHandler_ShowsErrorOnStatusFail(t *testing.T) {
    toaster := &mockToaster{}
    nbcClient := &mockNBC{
        statusErr: errors.New("nbc not found"),
    }
    
    handler := updates.NewHandler(toaster, nbcClient)
    handler.CheckForUpdates(context.Background())
    
    if len(toaster.errorToasts) == 0 {
        t.Error("expected error toast on status failure")
    }
}
```

#### Layer 3: Integration Tests (require GTK)

```go
// +build integration

// views/integration_test.go
package views_test

func TestPageBuildsWithoutPanic(t *testing.T) {
    // Requires GTK to be available
    // Tests that page construction doesn't crash
    // NOT testing visual correctness
    
    cfg := config.Default()
    toaster := &noopToaster{}
    
    // This will panic if there's a nil pointer, bad widget tree, etc.
    page := updates.NewPage(cfg, toaster)
    
    if page.Widget() == nil {
        t.Error("expected widget to be non-nil")
    }
}
```

### Test Infrastructure

```go
// testutil/mocks.go
package testutil

// NoopToaster is a test double for Toaster
type NoopToaster struct{}

func (t *NoopToaster) ShowToast(string)      {}
func (t *NoopToaster) ShowErrorToast(string) {}
func (t *NoopToaster) SetUpdateBadge(int)    {}

// RecordingToaster records all toasts for assertions
type RecordingToaster struct {
    Toasts      []string
    ErrorToasts []string
    BadgeValue  int
}

func (t *RecordingToaster) ShowToast(msg string) {
    t.Toasts = append(t.Toasts, msg)
}

func (t *RecordingToaster) ShowErrorToast(msg string) {
    t.ErrorToasts = append(t.ErrorToasts, msg)
}

func (t *RecordingToaster) SetUpdateBadge(count int) {
    t.BadgeValue = count
}
```

---

## Pattern 6: puregotk-Specific Patterns

**Confidence:** HIGH (verified from puregotk examples and current codebase)

### Memory Management

puregotk does NOT use finalizers. You must:

```go
// DO: Unref when done
widget := gtk.NewLabel("Hello")
defer widget.Unref()

// DON'T: Assume cleanup happens automatically
widget := gtk.NewLabel("Hello")
// Leaks if not unrefed!

// EXCEPTION: Widgets added to containers are managed by container
box.Append(&label.Widget)  // Container now owns it, don't unref
```

### Signal Connections

```go
// DO: Use pointer to function for signal handlers
cb := func() {
    doSomething()
}
button.ConnectClicked(&cb)  // Pointer to cb

// DON'T: Use inline function (GC issues)
button.ConnectClicked(&func() {  // Won't compile, but the pattern is wrong
    doSomething()
}())
```

### Storing Widget References

```go
// DO: Store references to widgets you need to update
type PageController struct {
    statusLabel *gtk.Label  // Keep reference for updates
}

// DON'T: Try to find widgets later by traversing tree
// GTK traversal is awkward and error-prone
```

### Main Thread Safety

```go
// DO: Always update widgets from main thread
go func() {
    result := expensiveOperation()
    RunOnMainThread(func() {
        label.SetText(result)
    })
}()

// DON'T: Update widgets from goroutines
go func() {
    result := expensiveOperation()
    label.SetText(result)  // Race condition, may crash
}()
```

---

## Anti-Patterns to Avoid

### Anti-Pattern 1: Monolithic View File

**What:** Single file with all UI code
**Why bad:** Impossible to test, find code, or refactor safely
**Instead:** Feature-based packages with clear boundaries

### Anti-Pattern 2: Global Mutable State

**What:** Module-level variables for manager instances
**Current code:**
```go
var (
    flatpakManager pm.Manager
    flatpakMu      sync.RWMutex
)
```
**Why problematic:** Hard to test, implicit dependencies
**Instead:** Pass dependencies explicitly, use interfaces

### Anti-Pattern 3: Callback Soup

**What:** Deeply nested callbacks for async operations
**Why bad:** Hard to follow, hard to error handle
**Instead:** Use channels for progress, structured error returns

### Anti-Pattern 4: Direct Widget Manipulation from Business Logic

**What:** Business logic knowing about GTK widget types
**Why bad:** Can't test business logic without GTK
**Instead:** Interface boundaries, data events instead of widget references

### Anti-Pattern 5: Ignoring Unref

**What:** Creating widgets without cleanup
**Why bad:** Memory leaks
**Instead:** Unref manually OR add to container (which manages lifecycle)

---

## Refactoring Approach

### Phase 1: Extract Shared Infrastructure

1. Extract `runOnMainThread` to `views/shared/async.go`
2. Extract interfaces to `views/shared/interfaces.go`
3. Extract widget builders to `views/shared/widgets.go`
4. Keep tests passing (currently none, so add foundation)

### Phase 2: Extract Progress System

1. Create unified `ProgressEvent` type
2. Create `ProgressDisplay` component
3. Refactor NBC progress to use new system
4. Add tests for progress handling

### Phase 3: Split Views by Feature

1. Create `views/system/` package, move system page
2. Create `views/updates/` package, move update pages
3. Create `views/applications/` package
4. Create `views/extensions/` package
5. Update `UserHome` to compose these

### Phase 4: Add Tests

1. Add unit tests for pure logic
2. Add integration tests for page construction
3. Add mock-based tests for handler logic

---

## Confidence Assessment

| Recommendation | Confidence | Rationale |
|----------------|------------|-----------|
| Feature-based packages | HIGH | Standard Go practice, proven scalable |
| Unified async/progress | HIGH | Already partially implemented, frostyard/pm pattern |
| Interface boundaries | HIGH | Standard Go testing practice |
| Widget builders | MEDIUM | Reduces boilerplate, but may over-engineer |
| Testing strategy | MEDIUM | GTK limits what's testable; interface mocks help |
| puregotk patterns | HIGH | Verified from examples and existing code |

---

## Sources

| Source | Type | Confidence |
|--------|------|------------|
| puregotk GitHub README | Official | HIGH |
| puregotk examples directory | Official | HIGH |
| frostyard/pm progress pattern | Official | HIGH |
| Chairlift existing codebase | Primary | HIGH |
| GTK4 Getting Started Guide | Official | MEDIUM (C-focused) |
| Go best practices | Standard | HIGH |

---

*Research completed: 2026-01-26*
