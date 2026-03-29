# Package Manager Wrappers

Each wrapper lives in its own package under `internal/` and follows a consistent pattern: module-level dry-run flag, availability check with cached variant (`IsInstalledCached()` using `sync.Once`), and context-based timeouts. All are called from `internal/views/` page builders. The cached availability check is important for the deferred-visibility startup pattern — multiple goroutines may check the same tool, and the result should only be computed once.

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
- **`UpdateInfo`** — appID, remoteRef, installedSize, downloadSize, newVersion
- **`ApplicationInfo`** — detailed metadata (description, developer, etc.)

### Operations

| Function | CLI command | Timeout | Notes |
|----------|------------|---------|-------|
| `ListUserApplications()` | `flatpak list --user --app --columns=...` | 60s | Tabular parsed |
| `ListSystemApplications()` | `flatpak list --system --app --columns=...` | 60s | Tabular parsed |
| `ListUpdates()` | `flatpak remote-ls --updates --columns=...` | 60s | Both user+system; includes `NewVersion` field |
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
- **`UpdateCheck`** / **`UpdateCheckOutput`** — available update details
- **`StagedUpdate`** — staged update in cache
- **`ListOutput`** / **`DiskOutput`** / **`PartitionOutput`** — disk information
- **`CacheListOutput`** / **`CachedImageMetadata`** — cached image details
- **`DownloadOutput`** — download result
- **`ValidateOutput`** — disk validation result
- **`ProgressEvent`** — step, progress percentage, messages, warnings, errors

### Event types for streaming

- `EventTypeStep` — new operation phase
- `EventTypeProgress` — percentage update
- `EventTypeMessage` — informational log line
- `EventTypeWarning` — non-fatal warning during operation
- `EventTypeError` — error during operation
- `EventTypeComplete` — operation finished

### ProgressEvent fields

| Field | Type | Used by |
|-------|------|---------|
| `Type` | `EventType` | All events — determines which case to handle |
| `Step` | `int` | `EventTypeStep` — current step number |
| `TotalSteps` | `int` | `EventTypeStep` — total number of steps |
| `StepName` | `string` | `EventTypeStep` — human-readable step description |
| `Percent` | `int` | `EventTypeProgress` — 0-100 completion percentage |
| `Message` | `string` | All types — descriptive text (progress detail, log line, warning/error text, completion summary) |

### Operations

| Function | CLI command | Mode | Timeout | Notes |
|----------|------------|------|---------|-------|
| `GetStatus()` | `nbc status --json` | Direct | 30min | JSON parsed |
| `CheckUpdate()` | `nbc check-update --json` | Direct | 30min | JSON parsed |
| `ListDisks()` | `nbc disk list --json` | Direct | 30min | JSON parsed |
| `ListCachedImages(cacheType)` | `nbc cache list --json` | Direct | 30min | JSON parsed |
| `Update(ctx, opts, progressCh)` | `pkexec nbc update --json` | Streaming | 30min | Caller provides channel; closed when done |
| `Download(ctx, opts, progressCh)` | `pkexec nbc download --json` | Streaming | 30min | Caller provides channel; closed when done |
| `Validate(ctx, device)` | `pkexec nbc disk validate --json` | pkexec | 30min | |
| `RemoveCachedImage(ctx, digest, cacheType)` | `pkexec nbc cache remove` | pkexec | 30min | State-changing |
| `ClearCache(ctx, cacheType)` | `pkexec nbc cache clear` | pkexec | 30min | State-changing |

### Streaming pattern

```go
progressCh := make(chan nbc.ProgressEvent)
go func() {
    err := nbc.Update(ctx, opts, progressCh)
    // channel is closed when done
}()
for event := range progressCh {
    evt := event // capture for closure
    sgtk.RunOnMainThread(func() {
        switch evt.Type {
        case nbc.EventTypeStep:
            progressBar.SetFraction(float64(evt.Step) / float64(evt.TotalSteps))
        case nbc.EventTypeProgress:
            progressBar.SetFraction(float64(evt.Percent) / 100.0)
        case nbc.EventTypeComplete:
            // done
        }
    })
}
```

### Shared progress UI helper (`internal/views/updates_page.go`)

The view layer consolidates NBC progress UI into a single `runNBCOperation()` method on `UserHome`. It accepts:
- `nbcOperationFunc` — type alias for `func(ctx context.Context, progressCh chan<- nbc.ProgressEvent) error`, matching the signatures of `nbc.Update` and `nbc.Download`
- `nbcOperationParams` — struct with operation-specific labels (`activeLabel`, `resetLabel`, `startSubtitle`, `completionMsg`, `successToast`, `failurePrefix`) and an optional `onFinished` callback

The helper creates a progress bar row and a log expander inside the given `ExpanderRow`, spawns goroutines for the operation and event processing, handles all six event types with appropriate icons and formatting, and restores button state with success/failure toasts on completion. `onNBCUpdateClicked` and `onNBCDownloadClicked` are thin wrappers that call `runNBCOperation` with operation-specific parameters and options.

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
