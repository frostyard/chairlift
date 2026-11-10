# Package Manager Wrappers

Living design document (formerly `yeti/package-managers.md`; folded into
`docs/` per [frostyard/core ADR-0025](https://github.com/frostyard/core/blob/main/docs/adr/0025-consolidate-repository-docs-into-docs.md)).

Each wrapper lives in its own package under `internal/` and follows a consistent pattern: module-level dry-run flag, availability check with cached variant (`IsInstalledCached()` using `sync.Once`), and context-based timeouts in two classes — 30s for read-only commands, 30m for state-changing ones, selected per invocation by each package's `commandTimeout(args)` helper from its `stateChangingCommands` map. `internal/fwupd` keeps the same two classes but passes a constant per call, because it has exactly one read command and one state-changing command and therefore needs no selector (see its section below). All are called from the domain structs of `internal/views` (`Brew`, `Flatpak`, `Firmware`, `System`, `Features`, `Maintenance`), always on a worker goroutine — a loader started with `views.Go`, or a `frostui.Job`'s `Run` on the serial job queue — never on the UI thread. Every command a wrapper starts is reported through `internal/cmdlog`: `cmdlog.Start` announces it, its stdout and stderr are teed line by line through the returned trace, `End` reports its error and run time, and `cmdlog.DryRun` reports a command that dry-run mode skipped; the window streams these events into its console. Each runner's subsection below says where. The cached availability check matters because the domains look for their tool on a goroutine at startup and gate their cards and update sources on the answer: several goroutines may ask about the same tool, and the result should only be computed once.

## Homebrew (`internal/homebrew/homebrew.go`)

Wraps the `brew` CLI. Uses JSON output (`--json=v2`) for structured data where available.

### Key types

- **`Package`** — name, version, pinned status, outdated flag, `InstalledOnRequest` bool, `Dependencies` string slice (struct field exists but not populated by current parsing), and `Latest`, the version an upgrade brings. `ListOutdated` sets `Latest` from `brew outdated --json=v2`'s `current_version` (`parseOutdated`); it is empty elsewhere. The Homebrew source of the Updates page shows each outdated package as `Version → Latest`.
- **`SearchResult`** — name plus a required `PackageKind` (`Formula` or
  `Cask`). The kind is not cosmetic: it selects whether installation adds
  `--cask`.

### Operations

| Function | CLI command | Timeout | Notes |
|----------|------------|---------|-------|
| `ListInstalledFormulae()` | `brew info --installed --json=v2 --formula` | 30s | JSON parsed |
| `ListInstalledCasks()` | `brew info --installed --json=v2 --cask` | 30s | JSON parsed |
| `ListOutdated()` | `brew outdated --json=v2` | 30s | JSON parsed by `parseOutdated`; returns both formulae and casks, `Version` from `installed_versions` (joined with `, `), `Latest` from `current_version`, and `Pinned` for formulae |
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

`Brew.loadBundles` (`internal/views/brew_bundles.go`) calls discovery on a
worker goroutine through `views.Go`, reads each Brewfile (at most 256 KiB,
`readBrewfile`) and lists its packages with `pageview.BrewfileEntries`, then
publishes the bundle cards and `bundleview.Present`'s text on the UI thread.
It runs after the startup Homebrew check whether or not Homebrew is
installed: without Homebrew the bundles still show, with their installs off.
`brew_bundles_group` is independent of `brew_group`: the Bundles tab of the
Apps page (`Brew.DrawBundles`) reads nothing of the installed inventory, and
the `RefreshApps` a live install calls does nothing while `brew_group` is
off. `Brew.AskBundle` takes the bundle's `bundleview.InstallGate`
(`TryStart`) before it asks through `Env.Dialogs` (a `frostui.Confirm`
listing the Brewfile and its package count); cancel resets the gate. On
confirm, `runBundle` queues `homebrew.BundleInstall` as one job on the serial
queue (`Env.Run`, source `brew`, key `bundle:<path>`), so a second request
cannot overlap a running install. A failed or canceled install resets the
gate, a failure with an error toast. A successful dry run uses
`actionmsg.BundleInstall(...).Complete == false`, shows the preview toast and
resets the gate, because nothing was installed. A live success completes the
gate (the card and its palette command then read installed), records an
activity entry and reloads the installed inventory.

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

`Brew.Search` and `runSearch` (`internal/views/brew_search.go`, the Find
more tab of the Apps page, `brew_search_group`) give each query a generation
from the search's own `actionstate.RefreshGate`, so an older slow query cannot
replace newer results; a search and an inventory reload never retire each
other. Results keep their `Formula` or `Cask` kind; each result row shows the
exact command an install runs (`pageview.BrewInstallCommand`), and
`pageview.BrewSearchStatus` says what the field searched for and found.
`askInstall` takes the result's `brewAction` gate (an `actionstate.Gate`) and
asks through `Env.Dialogs`; cancel restores it. On confirm `runInstall` queues
`homebrew.Install(result.Name, result.Kind == homebrew.Cask)` as a job (key
`formula:<name>` or `cask:<name>`) and shows `Installing…`.
`actionstate.PackageInstall` is the tested authority for restore/complete/refresh:
a failure restores the control with an error toast, a dry run restores it with
the `actionmsg.Install` preview toast, and a live success completes it as
`Installed`, records an activity entry and calls `RefreshApps`.
`Brew.RefreshApps` (`brew_apps.go`) takes a generation from the inventory's own
`RefreshGate`, lists formulae and then casks on one worker, does nothing while
`brew_group` is off, and publishes only while its generation is the newest; a
list that fails keeps the rows it had and records its error.

### Installed Homebrew actions

The Apps page shows every installed formula and cask, with the Flatpak
apps, in one sortable table with an inspector (`internal/views/apps.go`);
`Brew.Apps` gives the Homebrew rows and `Brew.Act` runs their actions. A
formula offers Pin or Unpin and Uninstall, a cask Uninstall, and an outdated
package Update, which queues the same upgrade job as the Updates page. A
formula's pinned state shows in the table and the inspector. Each installed
row has one `brewAction` (an `actionstate.Gate` plus the label of what runs)
shared by all its controls, so a pin and an uninstall cannot overlap; an
upgrade of the same name and the row's action wait for each other
(`rowInUse`, `upgradeInUse`). Every action asks first through `Env.Dialogs`
(`frostui.Confirm`): pin/unpin is a plain confirmation, while uninstall is
destructive and names whether the target is a formula or a cask.

After confirmation the row shows `Pinning…`, `Unpinning…` or
`Uninstalling…`, and `runPin`/`runUninstall` queue a job that calls
`homebrew.Pin`, `Unpin` or `Uninstall(name, isCask)` on the job's goroutine.
`Brew.finishMutation` applies `actionstate.PackagePin` or
`PackageUninstall`: a failure restores the controls and shows an error toast;
a dry run restores them with the `actionmsg.Pin`/`actionmsg.Uninstall`
preview toast; a live success completes the old controls, records an activity
entry and starts both the generation-guarded `RefreshApps` and
`loadOutdated`, because a pin or an uninstall changes the outdated list. The
reload, rather than the action, owns row replacement, so overlapping loads
cannot publish stale installed state; `brewKeepRunning` carries an action that
is still asked or running over a reload, so the reloaded row shares its gate.

### Homebrew update source (`internal/views/brew_source.go`)

The Homebrew source of the Updates page (`brewSource`, a `views.UpdateSource`)
exists while `brew_updates_group` or `brew_trust_group` is on, and is
unavailable when Homebrew is missing. `Brew.loadOutdated` takes a generation
from the outdated list's `RefreshGate`, runs `homebrew.ListOutdated` on a
worker, and on the UI thread publishes only while that generation is current;
a `done` callback, when given, runs for every reload, the superseded ones too,
so a control waiting on it is never stranded. `actionstate.OutdatedRefresh`
decides the result: a failed check keeps the last known rows and count and
reports `Error refreshing updates: ...` as the source's error; a successful
one replaces them, including with none. The source's items are the outdated
packages (`From` the installed version, `To` `Package.Latest`); a pinned
formula and a package of an untrusted tap are held (`HeldWhy` "pinned" or
"tap not trusted"): they count in the Updates figure (`Store.UpdateCount`
counts held items) but "Update everything" and "Update all" leave them out
and their button is off. The note comes from `pageview.BrewSourceNote`.

Each upgrade is one job (`Brew.upgradeJob`, key the package name) holding the
package's `brewAction` gate, so a repeated request cannot overlap it.
`Brew.finishUpgrade` applies `actionstate.PackageUpgrade`: a failure restores
the control and leaves the item and the count unchanged (an untrusted-tap
failure gets the `trustmsg` text, see "Tap trust" below); a dry run shows the
`actionmsg.Upgrade` preview and restores the control without a refresh,
because nothing changed; a live success completes the control, removes
exactly its item (which lowers the count), records an activity entry and
starts a full `loadOutdated`, then reloads the installed inventory once no
other upgrade waits. `Brew.UpdateMetadata` runs `brew update` as a job under
its own gate and applies `actionstate.MetadataUpdate`: a failure and a dry
run restore the control (with an error toast or the `actionmsg.SelfUpdate`
preview); a live success keeps the control busy and restores it from
`loadOutdated`'s `done` callback once the reload finished.

### Error handling

`runBrewCommand` is a thin wrapper: it applies the dry-run skip (before any `exec.Cmd` exists; the skip logs `[DRY-RUN] Would execute: brew ...` and reports the command with `cmdlog.DryRun`), builds a context from `commandTimeout(args)`, and delegates to the unexported `runBrewCommandAt(ctx context.Context, exe string, args ...string) (string, error)`, always passing `"brew"`. The executable path and context are parameters purely so `runner_test.go` can drive a `#!/bin/sh` script from `t.TempDir()` and control the deadline — the same seam `stageexec.Run` gives OS staging. No exported function takes a `context.Context`: callers get deadlines, not cancellation.

`runBrewCommandAt` starts the command in its own process group (`cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`) and sets `cmd.Cancel` to `syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)`, so brew's helper processes (git, curl, download workers) are killed with the command instead of being orphaned when only the direct child is signalled. `cmd.WaitDelay` (5s) bounds the wait, because those helpers inherit the stdout/stderr pipes and a straggler would otherwise hold `Wait` open indefinitely. `cmd.Run` still reaps the child. It announces the command with `cmdlog.Start`, tees stdout and stderr through the trace (`trace.Stdout(&stdout)`, `trace.Stderr(&stderr)`) while still buffering both for parsing and diagnostics, and ends it with `trace.End(err)`, so the console shows every brew command and its output as it runs.

Failures classify into exactly five distinct outcomes, checked in this order — `errors.Is` against `context.DeadlineExceeded`/`context.Canceled`, never `==` on `ctx.Err()`, so a wrapped cause still classifies:

| Condition | Result |
|-----------|--------|
| The context deadline expired (`commandTimeout(args)` elapsed) | `*Error`, message `Command '<exe> <args>' timed out`, unwrapping to `context.DeadlineExceeded` |
| The context was cancelled by its owner | `*Error`, message `Command '<exe> <args>' was canceled`, unwrapping to `context.Canceled` |
| The command exited non-zero and its diagnostic text (see below) matches `isUntrustedTapMessage` | `*UntrustedTapError` carrying the diagnostic message (see "Tap trust" below) |
| The command exited non-zero otherwise | `*Error` carrying the diagnostic message, unwrapping to the `*exec.ExitError` |
| The executable is missing (`exec.ErrNotFound` for a bare name, `fs.ErrNotExist` for an explicit path) | `*NotFoundError` ("Homebrew not found…") |

**Non-zero-exit diagnostics (`internal/homebrew/diagnostic.go`).** What a non-zero exit carries depends on whether `args[0]` is in `stateChangingCommands`:

- **Read-only command** (`info`, `search`, `outdated`, `tap-info`, ...): the diagnostic text is the whole stderr, and the message is `Brew command failed: <stderr>` verbatim. Stdout is ignored and nothing is logged. `searchKind` matches `"No formulae or casks found"` on this full text, and a namespace with no matches is an ordinary empty result, not a failure worth a log line.
- **State-changing command** (`install`, `upgrade`, `bundle`, `trust`, ...): the diagnostic text is `joinCommandStreams(stdout, stderr)` — both streams, stdout first, a blank stream omitted. `brew bundle` runs each entry's real installer (flatpak, mas, ...) with its stderr merged into stdout, buffers it, and replays it on stdout when the entry fails, while brew's own restatements ("Installing <name> has failed!", "`brew bundle` failed! N Brewfile dependencies failed to install") go to stderr. Reading stderr alone therefore discarded the only line naming the cause, and the bundle toast showed a progress line instead. The full joined text is logged as `Command '<exe> <args>' failed:` followed by the output, which is the evidence a bug report needs. The message is `Brew command failed: <summary>`, where `summarizeDiagnostic` picks one line: the first explicit error line (`isErrorLine`: a case-insensitive `Error:`/`error:` or `fatal:` prefix, or `Failure while executing`), else the first line reporting a failure (`isFailureLine`: contains `failed` or `failure`), else the last non-blank line. `diagnosticLines` keeps only the final state of carriage-return progress rewrites but treats a trailing CR as a CRLF line ending. `elideMiddle` cuts a line longer than 240 runes in the middle (keeping the last 60 runes, joined by ` … `), on rune boundaries, so both what failed and what it failed on survive. If both streams are blank, the message falls back to the exit status (`Brew command failed: exit status N`).

`isUntrustedTapMessage` is checked against the same diagnostic text, so for a state-changing command an untrusted-tap complaint on either stream still yields `*UntrustedTapError`. `diagnostic_test.go` calls `joinCommandStreams`, `summarizeDiagnostic`, `diagnosticLines`, `isErrorLine`, `isFailureLine` and `elideMiddle` directly; `runner_test.go` drives fake scripts through the bundle split-stream case, the one-line summary, the silent-exit fallback, untrusted-tap detection on stdout, and a read-only failure that keeps its full stderr.

A deadline and a cancellation never produce the same message, and neither surfaces as `signal: killed` — the process is killed by ChairLift's own `Cancel` func, so the raw wait error is replaced by the classified one. `Error` carries an `Err error` field and an `Unwrap() error` method, so `errors.Is(err, context.DeadlineExceeded)` works for callers while `Error` keeps satisfying `error` and keeps its human-readable `Message`; nothing in the views switches on its concrete type. `internal/homebrew/runner_test.go` covers each outcome against a fake script, asserts the deadline and cancellation messages differ and contain no `signal: killed`, and `TestRunBrewCommandAtKillsProcessGroup` proves the process-group kill by having the fake script spawn a background `sleep`, recording its PID, and polling `syscall.Kill(pid, 0)` until it returns `ESRCH` — proving the helper is gone, not merely that the parent returned.

### Tap trust (Homebrew 6) (`internal/homebrew/trust.go`)

Homebrew 6 introduced per-tap trust: formulae/casks from a tap that isn't marked trusted are invisible to normal `brew` operations. Critically, **`brew list`/`brew info` also refuse to load untrusted-tap formulae**, so there is no supported `brew` command that lists what's installed-but-untrusted — ChairLift has to reconstruct that set itself from on-disk state.

**Detection (`ListUntrustedTaps`)** combines three sources:
1. `brew tap-info --installed --json` — parsed for each tap's `name` and `trusted` flag (`parseUntrustedTapNames`); this is the only brew-provided signal, and it tells you *which taps* are untrusted but not *what's installed from them*. `runBrewCommandAt` runs every `tap-info` invocation with `HOMEBREW_NO_GITHUB_API=1` appended to the inherited environment: the JSON also reports whether each tap is private, which brew asks the GitHub API, and on Linux that lookup's credential probe (into a nonexistent `git credential-osxkeychain`) intermittently kills the command with `Error: Broken pipe`. `trusted` is local state, so nothing ChairLift reads is lost. No other brew command gets the variable (`TestRunBrewCommandAtKeepsTapInfoOffTheGitHubAPI`).
2. Cellar keg receipts (`installedFormulaeByTap`) — walks `<prefix>/Cellar/<formula>/<version>/INSTALL_RECEIPT.json` and reads `.source.tap`, since brew's own listing commands can't see these formulae. One receipt per keg is enough to attribute the formula to a tap.
3. Cask install receipts (`installedCasksByTap`) — reads `<prefix>/Caskroom/<token>/.metadata/INSTALL_RECEIPT.json` and takes `.source.tap`, mirroring the formula path. Homebrew's `Cask::Tab.create` writes that receipt directly in the cask's `.metadata` container, one level above the versioned subdirectories, and it is the authoritative origin recorded at install time. Tokens with no readable receipt, malformed JSON, or an empty `source.tap` are skipped, as are hidden directories. API-installed casks record `homebrew/cask`, which `tap-info` reports as trusted, so they never reach the result. The earlier scan of versioned `.metadata/*/*/Casks/*.json` metadata could not see tap-sourced casks at all: a cask installed from a tap is saved as a `.rb` Caskfile, so no `Casks/<token>.json` exists for it, and the casks the remediation UI exists for were silently dropped.

Only untrusted taps with at least one installed formula or cask are returned (`UntrustedTap{Name, Formulae, Casks}`, package names fully qualified as `tap/name`, ready to pass straight to `brew trust`); taps with nothing installed aren't actionable and are dropped.

**`TrustPackages(tap)`** runs `brew trust --formula <formulae...>` and/or `brew trust --cask <casks...>` for the given tap. This is a **per-user** operation (state lives in `~/.homebrew/trust.json`) — it does not use `pkexec` and does not require root, unlike bootc staging or updex writes.

Because `brew trust --formula/--cask ...` has `args[0] == "trust"`, and `trust` is one of homebrew's ten `stateChangingCommands`, `TrustPackages` runs under the 30-minute mutation timeout rather than the 30-second read budget — trusting a tap can trigger substantial work — and, by the same map membership, no-ops under dry-run at the exec layer (see "Cross-cutting: dry-run" below). A nil error from a dry run therefore does not mean anything was trusted. The Updates page shows each untrusted tap with installed packages as a warning banner in the Homebrew source's card (`brewSource.Extra`, text from `pageview.BrewTrustBanner`); the banner's close hides it until the next check, and the source's note counts the untrusted taps. The banner's action and the palette's "Trust <tap>" command call `Brew.AskTrust`, which takes the tap's gate and asks through `Env.Dialogs`, listing the packages (`pageview.UntrustedTap`). On confirm `runTrust` queues `homebrew.TrustPackages` as a job. On success it computes `decision := actionmsg.TapTrust(b.dryRun(), tap.Name)` once and gates every UI change on `decision.MutateUI` (exactly `!dryRun`): when true, the tap leaves the untrusted list (so its banner goes and its packages are no longer held), its gate completes, an activity entry is recorded, and `loadOutdated` reloads the outdated packages, since newly trusted packages may now show; when false, the tap and its banner stay, the gate is restored, and the outdated list is not re-queried. `decision.Toast` — a preview string under dry-run, "Trusted %s. Its packages can update again." otherwise — is shown in both states. A failure restores the gate with an error toast. This mirrors the `actionmsg.MaintenanceScript`/`ScriptDecision` pattern: the UI-mutation gate itself, not just the toast wording, is what `actionmsg_test.go` asserts.

**`UntrustedTapError`** — `runBrewCommandAt` (`internal/homebrew/homebrew.go`) inspects failed commands' diagnostic text (stderr for a read-only command; stdout and stderr joined for a state-changing one, see "Non-zero-exit diagnostics" above) for `"untrusted tap"` or `"taps are not trusted"` (`isUntrustedTapMessage`) and returns `*UntrustedTapError` instead of the generic `Error` — only on the non-zero-exit path, and only after the deadline and cancellation branches have been ruled out, so a timed-out or cancelled command is never misreported as a trust problem. The type is unchanged and unwrapped by the classification, so the one type-based dependency in the whole UI — `errors.As(err, &trustErr)` in `Brew.finishUpgrade` (`internal/views/brew_source.go`) — keeps working and turns the failure into the trust message and the tap's banner rather than raw brew output.

The upgrade-failure toast text adapts to whether the trust banner can show. `Brew.finishUpgrade` calls `trustmsg.UpgradeMessage(name, b.enabled(brewUpdatesPage, groupBrewTrust))` (`updates_page`/`brew_trust_group`) (`internal/views/trustmsg`, see "View-layer toast and decision helpers" below) and reloads the untrusted taps, so the tap's banner appears. With `brew_trust_group` on, the message points to the banner ("trust the tap in the Homebrew card of Updates"); with it off, the message is self-contained — it states the package can't be upgraded until its tap is trusted, with no reference to a part of the page that does not exist.

**Cross-group safety** — `brew_trust_group` and `brew_updates_group` are independent. `runTrust` (reachable with only `brew_trust_group` on) calls `loadOutdated`, which checks `brew_updates_group` first and does nothing (its `done` callback still runs, with false) when that group is off; `loadTaps` likewise returns at once when `brew_trust_group` is off, and the source's items are empty unless `brew_updates_group` is on. That is the config-driven-visibility invariant: code reachable from another group's action checks its own group before it touches that group's state.

### View-layer page presentation (`internal/views/pageview`)

`internal/views/pageview` is one of the seven pure leaf packages under
`internal/views/` (the others are `actionmsg`, `actionstate`, `bundleview`,
`featurestatus`, `flatpakstatus` and `trustmsg`, each in its own subsection
below). None of them imports a UI package. `pageview` owns the presentation
text and small decisions the command-center pages draw: the views draw it
with MyGo and compose no row text, status text, Help-link inventory,
os-release parsing or maintenance invocation of their own. Since
[ADR-0017](../adr/0017-mygo-command-center-window.md) the views themselves
hold headless tests (`ui.NewTester`), but the decisions stay in these
packages, where a table-driven test of a function is cheaper and clearer
than a frame ([engineering lessons](engineering-lessons.md#no-tests-in-puregotk-packages)).
The layout began as
[ADR-0007](../adr/0007-pure-leaf-packages-route-around-untestable-gtk.md),
which ADR-0017 supersedes.

Its exported outcomes are:

- `BrewBundle` returns the path alone or `description — path`, and
  `SearchResult` preserves the result's typed Formula/Cask label.
  `FlatpakApplication` (the application ID alone, or `ID (version)`) and
  `HomebrewPackage` (the version, plus the pinned marker) are kept and tested,
  but no command-center view calls them: the Apps table draws the version and
  the pinned state in columns of its own.
- `UntrustedTap` combines formulae and casks in that order, strips each
  tap-qualified package prefix for display, and includes the installed count;
  the trust confirmation lists it. `FlatpakUpdate` always includes the
  application ID, adds the version arrow only when a new version exists, and
  adds the user-installation suffix only for user updates; the Flatpak source
  takes its title.
- `brew_text.go` holds the Homebrew text of the command center:
  `BrewfileEntries` lists the packages a Brewfile installs (every `brew`,
  `cask`, `mas`, `vscode`, `flatpak`, `whalebrew`, `go` and `cargo` line, in
  order; comments, blank lines, `tap` lines and unreadable lines are left out,
  and a formula's or cask's tap prefix is dropped), for the bundle cards;
  `BrewShortName` drops a tap prefix (`ublue-os/tap/bluefin-cli` is
  `bluefin-cli`); `BrewCount` gives "1 package" or "3 packages";
  `BrewTrustBanner` is the untrusted-tap banner of the Updates page ("Homebrew
  skips <packages> because <tap> is not trusted. Trust the tap to update its
  packages again."; more than four package names are summed up as "a, b, c and
  N more"); `BrewSourceNote` is the line under the Homebrew source's title
  ("3 outdated · 1 held · 1 untrusted tap", "Up to date", or nothing before
  the first check); `BrewSearchStatus` says what the search field searched
  for and found; and `BrewInstallCommand` is the exact `brew install
  [--cask] <name>` an install from a search result runs, which the result
  row and its confirmation show.
- `FirmwareUpdate` titles the row with the device name (falling back to
  `Unknown device`) and builds the subtitle from the parts that exist —
  `Vendor · 1.2 → 1.3` — omitting an empty vendor and keeping only the version
  that is known. `FirmwareUpdatesSubtitle(count, err, retainedRows)` returns a
  `Status`: `Firmware is up to date` for zero updates, singular or plural
  "update available" text otherwise, and `Error checking firmware updates:
  <err>` when the query failed; the firmware source shows its text as its note
  or its error, and does not use the `Expandable` field.
  `FirmwareInstallConfirmation` returns the confirmation dialog's heading and
  body; the body always carries the power warning, then the restart sentence,
  the shutdown sentence, or the "some devices restart or disconnect" sentence,
  and always ends with `ChairLift will not restart the computer for you.`
  `FirmwareBlockedUpdate` returns the same row text for a device fwupd cannot
  flash now, appending the reason after ` · `; that item is held.
- `BootcUpdateSubtitle` distinguishes not staged, staged without a version,
  and staged with a version. `BootcStageResultSubtitle` returns that staged
  text after a staged action, otherwise preserves the stage script's final
  message when one exists and falls back to `System is up to date`. The
  native A/B counterparts are `SysupdateUpdateSubtitle`,
  `SysupdateStageResultSubtitle` and `SysupdateRollbackSubtitle`. The system
  image source uses them as its note.
- `system_overview.go` holds the system text of the Overview and the status
  bar: `ImageChannel` is the update channel an image reference names (its
  tag, looked for in the last path element only, so a registry port is not
  one; a reference by digest or without a tag names none), `StagedNote` is
  the system image source's note while an image is staged ("Staged <version>
  · applies on restart"), and `AppDisplayName` names a desktop application by
  its ID for a button that opens it ("Mission Center" of
  `io.missioncenter.MissionCenter`; an ID with no capitals stays as it is).
- `Feature` maps the updex description/name to title/subtitle (the Features
  page takes its title), while `FeatureGroupDescription` formats the loaded
  feature count before a check ended.
- `HelpResources` emits Website, Report Issues, and Community Discussions in
  that fixed order while omitting every unconfigured URL; the Help dialog
  lists them.
- `MaintenanceCommand` returns a direct script invocation for an
  unprivileged action and the exact `pkexec <script>` shape for a privileged
  action.
- `ParseOSRelease` ignores comments, blank lines, and lines without `=`;
  splits a retained line at its first `=`; removes quote characters at the
  value's edges;
  title-cases the key; marks `*URL` fields as links; and returns scanner
  failures. `ShortDigest` leaves digests of 19 characters or fewer unchanged
  and truncates longer values to 19 characters plus an ellipsis.
- The Problems page ([ADR-0015](../adr/0015-problems-page-and-opt-in-reporting.md)):
  `Problem` returns an item's title (`problems.Title`) and a subtitle from
  `ProblemWhen` ("Today at 15:04", "Yesterday at 15:04", "On Nov 5 at
  15:04", "On Nov 5, 2025 at 15:04", "N times, last …", or "Time unknown")
  plus the unit, feature, version, code, or an unknown kind's name.
  `ProblemsSystemStatus` returns the system section's description and its
  single status row for each problems.json outcome: `SourceMissing` gives
  "System checks are not available on this system"; `SourceUnreadable` and
  `SourceMalformed` give "System checks could not be read"; `SourceOK` with
  no item gives "No problems found" and "Last checked <when>"
  (`LastChecked`), and with items gives no row. `ProblemsAppsStatus` gives
  "No problems found" for an empty apps section, and both add the count of
  dismissed problems to the description. `ProblemDetails` returns the
  content of an open problem card: the hint, the recorded values (a field
  that failed its rule is marked "not valid", a missing one "not recorded"),
  and the local details when there are any. `problems_card.go` holds the
  command-center text: the section headers (`ProblemsSystemSection`,
  `ProblemsAppsSection` and their notes), `ProblemCardText` (a card's title,
  "<subject> · last seen <when>" subtitle from `ProblemSubject`, count
  "once"/"<n> times" from `ProblemCount`, "First seen <when>", and
  `ProblemSeverity`: an integrity error is danger, a TPM unlock that worked
  after a retry, an app error, a weekly count and an unknown kind only
  inform, every other kind is a warning), `ProblemsSubtitle` (the page
  header: how many problems and whether anything leaves the computer),
  `ProblemsSystemNotes` and `ProblemsAppsNotes` (the section headers' "Last
  checked" and dismissed-problem notes), `ReportingCardTitle`,
  `ReportingLimits`, `ReportingSentSummary` and `ReportingSendPending`, and
  `ReportingRunToast` (what a "Send pending reports" run did).
  `pageview/reporting.go` holds the rest of the reporting card:
  `ReportingDescription`, `ReportingWhat`, `SentRows`, `SentKindTitle`,
  `ReportingCountRow`, `ReportOffered` (the Report… button's rule),
  `ReportSummary`, `ReportButtonLabel` and `ReportedLabel`, so the phase-1
  promise row is gone. `ReportingMode` is kept and tested, but the mode
  control draws the `ReportingModeAsk`/`ReportingModeAuto` constants itself.
  `internal/views/problems_view.go` composes no text of its own.

The package's tests (`pageview_test.go`, `brew_text_test.go`,
`problems_test.go`, `problems_card_test.go`, `reporting_test.go`,
`system_overview_test.go`) call every exported function directly and
table-test every branch above. `wiring_test.go`
(`TestPageBuildersUsePurePresentations`) maps every non-test Go file of
`internal/views` to one domain (`viewDomains`: shared, apps, updates,
overview, features, problems, maintenance) and fails when a file belongs to
none. For the apps, updates, maintenance, features, problems and overview
domains it requires the `pageview`, `actionmsg`, `flatpakstatus`,
`bundleview`, `trustmsg`, `featurestatus` and `problems` calls that domain
must make, and rejects the retired inline implementations: an inline bundle
or Flatpak update `fmt.Sprintf`, a `"pkexec"` literal in the apps and updates
domains, an inline `pkexec`/script `exec.CommandContext` in Maintenance, the
inline feature count, the Problems page's own titles, hints, commands and the
phase-1 "nothing is sent" promise, and an inline os-release title-case or
digest cut.

### View-layer toast and decision helpers (`internal/views/actionmsg`, `internal/views/trustmsg`)

Two of the seven pure leaf packages under `internal/views/` hold the text
and, at the call sites that also change state on success, the accompanying
decision that the views use once a wrapper call returns. Each decision is one
tested function that gates the toast and the state change together, so a
table-driven test proves the mutation gate and the toast cannot drift apart.
Decision records: [ADR-0009](../adr/0009-dry-run-output-convention-and-single-decision-structs.md)
(the decision-struct rule these packages implement) and
[ADR-0017](../adr/0017-mygo-command-center-window.md) (the views are tested
headless, and the decisions stay in the leaf packages).

Every caller reads the dry-run flag once per decision: the Homebrew, Flatpak
and firmware domains read their wrapper's flag (`homebrew.IsDryRun`,
`flatpak.IsDryRun`, `fwupd.IsDryRun`, through a function field), the system
image reads `bootc.IsDryRun` or `sysupdate.IsDryRun`, and the Maintenance,
Features and Problems domains read `Env.DryRun`, the `--dry-run` flag the
app passes to `views.NewEnv`.

- **`internal/views/trustmsg`** (added for issue #57) — `UpgradeMessage(pkgName string, trustGroupAvailable bool) string`, the toast shown when a Homebrew upgrade fails with an `*homebrew.UntrustedTapError`; `Brew.finishUpgrade` calls it (see "Tap trust" above).
- **`internal/views/actionmsg`** (added for issue #56 and extended for issue #8) — builds the toast text for every state-changing action the pages start, and, at each call site where the view also changes a row, a control or a list on success, the execute/complete/mutate/confirm decision itself, so the same table-driven test in `actionmsg_test.go` that checks the toast also checks the gate (see "Dry-run mode" in [overview.md](./overview.md#dry-run-mode) for the general rule this implements). Exported surface, with its callers:
  - `ScriptDecision{Execute bool; Toast string}` + `MaintenanceScript(dryRun bool, title string) ScriptDecision` — `Maintenance.job` (`internal/views/maintenance.go`) computes it once, on the UI thread, before it queues the job; when `Execute` is false the job constructs no `exec.Cmd` at all (c1)
  - `BundleDump(dryRun bool, path string) string` — the Brewfile export's toast (`Brew.runExport`) (c1)
  - `BundleInstallDecision{Complete bool; Toast string}` + `BundleInstall(dryRun bool, name string) BundleInstallDecision` — `Brew.runBundle`: completes the bundle's gate in live mode, or resets it after a dry-run preview (issue #8)
  - `Cleanup(dryRun bool, tool, output string) string` — the Homebrew/Flatpak cleanup toast (`Maintenance.toolDone`) (c1)
  - `Install(dryRun bool, pkgName string) string` — the Homebrew search install toast (`Brew.runInstall`) (c2)
  - `Uninstall(dryRun bool, name string) string` — the Homebrew (`Brew.runUninstall`) and Flatpak (`Flatpak.finishUninstall`) uninstall toast (c2)
  - `Pin(dryRun bool, name string, pin bool) string` — the Homebrew formula pin/unpin toast (`Brew.runPin`) (c2)
  - `Upgrade(dryRun bool, pkgName string) string` — the Homebrew per-package upgrade toast (`Brew.finishUpgrade`) (c3)
  - `Update(dryRun bool, appID string) string` — the Flatpak per-app update toast (`Flatpak.finishUpdate`) (c3)
  - `SelfUpdate(dryRun bool, tool string) string` — the `brew update` metadata toast (`Brew.UpdateMetadata`) (c3)
  - `TapTrustDecision{MutateUI bool; Toast string}` + `TapTrust(dryRun bool, tapName string) TapTrustDecision` — `Brew.runTrust`: gates whether the tap leaves the untrusted list (and its banner goes), its gate completes, and the outdated packages reload (c3)
  - `BootcStage(dryRun bool, staged bool) string` and `SysupdateStage(dryRun bool, staged bool) string` — `System.finishStage`'s completion toast; string-only since the system image's note stays live in both modes and there is no mutation left to gate (c4)
  - `FirmwareInstallDecision{Toast string; IsError bool; RestoreControl bool; PublishList bool; CompletedLabel string}` + `FirmwareInstall(dryRun bool, installErr error, report fwupd.InstallReport, name string, needsReboot, needsShutdown bool) FirmwareInstallDecision` — `Firmware.installJob` builds it on the job's goroutine and `Firmware.finishInstall` applies it, from the device's own report rather than from an exit status or the shape of the update offer. Decision table: an install error gives `Firmware install failed: <err>` with an error toast and the button restored; dry-run gives the `[DRY-RUN] Preview: firmware for <device> would be installed — no changes made` preview with the button restored; a flashed install gives the completion text naming the restart, the shutdown, or nothing afterwards; a staged install gives `Firmware for <name> is ready. Restart the computer to finish.`; a not-applied install gives `Firmware for <name> was not installed: <reason>` with an error toast; an unverified install gives `fwupdmgr finished for <name>, but ChairLift could not check the result: <reason>` with an error toast. `PublishList` is true for every live result whose `report.ListErr` is nil. With a failed list the old items stay, so a flashed or staged install sets `CompletedLabel` (`Installed` or `Restart required`), which holds its item and keeps the install from being offered again, while a not-applied or unverified result restores the button
  - `ProblemDismissDecision{Persist bool; Hide bool; Toast string; Log string}` + `ProblemDismiss(dryRun bool, id, title string) ProblemDismissDecision` — `Problems.dismiss`: under dry-run nothing is written or hidden, the toast is `[DRY-RUN] Preview: dismiss <title> — no changes made` and the log line `[DRY-RUN] would dismiss problem <id>`; live, the page writes the dismissal and hides the card only after the write succeeds (`Dismissed: <title>`). `ProblemDismissFailed(title, err)` is the error toast of a failed write, which keeps the card, and `ProblemDetailsCopied(title)` the toast of "Copy details" (ADR-0015)
  - `ReportDecision` + `Report(dryRun bool, kind string)`, `ReportFailed(kind string, err error)`, and `ConsentDecision` + `ReportingConsent(dryRun, enabled bool, mode string)` — the Report dialog's send (`Problems.sendDialogReport`) and the consent switch (`Problems.applyReportingConsent` and the first-run Reporting step); see [specs/problem-reporting.md](../specs/problem-reporting.md)
  - `FeatureToggleDecision{Confirm bool; Toast string}` + `FeatureToggle(dryRun, enable bool, name string) FeatureToggleDecision` — `Features.toggled`: `Confirm` decides whether the feature's row and tile take the new state (and an activity entry and a reload follow) or keep the listing (c5)
  - `FeatureUpdateDecision{Toast string; Persistent bool; Recheck bool}` + `FeatureUpdate(dryRun bool, results []updex.FeatureUpdate, err error) FeatureUpdateDecision` — `Features.updated`: the features update's toast, whether it persists as an error toast, and whether the caller must run the update check again. `Recheck` is true for every outcome that ran the update — errors and failures included, because a failed run can still change state in part — and false only for the dry-run preview, where nothing ran; the domain then calls `recheckFeatureUpdates`, so the rows show the state after the update instead of the startup check's state (see the `featurestatus` section's re-check paragraph). It reads the per-feature results because updex returns a nil error when a feature is held (see the Updex section's "Update results"). Decision table: dry-run gives the preview; an error with no results gives "Features merged, but activation did not finish. Reboot to finish." for `*updex.ActivationIncompleteError` or "Update failed: <err>" for any other error (both persistent); nil results with a nil error, and `*updex.ResultsUnreadableError`, give "Feature update finished, but ChairLift could not read the results. See the log for details." (persistent; the outcome is unknown, so no success claim); empty results give "No enabled features to update."; only updated (and current) features keep the old text exactly; only current features give "Features are already up to date."; anything else joins, in order, "Updated: a, b.", "Waiting for a newer OS image: c (reason); ...", "Held by a rollback: e (reason).", "Failed: f (reason); ...", then "Changes apply after reboot." (when something updated and activation is complete), then the activation sentence or, when no feature failed, "Update failed: <err>"; persistent when any feature waits, is held or failed, or the error is set

  The plain-`string` functions (`BundleDump`, `Cleanup`, `Install`, `Uninstall`, `Pin`, `Upgrade`, `Update`, `SelfUpdate`, `BootcStage`, `SysupdateStage`) select toast wording only. Where an application action also changes row controls or requests an inventory refresh, the separate tested `actionstate` decision owns that effect. The decision-struct functions in `actionmsg` exist because their call sites have no wrapper- or `actionstate`-level gate for the *second* effect: script execution has no wrapper package; a bundle must distinguish a real completion from a dry-run wrapper success; a tap's removal from the untrusted list and a feature's new state are view-local state that the wrapper's own dry-run skip does not touch; the features update toast must read the per-feature results to tell a real update from a held one; a firmware install must publish a verified device list only when fwupd's own verification reported a real outcome; a Problems dismissal must hide its card only after a real write; and a report or a consent change must send or persist nothing in a dry run.

### View-layer update action state (`internal/views/actionstate`)

`internal/views/actionstate` is one of the seven pure leaf packages under
`internal/views`. It owns the state machines and complete outcome tables for
the Homebrew and Flatpak actions of the Apps and Updates pages:

- `Gate.TryStart` atomically moves idle to running and rejects every repeated
  callback while running; `Reset` makes a failed, previewed, or fully-refreshed
  action retryable; `Complete` permanently closes a live-upgraded row action.
  Every Homebrew control holds one (`brewAction` in `internal/views/brew.go`),
  and so does the firmware install (`Firmware.installGate`).
- `RefreshGate.Begin` assigns an increasing generation to each refresh and
  `IsCurrent` accepts only the newest, preventing a slower old query from
  publishing after a newer one. The Homebrew inventory, outdated packages,
  untrusted taps and search, the Flatpak installed apps and updates, the
  firmware list, the features listing and check, the system deployments and
  the Problems load each take their own.
- `PackageUpgrade(succeeded, dryRun)` returns exactly three outcomes: failure
  restores the control without changing rows; dry-run success also restores
  it without a refresh; live success requests both immediate row removal and
  a full refresh. `Brew.finishUpgrade` and `Flatpak.finishUpdate` use it.
- `PackageInstall`, `PackageUninstall`, and `PackagePin` share the installed
  inventory mutation outcomes: failure and dry-run success restore the row
  controls without a refresh; live success completes the old controls and
  requests a generation-guarded installed-package refresh.
  `Brew.runInstall`, `Brew.finishMutation` and `Flatpak.finishUninstall` use
  them.
- `MetadataUpdate(succeeded, dryRun)` returns exactly three outcomes: failure
  and dry-run success restore the top-level control without refreshing; live
  success requests a refresh and deliberately does not restore the control
  until that refresh completes (`Brew.UpdateMetadata`).
- `OutdatedRefresh(succeeded, currentCount, discoveredCount)` returns exactly
  two outcomes: failure keeps `currentCount` and does not authorize row
  replacement; success authorizes replacement and adopts `discoveredCount`,
  including zero. `Brew.loadOutdated` uses it, and the Flatpak update reload
  (`Flatpak.applyUpdates`) reuses it with
  `flatpakstatus.Result.Authoritative` as `succeeded`, so a load in which no
  installation returned a usable result keeps the last known Flatpak rows and
  count. An installation is usable when its query succeeds or when it returns
  one or more updates together with an error.
- `OutdatedPresentation(count)` returns `0 packages available` with expansion
  disabled for zero, `1 package available` with expansion enabled for one,
  and `%d packages available` with expansion enabled for larger counts. It is
  kept and tested, but no command-center view calls it: the Homebrew source's
  note comes from `pageview.BrewSourceNote`.

`actionstate_test.go` table-tests every outcome, races 64 callers against one
action gate (requiring exactly one acquisition), and proves 64 concurrent
refresh requests receive unique generations with exactly one current.
`wiring_test.go` (`TestViewsUseTheActionStateDecisions`) reads
`internal/views`' source and requires, per file, the generation guards and
decisions above: `flatpak.go` (the apps and update `RefreshGate`s,
`OutdatedRefresh` with `r.status.Authoritative`, `PackageUpgrade`,
`PackageUninstall`), `brew_apps.go` (the inventory gate, `PackagePin`,
`PackageUninstall`), `brew_source.go` (the outdated and taps gates,
`OutdatedRefresh`, `PackageUpgrade`, `MetadataUpdate`), `brew_search.go`
(the search gate, `PackageInstall`, and the result kind carried into
`brew install [--cask]`) and `brew.go` (the shared `actionstate.Gate`). The
outcomes themselves are behavior-tested in `internal/views`
(`flatpak_test.go`, for example `TestFlatpakNonAuthoritativeReloadKeepsKnownRows`
and `TestFlatpakLiveUpdateRemovesRowBeforeTheReload`, and `brew_test.go`, for
example `TestBrewFailedRefreshKeepsLastKnownRows` and
`TestBrewStaleGenerationsAreIgnored`).

### View-layer Brew bundle state (`internal/views/bundleview`)

`internal/views/bundleview` is one of the seven pure leaf packages under
`internal/views`. It owns the Bundles tab's load presentation and its
per-bundle concurrency state, leaving `internal/views/brew_bundles.go` to
draw the cards only.

`Present(count, warning, homebrewAvailable) Presentation` enumerates the
tab's outcomes: zero bundles without a warning produces the
`No bundles available` placeholder; zero with a warning produces the
`Bundles unavailable` placeholder carrying that warning; one bundle uses a
singular available description; several use a plural description; partial
results append the warning while keeping their cards; and any of those states
with Homebrew unavailable appends that install actions are disabled.
`Brew.loadBundles` calls it once with the warning and once without (the
tab's plain description, which does not repeat the warning). The view
composes none of this text itself.

`InstallGate` is zero-value-ready and has three states. `TryStart` atomically
moves idle to running and rejects every concurrent caller; `Reset` returns a
canceled, failed or dry-run action to idle; `Complete` permanently closes a
live successful action. `Brew.AskBundle` takes it before the confirmation,
and `Brew.runBundle` resets or completes it when the job ends.
`bundleview_test.go` races 64 callers against one gate and asserts exactly
one acquisition, then separately covers reset and completion.

### View-layer Flatpak update status (`internal/views/flatpakstatus`)

`internal/views/flatpakstatus` is one of the seven pure leaf packages under `internal/views/`. It turns the outcome of the two Flatpak update queries — how many updates are known, and which of the user/system installations could not be checked — into the Flatpak update source's status line plus whether the load may replace the known rows. Like `trustmsg` it imports nothing outside the standard library (`fmt`).

Exported surface:

- `Result{Subtitle string; Expandable bool; Authoritative bool}` — the state of one update load. `Authoritative` is `!userFailed || !systemFailed || count > 0`: true when at least one installation returned a usable result, where a usable result is a successful query or an errored query that returned one or more updates. It is false only when both installation queries failed and found no updates, which tells the caller the load learned nothing and must not replace the known rows or count. `Expandable` is `count > 0`; the command-center source does not use it.
- `Subtitle(count int, userFailed, systemFailed bool) Result` — derives that state. All three fields come from a single call so the wording and the replacement decision cannot drift apart, the same reason `actionmsg` returns `ScriptDecision`/`TapTrustDecision`/`FeatureToggleDecision` structs. Failure is taken as two `bool`s rather than `error` values, which is what keeps the package free of any dependency on `internal/flatpak`.

A failed query never invents updates. The distinguishable outcomes are: both queries ok with no updates → `All applications are up to date` (the only case that makes the up-to-date claim); both ok with updates → `1 update available` / `%d updates available`; exactly one query failed with no updates → `No updates found in the <ok> installation; the <failed> installation could not be checked`; exactly one failed with updates → the count followed by `; the <failed> installation could not be checked`; both failed with no updates → `Could not check for updates`, which makes no claim about update state; both failed with updates → `<count> update(s) available; the user and system installations could not be fully checked`, which publishes the healthy remotes' updates. `<ok>`/`<failed>` are the literal words `user` and `system`. `flatpakstatus_test.go` has one subtest per row (with both the user-failed and system-failed variants of the one-failed rows and both-failed count 0, 1, and 3 cases), and additionally asserts that all the subtitles are pairwise distinct, that "up to date" appears in the first case and no other, that singular and plural both read correctly, that `Authoritative` is true when at least one result is usable, and that both-failed with no updates is non-authoritative.

The package is pure and holds no state, so it is safe to call from a worker goroutine.

`Flatpak.loadUpdates` (`internal/views/flatpak.go`) is its only call site. It runs on the UI thread, takes a generation from `f.flatpakUpdatesRefresh.Begin()` (an `actionstate.RefreshGate`), and starts its worker with `views.Go`; the first load, `fpSource.Refresh` and a completed live per-app update all call it this way, so overlapping reloads take generations in the order they were requested. The worker keeps both `flatpak.ListUpdates` errors as values — each logged as `Error loading {user,system} flatpak updates: …` — appends each installation's returned updates whether or not its error is nil (a `*flatpak.RemoteError` still carries the healthy remotes' updates; an aborted query returns none), and calls `flatpakstatus.Subtitle(len(updates), userErr != nil, systemErr != nil)` once, on the worker goroutine. `Flatpak.applyUpdates` runs on the UI thread through `Env.Post`; its first statement returns when `!f.flatpakUpdatesRefresh.IsCurrent(generation)`, so a slower, older reload can neither re-add a row a newer one retired nor overwrite a newer count. It keeps the `Result` as the source's status (its `Subtitle` is the note, or the error when a query failed) and replaces `f.updates` only when `actionstate.OutdatedRefresh(r.status.Authoritative, len(f.updates), len(r.updates))` authorizes it. The update count is the length of those rows: `fpSource.Items` lists them, and `Store.UpdateCount` sums every source's items, so the count and the rows cannot drift apart and no separate counter exists. A host without `flatpak` makes the source unavailable (`Flatpak not installed`). The view holds no status text and makes no decision of its own.

The practical consequence is that a total failure — no installation returned a usable result — no longer renders as an all-up-to-date message and no longer wipes what was known: with both installations errored and no updates found, the source reads `Could not check for updates`, while the items of the last authoritative load stay (on the very first load that is no items). A partial failure is identified as partial rather than silently under-reported: when both installations return a `RemoteError` together with updates, the load is authoritative and the source names both installations; when only one installation failed, the text names that installation. `actionstate/wiring_test.go` pins the generation guard and the `OutdatedRefresh` routing; `flatpak_test.go` (`TestFlatpakNonAuthoritativeReloadKeepsKnownRows`, `TestFlatpakPartialFailureKeepsHealthyRows`, `TestFlatpakOverlappingReloadsKeepTheNewest`) proves the outcomes; and `flatpakstatus_test.go` pins the issue #263 partial-result rule with `TestBothInstallationsPartialPublishesHealthyUpdates`.

### View-layer feature update status (`internal/views/featurestatus`)

`internal/views/featurestatus` is one of the seven pure leaf packages under `internal/views/`. It owns every string and every decision the Features page's updex update check needs: a feature's update-status line, whether that feature has an update, and the features list's description. Unlike the others it imports one non-standard-library package, `internal/updex`, for the `CheckResult` type; `internal/updex` imports no UI package, so `go list -deps ./internal/views/featurestatus` names no MyGo or frostui package.

Exported surface:

- `Status{Subtitle string; HasUpdate bool; WaitingForImage bool; PendingReboot bool}` — the row state for one feature. `WaitingForImage` is true when a component of the feature needs a newer OS image; the callers count that feature separately from the updatable ones and never as an update, which is why `HasUpdate` is false in the same `Status`. `PendingReboot` is true when the feature has no update and no wait but a component whose new version is linked and becomes active at the next boot; such a feature is counted as neither an update nor a wait.
- `Feature(name string, results []updex.CheckResult) (Status, bool)` — derives that state from *all* of the feature's components. Both halves come from a single call so the wording and the update decision cannot drift apart, the same reason `flatpakstatus.Subtitle` returns a `Result`. The second return value is `false` when `len(results) == 0`, telling the caller not to derive a status from it and not to count the feature (`Summarize` leaves it out of `Rows`, so the page shows the row's base subtitle); putting that skip decision in the package is what makes the zero-components case table-testable at all.
- `Summary{Rows map[string]Status; Updates int; Waiting int}` and `Summarize(names []string, checks []updex.FeatureCheck) Summary` — the whole check at once: one `Status` per feature the page lists and the check answered for, plus the two counts `GroupDescription` takes. A check entry whose feature is not in `names` is ignored (not in `Rows`, not counted), and a feature whose check reported no components is absent from `Rows`. `Updates` and `Waiting` are counted from `Rows`, so a feature is counted once however many check entries name it (the last entry wins, as the row's subtitle does). **A name that is not in `Rows` must get its base subtitle back**: `Features.load` stores each row's `baseSubtitle` in `f.baseSubtitles` when it lists the features (empty, as the switch says on or off and the version has its own column, or "Masked by the administrator: it cannot be turned on here"), and `Features.subtitle` falls back to it for every name `Summarize` did not answer for, so a stale "update available" from an earlier check cannot stay after an update changed the state.
- `GroupDescription(totalFeatures, featuresWithUpdates, featuresWaiting int) string` — the description after a check that completed. Both counts are feature counts and they are disjoint: a waiting feature is counted in `featuresWaiting` only.
- `GroupDescriptionCheckFailed(totalFeatures int) string` — the description when the check itself failed; it makes no claim about update state.

**The ANY-component rule:** a feature has an update when **any** of its components reports one — not the first, not all. `Status.HasUpdate` is an OR across every element of `results` (unless the waiting rule below applies), and `featurestatus_test.go` asserts it by iterating every element of each case's slice rather than special-casing index 0, with cases placing the update first, last, in the middle, and in several components at once.

**The waiting rule:** `updex.CheckResult.NeedsImage` is set when a component's newest *staged* image is not linked and the next `features update` will not link it, because its `UPDEX_MIN_IMAGE_VERSION` is above the image that boots next (updex ADR-0016). That is not an error and not an update: the feature stays unlinked until a newer OS image boots. Because updex links nothing of a feature when one of its transfers needs an image, one waiting component makes the whole feature wait, so the waiting branch has **precedence** over every update branch: the `Status` is `WaitingForImage: true` and `HasUpdate: false` even when another component reports `UpdateAvailable`. The subtitle names every waiting component with its image in input order, and a waiting feature bumps `featuresWaiting`, never `featuresWithUpdates`.

**The feature-counting rule:** `featuresWithUpdates` is a count of **features**, not of components — a feature with three outdated components counts once. The package doc comment states this explicitly, since the description's first number is a feature count and its second must be one too or the sentence is incoherent. `featuresWaiting` is a feature count for the same reason.

**The pending-reboot rule:** `updex.CheckResult.PendingReboot` is true when a component's link names a version other than the merged one, so the new version takes effect at the next boot. That is neither an update nor a wait, but it is not nothing either, so `Feature` reports it once neither of those applies and gives the row the subtitle `<name> — changes apply after reboot` — the same words as the update action's `Changes apply after reboot.` toast. A pending-reboot feature is counted as neither an update nor a wait.

The seven subtitle branches, in precedence order, for a feature named `<name>`:

| Situation | Subtitle |
|-----------|----------|
| at least one component needs a newer OS image (takes precedence over every row below) | `<name> — waiting for a newer OS image (<component>: needs image >= <version>[, ...])` |
| exactly one component has an update, non-empty `CurrentVersion` | `<name> — update available for <component> (v<cur> → v<new>)` |
| exactly one component has an update, empty `CurrentVersion` | `<name> — update available for <component> (→ v<new>)` |
| two or more components have updates | `<name> — updates available for <n> components` |
| no update, no wait, at least one component pends a reboot | `<name> — changes apply after reboot` |
| no update, every component agrees on a non-empty `CurrentVersion` | `<name> — v<version>` |
| no update, components disagree or any `CurrentVersion` is empty | `<name> — up to date` |

No branch emits a bare `v` with nothing after it, and no branch presents one component's version as the feature's version unless every component agrees on it. The Features page shows each subtitle under the feature's title through `featureLine` (`internal/views/features_page.go`), which drops the leading `<name> — ` (the row already shows the feature) and turns `v<version>` into "Up to date" with the version in the row's version column. The descriptions are `%d features available — update check failed`, `%d features available — all up to date`, `%d features available (1 update)`, `%d features available (%d updates)`, `%d features available (1 waiting for a newer OS image)`, `%d features available (%d waiting for a newer OS image)`, and `%d features available (1 update, %d waiting for a newer OS image)` / `%d features available (%d updates, %d waiting for a newer OS image)` when the check found both; the waiting fragment keeps one wording for one feature and for many, and only its number changes; the leading `%d features available` fragment is reproduced verbatim from `pageview.FeatureGroupDescription`, the pre-check text, including its non-pluralized `features`, so only the status tail differs — which is also what makes a completed check that found nothing (`— all up to date`) visibly distinguishable from the pre-check state. `featurestatus_test.go` covers each branch as a table subtest and additionally asserts that the seven subtitle branches are pairwise distinct for a fixed feature name, that every case whose components need no image reports `WaitingForImage` false, that no subtitle renders a bare `v`, and that the seven descriptions are distinct from each other and from the pre-check string.

The package is pure and holds no state, so it is safe to call from a worker goroutine.

`Features.checkFeatureUpdates` (`internal/views/features.go`) is the only caller of `Summarize`, and — as with `Flatpak.applyUpdates` and `flatpakstatus` — the domain holds no text of its own: every string in the update-check path comes from `featurestatus`. `updex.CheckFeatures` runs on a worker goroutine started by `recheckFeatureUpdates` through `views.Go`, and `checkFeatureUpdates` runs on the UI thread; its first statement drops the result when its generation is stale: `recheckFeatureUpdates` calls `f.featureCheck.Begin()` (an `actionstate.RefreshGate`) and passes the generation along, so a check that a newer one has already replaced — the startup check finishing after an update's re-check, for example — publishes nothing, neither a status nor the description. When the check failed, `log.Printf("Feature update check failed: %v", err)` runs, the rows keep what the last check said, and `Features.groupDescription` becomes `featurestatus.GroupDescriptionCheckFailed(total)`, so the list no longer reads `%d features available`, which looked like a completed check that found nothing; the features source reports the error too. When the check succeeded, the domain builds the names from `f.features`, calls `featurestatus.Summarize(names, checks)` once, and replaces the last summary whole: a name in `summary.Rows` shows that `Status.Subtitle`, and every other name gets its base subtitle back (`Features.subtitle`). That reset is what stops a stale "update available" from surviving an update: the staged, not yet linked image of a feature that now waits for a newer OS image is reported as a wait, and a feature the check did not answer for reads as it did before any check. The description then comes from `featurestatus.GroupDescription(total, summary.Updates, summary.Waiting)`, including when both counts are `0`, so "all up to date" is actually reported rather than the pre-check string being left in place. Both counts are feature counts taken from `summary.Rows`, so a waiting feature is never counted as an update and a pending-reboot feature is counted as neither. The features update source (`featureSource`) lists the features with an update as its items (all updated by one run of the helper) and adds the waiting count to its note. A disabled `features_group` loads nothing (`Features.Start` and `load` return first), and `recheckFeatureUpdates` returns at once when the group is off or nothing is listed.

**The re-check after an update.** The startup check is not the only one: `Features.updated` runs the check again whenever `actionmsg.FeatureUpdate` sets `Recheck`, which is every outcome that ran the update and not the dry-run preview. Without it the rows kept the startup check's subtitles forever, because before an update nothing is staged, so that check read "update available"; the update then stages the image and updex links nothing of it while its `UPDEX_MIN_IMAGE_VERSION` is above the image that boots next (updex ADR-0016), so the row went on saying "update available" while the real state was "waiting for a newer OS image". With the re-check, `Summarize` reports the wait and the row says so; a feature whose link is in place reads `changes apply after reboot`. A failed run re-checks too, because it can change state in part. `featurestatus/wiring_test.go` (`TestFeaturesPageUsesSummarizeAndRechecks`) reads `features.go` and requires the generation guard before every publication in `checkFeatureUpdates`, the `Summarize` call, the base-subtitle fallback and the `if decision.Recheck` re-check, and rejects a per-entry `featurestatus.Feature(` call in the Features domain; `features_test.go` (`TestFeaturesCheckGenerationGate`, `TestFeaturesRecheckRestoresBaseSubtitles`, `TestFeaturesUpdateRunsTheHelperOnceAndRechecks`, `TestFeaturesUpdateDryRunDoesNotRecheck`) proves the behavior.

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
| `ListUpdates(user)` | `flatpak remotes [--user\|--system] --columns=name,options`, then `flatpak remote-ls --updates --app --columns=name,application,version,branch,origin [--user\|--system] <remote>` per enabled remote, and `flatpak list [--user\|--system] --app --columns=origin` only when a remote failed | 30s per query | Separate calls for user/system; `--app` excludes runtimes; see [Update queries run per remote](#update-queries-run-per-remote) |
| `Install(appID, user)` | `flatpak install -y [--user\|--system] <appID>` | 30m | State-changing |
| `Uninstall(appID, user)` | `flatpak uninstall -y [--user\|--system] <appID>` | 30m | State-changing |
| `Update(appID, user)` | `flatpak update -y [--user\|--system] [<appID>]` | 30m | State-changing; empty appID updates all |
| `AddRemote(name, url, user)` | `flatpak remote-add [--user\|--system] --if-not-exists <name> <url>` | 30m | State-changing; the first run's Progress step adds `FlathubRemoteName`/`FlathubRepoURL` to the user installation before it installs applications |
| `UninstallUnused()` | `flatpak uninstall --unused -y` (no scope flag) | 30m | Maintenance cleanup of every installation; see below |
| `Info(appID, user)` | `flatpak info --show-metadata [--user\|--system] <appID>` | 30s | Key-value parsed |
| `GetRemotes(user)` | `flatpak remotes --columns=name [--user\|--system]` | 30s | Lists configured remotes |

### State-changing commands

`install`, `uninstall`, `remove`, `update`, `remote-add` — exactly five keys. As in homebrew, the map selects both the dry-run skip (these are skipped entirely and return a mock message) and the timeout class: `commandTimeout(args []string)` returns `mutationTimeout` (30 minutes) when `args[0]` is one of the five keys and `readTimeout` (30 seconds) otherwise, including for empty args, and `runFlatpakCommand` passes it to `context.WithTimeout`. Flatpak's read budget matches homebrew's 30 seconds, so both wrappers agree. `flatpak_test.go`'s `TestCommandTimeout` iterates the real map and asserts `len(stateChangingCommands) == 5`.

### Error handling

`runFlatpakCommand` is a thin wrapper that calls `runFlatpakCommandCtx(context.Background(), args...)`. `runFlatpakCommandCtx(ctx, args...)` is the single dry-run gate (it skips state-changing commands before any `exec.Cmd` exists, logs `[DRY-RUN] Would execute: flatpak ...` and reports the command with `cmdlog.DryRun`), layers `commandTimeout(args)` onto the caller's `ctx` with `context.WithTimeout` (so the nearer deadline wins and cancelling `ctx` stops the command), and delegates to the unexported `runFlatpakCommandAt(ctx context.Context, exe string, args ...string) (string, error)`, always passing `"flatpak"`. The executable path and context are parameters purely so `runner_test.go` can drive a `#!/bin/sh` script from `t.TempDir()` and control the deadline — the same seam `stageexec.Run` gives OS staging and `runBrewCommandAt` gives `internal/homebrew`. No exported function takes a `context.Context`: callers get deadlines, not cancellation. flatpak runs unprivileged; no `pkexec` is involved on this path.

`runFlatpakCommandAt` starts the command in its own process group (`cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`) and sets `cmd.Cancel` to `syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)`, so flatpak's download helpers (download workers, ostree pulls) are killed with the command instead of being orphaned when only the direct child is signalled. `cmd.WaitDelay` (5s) bounds the wait, because those helpers inherit the stdout/stderr pipes and a straggler would otherwise hold `Wait` open indefinitely. `cmd.Run` still reaps the child. Like `runBrewCommandAt`, it announces the command with `cmdlog.Start`, tees stdout and stderr through the trace while buffering both, and ends it with `trace.End(err)`, so the console shows every flatpak command and its output as it runs.

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

### Unused-runtime cleanup covers every installation

`UninstallUnused` runs `flatpak uninstall --unused -y` with no scope flag, on
purpose. flatpak parses `uninstall` with `FLATPAK_BUILTIN_FLAG_ALL_DIRS`
(`app/flatpak-builtins-uninstall.c`): with no `--user`, `--system` or
`--installation` flag, `flatpak-main.c` selects the user installation, the
default system installation and every extra system installation from
`/etc/flatpak/installations.d`, and the `--unused` branch visits each of them.
This holds in flatpak 1.10, 1.14 and 1.16. Do not add a scope flag: running
`--user` then `--system` would skip the extra installations
(`.memory/corrections.jsonl`, 2026-11-05). `flatpak_test.go`
(`TestUninstallUnusedCoversEveryInstallation` and the "remove unused" row of
`TestCommandWrappersUseExpectedArguments`) asserts exactly one invocation with
no scope flag.

### Update queries exclude runtimes

`ListUpdates` builds each remote's argument list with the unexported pure
helper `updateListArgs(user bool, remote string) []string`, the only place the
`remote-ls` command is spelled. It passes `--app` — matching the precedent set by `listApplications` —
so runtimes and extensions are deliberately excluded from the results. The
Flatpak update source's items, and with them the Updates count, therefore only
ever describe applications, which is what the user can act on from the Apps
and Updates pages.

### Update queries run per remote

A remote-less `flatpak remote-ls --updates` fails the whole installation when
any one remote is unreachable, so a remote left behind by an uninstalled
application failed the Updates check and hid every other remote's updates,
although `flatpak update` itself still succeeded. `ListUpdates(user)` therefore
delegates to the unexported `listUpdates(ctx, user)`, which:

1. lists the installation's remotes with `updateRemoteListArgs`
   (`remotes <scope> --columns=name,options`) and keeps, in listed order, every
   remote whose options do not include `disabled` (`parseUpdateRemotes`) — the
   set flatpak's own remote-less query visits;
2. queries each of those remotes with `updateListArgs(user, remote)`, each
   under its own 30-second read budget, so one slow remote cannot starve the
   others;
3. only when at least one remote query failed, lists the origin of every
   installed **application** in the installation with `originListArgs`
   (`list <scope> --app --columns=origin`, parsed by `parseOrigins`). `--app`
   matches the inventory's own filter: a remote that still serves only
   runtimes — the usual leftover of an uninstall without `--unused` — can hide
   no update this inventory would list.

Outcomes, per installation:

| Situation | Updates returned | Error returned |
|-----------|------------------|----------------|
| Remote listing fails | `nil` | the listing's `*Error` / `*NotFoundError` |
| No enabled remotes | none | `nil` (only the listing runs) |
| Every remote query succeeds | all remotes' updates | `nil` (no origin listing) |
| A remote fails and no installed application comes from it | the healthy remotes' updates | `nil`; the failure is logged as `flatpak: ignoring <installation> remote "<name>", which no installed application uses: ...` |
| A remote fails and an installed application comes from it | the healthy remotes' updates | a `*flatpak.RemoteError{Remote, Installation, Err}` per such remote, joined with `errors.Join`; its message names the remote and installation and it unwraps to the query's `*Error` |
| A remote fails and the origin listing fails too | the healthy remotes' updates | the origin listing's error joined with a `*RemoteError` for every failed remote (none can be shown unused) |
| The caller's context is cancelled, or the executable is missing (`abortsUpdateQuery`) | `nil` | the cancellation / `*NotFoundError`; never judged as one remote's failure |

A per-query timeout (`context.DeadlineExceeded`) is a remote's own failure,
not an abort. The exported `ListUpdates` passes `context.Background()`; the
context parameter exists so cancellation is part of the contract and is
tested. `Flatpak.loadUpdates` (`internal/views/flatpak.go`) keeps the returned
updates even when the error is non-nil, so the healthy remotes' items are shown
while the source's status names the failed installation, or both installations when both queries returned an
error and at least one of them also returned updates (`flatpakstatus`). A
load is authoritative when at least one installation has a usable result: no
error, or one or more returned updates together with an error. It keeps the
last known rows and count only when both installations failed and neither
returned an update. `flatpak_test.go` covers each row of the table plus the argument builders and parsers directly.

## fwupd (`internal/fwupd/fwupd.go`)

Wraps the unprivileged `fwupdmgr` client of the Linux Vendor Firmware Service.
Every call is a D-Bus request to the `fwupd` daemon; the daemon does the work
and authorizes it with its own PolicyKit actions (`org.freedesktop.fwupd.update-internal`,
`org.freedesktop.fwupd.update-hotplug`, and others). ChairLift ships no
PolicyKit policy file and no helper binary for firmware, and nothing on this
path uses `pkexec` — decision record
[ADR-0012](../adr/0012-firmware-updates-use-fwupds-own-polkit.md).

### Key types

- **`Update`** — `DeviceID`, `DeviceName`, `Vendor`, `CurrentVersion`,
  `NewVersion`, `Summary`, plus `NeedsReboot` and `NeedsShutdown`. The last two
  come from the device's `needs-reboot` and `needs-shutdown` flags; they drive
  the confirmation dialog and the completion toast, because ChairLift never
  restarts or shuts down the computer itself.
- **`Update.Installable` and `Update.Reason`** — the blocked-device pair.
  `Installable` is true only when the flags contain `updatable`, `Problems` is
  empty, and `UpdateError` is empty, which is exactly when `fwupdmgr update`
  would flash the device. `Reason` explains why it is false (see "Blocked
  devices" below) and is empty for an installable device.
- **`Error`** — message plus an underlying cause, so `errors.Is` classifies a
  deadline or a cancellation while the message stays readable.
- **`DeviceState`** — what the device itself reports: `Version` (the installed
  version now), `UpdateState` (`pending`, `success`, `failed`,
  `failed-transient`, `needs-reboot`, or absent) and `UpdateError`. It comes
  from `fwupdmgr get-devices --json`, which lists every device.
- **`InstallResult`** — the classification of the device's own report:
  `InstallUnverified` (the state could not be read or matches no rule),
  `InstallFlashed` (the device reports the new version), `InstallStaged` (the
  device reports `needs-reboot` or `pending`, so the flash applies at the next
  restart while the version is still the old one) and `InstallNotApplied` (the
  device still reports the old version). `InstallUnverified` is the zero value,
  so a report that nothing filled in fails closed. `InstallReport` carries the
  result, its reason, the `[]Update` list used to rebuild the rows, and that
  list's error.

### Operations

| Function | CLI command | Timeout | Notes |
|----------|------------|---------|-------|
| `ListUpdates()` | `fwupdmgr get-updates --json` | 60s | Read-only; parses `{"Devices":[…]}` |
| `Install(deviceID)` | `fwupdmgr update --no-reboot-check <deviceID>` | 30m | State-changing, dry-run aware; validates the id first |
| `GetDeviceState(deviceID)` | `fwupdmgr get-devices --json` | 60s | Read-only; validates the id first; exit 2 means the device is not listed |
| `InstallAndVerify(update)` | that install, then `get-devices --json` and `get-updates --json` | 30m + 2 × 60s | Install error returned as the error; dry-run returns a zero report and makes no query |
| `VerifyInstall(before, state, found, stateErr)` | none | — | Pure: classifies the device's own state |
| `ParseDeviceState(data, deviceID)` | none | — | Pure: JSON plus an id in, `DeviceState` and found out |
| `IsInstalled()` / `IsInstalledCached()` | `exec.LookPath("fwupdmgr")` | — | Cached with `sync.Once`; the firmware source is unavailable until it answers true |
| `ParseUpdates(data)` | none | — | Pure: JSON in, `[]Update` out |
| `SetDryRun(bool)` / `IsDryRun()` | none | — | Module-level flag set once by `app.SetDryRun` (called from `app.Run` with `--dry-run`) |

Unlike Homebrew and Flatpak, fwupd has no `stateChangingCommands` map and no
`commandTimeout(args)`: there are two read commands (`get-updates --json` and
`get-devices --json`, 60s each) and one state-changing command (`update`, 30m),
so each caller passes its own timeout constant. `InstallAndVerify` runs the
install and then both reads in sequence, each read with its own 60s budget.

### Parsing rules

`ParseUpdates` reads the devices in fwupd's order and keeps a device only when
it has at least one release and a well-formed `DeviceId`. Releases are newest
first, so `Releases[0]` is the offered version; its `Version` and `Summary`
become `NewVersion` and `Summary`, while the device's `Version` becomes
`CurrentVersion`. Unknown JSON keys are ignored: `fwupdmgr` emits far more
fields than the page shows, and a newer client must not break the parse. A
device with no `Releases` key or an empty one has nothing to install and is
skipped.

### Blocked devices

`fwupdmgr get-updates --json` lists devices that it will not flash now. Such a
device has no `updatable` flag (for example only `updatable-hidden`: the laptop
lid is closed, or the host is on battery), or a non-empty `Problems` array, or
a non-empty `UpdateError` string. `fwupdmgr update <id>` skips such a device and
still exits 0, so a view that reacts to the exit status alone would report a
flash that never happened.

`ParseUpdates` therefore keeps these devices in the list, sets `Installable`
false, and fills `Reason` from the most specific source available:
`UpdateError` when it is set, otherwise `Needs attention: ` plus the problems
joined with `, `, otherwise `This device cannot be updated now`. Only a device
fails the id or release check and is dropped; a blocked device is never
dropped, because removing it would hide an update without telling the user why
it cannot be applied.

### Verification after an install

`fwupdmgr update` can exit 0 without flashing a device: the lid closes, the host
leaves battery power, or an update error appears while the confirmation dialog
is open. Neither a missing update offer nor a different offered version proves a
flash either — a device can leave the update list without being written, and a
new offer can appear for a device that was never touched. Only the device itself
is evidence, so `InstallAndVerify` reads `fwupdmgr get-devices --json` after the
install and passes the result to `VerifyInstall`. The `get-updates` list is read
afterwards for one purpose only: rebuilding the rows.

`VerifyInstall` applies these rules in this order, first match wins:

| Rule | Condition | Result | Reason |
|------|-----------|--------|--------|
| a | the state query failed | `InstallUnverified` | the state query's error text |
| b | the device is not listed | `InstallUnverified` | `fwupd does not show the device now; it can be restarting` |
| c | `UpdateState` is `needs-reboot` or `pending` | `InstallStaged` | — |
| d | `UpdateState` is `failed` or `failed-transient` | `InstallNotApplied` | `UpdateError`, or `the update failed` |
| e | `Version` equals the offered new version (and is not empty) | `InstallFlashed` | — |
| f | `Version` equals the device's old version | `InstallNotApplied` | `UpdateError`, else the device's own blocked reason, else `fwupd did not install the update` |
| g | anything else | `InstallUnverified` | `the device now reports version <Version>` |

`InstallUnverified` is the zero value of `InstallResult`, so an unknown outcome
can never read as a success. A failed state wins over a matching version
(rule d before rule e), so the check fails closed: a device that reports the
target version together with `failed` is not called installed. A staged result is a real install that still needs
a restart: a UEFI capsule reports `needs-reboot` while its version stays the old
one.

`InstallAndVerify` returns the install error as its error, so a failed command
and a command that changed nothing stay distinguishable. Under dry-run it
returns a zero `InstallReport` and makes no query at all.

### Exit-code rule

`ListUpdates` treats exit status **2** as an empty list with a nil error:
older fwupdmgr versions use it for "no updatable devices" (stderr "No updatable
devices"), and `{"Devices":[]}` with exit 0 is the newer form. Any other
non-zero exit is an `*Error` whose message carries the command's stderr, so the
firmware source can say that the check failed instead of claiming the firmware
is up to date.

### Device-id validation

`Install` rejects any id that is not 40 lowercase hexadecimal characters
(`^[0-9a-f]{40}$`, the format fwupd itself accepts) before it builds a command;
the returned `*Error` names the id constraint and no process runs. PolicyKit
does not inspect the arguments of the daemon's actions, so a value such as
`--force` must never reach `fwupdmgr`'s argv. The validator is the argument
injection defense for this wrapper.

### View rules

`Firmware` (`internal/views/firmware.go`) holds the firmware update source
(`updates_page`/`firmware_updates_group`, on by default since
[ADR-0013](../adr/0013-firmware-updates-are-on-by-default.md)). Its load runs
`fwupd.IsInstalledCached` and then `fwupd.ListUpdates` on one worker
goroutine; the source is unavailable until `fwupdmgr` is found, so a host
without fwupd never shows it, and a load already in flight blocks a second
one. `Firmware.publish` is the single writer of the device list, used by a
load (`applyLoad`) and by a verified install (`finishInstall`): it replaces
the whole list. Every load takes a generation from `f.refresh`
(`actionstate.RefreshGate`), so a stale query cannot replace a newer list. A
failed load publishes nothing: the old items stay, and the source's error is
`pageview.FirmwareUpdatesSubtitle(0, err, len(f.updates)).Subtitle`, so it
never claims the firmware is up to date.

Every listed device is an item of the source, so it counts in the Updates
figure (`Store.UpdateCount` counts held items too). An installable device is a
`Confirm` item (its button reads `Install…`): it installs one at a time, after
a confirmation, and never through "Update everything" (`firmwareSource.Jobs`
with no keys returns nothing). A blocked device is a held item: it has no
install control, and its detail is `pageview.FirmwareBlockedUpdate(...)` — the
normal row text plus ` · ` plus the `Reason` — so the user reads why the
device waits. The source's note is
`pageview.FirmwareUpdatesSubtitle(len(updates), nil, 0)`, which counts every
listed update.

One `Firmware.installGate` (`actionstate.Gate`) allows a single firmware
install at a time, held from the click until the install ends: a click that
finds the gate busy shows the toast `Another firmware update is installing`
and does nothing else. `Firmware.ask` opens the confirmation through
`Env.Dialogs`: a destructive `frostui.Confirm` with
`pageview.FirmwareInstallConfirmation`'s heading and body, the device, vendor
and versions, and a note that fwupd may ask for administrator authentication
through its own PolicyKit action. Cancel resets the gate; confirm queues
`Firmware.installJob` on the serial job queue. The job calls
`fwupd.InstallAndVerify` on its goroutine and builds `actionmsg.FirmwareInstall`
there; `finishInstall` applies the decision on the UI thread and resets the
gate last, in every case, also when the queue dropped the job before it ran.

The install flow keeps no row identity and never patches the list: there is
no item removal and no count decrement. `actionmsg.FirmwareInstall` decides
`PublishList`, `RestoreControl`, `IsError` and `CompletedLabel`. When
`PublishList` is set — for every live result whose `report.ListErr` is nil —
`finishInstall` takes a fresh `f.refresh` generation (which makes any load
still in flight stale) and publishes `report.Updates`. That list is the
refresh, and it is why a device that still has an update appears again. When
the list could not be read, the items stay as they are, the source's error is
`pageview.FirmwareUpdatesSubtitle(0, report.ListErr, len(f.updates))`, and a
finished install records its `CompletedLabel` (`Installed` or `Restart
required`) on the device, which holds the item and keeps the install from
being offered again. A live flashed or staged install records an activity
entry. `actionstate.PackageUpgrade` is not used on this path.

### Dry-run behavior

`Install` validates the id first — an invalid id is an error even under
dry-run, because the id is a programming contract, not a mutation — then logs
`[DRY-RUN] Would execute: fwupdmgr update --no-reboot-check <id>` and returns
nil without constructing a command. fwupd reports no `cmdlog.DryRun` event of
its own: the console shows that log line through the log tee, which classifies
a `[DRY-RUN]` line as a dry-run line. `InstallAndVerify` makes no query under
dry-run, so `Firmware.finishInstall` shows the `actionmsg.FirmwareInstall`
preview and publishes nothing. `ListUpdates` has no dry-run branch: it only
queries the daemon.

### Test seam

`runCommandAt(ctx context.Context, exe string, args ...string) (string, int, error)`
is the unexported runner. It returns stdout, the process exit status, and a
classified error, and the executable is a package variable whose default is
`"fwupdmgr"`, so `runner_test.go` and `fwupd_test.go` drive a `#!/bin/sh` fake
script from `t.TempDir()` and assert the exact argv a real install would use.
Like `runFlatpakCommandAt`, it starts the command in its own process group and
sets `cmd.Cancel` to kill that group, so a helper process dies with fwupdmgr;
`cmd.WaitDelay` bounds the wait. Deadlines and cancellations are classified
first and never surface as `signal: killed`. It reports every command through
`cmdlog` (`Start`, the stdout and stderr tee, `End`), so the console shows
each `fwupdmgr` call and its output.

## Shared OS stage executor (`internal/stageexec`)

`internal/stageexec` is a pure-Go, widget-free leaf package used only by the
fixed bootc and sysupdate staging adapters. `ProgressEvent`, `EventMessage`, and
`EventComplete` are defined once there and exposed from both providers through
type/constant aliases, so existing callers keep their provider-qualified API.

`Run(ctx, progressCh, name, args...)` owns the complete live process contract:
successful exit emits exactly one `EventComplete`; trimmed non-empty stdout and
stderr lines share one ordered `EventMessage` stream; non-zero exit includes the
exit code and last output line and emits no completion; deadline and
cancellation return distinct errors matching their context sentinels without
leaking `signal: killed`; missing bare names and explicit paths return
`*stageexec.NotFoundError`; and every path closes the channel. `Run` reports
the command through `cmdlog.Start`, each merged output line through the
trace's `Line`, and its end through `End`, so a stage script's output reaches
the console as it runs. `DryRun` reports `cmdlog.DryRun(scriptPath)`, emits
the synthetic preview plus completion, closes the channel, and constructs no
`exec.Cmd`. `stageexec_test.go` covers each outcome once against local scripts;
the provider tests cover only fixed-path/dry-run adapters and native A/B
detection.

Cancellation deliberately kills and reaps only the direct child. Both callers
run through `pkexec`, whose privileged child cannot be signaled by ChairLift as
an unprivileged process group. The unprivileged Homebrew and Flatpak executors
retain their separate process-group-kill behavior.

## bootc (`internal/bootc/`)

Wraps `bootc` for OSTree/composefs system updates, split across two files:
`bootc.go` (unprivileged status reads) and `stage.go` (fixed-path privileged
staging adapter). There is no separate CLI helper binary or Go client library;
status parsing uses `os/exec`, while stage execution delegates to
`internal/stageexec`.

### `GetStatus` (unprivileged)

`GetStatus(ctx)` runs `bootc status --format json` with **no** `pkexec` — this is a plain read, safe to call from any goroutine (`internal/bootc/bootc.go`). Output is unmarshaled into `Status{Spec, Status: {Booted, Staged, Rollback}}`, where each of `Booted`/`Staged`/`Rollback` is a `*Deployment` (nil-safe accessors: `ImageRef()`, `Version()`, `Timestamp()`, `Digest()`).

`GetStatus` is a one-line wrapper: `return getStatusFrom(ctx, bootcCommand)`. The unexported `getStatusFrom(ctx context.Context, name string) (*Status, error)` runs `<name> status --format json`, classifies the error, and parses the output. The executable name is a parameter purely so `bootc_test.go` can drive a `#!/bin/sh` script from `t.TempDir()` on a host with no `bootc` installed. The seam is unexported and its only production call site passes the fixed `bootcCommand` constant, so no caller-supplied or user-derived string can reach it. `getStatusFrom` reports the command through `cmdlog` (`Start` before it runs, `End` with its error); its JSON output is not teed line by line.

Failures classify into exactly four distinct outcomes, checked in this order — `errors.Is` against `context.DeadlineExceeded`/`context.Canceled`, never `==` on `ctx.Err()`, so a wrapped cause still classifies:

| Condition | Result |
|-----------|--------|
| The context deadline expired | `*Error`, message `bootc status timed out`, unwrapping to `context.DeadlineExceeded` |
| The context was cancelled by its owner | `*Error`, message `bootc status was canceled`, unwrapping to `context.Canceled` |
| The command exited non-zero | `*Error`, message `bootc status failed (exit N): <stderr>`, unwrapping to the `*exec.ExitError` — matching neither context sentinel |
| The executable is missing (`exec.ErrNotFound` for a bare name, `fs.ErrNotExist` for an explicit path) | `*NotFoundError` (`bootc not found`) |

The deadline and cancellation messages differ, and neither surfaces as `signal: killed`: `exec.CommandContext` kills the child when the context ends, so the classified error replaces the raw wait error. `bootc.Error` carries an `Err error` field and an `Unwrap() error` method — matching `internal/homebrew` and `internal/flatpak` — so callers distinguish all four outcomes with `errors.Is`/`errors.As` while `Error` keeps its human-readable `Message`. `bootc_test.go` covers all four against fake scripts (plus the success parse), so the whole classification is exercised without a real `bootc` on the host.

### Boot gate semantics

`bootc status` exits 0 with a null `booted` field on hosts that aren't running a bootc deployment at all — so the gate cannot be the exit code. `Status.Booted()` returns `s.Status.Booted != nil`. `IsBootcBooted(ctx)` calls `GetStatus` and returns that boolean (treating any error as "not booted"). `IsBootcBootedCached()` wraps it in a `sync.Once` with a 5s timeout, computing the result once and caching it for the lifetime of the process — this lets the system domain's status reads (`System.readDeployments`, on a worker goroutine) call it repeatedly without triggering redundant `bootc` invocations. **Do not use `/run/ostree-booted`** as a substitute gate: it is absent on snow's composefs-based deployments, so checking for it would hide bootc UI on every snow host.

### `StageUpdate` (privileged, streaming)

`StageUpdate(ctx, progressCh)` (`internal/bootc/stage.go`) retains the fixed
`pkexec /usr/libexec/bootc-update-stage` command and adapts
`stageexec.Run`/`DryRun` errors back to `bootc.Error` and `bootc.NotFoundError`.
Its progress types are aliases of the shared contract.

Failures classify into exactly four distinct outcomes, checked in this order — `errors.Is` against `context.DeadlineExceeded`/`context.Canceled`, never `==` on `ctx.Err()`:

| Condition | Result |
|-----------|--------|
| The context deadline expired | `*Error`, message `Update staging timed out`, unwrapping to `context.DeadlineExceeded` |
| The context was cancelled by its owner | `*Error`, message `Update staging was canceled`, unwrapping to `context.Canceled` |
| The script exited non-zero | `*Error`, message `update staging failed (exit N): <last output line>`, unwrapping to the `*exec.ExitError` — matching neither context sentinel |
| `pkexec` itself is missing | `*NotFoundError` (`pkexec not found`) |

The deadline and cancellation messages differ, and neither surfaces as
`signal: killed`. Exhaustive process tests live in
`internal/stageexec/stageexec_test.go`; `bootc/stage_test.go` checks the
fixed-path dry-run adapter without a real `pkexec` or `bootc`.

The direct-child cancellation rationale is owned by `internal/stageexec` above.
`getStatusFrom` is separate: `bootc status` is unprivileged and short-lived, so
`exec.CommandContext`'s default direct-child kill suffices.

**Why a stage script instead of `bootc upgrade`:** upstream `bootc upgrade`'s registry-transport pull fails on snow's composefs images. The stage script works around this by using `podman pull` (whose pull path works) to fetch the image into containers-storage, then running `bootc switch --transport containers-storage` to stage the already-pulled image — `podman` does the pull, `bootc` does the switch. This keeps the actual workaround logic in one place (the snow-shipped script, source of truth in the snosi project) instead of duplicating pull/switch orchestration inside ChairLift. The script is idempotent: it exits 0 without staging anything when the deployment is already current, so `StageUpdate` doubles as both "check for update" and "apply update".

### Event types

- `EventMessage` — one line of stage-script output
- `EventComplete` — sent once, after successful completion

This is intentionally flatter than a step/percent progress model, because the
stage script emits unstructured log lines, not a structured progress protocol.
Failures are returned by `StageUpdate` and handled once by the stage job (`System.finishStage`) after the
channel closes; there is no error event duplicating that path.

### Dry-run behavior

Unlike bootc's own dry-run flag (not used here), ChairLift's dry-run mode is handled entirely inside `StageUpdate`: if `dryRun` is set, it never invokes `pkexec` at all — it logs the command that would run, sends a synthetic `EventMessage` + `EventComplete`, closes the channel, and returns `nil`.

That part is tested in `internal/bootc/stage_test.go`. The view keeps its toast honest: `System.finishStage` (`internal/views/system.go`) re-reads the deployments after every stage job — a plain read, in both modes — and shows `actionmsg.BootcStage(dryRun, staged)`. Under dry-run that is a single, unambiguous preview string regardless of `staged` (which reflects real system state, not anything this run did); otherwise it is "System update staged. Restart to apply." or "System is up to date". The system image source's note, the Overview's deployment chain and the status bar are deliberately *not* dry-run-aware: they show the live state in both modes, since they are a persistent status display, not a per-run completion claim. A dry run also records no activity entry and does not mark the image as current.

### Operations

| Function | Command | Privilege | Timeout | Notes |
|----------|---------|-----------|---------|-------|
| `GetStatus(ctx)` | `bootc status --format json` | none | 30min (`DefaultContext`); the system domain uses `bootc.DefaultTimeout` too | JSON parsed into `Status` |
| `IsBootcBooted(ctx)` / `IsBootcBootedCached()` | (calls `GetStatus`) | none | 5s (cached variant) | Boot gate; cached variant memoizes via `sync.Once` |
| `StageUpdate(ctx, progressCh)` | `pkexec /usr/libexec/bootc-update-stage` | pkexec (`org.frostyard.ChairLift.bootc.stage`) | 30min (`DefaultContext`) | Streaming; idempotent; dry-run aware |
| `StageScriptAvailable()` | `os.Stat(StageScriptPath)` | none | — | Without the script the system image source is unavailable (`System.sourceAvailable`) |

### Streaming pattern

The stage runs as one `frostui.Job` on the serial job queue
(`System.stageJob`), so it never overlaps another change and its output shows
in the console:

```go
job.Run = func(ctx context.Context, p *frostui.Progress) error {
    events := make(chan bootc.ProgressEvent)
    errc := make(chan error, 1)
    go func() { errc <- stage(ctx, events) }() // bootc.StageUpdate or sysupdate.StageUpdate
    for ev := range events { // the stager closes the channel
        switch ev.Type {
        case bootc.EventMessage:
            p.Set(-1, ev.Message) // the job's status in the rail and the status bar
        case bootc.EventComplete:
            p.Set(1, "Complete")
        }
    }
    err := <-errc
    // Re-read the deployments in both modes, then hand them to Done.
    return err
}
job.Done = func(err error) { /* System.finishStage, on the UI thread */ }
```

`stageexec.Run` reports the same lines to the console through `cmdlog`, so
the job's progress shows the newest line while the console keeps all of
them.

### Progress UI (`internal/views/system.go`)

`System` holds the system image update source (`imageSource`, "System
image"), which applies when the last status read found a host of an enabled
updates group's kind with its stage script (`System.sourceAvailable`). The
source's item, its row's `Stage` button, "Update everything" and the
Overview's "Stage the system update" command queue `System.stageJob`; the
image source's `Extra` adds a "Check and stage" button while it offers no
item. A stage request while its job waits or runs does nothing
(`Jobs.Pending`). The job runs the fixed stager of the host's kind
(`bootc.StageUpdate`, or `sysupdate.StageUpdate` on a native A/B host, never
another path), sets each `EventMessage` as the job's progress status, and then
re-reads the deployments (`readDeployments`) with a fresh context, in both
modes. `System.finishStage` applies the result on the UI thread: it takes a
new generation from the deployments' `RefreshGate` (so an older status read
still in flight cannot replace it), publishes the re-read deployments to the
Overview's deployment chain, the status bar and the source's note, shows the
toast (`actionmsg.BootcStage`/`SysupdateStage`; an error toast on failure,
"System update canceled" when canceled), records an activity entry when a
live run staged a new image, and marks the image as current for this session
when a live run staged nothing (its last output line becomes the source's
note). A staged image offers no item, so it does not count as an update. A
job the queue dropped before it ran changes nothing. The Overview and
Maintenance draw the same deployments read-only (booted, staged and rollback
images, versions and digests); staging happens only through this job.

## snosi sysupdate (`internal/sysupdate/`)

The native A/B (systemd-sysupdate) OS-update wrapper, mirroring
`internal/bootc`'s split: `sysupdate.go` (detection, dry-run, errors,
context), `status.go` (unprivileged state-file reads and the presentation
decision), `rollback.go` (unprivileged previous-version discovery), and
`stage.go` (privileged staging via pkexec). Nothing here imports a UI
package, and every file is gate-tested.

### Host detection

`IsNativeAB()` is an `os.Stat` of `MarkerPath`
(`/usr/lib/snosi/native-ab`); `IsNativeABCached()` memoizes it via
`sync.Once`. The marker is snosi's own published contract — every snosi unit
gates on `ConditionPathExists=/usr/lib/snosi/native-ab` — and is the only
reliable signal: the `bootc` binary is absent on native A/B images (so a
bootc probe would hard-fail rather than return false cleanly), and
`os-release`'s `IMAGE_ID` is identical across the bootc and native A/B
variants of the same product. The system image source additionally requires
`StageScriptAvailable()` (`os.Stat` of
`StageScriptPath = /usr/libexec/snosi-sysupdate-stage`).

### Status reads (unprivileged)

The snosi stager writes two world-readable state files under `/run/snosi`
(tmpfs — both are cleared by the reboot that applies an update):

- `UpdateCheckPath` (`/run/snosi/update-check`), written on every stager run:
  `outcome=(current|staged|failed)`, `checked_at=<ISO-8601>`,
  `image=<channel>`, `running_version=`, `remote_version=`. The bootc-only
  `held-rollback` outcome never occurs on native; `ParseUpdateCheck` maps it
  (and any unknown value) to `OutcomeUnknown` rather than erroring.
- `StagedSemaphorePath` (`/run/snosi/update-staged`), present exactly while a
  downloaded update awaits its applying reboot: `image=`, `staged_at=`, and
  exactly one of `version=<14-digit>` (native) or `digest=sha256:...`
  (bootc transport — the file language is shared, so `ParseStagedUpdate`
  reads both and `DisplayVersion()` prefers the valid version, then a
  shortened digest).

`GetStatus()` reads both via unexported per-path seams
(`readUpdateCheckFrom`/`readStagedUpdateFrom`); an absent file is a normal
state reported as nil, and a read error is logged and treated as absent, so
the system image source falls back to the idle note rather than an error.
`Status.Presentation()` reduces the pair to `(outcome, version, checkedAt)`
scalars for `pageview.SysupdateUpdateSubtitle`, one row per outcome:
semaphore present → staged with the semaphore's version (the semaphore wins
over every check outcome — it legitimately outlives later "nothing newer"
check runs); no semaphore but `outcome=staged` → staged with
`remote_version`; `outcome=current` → current with the check time;
`outcome=failed` → failed; absent/unknown → idle. `Status.IsStaged()` is the
staged rule: true iff the semaphore exists or the check recorded
`outcome=staged`; the system domain then shows a staged deployment, and the
image source offers no item (a staged image is not an update to run). Versions follow the frozen native grammar `ValidVersion`
(`^[0-9]{14}$`, UTC `YYYYMMDDHHMMSS`) whose fixed width makes lexicographic
comparison numeric.

### Rollback discovery (unprivileged, read-only)

`RollbackVersion(ctx)` runs `lsblk -J -o PATH,PARTLABEL` and reads
`IMAGE_ID`/`IMAGE_VERSION` from `/usr/lib/os-release`. `otherSlotFromLsblk`
walks the JSON recursively (lsblk nests partitions under each disk's
`children`) selecting labels `<IMAGE_ID>_<version>_r` and excluding the
running slot **by its PARTLABEL** — never by comparing mount sources,
because a dm-verity root mounts from `/dev/mapper/root`, which never equals
a partition path. A fresh install's `_empty` label fails the prefix filter
naturally. `RollbackCandidate(other, running)` then accepts the other slot's
version only when it is **older** than the running one: after a stage the
inactive slot holds the newer pending version, which is not a rollback
target. The image source's `Extra` shows the result as a read-only
"Previous version" line on the Updates page
(`pageview.SysupdateRollbackSubtitle`), and the Overview's deployment chain
shows it as the rollback slot; actual rollback is a
systemd-boot menu selection at restart, deliberately outside ChairLift's
privilege boundary. `/boot` is never read (the ESP mounts `umask=0077`,
root-only).

### StageUpdate (privileged)

`StageUpdate(ctx, progressCh)` runs `pkexec /usr/libexec/snosi-sysupdate-stage`
(polkit action `org.frostyard.ChairLift.sysupdate.stage`,
`data/org.frostyard.ChairLift.sysupdate.policy`, exec.path annotation, no
argv). The script — shipped by the OS image, not ChairLift — checks the
sysupdate target for a newer version, downloads it into the inactive
root/verity slots via `systemd-sysupdate update`, verifies the result
against the signed index, and writes both state files; it never reboots and
is idempotent (exits 0 without staging when already current or when the
candidate already sits in the inactive slot). Output streams through `internal/stageexec`, exactly the same event, process,
error, completion, and closure contract as bootc. The stager takes no flock; ChairLift's
serial job queue and its one stage job at a time prevent UI-origin overlap, and the OS's
`snosi-sysupdate-stage.timer` is inert by default, but a concurrent
timer-driven run is a residual (currently theoretical) race that belongs to
the OS-owned helper to close.

### Event types

`ProgressEvent{Type, Message}` with `EventMessage` (one output line) and
`EventComplete` (success). Same enumeration as bootc's; failures are
returned as the function error, never as an event.

### Error classification (`stageexec.Run` / `StageUpdate`)

| Outcome | Returned type | `errors.Is` sentinel |
|---------|---------------|----------------------|
| Deadline | `*Error` "Update staging timed out" | `context.DeadlineExceeded` |
| Canceled | `*Error` "Update staging was canceled" | `context.Canceled` |
| Missing executable | `*NotFoundError` | — (`exec.ErrNotFound` or `fs.ErrNotExist` at start) |
| Non-zero exit | `*Error` carrying exit code + last output line | neither context sentinel |

### Dry-run behavior

`StageUpdate` short-circuits before constructing any `exec.Cmd`: logs the
would-be command, emits the synthetic `EventMessage`+`EventComplete` pair,
closes the channel, returns nil — pkexec is never invoked and no auth
dialog appears. Status and rollback reads are **not** dry-run-gated (they
are side-effect-free reads of real state), matching the bootc toast/subtitle
split: only the toast (`actionmsg.SysupdateStage`) is dry-run-aware.
`RollbackVersion`'s `lsblk` read is not reported through `cmdlog`, so it does
not show in the console.

### Operations

| Function | Command | Privilege | Timeout | Notes |
|----------|---------|-----------|---------|-------|
| `GetStatus()` | reads `/run/snosi/update-check` + `/run/snosi/update-staged` | none | — (local file reads) | Absent files are normal states, not errors |
| `IsNativeAB()` / `IsNativeABCached()` | `os.Stat(MarkerPath)` | none | — | Host-type gate; cached variant memoizes via `sync.Once` |
| `RollbackVersion(ctx)` | `lsblk -J -o PATH,PARTLABEL` + `/usr/lib/os-release` | none | caller's ctx | Older-only candidate rule |
| `StageUpdate(ctx, progressCh)` | `pkexec /usr/libexec/snosi-sysupdate-stage` | pkexec (`org.frostyard.ChairLift.sysupdate.stage`) | 30min (`DefaultContext`) | Streaming; idempotent; dry-run aware |
| `StageScriptAvailable()` | `os.Stat(StageScriptPath)` | none | — | Without the script the system image source is unavailable |

### Progress UI (`internal/views/system.go`)

Native A/B staging is the same `System.stageJob` as bootc's (see bootc's
"Progress UI" above), with `sysupdate.StageUpdate` as the stager. After the
job, `readDeployments` reads `GetStatus()` and `RollbackVersion()` from real
state (not stream output), so the source's note
(`pageview.SysupdateUpdateSubtitle`, or `pageview.SysupdateStageResultSubtitle`
after a live run that staged nothing), the rollback line, the staged slot and
the toast (`actionmsg.SysupdateStage`) all come from the state files and
partition labels. The first read (`System.loadDeployments`) runs the same
gates, `IsNativeABCached()` and `StageScriptAvailable()`.

## Updex (`internal/updex/updex.go`)

Manages system features (add-on software/configuration modules). Unlike other wrappers, updex does **not** shell out to a CLI for reads. It uses the `github.com/frostyard/updex/v2/updex` Go library directly for read operations, with a singleton `*updexapi.Client`. Write operations that require root are delegated via pkexec to the fixed absolute path `internal/updex.HelperPath` (`/usr/bin/chairlift-updex-helper`, built from `cmd/chairlift-updex-helper/main.go`) — never a bare, `$PATH`-resolved name, since `pkexec` matches the resolved absolute path against `data/org.frostyard.ChairLift.updex.policy`'s `org.freedesktop.policykit.exec.path` annotation to select the right action; see [overview.md](./overview.md#privileged-operations) for the full rationale and the matching `PREFIX=/usr` Makefile requirement.

### Key types

Type aliases to `github.com/frostyard/updex/v2/updex`:
- **`Feature`** (`FeatureInfo`) — name, description, enabled flag, documentation URL
- **`FeatureCheck`** (`CheckFeaturesResult`) — feature name plus `Results []CheckResult`, one entry per component, each carrying its own update-available flag and versions. There is no feature-level update flag: a feature has an update when *any* of its components does (see [`internal/views/featurestatus`](#view-layer-feature-update-status-internalviewsfeaturestatus))
- **`CheckResult`** — component name, current/available versions, and a structured per-component error when that component could not be checked; `NeedsImage` is `"needs image >= <version>"` when the component's newest staged image is not linked and `features update` would not link it, which is a wait for a newer OS image and not an error (updex ADR-0016; see [`internal/views/featurestatus`](#view-layer-feature-update-status-internalviewsfeaturestatus))
- **`FeatureUpdate`** (`UpdateFeaturesResult`) — feature name plus `Results []TransferUpdate`, the per-feature result of the update command; `RefreshSkipped` is host-wide
- **`TransferUpdate`** (`UpdateResult`) — one component's update result: `Downloaded`, `Linked`, `Installed`, `Held`, `NeedsImage`, `NotLinked`, `Error`, `NextActionMessage`

### Operations

| Function | Implementation | Mode | Timeout | Notes |
|----------|---------------|------|---------|-------|
| `IsInstalled()` | Go library: `client.Features()` (through the unexported `featuresLister` test seam) | Direct | 3s | Checks if updex features are configured: requires `err == nil` **and** `len(features) > 0`, because updex v2.2.0's `Features` returns an empty list with a nil error when no feature definitions exist; an empty list makes the Features page show "Feature Manager Not Available" and the features update source unavailable |
| `IsInstalledCached()` | Cached `IsInstalled()` | Direct | — | `sync.Once`, runs check at most once |
| `ListFeatures()` | Go library: `client.Features()` | Direct | 5min | Returns `[]Feature` |
| `CheckFeatures()` | Go library: `client.CheckFeatures()` | Direct | 5min | Preserves successful components when v2 reports structured partial failures, logs failed-component metadata, and returns `[]FeatureCheck`; true request failures still return an error |
| `EnableFeature(name)` | `pkexec /usr/bin/chairlift-updex-helper enable-feature <name>` | pkexec | 5min | State-changing |
| `DisableFeature(name)` | `pkexec /usr/bin/chairlift-updex-helper disable-feature <name>` | pkexec | 5min | State-changing |
| `UpdateFeatures()` | `pkexec /usr/bin/chairlift-updex-helper update` | pkexec | 5min | Downloads enabled features; returns `([]FeatureUpdate, error)` (see "Update results") |

### Update results

Since updex v2.1.0, `Client.UpdateFeatures` returns a nil error when the
next-boot image gate holds a feature or a rollback record holds it. The
transfer result then carries `NeedsImage` ("needs image >= X") or
`NotLinked` (with `Installed` false, nothing linked), or `Held` (the old links
stay; `Installed` is true when the held link names a version). A nil error
therefore does not mean every feature updated. updex v2.1.3 and later can also return an error that wraps
`sysext.ErrActivationIncomplete`: the extensions merged, but activation did
not finish, and a reboot finishes it. On a per-transfer failure updex returns
the results and an error together.

The helper prints the results on stdout in every one of those cases
(`updexhelper.WriteUpdateResult`) and exits 0, 1, or 3 (see the exit-code
contract in [overview.md](./overview.md#privileged-operations)).
The helper writes the JSON to the real stdout only: `main` sets
`os.Stdout = os.Stderr` before the SDK runs, because the SDK's
`sysext.DefaultRunner` sends `systemd-sysext`, `systemd-sysusers` and
`systemd-tmpfiles` output to `os.Stdout`, which would otherwise come before the
JSON. `updex.UpdateFeatures` parses stdout with `parseUpdateResults`: first the
whole trimmed stdout as a JSON array, then the last non-empty line (a helper
from an older ChairLift could have child text before its JSON). Empty stdout,
`null`, or no decodable array is a parse error, logged. The helper error wins:
with a non-nil helper error the parsed results (or nil) come back with it. When
the helper exits 0 and the parse fails, the result is nil results with
`*ResultsUnreadableError`: the outcome is unknown and must not read as success.
Exit code 3 gives
`*ActivationIncompleteError`; any other non-zero exit gives `*Error`
("command failed (exit N): stderr"), so a helper from an older ChairLift,
which prints nothing on stdout on failure, gives the old "Update failed: ..."
behaviour. Under dry-run no helper runs and the result is nil, nil.
`runHelper` returns the captured stdout also when the helper exits non-zero.
It reports the `pkexec` command through `cmdlog` (`Start`, the stdout and
stderr tee, `End`), so the helper's output shows in the console; under
dry-run it reports `cmdlog.DryRun` instead and starts nothing.

`SummarizeUpdate` (`internal/updex/summary.go`, no UI import) classifies
each feature as one `UpdateClass`, by this precedence: `ClassFailed` (any
transfer has `Error`; reason: the first `Error`) > `ClassWaitingForImage` (any
transfer has `NeedsImage` or `NotLinked`; reason: each `NeedsImage` transfer as
"component: needs image >= X" joined with ", ", or, when only `NotLinked`
transfers exist, the first `NextActionMessage` without its "not linked: "
prefix) > `ClassHeld` (any `Held`; reason: the first `NextActionMessage`) >
`ClassUpdated` (any `Downloaded` or `Linked`) > `ClassCurrent`. The summary
also keeps the host-wide `RefreshSkipped` text. `actionmsg.FeatureUpdate`
turns the summary into the toast.

The check side names the same wait. `CheckFeatures` sets `CheckResult.NeedsImage`
("needs image >= X") for a component whose newest *staged* image is not linked
and whose next `features update` will not link it (updex ADR-0016), and
`featurestatus.Feature` turns that into the row's "waiting for a newer OS
image" subtitle, which takes precedence over the update wording. Because the
check reads staged images only, it cannot know about a newer *published* image
that no update has downloaded yet: such a feature reads as update-available or
up to date until an update stages the image, and then the check that
`Features.updated` runs again after the update reports the wait (see the
`featurestatus` section).

**The unprivileged bootc fallback.** `CheckFeatures` runs as the ordinary,
unprivileged GUI user. The minimum image version gate compares a staged image
with the `IMAGE_VERSION` of the image that boots next, which updex reads with
`bootc status --json` (`imagecompat.NextBootImageVersion`); on a bootc host
that command refuses an unprivileged caller ("This command must be executed as
the root user"). Without a fallback the gate then errored on every component of
such a feature, `partitionFeatureChecks` dropped those components, and the
"waiting for a newer OS image" state never reached the page at all. The
read-only check client that `getClient` builds therefore uses
`updexapi.ClientConfig{BootcRunner: newCheckBootcRunner()}`: that runner
(`internal/updex/bootcrunner.go`) passes success, a missing bootc, a cancelled
context and root's own answer through unchanged, and turns any other
unprivileged failure into `exec.ErrNotFound`, which is exactly the answer
`NextBootImageVersion` falls back on — `IMAGE_VERSION` of the booted OS image
from os-release. The privileged link gate does not change: the root helper
builds its client with `updex.ClientConfig{}` and the real bootc, so it keeps
failing closed. Known limit: when bootc has staged a newer OS deployment, the
booted os-release is older than the image that boots next, so a row can read
"waiting for a newer OS image" although the next update would link; the reboot
into that OS clears it. Decision record:
[ADR-0011](../adr/0011-unprivileged-feature-check-compares-with-the-booted-image.md).

Updex v2 returns populated results plus an aggregate error when any component
check fails. `partitionFeatureChecks` removes only the error-bearing component
entries, returns their feature/component/error metadata for logging, and
suppresses that aggregate error; an error with no structured component failure
remains a whole-request failure. This preserves the v1 behavior in which
successful components still contribute to the update count. When every
component of a feature fails, `Summarize` leaves that feature out of `Rows`,
so `Features.subtitle` gives its row the base subtitle back (the
`baseSubtitle` it had before any check), and the description
still says "all up to date" when no other feature has an update or a wait,
while the component failures are logged. A whole-request failure is different:
the check returns an error, every row keeps its current subtitle, and the
description becomes `GroupDescriptionCheckFailed`. That wording is a known compatibility limitation; presenting an
explicit partial-failure state is a separate product change.

`updex_test.go` drives all three public write operations through a fake
`pkexec` on `PATH` and asserts that argv always begins with the fixed
`HelperPath`, followed by the exact subcommand/name shape. It also covers
missing-pkexec, non-zero/stderr, timeout, default-context, and wrapper dry-run
outcomes. A scripted fake `pkexec` also covers `UpdateFeatures` for exit 0 with JSON, exit 1 with JSON and stderr, exit 3 with JSON, exit 1 with empty stdout (an older helper) and exit 0 with child-process text before the JSON, with bad JSON and with empty stdout (both `*ResultsUnreadableError`), and exit 3 with text before the JSON; `TestParseUpdateResults` covers the whole-stdout and last-line forms; `summary_test.go` covers every `SummarizeUpdate` class and the precedence. Multi-component update aggregation remains in the pure
`internal/views/featurestatus` package, where every result is inspected and
the full decision table is tested as plain functions.

### Helper binary (`cmd/chairlift-updex-helper/main.go`)

A small standalone binary that accepts commands (`enable-feature`, `disable-feature`, `update`) and uses the updex Go library to perform privileged operations. It supports `--dry-run` for all three subcommands — `enable-feature`, `disable-feature`, and `update` — passing it through to the corresponding `updex.*Options.DryRun` field. Outputs JSON to stdout. Invoked via pkexec so that the main chairlift process does not need root.

`main.go` itself is thin dispatch only: strict `os.Args` parsing and each
subcommand's `Options` struct live in `internal/updexhelper`
(`internal/updexhelper/updexhelper.go`), a package with no UI import —
only stdlib plus `github.com/frostyard/updex/v2/updex` and `.../sysext`. That's what makes the
logic testable at all: neither `make verify` nor `make ci` ever runs `go test
./...`, both are scoped to `go test ./internal/...`, so a `_test.go` under
`cmd/chairlift-updex-helper` would never execute under any gate this repo
actually runs. `ParseInvocation` accepts only `enable-feature <name>
[--dry-run]`, `disable-feature <name> [--dry-run]`, and `update [--dry-run]`;
`SupportedCommands` is the complete first-argument surface matched by the
PolicyKit actions. `EnableOptions`, `DisableOptions`, and `UpdateOptions` set
`DryRun` exactly while leaving every other v2 option at its zero value.
`WriteResult` serializes successful SDK results and returns SDK errors unchanged
to `main`'s non-zero fatal path, including final sysext refresh failures; it
stays in use for `enable-feature` and `disable-feature`. The `update` command
uses `WriteUpdateResult`, which also writes non-nil results when the SDK error is
non-nil, then returns the error; `main` prints the error on stderr and exits
with `ExitCode(err)`: `ExitActivationIncomplete` (3) when the error wraps
`sysext.ErrActivationIncomplete`, else `ExitFailure` (1). `internal/updexhelper`
imports only stdlib, `github.com/frostyard/updex/v2/updex` and
`github.com/frostyard/updex/v2/sysext`. Tests
cover every accepted/rejected argv shape, the immutable command inventory, all
three complete option structs, successful JSON output, refresh-error
propagation, `ExitCode` (nil, plain, wrapped and joined
`ErrActivationIncomplete`) and `WriteUpdateResult` (results written on error, nothing written for nil results).

## Cross-cutting: dry-run

Every wrapper has `SetDryRun(bool)` and `IsDryRun() bool`. `--dry-run` (or
`-d`) makes `app.Run` call `app.SetDryRun`, which sets the flag of every
wrapper below once, before the window exists; the same flag reaches the views
as `Env.DryRun` through `views.NewEnv`. Behavior varies by wrapper:

| Wrapper | Dry-run behavior | Set by `app.SetDryRun`? |
|---------|-----------------|------------------------|
| Homebrew | Skips state-changing commands, returns a mock message, logs `[DRY-RUN] Would execute: brew ...` and reports `cmdlog.DryRun` | Yes |
| Flatpak | Skips state-changing commands, returns a mock message, logs `[DRY-RUN] Would execute: flatpak ...` and reports `cmdlog.DryRun` | Yes |
| fwupd | `Install` validates the id, then logs `[DRY-RUN] Would execute: fwupdmgr update ...` and runs nothing; `InstallAndVerify` makes no query | Yes |
| bootc | `StageUpdate` never invokes pkexec; `stageexec.DryRun` reports `cmdlog.DryRun`, emits synthetic `EventMessage`+`EventComplete` and returns. `System.finishStage` shows an explicit `actionmsg.BootcStage(dryRun, staged)` preview toast, distinct from its normal staged/up-to-date toasts; the image source's note and the deployment chain intentionally stay live (from `bootc.GetStatus()`) in both modes | Yes |
| sysupdate | `StageUpdate` never invokes pkexec; emits synthetic `EventMessage`+`EventComplete` and returns. The toast is `actionmsg.SysupdateStage(dryRun, staged)`; the note and the rollback line stay live (from the `/run/snosi` state files and partition labels) in both modes | Yes |
| Updex | Skips helper execution, logs `[DRY-RUN] would execute: pkexec /usr/bin/chairlift-updex-helper ...` and reports `cmdlog.DryRun`, and returns nil results and a nil error (the features update toast is the `actionmsg.FeatureUpdate` preview); the helper binary itself (`cmd/chairlift-updex-helper`, via `internal/updexhelper`) also honors `--dry-run` for all three subcommands, defense-in-depth even though `updex.runHelper` never invokes pkexec under dry-run | Yes |
| firstrun (first-run mode) | `ApplyColorScheme` runs no `gsettings` and logs `[DRY-RUN] would execute: gsettings ...`; `Complete` writes no marker and logs `[DRY-RUN] would write first-run markers: ...`, and its `Completion` decision gives the Done step the preview text. The first run's `remote-add` and `install` go through the Flatpak wrapper's own flag (see [overview.md — First-run mode](./overview.md#first-run-mode)) | Yes |

Custom maintenance scripts (config.yml `actions` entries) have no wrapper
package of their own, so `Maintenance.job` (`internal/views/maintenance.go`)
reads `Env.DryRun`. The execution gate is not an inline `if` either:
`actionmsg.MaintenanceScript(m.env.DryRun, title)` returns a
`ScriptDecision{Execute, Toast}` computed once, on the UI thread, before the
job is queued, and both "does it execute" and the toast text come from that
single tested call. When `Execute` is false the job constructs no `exec.Cmd`
(no `pkexec`, no direct script exec): it logs `[DRY-RUN] Would execute: ...`
and reports `cmdlog.DryRun`. A live run goes through `runMaintenanceCommand`,
which streams the script's output to the console through `cmdlog`. See
"View-layer toast and decision helpers" above for the full
`actionmsg`/`trustmsg` function and type list.

The decisions that pair a toast with a state change follow the same rule in
every domain; each is described with its caller above:

- **Bundles** — `homebrew.BundleInstall` already skips `brew bundle install`
  under dry-run, but a nil wrapper error does not mean the bundle is
  installed: `actionmsg.BundleInstall` returns `Complete: false` with a
  preview, and `Brew.runBundle` resets the bundle's `bundleview.InstallGate`
  (see "Configured Brew bundle discovery").
- **Homebrew installs, uninstalls, pins, upgrades and `brew update`** — the
  `actionstate` decisions restore the control after a dry run and request no
  refresh, because nothing changed; the toast is the `actionmsg` preview (see
  "Typed search and install", "Installed Homebrew actions" and "Homebrew
  update source").
- **Tap trust** — `actionmsg.TapTrust(b.dryRun(), tap)` keeps the tap, its
  banner and its held packages after a dry run (see "Tap trust").
- **Flatpak updates and uninstalls** — `Flatpak.finishUpdate` and
  `finishUninstall` use `actionstate.PackageUpgrade` and `PackageUninstall`
  with `flatpak.IsDryRun`: a dry run keeps the item and the installed app,
  shows the `actionmsg.Update`/`Uninstall` preview and reloads nothing. A live
  update removes its item before it reloads, because a reload in which both
  installation queries fail and neither returns updates keeps the old items
  (`flatpak_test.go`'s `TestFlatpakLiveUpdateRemovesRowBeforeTheReload` and
  `TestFlatpakFailedOrDryRunUpdateRestores` pin this).
- **Features** — `Features.toggled` computes
  `decision := actionmsg.FeatureToggle(f.env.DryRun, on, name)` once after a
  successful `updex.EnableFeature`/`DisableFeature` job and changes the
  feature's state only when `decision.Confirm` is true; under dry-run
  `updex.runHelper` returns before it invokes pkexec, so nothing was toggled
  and the row and tile keep the listing. A feature's switch and tile never
  change before the job ran: `AskToggle` asks first, and the job's result
  decides. `Features.updated` computes
  `decision := actionmsg.FeatureUpdate(f.env.DryRun, results, err)` and shows
  `decision.Toast` as an error toast when `decision.Persistent`, else as a
  plain toast; the job logs the error and every feature and transfer outcome
  (`logFeatureUpdate`), also to the console.
- **System image** — `System.finishStage` (see bootc's "Dry-run behavior").
- **Firmware** — `actionmsg.FirmwareInstall` with `fwupd.IsDryRun` (see
  fwupd's "View rules").

## Install-path consistency (`internal/installcheck`)

The source `make install` path (Makefile, `PREFIX` defaulting to `/usr`) and
the two packaged nFPM (deb/rpm/apk) layouts GoReleaser builds from
`.goreleaser.yaml` ship this repository's privileged surface and maintainer
configuration. They are hand-maintained text — a Makefile recipe and YAML
blocks — with no shared code path, so nothing stops them (or
`internal/updex.HelperPath`, the fixed absolute path `pkexec` matches against
the policy's `exec.path` annotation) from silently drifting apart.

ChairLift packages the bootc, sysupdate, and updex `.policy` files. It no longer
ships its old `.rules` files, which returned `YES` for every active local
member of the `sudo` group and bypassed authentication. Source installation
explicitly removes those legacy rule paths; package upgrades remove them as
obsolete tracked files. All three policies use normal administrator
authentication, while the updex policy selects one action for each supported
first argument and the helper validates the complete argv shape.

All three layouts install the repository's `config.yml` as package-owned
maintainer defaults at `/usr/share/chairlift/config.yml`. None installs
`/etc/chairlift/config.yml`: that higher-precedence path belongs to the
administrator and must survive package installation and upgrades unchanged.

GoReleaser has two nFPM entries. `frostyard-chairlift` is self-contained and
selects both `chairlift` and `chairlift-updex-helper` builds.
`frostyard-chairlift-system-integration` selects only the helper and packages
only maintainer config plus the bootc/sysupdate/updex policies, for pairing with a
user-scoped app installation. The two package names conflict to prevent
simultaneous ownership of the same fixed system files. The companion does not
provide either OS stager; a distro must provide trusted implementations at
`/usr/libexec/bootc-update-stage` and, on native A/B hosts,
`/usr/libexec/snosi-sysupdate-stage`. This split is decision
record [ADR-0006](../adr/0006-split-system-integration-package-with-mutual-conflicts.md).
Both package names must be registered for `frostyard/chairlift` in
frostyard/apt-publisher's `config/producers.tsv`: apt-publisher publishes every
`.deb` of a tag release and refuses the whole release if one carries an
unregistered name.
`TestGoreleaserDebPackagesAreRegisteredForApt` fails when a deb-producing
`nfpms[]` entry is added or renamed, so the registration changes first.

`internal/installcheck` holds regression tests, not production code, that turn
"verified by inspection" into real, gated checks. The first two guard the
installed layout itself:

- **`TestMakefileInstallUsesUsrPrefix`** runs `make -n install
  DESTDIR=<t.TempDir()>` — a dry run, so no compilation, no writes outside
  the temp dir, and no root — once with no `PREFIX` override and once with
  `PREFIX=/usr`, and asserts the printed `install -Dm...` lines place the
  updex helper at `DESTDIR` + `internal/updex.HelperPath` and all three
  policies under the fixed `/usr/share/polkit-1/actions` directory PolicyKit
  reads,
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
  `docs/design/engineering-lessons.md#check-every-collection-entry`),
  asserts each entry's `bindir` matches the directory of
  `internal/updex.HelperPath`, its updex/bootc/sysupdate policy
  `contents[].dst` entries
  equal the fixed polkit-1 actions paths, their policy/config modes remain
  `0644`, and no `.rules` content remains. It also requires every package to
  map the repository `config.yml` to
  `/usr/share/chairlift/config.yml` and rejects any content entry targeting
  `/etc/chairlift/config.yml`.
- **`TestGoreleaserPublishesSystemPackage`** requires exactly one
  full package and one integration package, verifies their build filters,
  mutual conflicts, unique IDs, and the integration package's exact four
  content mappings. This prevents the companion from accidentally acquiring
  the GUI binary or losing one of the root-owned integration files.

Both tests fail — not skip — if `internal/updex.HelperPath`, the Makefile's
`PREFIX` default, or `.goreleaser.yaml`'s `nfpms` block change independently
of one another; each was hand-verified during development by reverting one
of the three at a time and confirming only the test(s) that source depends
on turn red. The package imports no UI package, directly or transitively,
and it lives under `internal/...` so `make verify`, `make ci`, and CI's identical
`go test ./internal/... -run "^Test[^I]" -skip "Integration"` filter all
exercise it on every run, per
`docs/design/engineering-lessons.md#gates-run-only-internal-tests`.

A third regression test, **`TestGoreleaserLicenseIsGPL`**, guards a related
but distinct drift: `.goreleaser.yaml` briefly declared `license: MIT` in
both its top-level `metadata:` block and its `nfpms[]` entry's `license`
field, while the project's actual license is GPLv3-or-later (`LICENSE`, and
the About dialog of `internal/window/main.go`, `Main.showAbout`, which names
"GNU GPL 3.0"). Like the layout test above, it parses the
real, repo-root `.goreleaser.yaml` via the shared `loadGoreleaserConfig`
helper (no fixture) and asserts `cfg.Metadata.License` and, iterating
**every** `nfpms[]` entry with a per-index `t.Run` (not just `nfpms[0]`, per
`docs/design/engineering-lessons.md#check-every-collection-entry`),
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
not run anywhere — locally, in `make verify`, or in `make ci` — and neither
test shells out or renders anything. As with the license guard,
`MetadataConfig.Homepage` and `ReleaseConfig.Footer` in
`internal/installcheck/installcheck.go` exist solely so `yaml.Unmarshal` has
somewhere to put those values, exactly as `MetadataConfig.License` does;
without the struct fields yaml.v3 drops them and both tests would pass
vacuously regardless of what the YAML says.
