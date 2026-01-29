# Phase 1: Async Foundation - Research

**Researched:** 2026-01-26
**Domain:** GTK4/Go async operations, threading, GC safety
**Confidence:** HIGH

## Summary

This phase establishes the async infrastructure for Chairlift: a unified `RunOnMain()` function, callback registry to prevent GC issues, consistent error messaging, and structured async patterns. The research reveals that the core patterns already exist in the codebase but are fragmented across `views/userhome.go` and `pm/wrapper.go`.

The current `runOnMainThread()` function in `userhome.go` (lines 36-55) demonstrates the correct pattern using `glib.IdleAdd` with a callback registry to prevent garbage collection. This same pattern is duplicated in `pm/wrapper.go`. The phase goal is to consolidate this into a shared `internal/async` package and establish consistent usage across the codebase.

Error handling currently uses `ShowToast()` and `ShowErrorToast()` for user notifications, but lacks the "summary + technical hint" style decided in CONTEXT.md. Error messages need refinement to be more user-friendly while preserving technical details for debugging.

**Primary recommendation:** Extract the existing `runOnMainThread()` pattern to `internal/async/scheduler.go`, add structured error types, and refactor all goroutine-to-UI communication to use this unified entry point.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| puregotk/v4/glib | v0.0.0-20260115100645 | GTK main loop integration | Only option for puregotk - provides `IdleAdd` for thread-safe UI updates |
| Go sync package | Go 1.25 | Mutex coordination | Standard library, protects callback registry |
| Go context package | Go 1.25 | Cancellation propagation | Standard library, enables operation cancellation |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Go sync/atomic | Go 1.25 | Atomic counter for callback IDs | Thread-safe ID generation |
| puregotk/v4/adw | v0.0.0-20260115100645 | Toast notifications | Error/success feedback via `adw.Toast` |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| glib.IdleAdd | glib.TimeoutAdd | IdleAdd runs when idle (correct for UI updates); TimeoutAdd is time-based |
| Callback registry | Go finalizers | Finalizers unreliable with CGO-adjacent patterns; registry is explicit and proven |
| Mutex-protected map | sync.Map | Mutex is simpler for uintptr→func() pattern; sync.Map optimized for different access patterns |

**Installation:**
```bash
# Already in go.mod - no additional dependencies needed
go mod tidy
```

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── async/                    # NEW: Centralized async infrastructure
│   ├── scheduler.go          # RunOnMain(), callback registry
│   ├── operation.go           # Async operation wrapper (future phase)
│   └── errors.go             # UserError type, formatting
├── views/
│   └── userhome.go           # Uses async.RunOnMain() instead of local
└── pm/
    └── wrapper.go            # Uses async.RunOnMain() instead of local
```

### Pattern 1: Callback Registry for GC Safety
**What:** Store callbacks in a map keyed by ID to prevent garbage collection before GTK executes them.
**When to use:** Always when scheduling work on GTK main thread from goroutines.
**Example:**
```go
// Source: Existing pattern in userhome.go:28-55, consolidated
package async

import (
    "sync"
    "github.com/jwijenbergh/puregotk/v4/glib"
)

var (
    callbackMu sync.Mutex
    callbacks  = make(map[uintptr]func())
    callbackID uintptr
)

// RunOnMain schedules a function to run on the GTK main thread.
// This is the ONLY safe way to update UI from goroutines.
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
        return false // Remove source after execution
    })
    glib.IdleAdd(&cb, id)
}
```

### Pattern 2: User-Friendly Error Wrapper
**What:** Structured error type that separates user message from technical details.
**When to use:** All errors that may be displayed to users.
**Example:**
```go
// Source: Pattern based on CONTEXT.md decisions
package async

import "fmt"

// UserError wraps an error with user-friendly messaging.
type UserError struct {
    Summary    string // User-facing summary: "Couldn't install Firefox"
    Hint       string // Optional action hint: "Check your internet connection"
    Technical  error  // Original error for logging/debugging
}

func (e *UserError) Error() string {
    return e.Summary
}

func (e *UserError) Unwrap() error {
    return e.Technical
}

// FormatForUser returns the display message without technical details.
func (e *UserError) FormatForUser() string {
    if e.Hint != "" {
        return fmt.Sprintf("%s: %s", e.Summary, e.Hint)
    }
    return e.Summary
}

// FormatWithDetails returns message + technical error for expandable view.
func (e *UserError) FormatWithDetails() string {
    if e.Technical != nil {
        return fmt.Sprintf("%s\n\nDetails: %v", e.FormatForUser(), e.Technical)
    }
    return e.FormatForUser()
}
```

### Pattern 3: Async Operation with Main Thread Callback
**What:** Standard pattern for background work that updates UI on completion.
**When to use:** All long-running operations (network, subprocess, file I/O).
**Example:**
```go
// Source: Existing pattern throughout userhome.go, standardized
func doAsyncWork(button *gtk.Button, toaster ToastAdder) {
    // Disable UI during operation
    button.SetSensitive(false)
    button.SetLabel("Working...")

    go func() {
        // Do background work
        result, err := someBackendCall()

        // Update UI on main thread
        async.RunOnMain(func() {
            button.SetSensitive(true)
            button.SetLabel("Do Thing")

            if err != nil {
                // Use UserError for friendly messages
                if userErr, ok := err.(*async.UserError); ok {
                    toaster.ShowErrorToast(userErr.FormatForUser())
                } else {
                    toaster.ShowErrorToast(fmt.Sprintf("Unexpected error: %v", err))
                }
                return
            }
            toaster.ShowToast("Success!")
        })
    }()
}
```

### Anti-Patterns to Avoid
- **Direct widget updates from goroutines:** Always use `async.RunOnMain()`. GTK is not thread-safe; violations cause segfaults or silent corruption.
- **Inline anonymous callbacks to `glib.IdleAdd`:** Creates GC race conditions. Always store in registry.
- **Module-level `runOnMainThread` duplication:** Use the shared `async` package. Current code has this in two files.
- **Error messages without context:** "Failed" is bad. "Couldn't install Firefox: remote not reachable" is good.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Main thread scheduling | Custom glib.IdleAdd wrapper | `async.RunOnMain()` (this phase) | GC issues are subtle; one pattern prevents bugs |
| Callback GC prevention | runtime.KeepAlive | Callback registry map | KeepAlive doesn't work reliably with CGO-adjacent patterns |
| User error formatting | String concatenation | `async.UserError` type | Separates concerns, enables expandable details |
| Operation cancellation | Channel signaling | `context.Context` | Standard library, integrates with timeouts and deadlines |

**Key insight:** GTK main thread safety is a cross-cutting concern. Having multiple implementations guarantees inconsistency and subtle bugs that surface months later.

## Common Pitfalls

### Pitfall 1: GC Collects Callback Before GTK Uses It
**What goes wrong:** Go GC collects the callback function between when it's passed to `glib.IdleAdd` and when GTK actually executes it, causing segfaults or no-ops.
**Why it happens:** puregotk passes function pointers to C. If Go doesn't hold a reference, GC may collect the function.
**How to avoid:** Use callback registry pattern: store in map before `IdleAdd`, delete after execution.
**Warning signs:** Intermittent crashes, callbacks that "sometimes don't fire", crashes under memory pressure.

### Pitfall 2: Duplicate runOnMainThread Implementations Diverge
**What goes wrong:** `userhome.go` and `pm/wrapper.go` both have their own implementations. Over time, one gets fixed/improved but the other doesn't.
**Why it happens:** Copy-paste during initial development; no shared package existed.
**How to avoid:** Extract to `internal/async` package immediately. Update all call sites.
**Warning signs:** Two files with similar `idleCallback` registry code, different mutex names.

### Pitfall 3: Error Messages Lose Context in Goroutine Transitions
**What goes wrong:** Backend returns `err`, goroutine catches it, shows user "Operation failed" without explaining what or why.
**Why it happens:** Information loss when wrapping/unwrapping errors across boundaries.
**How to avoid:** Use `UserError` type that carries summary, hint, and original error. Format at display time, not at creation time.
**Warning signs:** Users report "it says error but doesn't say what error".

### Pitfall 4: Updating UI Directly from Progress Callbacks
**What goes wrong:** Progress callback from pm library updates widgets directly. Works most of the time, crashes occasionally.
**Why it happens:** pm library callbacks may run on different goroutines depending on the operation.
**How to avoid:** Progress callbacks schedule UI updates via `async.RunOnMain()`. Current `pm/wrapper.go` does this correctly (line 417).
**Warning signs:** Progress UI updates inconsistently, occasional crashes during operations.

### Pitfall 5: Mutex Deadlock Between UI and Progress Tracking
**What goes wrong:** UI update holds `callbackMu`, calls progress tracker that tries to acquire same mutex.
**Why it happens:** Callback registry mutex acquired, then code inside callback tries to schedule another callback.
**How to avoid:** Keep `RunOnMain()` implementation minimal - acquire lock, add to map, release lock, then call `glib.IdleAdd`. Never hold lock during GTK calls.
**Warning signs:** App freezes (not crashes) during complex operations.

## Code Examples

Verified patterns from official sources:

### Basic RunOnMain Usage
```go
// Source: Pattern verified in codebase views/userhome.go
import "github.com/frostyard/chairlift/internal/async"

// In a goroutine:
go func() {
    result := doExpensiveWork()
    
    async.RunOnMain(func() {
        label.SetText(result)
        spinner.Stop()
    })
}()
```

### Error Handling with UserError
```go
// Source: Pattern based on CONTEXT.md decisions
func loadData() error {
    data, err := backend.Fetch()
    if err != nil {
        return &async.UserError{
            Summary:   "Couldn't load application list",
            Hint:      "Check your internet connection",
            Technical: err,
        }
    }
    return nil
}

// At display time:
async.RunOnMain(func() {
    if err != nil {
        if userErr, ok := err.(*async.UserError); ok {
            // Show friendly message to user
            toaster.ShowErrorToast(userErr.FormatForUser())
            // Log technical details
            log.Printf("Error details: %v", userErr.Technical)
        }
    }
})
```

### Toast Notification Patterns (per CONTEXT.md)
```go
// Success: auto-dismiss
toast := adw.NewToast(message)
toast.SetTimeout(3)  // Auto-dismiss after 3 seconds
toastOverlay.AddToast(toast)

// Error: persistent until dismissed
toast := adw.NewToast(message)
toast.SetTimeout(0)  // 0 = don't auto-dismiss
toastOverlay.AddToast(toast)

// Multiple errors: stack them (per CONTEXT.md decision)
for _, err := range errors {
    toast := adw.NewToast(err.FormatForUser())
    toast.SetTimeout(0)
    toastOverlay.AddToast(toast)  // Each gets its own toast, stacked vertically
}
```

### Loading Delay Pattern (per CONTEXT.md)
```go
// Source: CONTEXT.md - 200-300ms delay before spinner
const loadingDelay = 250 * time.Millisecond

func showLoadingAfterDelay(spinner *gtk.Spinner, done <-chan struct{}) {
    select {
    case <-done:
        // Operation completed before delay - don't show spinner (avoid flicker)
        return
    case <-time.After(loadingDelay):
        async.RunOnMain(func() {
            spinner.Start()
            spinner.SetVisible(true)
        })
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| CGO GTK bindings (gotk3/gotk4) | puregotk (pure Go) | 2024 | 40s vs 15min compile, no C toolchain needed |
| Manual glib.IdleAdd | Callback registry pattern | Already in codebase | Prevents GC-related crashes |
| Error strings | Structured UserError | This phase | Better UX, preserves debug info |

**Deprecated/outdated:**
- `runtime.KeepAlive` for callback preservation: Doesn't work reliably with puregotk pattern. Use callback registry.
- Direct widget updates from goroutines: Never safe. Always route through `RunOnMain()`.

## Open Questions

Things that couldn't be fully resolved:

1. **Exact spinner delay timing**
   - What we know: CONTEXT.md says 200-300ms delay before showing spinner
   - What's unclear: Whether this should be configurable or hardcoded
   - Recommendation: Start with 250ms constant, extract to config if needed later

2. **Retry backoff strategy**
   - What we know: CONTEXT.md says 2-3 retries, auto-retry network issues
   - What's unclear: Exact backoff timing (linear, exponential)
   - Recommendation: Claude's discretion per CONTEXT.md - suggest exponential backoff: 1s, 2s, 4s

3. **Operation cancellation scope**
   - What we know: Context-based cancellation is standard Go pattern
   - What's unclear: Should page deactivation cancel in-progress operations?
   - Recommendation: Implement context cancellation now, wire up to page lifecycle in later phase

## Sources

### Primary (HIGH confidence)
- **Codebase analysis**: `internal/views/userhome.go` lines 28-55 (existing callback registry)
- **Codebase analysis**: `internal/pm/wrapper.go` lines 410-417 (duplicate pattern)
- **puregotk GitHub**: Verified `glib.IdleAdd` usage pattern
- **CONTEXT.md**: User decisions on error UX, loading patterns, retry behavior

### Secondary (MEDIUM confidence)
- **STACK.md research**: Async/progress framework patterns
- **ARCHITECTURE.md research**: Component boundaries, package structure
- **PITFALLS.md research**: Threading violations, GC issues

### Tertiary (LOW confidence)
- None - all findings verified against codebase or official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - puregotk is the only option, patterns verified in codebase
- Architecture: HIGH - pattern exists and works, just needs consolidation  
- Pitfalls: HIGH - derived from actual bugs/patterns in current codebase

**Research date:** 2026-01-26
**Valid until:** 60 days (stable domain, puregotk unlikely to change patterns)
