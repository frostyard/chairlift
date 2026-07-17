# ChairLift Overview

## Purpose

ChairLift is a GTK4/Libadwaita system management GUI for [Snow Linux](https://github.com/frostyard/snow), written in Go using [puregotk](https://codeberg.org/puregotk/puregotk) bindings (no CGO). It provides a unified interface for managing Homebrew and Flatpak applications, bootc system updates (staged via the snow `bootc-update-stage` script), system features (via updex), and maintenance tasks. The UI is YAML-configuration-driven, making it portable to other Linux distributions by toggling feature groups on/off.

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
        ├── internal/bootc/     bootc wrapper (status reads, pkexec stage script, line streaming)
        ├── internal/updex/     Updex feature manager (Go library reads, helper binary writes)
        └── internal/version/   Build metadata (ldflags injection)
```

### Dependency flow

`cmd → app → window → views → {config, homebrew, flatpak, bootc, updex}`

External shared library: `github.com/frostyard/snowkit` (published module, pinned in go.mod) provides:
- `gobj` — GObject type registration and instance registry
- `sgtk.RunOnMainThread()` — main-thread dispatch for GTK safety

### Views coordinator (`internal/views/views.go`)

The `views.go` file defines the central `UserHome` struct that holds references to all page widgets, config, and the `ToastAdder` interface. It provides:
- `New(cfg, toastAdder)` — constructor that initializes `UserHome`
- `ToastAdder` interface — `ShowToast(msg)`, `ShowErrorToast(msg)`, `SetUpdateBadge(count)` — implemented by Window

### Pages

The UI has six pages, each in its own file under `internal/views/`:

| Page | File | Purpose |
|------|------|---------|
| Applications | `applications_page.go` | Browse/install Flatpak (user+system) and Homebrew packages |
| Maintenance | `maintenance_page.go` | Homebrew/Flatpak cleanup, configurable maintenance scripts (executed via `exec.Command`/`pkexec`) |
| Updates | `updates_page.go` | bootc staged system updates, Flatpak updates, Homebrew outdated packages, untrusted-tap trust prompts |
| System | `system_page.go` | OS info (`/etc/os-release`), bootc deployment status, health monitor launch |
| Features | `features_page.go` | Toggle system features via `updex` tool |
| Help | `help_page.go` | Configurable links to website, issues, chat (opened via `xdg-open`) |

## Key Patterns

### GObject registration via snowkit

Application and Window are registered as GObject subtypes using `gobj.RegisterType()`. This returns a `gobject.Type` and an `*gobj.InstanceRegistry`. The pattern:

1. `init()` registers the type with `ClassInit` callback
2. `ClassInit` overrides `Constructed` to create the Go struct and pin it in the registry
3. Constructor (`New()`) calls `gobject.NewObject()` then retrieves the Go instance from the registry

See `internal/app/app.go` and `internal/window/window.go`.

### Async operations with main-thread dispatch

All external tool calls run in goroutines. UI updates are marshaled back via `sgtk.RunOnMainThread()`:

```go
go func() {
    result, err := homebrew.ListInstalledFormulae()
    sgtk.RunOnMainThread(func() {
        // update widgets here
    })
}()
```

### Deferred visibility (async startup)

To avoid blocking startup on slow tool-availability checks, groups that depend on optional tools (Homebrew, Flatpak, Updex) are built immediately with placeholder descriptions and then shown or hidden asynchronously. The pattern:

1. Build the UI group unconditionally (if config-enabled), with a placeholder description
2. Store a reference to the group on `UserHome` (e.g., `maintenanceBrewGroup`)
3. Spawn a goroutine that calls `IsInstalledCached()` (see below)
4. On the main thread, either hide the group (`SetVisible(false)`) or update its description

This applies to: `maintenanceBrewGroup`, `maintenanceFlatpakGroup`, `featuresGroup`/`featuresUnavailableGroup`. The Features page uses a dual-group approach — one for available features, one for "not available" — toggling visibility between them.

### bootc boot gate

bootc-related UI groups (system page's `bootc_status_group` and updates page's `bootc_updates_group`) are gated on `bootc.IsBootcBootedCached()`, which runs `bootc status --format json` once (via `sync.Once`) and reports true only when the parsed `status.booted` field is non-null. This is deliberately not a sentinel-file check: `/run/ostree-booted` is absent on snow's composefs-based deployments, so relying on it would hide the groups on every snow bootc host. `bootc status` itself exits 0 with a null `booted` entry on non-bootc hosts, so the gate must inspect the JSON body rather than the exit code.

### Dry-run mode

The `--dry-run` / `-d` flag is propagated to wrapper packages via `SetDryRun(true)`. Set once at startup in `app.New()` for homebrew, flatpak, bootc, and updex. Each wrapper handles dry-run differently:

- **Homebrew/Flatpak/Updex**: State-changing commands are skipped entirely (return mock/empty results)
- **bootc**: `StageUpdate` short-circuits before invoking pkexec: it logs the would-be command, emits a synthetic `EventMessage` + `EventComplete` pair on the progress channel, and returns — the stage script is never actually run

### Configuration-driven UI visibility

Each preference group on every page checks `config.IsGroupEnabled(pageName, groupName)` before building its widgets. Groups default to enabled if not specified in config. The `maintenance_cleanup_group` defaults to disabled in the default config.

### Package manager wrapper pattern

Each wrapper in `internal/` follows a consistent shape:
- Module-level `dryRun` flag with `SetDryRun()`/`IsDryRun()`
- `IsInstalled()` to check tool availability, plus `IsInstalledCached()` (`sync.Once`) for use from views during async startup
- Homebrew, Flatpak, and Updex implement both `IsInstalled()` and `IsInstalledCached()`
- List/Search/Install/Uninstall/Update functions
- Context-based timeouts (30s for Homebrew, 60s for Flatpak, 5min for updex, 30min for bootc)
- Custom error types where needed

### Streaming progress (bootc stage)

`bootc.StageUpdate(ctx, progressCh)` runs `pkexec /usr/libexec/bootc-update-stage`, streaming combined stdout+stderr line-by-line to the caller's channel and closing it when done:
1. Caller creates a `chan bootc.ProgressEvent` and passes it to `StageUpdate`
2. Each non-empty output line becomes an `EventMessage`; the channel is closed after either an `EventComplete` (success) or the function returning an error
3. Event types: `EventMessage`, `EventError`, `EventComplete` — deliberately simpler than a step/percent model because the stage script's own output is unstructured log lines, not a structured progress protocol
4. The view goroutine (`internal/views/updates_page.go`, `onBootcStageClicked`) reads events and dispatches UI updates to the main thread via `sgtk.RunOnMainThread`

**Why a stage script instead of `bootc upgrade`:** upstream `bootc upgrade`'s registry-transport pull currently fails on snow's composefs images. The snow-shipped `/usr/libexec/bootc-update-stage` script works around this: `podman pull` fetches the image into containers-storage (podman's pull path works where bootc's does not), then `bootc switch --transport containers-storage` stages the already-pulled image as the next boot deployment. This keeps snow's actual upgrade logic in one place (the snosi script) rather than duplicating pull/switch orchestration in ChairLift; ChairLift only invokes the script via pkexec and streams its output. The script is idempotent — it exits 0 without staging anything when the deployment is already current.

### bootc progress UI (updates page)

`onBootcStageClicked()` (`internal/views/updates_page.go`) drives the "System Update" expander: it disables the button, spawns `bootc.StageUpdate` in a goroutine, and processes the `ProgressEvent` channel on a second goroutine — `EventMessage` lines are appended to a log expander with timestamps, `EventError` surfaces an error toast, and `EventComplete` re-queries `bootc.GetStatus` to refresh the staged/booted summary and re-enables the button. The system page has a separate, simpler bootc path: `loadBootcStatus` (gated on `IsBootcBootedCached()`) calls `bootc.GetStatus` to show the booted/staged/rollback deployment images, versions, and digests, with no staging controls of its own — staging happens on the Updates page.

### Update badge tracking

The updates page tracks counts from bootc, Flatpak, and Homebrew separately (`bootcUpdateCount`, `flatpakUpdateCount`, `brewUpdateCount` fields on `UserHome`) using a `sync.Mutex`. `bootcUpdateCount` is 1 when `bootc.GetStatus()` reports a staged deployment, 0 otherwise — it is not a count of available updates, just a boolean folded into the badge total. The total is pushed to the window's sidebar badge via `ToastAdder.SetUpdateBadge()`.

### Privileged operations

bootc staging and updex require root for state-changing operations. They invoke commands through `pkexec` (PolicyKit). bootc runs `pkexec /usr/libexec/bootc-update-stage` directly (polkit action id `org.frostyard.ChairLift.bootc.stage`), while updex delegates to a separate `chairlift-updex-helper` binary via `pkexec`. Polkit policy files are installed for both: `data/org.frostyard.ChairLift.bootc.policy` and `data/org.frostyard.ChairLift.updex.policy`. Homebrew tap trust (`brew trust`) is explicitly per-user and does *not* go through pkexec — see [package-managers.md](./package-managers.md).

### Maintenance action execution

Configurable maintenance scripts (from `config.yml` `actions` entries) are executed via `runMaintenanceAction()` in `internal/views/maintenance_page.go`. The pattern:
1. Button is disabled and label set to "Running..."
2. A goroutine spawns the script via `exec.CommandContext` (5-minute timeout), using `pkexec` wrapper if `sudo: true`
3. On completion, the main thread re-enables the button and shows a success/error toast

### Keyboard shortcuts

The window registers keyboard accelerators (`internal/window/window.go`):
- `Ctrl+Q` → quit
- `Ctrl+?` → show shortcuts dialog
- `Alt+1` through `Alt+6` → navigate to each page (Applications, Maintenance, Updates, System, Features, Help)

Note: `GtkShortcutsWindow` is not available in puregotk, so a custom `adw.Window` with `adw.PreferencesGroup` rows is used for the shortcuts dialog.

### URL opening

Help page links are opened via `xdg-open` using `exec.Command`. The process is started asynchronously and its exit is waited on in a goroutine to avoid zombie processes.

## Configuration

### Config file search order

1. `/etc/chairlift/config.yml` — system-wide (highest priority)
2. `/usr/share/chairlift/config.yml` — package maintainer defaults
3. `config.yml` — relative to executable (development)

If no file is found, all features default to enabled (except `maintenance_cleanup_group` which defaults to disabled). See [CONFIG.md](../CONFIG.md) for the full reference.

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
| `system_page` | `system_info_group` | OS info from `/etc/os-release` |
| `system_page` | `bootc_status_group` | bootc deployment status display (gated on `bootc.IsBootcBootedCached()`) |
| `system_page` | `health_group` | System monitor launcher (configurable `app_id`, default: Mission Center) |
| `updates_page` | `bootc_updates_group` | bootc system updates — stage via `bootc-update-stage`, apply on restart (gated on `bootc.IsBootcBootedCached()` and stage script availability) |
| `updates_page` | `flatpak_updates_group` | Flatpak pending updates |
| `updates_page` | `brew_updates_group` | Homebrew outdated packages |
| `updates_page` | `brew_trust_group` | Untrusted Homebrew taps with installed packages (Homebrew 6 tap trust); hidden unless there is something to trust |
| `updates_page` | `updates_settings_group` | Update settings |
| `applications_page` | `flatpak_user_group` | User Flatpak applications |
| `applications_page` | `flatpak_system_group` | System Flatpak applications |
| `applications_page` | `brew_group` | Homebrew formulae and casks |
| `applications_page` | `brew_search_group` | Homebrew package search |
| `applications_page` | `brew_bundles_group` | Config key exists but has no corresponding UI builder in current code |
| `applications_page` | `applications_installed_group` | Installed apps launcher (configurable `app_id`, default: Bazaar) |
| `maintenance_page` | `maintenance_cleanup_group` | Custom cleanup scripts (5min timeout, pkexec for sudo); **disabled by default** |
| `maintenance_page` | `maintenance_brew_group` | Homebrew cleanup (deferred visibility) |
| `maintenance_page` | `maintenance_flatpak_group` | Flatpak unused cleanup (deferred visibility) |
| `maintenance_page` | `maintenance_optimization_group` | System optimization (placeholder) |
| `features_page` | `features_group` | Updex feature toggles |
| `help_page` | `help_resources_group` | Configurable links (website, issues, chat) |

## Build and Release

- **Build**: `make build` builds two binaries: `build/chairlift` (main app) and `build/chairlift-updex-helper` (privileged helper), both with `CGO_ENABLED=0`
- **Dev build**: `make dev` builds with `CGO_ENABLED=1` and `-race` flag for race detection
- **Version**: Set via ldflags by goreleaser (`buildVersion`, `buildCommit`, `buildDate`, `buildBy`)
- **Semantic versioning**: Uses [svu](https://github.com/caarlos0/svu) via `make bump`
- **CI**: GitHub Actions workflows for test, snapshot, and release (`.github/workflows/`)
- **Release**: GoReleaser config at `.goreleaser.yaml`
- **Other targets**: `make fmt` (gofmt), `make lint` (golangci-lint), `make install`/`make uninstall` (system install including polkit policies, icons, and wrapper script), `make build-linux-amd64`/`make build-linux-arm64` (cross-compilation)

### Runtime dependencies

- GTK 4 and libadwaita 1 (shared libraries loaded at runtime by puregotk)
- Homebrew (optional)
- Flatpak (optional)
- `bootc` + `/usr/libexec/bootc-update-stage` (both optional; UI gated on `bootc.IsBootcBootedCached()`, i.e. `bootc status` reporting a non-null `booted` deployment — not on any sentinel file)
- Updex features configured on the system (optional; read via Go library, writes via `chairlift-updex-helper`)

### Key external Go dependencies

| Module | Purpose |
|--------|---------|
| `codeberg.org/puregotk/puregotk` | GTK4/Adwaita bindings (no CGO) |
| `github.com/frostyard/snowkit` | GObject registration, main-thread dispatch |
| `github.com/frostyard/updex` | Updex Go library for feature reads and helper binary (currently pinned to v1.2.3 in go.mod) |
| `gopkg.in/yaml.v3` | YAML config parsing |
| `golang.org/x/text` | Title-casing OS release info keys |

There is no separate Go client library dependency for bootc: status/stage types (`Status`, `Deployment`, `ProgressEvent`, etc.) are defined locally in `internal/bootc`, parsed directly from `bootc status --format json` and the stage script's line output.

## Subsystem Details

- [Package Manager Wrappers](./package-managers.md) — Homebrew (including tap trust), Flatpak, bootc, and Updex wrapper details
