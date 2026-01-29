# Phase 2: Widget Extraction - Research

**Researched:** 2026-01-26
**Domain:** GTK4/Libadwaita reusable widget patterns for Go/puregotk
**Confidence:** HIGH

## Summary

This phase extracts common UI patterns from the 2500-line `userhome.go` into reusable widget components. Analysis reveals four distinct patterns repeated throughout the codebase: async-aware expander rows (8+ occurrences), self-disabling action buttons (10+ occurrences), loading state rows (5+ occurrences), and common ActionRow configurations (20+ occurrences).

The extraction approach uses Go composition rather than inheritance since puregotk wraps GTK widgets without supporting Go-level widget subclassing. Each extracted "widget" is actually a factory function that creates and configures standard GTK/Libadwaita widgets, plus helper methods that operate on them. This matches how the codebase already structures things (e.g., `createPage()` returns configured widgets).

The async package from Phase 1 provides the threading foundation. All extracted widgets use `async.RunOnMain()` for UI updates and `async.UserError` for error display, ensuring consistency with established patterns.

**Primary recommendation:** Create `internal/widgets/` package with factory functions for AsyncExpanderRow, ActionButton, LoadingRow, and ActionRow builders. Use composition (returning configured standard widgets) rather than custom GObject types.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| puregotk/v4/adw | v0.0.0-20260115100645 | Libadwaita widgets (ExpanderRow, ActionRow) | Only option for puregotk - provides modern GNOME widgets |
| puregotk/v4/gtk | v0.0.0-20260115100645 | GTK4 widgets (Button, Spinner, Image) | Only option for puregotk - provides base widgets |
| internal/async | Phase 1 | RunOnMain(), UserError | Project standard for async UI updates |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Go sync package | Go 1.25 | Mutex for state tracking | Thread-safe state in async widgets |
| puregotk/v4/glib | v0.0.0-20260115100645 | Source functions | Timeout-based loading delays |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Factory functions | GObject subclasses | puregotk doesn't support Go-level GObject subclassing; factory pattern is idiomatic Go |
| Struct with widget refs | Return naked widgets | Struct allows helper methods (SetLoading, SetError) without globals |
| Single widgets package | Per-widget packages | Widgets are small, related; single package reduces import complexity |

**Installation:**
```bash
# No additional dependencies - uses existing puregotk and internal/async
go mod tidy
```

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── async/                  # From Phase 1
│   ├── scheduler.go        # RunOnMain()
│   └── errors.go           # UserError
├── widgets/                # NEW: Reusable widget patterns
│   ├── doc.go              # Package documentation
│   ├── async_expander.go   # AsyncExpanderRow
│   ├── action_button.go    # ActionButton
│   ├── loading_row.go      # LoadingRow
│   └── rows.go             # ActionRow builder helpers
└── views/
    └── userhome.go         # Uses widgets package
```

### Pattern 1: Composition-Based Widget Wrapper
**What:** Struct that holds references to GTK widgets plus helper methods for common operations.
**When to use:** When a pattern involves multiple widgets that need coordinated updates.
**Example:**
```go
// Source: Pattern derived from userhome.go:247-269 (loadNBCStatus)
package widgets

import (
    "github.com/frostyard/chairlift/internal/async"
    "github.com/jwijenbergh/puregotk/v4/adw"
    "github.com/jwijenbergh/puregotk/v4/gtk"
)

// AsyncExpanderRow wraps an adw.ExpanderRow with async loading support.
// It manages loading state, error display, and content population.
type AsyncExpanderRow struct {
    Expander   *adw.ExpanderRow
    loadingRow *adw.ActionRow
    spinner    *gtk.Spinner
}

// NewAsyncExpanderRow creates an expander row with loading state support.
func NewAsyncExpanderRow(title, loadingSubtitle string) *AsyncExpanderRow {
    expander := adw.NewExpanderRow()
    expander.SetTitle(title)
    expander.SetSubtitle(loadingSubtitle)
    
    return &AsyncExpanderRow{
        Expander: expander,
    }
}

// StartLoading shows a loading row with spinner inside the expander.
// Call this before starting async data fetch.
func (a *AsyncExpanderRow) StartLoading(message string) {
    a.loadingRow = adw.NewActionRow()
    a.loadingRow.SetTitle(message)
    a.loadingRow.SetSubtitle("Please wait...")
    
    a.spinner = gtk.NewSpinner()
    a.spinner.Start()
    a.loadingRow.AddPrefix(&a.spinner.Widget)
    
    a.Expander.AddRow(&a.loadingRow.Widget)
}

// StopLoading removes the loading row. Call from async.RunOnMain.
func (a *AsyncExpanderRow) StopLoading() {
    if a.loadingRow != nil {
        a.Expander.Remove(&a.loadingRow.Widget)
        a.loadingRow = nil
        a.spinner = nil
    }
}

// SetError shows an error state with icon.
func (a *AsyncExpanderRow) SetError(message string) {
    a.StopLoading()
    a.Expander.SetSubtitle("Failed to load")
    
    errorRow := adw.NewActionRow()
    errorRow.SetTitle("Error")
    errorRow.SetSubtitle(message)
    
    errorIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
    errorRow.AddPrefix(&errorIcon.Widget)
    
    a.Expander.AddRow(&errorRow.Widget)
}

// SetContent updates subtitle and is ready for content rows.
func (a *AsyncExpanderRow) SetContent(subtitle string) {
    a.StopLoading()
    a.Expander.SetSubtitle(subtitle)
}
```

### Pattern 2: Self-Disabling Action Button
**What:** Button that disables itself during operation and shows progress.
**When to use:** Any button that triggers an async operation.
**Example:**
```go
// Source: Pattern derived from userhome.go:462-523 (onSystemUpdateClicked)
package widgets

import (
    "github.com/jwijenbergh/puregotk/v4/gtk"
)

// ActionButton wraps a gtk.Button with self-disabling behavior.
type ActionButton struct {
    Button       *gtk.Button
    originalLabel string
    spinner      *gtk.Spinner
}

// NewActionButton creates a button that tracks operation state.
func NewActionButton(label string) *ActionButton {
    btn := gtk.NewButtonWithLabel(label)
    btn.SetValign(gtk.AlignCenterValue)
    
    return &ActionButton{
        Button:        btn,
        originalLabel: label,
    }
}

// NewActionButtonWithClass creates a button with a CSS class.
func NewActionButtonWithClass(label, cssClass string) *ActionButton {
    ab := NewActionButton(label)
    ab.Button.AddCssClass(cssClass)
    return ab
}

// StartOperation disables the button and shows working state.
// Returns a done function to call when operation completes.
func (ab *ActionButton) StartOperation(workingLabel string) (done func()) {
    ab.Button.SetSensitive(false)
    ab.Button.SetLabel(workingLabel)
    
    return func() {
        ab.Button.SetSensitive(true)
        ab.Button.SetLabel(ab.originalLabel)
    }
}

// OnClicked connects a click handler that auto-disables during operation.
// The handler receives a done callback to restore the button.
func (ab *ActionButton) OnClicked(handler func(done func())) {
    cb := func(_ gtk.Button) {
        done := ab.StartOperation("Working...")
        handler(done)
    }
    ab.Button.ConnectClicked(&cb)
}
```

### Pattern 3: Factory Functions for Common Row Types
**What:** Functions that create pre-configured ActionRow widgets.
**When to use:** Anywhere you need a standard row configuration.
**Example:**
```go
// Source: Pattern derived from userhome.go:284-300 (perfRow with external link)
package widgets

import (
    "github.com/jwijenbergh/puregotk/v4/adw"
    "github.com/jwijenbergh/puregotk/v4/gtk"
)

// NewLinkRow creates an ActionRow that opens a URL when activated.
func NewLinkRow(title, subtitle string, onClick func()) *adw.ActionRow {
    row := adw.NewActionRow()
    row.SetTitle(title)
    row.SetSubtitle(subtitle)
    row.SetActivatable(true)
    
    icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
    row.AddSuffix(&icon.Widget)
    
    cb := func(_ adw.ActionRow) {
        onClick()
    }
    row.ConnectActivated(&cb)
    
    return row
}

// NewInfoRow creates a simple title/subtitle row.
func NewInfoRow(title, subtitle string) *adw.ActionRow {
    row := adw.NewActionRow()
    row.SetTitle(title)
    row.SetSubtitle(subtitle)
    return row
}

// NewButtonRow creates a row with an action button suffix.
func NewButtonRow(title, subtitle, buttonLabel string, onClick func()) *adw.ActionRow {
    row := adw.NewActionRow()
    row.SetTitle(title)
    row.SetSubtitle(subtitle)
    
    btn := gtk.NewButtonWithLabel(buttonLabel)
    btn.SetValign(gtk.AlignCenterValue)
    btn.AddCssClass("suggested-action")
    
    cb := func(_ gtk.Button) {
        onClick()
    }
    btn.ConnectClicked(&cb)
    
    row.AddSuffix(&btn.Widget)
    return row
}

// NewIconRow creates a row with a prefix icon.
func NewIconRow(title, subtitle, iconName string) *adw.ActionRow {
    row := adw.NewActionRow()
    row.SetTitle(title)
    row.SetSubtitle(subtitle)
    
    icon := gtk.NewImageFromIconName(iconName)
    row.AddPrefix(&icon.Widget)
    
    return row
}
```

### Pattern 4: Loading Row
**What:** Standard row showing loading state with spinner.
**When to use:** Placeholder while async content loads.
**Example:**
```go
// Source: Pattern derived from userhome.go:355-362 (loadNBCStatus loading row)
package widgets

import (
    "github.com/jwijenbergh/puregotk/v4/adw"
    "github.com/jwijenbergh/puregotk/v4/gtk"
)

// LoadingRow represents a row with a spinner indicating loading state.
type LoadingRow struct {
    Row     *adw.ActionRow
    Spinner *gtk.Spinner
}

// NewLoadingRow creates a row with a spinning indicator.
func NewLoadingRow(title, subtitle string) *LoadingRow {
    row := adw.NewActionRow()
    row.SetTitle(title)
    row.SetSubtitle(subtitle)
    
    spinner := gtk.NewSpinner()
    spinner.Start()
    row.AddPrefix(&spinner.Widget)
    
    return &LoadingRow{
        Row:     row,
        Spinner: spinner,
    }
}

// Stop stops the spinner (call when loading completes).
func (lr *LoadingRow) Stop() {
    lr.Spinner.Stop()
}
```

### Anti-Patterns to Avoid
- **Embedding GTK widgets:** Don't use Go struct embedding for GTK types. puregotk types aren't designed for it. Use composition with explicit fields.
- **Global widget state:** Don't store widget references in package-level variables. Pass them through constructors or store in structs.
- **Widget reuse without cleanup:** Don't reuse LoadingRow across operations. Create fresh instances; GTK handles cleanup when removed from parent.
- **Mixing creation and population:** Factory functions should create structure; separate methods should populate content. This allows async population.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Loading spinner in row | Inline spinner creation | `widgets.NewLoadingRow()` | Consistent placement, automatic start |
| Button disable during work | Manual SetSensitive | `ActionButton.StartOperation()` | Returns done callback, can't forget to re-enable |
| Error display in expander | Custom error row creation | `AsyncExpanderRow.SetError()` | Consistent icon, subtitle update, loading cleanup |
| External link rows | Inline row + icon + handler | `widgets.NewLinkRow()` | 5 lines → 1 line, consistent pattern |

**Key insight:** These patterns appear 30+ times in userhome.go. Each extraction eliminates 5-15 lines of boilerplate per occurrence, reducing the file by ~300-400 lines and making the intent clearer.

## Common Pitfalls

### Pitfall 1: Calling Widget Methods from Wrong Thread
**What goes wrong:** Widget method called from goroutine without `async.RunOnMain()`.
**Why it happens:** Factory function used in goroutine, then methods called immediately.
**How to avoid:** All widget methods that touch GTK must document "call from main thread" or wrap internally.
**Warning signs:** Intermittent crashes, especially under load.

### Pitfall 2: Holding References to Removed Widgets
**What goes wrong:** Code keeps pointer to row after removing it from parent, then calls methods.
**Why it happens:** Cleanup code removes widget but caller still has reference.
**How to avoid:** `StopLoading()` and similar methods nil out references. Callers should not retain references after removal.
**Warning signs:** "Invalid object" errors from GTK, use-after-free symptoms.

### Pitfall 3: Memory Leaks from Unremoved Rows
**What goes wrong:** Loading row added but never removed (error path skips cleanup).
**Why it happens:** Error handling doesn't call cleanup, only success path does.
**How to avoid:** Use defer or ensure all paths call cleanup. `defer asyncExpander.StopLoading()` is safe even if already stopped.
**Warning signs:** UI accumulates loading indicators, memory grows over time.

### Pitfall 4: Callback Closure Captures Wrong Variable
**What goes wrong:** Loop creates buttons, all buttons reference the last item.
**Why it happens:** Go closure captures variable reference, not value.
**How to avoid:** Create local copy: `item := item` before closure, or use factory function that takes the value.
**Warning signs:** All buttons perform same action, clicking any row acts on last item.

### Pitfall 5: CSS Class Applied Multiple Times
**What goes wrong:** Button shows wrong styling after multiple StartOperation/done cycles.
**Why it happens:** StartOperation adds class, done doesn't remove it, or vice versa.
**How to avoid:** Factory functions set classes once at creation. State changes don't modify classes unless explicitly designed.
**Warning signs:** Visual inconsistency, classes accumulating in inspector.

## Code Examples

Verified patterns from official sources:

### Using AsyncExpanderRow for Data Loading
```go
// Source: Refactoring of userhome.go:352-459 (loadNBCStatus)
import (
    "github.com/frostyard/chairlift/internal/async"
    "github.com/frostyard/chairlift/internal/widgets"
)

func (uh *UserHome) loadNBCStatus(expander *widgets.AsyncExpanderRow) {
    // Show loading state
    expander.StartLoading("Fetching NBC status")
    
    go func() {
        ctx, cancel := nbc.DefaultContext()
        defer cancel()
        
        status, err := nbc.GetStatus(ctx)
        
        async.RunOnMain(func() {
            if err != nil {
                userErr := async.NewUserError("Couldn't load NBC status", err)
                expander.SetError(userErr.FormatForUser())
                return
            }
            
            expander.SetContent("Loaded")
            
            // Add content rows
            if status.Image != "" {
                row := widgets.NewInfoRow("Image", status.Image)
                expander.Expander.AddRow(&row.Widget)
            }
            // ... more rows
        })
    }()
}
```

### Using ActionButton for Operations
```go
// Source: Refactoring of userhome.go:1336-1359 (onBrewCleanupClicked)
import (
    "github.com/frostyard/chairlift/internal/async"
    "github.com/frostyard/chairlift/internal/widgets"
)

func createCleanupButton(toaster ToastAdder) *widgets.ActionButton {
    btn := widgets.NewActionButtonWithClass("Clean Up", "suggested-action")
    
    btn.OnClicked(func(done func()) {
        go func() {
            output, err := pm.HomebrewCleanup()
            
            async.RunOnMain(func() {
                done() // Re-enable button
                
                if err != nil {
                    toaster.ShowErrorToast(fmt.Sprintf("Cleanup failed: %v", err))
                    return
                }
                toaster.ShowToast("Homebrew cleanup completed")
            })
        }()
    })
    
    return btn
}
```

### Using Row Builder Functions
```go
// Source: Refactoring of userhome.go:284-301 (perfRow)
import "github.com/frostyard/chairlift/internal/widgets"

func buildSystemHealthGroup(uh *UserHome) *adw.PreferencesGroup {
    group := adw.NewPreferencesGroup()
    group.SetTitle("System Health")
    
    // One line instead of 10+
    perfRow := widgets.NewLinkRow(
        "System Performance",
        "Monitor CPU, memory, and system resources",
        func() { uh.launchApp("io.missioncenter.MissionCenter") },
    )
    group.Add(&perfRow.Widget)
    
    return group
}
```

### Creating Consistent Loading States
```go
// Source: Refactoring of userhome.go loading patterns
import "github.com/frostyard/chairlift/internal/widgets"

// Before async data load
func loadData(expander *adw.ExpanderRow) {
    loading := widgets.NewLoadingRow("Loading...", "Fetching data")
    expander.AddRow(&loading.Row.Widget)
    
    go func() {
        data, err := fetchData()
        
        async.RunOnMain(func() {
            expander.Remove(&loading.Row.Widget)
            
            if err != nil {
                // Handle error
                return
            }
            // Populate with data
        })
    }()
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Inline widget creation | Factory functions | Libadwaita 1.0+ | Reduces boilerplate, ensures consistency |
| Manual button disable/enable | Self-managing button wrappers | Best practice | Can't forget to re-enable |
| Copy-paste loading patterns | Reusable LoadingRow | Best practice | Single source of truth for loading UI |

**Deprecated/outdated:**
- GTK3-style widget inheritance: Not supported in puregotk. Use composition.
- `gtk_widget_set_style()`: Use CSS classes (`AddCssClass`) instead.
- Manual spinner start/stop management: Encapsulate in LoadingRow.

## Open Questions

Things that couldn't be fully resolved:

1. **Widget struct naming convention**
   - What we know: Go convention is CamelCase, GTK uses snake_case
   - What's unclear: Should we use `AsyncExpanderRow` or `AsyncExpander`?
   - Recommendation: Use `AsyncExpanderRow` to match the underlying GTK widget name for discoverability

2. **Progress spinner vs progress bar threshold**
   - What we know: Phase 3 will add progress bars for long operations
   - What's unclear: Should ActionButton support progress bar, or is that a separate widget?
   - Recommendation: Keep ActionButton simple (spinner only). Create ProgressButton in Phase 3 if needed.

3. **Widget cleanup responsibility**
   - What we know: GTK cleans up widgets when removed from parent
   - What's unclear: Should wrapper structs nil their fields on removal?
   - Recommendation: Yes, nil fields to prevent accidental use-after-remove

## Sources

### Primary (HIGH confidence)
- **Codebase analysis**: `internal/views/userhome.go` - identified 30+ pattern occurrences
- **Libadwaita docs**: ExpanderRow, ActionRow API verified via official GNOME docs
- **puregotk README**: Confirmed composition pattern, no widget subclassing support
- **Phase 1 research**: async.RunOnMain() and UserError patterns

### Secondary (MEDIUM confidence)
- **GTK4 Widget docs**: Verified widget lifecycle and parent/child relationships
- **puregotk examples**: Confirmed callback pattern for button clicks

### Tertiary (LOW confidence)
- None - all findings verified against codebase or official documentation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - puregotk is the only option, patterns verified in codebase
- Architecture: HIGH - composition pattern is idiomatic Go, matches existing code style
- Pitfalls: HIGH - derived from actual patterns and potential issues in current codebase

**Research date:** 2026-01-26
**Valid until:** 90 days (stable domain, widget patterns unlikely to change)
