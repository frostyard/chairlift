# Testing Patterns

**Analysis Date:** 2026-01-26

## Test Framework

**Runner:**
- Go's built-in `testing` package
- No third-party test runners

**Assertion Library:**
- Standard Go testing assertions (`t.Error()`, `t.Fatal()`, `t.Errorf()`)
- No external assertion libraries detected

**Run Commands:**
```bash
make test              # Run all tests
go test -v ./...       # Run all tests with verbose output
go test -v ./internal/... -coverprofile=coverage.out -covermode=atomic  # With coverage
go test -race -short ./internal/...   # With race detector
```

## Test File Organization

**Location:**
- Co-located pattern expected (tests next to source files)
- Currently: **No test files exist in the codebase**

**Expected Naming:**
- `<source>_test.go` (e.g., `config_test.go` alongside `config.go`)

**Expected Structure:**
```
internal/
├── config/
│   ├── config.go
│   └── config_test.go      # Would go here
├── nbc/
│   ├── nbc.go
│   └── nbc_test.go         # Would go here
```

## Test Structure

**Suite Organization (Expected Pattern):**
```go
package config_test

import (
    "testing"
    
    "github.com/frostyard/chairlift/internal/config"
)

func TestLoad(t *testing.T) {
    t.Run("loads from default path", func(t *testing.T) {
        cfg := config.Load()
        if cfg == nil {
            t.Fatal("expected config to be loaded")
        }
    })
    
    t.Run("returns default when file not found", func(t *testing.T) {
        cfg := config.Load()
        if !cfg.IsGroupEnabled("system_page", "system_info_group") {
            t.Error("expected default config to enable system_info_group")
        }
    })
}
```

**Patterns:**
- Use subtests with `t.Run()` for related test cases
- Setup: performed at start of test function
- Teardown: use `defer` for cleanup or `t.Cleanup()`
- Assertion: compare expected vs actual with descriptive messages

## Mocking

**Framework:** None currently integrated

**Expected Patterns:**
```go
// Interface-based mocking for ToastAdder
type mockToastAdder struct {
    toasts      []string
    errorToasts []string
    badge       int
}

func (m *mockToastAdder) ShowToast(message string) {
    m.toasts = append(m.toasts, message)
}

func (m *mockToastAdder) ShowErrorToast(message string) {
    m.errorToasts = append(m.errorToasts, message)
}

func (m *mockToastAdder) SetUpdateBadge(count int) {
    m.badge = count
}
```

**What to Mock:**
- External commands (`exec.Command` calls to `nbc`, `pkexec`, `brew`, `flatpak`)
- File system operations for config loading
- GTK/UI dependencies (use interfaces like `ToastAdder`)

**What NOT to Mock:**
- Pure functions with no side effects
- Internal config/type operations
- JSON marshaling/unmarshaling

## Fixtures and Factories

**Test Data:**
- Currently no fixtures defined
- Expected pattern: use `testdata/` directory for fixture files

**Expected Location:**
```
internal/config/testdata/
├── valid-config.yml
├── minimal-config.yml
└── invalid-config.yml
```

**Test Data Factory Pattern:**
```go
func testConfig() *config.Config {
    return &config.Config{
        SystemPage: config.PageConfig{
            "system_info_group": config.GroupConfig{Enabled: true},
        },
    }
}
```

## Coverage

**Requirements:**
- No enforced coverage threshold
- Coverage uploaded to Codecov in CI (continues on error)

**View Coverage:**
```bash
go test -v ./internal/... -coverprofile=coverage.out -covermode=atomic
go tool cover -html=coverage.out -o coverage.html
```

## Test Types

**Unit Tests:**
- Focus on individual packages in `internal/`
- Run with: `go test -v ./internal/... -run "^Test[^I]" -skip "Integration"`
- Exclude integration tests by naming convention

**Integration Tests:**
- Naming convention: prefix with `TestIntegration` or include `Integration` in name
- Skipped in CI unit test runs
- Require external tools (nbc, flatpak, brew)

**Race Detection:**
- Run with: `go test -race -short ./internal/...`
- Part of CI pipeline (`race-test` job)
- Uses `-short` flag to reduce test duration

**E2E Tests:**
- Not currently implemented
- Manual testing via `make run` with `--dry-run` flag

## Common Patterns

**Async Testing:**
```go
func TestAsyncOperation(t *testing.T) {
    done := make(chan struct{})
    
    go func() {
        defer close(done)
        // Perform async operation
    }()
    
    select {
    case <-done:
        // Test passed
    case <-time.After(5 * time.Second):
        t.Fatal("test timed out")
    }
}
```

**Error Testing:**
```go
func TestErrorHandling(t *testing.T) {
    _, err := nbc.GetStatus(context.Background())
    
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    
    var notFoundErr *nbc.NotFoundError
    if errors.As(err, &notFoundErr) {
        // Expected error type
        return
    }
    
    t.Errorf("unexpected error type: %T", err)
}
```

**Context Testing:**
```go
func TestContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cancel() // Cancel immediately
    
    _, err := nbc.GetStatus(ctx)
    if err == nil {
        t.Fatal("expected error on cancelled context")
    }
}
```

**Table-Driven Tests:**
```go
func TestIsGroupEnabled(t *testing.T) {
    tests := []struct {
        name      string
        page      string
        group     string
        expected  bool
    }{
        {"valid enabled group", "system_page", "system_info_group", true},
        {"missing group defaults true", "system_page", "unknown_group", true},
        {"disabled group", "maintenance_page", "disabled_group", false},
    }
    
    cfg := config.Load()
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := cfg.IsGroupEnabled(tt.page, tt.group)
            if result != tt.expected {
                t.Errorf("got %v, want %v", result, tt.expected)
            }
        })
    }
}
```

## CI Pipeline

**Jobs (from `.github/workflows/test.yml`):**

| Job | Purpose | Command |
|-----|---------|---------|
| `lint` | Code quality | `golangci-lint` |
| `unit-test` | Unit tests + coverage | `go test -v ./internal/... -coverprofile=coverage.out` |
| `race-test` | Race condition detection | `go test -race -short ./internal/...` |
| `verify` | Go mod, vet, gofmt checks | `go mod tidy`, `go vet`, `gofmt` |
| `build` | Build binaries (amd64, arm64) | `make build` |

**Test Filtering in CI:**
- Unit tests exclude integration: `-run "^Test[^I]" -skip "Integration"`
- Race tests use `-short` flag

## Testing Guidelines

**Before Running Tests:**
1. Ensure dependencies are downloaded: `go mod download`
2. Format code: `make fmt`
3. Run linter: `make lint`

**When Writing Tests:**
1. Follow existing naming conventions
2. Use subtests for related cases
3. Mock external dependencies (commands, filesystem)
4. Test error paths explicitly
5. Add context cancellation tests for async operations
6. Use table-driven tests for multiple input variations

**Test Independence:**
- Each test should be independent
- Don't rely on test execution order
- Clean up any created resources

---

*Testing analysis: 2026-01-26*
