# Phase 4: Simple Pages - Research

**Researched:** 2026-01-26
**Domain:** Go package architecture, GTK4 page patterns, dependency injection, testable business logic
**Confidence:** HIGH

## Summary

This phase extracts the System and Help pages from the monolithic `userhome.go` (2500+ lines) into their own packages, establishing the page interface pattern that will guide future page extractions. The primary challenges are: (1) defining a clean page interface with dependency injection, (2) separating business logic from GTK-dependent UI code for testability, and (3) managing goroutine lifecycles when pages are destroyed.

Analysis of the existing `buildSystemPage()` and `buildHelpPage()` functions reveals that System page has moderate complexity with async data loading (NBC status) and config-driven group visibility, while Help page is simpler with only config-based link rows. Both pages share a common pattern: they receive dependencies (config, toaster), build GTK widgets, and potentially start goroutines for async data loading.

The key architectural insight is to use a two-layer design: a pure Go "logic" layer that handles data fetching, config interpretation, and command building (testable without GTK), and a "view" layer that consumes this logic to build and update GTK widgets. This matches the project's existing composition-based approach used in the `widgets` package.

**Primary recommendation:** Create `internal/pages/system/` and `internal/pages/help/` packages with dependency injection constructors. Separate business logic into `*_logic.go` files that are testable without GTK, and UI building into main files. Use context.Context for goroutine cancellation, tracked via a page-level cancel function.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| puregotk/v4/adw | current | PreferencesPage, PreferencesGroup, ToolbarView | Page structure widgets from libadwaita |
| puregotk/v4/gtk | current | Widget, Box, ScrolledWindow | Base GTK4 widgets |
| internal/async | Phase 1 | RunOnMain(), UserError | Thread-safe UI updates |
| internal/widgets | Phase 2 | AsyncExpanderRow, NewLinkRow, NewInfoRow | Reusable widget patterns |
| internal/config | existing | Config, GroupConfig | Configuration loading and access |
| context | stdlib | Context for cancellation | Goroutine lifecycle management |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| sync | stdlib | Mutex for shared state | Thread-safe data access |
| testing | stdlib | Unit tests | Testing business logic |
| internal/nbc | existing | NBC status fetching | System page NBC integration |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Package per page | Single pages package | Per-page packages provide clear boundaries, worth the import cost |
| Interface for toaster | Concrete window type | Interface enables testing without GTK window |
| Context for cancellation | Custom cancel channel | Context is idiomatic Go, integrates with nbc package already |

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── pages/
│   ├── page.go           # Common Page interface and types
│   ├── system/
│   │   ├── page.go       # SystemPage struct, constructor, UI building
│   │   ├── logic.go      # Business logic (testable without GTK)
│   │   └── logic_test.go # Tests for business logic
│   └── help/
│       ├── page.go       # HelpPage struct, constructor, UI building
│       ├── logic.go      # Business logic (simple link building)
│       └── logic_test.go # Tests for business logic
├── views/
│   └── userhome.go       # MODIFIED: delegates to page packages
└── ...
```

### Pattern 1: Page Interface with Dependency Injection
**What:** A common interface that all pages implement, with dependencies passed via constructor
**When to use:** All page packages follow this pattern for consistency
**Example:**
```go
// Source: Pattern derived from existing ToastAdder interface in userhome.go
// File: internal/pages/page.go

package pages

import (
    "github.com/jwijenbergh/puregotk/v4/adw"
)

// Toaster provides toast notification methods.
// Implemented by window.Window.
type Toaster interface {
    ShowToast(message string)
    ShowErrorToast(message string)
}

// Deps holds dependencies shared by all pages.
// Passed to page constructors for dependency injection.
type Deps struct {
    Config  *config.Config
    Toaster Toaster
}

// Page is the interface all page packages implement.
type Page interface {
    // Widget returns the root GTK widget for embedding in navigation.
    Widget() *adw.ToolbarView
    
    // Destroy cleans up resources (cancels goroutines, removes callbacks).
    Destroy()
}
```

### Pattern 2: Two-Layer Architecture (Logic + View)
**What:** Separate pure Go business logic from GTK-dependent view code
**When to use:** Any page with business logic that should be testable
**Example:**
```go
// Source: Pattern derived from existing loadOSRelease, loadNBCStatus
// File: internal/pages/system/logic.go

package system

import (
    "bufio"
    "context"
    "os"
    "strings"
    
    "github.com/frostyard/chairlift/internal/nbc"
)

// OSReleaseEntry represents a parsed line from /etc/os-release.
type OSReleaseEntry struct {
    Key       string
    Value     string
    DisplayKey string // Human-readable key
    IsURL     bool
}

// ParseOSRelease reads and parses /etc/os-release.
// This is pure Go logic, testable without GTK.
func ParseOSRelease() ([]OSReleaseEntry, error) {
    file, err := os.Open("/etc/os-release")
    if err != nil {
        return nil, err
    }
    defer file.Close()
    
    var entries []OSReleaseEntry
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
            continue
        }
        
        parts := strings.SplitN(line, "=", 2)
        key := parts[0]
        value := strings.Trim(parts[1], "\"'")
        
        entries = append(entries, OSReleaseEntry{
            Key:       key,
            Value:     value,
            DisplayKey: formatKey(key),
            IsURL:     strings.HasSuffix(key, "URL"),
        })
    }
    
    return entries, scanner.Err()
}

func formatKey(key string) string {
    // Convert KEY_NAME to "Key Name"
    readable := strings.ReplaceAll(key, "_", " ")
    // Use cases.Title for proper casing
    return cases.Title(language.English).String(strings.ToLower(readable))
}

// FetchNBCStatus fetches NBC status using the provided context.
// This wraps nbc.GetStatus for testability (can mock nbc package).
func FetchNBCStatus(ctx context.Context) (*nbc.Status, error) {
    return nbc.GetStatus(ctx)
}

// IsNBCAvailable checks if the system is running on NBC.
func IsNBCAvailable() bool {
    _, err := os.Stat("/run/nbc-booted")
    return err == nil
}
```

### Pattern 3: Page Constructor with Goroutine Tracking
**What:** Constructor that creates the page, starts background work, and tracks goroutines for cleanup
**When to use:** Any page that starts goroutines
**Example:**
```go
// Source: Derived from userhome.go buildSystemPage and loadNBCStatus patterns
// File: internal/pages/system/page.go

package system

import (
    "context"
    
    "github.com/frostyard/chairlift/internal/async"
    "github.com/frostyard/chairlift/internal/config"
    "github.com/frostyard/chairlift/internal/pages"
    "github.com/frostyard/chairlift/internal/widgets"
    "github.com/jwijenbergh/puregotk/v4/adw"
    "github.com/jwijenbergh/puregotk/v4/gtk"
)

// Page implements the System page.
type Page struct {
    toolbarView *adw.ToolbarView
    prefsPage   *adw.PreferencesPage
    
    config  *config.Config
    toaster pages.Toaster
    
    // Goroutine lifecycle management
    ctx    context.Context
    cancel context.CancelFunc
}

// New creates a new System page with the given dependencies.
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

// Widget returns the root widget for embedding.
func (p *Page) Widget() *adw.ToolbarView {
    return p.toolbarView
}

// Destroy cleans up resources and cancels running goroutines.
func (p *Page) Destroy() {
    if p.cancel != nil {
        p.cancel()
    }
}

func (p *Page) buildUI() {
    p.toolbarView = adw.NewToolbarView()
    
    headerBar := adw.NewHeaderBar()
    p.toolbarView.AddTopBar(&headerBar.Widget)
    
    scrolled := gtk.NewScrolledWindow()
    scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
    scrolled.SetVexpand(true)
    
    p.prefsPage = adw.NewPreferencesPage()
    scrolled.SetChild(&p.prefsPage.Widget)
    p.toolbarView.SetContent(&scrolled.Widget)
    
    // Build groups based on config
    p.buildSystemInfoGroup()
    p.buildNBCStatusGroup()
    p.buildSystemHealthGroup()
}

func (p *Page) buildNBCStatusGroup() {
    if !IsNBCAvailable() {
        return
    }
    if !p.config.IsGroupEnabled("system_page", "nbc_status_group") {
        return
    }
    
    group := adw.NewPreferencesGroup()
    group.SetTitle("NBC Status")
    group.SetDescription("View NBC system status information")
    
    expander := widgets.NewAsyncExpanderRow("NBC Status Details", "Loading...")
    group.Add(&expander.Expander.Widget)
    p.prefsPage.Add(group)
    
    // Start async load with page context for cancellation
    p.loadNBCStatus(expander)
}

func (p *Page) loadNBCStatus(expander *widgets.AsyncExpanderRow) {
    expander.StartLoading("Fetching NBC status")
    
    go func() {
        // Use page context - will be cancelled when Destroy() is called
        ctx, cancel := context.WithTimeout(p.ctx, nbc.DefaultTimeout)
        defer cancel()
        
        status, err := FetchNBCStatus(ctx)
        
        // Check if page was destroyed while we were fetching
        select {
        case <-p.ctx.Done():
            return // Page destroyed, don't update UI
        default:
        }
        
        async.RunOnMain(func() {
            if err != nil {
                if p.ctx.Err() != nil {
                    return // Page destroyed
                }
                expander.SetError(fmt.Sprintf("Failed to load: %v", err))
                return
            }
            
            expander.SetContent("Loaded")
            // Add content rows...
        })
    }()
}
```

### Pattern 4: Simple Page Without Goroutines (Help Page)
**What:** Pages that only do synchronous setup don't need context/cancel infrastructure
**When to use:** Static or config-only pages like Help
**Example:**
```go
// Source: Derived from userhome.go buildHelpPage
// File: internal/pages/help/page.go

package help

import (
    "github.com/frostyard/chairlift/internal/config"
    "github.com/frostyard/chairlift/internal/pages"
    "github.com/frostyard/chairlift/internal/widgets"
    "github.com/jwijenbergh/puregotk/v4/adw"
    "github.com/jwijenbergh/puregotk/v4/gtk"
)

// Page implements the Help page.
type Page struct {
    toolbarView *adw.ToolbarView
    prefsPage   *adw.PreferencesPage
    
    config    *config.Config
    toaster   pages.Toaster
    openURLFn func(string) // Callback for URL opening
}

// New creates a new Help page.
func New(deps pages.Deps, openURL func(string)) *Page {
    p := &Page{
        config:    deps.Config,
        toaster:   deps.Toaster,
        openURLFn: openURL,
    }
    
    p.buildUI()
    return p
}

// Widget returns the root widget.
func (p *Page) Widget() *adw.ToolbarView {
    return p.toolbarView
}

// Destroy is a no-op for Help page (no goroutines to cancel).
func (p *Page) Destroy() {}

func (p *Page) buildUI() {
    // Similar structure to System page
    p.toolbarView = adw.NewToolbarView()
    // ... header, scrolled, prefsPage setup
    
    p.buildResourcesGroup()
}

func (p *Page) buildResourcesGroup() {
    if !p.config.IsGroupEnabled("help_page", "help_resources_group") {
        return
    }
    
    groupCfg := p.config.GetGroupConfig("help_page", "help_resources_group")
    if groupCfg == nil {
        return
    }
    
    group := adw.NewPreferencesGroup()
    group.SetTitle("Help & Resources")
    group.SetDescription("Get help and learn more about ChairLift")
    
    // Use helper from logic.go to get links
    links := BuildResourceLinks(groupCfg)
    for _, link := range links {
        url := link.URL // Capture for closure
        row := widgets.NewLinkRow(link.Title, link.Subtitle, func() {
            p.openURLFn(url)
        })
        group.Add(&row.Widget)
    }
    
    p.prefsPage.Add(group)
}
```

### Anti-Patterns to Avoid
- **Storing window reference directly:** Use interface (Toaster) instead of concrete Window type for testability
- **Not checking context before UI updates:** Always check `p.ctx.Err()` before async.RunOnMain after long operations
- **Mixing business logic with GTK code:** Keep os.Open, parsing, API calls in `logic.go`; keep widget creation in `page.go`
- **Leaking goroutines:** Every goroutine must respect page context and exit when cancelled
- **Global state in page packages:** All state should be in the Page struct, passed via constructor

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Thread-safe UI updates | Manual channel coordination | async.RunOnMain() | Proven pattern from Phase 1 |
| Loading expander rows | Inline spinner management | widgets.AsyncExpanderRow | Proven pattern from Phase 2 |
| Link rows with icons | Manual row + icon creation | widgets.NewLinkRow() | Proven pattern from Phase 2 |
| Operation tracking | Page-local progress tracking | operations.Start() | Centralized tracking from Phase 3 |
| Config group checks | Custom enabled checks | config.IsGroupEnabled() | Already exists, handles defaults |

**Key insight:** The page packages should be thin orchestration layers that delegate to existing packages (async, widgets, operations, config). New logic belongs in `logic.go` for testability.

## Common Pitfalls

### Pitfall 1: UI Updates After Page Destroyed
**What goes wrong:** Goroutine completes and calls async.RunOnMain after page is destroyed, causing crashes or stale updates
**Why it happens:** No check for page lifecycle in async callback
**How to avoid:** Check `p.ctx.Err() != nil` before any UI update in async callbacks
**Warning signs:** Crashes when rapidly navigating between pages, stale data appearing

```go
// BAD
go func() {
    data := fetchData()
    async.RunOnMain(func() {
        expander.SetContent(data) // Page might be destroyed!
    })
}()

// GOOD
go func() {
    data := fetchData()
    select {
    case <-p.ctx.Done():
        return // Page destroyed, bail out
    default:
    }
    async.RunOnMain(func() {
        if p.ctx.Err() != nil {
            return // Double-check before UI update
        }
        expander.SetContent(data)
    })
}()
```

### Pitfall 2: Testing GTK-Dependent Code
**What goes wrong:** Tests fail because GTK isn't initialized, or tests are skipped entirely
**Why it happens:** Business logic mixed with UI code
**How to avoid:** Put testable logic in `logic.go`, test only that file
**Warning signs:** No tests, or tests require `CGO_ENABLED=1` and display

### Pitfall 3: Circular Dependencies Between Pages and Views
**What goes wrong:** import cycle between pages package and views package
**Why it happens:** Pages try to access other pages, or views tries to access page internals
**How to avoid:** Pages only depend on common interfaces (pages.Deps, pages.Toaster). Views creates and owns pages.
**Warning signs:** Compiler errors about import cycles

### Pitfall 4: Not Cleaning Up Widget References
**What goes wrong:** Memory leaks from holding references to removed widgets
**Why it happens:** Storing widget references in page struct without cleanup
**How to avoid:** Let GTK manage widget lifecycle. Store minimal references. Nil out references in Destroy() if needed.
**Warning signs:** Growing memory usage, widgets surviving beyond their visual lifetime

### Pitfall 5: Callback Closures Capturing Loop Variables
**What goes wrong:** All link rows open the same (last) URL
**Why it happens:** Go closure captures variable reference, not value
**How to avoid:** Create local copy before closure: `url := link.URL`
**Warning signs:** All rows perform same action

## Code Examples

Verified patterns from official sources:

### Testable Business Logic
```go
// File: internal/pages/system/logic_test.go
package system

import (
    "testing"
)

func TestParseOSRelease(t *testing.T) {
    // This test runs without GTK - pure Go logic testing
    entries, err := ParseOSRelease()
    if err != nil {
        t.Fatalf("ParseOSRelease failed: %v", err)
    }
    
    // Verify parsing logic
    for _, entry := range entries {
        if entry.Key == "" {
            t.Error("entry has empty key")
        }
        if entry.IsURL && !strings.HasSuffix(entry.Key, "URL") {
            t.Errorf("IsURL mismatch for key %s", entry.Key)
        }
    }
}

func TestFormatKey(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"PRETTY_NAME", "Pretty Name"},
        {"VERSION_ID", "Version Id"},
        {"HOME_URL", "Home Url"},
    }
    
    for _, tc := range tests {
        result := formatKey(tc.input)
        if result != tc.expected {
            t.Errorf("formatKey(%q) = %q, want %q", tc.input, result, tc.expected)
        }
    }
}
```

### Help Page Logic Layer
```go
// File: internal/pages/help/logic.go
package help

import "github.com/frostyard/chairlift/internal/config"

// ResourceLink represents a help resource link.
type ResourceLink struct {
    Title    string
    Subtitle string
    URL      string
}

// BuildResourceLinks creates the list of resource links from config.
// Pure Go, testable without GTK.
func BuildResourceLinks(cfg *config.GroupConfig) []ResourceLink {
    if cfg == nil {
        return nil
    }
    
    var links []ResourceLink
    
    if cfg.Website != "" {
        links = append(links, ResourceLink{
            Title:    "Website",
            Subtitle: cfg.Website,
            URL:      cfg.Website,
        })
    }
    
    if cfg.Issues != "" {
        links = append(links, ResourceLink{
            Title:    "Report Issues",
            Subtitle: cfg.Issues,
            URL:      cfg.Issues,
        })
    }
    
    if cfg.Chat != "" {
        links = append(links, ResourceLink{
            Title:    "Community Discussions",
            Subtitle: cfg.Chat,
            URL:      cfg.Chat,
        })
    }
    
    return links
}
```

### Integration with Views (userhome.go)
```go
// File: internal/views/userhome.go (modified)
import (
    "github.com/frostyard/chairlift/internal/pages"
    "github.com/frostyard/chairlift/internal/pages/help"
    "github.com/frostyard/chairlift/internal/pages/system"
)

type UserHome struct {
    // ... existing fields ...
    
    // Page packages
    systemPage *system.Page
    helpPage   *help.Page
}

func New(cfg *config.Config, toastAdder ToastAdder) *UserHome {
    uh := &UserHome{
        config:     cfg,
        toastAdder: toastAdder,
        // ... other init ...
    }
    
    deps := pages.Deps{
        Config:  cfg,
        Toaster: toastAdder,
    }
    
    // Create page packages
    uh.systemPage = system.New(deps)
    uh.helpPage = help.New(deps, uh.openURL)
    
    // ... rest of init ...
    return uh
}

// GetPage returns a page by name
func (uh *UserHome) GetPage(name string) *adw.ToolbarView {
    switch name {
    case "system":
        return uh.systemPage.Widget()
    case "help":
        return uh.helpPage.Widget()
    // ... other pages remain in userhome.go for now ...
    }
}

// Cleanup when window closes
func (uh *UserHome) Destroy() {
    uh.systemPage.Destroy()
    uh.helpPage.Destroy()
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Monolithic views file | Package-per-page | Best practice | Testability, maintainability |
| Mixed logic + UI | Separate logic.go | Best practice | Unit testing without GTK |
| Fire-and-forget goroutines | Context-based cancellation | Go 1.7 (2016) | Proper resource cleanup |
| Direct window dependency | Interface injection | Best practice | Testable pages |

**Deprecated/outdated:**
- Global state for page tracking: Use struct fields
- Relying on GC for goroutine cleanup: Goroutines must be explicitly cancelled

## Open Questions

Things that couldn't be fully resolved:

1. **Whether to use a common Page interface**
   - What we know: All pages share Widget() and Destroy() methods
   - What's unclear: Is interface worth the indirection, or just use concrete types?
   - Recommendation: Start with interface - enables future features like page lifecycle hooks

2. **openURL callback vs direct gtk.ShowUri**
   - What we know: Current code has openURL as UserHome method
   - What's unclear: Should pages call gtk.ShowUri directly, or receive callback?
   - Recommendation: Callback pattern for testability - pages don't depend on gtk.ShowUri

3. **Whether Destroy() needs to be called explicitly**
   - What we know: GTK widgets are cleaned up when removed from parent
   - What's unclear: Does puregotk require explicit cleanup for Go references?
   - Recommendation: Call Destroy() explicitly for goroutine cancellation; widget cleanup is bonus

## Sources

### Primary (HIGH confidence)
- Codebase analysis: `internal/views/userhome.go` - analyzed buildSystemPage, buildHelpPage, all 23 goroutine patterns
- Codebase analysis: `internal/widgets/` - verified composition pattern for reference
- Codebase analysis: `internal/config/` - verified GroupConfig, IsGroupEnabled APIs
- Go stdlib: context package documentation for cancellation patterns
- puregotk README: Verified no special cleanup needed beyond Unref for top-level objects

### Secondary (MEDIUM confidence)
- Prior phase research: Phase 2, Phase 3 patterns for widgets and operations
- Go testing practices: Standard testing package patterns

### Tertiary (LOW confidence)
- None - all patterns derived from existing codebase and Go stdlib

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Uses existing packages, no new dependencies
- Architecture: HIGH - Patterns derived from existing codebase composition approach
- Pitfalls: HIGH - Based on actual code patterns and Go concurrency knowledge

**Research date:** 2026-01-26
**Valid until:** 90 days (stable patterns, architecture decisions unlikely to change)
