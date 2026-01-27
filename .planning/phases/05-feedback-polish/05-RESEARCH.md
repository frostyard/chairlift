# Phase 5: Feedback Polish - Research

**Researched:** 2026-01-27
**Domain:** GTK4/Libadwaita empty states, status banners, retry patterns
**Confidence:** HIGH

## Summary

This phase implements user feedback polish: empty state pages with guidance text, persistent status banners (for dry-run mode), and retry buttons for failed operations. The requirements build on existing infrastructure from Phase 3 (operations with RetryFunc) and Phase 4 (page packages with dependency injection).

Libadwaita provides two purpose-built widgets: `AdwStatusPage` for empty states with icon, title, description and optional child widget, and `AdwBanner` for persistent status bars with dismissible/revealed state. The existing operations system already has RetryFunc support - this phase wires it to the UI. The existing codebase shows patterns for empty states via simple labels (popover.go line 97), but these should be upgraded to proper StatusPage widgets for consistency with GNOME HIG.

**Primary recommendation:** Use AdwStatusPage for empty states in lists, AdwBanner for dry-run mode indicator at window level, and wire RetryFunc to the existing retry button in operations popover (already implemented in popover.go).

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| puregotk (adw) | current | adw.StatusPage for empty state placeholders | GNOME HIG standard for empty/error states |
| puregotk (adw) | current | adw.Banner for persistent status display | Purpose-built for contextual information bars |
| puregotk (gtk) | current | gtk.Button for retry actions | Already used in popover.go for retry |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| internal/pm | current | IsDryRun() to check dry-run state | Determining banner visibility |
| internal/operations | current | RetryFunc on Operation struct | Enabling retry on failed operations |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| AdwStatusPage | gtk.Label with dim-label | StatusPage is HIG-compliant, has icon, better UX |
| AdwBanner | adw.Toast | Toast is transient, Banner persists and is dismissible |
| AdwBanner | Custom header modification | Banner is purpose-built, handles animation, dismissible |

**Installation:**
```bash
# Already available in puregotk
```

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── widgets/                  # EXTEND
│   ├── status_page.go        # NEW - StatusPage helpers for empty states
│   └── ...
├── window/                   # MODIFY
│   └── window.go             # Add Banner to window structure
├── operations/               # EXISTING - already has retry support
│   ├── popover.go            # Already has retry button wired
│   └── operation.go          # Already has RetryFunc field
└── views/                    # MODIFY
    └── userhome.go           # Use StatusPage for empty lists
```

### Pattern 1: Empty State with AdwStatusPage
**What:** Use AdwStatusPage when a list or container has no items to display
**When to use:** Expander rows with no children, list boxes with no rows, preference groups with no content

```go
// Source: Official Libadwaita AdwStatusPage documentation
func NewEmptyStateRow(title, description, iconName string) *adw.StatusPage {
    status := adw.NewStatusPage()
    status.SetTitle(title)
    status.SetDescription(description)
    status.SetIconName(iconName)
    // Optional: add .compact style class for inline use
    status.AddCssClass("compact")
    return status
}

// Usage in views:
if len(items) == 0 {
    status := widgets.NewEmptyState(
        "No Extensions Installed",
        "Install extensions from the Discover section to add functionality",
        "application-x-addon-symbolic",
    )
    // For inline use in expanders:
    status.AddCssClass("compact")
    expander.AddRow(&status.Widget)
}
```

### Pattern 2: Persistent Status Banner for Dry-Run Mode
**What:** AdwBanner at window level that shows when dry-run mode is active
**When to use:** Any application-wide state that users should be aware of

```go
// Source: Official Libadwaita AdwBanner documentation
func (w *Window) setupDryRunBanner() {
    w.dryRunBanner = adw.NewBanner("Dry-Run Mode Active")
    // Banner hidden by default, shown when dry-run enabled
    w.dryRunBanner.SetRevealed(false)
    
    // Optional button to disable dry-run
    w.dryRunBanner.SetButtonLabel("Disable")
    
    buttonClickedCb := func(_ adw.Banner) {
        pm.SetDryRun(false)
        w.dryRunBanner.SetRevealed(false)
    }
    w.dryRunBanner.ConnectButtonClicked(&buttonClickedCb)
}

// Banner placement: Add before main content
// Banner should be first child of window content container
```

### Pattern 3: Banner in Window Layout
**What:** Banner positioned at top of window content, before main layout
**When to use:** Dry-run mode or other persistent application states

```go
// Source: Window layout pattern for banner placement
func (w *Window) buildUI() {
    // Create main content (split view, etc.)
    mainContent := w.buildMainContent()
    
    // Create banner
    w.banner = adw.NewBanner("Dry-Run Mode: Changes will be simulated only")
    w.banner.SetRevealed(pm.IsDryRun())
    
    // Create vertical box to stack banner + content
    contentBox := gtk.NewBox(gtk.OrientationVerticalValue, 0)
    contentBox.Append(&w.banner.Widget)
    contentBox.Append(&mainContent.Widget)
    
    // Wrap in toast overlay
    w.toasts.SetChild(&contentBox.Widget)
}

// Update banner visibility when dry-run changes:
func (w *Window) updateDryRunBanner() {
    w.banner.SetRevealed(pm.IsDryRun())
}
```

### Pattern 4: Retry Button for Failed Operations (Already Implemented)
**What:** Retry button shown for failed operations in the operations popover
**When to use:** When Operation.State == StateFailed and Operation.RetryFunc != nil

```go
// Source: Already implemented in operations/popover.go lines 264-280
// The existing code already handles this pattern:
if op.State == StateFailed && op.RetryFunc != nil {
    retryBtn := gtk.NewButton()
    retryBtn.SetLabel("Retry")
    retryBtn.AddCssClass("suggested-action")
    retryBtn.SetValign(gtk.AlignCenterValue)

    opID := op.ID
    retryFn := op.RetryFunc
    clickedCb := func(_ gtk.Button) {
        if foundOp := Get(opID); foundOp != nil && foundOp.RetryFunc != nil {
            retryFn()
        }
    }
    retryBtn.ConnectClicked(&clickedCb)
    row.AddSuffix(&retryBtn.Widget)
}
```

### Pattern 5: Setting RetryFunc When Starting Operations
**What:** Capture retry function when registering an operation
**When to use:** All operations that can be meaningfully retried

```go
// Pattern for registering retryable operations
func (uh *UserHome) onInstallClick(btn *gtk.Button, pkgName string) {
    btn.SetSensitive(false)
    
    // Capture the retry function
    var doInstall func()
    doInstall = func() {
        op := operations.Start("Installing "+pkgName, operations.CategoryInstall, false)
        op.RetryFunc = doInstall  // Self-reference for retry
        
        go func() {
            err := pm.HomebrewInstall(pkgName, false)
            async.RunOnMain(func() {
                btn.SetSensitive(true)
                op.Complete(err)  // If err != nil, stays in active list for retry
            })
        }()
    }
    doInstall()
}
```

### Anti-Patterns to Avoid
- **Using labels for empty states:** Use AdwStatusPage instead of gtk.Label with dim-label CSS
- **Custom banner implementations:** Use AdwBanner - it handles reveal animation, button styling
- **Multiple banners stacked:** Only show one banner at a time for the most important state
- **Non-dismissible persistent banners:** Banners should generally be dismissible or have action button
- **Retry without clearing failed state:** When retry is clicked, remove the failed operation first

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Empty state display | Custom box with label + icon | AdwStatusPage | HIG-compliant, proper styling, accessibility |
| Status bar | Custom header label | AdwBanner | Reveal animation, button support, proper styling |
| Retry mechanism | Custom retry tracking | operations.RetryFunc | Already implemented in Phase 3 |
| Dismiss animation | Manual visibility toggle | Banner.SetRevealed() | Smooth animation built-in |

**Key insight:** Libadwaita 1.3+ provides AdwBanner and AdwStatusPage specifically for these patterns. Using them ensures GNOME HIG compliance and consistent visual language.

## Common Pitfalls

### Pitfall 1: StatusPage Without Compact Class in Inline Context
**What goes wrong:** StatusPage takes too much vertical space inside expander or popover
**Why it happens:** Default StatusPage is designed for full-page empty states
**How to avoid:** Add `.compact` CSS class when using inside expandable containers
**Warning signs:** Empty state pushes content off screen or looks oversized

```go
// GOOD - for inline use
status := adw.NewStatusPage()
status.AddCssClass("compact")

// GOOD - for full-page use (no compact class needed)
status := adw.NewStatusPage()
```

### Pitfall 2: Banner Not Updating on Dry-Run Change
**What goes wrong:** User enables dry-run via command line but banner doesn't appear
**Why it happens:** Banner visibility only checked at startup
**How to avoid:** Either check dry-run state periodically or use listener pattern
**Warning signs:** Dry-run mode active but no visual indication

```go
// Simple approach: check on page navigation
func (w *Window) onNavigate(pageName string) {
    w.banner.SetRevealed(pm.IsDryRun())
    // ...rest of navigation
}
```

### Pitfall 3: Retry Causes Duplicate Operations
**What goes wrong:** Clicking retry creates new operation while old failed one remains
**Why it happens:** Not clearing the failed operation before starting new one
**How to avoid:** In RetryFunc, dismiss the failed operation from active list first
**Warning signs:** Multiple entries for same operation in popover

```go
// Pattern: Clear failed operation, then retry
op.RetryFunc = func() {
    // The registry should move failed op to history on retry
    operations.DismissFailed(op.ID)
    doInstall()  // Start fresh operation
}
```

### Pitfall 4: Banner Obstructing Content
**What goes wrong:** Banner overlaps or pushes content in unexpected ways
**Why it happens:** Banner placed incorrectly in widget hierarchy
**How to avoid:** Banner should be in a vertical Box above main content, not overlay
**Warning signs:** Content layout shifts when banner appears/disappears, or banner overlaps content

### Pitfall 5: Empty State Shown Briefly During Loading
**What goes wrong:** "No items" empty state flickers before async load completes
**Why it happens:** Checking empty state before load finishes
**How to avoid:** Show loading state first, only show empty state after load completes with zero items
**Warning signs:** Flash of empty state then data appears

## Code Examples

Verified patterns from official sources:

### Creating AdwStatusPage
```go
// Source: Official Libadwaita AdwStatusPage documentation
status := adw.NewStatusPage()
status.SetTitle("No Extensions Installed")
status.SetDescription("Install extensions from the Discover section to add new functionality to your system")
status.SetIconName("application-x-addon-symbolic")

// For compact display in constrained spaces
status.AddCssClass("compact")
```

### Creating AdwBanner
```go
// Source: Official Libadwaita AdwBanner documentation (since 1.3)
banner := adw.NewBanner("Dry-Run Mode: Changes will be simulated only")
banner.SetRevealed(false)  // Hidden by default

// Show/hide with animation
banner.SetRevealed(true)   // Shows with slide animation
banner.SetRevealed(false)  // Hides with slide animation

// Optional action button
banner.SetButtonLabel("Disable")
cb := func(_ adw.Banner) {
    // Handle button click
    pm.SetDryRun(false)
    banner.SetRevealed(false)
}
banner.ConnectButtonClicked(&cb)
```

### Empty State Helper Widget
```go
// Reusable helper for creating empty states
package widgets

import (
    "github.com/jwijenbergh/puregotk/v4/adw"
)

// EmptyStateConfig configures an empty state display
type EmptyStateConfig struct {
    Title       string
    Description string
    IconName    string
    Compact     bool  // For inline use in expanders/popovers
}

// NewEmptyState creates a StatusPage configured for empty state display.
//
// Common icon names for empty states:
//   - "folder-symbolic" for empty file/folder lists
//   - "application-x-addon-symbolic" for empty extensions list
//   - "package-x-generic-symbolic" for empty package lists
//   - "emblem-synchronizing-symbolic" for empty operations list
//
// Must be called from the GTK main thread.
func NewEmptyState(cfg EmptyStateConfig) *adw.StatusPage {
    status := adw.NewStatusPage()
    status.SetTitle(cfg.Title)
    status.SetDescription(cfg.Description)
    if cfg.IconName != "" {
        status.SetIconName(cfg.IconName)
    }
    if cfg.Compact {
        status.AddCssClass("compact")
    }
    return status
}
```

### Retry Pattern with Operation Registration
```go
// Pattern for retryable operations with proper cleanup
func startRetryableOperation(name string, work func() error) {
    var doWork func()
    doWork = func() {
        op := operations.Start(name, operations.CategoryInstall, false)
        op.RetryFunc = func() {
            // Clear this failed operation from active list
            operations.DismissOperation(op.ID)
            // Start fresh
            doWork()
        }
        
        go func() {
            err := work()
            async.RunOnMain(func() {
                op.Complete(err)
            })
        }()
    }
    doWork()
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| gtk.Label for empty states | AdwStatusPage | Libadwaita 1.0 | Better UX, consistent styling, accessibility |
| InfoBar for status | AdwBanner | Libadwaita 1.3 | Modern styling, reveal animation, action button |
| Manual retry logic | Operation.RetryFunc | Phase 3 | Centralized retry handling |

**Deprecated/outdated:**
- gtk.InfoBar: Still works but AdwBanner is preferred for libadwaita apps
- Custom empty state boxes: Use StatusPage for consistency

## Open Questions

Things that couldn't be fully resolved:

1. **Multiple persistent states (dry-run + other states)**
   - What we know: Banner handles one status message
   - What's unclear: How to show multiple simultaneous persistent states
   - Recommendation: Prioritize most important state; for now only dry-run needs banner

2. **Retry clearing failed operation**
   - What we know: Failed operations stay in active list for retry (per Phase 3 design)
   - What's unclear: Whether to move to history or delete entirely when retry clicked
   - Recommendation: Move to history with "Retried" state, then start new operation

3. **Banner placement with BottomSheet**
   - What we know: Window uses BottomSheet for progress
   - What's unclear: Whether Banner should be inside or outside BottomSheet content
   - Recommendation: Banner should be outside/above BottomSheet content in the main vertical stack

## Sources

### Primary (HIGH confidence)
- Libadwaita 1.8.3 Documentation - AdwStatusPage class documentation
- Libadwaita 1.8.3 Documentation - AdwBanner class documentation (since 1.3)
- Existing codebase: operations/popover.go lines 264-280 (retry button implementation)
- Existing codebase: operations/operation.go line 77-78 (RetryFunc field)
- Existing codebase: pm/wrapper.go lines 47-60 (IsDryRun())

### Secondary (MEDIUM confidence)
- GNOME Human Interface Guidelines - Empty state patterns

### Tertiary (LOW confidence)
- None - all patterns verified with official documentation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Verified with official Libadwaita documentation
- Architecture: HIGH - Patterns derive from existing codebase + official widgets
- Pitfalls: HIGH - Based on verified widget behavior and existing code analysis

**Research date:** 2026-01-27
**Valid until:** 2026-02-27 (30 days - stable libadwaita)
