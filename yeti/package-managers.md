# Package Manager Wrappers

Each wrapper lives in its own package under `internal/` and follows a consistent pattern: module-level dry-run flag, availability check, and context-based timeouts. All are called from `internal/views/` page builders.

## Homebrew (`internal/homebrew/homebrew.go`)

Wraps the `brew` CLI. Uses JSON output (`--json=v2`) for structured data where available.

### Key types

- **`Package`** — name, version, pinned status, cask flag
- **`SearchResult`** — name, type (formula/cask)

### Operations

| Function | CLI command | Timeout | Notes |
|----------|------------|---------|-------|
| `ListInstalledFormulae()` | `brew list --formula --json` | 30s | JSON parsed |
| `ListInstalledCasks()` | `brew list --cask --json` | 30s | JSON parsed |
| `ListOutdated()` | `brew outdated --json` | 30s | JSON parsed |
| `Search(query)` | `brew search <query>` | 30s | Text output parsed |
| `Install(name, isCask)` | `brew install [--cask] <name>` | 30s | State-changing, dry-run aware |
| `Uninstall(name, isCask)` | `brew uninstall [--cask] <name>` | 30s | State-changing |
| `Upgrade(name)` | `brew upgrade <name>` | 30s | State-changing |
| `Update()` | `brew update` | 30s | State-changing |
| `Pin(name)` / `Unpin(name)` | `brew pin/unpin <name>` | 30s | State-changing |
| `Cleanup()` | `brew cleanup` | 30s | State-changing |
| `BundleDump()` | `brew bundle dump --file=-` | 30s | Outputs Brewfile to stdout |
| `BundleInstall(path)` | `brew bundle install --file=<path>` | 30s | State-changing |

### Error handling

Returns `Error` (wraps stderr message) or `NotFoundError` for missing packages. Timeouts produce a specific error message.

## Flatpak (`internal/flatpak/flatpak.go`)

Wraps the `flatpak` CLI. Parses tabular (whitespace-delimited) output.

### Key types

- **`Application`** — name, appID, version, branch, origin, installation (user/system)
- **`UpdateInfo`** — appID, remoteRef, installedSize, downloadSize
- **`ApplicationInfo`** — detailed metadata (description, developer, etc.)

### Operations

| Function | CLI command | Timeout | Notes |
|----------|------------|---------|-------|
| `ListUserApplications()` | `flatpak list --user --app --columns=...` | 60s | Tabular parsed |
| `ListSystemApplications()` | `flatpak list --system --app --columns=...` | 60s | Tabular parsed |
| `ListUpdates()` | `flatpak remote-ls --updates --columns=...` | 60s | Both user+system |
| `Install(appID)` | `flatpak install -y <appID>` | 60s | State-changing |
| `Uninstall(appID, userOrSystem)` | `flatpak uninstall -y [--user\|--system] <appID>` | 60s | State-changing |
| `Update(appID)` | `flatpak update -y <appID>` | 60s | State-changing |
| `UninstallUnused()` | `flatpak uninstall --unused -y` | 60s | Maintenance cleanup |
| `Info(appID)` | `flatpak info <appID>` | 60s | Key-value parsed |
| `GetRemotes()` | `flatpak remotes --columns=...` | 60s | Lists configured remotes |

## Snap (`internal/snap/snap.go`)

Uses the **snapd REST API** directly (via `github.com/snapcore/snapd` client library), not the `snap` CLI.

### Key types

- **`Application`** — name, ID, version, channel, confinement, developer, status

### Operations

| Function | Method | Timeout | Notes |
|----------|--------|---------|-------|
| `IsInstalled()` | `GET /v2/system-info` | 60s | Checks snapd availability |
| `IsInstalledCached()` | Cached `IsInstalled()` | — | `sync.Once` |
| `ListInstalledSnaps()` | `GET /v2/snaps` | 60s | API call |
| `IsSnapInstalled(name)` | `GET /v2/snaps/<name>` | 60s | API call |
| `Install(ctx, name)` | `POST /v2/snaps/<name>` | 60s | Returns changeID, async, dry-run aware |
| `WaitForChange(ctx, changeID)` | Polls `GET /v2/changes/<id>` | — | Polls every 500ms until done/error |

### Notable

- Uses snapd socket at `/run/snapd.socket`
- Snap install is async — returns a changeID that must be polled
- Supports interactive polkit authentication
- Handles `ErrNoSnapsInstalled` gracefully (returns empty list)

## NBC (`internal/nbc/nbc.go`)

Wraps the `nbc` CLI for bootc (OSTree-based) system updates. The most complex wrapper due to streaming progress and privilege requirements.

### Execution modes

1. **Direct** — read-only queries (`nbc status`, `nbc check-update`)
2. **pkexec** — privileged operations (`pkexec nbc update`, `pkexec nbc download`)
3. **Streaming** — privileged operations with line-by-line JSON progress output

### Key types

Re-exported from `github.com/frostyard/nbc/pkg/types`:
- **`StatusOutput`** — booted/staged image info, device, slots
- **`UpdateCheck`** — available update details
- **`ProgressEvent`** — step, progress percentage, messages, errors

### Event types for streaming

- `EventTypeStep` — new operation phase
- `EventTypeProgress` — percentage update
- `EventTypeMessage` — informational log line
- `EventTypeError` — error during operation
- `EventTypeComplete` — operation finished

### Operations

| Function | CLI command | Mode | Timeout | Notes |
|----------|------------|------|---------|-------|
| `GetStatus()` | `nbc status --json` | Direct | 30min | JSON parsed |
| `CheckUpdate()` | `nbc check-update --json` | Direct | 30min | JSON parsed |
| `ListDisks()` | `nbc disk list --json` | Direct | 30min | JSON parsed |
| `Update(opts)` | `pkexec nbc update --json` | Streaming | 30min | Channel-based progress |
| `Download(opts)` | `pkexec nbc download --json` | Streaming | 30min | Channel-based progress |
| `Install(opts)` | `pkexec nbc install --json` | Streaming | 30min | Channel-based progress |
| `Validate(device)` | `pkexec nbc disk validate --json` | pkexec | 30min | |
| `ClearCache()` | `pkexec nbc cache clear` | pkexec | 30min | State-changing |

### Streaming pattern

```go
events := nbc.Update(ctx, opts)
for event := range events {
    sgtk.RunOnMainThread(func() {
        switch event.EventType {
        case nbc.EventTypeProgress:
            progressBar.SetFraction(event.Progress / 100.0)
        case nbc.EventTypeComplete:
            // done
        }
    })
}
```

## Updex (`internal/updex/updex.go`)

Manages system features (add-on software/configuration modules). Unlike other wrappers, updex does **not** shell out to a CLI for reads. It uses the `github.com/frostyard/updex/updex` Go library directly for read operations, with a singleton `*updexapi.Client`. Write operations that require root are delegated to the `chairlift-updex-helper` binary (at `cmd/chairlift-updex-helper/main.go`) via pkexec.

### Key types

Type aliases to `github.com/frostyard/updex/updex`:
- **`Feature`** (`FeatureInfo`) — name, description, enabled flag, documentation URL
- **`FeatureCheck`** (`CheckFeaturesResult`) — feature name, update available flag, component versions
- **`CheckResult`** — component name, current/available versions

### Operations

| Function | Implementation | Mode | Timeout | Notes |
|----------|---------------|------|---------|-------|
| `IsInstalled()` | Go library: `client.Features()` | Direct | 3s | Checks if updex features are configured |
| `IsInstalledCached()` | Cached `IsInstalled()` | Direct | — | `sync.Once`, runs check at most once |
| `ListFeatures()` | Go library: `client.Features()` | Direct | 5min | Returns `[]Feature` |
| `CheckFeatures()` | Go library: `client.CheckFeatures()` | Direct | 5min | Returns `[]FeatureCheck` |
| `EnableFeature(name)` | `pkexec chairlift-updex-helper enable-feature <name>` | pkexec | 5min | State-changing |
| `DisableFeature(name)` | `pkexec chairlift-updex-helper disable-feature <name>` | pkexec | 5min | State-changing |
| `UpdateFeatures()` | `pkexec chairlift-updex-helper update` | pkexec | 5min | Downloads enabled features |

### Helper binary (`cmd/chairlift-updex-helper/main.go`)

A small standalone binary that accepts commands (`enable-feature`, `disable-feature`, `update`) and uses the updex Go library to perform privileged operations. It supports `--dry-run` and outputs JSON to stdout. It is invoked via pkexec so that the main chairlift process does not need root.

## Cross-cutting: dry-run

Every wrapper has `SetDryRun(bool)` and `IsDryRun() bool`. When dry-run is active:
- State-changing commands are skipped (return mock/empty results)
- Read-only commands still execute normally
- Set once at startup from `app.New()` based on `--dry-run` flag
- **Note**: `app.New()` calls `SetDryRun` on flatpak, homebrew, nbc, and updex. Snap defines `SetDryRun` but it is not called from `app.New()` (possible oversight — snap's `Install` does check `dryRun` internally).
