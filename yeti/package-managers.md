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

Since `trust` is one of homebrew's `stateChangingCommands`, `TrustPackages` already no-ops under dry-run at the exec layer (see "Cross-cutting: dry-run" below). But `trustTap` (`internal/views/updates_page.go`) used to always mutate the Untrusted Homebrew Taps UI on a successful (nil-error) call — removing the tap's row, hiding the group when empty, and refreshing outdated packages — even when nothing was actually trusted. That made a dry-run click visually remove the tap from the Untrusted Taps list as if it were now trusted, with no way to undo it from the UI. `trustTap` now computes `decision := actionmsg.TapTrust(homebrew.IsDryRun(), tap.Name)` once in its success branch and gates all three UI mutations on `decision.MutateUI` (exactly `!dryRun`): when true, behavior is unchanged from before; when false, the row stays, the group stays visible, `loadOutdatedPackages` is not re-queried, and the click's button is reset (`SetSensitive(true)`, `SetLabel("Trust")`) instead of being left stuck on "Trusting...". `decision.Toast` — a preview string under dry-run, the same "Trusted %s. Its packages can update again." string otherwise — is shown in both states. This mirrors the `actionmsg.MaintenanceScript`/`ScriptDecision` pattern: the UI-mutation gate itself, not just the toast wording, is what `actionmsg_test.go` asserts.

**`UntrustedTapError`** — `runBrewCommand` (`internal/homebrew/homebrew.go`) inspects failed commands' stderr for `"untrusted tap"` or `"taps are not trusted"` (`isUntrustedTapMessage`) and wraps the failure as `*UntrustedTapError` instead of the generic `Error`. Views type-switch on this to redirect users to the Untrusted Taps UI rather than showing raw brew output.

The upgrade-failure toast text adapts to whether that UI is actually available: `trustmsg.UpgradeMessage(pkgName, trustGroupAvailable bool)` (`internal/views/trustmsg`, see "View-layer toast and decision helpers" below) is called from the outdated-packages row's upgrade click handler as `trustmsg.UpgradeMessage(pkgName, uh.brewTrustGroup != nil)`. `uh.brewTrustGroup` is only ever assigned once, in `buildUpdatesPage` on the main thread before any goroutine that could read it starts, so reading it from the upgrade goroutine is race-free. When the Untrusted Homebrew Taps group exists (`brew_trust_group` enabled and built), the message points there ("see Untrusted Homebrew Taps below"); when it doesn't (group disabled, or not yet built), the message is self-contained — it states the package can't be upgraded until its tap is trusted, with no reference to "below" or the section name, since there is nothing to point to.

**Cross-group nil-safety** — `trustTap` (`internal/views/updates_page.go`) refreshes the outdated-packages list after a successful trust, since newly-trusted packages may now show as outdated. That refresh (`loadOutdatedPackages`) is gated only on `brew_trust_group`, not `brew_updates_group`, so it must tolerate `brew_updates_group` being disabled — in which case `uh.outdatedExpander` was never built and is nil. `loadOutdatedPackages` guards on `uh.outdatedExpander == nil` as its first statement, before any homebrew call or `sgtk.RunOnMainThread`, consistent with the config-driven-visibility invariant: a disabled group's widget fields stay nil, and any code reachable from another group's async callback must nil-guard before touching them.

### View-layer toast and decision helpers (`internal/views/actionmsg`, `internal/views/trustmsg`)

Two small, puregotk-free packages under `internal/views/` hold the text (and, at three call sites, the accompanying UI decision) that view handlers use once a wrapper call returns. Both follow `docs/agents/skills/gtk-headless-tests.md`'s prescribed fix: `internal/views` itself cannot host a `_test.go` (puregotk panics resolving GTK/graphene shared libraries at package init, before any test runs), so the decidable logic is extracted into a pure package and table-tested there instead.

- **`internal/views/trustmsg`** (added for issue #57) — `UpgradeMessage(pkgName string, trustGroupAvailable bool) string`, the toast shown when a Homebrew upgrade fails with an `*homebrew.UntrustedTapError`; see "Tap trust" above.
- **`internal/views/actionmsg`** (added for issue #56, this dry-run fix) — builds the toast text for every state-changing view action across the maintenance, applications, updates, and features pages, and, at the three call sites where the view also mutates a row/group/switch on success, the execute/mutate/confirm decision itself, so the same table-driven test in `actionmsg_test.go` that checks the toast also checks the gate (see "Dry-run mode" in [OVERVIEW.md](./OVERVIEW.md#dry-run-mode) for the general rule this implements). Exported surface, all added across this feature's chunks (c1-c5):
  - `ScriptDecision{Execute bool; Toast string}` + `MaintenanceScript(dryRun bool, title string) ScriptDecision` — gates whether `runMaintenanceAction` constructs and runs the configured script's `exec.Cmd` at all (c1)
  - `BundleDump(dryRun bool, path string) string` — Homebrew Brewfile dump toast (c1)
  - `Cleanup(dryRun bool, tool, output string) string` — Homebrew/Flatpak cleanup toast (c1)
  - `Install(dryRun bool, pkgName string) string` — Homebrew install toast (c2)
  - `Uninstall(dryRun bool, appID string) string` — Flatpak uninstall toast (c2)
  - `Upgrade(dryRun bool, pkgName string) string` — Homebrew per-package upgrade toast (c3)
  - `Update(dryRun bool, appID string) string` — Flatpak per-app update toast (c3)
  - `SelfUpdate(dryRun bool, tool string) string` — Homebrew self-update ("Update Homebrew" button) toast (c3)
  - `TapTrustDecision{MutateUI bool; Toast string}` + `TapTrust(dryRun bool, tapName string) TapTrustDecision` — gates whether `trustTap` removes the tap's row, hides the group, and refreshes outdated packages (c3)
  - `BootcStage(dryRun bool, staged bool) string` — bootc stage-button completion toast; string-only since the subtitle stays live in both modes and there is no mutation left to gate (c4)
  - `FeatureToggleDecision{Confirm bool; Toast string}` + `FeatureToggle(dryRun, enable bool, name string) FeatureToggleDecision` — gates whether `onFeatureToggled`'s switch confirms the flip or reverts it (c5)
  - `FeatureUpdate(dryRun bool) string` — Features page "Update" button toast (c5)

  The plain-`string` functions (`BundleDump`, `Cleanup`, `Install`, `Uninstall`, `Upgrade`, `Update`, `SelfUpdate`, `BootcStage`, `FeatureUpdate`) are correct as-is because the state-changing/no-op decision for those actions is already made and already tested one layer down, in the relevant wrapper package (`internal/homebrew`, `internal/flatpak`, `internal/bootc`, `internal/updex`) — there is nothing left for the view to gate beyond the toast wording. The three decision-struct functions exist because their call sites have no such wrapper-layer gate for the *second*, UI-side effect (script execution has no wrapper package at all; tap-trust row removal and switch confirmation are view-local state that the wrapper's own dry-run skip doesn't touch).

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

That part was already correct and already tested (`internal/bootc/stage_test.go`). What used to be wrong is downstream, in the view layer: `onBootcStageClicked` (`internal/views/updates_page.go`) always re-reads live `bootc.GetStatus()` after `StageUpdate` returns and used to show one of two completion-toned toasts — `"System update staged. Restart to apply."` or `"System is up to date"` — regardless of whether the click was a real stage or a dry-run no-op. Neither string said "preview", and `"System is up to date"` in particular read as a verified conclusion when, under dry-run, this click didn't actually check or change anything. The handler now computes `actionmsg.BootcStage(bootc.IsDryRun(), staged)` for that toast: under dry-run it returns a single, unambiguous preview string regardless of `staged` (since `staged` reflects real system state from `GetStatus`, not anything this click did); otherwise it returns the same two completion strings as before. The `bootcStageExpander` subtitle is deliberately *not* changed by this — it intentionally keeps reflecting live `GetStatus()` output in both dry-run and live mode, since the subtitle is a persistent status display (what state the system is actually in right now), not a per-click completion claim. Only the toast, which is inherently about "what did this click just do", needed the dry-run-aware text.

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

Manages system features (add-on software/configuration modules). Unlike other wrappers, updex does **not** shell out to a CLI for reads. It uses the `github.com/frostyard/updex/updex` Go library directly for read operations, with a singleton `*updexapi.Client`. Write operations that require root are delegated via pkexec to the fixed absolute path `internal/updex.HelperPath` (`/usr/bin/chairlift-updex-helper`, built from `cmd/chairlift-updex-helper/main.go`) — never a bare, `$PATH`-resolved name, since `pkexec` matches the resolved absolute path against `data/org.frostyard.ChairLift.updex.policy`'s `org.freedesktop.policykit.exec.path` annotation to select the right action; see [OVERVIEW.md](./OVERVIEW.md#privileged-operations) for the full rationale and the matching `PREFIX=/usr` Makefile requirement.

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
| `EnableFeature(name)` | `pkexec /usr/bin/chairlift-updex-helper enable-feature <name>` | pkexec | 5min | State-changing |
| `DisableFeature(name)` | `pkexec /usr/bin/chairlift-updex-helper disable-feature <name>` | pkexec | 5min | State-changing |
| `UpdateFeatures()` | `pkexec /usr/bin/chairlift-updex-helper update` | pkexec | 5min | Downloads enabled features |

### Helper binary (`cmd/chairlift-updex-helper/main.go`)

A small standalone binary that accepts commands (`enable-feature`, `disable-feature`, `update`) and uses the updex Go library to perform privileged operations. It supports `--dry-run` for all three subcommands — `enable-feature`, `disable-feature`, and `update` — passing it through to the corresponding `updex.*Options.DryRun` field. Outputs JSON to stdout. Invoked via pkexec so that the main chairlift process does not need root.

`main.go` itself is thin argv dispatch only: parsing `os.Args` and building each subcommand's `Options` struct live in `internal/updexhelper` (`internal/updexhelper/updexhelper.go`), a package with no puregotk import — only stdlib plus `github.com/frostyard/updex/updex`. That's what makes the logic testable at all: neither `gates_chunk` nor `make ci` ever runs `go test ./...`, both are scoped to `go test ./internal/...`, so a `_test.go` under `cmd/chairlift-updex-helper` would never execute under any gate this repo actually runs (see `docs/agents/skills/gtk-headless-tests.md` for the same "extract to a testable `internal/` package" pattern applied to GTK code). `internal/updexhelper` exports `HasDryRunFlag(args []string) bool` (pure — takes an args slice instead of reading `os.Args` directly) plus `EnableOptions`, `DisableOptions`, and `UpdateOptions`, each `func(dryRun bool) updex.*Options` setting `DryRun` to exactly the argument passed. `internal/updexhelper/updexhelper_test.go` table-tests all four functions, including the previously-dropped `update` case (see "Cross-cutting: dry-run" below).

## Cross-cutting: dry-run

Every wrapper has `SetDryRun(bool)` and `IsDryRun() bool`. Behavior varies by wrapper:

| Wrapper | Dry-run behavior | Called from `app.New()`? |
|---------|-----------------|------------------------|
| Homebrew | Skips state-changing commands, returns mock message | Yes |
| Flatpak | Skips state-changing commands, returns mock message | Yes |
| bootc | `StageUpdate` never invokes pkexec; emits synthetic `EventMessage`+`EventComplete` and returns. The Updates page's stage button shows an explicit `actionmsg.BootcStage(bootc.IsDryRun(), staged)` preview toast, distinct from its normal staged/up-to-date toasts; the expander subtitle intentionally stays live (from `bootc.GetStatus()`) in both modes | Yes |
| Updex | Skips helper execution, returns empty results; the helper binary itself (`cmd/chairlift-updex-helper`, via `internal/updexhelper`) also honors `--dry-run` for all three subcommands, defense-in-depth even though `updex.runHelper` never invokes pkexec under dry-run | Yes |
| views (custom maintenance scripts) | `runMaintenanceAction` never constructs an `exec.Cmd` (no `pkexec`, no direct script exec); logs `[DRY-RUN] Would execute: ...` instead | Yes |

Custom maintenance scripts (config.yml `actions` entries) have no wrapper package of their own, so `internal/views` carries its own `SetDryRun`/`IsDryRun` (`internal/views/dryrun.go`) rather than reusing one of the above. Unlike the other wrappers, the execution gate for this one is not just an `if IsDryRun()` branch inline in the view: `internal/views/actionmsg.MaintenanceScript(dryRun, title)` returns a `ScriptDecision{Execute, Toast}` computed once, before the goroutine spawns, and both the "does it execute" question and the toast text come from that single tested function call — not two independently-maintained conditionals. See "View-layer toast and decision helpers" above for the full `actionmsg`/`trustmsg` function and type list.

The Applications page's per-result Homebrew install button (`onHomebrewSearch`, `internal/views/applications_page.go`) and per-app Flatpak uninstall buttons (`loadFlatpakApplications`, both the user- and system-installation branches) show toasts built by `actionmsg.Install(homebrew.IsDryRun(), pkgName)` and `actionmsg.Uninstall(flatpak.IsDryRun(), appID)` respectively, rather than an unconditional "installed"/"uninstalled" string — the wrapper's own dry-run skip already makes `Install`/`Uninstall` a no-op, so the toast must say "would be installed/uninstalled" instead of claiming it happened. The list refresh after uninstall (`go uh.loadFlatpakApplications()`) stays unconditional since it re-queries live state either way.

The Updates page's per-package Homebrew upgrade button, per-app Flatpak update button, and the "Update Homebrew" self-update button (`internal/views/updates_page.go`) follow the same pattern: `actionmsg.Upgrade(homebrew.IsDryRun(), pkgName)`, `actionmsg.Update(flatpak.IsDryRun(), appID)`, and `actionmsg.SelfUpdate(homebrew.IsDryRun(), "Homebrew")` replace what were unconditional "upgraded"/"updated"/"updated successfully" toasts, since `upgrade` and `update` are both in their wrappers' `stateChangingCommands` and no-op under dry-run. The Flatpak update button's list refresh (`go uh.loadFlatpakUpdates()`) stays unconditional, same reasoning as the uninstall refresh above.

The Updates page's bootc "Check for Updates" stage button (`onBootcStageClicked`, `internal/views/updates_page.go`) follows the same `actionmsg` pattern, with one difference from the buttons above: unlike `Install`/`Upgrade`/etc., whose completion text is selected purely by `dryRun`, `BootcStage(dryRun, staged)` also takes the live `staged` result from the post-`wg.Wait()` `bootc.GetStatus()` re-read, because the non-dry-run branch still needs to pick between the "staged" and "up to date" strings. Under dry-run, `staged` is ignored entirely and a single preview string is returned instead — see "Dry-run behavior" under bootc above for why. The expander's `SetSubtitle` calls in the same code block are *not* routed through `actionmsg`; they keep reading live `GetStatus()` output unconditionally, since the subtitle is a persistent status display rather than a per-click completion claim.

The Features page's per-feature switch (`onFeatureToggled`, `internal/views/features_page.go`) follows the same decision-struct pattern as maintenance-script execution and tap trust: on a successful `updex.EnableFeature`/`DisableFeature` call, `decision := actionmsg.FeatureToggle(updex.IsDryRun(), enabled, name)` is computed once, and the switch's visual state is driven solely by `decision.Confirm` — `toggle.SetActive(enabled)` (confirming the flip) when `Confirm` is true, `toggle.SetActive(!enabled)` (reverting to the pre-click state) when it is false. Under dry-run, `updex.runHelper` returns before ever invoking pkexec, so nothing was actually toggled and the switch must not visually confirm a change that did not happen — this is the other "switch/list implies a state change after a preview" bug (the tap-trust row-removal case is the same pattern in Homebrew's Untrusted Taps list). The Update button (`onUpdateFeaturesClicked`) has no equivalent mutation to gate — its `SetSensitive`/`SetLabel` reset is unconditional in both modes — so its toast is a plain string, `actionmsg.FeatureUpdate(updex.IsDryRun())`.

## Install-path consistency (`internal/installcheck`)

Two installation paths ship this repository's privileged surface (the updex
helper binary and both PolicyKit policy/rules pairs): a source `make install`
(Makefile, `PREFIX` defaulting to `/usr`) and the packaged nFPM (deb/rpm/apk)
layout goreleaser builds from `.goreleaser.yaml`. Both are hand-maintained
text — a Makefile recipe and a YAML block — with no shared code path, so
nothing stops them (or `internal/updex.HelperPath`, the fixed absolute path
`pkexec` matches against the policy's `exec.path` annotation) from silently
drifting apart again the way the Makefile's old `/usr/local` default drifted
from the policy's `/usr/bin` in the bug this package now guards against.

`internal/installcheck` holds two regression tests, not production code, that
turn "verified by inspection" into a real, gated check:

- **`TestMakefileInstallUsesUsrPrefix`** runs `make -n install
  DESTDIR=<t.TempDir()>` — a dry run, so no compilation, no writes outside
  the temp dir, and no root — once with no `PREFIX` override and once with
  `PREFIX=/usr`, and asserts the printed `install -Dm...` lines place the
  updex helper at `DESTDIR` + `internal/updex.HelperPath` and both
  policy/rules pairs under `DESTDIR` + the fixed
  `/usr/share/polkit-1/{actions,rules.d}` PolicyKit reads. It shells out to
  the real `make` rather than parsing the Makefile textually because `make`
  itself is the authority on what a given `PREFIX`/`DESTDIR` combination
  actually resolves to (variable derivation, `$(DESTDIR)$(BINDIR)`
  concatenation, recipe ordering) — a hand-rolled Makefile parser would just
  be a second, divergence-prone implementation of `make`'s own substitution
  rules, and would stop being a regression test for the exact thing that
  broke (the *installed* path) the moment it disagreed with real `make`
  output.
- **`TestGoreleaserNfpmLayoutMatchesUsrPrefix`** parses the real, repo-root
  `.goreleaser.yaml` (not a fixture) with the already-vendored
  `gopkg.in/yaml.v3` and, iterating **every** `nfpms[]` entry (not just
  `nfpms[0]`, so adding or reordering a second package with the wrong layout
  still fails — per
  `docs/agents/skills/regression-tests-must-cover-every-collection-entry.md`),
  asserts each entry's `bindir` matches the directory of
  `internal/updex.HelperPath` and its updex/bootc policy+rules
  `contents[].dst` entries equal those same fixed polkit-1 paths.

Both tests fail — not skip — if `internal/updex.HelperPath`, the Makefile's
`PREFIX` default, or `.goreleaser.yaml`'s `nfpms` block change independently
of one another; each was hand-verified during development by reverting one
of the three at a time and confirming only the test(s) that source depends
on turn red. The package imports no puregotk, directly or transitively, so it
never trips `docs/agents/skills/gtk-headless-tests.md`'s constraint, and it
lives under `internal/...` so `gates_chunk`, `make ci`, and CI's identical
`go test ./internal/... -run "^Test[^I]" -skip "Integration"` filter all
exercise it on every run, per
`docs/agents/skills/gate-test-scope-is-internal-only.md` — not just the
heavier, less-frequent `make ci` deep gate.
