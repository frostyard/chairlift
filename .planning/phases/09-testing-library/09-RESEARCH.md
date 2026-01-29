# Phase 9: Testing & Library - Research

**Researched:** 2026-01-28
**Domain:** Go testing patterns, GTK4 integration testing, library extraction
**Confidence:** HIGH

## Summary

This phase adds test coverage to ChairLift's business logic and extracts reusable GTK4/Go patterns into the `/pkg/adwutil/` library. Research confirms the codebase already follows testable patterns: logic layers are separated from UI code with no GTK dependencies (help, system, maintenance, extensions, updates pages all have `logic.go` files), making unit testing straightforward using standard Go testing.

The existing test files demonstrate the established pattern: table-driven tests for pure functions, simple assertions without external frameworks. GTK integration tests require display access but can be skipped gracefully in headless CI using environment variable checks. Library extraction follows Go conventions with `/pkg/` for public APIs.

**Primary recommendation:** Use Go's standard `testing` package with table-driven tests for logic layers; skip GTK integration tests when `DISPLAY`/`WAYLAND_DISPLAY` are unset; extract to `/pkg/adwutil/` with godoc comments and working example apps.

## Standard Stack

The established tools for Go testing and library development:

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `testing` | stdlib | Test framework | Go standard, no dependencies, excellent tooling |
| `go test` | stdlib | Test runner | Built-in coverage, parallel execution, benchmarks |
| `testdata/` | convention | Test fixtures | Go tool ignores during build, standard placement |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `testing/quick` | stdlib | Property-based testing | For functions with wide input domains |
| `io/fs.FS` | stdlib | Mock filesystem | For config loading tests |
| `context` | stdlib | Timeout/cancellation | For testing async operations |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `testing` | testify | More assertions but adds dependency |
| `testing` | gomock | Auto-generates mocks but adds complexity |
| Manual mocks | mockery | Less boilerplate but requires code generation |

**Decision:** Use standard library only. The existing tests use plain `testing` package successfully. Interface mocking is simple enough with manual implementations given the codebase size.

## Architecture Patterns

### Recommended Test Structure

```
internal/
├── config/
│   ├── config.go
│   └── config_test.go        # Unit tests for parsing
├── pages/
│   └── help/
│       ├── logic.go           # Pure Go, no GTK
│       ├── logic_test.go      # Unit tests (existing)
│       ├── page.go            # GTK UI code
│       └── page_test.go       # Integration tests (skip if no display)
└── pm/
    ├── wrapper.go
    └── wrapper_test.go        # Tests with mock managers
pkg/
└── adwutil/
    ├── doc.go                 # Package documentation
    ├── async.go               # RunOnMain, callback registry
    ├── errors.go              # UserError type
    ├── widgets.go             # Row helpers, empty state
    ├── operations.go          # Operation tracking
    ├── examples/              # Runnable example apps
    │   └── basic/main.go
    └── README.md              # Usage documentation
```

### Pattern 1: Table-Driven Tests

**What:** Define test cases as data structures, iterate with subtests
**When to use:** Testing functions with multiple input/output combinations

```go
// Source: Go Wiki TableDrivenTests (https://go.dev/wiki/TableDrivenTests)
func TestBuildResourceLinks(t *testing.T) {
    tests := map[string]struct {
        cfg  *config.GroupConfig
        want int
    }{
        "nil config":      {cfg: nil, want: 0},
        "empty config":    {cfg: &config.GroupConfig{}, want: 0},
        "website only":    {cfg: &config.GroupConfig{Website: "https://example.com"}, want: 1},
        "all fields":      {cfg: &config.GroupConfig{Website: "a", Issues: "b", Chat: "c"}, want: 3},
    }

    for name, tc := range tests {
        t.Run(name, func(t *testing.T) {
            got := BuildResourceLinks(tc.cfg)
            if len(got) != tc.want {
                t.Errorf("got %d links, want %d", len(got), tc.want)
            }
        })
    }
}
```

### Pattern 2: Skip GTK Tests Without Display

**What:** Gracefully skip integration tests in headless CI
**When to use:** Any test that initializes GTK widgets

```go
// Source: GTK4 docs (https://docs.gtk.org/gtk4/running.html)
func TestPageConstruction(t *testing.T) {
    // Skip if no display available (headless CI)
    if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
        t.Skip("Skipping GTK test: no display available")
    }

    // Initialize GTK for testing
    gtk.Init()

    page := NewHelpPage(testDeps())
    if page == nil {
        t.Fatal("NewHelpPage returned nil")
    }

    // Verify widget structure exists
    widget := page.Widget()
    if widget == nil {
        t.Error("Widget() returned nil")
    }
}
```

### Pattern 3: Dependency Injection for Testability

**What:** Inject interfaces rather than concrete types
**When to use:** Testing code that depends on external services (pm, nbc, updex)

```go
// Source: Learn Go with Tests (https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/dependency-injection)

// Define interface for what page needs
type PMClient interface {
    FlatpakIsInstalled() bool
    ListFlatpakApplications() ([]FlatpakApplication, error)
}

// Mock implementation for tests
type mockPMClient struct {
    flatpakInstalled bool
    apps             []FlatpakApplication
    err              error
}

func (m *mockPMClient) FlatpakIsInstalled() bool {
    return m.flatpakInstalled
}

func (m *mockPMClient) ListFlatpakApplications() ([]FlatpakApplication, error) {
    return m.apps, m.err
}

func TestCountFlatpakUpdates(t *testing.T) {
    mock := &mockPMClient{
        flatpakInstalled: true,
        apps: []FlatpakApplication{
            {ID: "org.example.App1"},
            {ID: "org.example.App2"},
        },
    }

    count := CountFlatpakUpdatesWithClient(mock)
    if count != 2 {
        t.Errorf("got %d, want 2", count)
    }
}
```

### Pattern 4: Test Fixtures in testdata/

**What:** Store test input files in `testdata/` directory
**When to use:** Testing config parsing, file loading

```go
// Source: Dave Cheney (https://dave.cheney.net/2016/05/10/test-fixtures-in-go)
func TestLoadConfig(t *testing.T) {
    // testdata/ is ignored by go build, included in go test
    cfg, err := LoadFromPath("testdata/valid_config.yml")
    if err != nil {
        t.Fatalf("LoadFromPath failed: %v", err)
    }

    if !cfg.SystemPage["system_info_group"].Enabled {
        t.Error("expected system_info_group to be enabled")
    }
}
```

### Anti-Patterns to Avoid

- **Testing GTK internals:** Don't test widget appearance or layout; test that widgets exist and callbacks don't panic
- **Mocking too much:** Test real logic, only mock external dependencies (filesystem, network, pm library)
- **Ignoring test failures:** If tests fail in CI due to missing display, skip gracefully rather than removing tests
- **Testing private functions:** Test through public API; refactor if private functions need direct testing

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Test assertions | Custom assertion library | `t.Errorf` with descriptive messages | Standard, clear, no dependencies |
| Mock generation | Manual mock boilerplate | Interface + manual mock struct | Simple, explicit, easy to understand |
| Coverage reports | Custom coverage tool | `go test -cover` / `-coverprofile` | Built-in, integrates with CI |
| Test fixtures | Complex fixture framework | `testdata/` directory | Go convention, zero dependencies |
| Parallel tests | Custom parallelization | `t.Parallel()` | Built-in, handles race detection |

**Key insight:** Go's testing philosophy values simplicity over abstraction. The standard library provides everything needed for effective testing.

## Common Pitfalls

### Pitfall 1: GTK Tests Failing in CI

**What goes wrong:** Tests that create widgets fail with "cannot open display" errors
**Why it happens:** CI environments are headless (no X11/Wayland)
**How to avoid:** Check `DISPLAY`/`WAYLAND_DISPLAY` environment variables and `t.Skip()` if absent
**Warning signs:** Tests pass locally but fail in GitHub Actions / GitLab CI

### Pitfall 2: Race Conditions in Async Tests

**What goes wrong:** Tests intermittently fail with race detector enabled
**Why it happens:** GTK callbacks run on main thread, tests may check state before callback executes
**How to avoid:** Use channels or sync primitives to coordinate; run tests with `-race` flag
**Warning signs:** Flaky tests, different results on different machines

### Pitfall 3: Test Pollution from Globals

**What goes wrong:** Tests affect each other through shared global state
**Why it happens:** Packages like `operations` use singleton registries; `pm` has global managers
**How to avoid:** Reset global state in test setup/teardown; use dependency injection for new code
**Warning signs:** Tests pass individually but fail when run together

### Pitfall 4: Over-Mocking Pure Functions

**What goes wrong:** Tests mock everything, don't actually test the code
**Why it happens:** Misunderstanding of when mocking is appropriate
**How to avoid:** Only mock external dependencies (I/O, network, system calls); test logic directly
**Warning signs:** Tests pass but bugs slip through; 100% coverage with no real assertions

### Pitfall 5: Library Coupling to Application

**What goes wrong:** Extracted library has imports back to chairlift packages
**Why it happens:** Incomplete extraction, dependencies not fully identified
**How to avoid:** Library in `/pkg/adwutil/` should only import stdlib, puregotk, yaml; no chairlift internal imports
**Warning signs:** Circular import errors; library not usable in other projects

## Code Examples

Verified patterns from existing codebase and Go standards:

### Example: Testing Config Parsing with Fixtures

```go
// internal/config/config_test.go
func TestLoadFromPath_ValidConfig(t *testing.T) {
    cfg, err := loadFromPath("testdata/valid.yml")
    if err != nil {
        t.Fatalf("loadFromPath failed: %v", err)
    }

    // Verify expected structure
    if !cfg.SystemPage["system_info_group"].Enabled {
        t.Error("system_info_group should be enabled")
    }
}

func TestLoadFromPath_InvalidYAML(t *testing.T) {
    _, err := loadFromPath("testdata/invalid.yml")
    if err == nil {
        t.Error("expected error for invalid YAML, got nil")
    }
}
```

### Example: Testing UserError Formatting

```go
// pkg/adwutil/errors_test.go (or internal/async/errors_test.go)
func TestUserError_FormatForUser(t *testing.T) {
    tests := []struct {
        name    string
        err     *UserError
        want    string
    }{
        {
            name: "summary only",
            err:  NewUserError("Couldn't install Firefox", nil),
            want: "Couldn't install Firefox",
        },
        {
            name: "with hint",
            err:  NewUserErrorWithHint("Couldn't connect", "Check your internet", nil),
            want: "Couldn't connect: Check your internet",
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got := tc.err.FormatForUser()
            if got != tc.want {
                t.Errorf("FormatForUser() = %q, want %q", got, tc.want)
            }
        })
    }
}
```

### Example: Testing Operations Registry

```go
// pkg/adwutil/operations_test.go
func TestRegistry_StartAndComplete(t *testing.T) {
    // Create isolated registry for test (not the default singleton)
    r := &Registry{operations: make(map[uint64]*Operation)}

    op := r.start("Test Op", CategoryInstall, false, nil)

    if op.State != StateActive {
        t.Errorf("initial state = %v, want StateActive", op.State)
    }

    if r.activeCount() != 1 {
        t.Errorf("activeCount = %d, want 1", r.activeCount())
    }

    r.complete(op.ID, nil)

    if r.activeCount() != 0 {
        t.Errorf("after complete, activeCount = %d, want 0", r.activeCount())
    }

    if len(r.history) != 1 {
        t.Errorf("history length = %d, want 1", len(r.history))
    }
}
```

### Example: Integration Test with Display Check

```go
// internal/pages/help/page_test.go
func TestNewHelpPage_NoDisplay(t *testing.T) {
    if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
        t.Skip("Skipping GTK integration test: no display")
    }

    // GTK init (would panic without display)
    gtk.Init()

    deps := &pages.Deps{
        Config:  config.Load(),
        Toaster: &mockToaster{},
    }

    page := NewHelpPage(deps)
    if page == nil {
        t.Fatal("NewHelpPage returned nil")
    }

    // Verify widget tree exists
    widget := page.Widget()
    if widget == nil {
        t.Error("Widget() returned nil")
    }
}

type mockToaster struct{}
func (m *mockToaster) ShowToast(msg string)      {}
func (m *mockToaster) ShowErrorToast(msg string) {}
```

### Example: Library Package with Godoc

```go
// pkg/adwutil/doc.go
/*
Package adwutil provides reusable GTK4/Libadwaita patterns for Go applications.

This package extracts common patterns from ChairLift into a reusable library,
including async utilities, error handling, and widget helpers.

# Thread Safety

GTK is not thread-safe. All widget operations must occur on the GTK main thread.
Use [RunOnMain] to schedule UI updates from goroutines:

    go func() {
        result := doWork()
        adwutil.RunOnMain(func() {
            label.SetText(result)
        })
    }()

# Widgets

The package provides helper functions for common widget patterns:

    // Empty state display
    status := adwutil.NewEmptyState(adwutil.EmptyStateConfig{
        Title:       "No Items",
        Description: "Items will appear here",
        IconName:    "folder-symbolic",
    })

    // Action rows with buttons
    row := adwutil.NewButtonRow("Update", "v2.0 available", "Install", onClick)

# Operations Tracking

Track long-running operations with the registry:

    op := adwutil.Start("Installing Firefox", adwutil.CategoryInstall, true)
    go func() {
        err := install()
        adwutil.RunOnMain(func() {
            op.Complete(err)
        })
    }()

# Error Handling

User-friendly errors with technical details:

    err := adwutil.NewUserErrorWithHint(
        "Couldn't install Firefox",
        "Check your internet connection",
        technicalErr,
    )
*/
package adwutil
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `assert` packages | Plain `t.Errorf` | Go community consensus | Simpler, clearer failures |
| Loop variable capture | Fixed in Go 1.22 | 2024 | No more `tc := tc` in range loops |
| `interface{}` for fixtures | Generics | Go 1.18 (2022) | Type-safe test helpers |
| External coverage tools | `go test -cover` | Improved in Go 1.20+ | Better HTML reports |

**Deprecated/outdated:**
- `go-check`: Older test framework, community moved to standard `testing`
- `goconvey`: Adds complexity without proportional benefit for this project size

## Open Questions

Things that couldn't be fully resolved:

1. **puregotk testing patterns**
   - What we know: puregotk is experimental, no published testing guide
   - What's unclear: Whether widget property getters work reliably in tests
   - Recommendation: Verify widget existence only; don't test widget properties in depth

2. **Operations registry reset between tests**
   - What we know: Current implementation uses package-level singleton
   - What's unclear: Best approach for test isolation
   - Recommendation: Add `Reset()` function for test cleanup or use per-test registry instances

3. **Library versioning strategy**
   - What we know: Library starts in same repo at `/pkg/adwutil/`
   - What's unclear: When/how to extract to separate repo with semver
   - Recommendation: Extract when ChairLift ships v1.0 and patterns are proven stable

## Sources

### Primary (HIGH confidence)
- [Go Wiki: TableDrivenTests](https://go.dev/wiki/TableDrivenTests) - Official Go testing patterns
- [Go Doc Comments](https://tip.golang.org/doc/comment) - Package documentation conventions
- [GTK4 Testing](https://docs.gtk.org/gtk4/running.html) - Environment variables, display requirements
- Existing codebase: `internal/pages/*/logic_test.go` - Established patterns

### Secondary (MEDIUM confidence)
- [Learn Go with Tests](https://quii.gitbook.io/learn-go-with-tests/) - DI and mocking patterns
- [Dave Cheney: Test Fixtures](https://dave.cheney.net/2016/05/10/test-fixtures-in-go) - testdata convention
- [golang-standards/project-layout](https://github.com/golang-standards/project-layout) - /pkg directory convention

### Tertiary (LOW confidence)
- [puregotk GitHub](https://github.com/jwijenbergh/puregotk) - Library is experimental, testing approach unverified
- Web search results for "GTK headless testing" - Community approaches vary

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Go testing is well-documented and stable
- Architecture: HIGH - Patterns verified against existing codebase
- Pitfalls: MEDIUM - GTK-specific issues based on general GTK knowledge, not puregotk-specific testing

**Research date:** 2026-01-28
**Valid until:** 2026-03-28 (60 days - Go testing patterns are stable)
