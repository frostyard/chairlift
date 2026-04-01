# Package Manager Wrappers

Each wrapper lives in its own package under `internal/` and follows a consistent pattern: module-level dry-run flag, availability check with cached variant (`IsInstalledCached()` using `sync.Once`), and context-based timeouts. All are called from `internal/views/` page builders. The cached availability check is important for the deferred-visibility startup pattern — multiple goroutines may check the same tool, and the result should only be computed once.

## Homebrew (`internal/homebrew/homebrew.go`)

Wraps the `brew` CLI. Uses JSON output (`--json=v2`) for structured data where available.

### Key types

- **`Package`** — name, version, pinned status, outdated flag, `InstalledOnRequest` bool, `Dependencies` string slice (struct field exists but not populated by current parsing)
- **`SearchResult`** — name, description, homepage (only `Name` is populated by `Search()` — description and homepage fields exist but are always empty since search parses text output)

### Operations

| Function | CLI command | Timeout | Notes |
|----------|------------|---------|-------|
| `ListInstalledFormulae()` | `brew info --installed --json=v2 --formula` | 30s | JSON parsed |
| `ListInstalledCasks()` | `brew info --installed --json=v2 --cask` | 30s | JSON parsed |
| `ListOutdated()` | `brew outdated --json=v2` | 30s | JSON parsed; returns both formulae and casks |
| `Search(query)` | `brew search --formula <query>` | 30s | Text output parsed; formula-only search |
| `Install(name, isCask)` | `brew install [--cask] <name>` | 30s | State-changing, dry-run aware |
| `Uninstall(name, isCask)` | `brew uninstall [--cask] <name>` | 30s | State-changing |
| `Upgrade(name)` | `brew upgrade [<name>]` | 30s | State-changing; empty name upgrades all |
| `Update()` | `brew update` | 30s | State-changing |
| `Pin(name)` / `Unpin(name)` | `brew pin/unpin <name>` | 30s | State-changing |
| `Cleanup()` | `brew cleanup` | 30s | State-changing; returns output string |
| `BundleDump(path, force)` | `brew bundle dump [--file=<path>] [--force]` | 30s | State-changing; writes to file path |
| `BundleInstall(path)` | `brew bundle install [--file=<path>]` | 30s | State-changing |

### State-changing commands

The `stateChangingCommands` map includes: `install`, `uninstall`, `remove`, `upgrade`, `update`, `pin`, `unpin`, `bundle`, `cleanup`. When dry-run is active, these are skipped entirely and return a mock message.

### Error handling

Returns `Error` (wraps stderr message) or `NotFoundError` for missing Homebrew. Timeouts produce a specific error message.

## Flatpak (`internal/flatpak/flatpak.go`)

Wraps the `flatpak` CLI. Parses tabular (tab-delimited, falling back to whitespace) output.

### Key types

- **`Application`** — name, applicationID, version, branch, origin, installation (user/system), ref
- **`UpdateInfo`** — name, applicationID, newVersion, branch, origin, installation
- **`ApplicationInfo`** — embeds `Application`, adds description, runtime, permissions map

### Operations

| Function | CLI command | Timeout | Notes |
|----------|------------|---------|-------|
| `ListUserApplications()` | `flatpak list --user --app --columns=name,application,version,branch,origin,ref` | 60s | Tabular parsed |
| `ListSystemApplications()` | `flatpak list --system --app --columns=name,application,version,branch,origin,ref` | 60s | Tabular parsed |
| `ListUpdates(user)` | `flatpak remote-ls --updates --columns=name,application,version,branch,origin [--user\|--system]` | 60s | Separate calls for user/system |
| `Install(appID, user)` | `flatpak install -y [--user\|--system] <appID>` | 60s | State-changing |
| `Uninstall(appID, user)` | `flatpak uninstall -y [--user\|--system] <appID>` | 60s | State-changing |
| `Update(appID, user)` | `flatpak update -y [--user\|--system] [<appID>]` | 60s | State-changing; empty appID updates all |
| `UninstallUnused()` | `flatpak uninstall --unused -y` | 60s | Maintenance cleanup |
| `Info(appID, user)` | `flatpak info --show-metadata [--user\|--system] <appID>` | 60s | Key-value parsed |
| `GetRemotes(user)` | `flatpak remotes --columns=name [--user\|--system]` | 60s | Lists configured remotes |

### State-changing commands

`install`, `uninstall`, `remove`, `update`. When dry-run is active, these are skipped entirely.

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
| `DefaultContext()` | — | 60s | Helper: returns context with default timeout |

### Notable

- Uses snapd socket at `/run/snapd.socket`
- Snap install is async — returns a changeID that must be polled
- Supports interactive polkit authentication
- Handles `ErrNoSnapsInstalled` gracefully (returns empty list)
- `SetDryRun` is defined but not called from `app.New()` — snap's `Install` checks `dryRun` internally

## NBC (`internal/nbc/nbc.go`)

Wraps the `nbc` CLI for bootc (OSTree-based) system updates. The most complex wrapper due to streaming progress and privilege requirements.

### Execution modes

1. **Direct** (`runNbcCommandDirect`) — read-only queries (`nbc status`, `nbc check-update`, `nbc disk list`); always prepends `--json`; no pkexec
2. **pkexec** (`runNbcCommand`) — privileged non-streaming operations (`pkexec nbc cache list`, `pkexec nbc validate`); always prepends `--json`
3. **Streaming** (`runNbcCommandStreaming`) — privileged operations with line-by-line JSON progress output (`pkexec nbc update`, `pkexec nbc download`); always prepends `--json`

### Dry-run behavior

Unlike other wrappers, NBC does **not** skip execution in dry-run mode. Instead:
- `runNbcCommand` and `runNbcCommandStreaming` append `--dry-run` to the args for state-changing commands, delegating dry-run behavior to the `nbc` binary itself
- `Update()` and `Download()` also prepend their own `--dry-run` flag before calling the streaming runner

### State-changing detection

Two maps control this:
- `stateChangingCommands`: `install`, `update`, `download`
- `stateChangingCacheSubcommands`: `clear`, `remove` (checked when first arg is `cache`)

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

Locally defined option structs:
- **`UpdateOptions`** — Image, Device, Force, DownloadOnly, LocalImage, Auto, SkipPull, KernelArgs
- **`DownloadOptions`** — Image, ForInstall, ForUpdate
- **`InstallOptions`** — defined but no `Install()` function exists yet (Image, LocalImage, Device, Filesystem, Encrypt, Passphrase, KeyFile, TPM2, KernelArgs, RootPwdFile, SkipPull)

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
| `GetStatus()` | `nbc status` | Direct | 30min | JSON parsed |
| `CheckUpdate()` | `nbc check-update` | Direct | 30min | JSON parsed |
| `ListDisks()` | `nbc disk list` | Direct | 30min | JSON parsed |
| `ListCachedImages(cacheType)` | `pkexec nbc cache list [--install-images\|--update-images]` | pkexec | 30min | Requires elevated privileges |
| `Update(ctx, opts, progressCh)` | `pkexec nbc update` | Streaming | 30min | Caller provides channel; closed when done |
| `Download(ctx, opts, progressCh)` | `pkexec nbc download` | Streaming | 30min | Requires `ForInstall` or `ForUpdate` to be true |
| `Validate(ctx, device)` | `pkexec nbc validate --device <device>` | pkexec | 30min | Errors are expected — tries to parse JSON error response |
| `RemoveCachedImage(ctx, digest, cacheType)` | `pkexec nbc cache remove <digest> [--type <cacheType>]` | pkexec | 30min | State-changing |
| `ClearCache(ctx, cacheType)` | `pkexec nbc cache clear [--install\|--update]` | pkexec | 30min | State-changing |

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

A small standalone binary that accepts commands (`enable-feature`, `disable-feature`, `update`) and uses the updex Go library to perform privileged operations. It supports `--dry-run` for `enable-feature` and `disable-feature` (the `update` command does not pass dry-run). Outputs JSON to stdout. Invoked via pkexec so that the main chairlift process does not need root.

## Cross-cutting: dry-run

Every wrapper has `SetDryRun(bool)` and `IsDryRun() bool`. Behavior varies by wrapper:

| Wrapper | Dry-run behavior | Called from `app.New()`? |
|---------|-----------------|------------------------|
| Homebrew | Skips state-changing commands, returns mock message | Yes |
| Flatpak | Skips state-changing commands, returns mock message | Yes |
| NBC | Passes `--dry-run` flag to nbc binary (commands still execute) | Yes |
| Updex | Skips helper execution, returns empty results | Yes |
| Snap | Skips `Install`, returns mock changeID | No (possible oversight) |
