# ChairLift Overview

Living design document (formerly `yeti/OVERVIEW.md`; folded into `docs/` per
[frostyard/core ADR-0025](https://github.com/frostyard/core/blob/main/docs/adr/0025-consolidate-repository-docs-into-docs.md)).

## Purpose

ChairLift is the command center of [Snow Linux](https://github.com/frostyard/snosi): a system-management GUI written in Go on [MyGo](https://mygo.egoist.dev) (`github.com/egoist/mygo`), which draws its own widgets on GTK 3 and OpenGL loaded at run time with purego — no CGO ([ADR-0017](../adr/0017-mygo-command-center-window.md)). One window manages the OS image (staged via the snow `bootc-update-stage` script on bootc installs, or the snow `snosi-sysupdate-stage` script on native A/B installs), system features (via updex), Flatpak apps, Homebrew packages, device firmware (fwupd), the problems recorded on the computer, and cleanups. The window is a command center: a top bar with a command palette, a left rail of six pages (Overview, Updates, Apps, Features, Problems, Maintenance), a console that streams the commands ChairLift runs and their output (see [The console](#the-console-internalcmdlog) for the few that bypass it), and a status bar; Help is a dialog. The UI is YAML-configuration-driven: every card sits behind its configuration group, which makes ChairLift portable to other Linux distributions by toggling groups off. A separate first-run mode (`chairlift --first-run`, started by the pure-Go `chairlift-first-run` gate) replaces SNOW First Setup on images that ship its trigger; see [First-run mode](#first-run-mode). The Problems page shows the problems that the system checks, the user's crashes and frostyard apps recorded on this computer, and sends nothing until the user opts in to reporting; see [Problems page](#problems-page). The UI kit the window is drawn with is a module of its own, `github.com/frostyard/frostui`; see [frostui.md](./frostui.md).

## Architecture

```
cmd/chairlift/main.go                 Entry point: version injection, selfreport, --send-pending-reports, --help, app.Run
cmd/chairlift-updex-helper/main.go    Privileged helper for updex write operations
cmd/chairlift-first-run/main.go       Pure-Go first-run gate: exits or execs `chairlift --first-run`
        │
internal/app/app.go             MyGo bootstrap: app ID, single-instance lock, dry run, console wiring, poster
        │
internal/window/main.go         Command-center window: frostui shell, rail, palette, shortcuts,
        │                       Help/About/Shortcuts dialogs, status bar
internal/window/firstrun*.go    First-run wizard: state and flow (firstrun.go), drawing
        │                       (firstrun_view.go), the optional Reporting step (firstrun_reporting.go)
        │
internal/views/                 The six pages and the state they share, drawn through frostui
        │   env.go source.go store.go                  Env, the contracts (Page, UpdateSource, ...), Store
        │   overview.go system.go live.go              Overview; deployments, image source, live sampler
        │   updates.go firmware.go                     Updates; the firmware source
        │   apps.go flatpak.go brew*.go                Apps; the Flatpak and Homebrew domains and sources
        │   features_page.go features.go               Features; the updex domain and source
        │   problems_view.go problems.go problems_reporting.go   Problems
        │   maintenance_view.go maintenance.go         Maintenance
        │                       ├── internal/views/actionmsg/     ┐ pure leaf packages:
        │                       ├── internal/views/actionstate/   │ toast/decision text, async gates,
        │                       ├── internal/views/bundleview/    │ bundle action state, Flatpak and
        │                       ├── internal/views/trustmsg/      │ feature update-status text and
        │                       ├── internal/views/flatpakstatus/ │ decisions, page text, all
        │                       ├── internal/views/featurestatus/ │ table-tested
        │                       └── internal/views/pageview/      ┘
        │
        ├── internal/navigation/ The six pages in rail order, shortcuts, page visibility, pure navigation transitions
        ├── internal/cmdlog/    Command tracer: the backends report each command, its output and its end; the console subscribes
        ├── internal/sysinfo/   Host facts: /proc readings, statfs, uptime, kernel, DMI model, Secure Boot state
        ├── internal/activity/  ChairLift's own record of what it changed ($XDG_STATE_HOME/chairlift/activity.jsonl)
        ├── internal/config/    YAML config loading, feature group enablement
        ├── internal/homebrew/  Homebrew CLI wrapper (JSON output parsing)
        ├── internal/flatpak/   Flatpak CLI wrapper (tabular output parsing)
        ├── internal/fwupd/     fwupd CLI wrapper (JSON update list, unprivileged install)
        ├── internal/bootc/     bootc wrapper (status reads, fixed stage adapter)
        ├── internal/sysupdate/ native A/B detection, status, rollback, fixed stage adapter
        ├── internal/stageexec/ Pure-Go shared OS staging stream/event executor
        ├── internal/updex/     Updex feature manager (Go library reads, helper binary writes)
        ├── internal/updexhelper/ UI-free argv parsing and Options building for cmd/chairlift-updex-helper
        ├── internal/firstrun/  UI-free first-run logic: gate, markers, steps, apps list, actions
        ├── internal/i18n/      UI-free .po reader and catalog lookup for the first run (no translation shipped)
        ├── internal/problems/  UI-free, network-free Problems page data: sources, v1 validation, hints, dismiss store
        ├── internal/reporting/ The only sender: consent, sent log, HTTPS request, run rules
        ├── internal/selfreport/ ChairLift's own problem recorder (std problems) and its error codes
        └── internal/version/   Build metadata (ldflags injection)
```

### Dependency flow

`cmd/chairlift → app → window → views → {config, homebrew, flatpak, fwupd, bootc, sysupdate, updex, problems, reporting, sysinfo, activity, cmdlog}`.
`app` also sets every backend's dry-run flag, opens `activity`, and installs the `cmdlog` sink that feeds the console.
`window` and `views` draw through `github.com/frostyard/frostui` (frostyard's UI kit on MyGo: shell, palette, console, job queue, dialogs, cards), a module of its own that imports only MyGo, purego and the standard library,
and both read the pure `navigation` package.
The backends that run commands (`homebrew`, `flatpak`, `fwupd`, `bootc`, `updex`, `stageexec`) report them through `cmdlog`, a leaf that imports no UI and no backend.
The first-run mode adds `cmd/chairlift-first-run → firstrun → {config, flatpak, i18n}`
(no UI toolkit, guarded by `internal/installcheck.TestFirstRunGateImportsNoGTK`) and `app → window (firstrun*.go) → {firstrun, i18n, reporting}`.
The Problems page adds `views (problems*.go) → {problems, reporting, selfreport}` and
`problems → {selfreport, github.com/frostyard/std/problems}`; `app`,
`window/firstrun.go`, `views/problems.go` and `updex` call `selfreport` to record ChairLift's own problems.
No package may load a UI library at init: MyGo loads GTK only in `App.Run`
(`internal/installcheck.TestNoGTK4Bindings` keeps puregotk and snowkit out).

### The application and the window (`internal/app`, `internal/window`)

`app.Run(opts)` (`internal/app/app.go`) starts the window:

1. With `--dry-run`/`-d`, `app.SetDryRun` puts every backend in dry-run mode
   (see [Dry-run mode](#dry-run-mode)).
2. `frostui.SetAppID(opts.ID(), "ChairLift")` calls GLib's `g_set_prgname`
   before MyGo opens the display, so the Wayland app ID and the X11
   `WM_CLASS` match the installed `org.frostyard.ChairLift.desktop` without
   renaming the binary. `Options.ID` is `app.AppID`
   (`org.frostyard.ChairLift`) or, with `--first-run`, `app.FirstRunAppID`
   (`org.frostyard.ChairLift.FirstRun`).
3. `mygo.App.SetName` and `mygo.App.RequestSingleInstanceLock` keep one
   process per ID: a second start asks the first to show its window
   (`OnSecondInstance` calls `Show` and `Focus`) and exits 0.
4. `frostui.SetIcons` registers ChairLift's own symbolic logo
   (`window.LogoSVG`) beside the desktop's icon theme.
5. `mygo.App.WhenReady` opens the main window (`openMain`, logging
   `ChairLift activated`, then `window.Ready` logs `app: window presented in
   …`) or the first-run window (`openFirstRun`). Those lines, with
   `Running in dry-run mode` and the Problems page's `problems: page built …`,
   are the E2E readiness markers
   ([ADR-0008](../adr/0008-e2e-readiness-is-a-log-marker-contract.md)).

`openMain` loads the configuration (`config.Load`; a failure records
`selfreport.CodeConfigLoadFailed` and the error stays in `Env.ConfigErr`),
builds `views.NewEnv`, wires the console (`wireConsole`, see
[The console](#the-console-internalcmdlog)), opens the activity record,
builds `views.NewStore` and `window.NewMain`, creates the MyGo window
(1360×880, at least 1040×640, title bar hidden) with `Main.View` as its
content, attaches the poster, logs `window: built (pages=…)` and calls
`Store.Start`.

**The poster.** `app.poster.post` is the `Env.Post` of the window: it is safe
from any goroutine and calls MyGo's `Window.Update`. A function posted before
the window exists is kept and handed over, in order, by `attach`, so a loader
that ends before the window is created never loses its result.

**`window.Main`** (`internal/window/main.go`) holds the `Env`, the `Store`, the
`frostui.Shell` and the visible `navigation.Items`. `NewMain` takes the
visible pages from `navigation.VisibleItems(env.Enabled)`, sets
`Env.Navigate` to `Main.Navigate`, and reads the Help links. `Main.View` runs
every frame: it runs the command the palette's last frame chose, shows the
toasts queued since the last frame (`Env.TakeToasts`), shows the configuration
error once as a toast that stays (`ID: "config-error"`, `Timeout: -1`,
`LoadError.ToastMessage`; ADR-0003), shows why a close was refused,
binds the shortcuts, and draws `Shell.View` with a `frostui.Frame`:

- the rail: each visible page's title, icon, Alt+number hint and
  `Page.Badge()` (Updates: `Store.UpdateCount`; Features: features with an
  update; Problems: problems shown);
- the top bar: the palette field, the Check for updates button
  (`Store.CheckAll`, disabled while a source checks) and the main menu
  (Command palette, Keyboard shortcuts, Help, About ChairLift, Quit);
- the rail footer: `frostui.JobCard` of the running job and the host card
  (host name, hardware model or OS);
- the selected page's `View`, the palette's `Main.Commands`, the status bar
  (`statusStart`, `statusEnd`) and the console.

**`frostui.Waker`** coalesces redraw requests from many goroutines into at most
one empty post per interval (60 ms in `wireConsole`): `Window.Update`
rebuilds the view on every call, and a command that prints thousands of lines
must not rebuild it thousands of times. The console's notify calls
`Waker.Wake`.

### Views and the Store (`internal/views`)

`Env` (`internal/views/env.go`) is what every page and domain gets: `Config`,
`ConfigErr`, `DryRun`, `Post` (the poster), `Jobs` (`frostui.NewJobs(post,
console)`), `Console` (`frostui.NewConsole(5000)`: the newest 5000 lines),
`Dialogs` (one queue of confirmations the shell draws), `Activity` and `Now`.
Its methods are `Enabled(page, group)` and `Group(page, group)`, the toasts
(`Toast`, `ToastSuccess`, and `ToastError`, which logs and stays until
closed; MyGo toasts are plain text and parse no markup), `Navigate`, `Run`
(queue one job) and `Record` (the activity record); the generic `views.Go`
is the loader helper (see [UI thread](#the-ui-thread-envpost-and-go)).

`Store` (`internal/views/store.go`) is ChairLift's state: one domain struct per
backend, each owned by its file — `System` (`system.go`), `Live` (`live.go`),
`Features` (`features.go`), `Flatpak` (`flatpak.go`), `Brew` (`brew*.go`),
`Firmware` (`firmware.go`), `Problems` (`problems.go`,
`problems_reporting.go`) and `Maint` (`maintenance.go`) — and the six pages.
`NewStore` builds them and loads nothing; `Store.Start` starts each domain's
first load, and each domain loads only what its enabled groups need.
`Store.Page(id)` returns a page by its navigation ID. `Store.Sources`,
`Store.UpdateCount`, `Store.EverythingJobs`, `Store.UpdateEverything`,
`Store.CheckAll` and `Store.Checking` work across the update sources.

The contracts (`internal/views/source.go`):

- **`Page`**: `View(c)`, `Commands()` (its palette entries) and `Badge()`
  (the rail count and its tone).
- **`UpdateSource`**: one of the five sources of the Updates page, in run
  order — `SourceImage` (`System.Source`, bootc or native A/B), `SourceFeatures`
  (`Features.Source`), `SourceFlatpak` (`Flatpak.Source`), `SourceBrew`
  (`Brew.Source`) and `SourceFirmware` (`Firmware.Source`). It gives
  `Status()` (`Available`, `Checking`, `Err`, `CheckedAt`, `Note`), `Items()`
  (`UpdateItem`: key, title, detail, from/to versions, `Held` with
  `HeldWhy`, `Confirm`, the button's `Action`), `Refresh()`, `Jobs(keys...)`,
  `Update(key)` and `Extra(c)` (what only that source draws: a tap's trust
  banner, the native A/B rollback line). A source whose groups are disabled
  is nil, except the Features source, whose `Status().Available` says
  whether it applies; `Store.Sources` keeps only the available ones.
- **`SystemInfo`** and **`Deployment`** (what the Overview hero, the status
  bar, the rail's host card and Maintenance show of the host),
  **`ProblemSummary`** and **`FeatureSummary`** (what the Overview and the
  status bar show of those pages).
- **`AppEntry`**, **`AppKind`**, **`AppAction`** and **`AppsStatus`**: a row of the
  Apps table, which `Flatpak.Apps` and `Brew.Apps` each give, and the
  inspector's actions (`ActUpdate`, `ActOpen`, `ActPin`, `ActUnpin`,
  `ActUninstall`), dispatched to `Flatpak.Act` or `Brew.Act`.

Each domain takes its backend as function fields that the constructor fills
with the real wrappers, so the views' tests (`ui.NewTester`, headless, with
Pango and cairo but no GTK and no display) inject fakes and no test runs
brew, flatpak, bootc, fwupdmgr, updex or pkexec. The snapshot tests
(`TestSnapshotOverview`, `TestSnapshotUpdates`, `TestSnapshotApps`,
`TestSnapshotBrew`, `TestSnapshotFeatures`, `TestSnapshotProblems`,
`TestSnapshotMaintenance`, and `internal/window`'s `TestSnapshotFirstRunSteps`)
write PNGs of the pages when `CHAIRLIFT_SNAPSHOT_DIR` is set.

Decisions and text stay in seven pure leaf packages under `internal/views/`,
each table-tested: `internal/views/actionmsg` and `internal/views/trustmsg`
(toast text and UI decisions, see [package-managers.md](./package-managers.md#view-layer-toast-and-decision-helpers-internalviewsactionmsg-internalviewstrustmsg)),
`internal/views/actionstate` (Homebrew and Flatpak action outcomes, repeated-click
gates and refresh generations, see [package-managers.md](./package-managers.md#view-layer-update-action-state-internalviewsactionstate)),
`internal/views/bundleview` (Brew bundle presentation and per-bundle install
gating, see [package-managers.md](./package-managers.md#view-layer-brew-bundle-state-internalviewsbundleview)),
`internal/views/flatpakstatus` (the Flatpak source's status text and
whether a load is authoritative, applied by `Flatpak.loadUpdates`, see
[package-managers.md](./package-managers.md#view-layer-flatpak-update-status-internalviewsflatpakstatus)),
`internal/views/featurestatus` (each feature's update-status line — an
update, a feature that waits for a newer OS image, a new version that
applies after a reboot, an up-to-date feature — summarised at once by
`Summarize`, and the features description for every check outcome:
`GroupDescriptionCheckFailed` when the check itself failed and
`GroupDescription` when it completed; applied by
`Features.checkFeatureUpdates`, which composes no status text of its own and
runs again after an update through `Features.recheckFeatureUpdates`, see
[package-managers.md](./package-managers.md#view-layer-feature-update-status-internalviewsfeaturestatus)),
and `internal/views/pageview` (row text, page status, os-release parsing,
Help link order, maintenance-command selection, the Overview's image
channel and deployment notes, Homebrew text, and the Problems page's card,
status and detail text, see [package-managers.md](./package-managers.md#view-layer-page-presentation-internalviewspageview)).
`pageview/wiring_test.go` maps every non-test file of `internal/views` to a
domain and checks that each domain calls its pure helpers.
[ADR-0007](../adr/0007-pure-leaf-packages-route-around-untestable-gtk.md)
introduced the leaf packages because the GTK 4 window packages could hold no
tests; [ADR-0017](../adr/0017-mygo-command-center-window.md) superseded its
rule that window packages hold no tests, and kept the leaf packages.

### Pages

The window has six pages in the rail, in this order (`internal/navigation`).
Help is not a page, and neither is the first-run wizard
(`internal/window/firstrun*.go`), which `internal/navigation` does not list.

| Page        | Files (`internal/views/`)                                  | Purpose                                                                                                                                                                                                                                                                                  | Configuration groups                                                                                                                                             |
| ----------- | ---------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Overview    | `overview.go` (with `system.go`, `live.go`)                | The system image hero (OS name, channel, Secure Boot, the deployment chain Rollback → Booted → Staged, the update state and its main action: Check now, Update everything or Review), a strip of figures (Updates, Problems, Features on, Free space), the live chart and the disk card, System details and Recent activity | `system_page`: `bootc_status_group` (the hero), `system_info_group` (System details), `health_group` (the live chart and the system monitor button). Always shown |
| Updates     | `updates.go` (with `firmware.go` and each domain's source) | Check now and Update everything, the update run as a pipeline of every source's jobs, and one card per available source with a row per item                                                                                                                                              | `updates_page`: `bootc_updates_group`, `sysupdate_updates_group`, `firmware_updates_group`, `flatpak_updates_group`, `brew_updates_group`, `brew_trust_group`; the Features card follows `features_page`'s `features_group` |
| Apps        | `apps.go` (with `flatpak.go`, `brew*.go`)                  | Installed: one sortable, filterable table of Flatpak apps and Homebrew formulae and casks with an inspector (Update, Open, Pin/Unpin, Uninstall); Bundles: the configured Brewfiles; Find more: Homebrew search and install; the button that opens the configured software center         | `applications_page`: `applications_installed_group` (the software center button), `flatpak_user_group`, `flatpak_system_group`, `brew_group`, `brew_search_group` (Find more), `brew_bundles_group` (Bundles) |
| Features    | `features_page.go` (with `features.go`)                    | A toggle tile per feature that is on, a list of every feature with its state, version, update line and switch, Check for updates and Update features; toggles ask first                                                                                                                   | `features_page`: `features_group`                                                                                                                                |
| Problems    | `problems_view.go` (with `problems.go`, `problems_reporting.go`) | The system and apps problem lists as expandable cards, and the reporting card ([Problems page](#problems-page))                                                                                                                                                                    | `problems_page`: `problems_system_group`, `problems_apps_group`, `problems_reporting_group`                                                                      |
| Maintenance | `maintenance_view.go` (with `maintenance.go`)              | The Clean up card (configured actions, Homebrew cleanup, unused Flatpak runtimes; Clean everything), the free space and, read-only, the deployment chain                                                                                                                                  | `maintenance_page`: `maintenance_cleanup_group`, `maintenance_brew_group`, `maintenance_flatpak_group`                                                          |

How the configuration maps onto the window: the schema did not change with
the redesign. `system_page` feeds the Overview's cards; `help_page`'s
`help_resources_group` feeds the Help dialog's links; each `updates_page`
group is an update source of the Updates page (the Homebrew source shows
outdated packages with `brew_updates_group` and the untrusted-tap banners
with `brew_trust_group`, each without the other); each `applications_page`
group is a kind of row or a tab of Apps; `first_run_page` configures only the
first run. `navigation.VisibleItems` shows a page when any of its groups is
enabled; the Overview always shows, so the window always has a page.
`maintenance_page`'s deprecated `maintenance_optimization_group` is in no page's list. Cards that
read another page's groups check them too: the Overview's Updates, Problems and
Features figures appear only while those pages are visible, and Maintenance shows the deployment chain only with
`system_page`'s `bootc_status_group` on a host that booted an image.

**Help, About and Keyboard shortcuts** are dialogs of the window
(`frostui.Info`). `Main.showHelp` (F1, the main menu, the palette) lists
`pageview.HelpResources(website, issues, chat)` of `help_page`'s
`help_resources_group` as links, or says that no help links are configured
when the group is off or names no URL. `Main.showAbout` shows
`frostui.AboutBody` with the version, the website, the issue tracker, the
copyright (`LICENSE`'s line) and the license (MIT,
[ADR-0018](../adr/0018-relicense-mit-and-drop-inherited-translations.md)).
`Main.showShortcuts` lists `navigation.Shortcuts` of the visible pages
through `frostui.ShortcutsBody`.

## Key Patterns

### The UI thread, `Env.Post` and `Go`

Views and every state struct in `internal/views` and `internal/window` belong
to the UI thread, where MyGo runs every `View`. No external tool call runs
there. A loader copies what it needs, runs the backend on a goroutine that
reads no state, and hands the result back with `Env.Post` (MyGo's
`Window.Update`, through the poster). The usual shape is `views.Go`:

```go
gen := s.refreshGate.Begin() // on the UI thread
Go(s.env, func() deployments {
    // a goroutine: backend calls only, no state
    return s.readDeployments(ctx, wantBootc, wantNative)
}, func(d deployments) {
    // the UI thread again
    if !s.refreshGate.IsCurrent(gen) {
        return
    }
    s.applyDeployments(d)
})
```

A loader that publishes in several steps calls `Env.Post` itself
(`Firmware.load` posts the fwupdmgr probe, then the list). Reloads take an
`actionstate.RefreshGate` generation on the UI thread and drop a stale
result, so a slow load cannot replace a newer one: `System.loadDeployments`,
`Flatpak.loadApps` and `Flatpak.loadUpdates`, Homebrew's inventory,
outdated, taps and search loads, `Firmware.load`, `Features.load` and
`Features.recheckFeatureUpdates`, and `Problems.loadProblems`. The
exceptions to "UI thread only" are the kit's `Console.Add`/`Addf`/`Writer`
and a job's `Progress.Set`/`Log`, which any goroutine may call, and the
Problems page's consent under its mutex, which the send goroutine reads
before every request. Because `Window.Update` rebuilds the view on every
call, console output wakes the window through `frostui.Waker` (at most every
60 ms).

### The serial job queue (`frostui.Jobs`)

Every change a page starts — stage, enable, update, install, uninstall, pin,
trust, bundle, export, flash, clean — is a `frostui.Job{Source, Key, Title,
Run, Done}` on `Env.Jobs` (`Env.Run` builds and queues one). The queue runs
one job at a time, in order: `Run` on a goroutine with a cancelable context
and a `*frostui.Progress` (`Set` for the fraction and status line, `Log` for
a console line under the job's source), then `Done` with the error on the UI
thread. The queue logs `» <title>`, `✓ <title>`, `Canceled: <title>` or
`<title> failed: <err>` to the console, turns a panic in `Run` into the
job's error, keeps the last 200 ended jobs, and counts the batch since it was
last idle (`Batch`, `BatchFraction`: "Job 3 of 10" in the rail's job card and
the status bar). A job's `Source` is its domain (the `SourceID`,
`maintenance`) and its `Key` the item, so `Jobs.Find` and `Jobs.Pending` tell
any row — on the Updates page, in the Apps table, in the palette — that its
job waits or runs, and the domains check `Pending` before they queue, so an
item never has two jobs. A job dropped before it ran gets `Done` with
`context.Canceled` and no result, which each domain treats as "nothing
happened" (`stageResult.ran`, a nil `firmwareOutcome`, the features update's
`ran`). The rail's job card and the palette's "Stop the running jobs" call
`Jobs.CancelAll`: it drops the queued jobs and cancels the running one,
unless that job is `Unstoppable`.

An `Unstoppable` job's work goes on whatever happens to ChairLift: the OS
stage, the updex helper and a `sudo: true` maintenance script run as root
through `pkexec`, and fwupd's daemon does a firmware flash. Such a job has no
deadline, finishes first and keeps the queue busy until it ends. For the same
reason the main window refuses to close while one runs. `attachMain`
(`internal/app/app.go`, called by `openMain`) registers `OnClose` and calls
`PreventDefault` while `Main.CanClose()` is false, and the next frame shows a
toast naming the job (`ID: "close-refused"`, so closing again replaces it
rather than stacking). MyGo fires `OnClose` for the window's close button and
for each window `App.Quit` closes, which the main menu's Quit, Ctrl Q, SIGINT
and SIGTERM all reach, so one guard covers them; a second signal still exits
at once, in MyGo's own handler. The window does not close by itself when the
job ends. A stoppable job, or an `Unstoppable` one that only waits in the
queue, does not hold the window.

Confirmations come first, through `Env.Dialogs.Ask(frostui.Confirm{Title,
Body, Details, Note, Action, Destructive, OnConfirm, OnCancel})`, shown one
at a time by the shell; a privileged step says so in `Note` ("Needs
administrator authentication."). Only problem dismissals and report sends
stay off the queue: ADR-0015 gives them their own lock and pacing. Reads run
concurrently, outside the queue.

"Update everything" queues each available source's default `Jobs()` in
source order — no `Held` item and no `Confirm` item, so firmware never runs
in a batch — and does nothing (but say so in a toast) while jobs run. The
Overview hero and its palette command call `Store.UpdateEverything`; the
Updates page's button and its palette command call
`updatesPage.updateEverything`, which does the same and keeps the batch for
its pipeline.

### Tool availability on a goroutine

The first load of each domain checks its tool on a goroutine, because even a
cached check may run the tool the first time: `homebrew.IsInstalledCached`
(`Brew.Start`), `flatpak.IsInstalledCached` (`Flatpak.loadApps`/`loadUpdates`),
`fwupd.IsInstalledCached` (`Firmware.load`), `updex.IsInstalledCached`
(`Features.load`), and `Maintenance.Start` for the Homebrew and Flatpak
cleanups. Until the answer arrives, a card shows that it is loading or a
cleanup row that it is being checked; a missing tool then makes the source
unavailable (`SourceStatus.Available` false, so the Updates page leaves it
out), the Apps table leaves that kind out (`AppsStatus.Available`), the
Features page says that the feature manager is not available, and
Maintenance leaves the cleanup out.
Nothing loads for a disabled group.

### bootc boot gate

The bootc parts of the window — the Overview hero's deployments with `system_page`'s `bootc_status_group`, and the system image source with `updates_page`'s `bootc_updates_group` — are gated on `bootc.IsBootcBootedCached()` (`System.readDeployments`), which runs `bootc status --format json` once (via `sync.Once`) and reports true only when the parsed `status.booted` field is non-null. This is deliberately not a sentinel-file check: `/run/ostree-booted` is absent on snow's composefs-based deployments, so relying on it would hide the groups on every snow bootc host. `bootc status` itself exits 0 with a null `booted` entry on non-bootc hosts, so the gate must inspect the JSON body rather than the exit code.

### Native A/B gate

The system image source with `updates_page`'s `sysupdate_updates_group` is gated on `sysupdate.IsNativeABCached()` (an `os.Stat` of `/usr/lib/snosi/native-ab`, cached via `sync.Once`) plus `sysupdate.StageScriptAvailable()` (`/usr/libexec/snosi-sysupdate-stage` exists). Unlike the bootc case, a marker-file check is correct here: the marker is snosi's own published contract for "this host updates via systemd-sysupdate" — every snosi unit and script gates on it (`ConditionPathExists=/usr/lib/snosi/native-ab`) — whereas `/run/ostree-booted` was a foreign sentinel that snow's bootc deployments never wrote. The `bootc` binary is absent on native A/B images and `os-release`'s `IMAGE_ID` is identical across both variants, so neither is usable for this gate. The two OS-update gates are mutually exclusive at runtime: `System.readDeployments` reads bootc's status on a bootc host and the native A/B state on a native A/B host, and the one system image source takes the kind it found (`SystemInfo.Kind`).

### Dry-run mode

Decision record: [ADR-0009](../adr/0009-dry-run-output-convention-and-single-decision-structs.md)
(the `[DRY-RUN]` output convention and single decision structs).

The `--dry-run` / `-d` flag is propagated to the wrapper packages via `SetDryRun(true)`, set once at startup by `app.SetDryRun` (called from `app.Run`) for homebrew, flatpak, fwupd, bootc, sysupdate, updex and `internal/firstrun` (the first run's color-scheme write and completion markers). The views read `Env.DryRun` for what has no wrapper of its own: the configured maintenance scripts, the Features toggles and update, the Problems page's Dismiss and report sends, and the activity record. The status bar shows "Dry run: nothing changes".

**The general rule, applied uniformly:** every state-changing action branches on the relevant dry-run flag to show an explicit preview toast instead of a completed/saved/installed message. Anywhere that same action would _also_ change a row, a source's items or a switch on success, that decision is pulled out of the view and expressed as a small struct or an `actionstate.Decision` — `ScriptDecision.Execute`, `BundleInstallDecision.Complete`, `TapTrustDecision.MutateUI`, `FeatureToggleDecision.Confirm`, `FeatureUpdateDecision.Persistent`, `FirmwareInstallDecision.PublishList`, `ProblemDismissDecision.Hide`, `PackageInstall`, `PackageUninstall`, or `PackagePin`. The domain reads the flag once, builds the decision, and branches solely on it for both the change _and_ the toast, so a table-driven test proves the change and the toast cannot drift apart (see [package-managers.md](./package-managers.md#view-layer-toast-and-decision-helpers-internalviewsactionmsg-internalviewstrustmsg) for the full function/type list). `FeatureUpdateDecision.Persistent` is the same pattern for the features update's toast: it decides whether the toast stays on screen as an error toast, and the same struct's `Recheck` decides whether the update check runs again, so the rows show the state after the update instead of the startup check's state. Sites with no second decision to gate (package upgrade/update/self-update, Flatpak uninstall, cleanup, Brewfile export, and image stage toasts) get a plain string function instead. A dry run records no activity (`Env.Record`).

**Intentional exception:** the system image stage's completion **toast** is dry-run-aware (`actionmsg.BootcStage`, or `actionmsg.SysupdateStage` on a native A/B host), but its **status** deliberately is not. `System.stageJob` re-reads the deployments after the run in both modes — a plain read of what is staged or booted right now, not a completion claim — so the source's note and the hero stay accurate. Only the toast, which answers "what did this click just do," needs dry-run wording, which is why `BootcStage` and `SysupdateStage` are strings rather than decision structs.

Per-wrapper mechanics:

- **Homebrew/Flatpak**: state-changing commands are skipped entirely at the wrapper layer (they return mock/empty results and report `cmdlog.DryRun`, so the console shows `would run: …`); ordinary package-action toasts use the plain `actionmsg` string functions (`Install`, `Uninstall`, `Pin`, `Upgrade`, `Update`, `SelfUpdate`, `BundleDump`, `Cleanup`). Homebrew search installs and installed-package uninstall/pin actions pair that text with `actionstate` decisions: live success completes the old controls and refreshes the installed inventory, while failure or dry-run restores the controls. Brew bundle installation uses `BundleInstallDecision` with the same live-complete/dry-run-reset distinction.
- **Updex**: `EnableFeature`/`DisableFeature`/`UpdateFeatures` skip their `pkexec` call entirely under dry-run (reporting `cmdlog.DryRun`) and return empty/nil results (`UpdateFeatures` returns nil results and a nil error, and `actionmsg.FeatureUpdate` shows the preview toast); the helper binary itself (`cmd/chairlift-updex-helper`, dispatch logic in `internal/updexhelper`) also honors `--dry-run` for `update`, matching `enable-feature`/`disable-feature`, as defense-in-depth even though it's unreachable from the wrapper today.
- **bootc**: `StageUpdate` short-circuits before invoking pkexec: it logs the would-be command and `stageexec.DryRun` reports `cmdlog.DryRun` for the script, emits a synthetic `EventMessage` + `EventComplete` pair on the progress channel, and returns — the stage script is never actually run (see the exception above for the toast/status split).
- **sysupdate**: `StageUpdate` has the same shape as bootc's — under dry-run it logs the would-be `pkexec /usr/libexec/snosi-sysupdate-stage` command, emits the synthetic `EventMessage` + `EventComplete` pair, and never constructs an `exec.Cmd`. Status reads (`GetStatus`, `RollbackVersion`) are not dry-run-gated: they are unprivileged, side-effect-free reads of real state.
- **fwupd**: `Install` logs `[DRY-RUN] Would execute: fwupdmgr update --no-reboot-check <id>` and runs nothing, and `InstallAndVerify` returns an empty report (the console shows the line through the log tee, as a dry-run line).
- **Homebrew tap trust**: `Brew.runTrust` (`internal/views/brew_source.go`) computes `decision := actionmsg.TapTrust(dryRun, tap.Name)` once, after a successful `homebrew.TrustPackages` call, and gates removing the tap's banner and refreshing the outdated packages on `decision.MutateUI`.
- **Custom maintenance scripts**: `Maintenance.job` (`internal/views/maintenance.go`) calls `actionmsg.MaintenanceScript(env.DryRun, title)` once, on the UI thread before the job is queued, to get a `ScriptDecision{Execute, Toast}`: when `Execute` is false no `exec.Cmd` is ever constructed (no `pkexec`, no direct script exec) — only a `[DRY-RUN] Would execute: ...` log line and a `cmdlog.DryRun` console line — and the row reads as previewed.
- **Features toggles**: `Features.toggled` (`internal/views/features.go`) computes `decision := actionmsg.FeatureToggle(env.DryRun, on, name)` once, after a successful `updex.EnableFeature`/`DisableFeature` call, and branches solely on `decision.Confirm` to decide whether the row takes the new state, records the activity and reloads, or keeps the listing unchanged.
- **First run**: the first-run window reads `firstrun.IsDryRun()` once when the Progress step starts and passes it to `firstrun.CompleteAt`, whose `firstrun.Completion` decision (`WriteMarkers`, `Log`, `DoneDescription`) decides both whether the markers are written and the Done step's text (`[DRY-RUN] Preview: first run finished — no changes made`). `firstrun.ApplyColorScheme` logs `[DRY-RUN] would execute: gsettings ...`, and the Flatpak wrapper skips `remote-add` and `install`.
- **Firmware install**: `Firmware.installJob` (`internal/views/firmware.go`) calls `fwupd.InstallAndVerify`, then builds `decision := actionmsg.FirmwareInstall(dryRun, err, report, …)` once. `decision.PublishList` decides whether the verified device list is published, `decision.RestoreControl` whether the device offers its install again, and `decision.IsError` whether the toast stays on screen. `fwupdmgr` can exit 0 without flashing a device, so the job never reads the exit status as success on its own.
- **Problems page Dismiss**: `Problems.dismiss` (`internal/views/problems.go`) computes `decision := actionmsg.ProblemDismiss(env.DryRun, item.ID, title)` once. Under dry-run `decision.Persist` and `decision.Hide` are false: nothing is written to the dismiss store, the card stays, the toast is `[DRY-RUN] Preview: dismiss <title> — no changes made`, and the log line is `[DRY-RUN] would dismiss problem <id>`. Live, the page writes the dismissal and removes the card only after the write succeeds; a failed write keeps the card and shows `actionmsg.ProblemDismissFailed`. Recording a journal problem record (`internal/selfreport`) is logging, not a system change, so it also runs under dry-run.

### Configuration-driven UI visibility

Every card checks its own group with `Env.Enabled(page, group)` (`config.IsGroupEnabled`) before it draws, and every domain checks the groups it serves before it loads: a disabled group's card is never drawn and its data never read. Groups default to enabled if not specified in config. `maintenance_cleanup_group` defaults to disabled in the default config. Code that runs after an async action must not assume another group's state exists: the Homebrew groups are independent of each other (`brew_bundles_group` works without `brew_group`, `brew_trust_group` without `brew_updates_group`), and `Brew.loadOutdated`, reachable from a tap's trust and from the Apps actions, checks its own group first.

`internal/config/config.go` builds the effective `*Config` by overlaying a
parsed file onto `defaultConfig()` field by field, not by replacing it
wholesale — `mergeConfig` walks each page, `mergePage` walks each group within
a page, and `mergeGroup` walks each field within a group. The overlay is
driven by `rawConfig`/`rawPageConfig`/`rawGroupConfig`, a pointer-typed mirror
of `Config`/`PageConfig`/`GroupConfig` used only for YAML decoding: a `nil`
pointer means the file omitted (or explicitly nulled) that key, so
`defaultConfig()`'s value survives; a non-nil pointer means the file set that
key — including to an empty string or empty slice — so it replaces the
default outright. This is why `maintenance_cleanup_group` stays disabled
overall (and keeps its default `actions` entry) when a config file mentions
the group only to flip an unrelated field. When every search candidate is
absent, `Load()` returns `defaultConfig()`. An authoritative file that cannot
be read or validated instead returns `disabledConfig()`: the same defaults
for non-visibility fields, with every canonical group's `Enabled` field
forced to false.

**Structured load-error vocabulary (`internal/config/loaderror.go`).** A
stable `ErrorKind` enumerates why loading/validating a config file could
fail: `KindRead` ("read") for a filesystem/read failure (e.g. permission
denied opening the file, or a dangling symlink — distinct from "file does
not exist", which `Load()` treats as absent-and-fall-back, not an error); `KindParseType`
("parse/type") for a YAML syntax error, a value that decodes to the wrong
Go type, or a malformed source graph detected by pure shape/graph
inspection after the YAML parsed successfully (an unsupported node shape,
an alias with no target, an alias cycle, etc.) — like `KindSchema` below,
that shape-inspection case may legitimately carry a nil `Err`, since there
is no underlying parser error to wrap; and `KindSchema` ("schema") for a
validator-detected shape failure
found only after the document parsed successfully — e.g. an unknown key or a
value of the right YAML kind but the wrong shape — which may legitimately
carry a nil `Err` since shape inspection alone can detect the problem with
no underlying cause to wrap. `LoadError.Error()` renders `Path`, the exact
`Kind` string, `Detail`, and the cause (in that order, each only when
non-empty/non-nil), so the three `Kind` literals above always appear
verbatim in the message. `LoadError.Unwrap()` returns `Err` unchanged
(nil when there is none), which is what lets `errors.Is`/`errors.As` see
through a `*LoadError` to a wrapped sentinel or recover the original value
from a `fmt.Errorf("%w", ...)` wrapper.

**Single-document YAML parsing (`internal/config/source.go`).** The
unexported `parseYAMLDocument(path string, data []byte) (*yaml.Node,
*LoadError)` is the first thing in this package that actually constructs a
`LoadError`. It decodes `data` with `yaml.NewDecoder` and accepts exactly one
YAML document, with a fixed cardinality outcome per input shape:

- Empty input, and whitespace-only input, are both the valid empty result:
  `(nil, nil)` — no document, no error. This matches `Load()`'s existing
  "absent config" fallback semantics elsewhere in the package.
- A single well-formed document returns its `*yaml.Node` (`Kind ==
yaml.DocumentNode`, one child under `Content`) and a nil `*LoadError`.
- A malformed first document returns `(nil, err)` with `err.Kind ==
KindParseType`, `err.Path` copied verbatim, `err.Err` set to yaml.v3's own
  parser error, and `err.Detail` set to that same error's message — which
  yaml.v3 always renders as `"yaml: line N: ..."`, so the reported line is
  visible in `Detail` without a second look at `Err`.
- A trailing bare `---` is a second document, not a harmless end-of-stream
  marker: yaml.v3 decodes it as a second, null document, and
  `parseYAMLDocument` rejects it exactly like any other second document
  (`KindParseType`, `Detail` naming its line) rather than accepting the
  input as a single document.
- Any other second document — well-formed or malformed — is rejected the
  same way: a well-formed second document has no parser error to wrap, so
  `Err` is left nil and `Detail` names that document's own starting line
  instead; a malformed second document preserves _that_ document's parser
  error and line in `Err`/`Detail`, just like a malformed first document
  would.

These capabilities are now the runtime loading path:
`Load`/`loadFromPath` → `loadResolvedPath` → `parseAndValidate` →
`parseYAMLDocument`/`resolveEffective`/`validateSourceGraph`. Read failures
produce `KindRead`; malformed or wrong-type YAML produces `KindParseType`;
unknown schema names and invalid shapes produce `KindSchema`. No runtime
load uses the old permissive `yaml.Unmarshal` path.

**Exact merge-key recognition and tag normalization
(`internal/config/sourcegraph.go`).** `isMergeKey(n *yaml.Node) bool` and
`shortYAMLTag(tag string) string` are deliberate behaviorally exact reproductions
of two unexported predicates from `gopkg.in/yaml.v3` v3.0.1 itself —
`decode.go:isMerge` and `resolve.go:shortTag` — rather than reimplementations
from a description of YAML merge-key semantics, so this package's own
merge-key walk (`effectiveEntries`, described below) recognizes exactly the
same nodes yaml.v3's own decoder would treat as a merge key, node for node.
`isMergeKey` reports
true only for a `yaml.ScalarNode` whose `Value` is exactly `"<<"` and whose
`Tag` is one of: absent (`""`, the implicit/unresolved tag), the bare
non-specific tag (`"!"`), the short merge tag (`"!!merge"`), or its canonical
long form (`"tag:yaml.org,2002:merge"`) — the last two both recognized via
`shortYAMLTag`. A quoted `"<<"` (explicitly tagged `"!!str"`) is therefore an
ordinary key, not a merge key, and so is a merge-tagged scalar whose `Value`
isn't literally `"<<"`; non-scalar nodes (mapping, sequence, alias) and a nil
`*yaml.Node` are never merge keys either — `isMergeKey` is nil-safe rather
than panicking. `shortYAMLTag` rewrites a canonical `"tag:yaml.org,2002:xxx"`
tag to yaml.v3's short `"!!xxx"` form and returns any tag without that prefix
unchanged (including the empty tag, `"!"`, an already-short `"!!xxx"` tag, a
custom `"!xxx"` tag, and a long tag under a different authority such as
`"tag:example.com,2020:merge"`). These are the only two unexported helpers in
this package's source-graph slice that the spec singles out for a direct
unit test (`TestMergeKeyRecognition`, `TestShortYAMLTagNormalization` in
`sourcegraph_test.go`) rather than only exercising them through an exported
entry point, per
`docs/design/engineering-lessons.md#call-helpers-directly-in-tests`; neither
helper is called directly by runtime loading, but both are reached indirectly
through `resolveEffective` in the strict validator pipeline.

**Reachable source-graph shape validation
(`internal/config/sourcegraph.go`).** `validateSourceGraph(path string, doc
*yaml.Node) *LoadError` walks every node and content edge reachable from
`doc` — through document content, mapping keys, mapping values, sequence
entries, and alias targets, in each node's `Content` slice order — and
rejects a malformed source graph as `KindParseType` rather than panicking,
even against a synthetic `*yaml.Node` tree unreachable from real YAML text.
`doc == nil` succeeds unconditionally: it is `parseYAMLDocument`'s valid
empty-input result. Otherwise the checks are, in order:

- the root must be a `yaml.DocumentNode` with exactly one non-nil child;
- every reachable mapping must have an even number of content entries, and
  neither a key nor a value in any key/value pair may be nil;
- every reachable sequence must have no nil entries;
- every reachable scalar must have no content children;
- every reachable alias must have no content children and a non-nil
  `Alias` target; and
- every reachable node's `Kind` must be one of yaml.v3's five supported
  kinds (an all-zero `Kind` or any other unsupported value is rejected).

Node identity — the `*yaml.Node` pointer itself, not any decoded value —
drives an unseen/visiting/done state map keyed on that pointer, so a node
reached through several parents (a shared anchor aliased more than once, a
scalar reused as several mapping values) is validated exactly once, and
re-encountering a node still in the `visiting` state on the active
depth-first path is rejected as an alias cycle (covering both a self cycle,
where an anchored mapping's own value aliases back to itself, and a mutual
cycle between two anchored mappings). Because pure shape/graph rejections
have no underlying Go error to preserve, every `*LoadError` `validateSourceGraph`
returns carries a nil `Err`; `LoadError.Error()` still renders the full
`"config parse/type error: <path>: <detail>"` wording from `Path` and
`Detail` alone. A self merge (`&a {<<: *a}`) and a mutual merge cycle
between two anchored mappings are both rejected too, but as a byproduct of
the same generic alias-cycle rule above rather than merge-specific code: the
merge operand's alias is still in the `visiting` state by the time any
merge-shape check would run, so the cycle rule fires first. Every reachable
mapping's explicit keys must also be pairwise unique (see the duplicate-key
paragraph below); the memoized 64-consecutive-alias-hop bound and the
memoized 128-source-node-path-visit bound, both described below, now run
as a second pass once these checks prove the graph acyclic.
Runtime loading reaches `validateSourceGraph` through
`parseAndValidate` → `resolveEffective`.

**Merge-operand shape validation (`internal/config/sourcegraph.go`).**
Layered onto the traversal above: every mapping entry whose key
`isMergeKey` reports true additionally has its value checked as a merge
operand by `validateMergeOperand`, reproducing `gopkg.in/yaml.v3` v3.0.1
`decode.go`'s `merge`/`failWantMap` rule exactly rather than accepting any
value ordinary YAML content would allow. A merge operand is accepted only
as one of: a `yaml.MappingNode` literal; a `yaml.AliasNode` whose
_immediate_ `Alias` target is a `yaml.MappingNode`; or a `yaml.SequenceNode`
each of whose entries is itself one of the two prior shapes. Everything
else is rejected as `KindParseType` — a scalar operand, an alias to a
sequence or a scalar, a sequence containing a scalar or a nested sequence,
and a sequence containing an alias to a non-mapping all fail
(`isMergeOperandMapping` is the single-alias-hop predicate both the direct
and sequence-entry checks share). The immediate-target rule produces a
deliberate asymmetry: an alias-to-alias-to-mapping chain is perfectly valid
as an _ordinary_ mapping value (the generic traversal above only requires a
non-nil `Alias` target of any kind) but is rejected as a merge operand,
because yaml.v3 itself only unwraps one alias hop before deciding a merge
value is not a mapping. All four of `isMergeKey`'s recognized tag forms —
implicit, `"!"`, `"!!merge"`, and the canonical long tag — trigger this
check identically; a quoted `"<<"` key (tagged `"!!str"`) does not, so it
may hold any otherwise-valid value, including a bare scalar. Merge operands
are validated wherever the traversal reaches them — inside a complex
mapping key's own subtree (complex keys are traversed like any other node
but are not otherwise classified in this slice), and in a mapping entry a
later, still-unimplemented merge-precedence pass would go on to discard in
favor of a later `<<` entry — because this function proves every reachable
node well-formed, not just the nodes an eventual effective-merge result
would keep
(`docs/design/engineering-lessons.md#validate-discarded-merge-branches`).

**Duplicate explicit-key detection (`internal/config/sourcegraph.go`).**
Every reachable mapping's explicit keys must be pairwise unique under
`sourceKeyID{Kind, Value}` identity — a repeated key rejects the whole
graph as `KindParseType`, naming the duplicated key's value and, when the
parser recorded a positive line for it, that line. The identity
deliberately compares only `Kind` and `Value`, reproducing `gopkg.in/yaml.v3`
v3.0.1 `decode.go`'s own `uniqueKeys` predicate (inside `decoder.mapping`)
exactly rather than the tag-aware key identity
`docs/design/engineering-lessons.md#yaml-key-identity-uses-the-tag`
requires elsewhere in this package for merge-precedence purposes — that rule
is about a different concern (`effectiveKeyIdentity`, described just below)
and does not apply here, because yaml.v3's own duplicate-key guard never
looks at `Tag` either. Two surprising consequences follow directly: two
`<<` merge-key entries in the same mapping collide as duplicates regardless
of which of `isMergeKey`'s four accepted tag forms each carries (an
implicit `<<` and one explicitly tagged `!!merge` are still "the same key"),
and a bare `1` (yaml.v3 resolves it to `!!int`) collides with an explicitly
quoted `"1"` (`!!str`) even though their `Tag`s differ, because neither
`Kind` nor `Value` distinguishes them. Detection is per-mapping, not
global — the same key value recurring in a sibling mapping, or in a nested
mapping reachable through it, is never a duplicate — and it runs on every
reachable mapping regardless of how it is reached: directly, as a merge
operand's value, through an alias target, or inside a complex mapping key's
own subtree. The implementation is one `map[sourceKeyID]struct{}` per
mapping, sized from `len(n.Content)/2`, filled by a single linear pass over
key indices — deliberately not yaml.v3's own nested-loop `uniqueKeys`
comparison, which is pairwise (`O(k^2)` per mapping). This package's version
is `O(k)` per mapping and `O(V+E)` over the whole graph, so a mapping with
tens of thousands of keys does not make the validator's own running time
blow up the way the pairwise loop would.

**Effective (merge-precedence) key identity
(`internal/config/effectivekeys.go`).** A separate, standalone helper,
`effectiveKeyIdentity`, computes a comparable `effectiveKeyID` for a mapping
key node — the identity the merge-precedence resolver `effectiveEntries`
(`internal/config/effectivemerge.go`, described below) uses to decide
whether two keys from different merge sources are "the same key" and must
therefore resolve to one effective value rather than two. It is
deliberately a different type and a different comparison from
`sourceKeyID` above: `effectiveKeyIdentity` first follows `n`'s
`yaml.AliasNode` chain (any number of hops, guarded by a local
node-pointer seen-set so a synthetic self-referential alias handed to it
directly terminates instead of hanging) to its non-alias target, then
branches on that target's `Kind`. A `yaml.ScalarNode` target yields `{kind:
yaml.ScalarNode, tag: target.ShortTag(), value: target.Value}` —
`ShortTag()` is used rather than the raw `Tag` field because
`gopkg.in/yaml.v3` v3.0.1's `yaml.go` implements it to resolve an unset or
`"!"` tag from the node's own value (`resolve("", n.Value)` for scalars),
so this rule is exactly the tag-aware identity
`docs/design/engineering-lessons.md#yaml-key-identity-uses-the-tag`
requires: a bare `1` and an explicitly `!!int`-tagged `1` are the same
key, but a bare `1` and an explicitly `!!str`-tagged (quoted) `"1"` are not,
and `01` and `1` are not (different `Value`). A `yaml.MappingNode` or
`yaml.SequenceNode` target instead yields `{complex: target}` — pointer
identity on the target node alone, with no structural expansion of the
complex key's contents: two distinct nodes that look alike are different
keys, two aliases sharing one complex target are the same key, and a
complex identity is never equal to any scalar identity (the zero
`*yaml.Node` complex field only appears alongside the zero scalar fields
for a nil or dead-ended-alias input, and a real complex node's pointer is
never nil). This is the tag-blind `sourceKeyID` above's direct
counterpart, not a reuse of it: `sourceKeyID` intentionally omits `Tag` to
reproduce yaml.v3's own duplicate-key guard exactly, while
`effectiveKeyIdentity` intentionally includes it because merge-precedence
resolution must not let a `!!str "1"` entry silently discard or be
discarded by an `!!int 1` entry from another merge source. A complex key's
identity is used only to decide _precedence_ — which candidate wins, and
whether two explicit complex keys collide (see below) — never to compute
it: a _winning_ complex key is still fully resolved and emitted
structurally, alias-free, by `emitEffectiveNode` like any other node
(charged against `maxEffectiveOutputNodes` like everything else it
emits), while a _losing_ complex key (impossible for two candidates to
tie on, since distinct complex-key nodes always have distinct pointers,
but reachable when the complex key belongs to a losing merge candidate's
value) is never dereferenced, resolved, or emitted at all.

**Validate-first, alias-resolving effective emitter
(`internal/config/effective.go`).** `resolveEffective(path string, doc
*yaml.Node) (*yaml.Node, *LoadError)` is the slice's entry point toward an
"effective" (post-merge) configuration tree, and imports only
`gopkg.in/yaml.v3` plus stdlib. It always calls `validateSourceGraph(path,
doc)` first and returns that error unchanged on failure — Detail and Path
byte-identical to what `validateSourceGraph` itself would return for the
same input — so a caller cannot use `resolveEffective` to bypass
source-graph validation, and `doc == nil` returns `(nil, nil)` only _after_
that successful validation, matching `parseYAMLDocument`'s valid
empty-input result.

On success `resolveEffective` emits a fresh copy of `doc` via the
unexported `emitEffectiveNode`: the result is rooted at a `yaml.DocumentNode`
copy with exactly one resolved child, and every emitted node — document,
mappings, sequences, keys, and values — copies exactly `Kind`, `Style`,
`Tag`, `Value`, `Line`, `Column`, `HeadComment`, `LineComment`, and
`FootComment` from the source node it corresponds to, via the unexported
`emitNodeMetadata`, leaving `Anchor` as `""` and `Alias` as `nil` on every
copy and allocating a distinct `*yaml.Node` each time (mutating the result
never mutates `doc`). A `yaml.AliasNode` is never copied as itself: the
unexported `dereferenceAliasTarget` follows its (possibly multi-hop) `Alias`
chain to the non-alias target first, and the emitted node carries that
_target's_ metadata, not the alias node's own — this applies identically
whether the alias appears in mapping value position or as a mapping key.
Because the output must stay alias-free, a single source node reached
through more than one alias becomes more than one independent fresh copy in
the result, and each such copy is charged separately against the
`maxEffectiveOutputNodes` budget described just below.

For a mapping node specifically, `emitEffectiveNode` does not copy the
target's raw `Content` — it emits `effectiveEntries(target)`'s winning
entries instead (`internal/config/effectivemerge.go`, described in the next
section), so only a winning key and its winning value are ever resolved and
charged against the budget; a losing merge candidate's value is never
cloned. A recognized `<<` merge key is therefore never itself emitted as a
key, and the result contains no recognized merge directive and no
`yaml.AliasNode` anywhere, at any depth — including inside a retained
mapping key's own subtree or a retained value's own nested merge.

`resolveEffective`'s emit path is bounded by the unexported `const
maxEffectiveOutputNodes = 100000`: every retained document, mapping,
sequence, key and value node counts exactly once toward it, mirroring
`sourcebounds.go`'s `pathVisitCount` accounting rule that "key", "value" and
"alias target" only name which child is traversed next and add no separate
charge, and each independent copy an expanded alias produces is charged
separately (an alias node itself is never charged, since it is dereferenced
to its target before the budget check runs). The check happens in
`emitEffectiveNode`'s first statements for each node — incrementing and
comparing a per-call counter _before_ allocating a `*yaml.Node` for that
node or recursing into its `Content` — so the 100,001st attempted emission
fails before its own allocation and before any of its descendants are ever
visited, which is what makes an exponential alias expansion (a compact,
small source graph whose alias-free output would otherwise need on the
order of `2^n` nodes) abort at the boundary rather than run to completion.
Once the budget is exceeded on any call, every other pending call in the
same `resolveEffective` invocation also returns immediately without further
allocation or recursion. Exactly `maxEffectiveOutputNodes` emitted nodes
succeeds; `maxEffectiveOutputNodes`+1 fails. On overflow `resolveEffective`
returns a nil tree and a `KindParseType` `*LoadError` with `Path` copied
from the `path` argument, a nil `Err`, and a `Detail` naming the
100,000-node bound and a source line for the node whose emission would have
crossed it. That line comes from reusing `sourcebounds.go`'s existing
`collectSourceInventory` and `attributeSourceLines` pair on the validated
source document — the same all-paths line attribution
(`checkSourceGraphBounds` uses it too) rather than a second, separate line-
attribution implementation — computed once when constructing the resolver
error: a node's own line when positive, otherwise its nearest positive-line
ancestor over all root-reachable paths, otherwise `1` for a wholly synthetic
graph with no line metadata anywhere, so the reported line is always positive.

**Merge precedence, memoized candidate inventory, merge-free output
(`internal/config/effectivemerge.go`).** `effectiveEntries(m *yaml.Node)
[]effectiveEntry` computes a mapping node's complete, ordered, deduplicated
inventory of winning entries — the real, yaml.v3-compatible merge-precedence
result `emitEffectiveNode`'s mapping branch emits from, replacing the
earlier chunk's interim "retain the `<<` key as an ordinary entry" behavior.
An `effectiveEntry` is an `{id effectiveKeyID, key, value *yaml.Node}`
triple: `id` is the candidate's `effectiveKeyIdentity` (`effectivekeys.go`),
used to decide whether two candidates from different sources are "the same
key"; `key` and `value` are the source nodes for that candidate, not yet
copied.

The inventory is built in two passes, then deduplicated once: first, every
retained explicit entry, in `m`'s source `Content` order, skipping any
recognized merge directive (`isMergeKey`); second, every inherited
candidate, discovered by merge directives in source `Content` order, each
directive's sequence operands left to right (`mergeOperandMappings`
unwraps a direct mapping, or an alias's _immediate_ target, matching
`validateMergeOperand`'s already-accepted shapes), and each operand
mapping's own `effectiveEntries` computed recursively — so a merge nested
inside a merge operand is fully flattened before its candidates are
considered here. The combined candidate list is then deduplicated by
`effectiveKeyIdentity`, keeping the _first_ candidate for each identity.
Because explicit entries are always listed first, this yields
explicit-over-merged precedence; because merge candidates are discovered in
directive-then-sequence-then-recursive order, it yields
earlier-sequence-operand-over-later precedence; and because the explicit
pass runs unconditionally before the merge pass regardless of where the
`<<` directive appears in `m`'s `Content`, an explicit key suppresses a
matching inherited candidate whether the directive comes before or after
that explicit key in the source. This matches `gopkg.in/yaml.v3` v3.0.1
`decode.go`'s `decoder.mapping`/`decoder.merge` behavior exactly:
`mapping` decodes explicit entries first and skips `isMerge` keys, then
`merge` seeds its `mergedFields` set from every explicit parent key before
consuming operands in `Content` order and marking each merged key as
consumed — with nested merges inheriting that accumulated set rather than
resetting it, which is why `effectiveEntries`' own recursive calls flatten
rather than re-seed.

The emitted mapping's key order is therefore fully deterministic: retained
explicit entries in source `Content` order, then inherited winners in
merge-candidate discovery order (directives in `Content` order, sequence
operands first to last, each operand's own effective entry order) —
running the same fixture through `resolveEffective` twice yields identical
order both times.

`effectiveEntries` delegates to the unexported `effectiveEntriesWithMemo`,
threading an `effectiveMergeMemo` (`map[*yaml.Node][]effectiveEntry`) keyed
by mapping-node pointer identity through the recursion described above. The
memo lives on `effectiveEmitState` for one complete `resolveEffective` call:
a mapping node reached as a merge operand through more than one emitted
parent, or through more than one alias to the same anchor, has its candidate
inventory computed once and reused rather than recomputed once per parent —
`TestResolveEffectiveSharedOperandInventoriedOnce`'s 40-level doubling merge
chain (mirroring `buildSourceSharingDAG`'s construction, but merging via
`<<: [*prev, *prev]` at each level instead of plain aliasing) would need on
the order of `2^40` recursive computations without this memo and completes
in well under a bounded timeout with it.
`TestResolveEffectiveSharedOperandMemoSpansEmittedMappings` separately pins
the memo lifetime: thousands of distinct retained parent mappings merge one
wide shared operand, which would rebuild that inventory for every parent if
the memo were scoped only to a top-level `effectiveEntries` call. A losing
candidate's value is never resolved by this pass either — `effectiveEntries`
only ever returns the source nodes for winning entries, so
`emitEffectiveNode` never clones, and never charges against
`maxEffectiveOutputNodes`, anything a merge discarded.

The post-alias key-identity-collision rule is layered on top of, not a
replacement for, `checkDuplicateMappingKeys`'s tag-blind `Kind`+`Value`
duplicate-key validation above, which `validateSourceGraph` still runs
first, unchanged: `effectiveEntriesWithMemo` additionally scans each
mapping's own retained explicit entries (the same entries its first pass
collects) for two whose `effectiveKeyIdentity` compare equal _after_ alias
dereferencing — catching, for example, a literal scalar key and an alias
key targeting an anchored scalar with the same resolved tag and value,
which `checkDuplicateMappingKeys` cannot catch because the alias node's
own `Kind` and `Value` differ from its target's. Finding such a pair sets
`effectiveEmitState.collided` and records the _later_ key (in source
`Content` order) as `collisionKey`, aborting the whole `resolveEffective`
call — exactly like a node-budget overflow aborts it — without
inventorying or emitting anything further; `resolveEffective` then returns
`effectiveIdentityCollisionError(path, doc, st.collisionKey)`, a
`KindParseType` `*LoadError` with a nil `Err` and a `Detail` reusing the
same `collectSourceInventory`/`attributeSourceLines` line-attribution pair
(own line, else nearest positive-line ancestor over all paths, else `1`)
as the output-limit error above. The rule applies only to two _explicit_
keys of one source mapping: an inherited merge candidate colliding with an
explicit key of the same identity is ordinary suppression (the existing
first-candidate dedup pass), not this error, since only an explicit
key can ever be the "other side" of a genuine ambiguity about which of two
literally-written keys in the same mapping was meant.

`emitEffectiveNode`, `dereferenceAliasTarget`/`emitNodeMetadata`, the
`effectiveEmitState` counter/collision-tracking type,
`effectiveOutputLimitError`, `effectiveIdentityCollisionError`,
`effectiveEntry`, `effectiveMergeMemo`, `effectiveEntriesWithMemo`,
`explicitKeyCollision`, and `mergeOperandMappings`/`mergeOperandMapping`
are all unexported helpers reachable only from `resolveEffective`; per the
frozen `directCallAllowlist` in `sourcesurface_test.go`, no `_test.go` file
names any of them directly — `effective_test.go`, `effectivebound_test.go`,
`effectivemerge_test.go`, and `effectiveidentity_test.go` exercise them
only through `resolveEffective` itself, alongside `resolveEffective` and
`effectiveKeyIdentity`, the two additions that allowlist authorizes.
Runtime loading reaches `resolveEffective` only through
`parseAndValidate`; its lower-level helpers remain encapsulated behind that
validator entry point.

**Reachable inventory, all-paths line attribution, and the 64
consecutive-alias-hop and 128-source-node-path-visit bounds
(`internal/config/sourcebounds.go`).** Once
`walkSourceNode`'s traversal and every check above have proven a source
graph acyclic and well-formed, `validateSourceGraph`'s single call to
`checkSourceGraphBounds` runs a second, purely additive pass — it never
runs on a graph that failed an earlier check, so a cyclic or otherwise
malformed graph still surfaces its original error (an alias cycle, a
malformed merge operand, a duplicate key) rather than a bounds error.
`collectSourceInventory` walks the already-proven-acyclic graph exactly
once per unique node, in `Content` slice order, recording every reachable
node in discovery order, every parent→child content edge reachable in the
graph — including a second edge into a node reached through more than one
parent, not just the edge the walk happened to follow first — and each
node's first-encounter active nearest positive-line ancestor.

`attributeSourceLines` gives every reachable node a source line to report
in a bounds-limit error: a node's own `Line` when positive; otherwise the
line of a positive-line ancestor at the minimum number of content edges
from it over _all_ root-reachable paths, not merely the path the traversal
happened to discover it by first; otherwise `1`, the deterministic
fallback for a wholly synthetic graph with no line metadata anywhere. This
is a multi-source breadth-first search in the parent→child direction,
seeded with every reachable positive-line node at distance zero attributing
its own line and relaxing each edge `parent→child` with
`(dist(parent)+1, line(parent))`; because it walks `collectSourceInventory`'s
already-deduplicated node and edge lists, it visits each at most once and
is `O(V+E)`, so it never re-expands a shared subgraph once per path — the
same discipline that keeps a compact alias-sharing DAG linear rather than
exponential. When two positive-line ancestors tie at the same minimum
distance to a node, the node's first-encounter active ancestor wins if it
is one of the tied candidates; otherwise the candidate reached through the
parent with the lower discovery index wins. An alias edge participates in
this distance exactly like an ordinary content edge, so a node reachable
one alias hop below a positive-line ancestor can out-distance — and so
out-rank — a farther positive-line ancestor reached only through ordinary
content.

The alias-hop bound itself is a memoized dynamic program over node
identity: `hops(n) = 1 + hops(n.Alias)` for a `yaml.AliasNode`, and `0` for
every other kind, so descending from any non-alias node into ordinary
content always resets the count for that child path — which is why more
than 64 independent, shallow sibling aliases to the same anchored target
all succeed even though there are far more than 64 of them in the graph.
`aliasHopCount` memoizes by node pointer, so a node reachable through many
parents is computed once rather than once per path. `checkAliasHopLimit`
rejects the first node (in discovery order) whose `aliasHopCount` exceeds
64 as `KindParseType`, naming the 64-hop limit and reporting
`attributeSourceLines`' line for that node; exactly 64 consecutive hops
succeeds and 65 fails.

The 128-source-node-path-visit bound is a second, separate memoized
dynamic program over node identity: `pathVisitCount(n) = 1 +
max(pathVisitCount(child))` over every content edge reachable directly
from `n` — a document's, mapping's, or sequence's `Content` entries, or an
alias node's single `Alias` target — and `1` for a node with no such
children (an ordinary scalar). Every encountered node, including a
mapping's own key nodes, contributes exactly one visit; "key", "value",
and "alias target" only name which child is traversed next and add no
separate charge, so a scalar used as a mapping key counts once, not once
for being a scalar plus once for being a key, and an alias node plus its
target contribute two visits together, not one. `pathVisitCount` memoizes
by node pointer exactly like `aliasHopCount`, so wide siblings that all
share one deep target are each checked once and do not accumulate a
global count, and a compact alias-sharing DAG is evaluated in time linear
in its node and edge count rather than once per exponentially-many
root-to-leaf paths.

Because `pathVisitCount` is monotonically non-decreasing from any node
toward the root, once one node's count exceeds 128 every one of its
ancestors up to the graph's root does too; `checkSourcePathVisitLimit`
therefore does not report the first over-limit node found in discovery
order (which would almost always be the root itself, carrying no useful
attribution) but instead walks `inv.nodes` in discovery order for the
_boundary_ node — one whose own count exceeds 128 but whose every direct
child does not, i.e. the exact point along an offending path where the
count first crosses the limit — and reports `attributeSourceLines`' line
for that node, naming the 128-visit limit; this stays deterministic even
when more than one branch independently crosses the boundary, since
discovery order breaks the tie. Exactly 128 source-node visits on a
root-to-leaf path succeeds and 129 fails. When a graph violates both
bounds, `checkSourceGraphBounds` checks the alias-hop bound first, so the
alias-hop error is reported and the path-visit pass never runs. Every
error either pass returns has a nil `Err`, like every other
`validateSourceGraph` rejection.

**Canonical page/group inventory (`internal/config/schema.go`).** `SchemaPages()`
and `SchemaGroups(page)` derive the authoritative list of page names and,
per page, group names by reflection — never by a second hand-maintained
list — so the inventory cannot drift from the types it describes. Page names
come from the `yaml` struct tags on `Config`'s exported fields (via the
unexported `yamlFieldNames` helper, in struct declaration order); group names
for a page come from that page's map key set in `defaultConfig()` (via the
unexported `schemaPageGroups` helper, sorted lexicographically for a stable
API since map iteration order is not deterministic). `rawConfig` is _not_ a
schema authority here — it is `Config`'s pointer-typed YAML-decoding mirror
(see above), and a dedicated reflection test
(`TestRawConfigMatchesConfigFields`) proves its exported fields, field order,
and yaml tag names match `Config`'s exactly, so it stays a provably-in-sync
mirror rather than a second source of truth per
`docs/design/engineering-lessons.md#derive-rules-from-the-canonical-struct`.
Both exported functions return a freshly allocated slice (and `SchemaGroups`
an error for a page name outside `SchemaPages()`) on every call, so a caller
mutating a returned slice cannot affect a later call. An empty or duplicate
page/group name is reported as a returned `error`, never a panic, `log.Fatal`,
or `os.Exit`, so failures stay deterministic and assertable from tests.

`SchemaGroupFields()` and `SchemaActionFields()` extend the same inventory to
field names, reusing `yamlFieldNames` — no second tag-parsing rule.
`SchemaGroupFields()` reads `rawGroupConfig`'s yaml tags, in struct
declaration order, because `rawGroupConfig` (not `GroupConfig`) is what
yaml.v3 actually decodes a group into; `GroupConfig` remains the semantic
authority, and a dedicated reflection test
(`TestRawGroupConfigMatchesGroupConfigFields`) holds the two to each other —
same field count, same names and yaml tags per index, and each
`rawGroupConfig` field type equal to `GroupConfig`'s or exactly a pointer to
it (ignoring only that pointer-vs-value difference and `omitempty`) — so
`rawGroupConfig` cannot silently drift from `GroupConfig` per
`docs/design/engineering-lessons.md#derive-rules-from-the-canonical-struct`.
`SchemaActionFields()` reads `ActionConfig`'s yaml tags directly, since
`ActionConfig` has no raw/pointer mirror. Both return a freshly allocated
slice on every call and report an empty or duplicate field name as an error,
never a panic, `log.Fatal`, or `os.Exit`. `parseAndValidate` consumes this
inventory in production, so unknown pages, groups, and fields are rejected by
the runtime loader rather than silently ignored.

**The four-stage validator entry point (`internal/config/validate.go`).**
`parseAndValidate(path string, data []byte) (*rawConfig, *LoadError)` is the
unexported entry point that ties together every validation capability
described above into a single call: stage 1 is `parseYAMLDocument`
(source.go); stage 2 is `resolveEffective` (effective.go), which itself runs
`validateSourceGraph` (sourcegraph.go, including its `checkSourceGraphBounds`
bounds pass, sourcebounds.go) before emitting an alias-free, merge-resolved
effective document; stage 3 is this file's own document-level shape check;
and stage 4 decodes an accepted mapping document into a `*rawConfig`. A
stage 1 or stage 2 `*LoadError` is returned unchanged — `parseAndValidate`
can never bypass parser, source-graph, or bounds validation by continuing
past their errors.

The full pipeline, in order, is: `parseAndValidate` calls `parseYAMLDocument`
(stage 1) to parse `data` into a single `*yaml.Node` document, rejecting a
malformed document or a second document; then `resolveEffective` (stage 2),
which validates the reachable source-branch graph via `validateSourceGraph`
(including its node/path/alias-hop bounds) before resolving `<<` merges and
aliases into one alias-free effective tree; then this file's own ChairLift
schema/shape/declared-type validation (stage 3) walks that effective tree
page by page, group by group, group-field by group-field, and (for
`actions`) action-entry by action-field; and finally, only once every prior
stage and every entry at every level has passed, stage 4 decodes the
already-validated effective document into a `*rawConfig` via
`yaml.Node.Decode`. A failure at any earlier stage returns immediately
without running any later stage.

Every reachable outcome of that pipeline classifies to exactly one of three
results — `KindParseType`, `KindSchema`, or a valid decoded/no-op result —
per this decision table:

| Effective input                                                                   | Result                     |
| --------------------------------------------------------------------------------- | -------------------------- |
| malformed YAML, second document, or source duplicate                              | `KindParseType`            |
| cyclic, malformed, source-path-over-budget, or effective-output-over-budget graph | `KindParseType`            |
| empty document or top-level null                                                  | valid no-op overlay        |
| top-level scalar or sequence                                                      | `KindParseType`            |
| unknown top-level page                                                            | `KindSchema`               |
| known page with null                                                              | valid no-op for that page  |
| known page with scalar or sequence                                                | `KindParseType`            |
| unknown group under a known page                                                  | `KindSchema`               |
| known group with null                                                             | valid no-op for that group |
| known group with scalar or sequence                                               | `KindParseType`            |
| unknown group field                                                               | `KindSchema`               |
| `actions` value that is not null or a sequence                                    | `KindParseType`            |
| action entry that is null or otherwise not a mapping                              | `KindParseType`            |
| unknown action field                                                              | `KindSchema`               |
| known field that cannot decode into its declared Go type                          | `KindParseType`            |
| valid effective document                                                          | decoded `*rawConfig`       |

The first four rows are produced by stages 1-2 (`parseYAMLDocument`,
`validateSourceGraph`, `resolveEffective`) and always take precedence: a
parser, graph, source-duplicate, or resolver-bound failure is returned
before stage 3 ever inspects a single mapping entry. Every remaining row is
produced by stage 3's per-level entry walk or stage 4's final `Decode`.

At every one of the four name levels (page, group, group field, action
field), each mapping entry is classified in this fixed order, and later
steps never run once an earlier one has already classified the entry:

1. **Key shape.** A mapping key counts as a name only when its effective
   node is a scalar whose `ShortTag()` is exactly `!!str`. An integer,
   boolean, or null scalar key; a custom-tagged scalar; a sequence or
   mapping key; or an alias to any of those, is rejected as `KindParseType`
   via `validatorKeyShapeError` before its name or value is inspected at
   all. `schemaKeyName` implements this rule; the quoted string key `"<<"`
   resolves to tag `!!str` and is therefore an ordinary name (rejected as
   `KindSchema` if absent from the canonical inventory, like any other
   unrecognized name) — it is only the bare, unquoted `<<` merge key that
   carries yaml.v3's own `!!merge` tag, and that key is consumed by
   `resolveEffective` during stage 2, long before `schemaKeyName` ever runs,
   so it never reaches this classification at all.
2. **Name membership**, checked without descending into that entry's value.
   A well-formed string key that is not present in the canonical inventory
   for that level is rejected as `KindSchema`, naming the literal offending
   key and a positive line — regardless of whether the associated value is
   null, a scalar, a sequence, or a mapping. An unknown name's value is
   never inspected, decoded, or recursed into.
3. **Value inspection**, only for a name that passed step 2. A known page's
   or group's non-null scalar/sequence value is `KindParseType`; a null
   value is a no-op; a mapping value recurses one schema level down. A known
   group field's or action field's value is decoded into a fresh instance of
   that field's declared Go type, with any `*yaml.TypeError` (or other
   decode failure) preserved in `Err` and classified `KindParseType`.

Page, group, group-field, and action-field name inventories are never a
hand-maintained list in `validate.go`. `SchemaPages()`, `SchemaGroupFields()`,
and `SchemaActionFields()` (schema.go) reflect directly on the canonical
`Config`, `rawGroupConfig`, and `ActionConfig` structs, so adding, renaming,
or removing a field on one of those structs changes the accepted schema
automatically, with no parallel edit required in the validator. `SchemaGroups(page)`
is derived differently: it reads the group names present as map keys in that
page's entry in `defaultConfig()`, not struct fields of `Config` — a group
added only to `defaultConfig()`'s map (with no corresponding struct field)
still changes the accepted schema, and a parity test keeps that map's keys
from drifting out of sync with the fields the validator otherwise expects.

**Interpretation I8: runtime validation precedes `mergePage`'s tolerant
mechanics.** `mergePage` (config.go) mechanically
tolerates a group name absent from `defaultConfig()` for a page: it
synthesizes a zero `GroupConfig{Enabled: true}` as the merge base and
proceeds, matching `IsGroupEnabled`'s "missing group -> enabled" fallback.
`parseAndValidate`'s `validateGroupEntries`, by contrast, rejects that same
unknown group name outright as `KindSchema`. Runtime loading always calls
`parseAndValidate` before `mergeConfig`, so unrecognized groups can never
reach `mergePage`; its tolerance remains useful only to package-internal
callers operating on already trusted `rawConfig` values.

Once both prior stages succeed, stage 3 classifies the effective document by
its top-level node shape into exactly one of these outcomes:

- **Nil effective document** (the shared "empty or whitespace-only input"
  result `parseYAMLDocument`/`resolveEffective` already return for that
  case) — a non-nil, zero-valued `*rawConfig` (every page map nil/empty) and
  no error. This is a usable no-op overlay, matching the "absent config"
  semantics elsewhere in this package. Stage 4's `Decode` call never runs
  for this outcome.
- **Top-level null scalar** (YAML's `null`/`~`, or an absent value, which
  yaml.v3 resolves to tag `!!null`) — treated identically to the nil-document
  outcome above: a non-nil, zero-valued `*rawConfig`, no error, and no
  `Decode` call.
- **Top-level scalar that is not null, or a top-level sequence** — rejected
  as `KindParseType` via the unexported `validatorShapeError`, which names
  the actual shape found and a positive source line from the unexported
  `effectiveNodeLine` (`n.Line`, clamped to `1` when yaml.v3 left it
  non-positive, mirroring `effectiveOutputLimitError`'s own clamp). No
  `Decode` call runs for this outcome either, since there is no mapping to
  decode.
- **Top-level mapping** — accepted structurally, and its entries are
  classified one at a time, in effective `Content` order, by
  `validatePageEntries` (interpretation I3's per-entry order: key shape,
  then name membership, then value shape):
  - A mapping key whose effective node is not a scalar with `ShortTag() ==
"!!str"` — an integer, boolean, or null scalar; a custom-tagged scalar;
    a sequence; a mapping; or an alias to any of those (already
    dereferenced into a copy of its target by `resolveEffective`) — is
    rejected as `KindParseType` via the unexported `validatorKeyShapeError`
    before its name or value is ever inspected. The unexported
    `schemaKeyName(key *yaml.Node) (string, bool)` implements this rule; a
    quoted `"<<"` key resolves to tag `!!str` (an ordinary name), while a
    bare `<<` merge key carries `!!merge` and is consumed by
    `resolveEffective` long before this walk runs, so it never reaches
    `schemaKeyName` at all.
  - A well-formed string key that is not one of `SchemaPages()`'s canonical
    page names (schema.go, reflected off `Config`'s yaml tags — never a
    literal list in validate.go) is rejected as `KindSchema` via the
    unexported `validatorSchemaError`, naming the literal offending key and
    a positive line, without descending into that entry's value at all.
    (`SchemaPages()` returning an error — only possible for a malformed
    struct tag on `Config` itself, which cannot happen for that canonical
    struct — is still surfaced defensively as a `KindSchema` `*LoadError`
    via the unexported `validatorSchemaPagesError`, wrapping the reflect
    error, rather than ignored or panicked on.)
  - A known page's **null** value is accepted as a no-op for that page.
  - A known page's **non-null scalar or sequence** value is rejected as
    `KindParseType` via the unexported `validatorPageValueShapeError`,
    naming the page, the actual shape found, and a positive line.
  - A known page's **mapping** value has its own entries — groups — classified
    the same way, one schema level down, by `validateGroupEntries`:
    - A non-`!!str` mapping key is rejected as `KindParseType` via
      `validatorKeyShapeError`, exactly as at page level.
    - A well-formed string key that is not one of `SchemaGroups(page)`'s
      canonical group names for that page (schema.go, reflected off
      `defaultConfig()` — never a literal list in validate.go) is rejected as
      `KindSchema` via `validatorSchemaError`, naming the offending key and a
      positive line, without descending into its value. (`SchemaGroups`
      erroring — only possible for a page unknown to it, which cannot happen
      once `page` has passed `SchemaPages()` — is still surfaced defensively
      as `KindSchema` via the unexported `validatorSchemaGroupsError`.)
    - A known group's **null** value is a no-op for that group; its
      **non-null scalar or sequence** value is `KindParseType` via the
      unexported `validatorGroupValueShapeError`, naming the group, shape,
      and line.
    - A known group's **mapping** value has its own entries — group fields —
      classified by `validateGroupFieldEntries`, sourced from the unexported
      `groupFieldTypes()` (reflection over `rawGroupConfig`'s yaml tags and
      declared Go field types — never a literal list in validate.go, aside
      from the literal name `"actions"` itself, needed for the special case
      below):
      - A non-`!!str` mapping key is rejected as `KindParseType`, exactly as
        at page and group level; an unrecognized field name is rejected as
        `KindSchema`, exactly as an unrecognized group name is.
      - The special-cased `actions` field is recognized as a known name and
        validated structurally by the unexported `validateActionsEntries`,
        instead of by a generic decode into `*[]ActionConfig` (interpretation
        I5, because yaml.v3 silently ignores an unknown struct field on
        decode and silently decodes a null sequence entry into a zero
        `ActionConfig`):
        - Its value must be **null** (a no-op — no actions configured) or a
          **sequence**; any other shape (a non-null scalar or a mapping) is
          rejected as `KindParseType` via the unexported
          `validatorActionsValueShapeError`, naming the actual shape found
          and a positive line.
        - Every sequence entry must be a YAML **mapping**; a **null** entry
          is explicitly not accepted as a zero action, and a scalar or
          sequence entry is likewise rejected — all as `KindParseType` via
          the unexported `validatorActionEntryShapeError`.
        - Each mapping entry's own fields are classified by the unexported
          `validateActionFieldEntries`, one schema level down from a group's
          fields and sourced from the unexported `actionFieldTypes()`
          (reflection over `ActionConfig`'s yaml tags and declared Go field
          types — never a literal list in validate.go): a non-`!!str`
          mapping key is `KindParseType`, exactly as at page/group/group-field
          level; an unrecognized action-field name (e.g. `SchemaActionFields()`
          not listing it) is `KindSchema` via `validatorSchemaError`, naming
          the offending key and a positive line, without descending into its
          value; and a known field's effective value node is decoded into a
          fresh value of its declared Go type
          (`reflect.New(fieldType).Interface()`), a decode failure
          (yaml.v3's own `*yaml.TypeError`, e.g. `sudo: {a: 1}` into `bool`)
          classified `KindParseType` via `validatorDecodeError`, with the
          yaml.v3 error's message in `Detail` and the error itself preserved
          in `Err`.
      - Every other known field's effective value node is decoded into a
        fresh value of that field's declared Go type,
        `reflect.New(fieldType).Interface()` (interpretation I4) — e.g.
        `enabled`'s `*bool`, `bundles_paths`'s `*[]string`. A decode failure
        (yaml.v3's own `*yaml.TypeError`, e.g. `enabled: [1, 2]` into
        `*bool`) is classified `KindParseType` via the unexported
        `validatorDecodeError`, with the yaml.v3 error's message in `Detail`
        and the error itself preserved in `Err` — the same builder stage 4's
        final `Decode` call uses.

  Once every entry passes, stage 4 calls the effective document's
  `yaml.Node.Decode` into a `*rawConfig`. A `Decode` failure here is
  classified `KindParseType` via the unexported `validatorDecodeError`,
  with the yaml.v3 error's message in `Detail` and the error itself
  preserved in `Err` — a defensive final step, since
  `validateSourceGraph`/`resolveEffective` and the page-entry walk above
  already prove the effective document is well-formed by the time `Decode`
  runs, but any residual decode failure is still classified rather than
  left unhandled. Once every level's fields pass, an explicitly set zero
  value survives the decode as set, not as omission: `enabled: false`,
  `app_id: ""`, and `bundles_paths: []` all decode to non-nil pointers
  (`*bool`, `*string`, `*[]string`) carrying `false`, `""`, and an empty
  non-nil slice respectively — `rawGroupConfig`'s pointer fields exist
  precisely so a merge onto `defaultConfig()` can tell "explicitly set to
  the zero value" apart from "key absent."

Per the guarded `directCallAllowlist` in `sourcesurface_test.go`,
`parseAndValidate` and `resolveCandidatePath` are the runtime entry helpers
authorized for direct test calls;
`validatorShapeError`, `validatorDecodeError`, `effectiveNodeLine`,
`validatePageEntries`, `schemaKeyName`, `validatorSchemaError`,
`validatorSchemaPagesError`, `validatorKeyShapeError`,
`validatorPageValueShapeError`, `describeNodeShape`,
`validateGroupEntries`, `validatorSchemaGroupsError`,
`validatorGroupValueShapeError`, `validateGroupFieldEntries`,
`groupFieldTypes`, `validatorGroupFieldTypesError`,
`validateActionsEntries`, `validatorActionsValueShapeError`,
`validatorActionEntryShapeError`, `validateActionFieldEntries`,
`actionFieldTypes`, and `validatorActionFieldTypesError` are all exercised
only indirectly, through `parseAndValidate`, in `validate_test.go`.

**Runtime loading, precedence, and diagnostics
(`internal/config/config.go`, `paths.go`, `diagnostic.go`).** `Load` resolves
each configured candidate to the exact path it will read, then calls
`loadResolvedPath`, which reads bytes, runs `parseAndValidate`, and only then
merges the validated overlay onto `defaultConfig()`. A missing candidate
(`errors.Is(err, fs.ErrNotExist)`) advances to the next search location
unless `danglingSymlink(path)` reports that the directory entry is a symlink
whose target is missing: `os.ReadFile` reports ENOENT for such a link exactly
as for an absent path, so `Load` uses `os.Lstat` to tell them apart and treats
the dangling link as a present, unreadable authoritative file (`KindRead`).
Otherwise an administrator's broken `/etc/chairlift/config.yml` link would
silently fall back to package defaults. Any
other read failure, or any parse/type/schema failure in the first file that
exists, makes that file authoritative: lower-priority files are not read,
`Load` returns `disabledConfig()` plus the structured `*LoadError`, and every
canonical feature group is hidden while non-visibility defaults remain
available. If all candidates are absent, the ordinary built-in defaults and
a nil error are returned.

`resolveCandidatePath` makes diagnostic paths deterministic and absolute when
the operating system supplies a working directory: absolute candidates are
cleaned; a relative candidate prefers a file beside the executable and
otherwise resolves against the current working directory. The
executable-relative probe uses `os.Lstat`, so a dangling symlink beside the
executable stays the selected candidate and fails closed in `Load` instead of
falling through to the working directory. Filesystem reads,
the executable path, and the working directory have narrow package-level
seams so precedence and permission failures are testable without relying on
host permissions.

On an authoritative failure, `LoadError.LogMessage` prefixes the full
path-and-cause diagnostic with `CONFIGURATION ERROR`, states that all groups
were disabled, and instructs the user to restart after fixing the file.
`config.Load` logs that line itself. `openMain` (`internal/app/app.go`)
keeps the same `LoadError` in `Env.ConfigErr`, and `Main.View` shows
`LoadError.ToastMessage` once, in the window's first frame, as an error toast
with `ID: "config-error"` and `Timeout: -1`, so the startup error stays on
screen until the user closes it instead of expiring; the first-run window
does the same (`newFirstRun`). MyGo toasts are plain text and parse no
markup, so a path or tool error containing `<`, `>` or `&` shows as it is.
Every toast is built and shown on the UI thread: the views queue them with
`Env.Toast`, `Env.ToastSuccess` and `Env.ToastError`, and `Main.View` hands
them to MyGo (`Env.TakeToasts`).

### Package manager wrapper pattern

Each wrapper in `internal/` follows a consistent shape:

- Module-level `dryRun` flag with `SetDryRun()`/`IsDryRun()`
- `IsInstalled()` to check tool availability, plus `IsInstalledCached()` (`sync.Once`), which the domains call on a goroutine during their first load (see [Tool availability on a goroutine](#tool-availability-on-a-goroutine))
- Homebrew, Flatpak, fwupd, and Updex implement both `IsInstalled()` and `IsInstalledCached()`
- List/Search/Install/Uninstall/Update functions
- Context-based timeouts. Homebrew and Flatpak both use a two-class model selected per invocation by an unexported `commandTimeout(args)` helper: 30s for read-only commands, 30m for state-changing ones (the keys of each package's `stateChangingCommands` map). fwupd passes its timeout per call instead: 60s for each read. updex's reads use 5min and bootc's 30min. A change a cancel cannot stop has no deadline, because a deadline would kill only the client and free the job queue while the work goes on: a firmware install, the updex helper's commands and the OS stage.
- Custom error types where needed
- Every external command is reported to `internal/cmdlog` (`Start`, the `Stdout`/`Stderr` tees, `End`; `DryRun` for a skipped command), so the console shows it with its output (see [The console](#the-console-internalcmdlog))

### Shared OS staging progress (`internal/stageexec`)

`bootc.StageUpdate` and `sysupdate.StageUpdate` retain their provider APIs and
fixed commands — `pkexec /usr/libexec/bootc-update-stage` and `pkexec
/usr/libexec/snosi-sysupdate-stage`, respectively — but both delegate execution
to the pure-Go `internal/stageexec` leaf package:

1. The caller creates the provider's `ProgressEvent` channel; both provider
   types are aliases of `stageexec.ProgressEvent`.
2. Each non-empty output line becomes an `EventMessage`; the channel is closed after either an `EventComplete` (success) or the function returning an error
3. Event types: `EventMessage` and `EventComplete` — deliberately simpler than
   a step/percent model because the stage script's own output is unstructured
   log lines, not a structured progress protocol. Failures return as errors;
   they are not duplicated into the event stream.
4. `stageexec.Run` owns merged stdout/stderr, non-zero exits with the last output
   line, deadline/cancellation classification, missing bare-name or absolute-path
   executables, direct-child kill/reap, the single success completion, and
   channel closure. `stageexec.DryRun` owns the synthetic preview/completion and
   closure without constructing an `exec.Cmd`.
5. `System.stageJob` (`internal/views/system.go`) reads the same event
   contract for both kinds of host on its job's goroutine: each
   `EventMessage` becomes the job's status line (`Progress.Set`), and the
   job's `Done` applies the result on the UI thread. The script's output
   reaches the console through `stageexec.Run`'s `cmdlog` trace.

**Caller-visible outcomes.** Provider adapters preserve `bootc.Error` /
`sysupdate.Error` and `NotFoundError` while carrying the shared executor's
message and cause. Both context-taking bootc functions classify failures with
`errors.Is` against the context sentinels, and `bootc.Error` has an `Err error`
field plus `Unwrap() error` so callers can tell them apart:

- Either provider's `StageUpdate` — deadline: provider `*Error` "Update staging timed out" unwrapping to `context.DeadlineExceeded`; cancellation: provider `*Error` "Update staging was canceled" unwrapping to `context.Canceled`; non-zero exit: provider `*Error` "update staging failed (exit N): <last output line>" matching neither sentinel; missing `pkexec`: provider `*NotFoundError`.
- `GetStatus` — deadline: `*Error` "bootc status timed out" unwrapping to `context.DeadlineExceeded`; cancellation: `*Error` "bootc status was canceled" unwrapping to `context.Canceled`; non-zero exit: `*Error` "bootc status failed (exit N): <stderr>" matching neither sentinel; missing `bootc`: `*NotFoundError`. `GetStatus(ctx)` is `return getStatusFrom(ctx, bootcCommand)`; the unexported `getStatusFrom` seam exists so tests exercise all of these against a fake script without a real `bootc`.

The deadline and cancellation messages differ in both functions, and neither ever surfaces as `signal: killed`.

**OS staging direct-kills; homebrew and flatpak kill the process group.** On
cancellation `stageexec.Run` kills only the direct child (`cmd.Process.Kill()`)
and sets no `Setpgid`, because both staging adapters run under `pkexec` and the
privileged child cannot be signaled as an unprivileged process group. The
unprivileged Homebrew and Flatpak runners instead kill their whole process
groups so download helpers are not orphaned. Making either privileged staging
path group-killable is a privilege-model change.

**Why a stage script instead of `bootc upgrade`:** upstream `bootc upgrade`'s registry-transport pull currently fails on snow's composefs images. The snow-shipped `/usr/libexec/bootc-update-stage` script works around this: `podman pull` fetches the image into containers-storage (podman's pull path works where bootc's does not), then `bootc switch --transport containers-storage` stages the already-pulled image as the next boot deployment. This keeps snow's actual upgrade logic in one place (the snosi script) rather than duplicating pull/switch orchestration in ChairLift; ChairLift only invokes the script via pkexec and streams its output. The script is idempotent — it exits 0 without staging anything when the deployment is already current.

### System image staging (the image source)

`System` (`internal/views/system.go`) owns the deployments and the "System
image" update source (`imageSource`). `System.loadDeployments` reads them on a
goroutine under a `RefreshGate` generation, when `system_page`'s
`bootc_status_group` or one of the two OS-update groups asks for them:
`bootc.GetStatus` on a bootc host (booted, staged and rollback image,
version, digest, pin, build time), or on a native A/B host the
`/run/snosi` state files (`sysupdate.GetStatus`) and the rollback slot
(`sysupdate.RollbackVersion`). `System.compose` turns them, with the host
facts, into `SystemInfo`, which the Overview hero, the status bar, the rail's
host card and Maintenance read.

The source is available when the load found a host of an enabled group's
kind with its stage script. Its one item, `Stage`, is offered while nothing
is staged and nothing says the host is current: only the stage script can ask
the registry for a newer image, and it needs root, so on bootc the item
stands for "check for and stage the newest image" until a live run this
session staged nothing; a native A/B host has the stager's own record of its
last check (`sysupdate.OutcomeCurrent`). While no item is offered,
`imageSource.Extra` draws a "Check and stage" button; on a native A/B host it
also draws the read-only previous-version line
(`pageview.SysupdateRollbackSubtitle`). A staged image is the source's note
(`pageview.StagedNote`) and the Overview's "Restart to finish" state;
ChairLift never restarts the computer itself.

`System.stageJob` is a job on the serial queue. It runs the fixed stage
script of the host's kind through pkexec (`bootc.StageUpdate` or
`sysupdate.StageUpdate`; never another path) with no deadline (the job is
`Unstoppable`), shows each `EventMessage` as the job's status, and then
re-reads the deployments in both modes under `bootc.DefaultTimeout`, so the
source and the hero show what is really staged. `System.finishStage` applies the run on the UI thread: it takes a new
generation (the run's read is newer than any load in flight), shows the
error (a cancel is a plain toast), or the dry-run-aware toast
(`actionmsg.BootcStage`, `actionmsg.SysupdateStage`); after a live run that
staged nothing it keeps the script's last line as the source's note
(`pageview.BootcStageResultSubtitle`, `pageview.SysupdateStageResultSubtitle`),
and after a live run that staged a new image it records the activity.
"Check now" re-reads the status only; it never prompts.

### Firmware updates (the firmware source)

`internal/fwupd` wraps the unprivileged `fwupdmgr` client of the Linux Vendor
Firmware Service. `Firmware` (`internal/views/firmware.go`) holds its devices
and the firmware update source of `updates_page`'s `firmware_updates_group`.
`Firmware.load` checks `fwupd.IsInstalledCached()` on a goroutine; without
`fwupdmgr` the source stays unavailable, so a host without fwupd never sees
it. Otherwise the source shows as checking, `fwupdmgr get-updates --json`
runs, and `Firmware.applyLoad` publishes the list under a `RefreshGate`
generation, so a stale query cannot replace a newer list. `Firmware.publish`
is the only writer of the rows: it replaces the whole list. A query that
exits 2 ("no updatable devices") is an empty list, not an error; any other
failure keeps the rows of the last list and shows
`pageview.FirmwareUpdatesSubtitle`'s error text as the source's error.

Every device with an update is an item with `Confirm` set and the action
`Install…`: firmware is never part of "Update everything" (`Jobs` with no
keys returns nothing). A device that fwupd will not flash now — no
`updatable` flag (a closed lid, a host on battery), a non-empty `Problems`
array, or a non-empty `UpdateError` — stays listed as a held item with its
reason in the detail (`pageview.FirmwareBlockedUpdate`) and no install
button. Without that rule a zero exit from a skipped device would read as a
successful flash.

One `actionstate.Gate` (`installGate`) covers the whole source, from the
Install click until that install ends, so only one confirmation and one
install are ever under way: a click that finds the gate taken shows the toast
`Another firmware update is installing` and does nothing else.
`Firmware.ask` opens a destructive confirmation with
`pageview.FirmwareInstallConfirmation`: the body always warns not to turn off
or unplug the computer and to keep the power connected, then states whether
the device needs a restart, a shutdown, or nothing, and that ChairLift will
not restart the computer itself; its details name the device, vendor and
versions, and its note says that fwupd may ask for administrator
authentication through its own PolicyKit action. Cancel releases the gate.

On confirm, `Firmware.installJob` queues the install. `fwupdmgr update` can
exit 0 without flashing a device, and neither a missing update offer nor a
different offered version proves a flash, so `fwupd.InstallAndVerify`
follows the install with `fwupdmgr get-devices --json` — the device's own
report. `fwupd.VerifyInstall` classifies it, first match wins: a failed
state query is `InstallUnverified` with its error text; a device that is not
listed is `InstallUnverified` because it can be restarting; `UpdateState` of
`needs-reboot` or `pending` is `InstallStaged`; `UpdateState` of `failed` or
`failed-transient` is `InstallNotApplied`, even with a matching version (fail
closed); the new version is `InstallFlashed`; the old version is
`InstallNotApplied`; anything else is `InstallUnverified` and names the
version the device reports. `InstallUnverified` is the zero value of
`InstallResult`, so an unknown outcome cannot read as a success. The
`get-updates` list is read afterwards only to rebuild the rows.

`actionmsg.FirmwareInstall` turns the report into one decision, which
`Firmware.finishInstall` applies on the UI thread: `decision.PublishList`
publishes `report.Updates` as the new list (after a fresh generation, so a
load still in flight is stale), `decision.RestoreControl` lets the device
offer its install again, `decision.IsError` selects the error toast, and
`decision.CompletedLabel` records a finished install (`Installed` or
`Restart required`) on the old row when the list could not be read, which
then also shows the list error. No row is removed by identity: the published
list is the whole list, so a device that still has an update stays visible.
A live flash or staged flash is recorded in the activity. The gate resets
last, in every case. Every call is unprivileged — fwupd's own PolicyKit
actions authorize the daemon — which is decision record
[ADR-0012](../adr/0012-firmware-updates-use-fwupds-own-polkit.md).

### The Updates page and the update counts

The update counts have one owner per source: each `UpdateSource` owns its
items, and nothing keeps a separate counter. `Store.UpdateCount` sums the
items of every available source, held ones included, and counts the sources
that have any; the Updates rail badge (`updatesPage.Badge`), the Overview's
Updates figure and the status bar's "N updates" all read it. A staged system
image offers no item, so it adds nothing; the Overview shows it as "Restart to
finish" instead. A firmware device that fwupd will not flash now is a held
item, so it counts.

Each domain keeps its items known: a live success removes its item before
any reload (Homebrew's `finishUpgrade`, Flatpak's `finishUpdate`, Features'
`updated`), a failure
or a dry run keeps it, and a failed reload keeps the last known items
(`actionstate.OutdatedRefresh` for Homebrew and for a Flatpak load in which
no installation returned a usable result, `Firmware.applyLoad` for firmware,
the last successful check minus completed live updates for features).
`Features.updated` retires only `updex.ClassUpdated` features after a live run
with no error, before rechecking. Held, waiting, failed, current and unreported
features keep their items; failed runs and dry runs retire nothing. Only a
successful, current-generation check clears these completion markers and
replaces the summary. The features source's items all run as one
helper `update` (`Features.updateJob`), because the helper updates every
feature that is on.

The Updates page (`internal/views/updates.go`) owns no update state. Its
header says how many items are ready and held, when the sources were last
checked, and that staged system updates apply on restart, with Check now
(`updatesPage.checkNow`: `Refresh` on every source, and it forgets the last
run) and Update everything (disabled with a reason while jobs run or nothing
runnable is left). While jobs run or any source has items, the run card draws
the update run as a `frostui.Pipeline`: one stage per source, one step per
item, each step's state from its job (`frostui.StateOf`: queued, running with
its time, done, failed), else held for a held item or one that waits for a
confirmation, else ready. A stage's time adds each of its jobs once, so the
features that share one helper run count that run's time once. Then one card per available source: its title, blurb,
status (checking, its error, its note, when it was last checked), its
`Extra`, "Update all" for its default jobs, and a row per item with the
versions and its button or what its job did. The page keeps the last batch
of jobs, so a row its run updated still says "Updated" after the source
dropped the item, until the next batch or Check now.

### The console (`internal/cmdlog`)

`internal/cmdlog` is the command tracer. A backend announces a command with
`cmdlog.Start(name, args...)`, tees its stdout and stderr through
`Run.Stdout`/`Run.Stderr` (or reports lines it reads itself with `Run.Line`),
and ends it with `Run.End(err)`; a dry-run preview that starts no process is
reported with `cmdlog.DryRun`. A desktop app that ChairLift starts in process,
with no command of its own, is reported with `cmdlog.Launch(id)`: its events
carry the desktop file ID and `Event.Launch`, and its `End` comes when the
launch returns, not when the app exits. With no sink every call is a cheap
no-op (a nil `*Run` is valid), so the backends and their tests behave as
before. Lines are split on `\n` or `\r`, trimmed, and cut at `cmdlog.MaxLine`
(4096 bytes), so one runaway line cannot hold the console's memory. The
package is a leaf: it imports no UI package and no backend.

Who reports: `homebrew.runBrewCommandAt` and its dry-run skip, Flatpak's
`runFlatpakCommandAt` and its dry-run skip, the Homebrew and Flatpak
`IsInstalled` version probes, fwupd's `runCommandAt`, `bootc.GetStatus`,
`updex.runHelper` (the pkexec helper, and `cmdlog.DryRun` in dry run),
`stageexec.Run` and `stageexec.DryRun` (both stage scripts),
`views.runMaintenanceCommand` (the configured maintenance scripts),
sysupdate's `runLsblk` (the rollback slot read) and the Problems page's
`execStream` (its `coredumpctl` and `journalctl` reads, whose machine output
is not teed). `views.launchDesktopApp` reports each desktop app launch with
`cmdlog.Launch`: the Overview's system monitor, the Apps page's software
center and a Flatpak app's Open. The first run's `gsettings` and settings
launcher call `cmdlog.Start` too, but only the main window installs a sink,
so nothing shows them (the first-run window has no console).
`internal/installcheck.TestEveryCommandIsTraced` requires a `cmdlog.Start`
beside every `exec.Command` under `internal/` and a `cmdlog.Launch` beside
every `frostui.LaunchDesktopApp` call.

`app.wireConsole` connects it to the window: `cmdlog.SetSink` turns each event
into a console line — `KindStart` a `Command` line (`$ argv`, its source the
program's base name, through pkexec to the program it runs), `KindLine` an
`Output` line, `KindDryRun` a `Dry run` line (`would run: …`), and `KindEnd` a
`Debug` line `exited after <time>` or a `Warn` line `exited after <time>:
<err>`. A launch (`Event.Launch`) is a `Command` line `in-process launch of
<id> (GIO)` with the source `launch`, and its end a `Debug` line `launched
after <time>` or a `Warn` line `launch failed after <time>: <err>`. It also
tees the standard logger into the console (`Console.Writer(LevelDebug, "log")`
beside stderr, with the timestamp stripped): a line with `[DRY-RUN]` is a `Dry
run` line, a line with "error" or "failed" a `Warn` line, anything else
`Debug`. The job queue adds `Info`, `Warn` and `Error` lines for each job's
start and end, and a job's `Progress.Log` adds lines under its source.

The console (`frostui.Console`, `frostui.ConsoleView`) keeps the newest 5000
lines in a ring buffer and is safe from any goroutine; its notify wakes the
window through `frostui.Waker`. The pane opens under the page (Ctrl J, the
status bar's Console item, the palette, a click on the running job in the
status bar), filters by level (Debug is hidden by default) and by text,
follows the newest line, copies the lines shown, and clears.

### The command palette

Ctrl K (or the top bar's field, or the main menu) opens `frostui.Palette`:
a dialog with a field that filters the commands, ranked by
`frostui.Match` (an ordered subsequence; word starts, runs and a match at
the start count more), chosen with Up, Down and Enter. `Main.Commands`
builds the list every frame: each visible page's `Commands()`, then "Go to"
each visible page (with its Alt+number), then the window's own: Check for
updates (Ctrl R), Stop the running jobs, Show or hide the console (Ctrl J),
Keyboard shortcuts (Ctrl ?), Help (F1) and About ChairLift. A command may be
`Disabled` with a `Reason`. A chosen command runs at the start of the next
frame, first thing in `Main.View` (`Palette.RunPending`), so the page it opens
and the toast it makes show in that frame. The palette keys its rows by `Command.ID`, so IDs must be unique;
`uniqueCommands` keeps the first of each ID, because two pages may list the
same shared command (Homebrew's on Apps and Updates). The palette lists only
actions the pages offer, through the same domain methods, confirmations and
fixed paths ([ADR-0017](../adr/0017-mygo-command-center-window.md)).

### The status bar

`Main.statusStart` and `Main.statusEnd` build the status bar every frame. At
its start: the running image (`<OS name> <booted version>`), the update
channel, the uptime, a staged image (`<version> staged`, which opens the
Overview), and "Dry run: nothing changes" under `--dry-run`. At its end: the
running job with the batch's progress (a click opens the console), "N
updates" from `Store.UpdateCount` (opens Updates, while Updates is visible),
"N problems" from `Problems.Summary` (opens Problems, in the warning tone when
a problem needs attention), the free space of the home folder's file system,
and the shell's own Console item.

### Live figures (`internal/sysinfo`, `health_group`)

`internal/sysinfo` reads the host facts without running a command: processor
use from two readings of `/proc/stat` (`ParseCPU`, `CPUPercent`), memory from
`/proc/meminfo` (`ParseMemory`: MemTotal and MemAvailable), free space with
statfs (`ReadDisk`), uptime from `/proc/uptime`, the kernel release
(`Kernel`, uname), the hardware model from DMI (`Model`), and the Secure Boot
state from its EFI variable (`ParseSecureBoot`). `System.readFacts` reads the
host facts once at start (os-release, host name, kernel, model, Secure Boot,
uptime, which `System.Info` then advances with the clock).

`Live` (`internal/views/live.go`) samples on a goroutine of its own for the
app's life, through a `sysinfo.Sampler` of the home directory's file system.
With `system_page`'s `health_group` on it samples everything each second and
keeps one minute of processor and memory history for the Overview's "Right
now" chart (`frostui.LiveChart`), beside the configured system monitor button
(`app_id`, default `io.missioncenter.MissionCenter`). `liveSampler` copies its
backend functions and publication callback before starting. Its readings go
into `liveBuffer`, a mutex-protected transfer buffer capped at 60 samples;
only the UI thread applies them to `Live.latest`, `cpu` and `mem`.

`Main.Navigate` tells `Live.SetVisible` whether Overview is selected. `Main.View`
also synchronizes that choice before drawing and after the shell's rail can
change the selection. While Overview is selected, each sample requests a
post; elsewhere, only a change to the disk's total or formatted free space
requests one. At most one post is pending, and it drains the latest buffered
samples. Returning to Overview takes the buffer on the UI thread immediately,
so its first frame has the newest minute of history without an extra post.
`TestLivePostsOnlyForVisibleChanges` counts posts and checks that hidden
sampling leaves UI-owned history untouched until delivery;
`TestLiveCoalescesSamplesWhileAPostWaits` covers delayed posts and navigation.

With the health group off, the sampler reads only free space every 5 seconds
and posts only when the shown value changes, whatever page is selected.
The status bar, Overview's Free space figure and disk card, and Maintenance
use that free space whatever the configuration.

### Recent activity (`internal/activity`)

`internal/activity` is ChairLift's own record of what it changed on this
computer for this user ("Staged Snow Linux 2026.11.08", "Updated Firefox",
"Turned on Docker"): only what ChairLift itself did and saw succeed, not a
history of the system. It is a JSON Lines file,
`$XDG_STATE_HOME/chairlift/activity.jsonl` (else
`$HOME/.local/state/chairlift/activity.jsonl`; `activity.DefaultPath`),
written whole through a temporary file and a rename, at most
`activity.MaxEntries` (200) entries. `app.openMain` opens it; the domains add
an entry with `Env.Record(source, title, detail)` after a live success
(stage, feature toggle and update, Flatpak and Homebrew changes, firmware,
cleanups). `Env.Record` does nothing in a dry run, and `activity.Open` with
`dryRun` keeps entries in memory and never writes the file (ADR-0009). The
Overview's Recent activity card lists the newest five with
`activity.When` ("just now", "12 min ago", "3 h ago", "Yesterday", a
weekday within a week, else a date).

### Privileged operations

Decision records: [ADR-0001](../adr/0001-fixed-path-pkexec-privilege-boundary.md)
(the fixed-path pkexec boundary and helper argv re-validation),
[ADR-0002](../adr/0002-usr-prefix-is-the-only-supported-install-prefix.md)
(`PREFIX=/usr`), and
[ADR-0006](../adr/0006-split-system-integration-package-with-mutual-conflicts.md)
(the system-integration package split).

bootc staging, native A/B staging, and updex require root for state-changing operations. They invoke commands through `pkexec` (PolicyKit). bootc runs `pkexec /usr/libexec/bootc-update-stage` directly (polkit action id `org.frostyard.ChairLift.bootc.stage`), native A/B staging runs `pkexec /usr/libexec/snosi-sysupdate-stage` directly (`internal/sysupdate.StageScriptPath`, action id `org.frostyard.ChairLift.sysupdate.stage`), and updex delegates to the fixed absolute path `internal/updex.HelperPath` (`/usr/bin/chairlift-updex-helper`) via `pkexec`. Polkit policy files are installed for all three: `data/org.frostyard.ChairLift.bootc.policy`, `data/org.frostyard.ChairLift.sysupdate.policy`, and `data/org.frostyard.ChairLift.updex.policy`. ChairLift deliberately ships no `.rules` files: the policies require normal administrator authentication (`auth_admin`, with `auth_admin_keep` for an active local session) rather than granting blanket passwordless access to a login group. Source installation removes the two legacy ChairLift `.rules` files so an older passwordless rule cannot survive an upgrade. Homebrew tap trust (`brew trust`) is explicitly per-user and does _not_ go through pkexec — see [package-managers.md](./package-managers.md). Firmware installs are the other unprivileged mutation: `internal/fwupd` calls `fwupdmgr update --no-reboot-check <DeviceId>` directly and the fwupd daemon authorizes it with fwupd's own PolicyKit actions, so ChairLift contributes no policy file and no helper to that path ([ADR-0012](../adr/0012-firmware-updates-use-fwupds-own-polkit.md)).

**Why the helper path must be absolute, and why `PREFIX=/usr`:** `pkexec`
resolves the program it's asked to run to an absolute path and compares it
textually against the `org.freedesktop.policykit.exec.path` annotation on
each action in `data/org.frostyard.ChairLift.updex.policy` (all three actions
annotate `/usr/bin/chairlift-updex-helper`). The policy also uses
`org.freedesktop.policykit.exec.argv1` to select the corresponding
`enable-feature`, `disable-feature`, or `update` action from the first helper
argument. PolicyKit does not validate the remainder of argv: the privileged
helper's pure `internal/updexhelper.ParseInvocation` boundary accepts only
`enable-feature <name> [--dry-run]`, `disable-feature <name> [--dry-run]`, and
`update [--dry-run]`, rejecting extra, misplaced, or unknown arguments before
calling updex. A bare, `$PATH`-resolved command name can resolve to a different
absolute path depending on the invoking process's `$PATH`, which makes the
path comparison miss and falls `pkexec` back to the generic, more restrictive
action. `internal/updex/updex.go`'s `runHelper` therefore always invokes
`HelperPath` (never a bare name).

**Helper output and exit-code contract.** On success the helper prints its
result as JSON on stdout and exits 0; for `update` that is a JSON array of
`updex.UpdateFeaturesResult`. On failure it prints the error on stderr and
exits non-zero: `updexhelper.ExitFailure` (1) for any failure, and
`updexhelper.ExitActivationIncomplete` (3) when the error wraps
`sysext.ErrActivationIncomplete` (the extensions merged, but activation did
not finish; a reboot finishes it). pkexec passes the code through. The
`update` command also prints its results array on stdout when it fails,
because updex returns the per-feature results together with the error
(`updexhelper.WriteUpdateResult`); `enable-feature` and `disable-feature`
print nothing on failure, and ChairLift never sets `Now`, so they never
refresh. The GUI and the helper can come from different ChairLift versions
(the system-integration package ships the helper alone), so the contract is
additive: success output is unchanged; an older GUI ignores stdout and
treats every non-zero exit as failure, which stays safe for code 3; a newer
GUI with an older helper sees exit 1 with empty stdout and shows the plain
"Update failed: ..." toast.

The helper keeps stdout for the JSON result alone. The updex SDK runs
`systemd-sysext`, `systemd-sysusers` and `systemd-tmpfiles` with
`cmd.Stdout = os.Stdout`, and their text would come before the JSON. So
`main` keeps the real stdout for the result (all three commands write to it)
and sets `os.Stdout = os.Stderr` before the SDK runs: child-process output goes
to stderr. The GUI still reads defensively, for helpers from older ChairLift
versions: `updex.UpdateFeatures` decodes the whole trimmed stdout as a JSON
array, then falls back to the last non-empty line (the JSON ends with a
newline). When the helper exits 0 and no array can be read (empty stdout, bad
JSON, `null`), the outcome is unknown: the result is nil results with
`*updex.ResultsUnreadableError`, logged, and the Features toast says "Feature
update finished, but ChairLift could not read the results. See the log for
details." as a persistent error toast. It never claims success. When the helper
exits non-zero, the helper error wins (`*ActivationIncompleteError` or
`*Error`) and any results that did parse come with it.

Separately, `polkitd` reads application policies from the fixed directory
`/usr/share/polkit-1/actions` — not `$XDG_DATA_DIRS`, not any
`$PREFIX`-derived path — so the Makefile's `install`/`uninstall` targets
require `PREFIX=/usr` (the default since issue #59) for a source install's
polkit assets to land somewhere polkit actually looks. These constraints are
system facts, not values ChairLift decides; the Makefile and
`internal/updex.HelperPath` exist to conform to them, matching the layout
`.goreleaser.yaml`'s nFPM packages already use.

**System-integration delivery:** GoReleaser publishes two mutually exclusive
package shapes. `frostyard-chairlift` is the existing self-contained package
with both application binaries, desktop assets, maintainer config, and
policies. `frostyard-chairlift-system-integration` is the root-owned companion
for a user-scoped GUI delivery such as the Homebrew cask: its build filter
contains only `chairlift-updex-helper`, and its contents contain all three
policies plus `/usr/share/chairlift/config.yml`. The packages declare conflicts
because they intentionally own the same privileged files.

The integration package does **not** ship `bootc-update-stage` or
`snosi-sysupdate-stage`. Those operations are distro policy, so an image that
enables `bootc_updates_group` must provide a trusted implementation at the
existing fixed `/usr/libexec/bootc-update-stage` path, and native A/B images
ship `/usr/libexec/snosi-sysupdate-stage` (plus the `/usr/lib/snosi/native-ab`
marker) themselves. Keeping the paths fixed preserves the PolicyKit executable
boundary; making either a user-writable config value would allow the GUI
configuration to redirect a root execution. The system image source is
available only when the host's stage script is installed
(`bootc.StageScriptAvailable`, `sysupdate.StageScriptAvailable`, read by
`System.readDeployments`), so an absent distro helper leaves the source out
of the Updates page.

### Maintenance action execution

`Maintenance` (`internal/views/maintenance.go`) lists the cleanups of the
"Clean up" card in the order "Clean everything" runs them
(`Maintenance.Cleanups`): the configured `actions` of
`maintenance_cleanup_group` (keys `script:<index>`), the Homebrew cleanup
(`brew cleanup`, `maintenance_brew_group`) and the removal of unused Flatpak
runtimes (`flatpak uninstall --unused -y`, `maintenance_flatpak_group`). Each
row shows the exact command it runs; `TestMaintenanceBackendCommandsMatchTheRows`
keeps the Homebrew and Flatpak rows in step with `homebrew.Cleanup` and
`flatpak.UninstallUnused`. A tool still being checked shows a spinner and
offers nothing; a missing tool's cleanup is left out.

Every cleanup asks first (`Maintenance.Ask`: the command, and "Needs
administrator authentication." for a `sudo: true` action);
`Maintenance.AskEverything` asks once, listing every cleanup that can run,
and queues them all. `Maintenance.job` builds the job:

1. For a configured action, `decision := actionmsg.MaintenanceScript(env.DryRun, title)`
   is computed once, on the UI thread, before the job is queued; the job never
   recomputes it (see "Dry-run mode" above). The command comes from
   `pageview.MaintenanceCommand(script, sudo)`: the script itself, or
   `pkexec <script>` for `sudo: true`.
2. The job's `Run` logs `Running action: <title> (script: …, sudo: …)`. When
   `decision.Execute` is false (dry run) it constructs no `exec.Cmd` at all:
   it logs `[DRY-RUN] Would execute: ...` and reports `cmdlog.DryRun`.
   Otherwise `runMaintenanceCommand` runs the command with
   `exec.CommandContext` (`WaitDelay` 5 seconds for children that hold its
   output open), announced with `cmdlog.Start`, its stdout and stderr
   streamed to the console through the trace's tees, and ended with `End`.
   An unprivileged script runs under a 5-minute timeout
   (`maintScriptTimeout`). A `sudo: true` script has no deadline: a timeout
   would kill only pkexec and free the queue while the root script goes on.
   A timeout is the error `timed out after 5m0s`; a cancel is
   `context.Canceled`.
3. The job's `Done`, on the UI thread, sets the row's outcome (done,
   previewed, failed with its error, canceled), shows `decision.Toast`
   (dry run), a success toast, or the error toast `<title> failed: <err>`,
   and records the activity after a live success.

The Homebrew and Flatpak cleanups run `homebrew.Cleanup` and
`flatpak.UninstallUnused` (both report through `cmdlog` themselves) and toast
`actionmsg.Cleanup(dryRun, tool, output)`, where `dryRun` is that wrapper's
own flag.

### Keyboard shortcuts

`internal/navigation` is the single UI-free authority for the rail's page
order, titles, icons, the advertised shortcuts and the page-selection
transition. Each `navigation.Item` maps a page to the configuration groups
that show content on it; `navigation.VisibleItems` filters that inventory
with `Config.IsGroupEnabled`, always keeps the Overview (`AlwaysShow`), and
assigns compacted Alt+number keys over the visible pages.
`navigation.Shortcuts(visible)` is the one list of shortcuts: "Go to <page>"
for each visible page, then the general ones. `internal/window` uses it for
the rail, the bindings and the shortcuts dialog, so the bound and the
advertised keys are the same list.

The shortcuts are:

- `Alt+1` through `Alt+N` → the first through Nth visible page in rail
  order, with omitted pages leaving no gaps (with every page visible:
  Overview 1, Updates 2, Apps 3, Features 4, Problems 5, Maintenance 6)
- `Ctrl+K` → the command palette
- `Ctrl+J` → show or hide the console
- `Ctrl+R` → check for updates (`Store.CheckAll`)
- `Ctrl+?` → the keyboard shortcuts dialog
- `F1` → the Help dialog
- `Ctrl+Q` → quit

`frostui.Shell.View` binds Ctrl K and Ctrl J itself; `Main.bindShortcuts`
binds every other shortcut of the list with MyGo's `OnWindowShortcut`
(`keyOf` maps navigation's key names to MyGo's; `Ctrl` is MyGo's `Cmd`
modifier) and dispatches it in `Main.runAction`; the main menu shows the same
keys. The rail, the palette's "Go to" commands, the pages' links
(`Env.Navigate`) and the shortcuts all go through `Main.Navigate`, which
applies `navigation.Resolve` against the visible inventory and ignores an
omitted or unknown page; `Main.View` falls back to the first visible page if
the selection is not visible.

Tests: `internal/navigation`'s tests cover the canonical order, `Resolve`,
the Overview that always shows, each group keeping its page, compacted
shortcuts, unique shortcuts that cover every page, copies that cannot change
the canonical state, `TestEveryConfiguredGroupHasOneHome` (every schema group
except Help's, first run's and the deprecated one belongs to exactly one
page) and `TestViewsGateEveryGroup` (every page's groups are named in
`internal/views`). `internal/window`'s `TestEveryAdvertisedShortcutWorks`
presses every shortcut of `navigation.Shortcuts` in a headless Tester and
checks what it did, and `TestPaletteListsEveryVisiblePage` checks the
palette's "Go to" commands.

### URL and application opening

Links are MyGo links: `ui.Link` in the Help dialog (`Main.showHelp`), the
About dialog, the Overview's os-release URLs and the Problems page's "What is
sent" opens the URL with the desktop's default handler (MyGo's URL opener,
GIO's `g_app_info_launch_default_for_uri` on Linux); the first run's Reporting
step calls `c.OpenURLThen` and logs a failure. No `xdg-open` is run.

Desktop applications start by their desktop file ID through
`frostui.LaunchDesktopApp` (frostui v0.5.1 and later): it resolves the desktop
entry and launches it in process through GIO (`g_desktop_app_info_new` and
`g_desktop_app_info_launch_uris_as_manager`, from `libgio-2.0.so.0` loaded
with purego), detached from ChairLift's stdout and stderr, and does not wait
for the app. No launcher program runs. A missing, deleted (`Hidden=true`) or
unlaunchable entry fails before the call returns, with GIO's reason in the
error. The lookup and the launch are synchronous and can block, so
`Env.launchApp` runs the page's `launch` backend (`launchDesktopApp`, which
reports through `cmdlog.Launch`) on a goroutine and hands the result back with
`Env.Post`; a failure is an error toast ("Failed to launch <id>") with GIO's
reason in the log. The Overview's system monitor button and palette command
use it through `overviewPage.openHealthApp`; the Apps page's software center
button and palette command and a Flatpak app's Open use it through
`Flatpak.LaunchApp`. A launch is not a state change and is not a job, so it
also runs in dry run. GIO gets no display launch context, so startup
notification and Wayland focus activation are not supplied.

### Known limits

These hold today; none is a defect with an open issue. They were first
recorded in the retired `.planning/codebase/CONCERNS.md`, checked again
against the code for [ADR-0016](../adr/0016-adopt-the-repository-baseline.md),
and again after the MyGo port.

- **App launch:** `launchDesktopApp` (`internal/views/env.go`, over
  `frostui.LaunchDesktopApp` in the frostui module) passes the
  application ID to GIO without a check of its form beyond frostui's (not
  empty, no NUL byte). GIO launches any desktop entry it finds by that ID,
  including one hidden from the menu (`NoDisplay`). The IDs come
  from the configuration (`app_id` of `applications_installed_group`, default
  `io.github.kolunmi.Bazaar`, and of `system_page`'s `health_group`, default
  `io.missioncenter.MissionCenter`) or from flatpak's own list of installed
  apps (the Apps inspector's Open), never from typed input, and no shell
  runs.
- **Maintenance scripts with `sudo: true`:** `runMaintenanceCommand` runs
  `pkexec <script>` with the script path from the configuration
  (`pageview.MaintenanceCommand`), outside the fixed helper set of
  "Privileged operations". PolicyKit's default `pkexec` action still asks
  for administrator authentication, and on a packaged install the
  configuration comes from the root-owned `/etc/chairlift` or
  `/usr/share/chairlift` file. ChairLift does not check the script path. It
  logs the title, script and `sudo` value when a run starts (and the
  `[DRY-RUN]` line in dry-run mode), streams the script's output to the
  console through `cmdlog`, and shows a failure as an error toast and on the
  cleanup's row; the output is not kept after the console's 5000 lines or the
  end of the session.
- **Commands outside the console:** the first run's `gsettings` and
  settings launcher report through `cmdlog`, but only the main window
  installs a sink, so the first-run window shows neither (see
  [The console](#the-console-internalcmdlog)).
- **A job that cannot stop:** Stop (the rail's job card, the palette) drops
  the queued jobs and cancels a stoppable running job, but an `Unstoppable`
  job has no deadline and no way to be stopped. If one hangs, it holds the
  queue, and with it the window's close, until it ends or ChairLift is killed
  (a second SIGTERM exits).
- **Homebrew inventory:** `Brew.RefreshApps` lists formulae and then casks,
  one after the other, under one generation.
- **Large inventories:** no limit caps the size of an inventory, and every
  frame of the Apps page builds the merged, filtered and sorted row list
  (`appsSort(appsFilter(...))`) and the rows of every visible kind
  (`Flatpak.Apps`, `Brew.Apps`), so thousands of packages make each frame
  slower.

## Problems page

Decision record: [ADR-0015](../adr/0015-problems-page-and-opt-in-reporting.md);
the exact contracts are [specs/problems-page.md](../specs/problems-page.md) and
[specs/problem-reporting.md](../specs/problem-reporting.md). The page reads
records on this computer and sends nothing until the user turns reporting on
(phase 2 of plan T857); `internal/reporting` is the only sender.

### Groups and visibility

`internal/navigation` lists `problems` (title "Problems", icon
`dialog-warning`, config page `problems_page`) between Features and
Maintenance, so with every page visible it is Alt+5. Its groups are
`problems_system_group`, `problems_apps_group` and
`problems_reporting_group`, all enabled by default. The page is omitted when
all 3 are off, also after a configuration error (ADR-0003). `Problems`
(`internal/views/problems.go`) reads which groups are on when it is built;
`Problems.Start` runs the first load: `Problems.loadProblems` runs
`problems.Load` on a worker goroutine under a `RefreshGate` generation and
publishes the lists on the UI thread through `Env.Post`; a disabled group's
data is never loaded, and the view (`internal/views/problems_view.go`) draws
only the enabled groups. See "Reporting" below for the reporting card. After
the lists are published the page logs `problems.BuiltMarker`: `problems: page
built system=<n> apps=<n> source=<outcome>` (`source=disabled` without the
system group); with both data groups off `Start` logs it at once with
`system=0 apps=0`. `Problems.Summary` gives the Overview's Problems figure,
the rail badge and the status bar their count (the shown problems) and
attention (the shown system problems).

### Sources (`internal/problems`)

`internal/problems` imports no UI package and no network package
(`TestNoNetworkImports`, `TestNoNetworkDependencies`). `problems.Load` reads:

- `/var/lib/snosi-health/problems.json` (`ReadSystem`, at most 1 MiB) for
  both data groups, because its envelope (`image.id`, `image.version`,
  `secure`) is the image identity of the user items too. Its outcome is 1 of
  4: `SourceMissing` (no file; 1 card, "System checks are not available on
  this system"), `SourceUnreadable` (open or read failed) and
  `SourceMalformed` (bad JSON, `format` not 1, wrong `schema`, no problems
  list, over 1 MiB; both show "System checks could not be read", and
  `Malformed` also records `problems.source_invalid`), and `SourceOK`. The
  system group's text for each outcome comes from
  `pageview.ProblemsSystemStatus`: the card for each of the 3 failures, "No
  problems found" with "Last checked <when>" for an empty `SourceOK`, and no
  card but a "Last checked <when>" note in the section's header
  (`pageview.ProblemsSystemNotes`) when items are shown.
- `/var/lib/snosi-health/details.json` (at most 4 MiB) only for system
  items, and only when readable: EACCES (mode 0640 root:sudo) is normal.
- The user's crashes: `coredumpctl --json=short --no-pager --since=-90d`
  with a 10-second timeout; ChairLift stops reading at 4 MiB of output,
  kills coredumpctl and logs an error. Only the current UID's entries are
  kept; "No coredumps found" (exit 1) and a missing coredumpctl are an empty
  list.
- The S8 app records of the user journal: ChairLift runs journalctl itself
  with std `problems.Read`'s arguments (`problems.JournalArgs`: `--user`,
  the last 90 days, at most 500 entries) and a 10-second timeout, and gives
  the output to std's `problems.ParseJSON`. It does not call std's `Read`,
  which keeps all output in memory: a forged entry with a huge field could
  exhaust memory, because 500 entries do not bound bytes. `readJournal`
  reads at most 8 MiB (`MaxJournalOutput`) and 4 KiB of standard error;
  at the cap it kills journalctl, parses only the complete lines before it
  and logs the truncation. It passes no `--output-fields`: std's `Validate`
  drops a record with an unregistered `FROSTYARD_*` field, and a field list
  would hide that field from it. std drops forged records, and the count
  goes to 1 log line only.

Every detail value of every item is checked again against the v1 rules of
its kind (`validateDetail`); a number or boolean must be that JSON token
type, so `"retries": "1"` fails. A value that fails is shown, marked not valid,
and the item is not reportable; `Reportable` also needs the file's own flag,
a valid `week` and a valid envelope. An unknown kind is shown as "An unknown
problem was recorded" and is never reportable. A system crash that a user
crash repeats (same program and signal, within the system record's time
±1 minute) shows once, in the Apps group (`MergeCrashes`). Each list holds
at most 200 items.

### Presentation and actions

The page shows the problem lists on the left and the reporting card beside
them. Each list has a header (its title, its note, and status notes such as
the last check and the dismissed count from `pageview.ProblemsSystemNotes`
and `pageview.ProblemsAppsNotes`), then "Loading…", the card that stands for
no item (`pageview.ProblemsSystemStatus`/`ProblemsAppsStatus`: "No problems
found", "System checks are not available on this system", "System checks
could not be read"), or one card per problem. A problem's card
(`Problems.problemCard`) is a button that expands it: the kind's icon, the
title and subtitle of `pageview.ProblemCardText` (`problems.Title`, in plain
words, such as "An update could not be prepared" or "Frostmail stopped
unexpectedly", and the subject and when it was last seen), and a count pill
toned by severity. Opened, it shows `pageview.ProblemDetails`: "What it
means" (`problems.Hint`: a hint for each kind, each value of the hinted enums
and each known code; an unknown code gets its class's hint), "Recorded
values", and "On this computer" (details.json values, units, messages, the
program path, and an app record's `MESSAGE`, `_COMM` and unit), then when it
was first seen and its actions. Text is drawn as plain text (MyGo parses no
markup), and the view composes no problem text of its own. The rail badge is
the number of problems shown, in the warning tone.

- **Copy details** (`Problems.copyDetails`) puts `problems.DetailsText(item)`
  on the clipboard through the window (`c.WriteClipboard`). It is local text:
  it can hold names and paths that a report never holds.
- **Dismiss** (`Problems.dismiss`) writes the id to
  `$XDG_STATE_HOME/chairlift/problems-seen.json` (temp file and rename,
  0600, at most 1000 ids) through `problems.Seen.Dismiss` on a worker
  goroutine, then removes the card; see "Dry-run mode" for
  `actionmsg.ProblemDismiss`. A dismissed item shows again when its last
  time is after the dismissal. Dismissals stay off the job queue.
- **Reload problems** (the header's button and the palette) reads the
  problems again; it keeps the consent and the sent log in memory, so a
  reload never overwrites a newer choice of the user.

`problems.BuildPayload` builds the `frostyard.problem/1` document of a
reportable item from validated schema fields only; only
`internal/reporting` calls it, and only after the user opted in. Its tests
validate every payload of the fixtures against a vendored byte copy of core's
`v1.json` (sha256 pinned) with a JSON-schema-subset validator, which lives in
`internal/problems/schemacheck` so the sender's tests use the same one.

`CHAIRLIFT_PROBLEMS_FIXTURES=<absolute folder>` replaces every source, only
with `--dry-run` (`problems.DecideFixtures`); `make e2e` uses it.

### Reporting (`internal/reporting`)

Full contract: [specs/problem-reporting.md](../specs/problem-reporting.md).
`internal/reporting` is a pure leaf package (no UI package) and the only
production importer of `net/http`; `internal/installcheck/reporting_test.go`
fails when another package imports it, or when another non-test Go file names
`report.frostyard.org`.

- **Consent** (`consent.go`, `writer.go`):
  `$XDG_CONFIG_HOME/chairlift/reporting.json` (else
  `$HOME/.config/chairlift/reporting.json`, 0600) records `enabled`, `mode`
  (`ask`/`auto`), `decided_at` and the agreed `schema`. It is off until the
  user turns it on, and every failure reads as off. A schema that adds a kind
  or a field is a new data generation (`dataGeneration`), so sending stops
  until the user agrees again; an enum-only schema keeps it on.
  The page and the first-run step write the small file synchronously on the
  UI thread with 1 in-memory authority: a withdrawal reaches memory before
  its save, a grant only after a successful save, and no queued write can
  restore an older value. `reporting.FileConsent` reads the file again on
  every call, and a live send uses `reporting.LiveConsent` (it reads the file
  first, then memory, and applies `reporting.Stricter`) before every request, so a local Off stops it at once and another ChairLift process
  can only make it stricter. The switch and the mode control stay disabled
  until the first load published its value.
- **Sent log** (`state.go`): `$XDG_STATE_HOME/chairlift/reports.json` (else
  `$HOME/.local/state/chairlift/reports.json`) holds the sent entries, the
  week of the last count and `retry_after`; at most 500 entries, none older
  than 90 days. `LoadState`'s error stops the automatic path: it could not
  enforce the caps.
- **Envelope** (`envelope.go`): `ForItem` builds a report with a new
  `report_id`, and `Prepare` indents the body for the dialog: `Report.Text()`
  is `string(Report.Body)`, so the dialog shows exactly what is sent. Kind
  `crash` is refused with `ErrLocalOnly`. `BuildCount`/`NewCountReport` build
  the weekly count: `install_age` (from `/etc/machine-id`'s modification
  time) and `channel` (`latest`/`other` from the bootc image reference).
- **Sender** (`sender.go`): `POST <endpoint>` with `Content-Type:
  application/json`, no `User-Agent`, a 15 s timeout, at most 8191 bytes of
  body, no redirect followed, 2xx is success and every other answer is a
  `*StatusError`. An empty endpoint is `ErrNoEndpoint` and makes no request:
  only `ParseEndpoint` chooses the default, so an invalid administrator
  setting disables sending instead of reaching the public service.
- **Run** (`run.go`): the one automatic path, in order: group off, consent,
  ask mode, release client, the lock, the sent log, the pause; then the
  pending reports (reportable, not `crash`, not sent this week, at most 20 in
  24 hours), then the count when this ISO week was not counted. 1 send
  reserves (entry or count week plus a conservative pause), saves, reads the
  consent again just before the request, and settles: a success restores the
  earlier pause, a failure keeps it. A withdrawal rolls the reservation back
  and stops without a failure. A process that exits during a request can only
  under-send. `Run` reads the pause once, at its start, so its own
  reservation never blocks the rest of the run. A dry run previews and writes
  nothing. `manual.go`'s `SendReport` is the dialog's send: it ignores the
  pause (the user asked), refuses an unreadable sent log, and shares the caps
  and the lock. Both builders encode without HTML escaping, so enum text such
  as `<1w` stays readable.
- **Endpoint** (`endpoint.go`): `https://report.frostyard.org/v1/report` by
  default; `problems_reporting_group.endpoint` counts only from an
  administrator-owned file (`config.Config.ReportEndpoint`).
- **The windowless mode** (`cli.go`): `chairlift --send-pending-reports`
  runs in `main` before `app.Run`, so it loads no GTK (MyGo loads it only in
  `App.Run`); it exits 1 for a
  configuration error or a failed send, and 0 with 1 line when there is
  nothing to do. It loads the dismiss store the page uses, so both paths
  send the same items.
- **The page** (`internal/views/problems_reporting.go`, drawn by
  `problems_view.go`'s `reportsCard`): a reporting card with the consent
  switch, the Ask/Auto mode (a two-segment control, enabled only while
  reporting is on), the limits, "Send pending reports" in Auto mode, the
  weekly count's Review… row in Ask mode, the "What is sent" link and the
  collapsible list of sent reports; a Report… button on each offered problem
  card (`Problems.reportOffered`), which opens the Report dialog: a modal
  with the report's `Text()` in a monospaced, selectable, scrollable box and
  Cancel and Send; and `reporting.Run` in Auto mode
  (`Problems.runAutomaticReports`). Consent changes are written
  synchronously on the UI thread (`Problems.applyReportingConsent`), and the
  consent and sent log load also when both data groups are off, so a
  consent can always be withdrawn. A new automatic choice starts an
  automatic run, also under `--dry-run`, where it only logs the previews
  (`reporting.StartsAutoRun`). The decisions come from `actionmsg.Report`
  and `actionmsg.ReportingConsent` (ADR-0009). Report sends stay off the job
  queue: `reports.lock` and the pace order them.
- **The first run** shows the optional Reporting step between Style and Apps
  when the reporting group is on and `/var/lib/snosi-health/problems.json`
  exists (`firstrun.ReportingAvailable`); ADR-0014's `Steps`, markers and
  `--mark-done` do not change (see "First-run mode").

### ChairLift's own problems (`internal/selfreport`)

`cmd/chairlift/main.go` sets `version.Version` from the GoReleaser-stamped
`main.buildVersion`, then calls `selfreport.Init(version.Version)` (std
`problems.New("org.frostyard.ChairLift", version, ComponentGUI)` and
`CrashOutput`) and defers `RecoverPanic` directly in `main`. A dev or
snapshot version is not a release semver, so the recorder is inert and
`Init` logs that once. 4 coded errors, constants in `internal/selfreport`
only, are recorded at their paths: `config.load_failed`
(`internal/app/app.go`'s `openMain` and `internal/window/firstrun.go`'s
`NewFirstRun`, on a configuration error),
`problems.source_invalid` (`internal/views/problems.go`, a malformed
problems.json),
`firstrun.complete_failed` (`internal/window/firstrun.go`, the completion
markers could not be written) and `updex.results_unreadable`
(`internal/updex`, the helper exited 0 with unreadable results).
`TestCodesAreRecordedAtTheirPaths` reads the source to prove each site.
The records stay in the user's journal and show in the Apps group.

## First-run mode

Decision records: [ADR-0014](../adr/0014-first-run-mode.md), whose
translation decision
[ADR-0018](../adr/0018-relicense-mit-and-drop-inherited-translations.md)
amends (no translation is shipped). The first-run mode replaces SNOW First Setup (frostyard/first-setup) on images that ship its
trigger. ChairLift itself ships no autostart entry, so on any other
distribution nothing starts on its own.

### Binaries and trigger

- `cmd/chairlift-first-run` (`/usr/bin/chairlift-first-run`, GUI package
  `frostyard-chairlift` only) is pure Go: `internal/installcheck`'s
  `TestFirstRunGateImportsNoGTK` fails when `go list -deps` of it,
  `internal/firstrun` or `internal/i18n` contains a UI toolkit package
  (puregotk, snowkit, MyGo or `github.com/frostyard/frostui`), and proves that
  `internal/window` does depend on MyGo and frostui, so the prefixes still
  match. The gate starts no UI at all, so it exits at once on every later
  login; `main.go` only calls
  `firstrun.Gate`, prints its output and `syscall.Exec`s.
- `firstrun.Gate` outcomes, in order: an unknown argument prints the usage to
  stderr and exits 2; `--help`/`-h` prints the usage to stdout and exits 0;
  UID 0 exits 0 with `first-run: skipped for root` (also for `--mark-done`);
  no config home (neither `XDG_CONFIG_HOME` nor `HOME`) exits 1;
  `--mark-done` calls `firstrun.Complete` and exits 0, or 1 when the write
  fails; an existing marker exits 0 with `first-run: already done`; otherwise
  it execs the fixed path `/usr/bin/chairlift` with argv `chairlift
  --first-run` (plus `--dry-run`).
- snosi ships the trigger on Snow and Snowfield:
  `/etc/xdg/autostart/org.frostyard.ChairLift.FirstRun.desktop` with
  `Exec=/usr/bin/chairlift-first-run`, and the list
  `/usr/share/snow/first-run/apps.json`.

### Application and window

`app.ParseArgs` reads `--first-run` (and `--dry-run`/`-d`) from the command
line, and `app.Run` then uses the application ID
`org.frostyard.ChairLift.FirstRun` (`app.FirstRunAppID`) for
`frostui.SetAppID` and its own single-instance lock; with the main ID a
running ChairLift would take the activation and the window would never show.
`chairlift --help` (`printUsage` in `cmd/chairlift/main.go`) lists
`--first-run`. When MyGo is ready, `openFirstRun` builds
`window.NewFirstRun(poster.post)`, creates a 760×680 window (at least
640×560) with `FirstRun.View` as its content, attaches the poster, and calls
`FirstRun.Presented`, which logs `first-run: window presented`, the readiness
marker that `make e2e` and the lab read (ADR-0008 style).

The window is a MyGo wizard (`internal/window/firstrun.go` for the state and
the flow, `firstrun_view.go` for the drawing): a centered card
(`frostui.Card`: the panel face over a hairline, square, with no shadow)
with a step indicator, the current step's content, and Back and Next under
it. It draws in the kit's Frostyard themes, as the main window does
([frostui.md](./frostui.md#the-frostyard-look)). All its
text goes through `i18n.T`. Its state belongs to the UI thread; the
connectivity check and the Progress worker copy their inputs and hand their
results back through the app's poster (`Window.Update`), which keeps what is
posted before the window exists. The connectivity check starts in the first
frame (`FirstRun.start`, called by `View`), not in `NewFirstRun`.

Toasts sit at the top of the window, over the step indicator, each with a
close button (`topToasts`): MyGo's own toasts sit at the bottom, where they
would cover Back and Next, and have no close button. When `config.Load`
returns an error, `newFirstRun` queues `LoadError.ToastMessage()` as an
error toast with `Timeout: -1`, as the main window does, so it stays until
closed; the toast is plain text. The Apps step is hidden and the user can
still finish.

**Close guard.** `Flow.CanClose` is false only on the Progress step before
`FinishProgress`. `openFirstRun` registers `OnClose` on the window and calls
`PreventDefault` while `FirstRun.CanClose()` is false, so a close — the
window's close button, `mygo.App.Quit`, or SIGTERM — is refused while the
Flatpak actions and the marker write run. After Progress ends, close works
normally, and the Done step's Exit calls `mygo.App.Quit`.

### Steps and flow

`firstrun.Flow` owns the order Welcome, Connectivity, Style, Apps, Progress,
Done and every transition:

- **Welcome** — `Welcome, {}` with `firstrun.WelcomeName` (the GECOS full name's
  first comma field, else the login name, else `$USER`/`$LOGNAME`), Start, and
  Accessibility (`gnome-control-center universal-access`).
- **Connectivity** — status only: `firstrun.CheckConnectivity` dials
  `dl.flathub.org:443` over TCP with a 5 s timeout (`ConnChecking`,
  `ConnOnline`, `ConnOffline`). Next waits while the check runs, because the
  result decides whether Apps shows. Offline shows Check Again and Open
  Settings (`gnome-control-center wifi`); the user can go on.
- **Style** — Light/Dark tiles, each with a sketch of a window in
  `frostui.FrostyardTheme(dark)` with the desktop's preferences minus its
  accent (`schemePreview`). Each choice calls `Request` on the window's
  one `firstrun.SchemeWriter` from the UI thread. The writer has one worker
  goroutine and a mutex-protected pending value: a request during a write
  replaces the pending value, so the latest choice wins and at most the
  running write and the latest one run. `SchemeWriter.Wait` blocks until no
  write is running or pending; the Progress sequence calls it before the
  completion. Each write is
  `firstrun.ApplyColorScheme`: `gsettings set org.gnome.desktop.interface
  color-scheme prefer-light|prefer-dark`, never `gtk-theme`. The window
  shows the choice at once (`forceAppearance`: `mygo.Theme.SetSource` with
  `ThemeLight` or `ThemeDark`, which `frostui.Adapt` turns into Frostyard
  light or Frostyard); the desktop follows when the write lands.
- **Apps** — visible only when the list loaded, `first_run_apps_group` is
  enabled, and the check said online (`Flow.Visible`). One card per
  category with its switch, and under a category that is on, one switch per
  app; no icons and no network fetch.
- **Progress** — Back is not possible; Next waits until every action ended.
  `firstrun.PlanActions` gives no action when no app is selected, else
  `flatpak remote-add --user --if-not-exists flathub
  https://dl.flathub.org/repo/flathub.flatpakrepo` (`flatpak.AddRemote`) and
  then `flatpak install -y --user <id>` per selected app
  (`flatpak.Install(id, true)`). `firstrun.Execute` runs every action, also
  after a failure, and emits Running, then Succeeded or Failed per row.
- **Done** — `Finished!`, the completion text, Exit, and a collapsible
  `Error log`. `firstrun.DoneState` composes it from the failed actions'
  entries (`firstrun.ErrorLog`) and the completion outcome: the warning icon
  and the error log show when an action failed or the completion could not be
  recorded. No Back, no Next.

The settings buttons show only when `gnome-control-center` is on `PATH`
(`firstrun.SettingsAvailable`); they are launches, not state changes, so they
also run in dry run, and they have no config keys (GNOME-only).

### Application list

`firstrun.LoadConfiguredApps` returns `ErrAppsDisabled` when
`first_run_page`/`first_run_apps_group` is disabled — also after a
configuration error, because `disabledConfig()` turns every group off — and
otherwise `firstrun.LoadApps(apps_path)`. `LoadApps`/`ParseApps` accept
first-setup's shape, `{"<category>": [{"name", "id", "active"}]}` with an
absent `active` meaning true, and reject a read error, a file over 1 MiB,
invalid JSON or trailing data, unknown fields, an empty object or category,
an empty name, an ID that is not a valid Flatpak application ID
(`firstrun.ValidFlatpakID`: at least 3 elements, `-` only in the last, at
most 255 bytes) and a duplicate ID. Categories show in the order browsers,
utilities, office, then the other keys sorted; known keys get first-setup's
translated title and subtitle, an unknown key shows the key. Any error hides
Apps and is logged (`first-run: Apps step hidden: ...`). `firstrun.Selection`
holds the switches: a category starts on when at least one of its apps is
active; `Selected` returns the apps whose category and own switch are on, in
list order.

### Markers and completion

The config home is `$XDG_CONFIG_HOME` when it is not empty, else
`$HOME/.config`. `firstrun.IsDone` is true when
`<config home>/chairlift/first-run.done` or first-setup's
`<config home>/snow-first-setup.done` exists. `firstrun.MarkDone` is the only
writer: it creates `<config home>/chairlift` (0700), writes both markers as
empty 0644 files (an existing file is kept), and removes the legacy per-user
`<config home>/autostart/org.frostyard.FirstSetup.autostart.desktop`.
`firstrun.Complete(dryRun, paths)` builds the `firstrun.Completion` decision
and calls `MarkDone` only when `WriteMarkers` is true;
`chairlift-first-run --mark-done` reaches it through `Gate`. The window calls
`firstrun.CompleteAt(dryRun, paths, pathsErr)` after the Progress actions end
(also when some failed) and before Done shows. The outcomes:

- success — `DoneDescription` "This computer is ready.", no
  warning;
- dry run — no write, `[DRY-RUN] would write first-run markers: ...`, the
  dry-run preview text (a missing config home is no failure here);
- a live run with no config home, or a `MarkDone` error — `Err` is set,
  `ShowWarning` is true, the `Log` line is `first-run: could not record
  completion: ...`, and `Completion.ErrorLog` gives one entry, "Setup failed:
  Could not record that the first run is finished" with the error text. There
  is no retry button.

`firstrun.DoneState(actionEntries, completion, gate)` composes the Done step.
`gate` is the gate state after the completion, from
`firstrun.GateStateAt(paths, pathsErr)`, one of 3 values:

- `GateRunsAgain` — the paths are known and no marker exists: the gate
  execs ChairLift at the next login. Only this state gets the description
  "The first run starts again at your next login" after a failure.
- `GateDone` — a marker exists. `MarkDone` can fail after it wrote the
  markers, for example when the legacy autostart path is a non-empty
  directory; the gate then skips the next login, and the description is
  "Setup finished with errors".
- `GateUnknown` — the window has no config home (`pathsErr`). The gate in
  that environment exits 1 and starts nothing, so the description is also
  "Setup finished with errors", with no promise of another run.
  `GateUnknown` is the zero value, so an unset state never promises a run.

The warning icon and the error log show after every completion failure.

`firstrun.RunProgress(actions, steps)` is the Progress worker's whole
sequence, in this order: run every action (`Execute`), drain the
color-scheme writer (`Drain`, the window's `SchemeWriter.Wait`), record the
completion (`Complete`, `CompleteAt`), then read the gate state (`Gate`,
`GateStateAt`) and compose `DoneState`. The window runs it on a worker
goroutine (`FirstRun.startProgress`) and posts `finishProgress` (which ends
the Progress step, so the window may close again) only after it returns, so the last style choice is written before Exit can end the
process; Back is not possible from Progress, so no newer choice can follow
the drain. Wiring tests
in `internal/firstrun` prove both paths reach the one writer, that only
`MarkDone` writes files, and that only `internal/firstrun/markers.go` spells
the marker names.

**Legacy marker end rule.** Reading `snow-first-setup.done` never ends.
Writing it (`writeLegacyMarker` in `internal/firstrun/markers.go`) ends no
earlier than 90 days after the first snosi main image without first-setup
publishes. The change that removes the write records that date and the
ChairLift release that drops it here. Status: the write is active; the end
date is not set yet.

### Translations

No translation is shipped: the first run shows English in every locale.
The catalogs inherited from first-setup were removed with the MIT relicense,
and the msgids' sentences written again in ChairLift's own words
([ADR-0018](../adr/0018-relicense-mit-and-drop-inherited-translations.md));
`internal/i18n/po/README.md` says how to add an MIT-licensed one. The
machinery stays. `internal/i18n` embeds the `internal/i18n/po` directory
(`//go:embed po`, which builds with no `.po` file in it) and reads each
`<name>.po` catalog at its root with its own parser (`ParsePO`:
msgid/msgstr, multi-line strings, C escapes; fuzzy and empty entries
skipped; msgctxt and plurals are errors). `NewTranslator` takes the catalog
filesystem (`fs.FS`): the process translator behind `T` reads the embedded
catalogs, and the unit tests pass `fstest.MapFS` catalogs that they write
themselves. `Candidates` applies
gettext's locale choice: the first non-empty of `LC_ALL`, `LC_MESSAGES`,
`LANG`; English for an empty, `C` or `POSIX` locale (then `LANGUAGE` is
ignored); else each `LANGUAGE` entry, then the locale, each as `ll_CC` then
`ll` after the codeset and modifier are removed, with `zh_CN`/`zh_SG` mapped to
`zh_Hans` and `zh_TW`/`zh_HK`/`zh_MO` to `zh_Hant`. `T` returns the first
catalog's translation, else the msgid; `Format` fills `{}` placeholders. Every
first-run msgid is a constant in `internal/i18n/msgids.go`, and a test fails
when an embedded file holds a msgid outside that list.

## Configuration

Decision records: [ADR-0003](../adr/0003-two-tier-config-with-fail-closed-semantics.md)
(two-tier search order and fail-closed semantics),
[ADR-0004](../adr/0004-configuration-error-diagnostic-vocabulary.md)
(the `CONFIGURATION ERROR` diagnostic vocabulary), and
[ADR-0005](../adr/0005-config-schema-reflected-from-canonical-struct.md)
(the reflected schema and overlay semantics).

### Config file search order

1. `/etc/chairlift/config.yml` — system-wide (highest priority)
2. `/usr/share/chairlift/config.yml` — package-maintainer defaults installed
   by both source `make install` and nFPM packages
3. `config.yml` — beside the executable when present, otherwise relative to
   the current working directory (development fallback)

Only a missing candidate advances the search; a dangling symlink is present,
not missing. The first existing candidate is
authoritative; a read, parse, type, or schema error disables every feature
group and produces both a high-signal log entry and a persistent toast. If no
file is found, all features default to enabled except
`maintenance_cleanup_group`, which defaults to disabled. See
[CONFIG.md](../../CONFIG.md) for the full reference.

Both packaging paths own only the `/usr/share` candidate and may replace it on
upgrade. Neither writes `/etc/chairlift/config.yml`; that higher-precedence
path remains administrator-owned, so local policy is never overwritten by a
ChairLift install or package update.

### Config structure

```yaml
page_name:
  group_name:
    enabled: true/false
    # Optional per-group fields:
    app_id: "..." # External app to launch
    actions: # Custom cleanup scripts (maintenance_cleanup_group)
      - title: "..."
        script: "/path/to/script"
        sudo: true/false
    bundles_paths: [...] # Homebrew bundle directories
    apps_path: "..." # First-run application list (first_run_page only)
    website: "..." # Help dialog links (help_page)
    issues: "..."
    chat: "..."
```

### Key config groups

Every group gates its own card or its own part of a card; the page column is
the configuration page, not always the page of the window that shows it (see
[Pages](#pages)).

| Page                | Group                            | Controls                                                                                                                                                                                                |
| ------------------- | -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `system_page`       | `system_info_group`              | The Overview's System details card: the computer (host name, model, kernel, Secure Boot, uptime, image, build time, digest) and the fields of `/etc/os-release`                                         |
| `system_page`       | `bootc_status_group`             | The Overview's system image hero with the deployment chain (gated on `bootc.IsBootcBootedCached()` or the native A/B marker), and Maintenance's read-only deployment chain                              |
| `system_page`       | `health_group`                   | The Overview's live processor and memory chart (sampled each second) and the system monitor button (configurable `app_id`, default: Mission Center)                                                    |
| `updates_page`      | `bootc_updates_group`            | The system image source on a bootc host — stage via `bootc-update-stage`, apply on restart (gated on `bootc.IsBootcBootedCached()` and stage script availability)                                      |
| `updates_page`      | `sysupdate_updates_group`        | The system image source on a native A/B host — stage via `snosi-sysupdate-stage`, apply on restart, with a read-only previous-version line (gated on `sysupdate.IsNativeABCached()` and stage script availability) |
| `updates_page`      | `firmware_updates_group`         | The firmware source: device firmware through fwupd, one device at a time after a confirmation (gated on `fwupd.IsInstalledCached()`); enabled by default since [ADR-0013](../adr/0013-firmware-updates-are-on-by-default.md) |
| `updates_page`      | `flatpak_updates_group`          | The Flatpak source: pending updates of the user and system installations                                                                                                                               |
| `updates_page`      | `brew_updates_group`             | The Homebrew source's outdated packages, Update all and the `brew update` row                                                                                                                         |
| `updates_page`      | `brew_trust_group`               | The Homebrew source's banners for untrusted taps with installed packages (Homebrew 6 tap trust); shown only when there is something to trust                                                          |
| `applications_page` | `flatpak_user_group`             | The user installation's Flatpak apps in the Apps table, with update, open and uninstall actions                                                                                                        |
| `applications_page` | `flatpak_system_group`           | The system installation's Flatpak apps in the Apps table, with update, open and uninstall actions                                                                                                      |
| `applications_page` | `brew_group`                     | Installed Homebrew formulae/casks in the Apps table with update, uninstall and formula pin/unpin actions, and the Brewfile export                                                                      |
| `applications_page` | `brew_search_group`              | The Apps page's Find more tab: typed Homebrew formula/cask search and confirmed install                                                                                                                |
| `applications_page` | `brew_bundles_group`             | The Apps page's Bundles tab: curated `*.Brewfile` bundles discovered from every configured `bundles_paths` directory, with guarded install actions                                                     |
| `applications_page` | `applications_installed_group`   | The Apps page's button that opens the external software center for discovery/install (configurable `app_id`, default: Bazaar); ChairLift has no direct Flatpak-install UI                              |
| `maintenance_page`  | `maintenance_cleanup_group`      | The configured cleanup scripts (5min timeout unless sudo, pkexec for sudo); **disabled by default**                                                                                                                 |
| `maintenance_page`  | `maintenance_brew_group`         | The Homebrew cleanup row (shown once Homebrew is found)                                                                                                                                                 |
| `maintenance_page`  | `maintenance_flatpak_group`      | The unused Flatpak runtimes row (shown once Flatpak is found)                                                                                                                                           |
| `maintenance_page`  | `maintenance_optimization_group` | Deprecated: renders nothing and is in no navigation item; kept in the schema so files that name it still load                                                                                           |
| `features_page`     | `features_group`                 | The Features page and the Updates page's features source: updex feature toggles and updates                                                                                                            |
| `help_page`         | `help_resources_group`           | The Help dialog's links (website, issues, chat)                                                                                                                                                         |
| `problems_page`     | `problems_system_group`          | Problems page: snosi-health's system problems; "System checks are not available on this system" without `/var/lib/snosi-health/problems.json` ([ADR-0015](../adr/0015-problems-page-and-opt-in-reporting.md)) |
| `problems_page`     | `problems_apps_group`            | Problems page: the user's crashes (coredumpctl) and frostyard app problem records (user journal) |
| `problems_page`     | `problems_reporting_group`       | Problems page: the reporting card (switch, Ask/Auto mode, sent reports, weekly count); `endpoint` counts only from an administrator-owned file ([spec](../specs/problem-reporting.md)) |
| `first_run_page`    | `first_run_apps_group`           | Not a page: the first run's Apps step and its `apps_path` list (default `/usr/share/snow/first-run/apps.json`); disabled, a configuration error, or a missing/invalid list hides the step ([ADR-0014](../adr/0014-first-run-mode.md)) |

## Build and Release

- **Build**: `make build` builds three binaries: `build/chairlift` (main app), `build/chairlift-updex-helper` (privileged helper), and `build/chairlift-first-run` (pure-Go first-run gate), all with `CGO_ENABLED=0`. The window is a MyGo production build (`-tags mygo_noinspector`, `-X github.com/egoist/mygo.production=1`, in the Makefile's `APP_TAGS`/`APP_LDFLAGS` and in `.goreleaser.yaml`): no inspector and no automation plugin. `make build-automation` builds `build/chairlift-automation` (`-tags automation`), a development window that `mygo-automation` can drive and capture (`MYGO_AUTOMATION=<socket>`); it is never shipped.
- **Tests**: `make test` runs `go test ./...`. The views, the window, the kit and the app are tested headless with MyGo's `ui.NewTester`, which renders on the CPU and needs Pango and cairo but no GTK and no display (the CI unit-test jobs install `libpangocairo-1.0-0` and a font). Set `CHAIRLIFT_SNAPSHOT_DIR` to make the snapshot tests write PNGs of the pages.
- **Make gate** ([ADR-0016](../adr/0016-adopt-the-repository-baseline.md); core ADR-0043, ADR-0044): `make verify` runs the non-mutating checks — `go mod tidy -diff`, `go vet`, the gofmt check, `golangci-lint` at the `mise.toml` pin (`make lint-version-check` refuses another release), the template gate (`scripts/check-template.mjs` and `test/check-template.test.mjs`: instruction aliases, the `docs/README.md` index, `.github/risk-paths.json`, `.memory/corrections.jsonl`, vendored hashes) — and then the unit tests (`./internal/...` under `-run "^Test[^I]" -skip "Integration"`). `make check` formats first and then runs `verify`. `mise install` provisions the pinned golangci-lint and Node (22 or later is required); Go comes from go.mod.
- **CI mirror**: `make ci` runs `make verify`, then the corrections check against `BASE` when set (`node scripts/check-corrections.mjs --base`), the race detector, and the build, in fail-fast order; the Verify job of `.github/workflows/ci.yml` runs it with the PR base. Its build step reproduces CI's `linux/amd64` + `linux/arm64` matrix into `build/ci-linux-<arch>/` before rebuilding natively, so a compile failure on the non-host architecture cannot pass locally. Codecov's remote project status additionally rejects coverage regressions greater than one percentage point, with no fixed project or patch target; it cannot be mirrored locally. The runtime-dependent E2E job is deliberately separate: `make e2e` builds the three binaries, executes the application's `--help` path (which must list `--first-run`), boots the dry-run window under a private D-Bus/Xvfb session (MyGo draws with Mesa's llvmpipe there) with `CHAIRLIFT_PROBLEMS_FIXTURES` set to the fixtures of `test/e2e/testdata/problems/`, polls all four readiness markers (`Running in dry-run mode`, `ChairLift activated`, `app: window presented`, and `problems: page built system=16 apps=7 source=ok`) for at most 30 seconds, requires one second of post-readiness stability, then terminates its private process group, stages the real `make install` layout under a temporary `DESTDIR` (which must hold an executable `usr/bin/chairlift-first-run` and no autostart path), and executes the staged `chairlift-updex-helper` rejection paths. For the first run it also checks `chairlift-first-run --help`, runs `--mark-done` and the gate against temporary homes (as `nobody` when the test runs as root), and boots `chairlift --first-run --dry-run` until it logs `first-run: window presented`, then checks that no marker exists. Its Go test package lives at `test/e2e`, imports no UI package, and is enforced by that explicit target rather than the `./internal/...` unit-test filter. The readiness markers are a log-line contract — decision record [ADR-0008](../adr/0008-e2e-readiness-is-a-log-marker-contract.md).
- **Dev build**: `make dev` builds with `CGO_ENABLED=1` and `-race` flag for race detection
- **Version**: Set via ldflags by goreleaser (`buildVersion`, `buildCommit`, `buildDate`, `buildBy`)
- **Semantic versioning**: Uses [svu](https://github.com/caarlos0/svu) via `make bump`
- **CI**: GitHub Actions workflows for CI (`ci.yml`, workflow name `Tests`), snapshot, release, and PR triage (`risk-paths.yml`, workflow name `PR triage`, which runs core's `pr-triage.yml` to add the level label `level:L1` to `level:L4` from `.github/risk-paths.json` at the base, and `risk:high` for L1 and L2, and to remove `ready-for-human` when the head moves after the label; core ADR-0077, ADR-0078) (`.github/workflows/`); per [ADR-0034](../org-adrs.md), snapshot publishers use the repository-scoped `goreleaser-nightly` concurrency group with in-progress cancellation so only the newest tested `main` commit publishes to the rolling `dev` release and concurrent GoReleaser uploads cannot collide. Every external `uses:` reference in every workflow is pinned to a full 40-character commit SHA (with its version or source ref retained as a comment); `internal/installcheck.TestWorkflowActionsUseImmutableCommitSHAs` inventories both `.yml` and `.yaml` workflow files and rejects mutable tags, branches, short SHAs, and expressions while allowing repository-local `./` actions.
- **Release**: GoReleaser config at `.goreleaser.yaml`. Its `metadata.homepage` is the single source of truth for the repository URL and is consumed by `release.footer`, whose "Full Changelog" link is templated from `{{ .Metadata.Homepage }}` rather than a hardcoded owner; two static tests guard that pairing — see the "Install-path consistency (`internal/installcheck`)" section of [package-managers.md](./package-managers.md#install-path-consistency-internalinstallcheck)
- **Debian publication**: a tag push runs `.github/workflows/release.yml`. GoReleaser creates the GitHub release with the archives, `checksums.txt` and the deb/rpm/apk packages. `actions/attest-build-provenance` then attests all of them, and the final step sends a `publish-deb` `repository_dispatch` with the repository and tag to `frostyard/apt-publisher`, authenticated by `APT_PUBLISH_TOKEN`. apt-publisher is the only writer of `https://repository.frostyard.org/debian/` ([frostyard/core ADR-0055](../org-adrs.md)). It downloads every `.deb` asset of the tag, verifies its provenance against `refs/tags/<tag>`, and publishes it to the codenames registered for chairlift in its `config/producers.tsv`. After apt can download the new packages through the public URL, it dispatches `build` to the image repositories registered there (ADR-0056). The request is not `continue-on-error`, so a failed request fails the release. The rolling `dev` prerelease from `snapshot.yml` is never published to the APT repository and dispatches nothing. `internal/installcheck/release_workflow_test.go` pins the exact release and snapshot step sequences, the request's inputs, the workflow permissions and attestation subjects, and that the only `.deb` packages GoReleaser builds are the two names that must be registered with apt-publisher.
- **Other targets**: `make fmt` (gofmt), `make lint` (golangci-lint at the `mise.toml` pin), `make install`/`make uninstall` (system install including the three binaries, polkit policies, icons, and wrapper script, and no autostart entry; default `PREFIX=/usr`, the only prefix that matches where polkit reads policy files and the updex helper's fixed pkexec exec-path annotation — see "Privileged operations" above), `make build-linux-amd64`/`make build-linux-arm64` (cross-compilation)

### Runtime dependencies

- GTK 3, libepoxy with an OpenGL driver (Mesa), and Pango with cairo: shared libraries that MyGo loads at run time with purego, in
  `App.Run` only, so no development headers or CGO are needed to build. MyGo
  draws with OpenGL on the GPU and falls back to its CPU renderer when OpenGL
  would not run on a GPU or its GPU renderer fails (`MYGO_GPU=0` forces the CPU
  renderer). The `frostyard-chairlift` `.deb` declares them
  (`.goreleaser.yaml` `overrides.deb.dependencies`: `libgtk-3-0t64 |
  libgtk-3-0`, `libpangocairo-1.0-0`, `libepoxy0`); the `.rpm` and `.apk`
  and the system-integration package declare no UI libraries, and
  `internal/installcheck.TestGoreleaserDebDeclaresUIDependencies` pins that.
- `pkexec` and a running PolicyKit authentication agent are needed for
  privileged actions. Both nFPM packages depend on `pkexec` for deb,
  `/usr/bin/pkexec` for rpm, and `polkit` for apk. A desktop session must
  supply its own authentication agent.
- Homebrew (optional)
- Flatpak (optional)
- fwupd / `fwupdmgr` (optional; the firmware-updates group is enabled by default and its source stays out of the Updates page unless `fwupdmgr` is present)
- `bootc` + `/usr/libexec/bootc-update-stage` (both optional; the image source and the hero's deployments are gated on `bootc.IsBootcBootedCached()`, i.e. `bootc status` reporting a non-null `booted` deployment — not on any sentinel file)
- `/usr/lib/snosi/native-ab` marker + `/usr/libexec/snosi-sysupdate-stage` (both optional, shipped by native A/B OS images; gated on `sysupdate.IsNativeABCached()` and `sysupdate.StageScriptAvailable()`)
- Updex features configured on the system (optional; read via Go library, writes via `chairlift-updex-helper`)
- GIO (`libgio-2.0.so.0`, part of GLib, which GTK 3 itself requires) for the system monitor, the software center and a Flatpak app's Open: `frostui.LaunchDesktopApp` loads it with purego and launches the desktop entry in process, so no launcher program is needed
- Problems page: `/var/lib/snosi-health/problems.json` and `details.json` (optional, written by snosi-health), `coredumpctl` (optional), and `journalctl` (the user journal)
- First run only: `gsettings` (the Style step), `flatpak` (the Apps step), and `gnome-control-center` (optional; the settings buttons show only when it is on `PATH`)

### Key external Go dependencies

| Module                           | Purpose                                                                                     |
| -------------------------------- | ------------------------------------------------------------------------------------------- |
| `github.com/egoist/mygo`         | The window: MyGo's native UI (`mygo/ui`) on GTK 3 and OpenGL, the headless `ui.Tester`; kept behind frostui and the views ([ADR-0017](../adr/0017-mygo-command-center-window.md)) |
| `github.com/frostyard/frostui`   | The UI kit on MyGo: shell, palette, console, job queue, dialogs, cards, and the Frostyard themes the window draws in; `frostuitest` for the tests' UI thread ([frostui.md](./frostui.md)) |
| `github.com/ebitengine/purego`   | Calling C without CGO: MyGo's library loading, `frostui.SetAppID`'s `g_set_prgname`, and `frostui.LaunchDesktopApp`'s GIO launch |
| `github.com/frostyard/updex/v2`  | Updex Go library for feature reads and helper binary (currently pinned to v2.2.0 in go.mod) |
| `gopkg.in/yaml.v3`               | YAML config parsing                                                                         |
| `golang.org/x/text`              | Title-casing OS release info keys                                                           |
| `github.com/frostyard/std`       | `problems`: ChairLift's own problem records and the journal reader of the Problems page    |

The GTK 4 bindings (`codeberg.org/puregotk/puregotk`) and `github.com/frostyard/snowkit` are gone since ADR-0017; `internal/installcheck.TestNoGTK4Bindings` fails if either comes back, because GTK 3 and GTK 4 cannot share a process.

There is no separate Go client library dependency for bootc: status/stage types (`Status`, `Deployment`, `ProgressEvent`, etc.) are defined locally in `internal/bootc`, parsed directly from `bootc status --format json` and the stage script's line output.

## Subsystem Details

- [Package Manager Wrappers](./package-managers.md) — Homebrew (including tap trust), Flatpak, fwupd, bootc, sysupdate and Updex wrapper details, and the pure view-layer leaf packages
- [frostui](./frostui.md) — where the UI kit lives now, what ChairLift draws itself, and how it draws in the Frostyard look
- [Engineering lessons](./engineering-lessons.md) — binding rules learned from defects and reviews
