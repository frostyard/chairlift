# adwutil

Reusable GTK4/Libadwaita patterns for Go applications.

This library extracts common patterns from [ChairLift](https://github.com/frostyard/chairlift) into a reusable toolkit for building GTK4/Libadwaita applications with pure Go (via [puregotk](https://github.com/jwijenbergh/puregotk)).

## Features

- **Thread-safe async utilities** - Safe goroutine-to-UI communication with `RunOnMain`
- **User-friendly error handling** - `UserError` type separates user messages from technical details
- **Widget helpers** - Common patterns for ActionRows, empty states, and more
- **Operations tracking** - Track, cancel, and display progress of async operations

## Installation

```bash
go get github.com/frostyard/chairlift/pkg/adwutil
```

## Quick Start

### Thread-Safe UI Updates

GTK is not thread-safe. Use `RunOnMain` to update UI from goroutines:

```go
import "github.com/frostyard/chairlift/pkg/adwutil"

go func() {
    result := doExpensiveWork()
    adwutil.RunOnMain(func() {
        label.SetText(result)
    })
}()
```

### User-Friendly Errors

Create errors that show users clear messages while preserving technical details:

```go
err := adwutil.NewUserErrorWithHint(
    "Couldn't install Firefox",
    "Check your internet connection",
    technicalErr,
)

// For UI display
message := err.FormatForUser()  // "Couldn't install Firefox: Check your internet connection"

// For logging
details := err.FormatWithDetails()  // Includes technical error
```

### Widget Helpers

Create common widget patterns with less boilerplate:

```go
// Empty state placeholder
emptyState := adwutil.NewEmptyState(adwutil.EmptyStateConfig{
    Title:       "No Items",
    Description: "Items will appear here when added",
    IconName:    "folder-symbolic",
})

// Row with action button
row := adwutil.NewButtonRow(
    "Firefox",
    "Web browser",
    "Install",
    func() { installApp("firefox") },
)

// Link row (opens URL or app)
link := adwutil.NewLinkRow(
    "Documentation",
    "View online docs",
    func() { openURL("https://docs.example.com") },
)

// Info row (read-only display)
info := adwutil.NewInfoRow("Version", "1.0.0")

// Icon row (with prefix icon)
status := adwutil.NewIconRow("Status", "All systems operational", "object-select-symbolic")
```

### Operations Tracking

Track long-running operations with progress and cancellation:

```go
// Start a tracked operation
op := adwutil.Start("Installing Firefox", adwutil.CategoryInstall, true)

go func() {
    err := install("firefox")
    adwutil.RunOnMain(func() {
        op.Complete(err)
    })
}()

// Update progress (from goroutine)
adwutil.RunOnMain(func() {
    op.UpdateProgress(0.5, "Downloading...")
})

// Cancel operation
op.Cancel()

// Listen for operation changes
adwutil.AddListener(func(op *adwutil.Operation) {
    updateUI(op)
})
```

## Examples

See the [examples](./examples/) directory for complete working applications:

- [`basic`](./examples/basic/) - Demonstrates RunOnMain, widgets, and error handling
- [`operations`](./examples/operations/) - Demonstrates async operation tracking with progress

Run examples:

```bash
cd pkg/adwutil/examples/basic
go run main.go
```

## API Reference

See [pkg.go.dev documentation](https://pkg.go.dev/github.com/frostyard/chairlift/pkg/adwutil) for full API reference.

### Key Types

| Type | Description |
|------|-------------|
| `UserError` | User-friendly error with summary, hint, and technical details |
| `EmptyStateConfig` | Configuration for empty state displays |
| `Operation` | Tracked async operation with progress and cancellation |
| `Registry` | Operations registry (singleton available via package functions) |
| `Category` | Operation category (Install, Update, Loading, Maintenance) |
| `State` | Operation state (Active, Completed, Failed, Cancelled) |

### Key Functions

| Function | Description |
|----------|-------------|
| `RunOnMain(fn)` | Schedule function to run on GTK main thread |
| `NewUserError(summary, err)` | Create user error without hint |
| `NewUserErrorWithHint(summary, hint, err)` | Create user error with hint |
| `NewEmptyState(cfg)` | Create empty state StatusPage |
| `NewButtonRow(title, subtitle, label, onClick)` | Create row with action button |
| `NewButtonRowWithClass(title, subtitle, label, class, onClick)` | Create row with styled button |
| `NewLinkRow(title, subtitle, onClick)` | Create row with link icon |
| `NewInfoRow(title, subtitle)` | Create read-only info row |
| `NewIconRow(title, subtitle, iconName)` | Create row with prefix icon |
| `Start(name, category, cancellable)` | Start tracked operation |
| `StartWithContext(ctx, name, category)` | Start operation with context cancellation |

### Operation Categories

| Category | Use Case |
|----------|----------|
| `CategoryInstall` | Package installation operations |
| `CategoryUpdate` | System update operations |
| `CategoryLoading` | Data loading operations |
| `CategoryMaintenance` | Cleanup and maintenance operations |

### Operation States

| State | Description |
|-------|-------------|
| `StateActive` | Operation is in progress |
| `StateCompleted` | Operation finished successfully |
| `StateFailed` | Operation finished with an error |
| `StateCancelled` | Operation was cancelled by user |

## Design Philosophy

### Error Tone

Error messages follow GNOME HIG guidelines:
- Use "Couldn't" not "Failed to" or "Error:"
- Keep summaries short and action-oriented
- Include hints only when actionable

### Thread Safety

All widget operations must happen on the GTK main thread. The `RunOnMain` function provides a safe way to schedule UI updates from goroutines while preventing callback garbage collection.

### Callback Registry

Signal callbacks passed to GTK widget methods are stored in a registry to prevent Go's garbage collector from collecting them before the signals fire.

## Requirements

- Go 1.22+
- GTK4 and Libadwaita installed
- puregotk v4

## License

MIT License - see [LICENSE](../../LICENSE)
