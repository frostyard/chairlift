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

### Tap trust (Homebrew 6) (`internal/homebrew/trust.go`)

Homebrew 6 introduced per-tap trust: formulae/casks from a tap that isn't marked trusted are invisible to normal `brew` operations. Critically, **`brew list`/`brew info` also refuse to load untrusted-tap formulae**, so there is no supported `brew` command that lists what's installed-but-untrusted — ChairLift has to reconstruct that set itself from on-disk state.

**Detection (`ListUntrustedTaps`)** combines three sources:
1. `brew tap-info --installed --json` — parsed for each tap's `name` and `trusted` flag (`parseUntrustedTapNames`); this is the only brew-provided signal, and it tells you *which taps* are untrusted but not *what's installed from them*.
2. Cellar keg receipts (`installedFormulaeByTap`) — walks `<prefix>/Cellar/<formula>/<version>/INSTALL_RECEIPT.json` and reads `.source.tap`, since brew's own listing commands can't see these formulae. One receipt per keg is enough to attribute the formula to a tap.
3. Caskroom metadata (`installedCasksByTap`) — walks `<prefix>/Caskroom/<token>/.metadata/*/*/Casks/*.json` and reads `.tap`. Glob results are lexically, not chronologically, ordered (`"9"` sorts after `"10"`), so the newest file is picked by `mtime`, not by glob order. Casks installed via the Homebrew API (no local `Casks/<token>.json`) are skipped — they belong to `homebrew/cask`, which is always trusted.

Only untrusted taps with at least one installed formula or cask are returned (`UntrustedTap{Name, Formulae, Casks}`, package names fully qualified as `tap/name`, ready to pass straight to `brew trust`); taps with nothing installed aren't actionable and are dropped.

**`TrustPackages(tap)`** runs `brew trust --formula <formulae...>` and/or `brew trust --cask <casks...>` for the given tap. This is a **per-user** operation (state lives in `~/.homebrew/trust.json`) — it does not use `pkexec` and does not require root, unlike bootc staging or updex writes.

**`UntrustedTapError`** — `runBrewCommand` (`internal/homebrew/homebrew.go`) inspects failed commands' stderr for `"untrusted tap"` or `"taps are not trusted"` (`isUntrustedTapMessage`) and wraps the failure as `*UntrustedTapError` instead of the generic `Error`. Views type-switch on this to redirect users to the Untrusted Taps UI rather than showing raw brew output.

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

## bootc (`internal/bootc/`)

Wraps `bootc` for OSTree/composefs system updates, split across two files: `bootc.go` (unprivileged status reads) and `stage.go` (privileged update staging). Deliberately does not shell out to any separate CLI helper binary or Go client library — status parsing and stage-script invocation are both implemented directly against `os/exec`.

### `GetStatus` (unprivileged)

`GetStatus(ctx)` runs `bootc status --format json` with **no** `pkexec` — this is a plain read, safe to call from any goroutine (`internal/bootc/bootc.go`). Output is unmarshaled into `Status{Spec, Status: {Booted, Staged, Rollback}}`, where each of `Booted`/`Staged`/`Rollback` is a `*Deployment` (nil-safe accessors: `ImageRef()`, `Version()`, `Timestamp()`, `Digest()`).

### Boot gate semantics

`bootc status` exits 0 with a null `booted` field on hosts that aren't running a bootc deployment at all — so the gate cannot be the exit code. `Status.Booted()` returns `s.Status.Booted != nil`. `IsBootcBooted(ctx)` calls `GetStatus` and returns that boolean (treating any error as "not booted"). `IsBootcBootedCached()` wraps it in a `sync.Once` with a 5s timeout, computing the result once and caching it for the lifetime of the process — this lets multiple view goroutines call it during async startup without triggering redundant `bootc` invocations. **Do not use `/run/ostree-booted`** as a substitute gate: it is absent on snow's composefs-based deployments, so checking for it would hide bootc UI on every snow host.

### `StageUpdate` (privileged, streaming)

`StageUpdate(ctx, progressCh)` (`internal/bootc/stage.go`) runs `pkexec /usr/libexec/bootc-update-stage`, merging stdout+stderr and streaming each trimmed non-empty line to `progressCh` as an `EventMessage`. `progressCh` is always closed before returning (`defer close`). On successful exit it sends a final `EventComplete`; on failure it returns an `Error` (including the last output line for context) or a `NotFoundError` if pkexec itself is missing. If the context is canceled/times out mid-stream, the child process is killed and reaped before returning `ctx.Err()`.

**Why a stage script instead of `bootc upgrade`:** upstream `bootc upgrade`'s registry-transport pull fails on snow's composefs images. The stage script works around this by using `podman pull` (whose pull path works) to fetch the image into containers-storage, then running `bootc switch --transport containers-storage` to stage the already-pulled image — `podman` does the pull, `bootc` does the switch. This keeps the actual workaround logic in one place (the snow-shipped script, source of truth in the snosi project) instead of duplicating pull/switch orchestration inside ChairLift. The script is idempotent: it exits 0 without staging anything when the deployment is already current, so `StageUpdate` doubles as both "check for update" and "apply update".

### Event types

- `EventMessage` — one line of stage-script output
- `EventError` — surfaced by the view layer when `StageUpdate` returns an error
- `EventComplete` — sent once, after successful completion

This is intentionally flatter than a step/percent progress model, because the stage script emits unstructured log lines, not a structured progress protocol.

### Dry-run behavior

Unlike bootc's own dry-run flag (not used here), ChairLift's dry-run mode is handled entirely inside `StageUpdate`: if `dryRun` is set, it never invokes `pkexec` at all — it logs the command that would run, sends a synthetic `EventMessage` + `EventComplete`, closes the channel, and returns `nil`.

### Operations

| Function | Command | Privilege | Timeout | Notes |
|----------|---------|-----------|---------|-------|
| `GetStatus(ctx)` | `bootc status --format json` | none | 30min (`DefaultContext`); views use the standard 30min context | JSON parsed into `Status` |
| `IsBootcBooted(ctx)` / `IsBootcBootedCached()` | (calls `GetStatus`) | none | 5s (cached variant) | Boot gate; cached variant memoizes via `sync.Once` |
| `StageUpdate(ctx, progressCh)` | `pkexec /usr/libexec/bootc-update-stage` | pkexec (`org.frostyard.ChairLift.bootc.stage`) | 30min (`DefaultContext`) | Streaming; idempotent; dry-run aware |
| `StageScriptAvailable()` | `os.Stat(StageScriptPath)` | none | — | Used to hide the updates-page group when the script isn't installed |

### Streaming pattern

```go
progressCh := make(chan bootc.ProgressEvent)
go func() {
    err := bootc.StageUpdate(ctx, progressCh)
    // channel is closed when done
}()
for event := range progressCh {
    evt := event // capture for closure
    sgtk.RunOnMainThread(func() {
        switch evt.Type {
        case bootc.EventMessage:
            // append to log expander with timestamp
        case bootc.EventError:
            // show error toast
        case bootc.EventComplete:
            // re-query GetStatus to refresh staged/booted summary
        }
    })
}
```

### Progress UI (`internal/views/updates_page.go`)

`onBootcStageClicked()` drives the updates page's "System Update" expander directly (there is a single staging operation, so no shared cross-operation helper is needed) — it disables the button, spawns `bootc.StageUpdate` in a goroutine, and processes events on a second goroutine, restoring button state and showing a toast on completion. The system page's `loadBootcStatus()` is a separate, read-only path: it calls `bootc.GetStatus` to display the booted/staged/rollback deployment images, versions, and digests, with no staging controls — staging only happens from the Updates page.

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
| bootc | `StageUpdate` never invokes pkexec; emits synthetic `EventMessage`+`EventComplete` and returns | Yes |
| Updex | Skips helper execution, returns empty results | Yes |
| Snap | Skips `Install`, returns mock changeID | No (possible oversight) |
