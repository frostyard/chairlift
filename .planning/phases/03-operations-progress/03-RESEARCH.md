# Phase 3: Operations & Progress - Research

**Researched:** 2026-01-26
**Domain:** GTK4/Libadwaita operations tracking, progress UI, cancellation patterns
**Confidence:** HIGH

## Summary

This phase implements a centralized operations tracking system with progress feedback and cancellation support. The core challenge is building a thread-safe operations registry that coordinates between async operations (running in goroutines) and UI updates (running on GTK main thread), while providing consistent visual feedback across the application.

The existing codebase already has strong foundations: the `async` package with `RunOnMain()` for thread-safe UI updates, `widgets` package with reusable patterns (AsyncExpanderRow, ActionButton, LoadingRow), and the `pm` package with progress callback infrastructure. This phase builds on these foundations to add centralized tracking.

**Primary recommendation:** Build an `operations` package that provides a registry for tracking operations, integrating with existing widgets for inline progress and a new header popover for centralized viewing.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| puregotk (adw) | current | AdwAlertDialog for confirmation dialogs | GNOME HIG-compliant dialogs since libadwaita 1.5 |
| puregotk (gtk) | current | gtk.Spinner for short ops, gtk.ProgressBar for long ops | Standard GTK4 progress widgets |
| puregotk (gtk) | current | gtk.MenuButton + gtk.Popover for operations panel | Standard approach for header dropdown panels |
| puregotk (adw) | current | adw.Toast for cancellation notifications | Existing pattern in codebase for notifications |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| sync | stdlib | Mutex for thread-safe registry access | Protecting shared operation state |
| context | stdlib | Context with cancel for operation cancellation | Propagating cancellation to running operations |
| time | stdlib | Tracking operation start time, duration, thresholds | Determining when to show cancel button (>5s), progress bar (>30s) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Header popover | BottomSheet (existing) | User decided header popover - already exists but not used for operations |
| Custom progress | adw.StatusPage | StatusPage is for empty/error states, not inline progress |

**Installation:**
```bash
# Already available in puregotk
```

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── operations/         # NEW - centralized operations tracking
│   ├── doc.go          # Package documentation
│   ├── registry.go     # Thread-safe operation registry
│   ├── operation.go    # Operation struct and lifecycle
│   └── popover.go      # Header popover UI widget
├── widgets/            # EXISTING - extend with progress widgets
│   ├── progress_row.go # NEW - ActionRow with embedded progress
│   └── ...
├── async/              # EXISTING - unchanged
└── views/              # EXISTING - update to use operations package
```

### Pattern 1: Operations Registry (Singleton with Thread-Safe Access)
**What:** A global registry that tracks all active and completed operations with mutex protection
**When to use:** Always - all operations must register with the central registry

```go
// Source: Pattern derived from existing pm/wrapper.go mutex patterns
package operations

import (
    "context"
    "sync"
    "time"
)

type OperationState int

const (
    StateActive OperationState = iota
    StateCompleted
    StateFailed
    StateCancelled
)

type Operation struct {
    ID          string
    Name        string
    Category    string  // "install", "update", "loading"
    State       OperationState
    StartedAt   time.Time
    EndedAt     time.Time
    Progress    float64 // 0.0-1.0, -1 for indeterminate
    Message     string
    Cancellable bool
    CancelFunc  context.CancelFunc
    Error       error
    RetryFunc   func() // Called when retry clicked for failed ops
}

type Registry struct {
    mu         sync.RWMutex
    operations map[string]*Operation
    history    []*Operation // Completed ops for current session
    listeners  []func(*Operation) // UI update callbacks
}

var defaultRegistry = &Registry{
    operations: make(map[string]*Operation),
}

func Start(name, category string, cancellable bool) *Operation {
    // Create context with cancel if cancellable
    // Add to registry
    // Notify listeners
}

func (o *Operation) UpdateProgress(progress float64, message string) {
    // Lock, update, notify listeners via RunOnMain
}

func (o *Operation) Complete(err error) {
    // Move to completed/failed state
    // Move to history if successful, keep in active if failed with retry
}
```

### Pattern 2: Inline Progress with Widget Composition
**What:** Progress displays inline next to the triggering widget (button, row)
**When to use:** Primary location for all operation progress

```go
// Source: Derived from existing ActionButton pattern
package widgets

// ProgressButton extends ActionButton with progress display
type ProgressButton struct {
    *ActionButton
    progressBar *gtk.ProgressBar
    spinner     *gtk.Spinner
    cancelBtn   *gtk.Button
    operation   *operations.Operation
}

func (pb *ProgressButton) StartOperation(name, category string, cancellable bool) *operations.Operation {
    op := operations.Start(name, category, cancellable)
    pb.showProgress(op)
    return op
}

func (pb *ProgressButton) showProgress(op *operations.Operation) {
    // Initially show spinner
    // After 30s threshold, transition to progress bar
    // If cancellable, show cancel button
}
```

### Pattern 3: Header Popover for Operations Overview
**What:** A MenuButton in the header that opens a popover showing all operations
**When to use:** Secondary location - shows operations running in background

```go
// Source: Existing window.go buildMenuButton pattern + GTK4 MenuButton docs
func BuildOperationsButton() *gtk.MenuButton {
    button := gtk.NewMenuButton()
    button.SetIconName("view-paged-symbolic") // Or "emblem-synchronizing-symbolic"
    button.SetTooltipText("Operations")
    
    // Badge overlay for count
    // Popover with adw.ViewStack for active/history tabs
    
    popover := gtk.NewPopover()
    content := buildOperationsContent()
    popover.SetChild(content)
    button.SetPopover(popover)
    
    return button
}
```

### Pattern 4: Confirmation Dialog for Cancellation
**What:** Use AdwAlertDialog for cancel confirmation per user decision
**When to use:** Always when user clicks cancel on a cancellable operation

```go
// Source: Official Libadwaita AdwAlertDialog documentation
func showCancelConfirmation(window *gtk.Window, opName string, onConfirm func()) {
    dialog := adw.NewAlertDialog("Cancel Operation?", "")
    dialog.FormatBody("This will cancel %s. This action cannot be undone.", opName)
    
    dialog.AddResponse("continue", "_Continue")
    dialog.AddResponse("cancel", "_Cancel Operation")
    
    dialog.SetResponseAppearance("cancel", adw.ResponseDestructiveValue)
    dialog.SetDefaultResponse("continue")
    dialog.SetCloseResponse("continue")
    
    responseCb := func(dialog adw.AlertDialog, response string) {
        if response == "cancel" {
            onConfirm()
            showCancelledToast()
        }
    }
    dialog.ConnectResponse(&responseCb)
    
    dialog.Present(&window.Widget)
}
```

### Anti-Patterns to Avoid
- **Modifying UI from goroutines:** Always use async.RunOnMain() - GTK is not thread-safe
- **Holding mutex while calling UI code:** Can cause deadlocks; copy data, release lock, then update UI
- **Forgetting to register operations:** All long-running operations MUST use the registry
- **Mixing spinners and progress bars:** Use spinner for <30s, progress bar for >30s, never both
- **Not cleaning up completed operations:** Move successful ops to history; keep failed ops visible for retry

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Thread-safe UI updates | Custom channel system | async.RunOnMain() | Already exists, tested, handles callback registry |
| Spinner animation | Custom animation | gtk.Spinner | GTK handles animation, accessibility |
| Progress indication | Custom progress widget | gtk.ProgressBar | Supports pulse() for indeterminate, fraction for determinate |
| Toast notifications | Custom notification system | adw.Toast + ToastOverlay | Already in codebase, GNOME HIG compliant |
| Confirmation dialogs | Custom dialog window | adw.AlertDialog | Proper response handling, accessibility, keyboard nav |
| Button state management | Manual enable/disable | ActionButton.StartOperation() | Already handles label, sensitivity |

**Key insight:** GTK4/Libadwaita provide complete progress and feedback primitives. The value-add is the centralized registry that coordinates these existing widgets.

## Common Pitfalls

### Pitfall 1: Deadlock from UI Updates Inside Lock
**What goes wrong:** Holding mutex while calling async.RunOnMain(), which may block or callback while still locked
**Why it happens:** Natural to update state and UI together
**How to avoid:** Copy operation data under lock, release lock, then call RunOnMain with copied data
**Warning signs:** Application freezes when starting/completing operations

```go
// BAD
func (r *Registry) UpdateOperation(id string, progress float64) {
    r.mu.Lock()
    defer r.mu.Unlock()
    op := r.operations[id]
    op.Progress = progress
    async.RunOnMain(func() {
        // Still holding lock!
        updateUI(op)
    })
}

// GOOD
func (r *Registry) UpdateOperation(id string, progress float64) {
    r.mu.Lock()
    op := r.operations[id]
    op.Progress = progress
    opCopy := *op // Copy
    r.mu.Unlock() // Release before UI update
    
    async.RunOnMain(func() {
        updateUI(&opCopy)
    })
}
```

### Pitfall 2: Orphaned Operations on Cancel
**What goes wrong:** Cancelling operation but goroutine continues running, context not propagated
**Why it happens:** Not using context properly in the operation chain
**How to avoid:** Always pass context.Context through entire operation chain; check ctx.Done()
**Warning signs:** Cancel shows as completed but operation continues in background

### Pitfall 3: Badge Count Out of Sync
**What goes wrong:** Operations popover badge shows wrong count
**Why it happens:** Multiple code paths updating count without going through registry
**How to avoid:** Badge count derives from registry.ActiveCount() on every change; never cache
**Warning signs:** Badge shows 0 but popover shows active operations, or vice versa

### Pitfall 4: Memory Leak from Accumulated History
**What goes wrong:** History grows unbounded over long session
**Why it happens:** Requirements say "keep all completed operations from current session"
**How to avoid:** Cap history at reasonable limit (e.g., 100 items); drop oldest when exceeding
**Warning signs:** Memory usage grows with app usage duration

### Pitfall 5: Progress Bar Flicker on Fast Operations
**What goes wrong:** Progress bar appears and disappears rapidly for quick operations
**Why it happens:** Threshold check happens after operation starts
**How to avoid:** Only show progress bar after 30s threshold elapsed; start with spinner that transitions
**Warning signs:** Visual flashing during rapid successive operations

## Code Examples

Verified patterns from official sources:

### Creating gtk.Spinner
```go
// Source: GTK4 Spinner documentation
spinner := gtk.NewSpinner()
spinner.Start() // Begin animation
// ... later
spinner.Stop()  // Stop animation
```

### Creating gtk.ProgressBar with Indeterminate Mode
```go
// Source: GTK4 ProgressBar documentation
progressBar := gtk.NewProgressBar()
progressBar.SetHexpand(true)

// Indeterminate (pulsing) mode
progressBar.Pulse()  // Call repeatedly to animate pulse

// Determinate mode
progressBar.SetFraction(0.5)  // 50% complete
progressBar.SetShowText(true)
progressBar.SetText("50%")
```

### Creating Header MenuButton with Custom Popover
```go
// Source: GTK4 MenuButton documentation
menuButton := gtk.NewMenuButton()
menuButton.SetIconName("emblem-synchronizing-symbolic")
menuButton.SetTooltipText("Operations")
menuButton.SetHasFrame(false)  // Flat button style

popover := gtk.NewPopover()
popover.SetChild(&content.Widget)
menuButton.SetPopover(popover)

// Add to header bar
headerBar.PackEnd(&menuButton.Widget)
```

### Creating AdwAlertDialog for Confirmation
```go
// Source: Libadwaita 1.5+ AlertDialog documentation
dialog := adw.NewAlertDialog("Cancel Download?", "")
dialog.SetBody("The download will be stopped and progress will be lost.")

dialog.AddResponse("continue", "_Continue")
dialog.AddResponse("cancel", "_Cancel Download")

dialog.SetResponseAppearance("cancel", adw.ResponseDestructiveValue)
dialog.SetDefaultResponse("continue")
dialog.SetCloseResponse("continue")

responseCb := func(d adw.AlertDialog, response string) {
    if response == "cancel" {
        // Handle cancellation
    }
}
dialog.ConnectResponse(&responseCb)

dialog.Present(&window.Widget)
```

### Operation Lifecycle with Context
```go
// Pattern for cancellable operation
func StartCancellableOperation(name string) {
    ctx, cancel := context.WithCancel(context.Background())
    
    op := operations.Register(name, operations.CategoryInstall, true)
    op.CancelFunc = cancel
    
    go func() {
        defer op.Complete(nil)
        
        for i := 0; i < 100; i++ {
            select {
            case <-ctx.Done():
                op.Complete(ctx.Err())
                return
            default:
                // Do work
                time.Sleep(100 * time.Millisecond)
                op.UpdateProgress(float64(i)/100.0, fmt.Sprintf("Step %d/100", i+1))
            }
        }
    }()
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| GtkMessageDialog | AdwAlertDialog | Libadwaita 1.5 | Better mobile support, response handling |
| GtkSpinner only | Spinner + ProgressBar combo | GTK4 | Clear distinction between indeterminate/determinate |
| Modal progress dialogs | Inline progress + background indicator | GNOME HIG 2020+ | Non-blocking UX per GNOME HIG |

**Deprecated/outdated:**
- GtkMessageDialog: Still works but AdwAlertDialog is preferred for libadwaita apps
- Blocking progress dialogs: Anti-pattern per GNOME HIG; use inline progress

## Open Questions

Things that couldn't be fully resolved:

1. **Operation ID generation strategy**
   - What we know: Need unique IDs for each operation
   - What's unclear: Best approach (UUID, incrementing int, descriptive composite key)
   - Recommendation: Use incrementing uint64 with atomic operations for simplicity

2. **Exact threshold detection for cancel button visibility**
   - What we know: User decided >5s operations are cancellable
   - What's unclear: Should cancel button appear immediately for known-long ops, or after 5s elapsed?
   - Recommendation: Per CONTEXT.md "Cancel button appears immediately when operation starts (for cancellable operations)" - show immediately if operation is marked cancellable

3. **Retry mechanism for failed operations**
   - What we know: Failed operations stay in active list with retry option
   - What's unclear: How retry function is captured and stored
   - Recommendation: Store retry function pointer in Operation struct at creation time

## Sources

### Primary (HIGH confidence)
- GTK4 Documentation (docs.gtk.org/gtk4) - ProgressBar, Spinner, MenuButton, Popover APIs
- Libadwaita 1.8.3 Documentation - AlertDialog, Toast, ToastOverlay APIs
- Existing codebase: async/scheduler.go, widgets/*.go, pm/wrapper.go patterns

### Secondary (MEDIUM confidence)
- GNOME Human Interface Guidelines - inline progress preferred over modal dialogs

### Tertiary (LOW confidence)
- None - all patterns verified with official documentation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Verified with official GTK4/libadwaita documentation
- Architecture: HIGH - Patterns derive from existing codebase patterns
- Pitfalls: MEDIUM - Based on general GTK/Go threading knowledge + codebase analysis

**Research date:** 2026-01-26
**Valid until:** 2026-02-26 (30 days - stable GTK4/libadwaita)
