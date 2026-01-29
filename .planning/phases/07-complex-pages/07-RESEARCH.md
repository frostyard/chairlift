# Phase 7: Complex Pages - Research

**Researched:** 2026-01-28
**Domain:** GTK4/Libadwaita page extraction, multi-PM application management, sidebar navigation
**Confidence:** HIGH

## Summary

This phase extracts the Applications and Updates pages from userhome.go (~1800 lines) into their own packages, following the established page pattern from Phases 4-6. The research investigated sidebar navigation patterns for the Applications page, empty state handling for the Updates page, and the existing codebase patterns for page extraction.

The codebase already has a well-established page architecture:
- Pages implement the `pages.Page` interface (Widget(), Destroy())
- Dependencies injected via `pages.Deps` struct
- Logic/UI separation in `logic.go` (testable) and `page.go` (GTK-dependent)
- Context-based lifecycle for goroutine cancellation
- Operations system from Phase 3 for progress tracking

**Primary recommendation:** Follow the existing page extraction pattern exactly. Use `AdwNavigationSplitView` for the Applications page sidebar (already used at window level). Reuse existing widget helpers and empty state patterns.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| puregotk/v4/adw | current | Libadwaita bindings | Already in use, provides AdwNavigationSplitView, AdwPreferencesPage |
| puregotk/v4/gtk | current | GTK4 bindings | Already in use, provides SearchEntry, ListBox |
| frostyard/pm | 0.2.1 | Package manager abstraction | Already wrapped in internal/pm for Flatpak, Homebrew, Snap |
| frostyard/nbc | 0.14.0 | NBC system updates | Already wrapped in internal/nbc for system updates |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| internal/operations | - | Operation tracking | All async PM operations for progress popover |
| internal/widgets | - | Reusable widgets | EmptyState, AsyncExpanderRow, button rows |
| internal/pages | - | Page interface/deps | All page packages |
| internal/async | - | Main thread marshaling | All goroutine UI updates |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| AdwNavigationSplitView | AdwOverlaySplitView | OverlaySplitView overlays sidebar; NavigationSplitView is better for persistent sidebar lists |
| PreferencesPage | Custom GtkBox | PreferencesPage provides consistent styling with other pages |

**Installation:** No new dependencies required - all libraries already in go.mod.

## Architecture Patterns

### Recommended Project Structure
```
internal/pages/
├── page.go              # Page interface, Deps struct (existing)
├── applications/
│   ├── logic.go         # PM data fetching, search, data types (no GTK)
│   ├── logic_test.go    # Unit tests for logic
│   └── page.go          # UI construction, event handlers
└── updates/
    ├── logic.go         # Update checking, data types (no GTK)
    ├── logic_test.go    # Unit tests for logic
    └── page.go          # UI construction, event handlers
```

### Pattern 1: Page Extraction Pattern (from existing pages)
**What:** Extract page from userhome.go into its own package following established pattern.
**When to use:** Every page extraction.
**Example:**
```go
// Source: internal/pages/system/page.go (existing pattern)
package applications

type Page struct {
    toolbarView *adw.ToolbarView
    prefsPage   *adw.PreferencesPage

    config  *config.Config
    toaster pages.Toaster

    // Goroutine lifecycle management
    ctx    context.Context
    cancel context.CancelFunc
}

func New(deps pages.Deps) *Page {
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

func (p *Page) Widget() *adw.ToolbarView { return p.toolbarView }
func (p *Page) Destroy() { if p.cancel != nil { p.cancel() } }
```

### Pattern 2: Nested NavigationSplitView for Applications Sidebar
**What:** Applications page uses its own NavigationSplitView for PM sidebar navigation.
**When to use:** Applications page only (per CONTEXT.md decision).
**Example:**
```go
// Source: Libadwaita AdwNavigationSplitView documentation
func (p *Page) buildUI() {
    p.toolbarView = adw.NewToolbarView()

    // Create split view for sidebar navigation
    p.splitView = adw.NewNavigationSplitView()
    p.splitView.SetSidebarWidthFraction(0.25)
    p.splitView.SetMinSidebarWidth(180)
    p.splitView.SetMaxSidebarWidth(280)

    // Sidebar with PM list
    sidebarPage := p.buildSidebar()
    p.splitView.SetSidebar(sidebarPage)

    // Content area with package list
    contentPage := p.buildContent()
    p.splitView.SetContent(contentPage)

    p.toolbarView.SetContent(&p.splitView.Widget)
}
```

### Pattern 3: Empty State Using AdwStatusPage
**What:** Show empty state when no updates available.
**When to use:** Updates page no-updates state, empty PM sections.
**Example:**
```go
// Source: internal/widgets/empty_state.go (existing pattern)
if len(updates) == 0 {
    emptyState := widgets.NewEmptyState(widgets.EmptyStateConfig{
        Title:       "No Updates Available",
        Description: "All packages are up to date",
        IconName:    "software-update-available-symbolic",
        Compact:     false,
    })
    group.Add(&emptyState.Widget)
}
```

### Pattern 4: Search with SearchEntry + Debounced search-changed
**What:** Unified search across all PMs with debounced input.
**When to use:** Applications page search.
**Example:**
```go
// Source: GTK4 SearchEntry documentation
p.searchEntry = gtk.NewSearchEntry()
p.searchEntry.SetHexpand(true)

// search-changed signal has built-in delay for reactive filtering
searchChangedCb := func(entry gtk.SearchEntry) {
    query := entry.GetText()
    if query == "" {
        p.clearSearchResults()
        return
    }
    go p.performSearch(query)
}
p.searchEntry.ConnectSearchChanged(&searchChangedCb)
```

### Pattern 5: Reboot Prompt Dialog (for Updates)
**What:** Show reboot prompt after NBC update completes.
**When to use:** After successful NBC update.
**Example:**
```go
// Source: internal/operations/dialogs.go (AdwAlertDialog pattern)
func ShowRebootPrompt(window *gtk.Window) {
    dialog := adw.NewAlertDialog("System Update Complete", "")
    dialog.SetBody("A reboot is required to apply the system update.")

    dialog.AddResponse("later", "_Later")
    dialog.AddResponse("reboot", "_Reboot Now")

    dialog.SetResponseAppearance("reboot", adw.ResponseSuggestedValue)
    dialog.SetDefaultResponse("later")
    dialog.SetCloseResponse("later")

    responseCb := func(_ adw.AlertDialog, response string) {
        if response == "reboot" {
            // Initiate reboot via nbc or systemctl
        }
    }
    dialog.ConnectResponse(&responseCb)
    dialog.Present(&window.Widget)
}
```

### Anti-Patterns to Avoid
- **Monolithic page file:** userhome.go is 1800+ lines - extract to separate packages
- **Direct pm package calls in UI code:** Use logic.go layer for testability
- **Blocking PM operations on main thread:** Always use goroutines with async.RunOnMain
- **Ignoring context cancellation:** Always check p.ctx.Done() before UI updates from goroutines
- **Hardcoded PM availability checks:** Use existing pm.FlatpakIsInstalled(), pm.HomebrewIsInstalled(), pm.SnapIsInstalled()

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Empty state display | Custom label/box | widgets.NewEmptyState | HIG-compliant StatusPage, consistent styling |
| Async expander loading | Manual spinner management | widgets.AsyncExpanderRow | Handles loading/error states consistently |
| Button rows | Manual ActionRow+Button | widgets.NewButtonRow | Consistent styling, click handling |
| Icon rows | Manual ActionRow+Image | widgets.NewIconRow | Consistent prefix icon placement |
| Operation tracking | Manual progress state | operations.Start/Complete | Unified operations popover integration |
| Main thread updates | Custom glib.IdleAdd | async.RunOnMain | Consistent thread safety |
| Confirmation dialogs | Custom window | operations.ShowCancelConfirmation or AdwAlertDialog | HIG-compliant response handling |
| PM availability | Custom exec.LookPath | pm.FlatpakIsInstalled() etc | Cached availability, thread-safe |
| PM operations | Direct exec.Command | internal/pm wrapper functions | Dry-run support, error handling |

**Key insight:** The codebase has a rich widget library and established patterns. Every common UI need already has a helper. Check internal/widgets/ and internal/operations/ before building custom solutions.

## Common Pitfalls

### Pitfall 1: Goroutine UI Updates Without Context Check
**What goes wrong:** Goroutine updates UI after page is destroyed, causing crashes or visual artifacts.
**Why it happens:** Long-running PM operations complete after user navigates away.
**How to avoid:** Always check context before UI update:
```go
go func() {
    result, err := fetchData()

    select {
    case <-p.ctx.Done():
        return // Page destroyed
    default:
    }

    async.RunOnMain(func() {
        if p.ctx.Err() != nil {
            return // Double-check
        }
        // Safe to update UI
    })
}()
```
**Warning signs:** UI updates appearing after navigation, nil pointer panics in async callbacks.

### Pitfall 2: Callback Closure Variable Capture
**What goes wrong:** All buttons in a loop reference the same variable.
**Why it happens:** Go closures capture by reference, not value.
**How to avoid:** Create local copy before closure:
```go
for _, pkg := range packages {
    pkg := pkg // CRITICAL: capture in local variable

    btn := gtk.NewButtonWithLabel("Install")
    clickedCb := func(_ gtk.Button) {
        p.installPackage(pkg.Name) // Uses captured pkg
    }
    btn.ConnectClicked(&clickedCb)
}
```
**Warning signs:** All buttons perform action on last item in list.

### Pitfall 3: Missing Operations Integration
**What goes wrong:** PM operations don't appear in operations popover, no retry capability.
**Why it happens:** Forgetting to wrap operations with operations.Start/Complete.
**How to avoid:** All long-running operations should use:
```go
op := operations.Start("Install Firefox", operations.CategoryInstall, false)
op.RetryFunc = func() { p.onInstallClicked(btn, pkgName) }

go func() {
    err := pm.FlatpakInstall(appID, true)
    async.RunOnMain(func() {
        op.Complete(err)
        // Update UI
    })
}()
```
**Warning signs:** Operations don't show in header popover, no history of operations.

### Pitfall 4: Forgetting Config-Driven Groups
**What goes wrong:** Groups always show even when disabled in config.
**Why it happens:** Skipping the config check pattern.
**How to avoid:** Always check config before building groups:
```go
func (p *Page) buildFlatpakGroup() {
    if !p.config.IsGroupEnabled("applications_page", "flatpak_group") {
        return
    }
    if !pm.FlatpakIsInstalled() {
        return // Also check availability
    }
    // Build group...
}
```
**Warning signs:** Groups appear when they should be hidden per config.yml.

### Pitfall 5: Blocking Search Operations
**What goes wrong:** UI freezes during search.
**Why it happens:** Search performed on main thread.
**How to avoid:** Search in goroutine, show loading state:
```go
func (p *Page) onSearchChanged() {
    query := p.searchEntry.GetText()
    p.showSearchLoading()

    go func() {
        results, err := p.client.Search(p.ctx, query)

        async.RunOnMain(func() {
            if err != nil {
                p.showSearchError(err)
                return
            }
            p.displaySearchResults(results)
        })
    }()
}
```
**Warning signs:** UI unresponsive during typing in search box.

## Code Examples

Verified patterns from official sources and existing codebase:

### Applications Page Sidebar Navigation
```go
// Source: Existing window.go pattern + Libadwaita docs
func (p *Page) buildSidebar() *adw.NavigationPage {
    toolbarView := adw.NewToolbarView()

    headerBar := adw.NewHeaderBar()
    headerBar.SetShowEndTitleButtons(false)
    toolbarView.AddTopBar(&headerBar.Widget)

    scrolled := gtk.NewScrolledWindow()
    scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
    scrolled.SetVexpand(true)

    p.sidebarList = gtk.NewListBox()
    p.sidebarList.SetSelectionMode(gtk.SelectionSingleValue)
    p.sidebarList.AddCssClass("navigation-sidebar")

    // Add PM entries
    p.addSidebarRow("All Applications", "package-x-generic-symbolic")
    if pm.FlatpakIsInstalled() {
        p.addSidebarRow("Flatpak", "application-x-flatpak-symbolic")
    }
    if pm.HomebrewIsInstalled() {
        p.addSidebarRow("Homebrew", "package-x-generic-symbolic")
    }
    if pm.SnapIsInstalled() {
        p.addSidebarRow("Snap", "package-x-generic-symbolic")
    }

    rowActivatedCb := func(listbox gtk.ListBox, rowPtr uintptr) {
        row := gtk.ListBoxRowNewFromInternalPtr(rowPtr)
        p.onSidebarRowActivated(*row)
    }
    p.sidebarList.ConnectRowActivated(&rowActivatedCb)

    scrolled.SetChild(&p.sidebarList.Widget)
    toolbarView.SetContent(&scrolled.Widget)

    return adw.NewNavigationPage(&toolbarView.Widget, "Package Managers")
}
```

### Updates Page Section Pattern
```go
// Source: Existing userhome.go pattern
func (p *Page) buildNBCUpdatesGroup() {
    if !nbc.IsNBCBooted() {
        return
    }
    if !p.config.IsGroupEnabled("updates_page", "nbc_updates_group") {
        return
    }

    group := adw.NewPreferencesGroup()
    group.SetTitle("System Updates")
    group.SetDescription("Check for NBC system updates")

    // Check row
    p.nbcCheckRow = adw.NewActionRow()
    p.nbcCheckRow.SetTitle("Check for Updates")
    p.nbcCheckRow.SetSubtitle("Checking...")

    checkBtn := gtk.NewButtonWithLabel("Check")
    checkBtn.SetValign(gtk.AlignCenterValue)
    clickedCb := func(_ gtk.Button) {
        p.onCheckNBCUpdates()
    }
    checkBtn.ConnectClicked(&clickedCb)
    p.nbcCheckRow.AddSuffix(&checkBtn.Widget)
    group.Add(&p.nbcCheckRow.Widget)

    // Update row (shown when update available)
    p.nbcUpdateRow = adw.NewActionRow()
    p.nbcUpdateRow.SetVisible(false) // Hidden until update found
    // ...

    p.prefsPage.Add(group)

    // Auto-check on page load
    go p.checkNBCUpdateAvailability()
}
```

### Unified Search Implementation
```go
// Source: GTK4 SearchEntry docs + existing homebrew search pattern
type SearchResult struct {
    Name        string
    Description string
    PM          string // "flatpak", "homebrew", "snap"
    IsInstalled bool
}

func (l *Logic) Search(ctx context.Context, query string) ([]SearchResult, error) {
    var results []SearchResult
    var mu sync.Mutex
    var wg sync.WaitGroup

    // Search all available PMs in parallel
    if pm.FlatpakIsInstalled() {
        wg.Add(1)
        go func() {
            defer wg.Done()
            flatpakResults, _ := l.searchFlatpak(ctx, query)
            mu.Lock()
            results = append(results, flatpakResults...)
            mu.Unlock()
        }()
    }

    if pm.HomebrewIsInstalled() {
        wg.Add(1)
        go func() {
            defer wg.Done()
            brewResults, _ := l.searchHomebrew(ctx, query)
            mu.Lock()
            results = append(results, brewResults...)
            mu.Unlock()
        }()
    }

    // Snap search if available...

    wg.Wait()
    return results, nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| GtkMessageDialog | AdwAlertDialog | Libadwaita 1.5 | Use for reboot prompt |
| Manual progress dialogs | Inline progress + operations popover | Phase 3 | No modal progress dialogs |
| Blocking sync PM calls | Goroutine + async.RunOnMain | Established | Non-blocking UI |
| Hardcoded PM sections | Config-driven groups | Established | Flexible deployment |

**Deprecated/outdated:**
- Modal progress windows: Anti-pattern per GNOME HIG; use inline progress
- GtkMessageDialog: Use AdwAlertDialog instead

## Open Questions

Things that couldn't be fully resolved:

1. **Snap search API availability**
   - What we know: pm.SnapInstall and ListInstalledSnaps exist
   - What's unclear: Whether pm library supports snap search (snap find)
   - Recommendation: Check pm library for Searcher interface support for Snap; if not available, show Snap section without search or link to Snap Store

2. **Flatpak update detection granularity**
   - What we know: ListFlatpakUpdates exists but returns empty (TODO in wrapper)
   - What's unclear: Whether pm library fully implements update detection
   - Recommendation: Check pm library's current implementation; may need direct flatpak command as fallback

3. **Applications page "All Apps" view**
   - What we know: CONTEXT.md marks as Claude's discretion
   - What's unclear: Whether to show combined list or just search results
   - Recommendation: If search is unified, "All Apps" sidebar item can trigger search mode with empty query showing installed apps

## Sources

### Primary (HIGH confidence)
- Existing codebase: internal/pages/system/page.go, maintenance/page.go, extensions/page.go - established page extraction pattern
- Existing codebase: internal/widgets/empty_state.go - EmptyState pattern
- Existing codebase: internal/window/window.go - NavigationSplitView usage
- Existing codebase: internal/pm/wrapper.go - PM wrapper pattern
- [Libadwaita AdwNavigationSplitView](https://gnome.pages.gitlab.gnome.org/libadwaita/doc/main/class.NavigationSplitView.html) - Official API docs

### Secondary (MEDIUM confidence)
- [GNOME HIG Sidebars](https://developer.gnome.org/hig/patterns/nav/sidebars.html) - Design guidelines
- [GTK4 SearchEntry](https://docs.gtk.org/gtk4/class.SearchEntry.html) - Search input pattern

### Tertiary (LOW confidence)
- WebSearch results for "puregotk NavigationSplitView" - No specific examples found; rely on C API translation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All libraries already in use, patterns established
- Architecture: HIGH - Existing page pattern is well-documented and consistent
- Pitfalls: HIGH - Observed directly in codebase (context checks, closure capture)
- Sidebar navigation: MEDIUM - AdwNavigationSplitView API known, but nested usage needs testing
- Search API: MEDIUM - SearchEntry pattern clear, but PM search capabilities vary

**Research date:** 2026-01-28
**Valid until:** 60 days (stable codebase patterns)
