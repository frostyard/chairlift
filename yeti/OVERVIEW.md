# ChairLift Overview

## Purpose

ChairLift is a GTK4/Libadwaita system management GUI for [Snow Linux](https://github.com/frostyard/snow), written in Go using [puregotk](https://codeberg.org/puregotk/puregotk) bindings (no CGO). It provides a unified interface for managing Homebrew packages, Flatpak/Snap applications, NBC bootc system updates, system features (via updex), and maintenance tasks. The UI is YAML-configuration-driven, making it portable to other Linux distributions by toggling feature groups on/off.

## Architecture

```
cmd/chairlift/main.go                 Entry point: version injection, app creation
cmd/chairlift-updex-helper/main.go    Privileged helper for updex write operations
        │
internal/app/app.go             GObject-registered Application (adw.Application subtype)
        │
internal/window/window.go       Main window: NavigationSplitView, sidebar, content stack
        │
internal/views/                 Page builders and event handlers (one file per page)
        │
        ├── internal/config/    YAML config loading, feature group enablement
        ├── internal/homebrew/  Homebrew CLI wrapper (JSON output parsing)
        ├── internal/flatpak/   Flatpak CLI wrapper (tabular output parsing)
        ├── internal/nbc/       NBC bootc wrapper (pkexec, streaming progress)
        ├── internal/snap/      Snap wrapper (snapd REST API, not CLI)
        ├── internal/updex/     Updex feature manager (Go library reads, helper binary writes)
        └── internal/version/   Build metadata (ldflags injection)
```

### Dependency flow

`cmd → app → window → views → {config, homebrew, flatpak, nbc, snap, updex}`

External shared library: `github.com/frostyard/snowkit` (published module, pinned in go.mod) provides:
- `gobj` — GObject type registration and instance registry
- `sgtk.RunOnMainThread()` — main-thread dispatch for GTK safety

### Pages

The UI has six pages, each in its own file under `internal/views/`:

| Page | File | Purpose |
|------|------|---------|
| Applications | `applications_page.go` | Browse/install Flatpak, Snap, Homebrew packages; bundle install |
| Maintenance | `maintenance_page.go` | Homebrew/Flatpak cleanup, configurable maintenance scripts |
| Updates | `updates_page.go` | NBC system updates, Flatpak updates, Homebrew outdated packages |
| System | `system_page.go` | OS info (`/etc/os-release`), NBC bootc status, health monitor launch |
| Features | `features_page.go` | Toggle system features via `updex` tool |
| Help | `help_page.go` | Configurable links to website, issues, chat |

## Key Patterns

### GObject registration via snowkit

Application and Window are registered as GObject subtypes using `gobj.RegisterType()`. This returns a `gobject.Type` and an `*gobj.InstanceRegistry`. The pattern:

1. `init()` registers the type with `ClassInit` callback
2. `ClassInit` overrides `Constructed` to create the Go struct and pin it in the registry
3. Constructor (`New()`) calls `gobject.NewObject()` then retrieves the Go instance from the registry

See `internal/app/app.go:38-65` and `internal/window/window.go:60-88`.

### Async operations with main-thread dispatch

All external tool calls run in goroutines. UI updates are marshaled back via `sgtk.RunOnMainThread()`:

```go
go func() {
    result, err := homebrew.ListInstalledFormulae(ctx)
    sgtk.RunOnMainThread(func() {
        // update widgets here
    })
}()
```

### Dry-run mode

The `--dry-run` / `-d` flag is propagated to every wrapper package via `SetDryRun(true)`. Each wrapper skips state-changing commands when dry-run is active. This is set once at startup in `app.New()`.

### Configuration-driven UI visibility

Each preference group on every page checks `config.IsGroupEnabled(pageName, groupName)` before building its widgets. Groups default to enabled if not specified in config.

### Package manager wrapper pattern

Each wrapper in `internal/` follows a consistent shape:
- Module-level `dryRun` flag with `SetDryRun()`/`IsDryRun()`
- `IsInstalled()` to check tool availability, plus `IsInstalledCached()` (`sync.Once`) for use from views
- List/Search/Install/Uninstall/Update functions
- Context-based timeouts (30s for Homebrew, 60s for Flatpak/Snap, 5min for updex, 30min for NBC)
- Custom error types where needed

### Streaming progress (NBC)

NBC operations (update, download, install) use channel-based streaming:
1. `nbc.Update()` returns a `<-chan ProgressEvent`
2. The view goroutine reads events and dispatches UI updates to the main thread
3. Event types: `EventTypeStep`, `EventTypeProgress`, `EventTypeMessage`, `EventTypeError`, `EventTypeComplete`

### Update badge tracking

The updates page tracks counts from NBC, Flatpak, and Homebrew separately using a `sync.Mutex`. The total is pushed to the window's sidebar badge via `ToastAdder.SetUpdateBadge()`.

### Privileged operations

NBC and updex require root for state-changing operations. They invoke commands through `pkexec` (PolicyKit). NBC calls `pkexec nbc ...` directly, while updex delegates to a separate `chairlift-updex-helper` binary via `pkexec`. Polkit policy files are installed for both: `data/org.frostyard.ChairLift.nbc.policy` and `data/org.frostyard.ChairLift.updex.policy`.

## Configuration

### Config file search order

1. `/etc/chairlift/config.yml` — system-wide (highest priority)
2. `/usr/share/chairlift/config.yml` — package maintainer defaults
3. `config.yml` — relative to executable (development)

If no file is found, all features default to enabled. See [CONFIG.md](../CONFIG.md) for the full reference.

### Config structure

```yaml
page_name:
  group_name:
    enabled: true/false
    # Optional per-group fields:
    app_id: "..."          # External app to launch
    actions:               # Custom scripts (updates/maintenance)
      - title: "..."
        script: "/path/to/script"
        sudo: true/false
    bundles_paths: [...]   # Homebrew bundle directories
    website: "..."         # Help page URLs
    issues: "..."
    chat: "..."
```

### Key config groups

| Page | Group | Controls |
|------|-------|----------|
| `system_page` | `nbc_status_group` | NBC bootc status display |
| `system_page` | `health_group` | System monitor launcher (configurable `app_id`) |
| `updates_page` | `nbc_updates_group` | NBC bootc system updates |
| `applications_page` | `snap_group` | Snap package listing |
| `applications_page` | `brew_bundles_group` | Brewfile bundles from `bundles_paths` |
| `maintenance_page` | `maintenance_cleanup_group` | Custom cleanup scripts |
| `features_page` | `features_group` | Updex feature toggles |

## Build and Release

- **Build**: `make build` builds two binaries: `build/chairlift` (main app) and `build/chairlift-updex-helper` (privileged helper), both with `CGO_ENABLED=0`
- **Version**: Set via ldflags by goreleaser (`buildVersion`, `buildCommit`, `buildDate`, `buildBy`)
- **Semantic versioning**: Uses [svu](https://github.com/caarlos0/svu) via `make bump`
- **CI**: GitHub Actions workflows for test, snapshot, and release (`.github/workflows/`)
- **Release**: GoReleaser config at `.goreleaser.yaml`

### Runtime dependencies

- GTK 4 and libadwaita 1 (shared libraries loaded at runtime by puregotk)
- Homebrew (optional)
- Flatpak (optional)
- Snap/snapd (optional)
- NBC (`/usr/bin/nbc`) (optional)
- Updex features configured on the system (optional; read via Go library, writes via `chairlift-updex-helper`)

### Key external Go dependencies

| Module | Purpose |
|--------|---------|
| `codeberg.org/puregotk/puregotk` | GTK4/Adwaita bindings (no CGO) |
| `github.com/frostyard/snowkit` | GObject registration, main-thread dispatch |
| `github.com/frostyard/nbc` | NBC types (ProgressEvent, StatusOutput, etc.) |
| `github.com/frostyard/updex` | Updex Go library for feature reads and helper binary |
| `github.com/snapcore/snapd` | Snapd client library |
| `gopkg.in/yaml.v3` | YAML config parsing |

## Subsystem Details

- [Package Manager Wrappers](./package-managers.md) — Homebrew, Flatpak, Snap, NBC, and Updex wrapper details
