# Package Manager Wrappers

Each wrapper lives in its own package under `internal/` and follows a consistent pattern: module-level dry-run flag, availability check with cached variant (`IsInstalledCached()` using `sync.Once`), and context-based timeouts in two classes — 30s for read-only commands, 30m for state-changing ones, selected per invocation by each package's `commandTimeout(args)` helper from its `stateChangingCommands` map. All are called from `internal/views/` page builders. The cached availability check is important for the deferred-visibility startup pattern — multiple goroutines may check the same tool, and the result should only be computed once.

## Homebrew (`internal/homebrew/homebrew.go`)

Wraps the `brew` CLI. Uses JSON output (`--json=v2`) for structured data where available.

### Key types

- **`Package`** — name, version, pinned status, outdated flag, `InstalledOnRequest` bool, `Dependencies` string slice (struct field exists but not populated by current parsing)
- **`SearchResult`** — name plus a required `PackageKind` (`Formula` or
  `Cask`). The kind is not cosmetic: it selects whether installation adds
  `--cask`.

### Operations

| Function | CLI command | Timeout | Notes |
|----------|------------|---------|-------|
| `ListInstalledFormulae()` | `brew info --installed --json=v2 --formula` | 30s | JSON parsed |
| `ListInstalledCasks()` | `brew info --installed --json=v2 --cask` | 30s | JSON parsed |
| `ListOutdated()` | `brew outdated --json=v2` | 30s | JSON parsed; returns both formulae and casks |
| `Search(query)` | `brew search --formula <query>`, then `brew search --cask <query>` | 30s each | Text output parsed into typed formula/cask results; one namespace's normal no-match exit is treated as empty |
| `Install(name, isCask)` | `brew install [--cask] <name>` | 30m | State-changing, dry-run aware |
| `Uninstall(name, isCask)` | `brew uninstall [--cask] <name>` | 30m | State-changing, dry-run aware |
| `Upgrade(name)` | `brew upgrade [<name>]` | 30m | State-changing; empty name upgrades all |
| `Update()` | `brew update` | 30m | State-changing |
| `Pin(name)` / `Unpin(name)` | `brew pin/unpin <name>` | 30m | State-changing, dry-run aware |
| `Cleanup()` | `brew cleanup` | 30m | State-changing; returns output string |
| `BundleDump(path, force)` | `brew bundle dump [--file=<path>] [--force]` | 30m | State-changing; writes to file path |
| `BundleInstall(path)` | `brew bundle install [--file=<path>]` | 30m | State-changing, dry-run aware |
| `AvailableBundles(paths)` | none | — | Discovers immediate `*.Brewfile` entries from every configured directory |

### State-changing commands

The `stateChangingCommands` map has exactly ten keys: `install`, `uninstall`, `remove`, `upgrade`, `update`, `pin`, `unpin`, `bundle`, `cleanup`, `trust`. The map now drives two things. First, when dry-run is active these commands are skipped entirely and return a mock message. Second, `commandTimeout(args []string)` — a pure selector reading only `args` and this map — returns `mutationTimeout` (30 minutes) when `args[0]` is one of the ten keys and `readTimeout` (30 seconds) otherwise, including for empty args; `runBrewCommand` passes its result to `context.WithTimeout`. Read commands therefore keep the old 30-second budget while installs, upgrades and bundle operations — which download and build — get 30 minutes. `homebrew_test.go`'s `TestCommandTimeout` iterates the real map and asserts `len(stateChangingCommands) == 10`, so a newly added key cannot go untested.

### Configured Brew bundle discovery

`AvailableBundles(paths []string) ([]Bundle, error)`
(`internal/homebrew/bundles.go`) does no Homebrew invocation. It resolves each
non-empty configured directory to an absolute path and scans only its immediate
entries whose names end exactly in `.Brewfile`. A candidate must resolve to a
regular file. Its display name is the filename without `.Brewfile`, its
absolute path is retained for `brew bundle install --file=...`, and a
first-line `#` comment becomes its optional description. Reading the first
line is bounded to 64 KiB, so a malformed file cannot force an unbounded
description allocation.

The outcomes are deliberately lossless and deterministic:

- a configured directory that does not exist contributes no rows and no
  error, allowing one config to name paths for several distribution variants;
- an empty path, unreadable/non-directory configured path, broken candidate,
  or unreadable Brewfile contributes a joined diagnostic, while bundles from
  other readable paths are still returned;
- a readable directory with no immediate `*.Brewfile` regular files
  contributes no rows and no error;
- repeating the same cleaned absolute file path contributes one row;
- same-named Brewfiles from different directories both remain visible, sorted
  by name and then absolute path, so configuration order never silently hides
  a distinct bundle.

`loadBrewBundles` on the Applications page calls discovery from a worker
goroutine and applies every widget change through one
`sgtk.RunOnMainThread` closure. `brew_bundles_group` is independent of
`brew_group`, so this path neither reads nor refreshes the formulae/casks
expanders. A successful live `BundleInstall` leaves the clicked row labelled
`Installed` and permanently insensitive. A failed install restores the
`Install` action. A successful dry-run uses
`actionmsg.BundleInstall(...).Complete == false`, shows an explicit preview,
and restores the action because nothing was installed. Each row owns a
`bundleview.InstallGate`, so a second callback cannot overlap a running
install even if invoked independently of GTK's insensitive-button guard.

### Typed search and install

`Search` trims the query and delegates to the pure injected seam
`searchWith(run, query)`. It queries formulae and casks separately because
Homebrew's combined human-readable search output does not reliably label every
result, while `--formula` and `--cask` make the namespace unambiguous. Homebrew
exits non-zero when one namespace has no matches; only its specific
`No formulae or casks found` diagnostic becomes an empty category. Other
errors remain failures. `homebrew_test.go` drives a fake runner to assert both
argv sequences, type preservation, empty-category handling, error
propagation, blank-query behavior, and header filtering.

`onHomebrewSearch` assigns each query a `searchRefresh` generation so an older
slow query cannot replace newer results. Rows display `Formula` or `Cask`, and
the confirmation callback carries that type into
`homebrew.Install(result.Name, result.Kind == homebrew.Cask)`. Each result owns
an `actionstate.Gate`: cancel resets it, confirmation changes the button to
`Installing...`, failure and dry-run restore it, and a live success completes
it as `Installed`. `actionstate.PackageInstall` is the tested authority for
restore/complete/refresh decisions. Live success starts
`loadHomebrewPackages`; that loader has its own `brewPackagesRefresh`
generation, nil-guards the independently configurable installed-package
group, and clear-then-repopulates separate formula/cask `rowset.Tracker`
values.

### Installed Homebrew actions

Each installed formula row has Pin/Unpin and Uninstall controls; each cask row
has Uninstall. A formula's pinned state is visible in both its subtitle and
the Pin/Unpin label. The row shares one `actionstate.Gate` across all of its
controls, so a pin and uninstall cannot overlap. Every action requires an
Adwaita confirmation: pin/unpin is suggested, while uninstall is destructive
and identifies whether the target is a formula or cask.

After confirmation, all controls on the row become insensitive and the
primary control shows `Pinning...`, `Unpinning...`, or `Uninstalling...`;
only then does a worker goroutine call `homebrew.Pin`, `Unpin`, or
`Uninstall(name, isCask)`. `actionstate.PackagePin` and
`PackageUninstall` enumerate the outcomes. Failure restores the original
controls and reports the error, and dry-run success restores them with an
explicit preview toast. Live success completes the old controls and starts
the generation-guarded `loadHomebrewPackages` refresh. The refresh, rather
than the action callback, owns row replacement, so overlapping loads cannot
publish stale installed state.

### Error handling

`runBrewCommand` is a thin wrapper: it applies the dry-run skip (before any `exec.Cmd` exists), builds a context from `commandTimeout(args)`, and delegates to the unexported `runBrewCommandAt(ctx context.Context, exe string, args ...string) (string, error)`, always passing `"brew"`. The executable path and context are parameters purely so `runner_test.go` can drive a `#!/bin/sh` script from `t.TempDir()` and control the deadline — the same seam `runStageStreaming` gives `internal/bootc`. No exported function takes a `context.Context`: callers get deadlines, not cancellation.

`runBrewCommandAt` starts the command in its own process group (`cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`) and sets `cmd.Cancel` to `syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)`, so brew's helper processes (git, curl, download workers) are killed with the command instead of being orphaned when only the direct child is signalled. `cmd.WaitDelay` (5s) bounds the wait, because those helpers inherit the stdout/stderr pipes and a straggler would otherwise hold `Wait` open indefinitely. `cmd.Run` still reaps the child.

Failures classify into exactly five distinct outcomes, checked in this order — `errors.Is` against `context.DeadlineExceeded`/`context.Canceled`, never `==` on `ctx.Err()`, so a wrapped cause still classifies:

| Condition | Result |
|-----------|--------|
| The context deadline expired (`commandTimeout(args)` elapsed) | `*Error`, message `Command '<exe> <args>' timed out`, unwrapping to `context.DeadlineExceeded` |
| The context was cancelled by its owner | `*Error`, message `Command '<exe> <args>' was canceled`, unwrapping to `context.Canceled` |
| The command exited non-zero and its stderr matches `isUntrustedTapMessage` | `*UntrustedTapError` carrying the stderr text (see "Tap trust" below) |
| The command exited non-zero otherwise | `*Error` carrying the stderr text, unwrapping to the `*exec.ExitError` |
| The executable is missing (`exec.ErrNotFound` for a bare name, `fs.ErrNotExist` for an explicit path) | `*NotFoundError` ("Homebrew not found…") |

A deadline and a cancellation never produce the same message, and neither surfaces as `signal: killed` — the process is killed by ChairLift's own `Cancel` func, so the raw wait error is replaced by the classified one. `Error` carries an `Err error` field and an `Unwrap() error` method, so `errors.Is(err, context.DeadlineExceeded)` works for callers while `Error` keeps satisfying `error` and keeps its human-readable `Message`; nothing in the views switches on its concrete type. `internal/homebrew/runner_test.go` covers each outcome against a fake script, asserts the deadline and cancellation messages differ and contain no `signal: killed`, and `TestRunBrewCommandAtKillsProcessGroup` proves the process-group kill by having the fake script spawn a background `sleep`, recording its PID, and polling `syscall.Kill(pid, 0)` until it returns `ESRCH` — proving the helper is gone, not merely that the parent returned.

### Tap trust (Homebrew 6) (`internal/homebrew/trust.go`)

Homebrew 6 introduced per-tap trust: formulae/casks from a tap that isn't marked trusted are invisible to normal `brew` operations. Critically, **`brew list`/`brew info` also refuse to load untrusted-tap formulae**, so there is no supported `brew` command that lists what's installed-but-untrusted — ChairLift has to reconstruct that set itself from on-disk state.

**Detection (`ListUntrustedTaps`)** combines three sources:
1. `brew tap-info --installed --json` — parsed for each tap's `name` and `trusted` flag (`parseUntrustedTapNames`); this is the only brew-provided signal, and it tells you *which taps* are untrusted but not *what's installed from them*.
2. Cellar keg receipts (`installedFormulaeByTap`) — walks `<prefix>/Cellar/<formula>/<version>/INSTALL_RECEIPT.json` and reads `.source.tap`, since brew's own listing commands can't see these formulae. One receipt per keg is enough to attribute the formula to a tap.
3. Caskroom metadata (`installedCasksByTap`) — walks `<prefix>/Caskroom/<token>/.metadata/*/*/Casks/*.json` and reads `.tap`. Glob results are lexically, not chronologically, ordered (`"9"` sorts after `"10"`), so the newest file is picked by `mtime`, not by glob order. Casks installed via the Homebrew API (no local `Casks/<token>.json`) are skipped — they belong to `homebrew/cask`, which is always trusted.

Only untrusted taps with at least one installed formula or cask are returned (`UntrustedTap{Name, Formulae, Casks}`, package names fully qualified as `tap/name`, ready to pass straight to `brew trust`); taps with nothing installed aren't actionable and are dropped.

**`TrustPackages(tap)`** runs `brew trust --formula <formulae...>` and/or `brew trust --cask <casks...>` for the given tap. This is a **per-user** operation (state lives in `~/.homebrew/trust.json`) — it does not use `pkexec` and does not require root, unlike bootc staging or updex writes.

Because `brew trust --formula/--cask ...` has `args[0] == "trust"`, and `trust` is one of homebrew's ten `stateChangingCommands`, `TrustPackages` runs under the 30-minute mutation timeout rather than the 30-second read budget — trusting a tap can trigger substantial work — and, by the same map membership, already no-ops under dry-run at the exec layer (see "Cross-cutting: dry-run" below). But `trustTap` (`internal/views/updates_page.go`) used to always mutate the Untrusted Homebrew Taps UI on a successful (nil-error) call — removing the tap's row, hiding the group when empty, and refreshing outdated packages — even when nothing was actually trusted. That made a dry-run click visually remove the tap from the Untrusted Taps list as if it were now trusted, with no way to undo it from the UI. `trustTap` now computes `decision := actionmsg.TapTrust(homebrew.IsDryRun(), tap.Name)` once in its success branch and gates all three UI mutations on `decision.MutateUI` (exactly `!dryRun`): when true, behavior is unchanged from before; when false, the row stays, the group stays visible, `loadOutdatedPackages` is not re-queried, and the click's button is reset (`SetSensitive(true)`, `SetLabel("Trust")`) instead of being left stuck on "Trusting...". `decision.Toast` — a preview string under dry-run, the same "Trusted %s. Its packages can update again." string otherwise — is shown in both states. This mirrors the `actionmsg.MaintenanceScript`/`ScriptDecision` pattern: the UI-mutation gate itself, not just the toast wording, is what `actionmsg_test.go` asserts.

**`UntrustedTapError`** — `runBrewCommandAt` (`internal/homebrew/homebrew.go`) inspects failed commands' stderr for `"untrusted tap"` or `"taps are not trusted"` (`isUntrustedTapMessage`) and returns `*UntrustedTapError` instead of the generic `Error` — only on the non-zero-exit path, and only after the deadline and cancellation branches have been ruled out, so a timed-out or cancelled command is never misreported as a trust problem. The type is unchanged and unwrapped by the classification, so the one type-based dependency in the whole UI — `errors.As(err, &trustErr)` at `internal/views/updates_page.go:298-300` — keeps working and redirects users to the Untrusted Taps UI rather than showing raw brew output.

The upgrade-failure toast text adapts to whether that UI is actually available: `trustmsg.UpgradeMessage(pkgName, trustGroupAvailable bool)` (`internal/views/trustmsg`, see "View-layer toast and decision helpers" below) is called from the outdated-packages row's upgrade click handler as `trustmsg.UpgradeMessage(pkgName, uh.brewTrustGroup != nil)`. `uh.brewTrustGroup` is only ever assigned once, in `buildUpdatesPage` on the main thread before any goroutine that could read it starts, so reading it from the upgrade goroutine is race-free. When the Untrusted Homebrew Taps group exists (`brew_trust_group` enabled and built), the message points there ("see Untrusted Homebrew Taps below"); when it doesn't (group disabled, or not yet built), the message is self-contained — it states the package can't be upgraded until its tap is trusted, with no reference to "below" or the section name, since there is nothing to point to.

**Cross-group nil-safety** — `trustTap` (`internal/views/updates_page.go`) refreshes the outdated-packages list after a successful trust, since newly-trusted packages may now show as outdated. That refresh (`loadOutdatedPackages`) is gated only on `brew_trust_group`, not `brew_updates_group`, so it must tolerate `brew_updates_group` being disabled — in which case `uh.outdatedExpander` was never built and is nil. `loadOutdatedPackages` guards on `uh.outdatedExpander == nil` as its first statement, before any homebrew call or `sgtk.RunOnMainThread`, consistent with the config-driven-visibility invariant: a disabled group's widget fields stay nil, and any code reachable from another group's async callback must nil-guard before touching them.

### View-layer page presentation (`internal/views/pageview`)

`internal/views/pageview` is one of the nine puregotk-free leaf packages under
`internal/views/`. It owns the widget-independent presentation decisions shared
by all six page builders. The GTK files create and mutate widgets, but no longer
reimplement the variable row text, status text, Help-link inventory, os-release
parsing, or maintenance invocation that this package returns.

Its exported outcomes are:

- `FlatpakApplication` returns the application ID alone when no version is
  known and `ID (version)` otherwise; `HomebrewPackage` returns the version
  alone or appends the pinned marker; `BrewBundle` returns the path alone or
  `description — path`; and `SearchResult` preserves the result's typed
  Formula/Cask label.
- `UntrustedTap` combines formulae and casks in that order, strips each
  tap-qualified package prefix for display, and includes the installed count.
  `FlatpakUpdate` always includes the application ID, adds the version arrow
  only when a new version exists, and adds the user-installation suffix only
  for user updates.
- `BootcUpdateSubtitle` distinguishes not staged, staged without a version,
  and staged with a version. `BootcStageResultSubtitle` returns that staged
  text after a staged action, otherwise preserves the stage script's final
  message when one exists and falls back to `System is up to date`.
- `Feature` maps the updex description/name to title/subtitle, while
  `FeatureGroupDescription` formats the loaded feature count.
- `HelpResources` emits Website, Report Issues, and Community Discussions in
  that fixed order while omitting every unconfigured URL.
- `MaintenanceCommand` returns a direct script invocation for an
  unprivileged action and the exact `pkexec <script>` shape for a privileged
  action.
- `ParseOSRelease` ignores comments, blank lines, and lines without `=`;
  splits a retained line at its first `=`; removes quote characters at the
  value's edges;
  title-cases the key; marks `*URL` fields as links; and returns scanner
  failures. `ShortDigest` leaves digests of 19 characters or fewer unchanged
  and truncates longer values to 19 characters plus an ellipsis.

`pageview_test.go` calls every exported function directly and table-tests every
branch above. `wiring_test.go` inventories all six page files and requires each
to call its corresponding `pageview` functions while rejecting the retired
inline implementations. This supplies headless enforcement without adding a
test binary to the puregotk-importing parent package.

### View-layer toast and decision helpers (`internal/views/actionmsg`, `internal/views/trustmsg`)

Two of the nine small, puregotk-free packages under `internal/views/` (the others are `internal/views/actionstate`, `internal/views/badgestate`, `internal/views/bundleview`, `internal/views/rowset`, `internal/views/flatpakstatus`, `internal/views/featurestatus` and `internal/views/pageview`, each documented in its own subsection) hold the text and, at four call sites, the accompanying UI decision that view handlers use once a wrapper call returns. Both follow `docs/agents/skills/gtk-headless-tests.md`'s prescribed fix: `internal/views` itself cannot host a `_test.go` (puregotk panics resolving GTK/graphene shared libraries at package init, before any test runs), so the decidable logic is extracted into a pure package and table-tested there instead.

- **`internal/views/trustmsg`** (added for issue #57) — `UpgradeMessage(pkgName string, trustGroupAvailable bool) string`, the toast shown when a Homebrew upgrade fails with an `*homebrew.UntrustedTapError`; see "Tap trust" above.
- **`internal/views/actionmsg`** (added for issue #56 and extended for issue #8) — builds the toast text for every state-changing view action across the maintenance, applications, updates, and features pages, and, at the four call sites where the view also mutates a row/group/switch on success, the execute/complete/mutate/confirm decision itself, so the same table-driven test in `actionmsg_test.go` that checks the toast also checks the gate (see "Dry-run mode" in [OVERVIEW.md](./OVERVIEW.md#dry-run-mode) for the general rule this implements). Exported surface:
  - `ScriptDecision{Execute bool; Toast string}` + `MaintenanceScript(dryRun bool, title string) ScriptDecision` — gates whether `runMaintenanceAction` constructs and runs the configured script's `exec.Cmd` at all (c1)
  - `BundleDump(dryRun bool, path string) string` — Homebrew Brewfile dump toast (c1)
  - `BundleInstallDecision{Complete bool; Toast string}` + `BundleInstall(dryRun bool, name string) BundleInstallDecision` — completes a successfully installed bundle row in live mode, or resets it after a dry-run preview (issue #8)
  - `Cleanup(dryRun bool, tool, output string) string` — Homebrew/Flatpak cleanup toast (c1)
  - `Install(dryRun bool, pkgName string) string` — Homebrew install toast (c2)
  - `Uninstall(dryRun bool, name string) string` — Homebrew or Flatpak uninstall toast (c2)
  - `Pin(dryRun bool, name string, pin bool) string` — Homebrew formula pin/unpin toast (c2)
  - `Upgrade(dryRun bool, pkgName string) string` — Homebrew per-package upgrade toast (c3)
  - `Update(dryRun bool, appID string) string` — Flatpak per-app update toast (c3)
  - `SelfUpdate(dryRun bool, tool string) string` — Homebrew self-update ("Update Homebrew" button) toast (c3)
  - `TapTrustDecision{MutateUI bool; Toast string}` + `TapTrust(dryRun bool, tapName string) TapTrustDecision` — gates whether `trustTap` removes the tap's row, hides the group, and refreshes outdated packages (c3)
  - `BootcStage(dryRun bool, staged bool) string` — bootc stage-button completion toast; string-only since the subtitle stays live in both modes and there is no mutation left to gate (c4)
  - `FeatureToggleDecision{Confirm bool; Toast string}` + `FeatureToggle(dryRun, enable bool, name string) FeatureToggleDecision` — gates whether `onFeatureToggled`'s switch confirms the flip or reverts it (c5)
  - `FeatureUpdate(dryRun bool) string` — Features page "Update" button toast (c5)

  The plain-`string` functions (`BundleDump`, `Cleanup`, `Install`, `Uninstall`, `Pin`, `Upgrade`, `Update`, `SelfUpdate`, `BootcStage`, `FeatureUpdate`) select toast wording only. Where an application action also changes row controls or requests an inventory refresh, the separate tested `actionstate` decision owns that UI-side effect. The four decision-struct functions in `actionmsg` exist because their call sites have no wrapper- or `actionstate`-level gate for the *second* effect: script execution has no wrapper package; a bundle row must distinguish a real completion from a dry-run wrapper success; tap-trust row removal and switch confirmation are view-local state that the wrapper's own dry-run skip does not touch.

### View-layer update action state (`internal/views/actionstate`)

`internal/views/actionstate` is one of the nine puregotk-free leaf packages
under `internal/views`. It owns the state machines and complete outcome tables
for the Applications and Updates pages' Homebrew mutation controls:

- `Gate.TryStart` atomically moves idle to running and rejects every repeated
  callback while running; `Reset` makes a failed, previewed, or fully-refreshed
  action retryable; `Complete` permanently closes a live-upgraded row action.
- `RefreshGate.Begin` assigns an increasing generation to each metadata
  refresh and `IsCurrent` accepts only the newest, preventing a slower old
  query from publishing after a newer one.
- `PackageUpgrade(succeeded, dryRun)` returns exactly three outcomes: failure
  restores the control without changing rows; dry-run success also restores
  it without a refresh; live success requests both immediate row removal and
  a full outdated-metadata refresh.
- `PackageInstall`, `PackageUninstall`, and `PackagePin` share the installed
  inventory mutation outcomes: failure and dry-run success restore the row
  controls without a refresh; live success completes the old controls and
  requests a generation-guarded installed-package refresh.
- `MetadataUpdate(succeeded, dryRun)` returns exactly three outcomes: failure
  and dry-run success restore the top-level control without refreshing; live
  success requests a refresh and deliberately does not restore the control
  until that refresh completes.
- `OutdatedRefresh(succeeded, currentCount, discoveredCount)` returns exactly
  two outcomes: failure keeps `currentCount` and does not authorize row
  replacement; success authorizes replacement and adopts `discoveredCount`,
  including zero.
- `OutdatedPresentation(count)` returns `0 packages available` with expansion
  disabled for zero, `1 package available` with expansion enabled for one,
  and `%d packages available` with expansion enabled for larger counts.

`actionstate_test.go` table-tests every outcome, races 64 callers against one
action gate (requiring exactly one acquisition), and proves 64 concurrent
refresh requests receive unique generations with exactly one current.
`wiring_test.go` and `applications_wiring_test.go` statically check the
puregotk-importing views use those decisions, confirmation/progress states,
shared gates, row removal, count decrement, versioned refresh callbacks, and
clear/add bookkeeping; no `_test.go` is added to `internal/views`.

### View-layer update badge state (`internal/views/badgestate`)

`internal/views/badgestate` is one of the nine puregotk-free leaf packages
under `internal/views`. `Counts` replaces the three independent integer fields
that previously lived on `UserHome` with one mutex-protected owner for Bootc,
Flatpak, and Homebrew update counts. `Set(source, count)` models a completed
provider refresh and replaces that provider's prior value; `Add(source,
delta)` models an immediate row-level change such as a successful Homebrew
upgrade. Both clamp negative results to zero and return an atomic
`Snapshot{Count, Total}`. `Get` and `Total` provide locked reads.

The view still performs widget mutation through `sgtk.RunOnMainThread`; the
leaf package owns only integers and synchronization. `badgestate_test.go`
proves the zero value, multi-provider totals, replacement rather than
accumulation across repeated refreshes, decrement/clamping behavior, unknown
source rejection, and concurrent changes under the race detector.
`wiring_test.go` verifies `views.go` and `updates_page.go` route all three
providers and the displayed total through this owner, and rejects the retired
independent count fields.

### View-layer Brew bundle state (`internal/views/bundleview`)

`internal/views/bundleview` is one of the nine puregotk-free leaf packages
under `internal/views`. It owns the bundle group's load presentation and its
per-row concurrency state, leaving `applications_page.go` to construct and
update widgets only.

`Present(count, warning, homebrewAvailable) Presentation` enumerates the
group-level outcomes: zero bundles without a warning produces the
`No bundles available` placeholder; zero with a warning produces the
`Bundles unavailable` placeholder carrying that warning; one bundle uses a
singular available description; several use a plural description; partial
results append the warning while keeping their rows; and any of those states
with Homebrew unavailable appends that install actions are disabled. The view
composes none of this group/placeholder text itself.

`InstallGate` is zero-value-ready and has three states. `TryStart` atomically
moves idle to running and rejects every concurrent caller; `Reset` returns a
failed or dry-run action to idle; `Complete` permanently closes a live
successful action. `bundleview_test.go` races 64 callers against one gate and
asserts exactly one acquisition, then separately covers reset and completion.
The type has no GTK dependency and the callback still performs every actual
button mutation on the main thread.

### View-layer row bookkeeping (`internal/views/rowset`)

`internal/views/rowset` is one of the nine puregotk-free leaf packages under `internal/views/` (its siblings are `internal/views/actionmsg`, `internal/views/actionstate`, `internal/views/badgestate`, `internal/views/bundleview`, `internal/views/trustmsg`, `internal/views/flatpakstatus`, `internal/views/featurestatus` and `internal/views/pageview`). It holds single-row removal and clear-then-repopulate bookkeeping for rows a view adds to an expander, so a successful action can remove exactly its row and a later list reload does not accumulate stale rows. Like `actionmsg`, `actionstate`, `badgestate`, `bundleview` and `trustmsg`, it exists because `internal/views` itself cannot host a `_test.go` (puregotk panics resolving GTK/graphene shared libraries at package init, before any test runs — `docs/agents/skills/gtk-headless-tests.md`); unlike them it imports nothing at all outside the standard library.

Exported surface:

- `Tracker[T comparable]` — a generic, zero-value-ready value type holding the rows added since the last clear. It is generic and takes its removal actions as callbacks precisely so it never names a widget type, which is what keeps its dependency graph free of puregotk.
- `Add(row T)` — records a row that has just been added to the container.
- `Len() int` — how many rows are currently tracked.
- `Remove(row T, remove func(T)) bool` — removes the first matching tracked
  row, invokes the callback once, preserves the order of all other rows, and
  reports whether it found the row.
- `Clear(remove func(T))` — invokes the caller-supplied removal callback once per tracked row, in insertion order, then resets the slice to nil. A no-op on an empty or zero-value tracker.

`Tracker` has no mutex, generation counter, or in-flight flag by design: GTK main-thread safety is a property of the call site, which keeps the clear-and-repopulate sequence inside a single `sgtk.RunOnMainThread` closure, so the tracker is only ever touched from the main thread. `rowset_test.go` drives several successive simulated loads (including an empty load after a non-empty one) against a fake, non-GTK container and asserts after every load that the container holds exactly that load's rows.

### View-layer Flatpak update status (`internal/views/flatpakstatus`)

`internal/views/flatpakstatus` is one of the nine puregotk-free leaf packages under `internal/views/`. It turns the outcome of the two Flatpak update queries — how many updates are known, and which of the user/system installations could not be checked — into the Flatpak updates expander's subtitle text plus whether the expander should be expandable. Like `actionmsg`, `actionstate`, `badgestate`, `bundleview`, `trustmsg`, `rowset` and `pageview` it exists because `internal/views` itself cannot host a `_test.go` (puregotk panics resolving GTK/Libadwaita/GLib/graphene shared libraries at package init, before any test runs — `docs/agents/skills/gtk-headless-tests.md`); like `rowset` it imports nothing at all outside the standard library (`fmt`).

Exported surface:

- `Result{Subtitle string; Expandable bool}` — the expander state for one update load.
- `Subtitle(count int, userFailed, systemFailed bool) Result` — derives that state. Both halves come from a single call so the wording and the expansion decision cannot drift apart, the same reason `actionmsg` returns `ScriptDecision`/`TapTrustDecision`/`FeatureToggleDecision` structs. Failure is taken as two `bool`s rather than `error` values, which is what keeps the package free of any dependency on `internal/flatpak`.

`Expandable` is `count > 0` in every case: a failed query never invents updates, so there is nothing to expand that the count does not already reflect, while the rows that *were* found in a partially failed load are real and stay reachable. The five distinguishable outcomes are: both queries ok with no updates → `All applications are up to date` (the only case that makes the up-to-date claim); both ok with updates → `1 update available` / `%d updates available`; exactly one query failed with no updates → `No updates found in the <ok> installation; the <failed> installation could not be checked`; exactly one failed with updates → the count followed by `; the <failed> installation could not be checked`; both failed → `Could not check for updates`, which makes no claim about update state at all. `<ok>`/`<failed>` are the literal words `user` and `system`. `flatpakstatus_test.go` has one subtest per row (with both the user-failed and system-failed variants of the one-failed rows), and additionally asserts that all the subtitles are pairwise distinct, that "up to date" appears in the first case and no other, and that singular and plural both read correctly.

The package is pure and holds no state, so it is safe to call from a worker goroutine or from inside an `sgtk.RunOnMainThread` closure.

`loadFlatpakUpdates` (`internal/views/updates_page.go`) is its only call site. It keeps both `flatpak.ListUpdates` errors as values — `userErr` and `systemErr`, still logged exactly as before via the two `log.Printf("Error loading {user,system} flatpak updates: %v", …)` lines — instead of dropping them once logged, and then calls `flatpakstatus.Subtitle(len(allUpdates), userErr != nil, systemErr != nil)` once on the worker goroutine, before entering `sgtk.RunOnMainThread`. Inside that closure (past the `if uh.flatpakUpdatesExpander == nil { return }` guard, which stays because `flatpak_updates_group` can be disabled and the expander then never gets built) the result is applied unconditionally: `SetSubtitle(result.Subtitle)` and `SetEnableExpansion(result.Expandable)` run on *every* path, including the zero-update path, and only the building of the per-update rows is skipped when there are none. The view holds no subtitle text and makes no decision of its own; the old hard-coded `"All applications are up to date"` and `fmt.Sprintf("%d updates available", …)` strings are gone from it.

The practical consequence is that a total failure — both installations unqueryable — no longer renders as an all-up-to-date message: `allUpdates` is empty for the same reason it is empty when everything really is current, and only the retained errors distinguish the two, so the expander reads `Could not check for updates`. A partial failure is identified as partial rather than silently under-reported: the rows that were found are shown and expandable, with the subtitle naming the installation that could not be checked. The badge deliberately remains a plain count — `uh.updateCounts.Set(badgestate.Flatpak, len(allUpdates))` carries no error state — so a total failure shows a badge contribution of `0` next to an honest subtitle rather than an invented number.

### View-layer feature update status (`internal/views/featurestatus`)

`internal/views/featurestatus` is one of the nine puregotk-free leaf packages under `internal/views/`. It owns every string and every decision the Features page's updex update check needs: a feature row's subtitle, whether that feature has an update, and the features group's description. Like `actionmsg`, `actionstate`, `badgestate`, `bundleview`, `trustmsg`, `rowset`, `flatpakstatus` and `pageview` it exists because `internal/views` itself cannot host a `_test.go` (puregotk panics resolving GTK/Libadwaita/GLib/graphene shared libraries at package init, before any test runs — `docs/agents/skills/gtk-headless-tests.md`). Unlike them it imports one non-standard-library package, `internal/updex`, for the `CheckResult` type; that is safe because `internal/updex` is itself puregotk-free (`go list -deps ./internal/updex | grep -c puregotk` prints `0`), and `go list -deps ./internal/views/featurestatus | grep -c puregotk` prints `0` too.

Exported surface:

- `Status{Subtitle string; HasUpdate bool}` — the row state for one feature.
- `Feature(name string, results []updex.CheckResult) (Status, bool)` — derives that state from *all* of the feature's components. Both halves come from a single call so the wording and the update decision cannot drift apart, the same reason `flatpakstatus.Subtitle` returns a `Result`. The second return value is `false` when `len(results) == 0`, telling the caller to leave the row's existing subtitle untouched and not to count the feature; putting that skip decision in the package is what makes the zero-components case table-testable at all.
- `GroupDescription(totalFeatures, featuresWithUpdates int) string` — the group description after a check that completed.
- `GroupDescriptionCheckFailed(totalFeatures int) string` — the group description when the check itself failed; it makes no claim about update state.

**The ANY-component rule:** a feature has an update when **any** of its components reports one — not the first, not all. `Status.HasUpdate` is an OR across every element of `results`, and `featurestatus_test.go` asserts it by iterating every element of each case's slice rather than special-casing index 0, with cases placing the update first, last, in the middle, and in several components at once.

**The feature-counting rule:** `featuresWithUpdates` is a count of **features**, not of components — a feature with three outdated components counts once. The package doc comment states this explicitly, since the description's first number is a feature count and its second must be one too or the sentence is incoherent.

The five subtitle branches, for a feature named `<name>`:

| Situation | Subtitle |
|-----------|----------|
| exactly one component has an update, non-empty `CurrentVersion` | `<name> — update available for <component> (v<cur> → v<new>)` |
| exactly one component has an update, empty `CurrentVersion` | `<name> — update available for <component> (→ v<new>)` |
| two or more components have updates | `<name> — updates available for <n> components` |
| no updates, every component agrees on a non-empty `CurrentVersion` | `<name> — v<version>` |
| no updates, components disagree or any `CurrentVersion` is empty | `<name> — up to date` |

No branch emits a bare `v` with nothing after it, and no branch presents one component's version as the feature's version unless every component agrees on it. The group descriptions are `%d features available — update check failed`, `%d features available — all up to date`, `%d features available (1 update)` and `%d features available (%d updates)`; the leading `%d features available` fragment is reproduced verbatim from `loadFeatures`' own pre-check string, including its non-pluralized `features`, so only the update tail differs — which is also what makes a completed check that found nothing (`— all up to date`) visibly distinguishable from the pre-check state. `featurestatus_test.go` covers each branch as a table subtest and additionally asserts that the five subtitle branches are pairwise distinct for a fixed feature name, that no subtitle renders a bare `v`, and that the four group descriptions are distinct from each other and from the pre-check string.

The package is pure and holds no state, so it is safe to call from a worker goroutine or from inside an `sgtk.RunOnMainThread` closure.

`checkFeatureUpdates` (`internal/views/features_page.go`) is its only call site, and — as with `loadFlatpakUpdates` and `flatpakstatus` — the view holds no text of its own: every string in the update-check path now comes from `featurestatus`. `updex.CheckFeatures` still runs on the worker goroutine and all widget access still happens inside the single existing `sgtk.RunOnMainThread` closure. Inside it, the features group's description is set on **every** outcome. When the check failed, the existing `log.Printf("Feature update check failed: %v", err)` is kept and the description becomes `featurestatus.GroupDescriptionCheckFailed(totalFeatures)` before returning, so the group no longer keeps reading `%d features available` — which looked like a completed check that found nothing. When the check succeeded, the description is set unconditionally from `featurestatus.GroupDescription(totalFeatures, updateCount)`, including when `updateCount` is `0`, so "all up to date" is actually reported rather than the pre-check string being left in place. Per feature the view calls `featurestatus.Feature(check.Feature, check.Results)` over the whole `Results` slice — `check.Results[0]` is gone from the file, and with it the bug that a feature whose second component was outdated read as up to date — and counts one per feature with `status.HasUpdate`, so the description's two numbers are both feature counts. The zero-component skip is preserved as the `ok == false` return: the row keeps its existing subtitle and is not counted. Both guards that config-driven visibility requires stay: `features_group` can be disabled, so every `SetDescription` (the failure one included) sits behind `uh.featuresGroup != nil` and the `uh.featureRows` lookup keeps its `!ok { continue }`.

## Flatpak (`internal/flatpak/flatpak.go`)

Wraps the `flatpak` CLI. Parses tabular (tab-delimited, falling back to whitespace) output.

### Key types

- **`Application`** — name, applicationID, version, branch, origin, installation (user/system), ref
- **`UpdateInfo`** — name, applicationID, newVersion, branch, origin, installation
- **`ApplicationInfo`** — embeds `Application`, adds description, runtime, permissions map

### Operations

| Function | CLI command | Timeout | Notes |
|----------|------------|---------|-------|
| `ListUserApplications()` | `flatpak list --user --app --columns=name,application,version,branch,origin,ref` | 30s | Tabular parsed |
| `ListSystemApplications()` | `flatpak list --system --app --columns=name,application,version,branch,origin,ref` | 30s | Tabular parsed |
| `ListUpdates(user)` | `flatpak remote-ls --updates --app --columns=name,application,version,branch,origin [--user\|--system]` | 30s | Separate calls for user/system; `--app` excludes runtimes |
| `Install(appID, user)` | `flatpak install -y [--user\|--system] <appID>` | 30m | State-changing |
| `Uninstall(appID, user)` | `flatpak uninstall -y [--user\|--system] <appID>` | 30m | State-changing |
| `Update(appID, user)` | `flatpak update -y [--user\|--system] [<appID>]` | 30m | State-changing; empty appID updates all |
| `UninstallUnused()` | `flatpak uninstall --unused -y` | 30m | Maintenance cleanup |
| `Info(appID, user)` | `flatpak info --show-metadata [--user\|--system] <appID>` | 30s | Key-value parsed |
| `GetRemotes(user)` | `flatpak remotes --columns=name [--user\|--system]` | 30s | Lists configured remotes |

### State-changing commands

`install`, `uninstall`, `remove`, `update` — exactly four keys. As in homebrew, the map selects both the dry-run skip (these are skipped entirely and return a mock message) and the timeout class: `commandTimeout(args []string)` returns `mutationTimeout` (30 minutes) when `args[0]` is one of the four keys and `readTimeout` (30 seconds) otherwise, including for empty args, and `runFlatpakCommand` passes it to `context.WithTimeout`. Flatpak's read budget matches homebrew's 30 seconds, so both wrappers agree. `flatpak_test.go`'s `TestCommandTimeout` iterates the real map and asserts `len(stateChangingCommands) == 4`.

### Error handling

`runFlatpakCommand` is a thin wrapper: it applies the dry-run skip (before any `exec.Cmd` exists), builds a context from `commandTimeout(args)`, and delegates to the unexported `runFlatpakCommandAt(ctx context.Context, exe string, args ...string) (string, error)`, always passing `"flatpak"`. The executable path and context are parameters purely so `runner_test.go` can drive a `#!/bin/sh` script from `t.TempDir()` and control the deadline — the same seam `runStageStreaming` gives `internal/bootc` and `runBrewCommandAt` gives `internal/homebrew`. No exported function takes a `context.Context`: callers get deadlines, not cancellation. flatpak runs unprivileged; no `pkexec` is involved on this path.

`runFlatpakCommandAt` starts the command in its own process group (`cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`) and sets `cmd.Cancel` to `syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)`, so flatpak's download helpers (download workers, ostree pulls) are killed with the command instead of being orphaned when only the direct child is signalled. `cmd.WaitDelay` (5s) bounds the wait, because those helpers inherit the stdout/stderr pipes and a straggler would otherwise hold `Wait` open indefinitely. `cmd.Run` still reaps the child.

Failures classify into exactly four distinct outcomes, checked in this order — `errors.Is` against `context.DeadlineExceeded`/`context.Canceled`, never `==` on `ctx.Err()`, so a wrapped cause still classifies:

| Condition | Result |
|-----------|--------|
| The context deadline expired (`commandTimeout(args)` elapsed) | `*Error`, message `Command '<exe> <args>' timed out`, unwrapping to `context.DeadlineExceeded` |
| The context was cancelled by its owner | `*Error`, message `Command '<exe> <args>' was canceled`, unwrapping to `context.Canceled` |
| The command exited non-zero | `*Error` carrying the stderr text ("Flatpak command failed: …"), unwrapping to the `*exec.ExitError` |
| The executable is missing (`exec.ErrNotFound` for a bare name, `fs.ErrNotExist` for an explicit path) | `*NotFoundError` ("Flatpak not found…") |

A deadline and a cancellation never produce the same message, and neither surfaces as `signal: killed` — the process is killed by ChairLift's own `Cancel` func, so the raw wait error is replaced by the classified one. `Error` carries an `Err error` field and an `Unwrap() error` method, so `errors.Is(err, context.DeadlineExceeded)` works for callers while `Error` keeps satisfying `error` and keeps its human-readable `Message`; nothing in the views switches on its concrete type. `internal/flatpak/runner_test.go` covers each outcome against a fake script, asserts the deadline and cancellation messages differ and contain no `signal: killed`, and `TestRunFlatpakCommandAtKillsProcessGroup` proves the process-group kill by having the fake script spawn a background `sleep`, recording its PID, and polling `syscall.Kill(pid, 0)` until it returns `ESRCH` — proving the helper is gone, not merely that the parent returned.

`flatpak_test.go` separately drives every public CLI wrapper against a fake
`flatpak` on `PATH`, asserting exact user/system arguments, parsing, and query
failure propagation. It table-tests application and update parsing and
iterates all four real state-changing command keys to prove dry-run returns
before execution. This complements runner classification rather than chasing
a percentage in isolation.

### Update queries exclude runtimes

`ListUpdates` builds its argument list with the unexported pure helper
`updateListArgs(user bool) []string`, the only place the `remote-ls` command is
spelled. It passes `--app` — matching the precedent set by `listApplications` —
so runtimes and extensions are deliberately excluded from the results. Update
rows and the sidebar update badge therefore only ever describe applications,
which is what the user can act on from the applications page.

## bootc (`internal/bootc/`)

Wraps `bootc` for OSTree/composefs system updates, split across two files: `bootc.go` (unprivileged status reads) and `stage.go` (privileged update staging). Deliberately does not shell out to any separate CLI helper binary or Go client library — status parsing and stage-script invocation are both implemented directly against `os/exec`.

### `GetStatus` (unprivileged)

`GetStatus(ctx)` runs `bootc status --format json` with **no** `pkexec` — this is a plain read, safe to call from any goroutine (`internal/bootc/bootc.go`). Output is unmarshaled into `Status{Spec, Status: {Booted, Staged, Rollback}}`, where each of `Booted`/`Staged`/`Rollback` is a `*Deployment` (nil-safe accessors: `ImageRef()`, `Version()`, `Timestamp()`, `Digest()`).

`GetStatus` is a one-line wrapper: `return getStatusFrom(ctx, bootcCommand)`. The unexported `getStatusFrom(ctx context.Context, name string) (*Status, error)` runs `<name> status --format json`, classifies the error, and parses the output. The executable name is a parameter purely so `bootc_test.go` can drive a `#!/bin/sh` script from `t.TempDir()` on a host with no `bootc` installed — the same seam `runStageStreaming` gives staging and `runBrewCommandAt`/`runFlatpakCommandAt` give the two package-manager wrappers. The seam is unexported and its only production call site passes the fixed `bootcCommand` constant, so no caller-supplied or user-derived string can reach it.

Failures classify into exactly four distinct outcomes, checked in this order — `errors.Is` against `context.DeadlineExceeded`/`context.Canceled`, never `==` on `ctx.Err()`, so a wrapped cause still classifies:

| Condition | Result |
|-----------|--------|
| The context deadline expired | `*Error`, message `bootc status timed out`, unwrapping to `context.DeadlineExceeded` |
| The context was cancelled by its owner | `*Error`, message `bootc status was canceled`, unwrapping to `context.Canceled` |
| The command exited non-zero | `*Error`, message `bootc status failed (exit N): <stderr>`, unwrapping to the `*exec.ExitError` — matching neither context sentinel |
| The executable is missing (`exec.ErrNotFound` for a bare name, `fs.ErrNotExist` for an explicit path) | `*NotFoundError` (`bootc not found`) |

The deadline and cancellation messages differ, and neither surfaces as `signal: killed`: `exec.CommandContext` kills the child when the context ends, so the classified error replaces the raw wait error. `bootc.Error` carries an `Err error` field and an `Unwrap() error` method — matching `internal/homebrew` and `internal/flatpak` — so callers distinguish all four outcomes with `errors.Is`/`errors.As` while `Error` keeps its human-readable `Message`. `bootc_test.go` covers all four against fake scripts (plus the success parse), so the whole classification is exercised without a real `bootc` on the host.

### Boot gate semantics

`bootc status` exits 0 with a null `booted` field on hosts that aren't running a bootc deployment at all — so the gate cannot be the exit code. `Status.Booted()` returns `s.Status.Booted != nil`. `IsBootcBooted(ctx)` calls `GetStatus` and returns that boolean (treating any error as "not booted"). `IsBootcBootedCached()` wraps it in a `sync.Once` with a 5s timeout, computing the result once and caching it for the lifetime of the process — this lets multiple view goroutines call it during async startup without triggering redundant `bootc` invocations. **Do not use `/run/ostree-booted`** as a substitute gate: it is absent on snow's composefs-based deployments, so checking for it would hide bootc UI on every snow host.

### `StageUpdate` (privileged, streaming)

`StageUpdate(ctx, progressCh)` (`internal/bootc/stage.go`) runs `pkexec /usr/libexec/bootc-update-stage` via the unexported `runStageStreaming(ctx, progressCh, name, args...)` seam (which tests drive with a local fake script instead of `pkexec`), merging stdout+stderr and streaming each trimmed non-empty line to `progressCh` as an `EventMessage`. `progressCh` is always closed before returning (`defer close`). On successful exit it sends a final `EventComplete`.

Failures classify into exactly four distinct outcomes, checked in this order — `errors.Is` against `context.DeadlineExceeded`/`context.Canceled`, never `==` on `ctx.Err()`:

| Condition | Result |
|-----------|--------|
| The context deadline expired | `*Error`, message `Update staging timed out`, unwrapping to `context.DeadlineExceeded` |
| The context was cancelled by its owner | `*Error`, message `Update staging was canceled`, unwrapping to `context.Canceled`; if cancellation instead wins the race inside the streaming `select`, the child is killed and reaped and the bare `ctx.Err()` (`context.Canceled`) is returned, which classifies identically |
| The script exited non-zero | `*Error`, message `update staging failed (exit N): <last output line>`, unwrapping to the `*exec.ExitError` — matching neither context sentinel |
| `pkexec` itself is missing | `*NotFoundError` (`pkexec not found`) |

The deadline and cancellation messages differ, and neither surfaces as `signal: killed`. `stage_test.go` covers all of these against fake `#!/bin/sh` scripts, so no test needs a real `pkexec` or `bootc`.

**Why bootc direct-kills instead of killing the process group:** on cancellation `runStageStreaming` calls `cmd.Process.Kill()` on the direct child only, and `internal/bootc` deliberately sets no `Setpgid`/`cmd.Cancel` process-group kill. The staging child runs under `pkexec` and is therefore root-owned, so this unprivileged process cannot signal it or its process group — a `syscall.Kill(-pid, SIGKILL)` would fail by design. That is the exception: the unprivileged `runBrewCommandAt` and `runFlatpakCommandAt` runners *do* kill the whole process group, because their children are owned by the same user. Making the privileged path group-killable would be a privilege-model change, not a runner change. `getStatusFrom` needs neither: `bootc status` is unprivileged and short-lived, and `exec.CommandContext`'s default kill of the direct child suffices.

**Why a stage script instead of `bootc upgrade`:** upstream `bootc upgrade`'s registry-transport pull fails on snow's composefs images. The stage script works around this by using `podman pull` (whose pull path works) to fetch the image into containers-storage, then running `bootc switch --transport containers-storage` to stage the already-pulled image — `podman` does the pull, `bootc` does the switch. This keeps the actual workaround logic in one place (the snow-shipped script, source of truth in the snosi project) instead of duplicating pull/switch orchestration inside ChairLift. The script is idempotent: it exits 0 without staging anything when the deployment is already current, so `StageUpdate` doubles as both "check for update" and "apply update".

### Event types

- `EventMessage` — one line of stage-script output
- `EventComplete` — sent once, after successful completion

This is intentionally flatter than a step/percent progress model, because the
stage script emits unstructured log lines, not a structured progress protocol.
Failures are returned by `StageUpdate` and handled once by the view after the
channel closes; there is no error event duplicating that path.

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
        case bootc.EventComplete:
            // mark streamed activity complete
        }
    })
}
// After the channel closes, handle the returned error or re-query GetStatus.
```

### Progress UI (`internal/views/updates_page.go`)

`onBootcStageClicked()` drives the updates page's "System Update" expander directly (there is a single staging operation, so no shared cross-operation helper is needed) — it disables the button, spawns `bootc.StageUpdate` in a goroutine, and processes events on a second goroutine, restoring button state and showing a toast on completion. The system page's `loadBootcStatus()` is a separate, read-only path: it calls `bootc.GetStatus` to display the booted/staged/rollback deployment images, versions, and digests, with no staging controls — staging only happens from the Updates page.

## Updex (`internal/updex/updex.go`)

Manages system features (add-on software/configuration modules). Unlike other wrappers, updex does **not** shell out to a CLI for reads. It uses the `github.com/frostyard/updex/updex` Go library directly for read operations, with a singleton `*updexapi.Client`. Write operations that require root are delegated via pkexec to the fixed absolute path `internal/updex.HelperPath` (`/usr/bin/chairlift-updex-helper`, built from `cmd/chairlift-updex-helper/main.go`) — never a bare, `$PATH`-resolved name, since `pkexec` matches the resolved absolute path against `data/org.frostyard.ChairLift.updex.policy`'s `org.freedesktop.policykit.exec.path` annotation to select the right action; see [OVERVIEW.md](./OVERVIEW.md#privileged-operations) for the full rationale and the matching `PREFIX=/usr` Makefile requirement.

### Key types

Type aliases to `github.com/frostyard/updex/updex`:
- **`Feature`** (`FeatureInfo`) — name, description, enabled flag, documentation URL
- **`FeatureCheck`** (`CheckFeaturesResult`) — feature name plus `Results []CheckResult`, one entry per component, each carrying its own update-available flag and versions. There is no feature-level update flag: a feature has an update when *any* of its components does (see [`internal/views/featurestatus`](#view-layer-feature-update-status-internalviewsfeaturestatus))
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

`updex_test.go` drives all three public write operations through a fake
`pkexec` on `PATH` and asserts that argv always begins with the fixed
`HelperPath`, followed by the exact subcommand/name shape. It also covers
missing-pkexec, non-zero/stderr, timeout, default-context, and wrapper dry-run
outcomes. Multi-component update aggregation remains in the pure
`internal/views/featurestatus` package, where every result is inspected and
the full decision table is tested without constructing GTK widgets.

### Helper binary (`cmd/chairlift-updex-helper/main.go`)

A small standalone binary that accepts commands (`enable-feature`, `disable-feature`, `update`) and uses the updex Go library to perform privileged operations. It supports `--dry-run` for all three subcommands — `enable-feature`, `disable-feature`, and `update` — passing it through to the corresponding `updex.*Options.DryRun` field. Outputs JSON to stdout. Invoked via pkexec so that the main chairlift process does not need root.

`main.go` itself is thin dispatch only: strict `os.Args` parsing and each
subcommand's `Options` struct live in `internal/updexhelper`
(`internal/updexhelper/updexhelper.go`), a package with no puregotk import —
only stdlib plus `github.com/frostyard/updex/updex`. That's what makes the
logic testable at all: neither `gates_chunk` nor `make ci` ever runs `go test
./...`, both are scoped to `go test ./internal/...`, so a `_test.go` under
`cmd/chairlift-updex-helper` would never execute under any gate this repo
actually runs. `ParseInvocation` accepts only `enable-feature <name>
[--dry-run]`, `disable-feature <name> [--dry-run]`, and `update [--dry-run]`;
`SupportedCommands` is the complete first-argument surface matched by the
PolicyKit actions. `EnableOptions`, `DisableOptions`, and `UpdateOptions` set
`DryRun` exactly. Tests cover every accepted/rejected argv shape, the immutable
command inventory, and all three option builders.

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

The Applications page's configured Brew bundle rows use the same paired
decision pattern. `homebrew.BundleInstall` already skips `brew bundle install`
under dry-run because `bundle` is state-changing, but a nil wrapper error does
not mean the row should read `Installed`: `actionmsg.BundleInstall` returns
`BundleInstallDecision{Complete: false, Toast: <preview>}` for that outcome,
and the callback resets both its `bundleview.InstallGate` and button. A live
success returns `Complete: true`, permanently completes the gate, labels the
button `Installed`, and leaves it insensitive. A command failure resets the
gate and button without showing a success/preview toast. `TryStart` and
`SetSensitive(false)` both happen before the worker goroutine starts, so
repeated callbacks cannot overlap an install.

The Applications page's per-result Homebrew install button
(`onHomebrewSearch`, `internal/views/applications_page.go`) and per-app Flatpak
uninstall buttons (`loadFlatpakApplications`, both user and system branches)
show toasts built by `actionmsg.Install(homebrew.IsDryRun(), result.Name)` and
`actionmsg.Uninstall(flatpak.IsDryRun(), appID)`, rather than unconditional
completion claims. The Homebrew path restores its install control after a
dry-run and does not refresh, because nothing changed; only a live success
completes the control and starts the generation-guarded installed-package
refresh described above.

Installed Homebrew formula/cask rows follow the same decision path for
uninstall, and formula rows add pin/unpin. They confirm before starting,
disable every mutation control on the row while the worker runs, use
`actionmsg.Uninstall`/`actionmsg.Pin` for live versus preview wording, restore
on failure or dry-run, and refresh the installed inventory only after live
success. New Flatpak discovery and install deliberately remain in the
configured external manager; ChairLift's direct Flatpak UI lists and
uninstalls installed applications.

The Flatpak list refresh after uninstall remains unconditional because it
re-queries live state either way. Each successful loader branch first clears
the prior `flatpakUserRows` or `flatpakSystemRows` tracker and then rebuilds it
inside one main-thread closure. Homebrew's installed formula/cask loader now
uses the same separate-tracker clear-before-repopulate pattern, with an
additional refresh generation because multiple Homebrew actions can request
overlapping reloads. Not-installed and error branches change the subtitle and
preserve the last known rows.

The Updates page's per-package Homebrew upgrade button, per-app Flatpak update button, and the "Update Homebrew" self-update button (`internal/views/updates_page.go`) follow the same toast pattern: `actionmsg.Upgrade(dryRun, pkgName)`, `actionmsg.Update(flatpak.IsDryRun(), appID)`, and `actionmsg.SelfUpdate(dryRun, "Homebrew")` replace what were unconditional "upgraded"/"updated"/"updated successfully" toasts, since `upgrade` and `update` are both in their wrappers' `stateChangingCommands` and no-op under dry-run. The Flatpak update button's list refresh (`go uh.loadFlatpakUpdates()`) stays unconditional, same reasoning as the uninstall refresh above.

The two Homebrew paths additionally use `actionstate.Gate` before spawning a
goroutine and immediately make the clicked button insensitive with an
`Updating...` or `Upgrading...` label, so a repeated callback cannot overlap
the operation even if it bypasses GTK's insensitive-button guard. A command
failure restores the original label/sensitivity and leaves
the Homebrew value in `updateCounts`, the tracked rows, and the sidebar badge
unchanged. A dry-run wrapper success shows the preview toast and restores the same control
without refreshing because no metadata or package state changed.

A live per-package success completes its gate, removes exactly its tracked row
through `rowset.Tracker.Remove`, decrements the Homebrew count (and therefore
the aggregate sidebar badge) through `updateCounts.Add(badgestate.Homebrew,
-1)`, and starts a full `ListOutdated` refresh. A live top-level `brew update`
keeps its button busy while that refresh runs and
restores it from the refresh's main-thread completion callback. Each request
takes a generation from `actionstate.RefreshGate`; its main-thread result
first proves that generation is still current, so a slower older query cannot
overwrite rows or counts produced by a newer request. A superseded request
still invokes its completion callback with failure, ensuring the control that
requested it is not stranded.

The refresh uses `actionstate.OutdatedRefresh`: only a successful current
query clears/rebuilds rows and adopts `len(packages)` as the count; a failed
current query changes the expander subtitle to
`Error refreshing updates: ...` but retains the last known rows/count. All
external Homebrew calls remain on worker goroutines and all widget mutations,
including row removal and control restoration, remain inside
`sgtk.RunOnMainThread`.

The Updates page's bootc "Check for Updates" stage button (`onBootcStageClicked`, `internal/views/updates_page.go`) follows the same `actionmsg` pattern, with one difference from the buttons above: unlike `Install`/`Upgrade`/etc., whose completion text is selected purely by `dryRun`, `BootcStage(dryRun, staged)` also takes the live `staged` result from the post-`wg.Wait()` `bootc.GetStatus()` re-read, because the non-dry-run branch still needs to pick between the "staged" and "up to date" strings. Under dry-run, `staged` is ignored entirely and a single preview string is returned instead — see "Dry-run behavior" under bootc above for why. The expander's `SetSubtitle` calls in the same code block are *not* routed through `actionmsg`; they keep reading live `GetStatus()` output unconditionally, since the subtitle is a persistent status display rather than a per-click completion claim.

The Features page's per-feature switch (`onFeatureToggled`, `internal/views/features_page.go`) follows the same decision-struct pattern as maintenance-script execution and tap trust: on a successful `updex.EnableFeature`/`DisableFeature` call, `decision := actionmsg.FeatureToggle(updex.IsDryRun(), enabled, name)` is computed once, and the switch's visual state is driven solely by `decision.Confirm` — `toggle.SetActive(enabled)` (confirming the flip) when `Confirm` is true, `toggle.SetActive(!enabled)` (reverting to the pre-click state) when it is false. Under dry-run, `updex.runHelper` returns before ever invoking pkexec, so nothing was actually toggled and the switch must not visually confirm a change that did not happen — this is the other "switch/list implies a state change after a preview" bug (the tap-trust row-removal case is the same pattern in Homebrew's Untrusted Taps list). The Update button (`onUpdateFeaturesClicked`) has no equivalent mutation to gate — its `SetSensitive`/`SetLabel` reset is unconditional in both modes — so its toast is a plain string, `actionmsg.FeatureUpdate(updex.IsDryRun())`.

## Install-path consistency (`internal/installcheck`)

The source `make install` path (Makefile, `PREFIX` defaulting to `/usr`) and
the two packaged nFPM (deb/rpm/apk) layouts GoReleaser builds from
`.goreleaser.yaml` ship this repository's privileged surface and maintainer
configuration. They are hand-maintained text — a Makefile recipe and YAML
blocks — with no shared code path, so nothing stops them (or
`internal/updex.HelperPath`, the fixed absolute path `pkexec` matches against
the policy's `exec.path` annotation) from silently drifting apart.

ChairLift packages only the bootc and updex `.policy` files. It no longer
ships its old `.rules` files, which returned `YES` for every active local
member of the `sudo` group and bypassed authentication. Source installation
explicitly removes those legacy rule paths; package upgrades remove them as
obsolete tracked files. Both policies now use normal administrator
authentication, while the updex policy selects one action for each supported
first argument and the helper validates the complete argv shape.

All three layouts install the repository's `config.yml` as package-owned
maintainer defaults at `/usr/share/chairlift/config.yml`. None installs
`/etc/chairlift/config.yml`: that higher-precedence path belongs to the
administrator and must survive package installation and upgrades unchanged.

GoReleaser has two nFPM entries. `frostyard-chairlift` is self-contained and
selects both `chairlift` and `chairlift-updex-helper` builds.
`frostyard-chairlift-system-integration` selects only the helper and packages
only maintainer config plus the bootc/updex policies, for pairing with a
user-scoped app installation. The two package names conflict to prevent
simultaneous ownership of the same fixed system files. The companion does not
provide `/usr/libexec/bootc-update-stage`; a distro must bake its own trusted
implementation at that exact policy-annotated path.

`internal/installcheck` holds regression tests, not production code, that turn
"verified by inspection" into real, gated checks. The first two guard the
installed layout itself:

- **`TestMakefileInstallUsesUsrPrefix`** runs `make -n install
  DESTDIR=<t.TempDir()>` — a dry run, so no compilation, no writes outside
  the temp dir, and no root — once with no `PREFIX` override and once with
  `PREFIX=/usr`, and asserts the printed `install -Dm...` lines place the
  updex helper at `DESTDIR` + `internal/updex.HelperPath` and both policies
  under the fixed `/usr/share/polkit-1/actions` directory PolicyKit reads,
  removes both legacy rules from `DESTDIR/usr/share/polkit-1/rules.d`,
  installs maintainer defaults at
  `DESTDIR/usr/share/chairlift/config.yml`, and never targets the
  administrator-owned `/etc/chairlift/config.yml`. It shells out to
  the real `make` rather than parsing the Makefile textually because `make`
  itself is the authority on what a given `PREFIX`/`DESTDIR` combination
  actually resolves to (variable derivation, `$(DESTDIR)$(BINDIR)`
  concatenation, recipe ordering) — a hand-rolled Makefile parser would just
  be a second, divergence-prone implementation of `make`'s own substitution
  rules, and would stop being a regression test for the exact thing that
  broke (the *installed* path) the moment it disagreed with real `make`
  output. If `make` is not installed, the check skips with an explicit
  diagnostic; when `make` is available, command or layout failures remain hard
  failures.
- **`TestGoreleaserNfpmLayoutMatchesUsrPrefix`** parses the real, repo-root
  `.goreleaser.yaml` (not a fixture) with the already-vendored
  `gopkg.in/yaml.v3` and, iterating **every** `nfpms[]` entry (not just
  `nfpms[0]`, so adding or reordering a second package with the wrong layout
  still fails — per
  `docs/agents/skills/regression-tests-must-cover-every-collection-entry.md`),
  asserts each entry's `bindir` matches the directory of
  `internal/updex.HelperPath`, its updex/bootc policy `contents[].dst` entries
  equal the fixed polkit-1 actions paths, their policy/config modes remain
  `0644`, and no `.rules` content remains. It also requires every package to
  map the repository `config.yml` to
  `/usr/share/chairlift/config.yml` and rejects any content entry targeting
  `/etc/chairlift/config.yml`.
- **`TestGoreleaserPublishesSystemIntegrationPackage`** requires exactly one
  full package and one integration package, verifies their build filters,
  mutual conflicts, unique IDs, and the integration package's exact three
  content mappings. This prevents the companion from accidentally acquiring
  the GUI binary or losing one of the root-owned integration files.

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

A third regression test, **`TestGoreleaserLicenseIsGPL`**, guards a related
but distinct drift: `.goreleaser.yaml` briefly declared `license: MIT` in
both its top-level `metadata:` block and its `nfpms[]` entry's `license`
field, while the project's actual license is GPLv3-or-later (`LICENSE`, and
`internal/window/window.go`'s about dialog, which sets
`gtk.LicenseGpl30Value` — puregotk's "GPL 3.0 or later" enum value, distinct
from `LicenseGpl30OnlyValue`). Like the layout test above, it parses the
real, repo-root `.goreleaser.yaml` via the shared `loadGoreleaserConfig`
helper (no fixture) and asserts `cfg.Metadata.License` and, iterating
**every** `nfpms[]` entry with a per-index `t.Run` (not just `nfpms[0]`, per
`docs/agents/skills/regression-tests-must-cover-every-collection-entry.md`),
each entry's `License` field equal the fixed SPDX identifier
`GPL-3.0-or-later`. `GoreleaserConfig.Metadata` (`MetadataConfig.License`)
and `NfpmConfig.License` (`internal/installcheck/installcheck.go`) exist
specifically to give `yaml.Unmarshal` somewhere to put these two values;
without those struct fields yaml.v3 silently drops them and the test would
pass vacuously regardless of what the YAML says. This test exists so a
future edit reintroducing MIT (or any other license) in either location —
the exact regression that motivated it — fails the gate instead of shipping
mislabeled deb/rpm/apk package metadata again.

A fourth and fifth regression test guard the same class of drift for the
**repository URL**. `.goreleaser.yaml`'s `metadata.homepage`
(`https://github.com/frostyard/chairlift`) is the **single source of truth**
for that URL: GoReleaser Pro v2.13+ exposes the global `metadata:` block as
template context to every templated field, so `release.footer`'s "Full
Changelog" line derives its URL from `{{ .Metadata.Homepage }}` plus the
`/compare/{{ .PreviousTag }}...{{ .Tag }}` suffix. The footer therefore
contains no repository-owner literal at all, and `{{ .ProjectName }}` is
deliberately **not** concatenated onto it — the homepage already ends in the
repository name, so appending the project name would produce a doubled path.
The regression this guards: the footer used to hardcode the repository's
previous owner in that URL while `metadata.homepage` already named the
current one, so a single file identified one repository two disagreeing ways
and every generated release note's Full Changelog link pointed at the wrong
owner. Deriving the footer from the homepage removes the duplicate rather
than policing it, and the two tests keep the now-load-bearing source of truth
honest:

- **`TestGoreleaserMetadataHomepageIsFrostyardRepo`** asserts
  `cfg.Metadata.Homepage` still equals `https://github.com/frostyard/chairlift`,
  so the value the footer depends on cannot silently drift.
- **`TestGoreleaserReleaseFooterUsesMetadataHomepage`** asserts the parsed
  `release.footer` has exactly one "Full Changelog" line, that it references
  `{{ .Metadata.Homepage }}`, that it keeps the
  `/compare/{{ .PreviousTag }}...{{ .Tag }}` suffix, and that it contains no
  `.ProjectName`; a separate check rejects any `github.com/` literal anywhere
  in the footer, which catches a rewrite to a hardcoded *current*-owner URL
  just as surely as a stale one. An absent or empty footer, or any line count
  other than exactly one "Full Changelog" line, is a `t.Fatal`, not a silent
  pass, so the test cannot succeed vacuously against a config whose footer
  was deleted.

Both assert on the **template text** parsed out of the YAML — never a
rendered value. The footer is a Go template expanded by GoReleaser only at
release time, and GoReleaser Pro (this config sets `pro: true` and a
`nightly:` block) is not installed on the gate host or in `make ci`; it runs
only in `.github/workflows/{release,snapshot}.yml` via `goreleaser-action`
with a `GORELEASER_KEY` secret. `goreleaser check` is therefore deliberately
not run anywhere — locally, in `gates_chunk`, or in `make ci` — and neither
test shells out or renders anything. As with the license guard,
`MetadataConfig.Homepage` and `ReleaseConfig.Footer` in
`internal/installcheck/installcheck.go` exist solely so `yaml.Unmarshal` has
somewhere to put those values, exactly as `MetadataConfig.License` does;
without the struct fields yaml.v3 drops them and both tests would pass
vacuously regardless of what the YAML says.
