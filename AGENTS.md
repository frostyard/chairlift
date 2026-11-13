# AGENTS

ChairLift is the command center of
[Snow Linux](https://github.com/frostyard/snosi): a system-management GUI
written in idiomatic Go on [MyGo](https://mygo.egoist.dev)
(`github.com/egoist/mygo`), which draws its own widgets on GTK 3 and OpenGL
loaded at run time with purego — **no CGO**
([ADR-0017](docs/adr/0017-mygo-command-center-window.md)). The window is a
command center: a top bar with a command palette, a left rail of six pages
(Overview, Updates, Apps, Features, Problems, Maintenance), a console that
streams every command ChairLift runs, and a status bar. The UI is
YAML-configuration-driven; every card sits behind its configuration group.

## Build, test, lint

The app builds pure-Go (`CGO_ENABLED=0`); the race detector needs CGO.

- `make build` — builds `build/chairlift`, `build/chairlift-updex-helper`
  and `build/chairlift-first-run` (all `CGO_ENABLED=0`). The window is a MyGo
  production build (`-tags mygo_noinspector`, `-X
  github.com/egoist/mygo.production=1`): no inspector, no automation plugin.
  `make build-automation` builds `build/chairlift-automation`, which
  `mygo-automation` can drive and capture (`MYGO_AUTOMATION=<socket>`).
- `make test` — `go test ./...`. Views are tested headless with MyGo's
  `ui.NewTester`, which needs Pango and cairo (no GTK, no display); set
  `CHAIRLIFT_SNAPSHOT_DIR` to write PNGs of the pages from the snapshot
  tests.
- `make fmt` — `gofmt -s -w .`.
- `make lint` — `golangci-lint run`, after `make lint-version-check` proves
  the installed golangci-lint is the `mise.toml` pin.
- `mise install` — provisions the pinned golangci-lint and Node (`mise.toml`,
  checksums in `mise.lock`; core ADR-0043). Go comes from go.mod's `go` line,
  the only Go pin. Bump a tool in `mise.toml` and `mise.lock` only.
- `make verify` — the credential-free gate that leaves `git status` unchanged
  (core ADR-0044, [ADR-0016](docs/adr/0016-adopt-the-repository-baseline.md)):
  `go mod tidy -diff`, `go vet`, the gofmt check, the pinned lint, the
  template gate (`make verify-template`: `scripts/check-template.mjs` and
  `test/check-template.test.mjs`, Node 22 or later), then the unit tests.
  The template gate checks the instruction aliases, that every doc under
  `docs/` is linked from `docs/README.md`, `.github/risk-paths.json`,
  `.memory/corrections.jsonl`, and the hashes of the 2 files vendored from
  core. The `scripts/` files and `test/check-template.test.mjs` are
  byte-identical copies from `bketelsen/agentic-template`: copy them again,
  never edit them here.
- `make check` — `make fmt`, then `make verify`.
- `make ci` — runs `make verify`, then the corrections check against `BASE`
  when it is set (`make ci BASE=origin/main`; `make corrections-base` fails
  when `BASE` does not resolve to a commit, and skips only a base that
  predates `scripts/check-corrections.mjs`), the race detector and the
  build: every **host-independent** CI gate, in fail-fast order.
  The build step reproduces CI's `linux/amd64` + `linux/arm64` matrix into
  `build/ci-linux-<arch>/`, then rebuilds natively, so a cross-arch-only
  compile failure cannot pass locally and break CI. Run it before pushing;
  the Verify job of `.github/workflows/ci.yml` runs it. Codecov's remote project status
  additionally rejects coverage regressions greater than one percentage point;
  it has no fixed coverage target and cannot be mirrored locally.
- `make e2e` — builds the three executables, checks the application's real
  `--help` surface (including `--first-run`), starts the dry-run window
  under a private D-Bus/Xvfb session, stages `make install` (which must hold
  `usr/bin/chairlift-first-run` and no autostart path), and executes the
  installed privileged helper's rejection paths. It also checks
  `chairlift-first-run --help`, runs `--mark-done` and the gate against
  temporary homes (as `nobody` when the test runs as root), and starts
  `chairlift --first-run --dry-run` until it logs `first-run: window
  presented`, with no marker written. Main-window startup runs with
  `CHAIRLIFT_PROBLEMS_FIXTURES` set to `test/e2e/testdata/problems/` and
  polls the four readiness log markers (the fourth is `problems: page built
  system=16 apps=7 source=ok`) for up to 30 seconds, requires one additional second of process stability, and
  terminates the private process group as soon as the smoke check passes.
  Before it stops either window, it reads the X server's windows with
  `xwininfo -root -tree` and requires one whose `WM_CLASS` instance is the
  app ID (`org.frostyard.ChairLift`, or `org.frostyard.ChairLift.FirstRun`):
  GTK names it, and the Wayland app ID, after GLib's program name, and GNOME
  matches it with the desktop entry, so a window named after the executable
  gets a generic icon. It
  requires GTK 3, libepoxy and Mesa (MyGo draws with llvmpipe under Xvfb),
  Pango, `dbus-run-session`, `xvfb-run`, and `xwininfo` (x11-utils); the hosted E2E job installs those
  runtime dependencies explicitly because ordinary unit-test hosts carry only
  Pango and cairo for the Tester.
- `make install`'s default `PREFIX` is `/usr` — the only prefix under which
  the installed PolicyKit policy files land where `polkitd` reads them
  (`/usr/share/polkit-1/actions`) and the updex helper's installed
  path matches its fixed `pkexec` exec-path annotation (see the privilege
  boundary invariant below). It installs maintainer defaults at
  `/usr/share/chairlift/config.yml` and must never install or overwrite the
  administrator-owned `/etc/chairlift/config.yml`. GoReleaser publishes both
  the self-contained `frostyard-chairlift` package and the mutually exclusive
  `frostyard-chairlift-system-integration` companion for user-scoped GUI
  installs; every nFPM entry carrying policies must retain the same fixed
  paths. Both packages depend on the system's pkexec provider; a desktop
  session must supply a PolicyKit authentication agent.

Every Makefile target is a command, not a file, so every target is declared
`.PHONY`. Otherwise make compares the target with the file system: the
top-level `test/` directory once made `make test`, and therefore `make bump`,
print `'test' is up to date` and run no tests. When adding a target, add it
to `.PHONY`; `internal/installcheck/makefile_phony_test.go` fails when a
target is missing from `.PHONY` or `.PHONY` names a target that does not
exist.

CI (`.github/workflows/ci.yml`, workflow name `Tests`) filters tests with `-run "^Test[^I]"
-skip "Integration"`. That filter excludes *any* test whose name begins `TestI`
— not only `TestIntegration` — or contains `Integration` anywhere. Those names
are reserved for tests that require a real environment (a live `brew`,
`flatpak`, `bootc`, or GTK display). Ordinary unit tests must not use the
`TestI` prefix: a test that trips the filter is never executed by `make ci` or
by CI and therefore protects nothing. The accident is easy to make, because
plain unit-test names such as `TestIsValid`, `TestInitConfig`, or `TestIndexOf`
all start with `TestI` and would be silently skipped; name them so the first
letter after `Test` is not `I` (see
[engineering lessons](docs/design/engineering-lessons.md#no-tests-in-puregotk-packages)).
`internal/installcheck.TestGateFilterHidesNoHermeticTest` now fails when the
filter hides a hermetic test or when a test function sits outside `./internal/...`.
Its walk skips what the go tool's `./...` pattern skips — directories starting
with `.` or `_`, `testdata`, and nested modules — so agent worktrees under
`.claude/worktrees/` do not count as hidden tests.

The separately invoked tests under `test/e2e/` are outside the
`./internal/...` unit-test scope by design. They are enforced by the E2E
workflow's explicit `make e2e` step; do not assume adding a test outside
`internal/` is enough without that dedicated gate.

There are no generated files and no codegen step; everything under version
control is hand-written Go, YAML, and data assets.

## Releases and dependencies

- **Tags.** Release tags are `vMAJOR.MINOR.PATCH` (`git tag -l 'v*'` lists
  them). `.svu.yml` sets `tag.prefix: "v"`, `v0: true` and `always: true`.
- **Version rule.** `make bump` builds, tests, formats and lints, refuses a
  dirty tree, then tags `$(svu next)` with an annotated tag and pushes it to
  `origin`. [svu](https://github.com/caarlos0/svu) derives the next version
  from the commit messages since the last tag (Conventional Commits). `v0: true`
  keeps the major version at 0.
- **Who tags.** Brian. Agents never run `make bump`, create or push a tag, or
  cut a release.
- **What a tag does.** A tag push runs `.github/workflows/release.yml`:
  GoReleaser Pro (`release --clean`), build-provenance attestation, then a
  `publish-deb` dispatch to `frostyard/apt-publisher`. See the APT-publishing
  invariant above. `snapshot.yml` only refreshes the rolling `dev` prerelease.
- **External UI dependency:** `github.com/egoist/mygo` (direct, the window;
  pinned in go.mod). It is young and has one main author: keep its calls
  behind `github.com/frostyard/frostui` and the views, and read its source in
  `GOMODCACHE` after a bump (the theme reset per frame, `Window.Update`
  semantics, the Tester). `github.com/frostyard/frostui` (direct, the UI kit
  on MyGo; pinned in go.mod) requires its own MyGo version, so bump the two
  together. Until its v1 a minor version may change its API: after a bump,
  read the `API changes` section of its `docs/design/frostui.md`.
- **Internal dependencies** (`go.mod`): `github.com/frostyard/updex/v2`
  (direct, feature SDK and the helper's engine) and `github.com/frostyard/std`
  (direct since ADR-0015: its `problems` package writes ChairLift's own
  problem records and reads the user journal's records; updex also uses its
  `reporter`). Take versions from `go.mod`, never from
  prose. Other frostyard pins: the `frostyard/apt-publisher` dispatch in
  `release.yml`, and `docs/org-adrs.md` for org ADRs in frostyard/core. After
  an updex bump, read the SDK source in `GOMODCACHE` for changed behavior
  (the operation lock, image gates) before merging. After a std bump, read
  `problems` in `GOMODCACHE` (the reader's validation, and `Read`'s
  journalctl arguments, which `problems.JournalArgs` copies) and check that updex's `reporter` use still builds.
- **Consumers.** Snow Linux images (`frostyard/snosi`) install the `.deb` from
  `https://repository.frostyard.org/debian/` (README.md); apt-publisher
  dispatches the image rebuilds. The Homebrew cask (`frostyard/homebrew-tap`,
  `Casks/chairlift.rb`) installs from the release archive and names files in
  it by path, so a change to the archive's contents needs a matching cask
  change. It runs on the host's libraries, so its users need GTK 3, Pango and
  libepoxy, which the deb declares and the cask cannot.
- **Debian codenames.** apt-publisher publishes the 1 unmarked build of each
  `.deb` to every codename it registers for chairlift: `trixie` and `forky`
  ([core Plan 0009](https://github.com/frostyard/core/blob/main/docs/plans/0009-debian-publication-through-apt-publisher.md),
  Phase 6). This works because the binaries are static and the deb's
  `Depends` name packages that both codenames ship
  (`TestGoreleaserDebDeclaresUIDependencies` pins the UI libraries, and
  `TestGoreleaserPackagesDependOnPkexec` checks `pkexec`). Before a change to
  those `Depends` merges, install the package and start its dry-run window on
  each codename; a name that differs between them needs `~deb13` and `~deb14`
  builds instead.

## Repository invariants

An agent must not break these:

- **Privilege boundary.** State-changing operations that require root go
  through `pkexec` (PolicyKit) with fixed, installed polkit policies and fixed
  helper binaries only: `pkexec /usr/libexec/bootc-update-stage` (action
  `org.frostyard.ChairLift.bootc.stage`), `pkexec
  /usr/libexec/snosi-sysupdate-stage` (`internal/sysupdate.StageScriptPath`,
  action `org.frostyard.ChairLift.sysupdate.stage`, native A/B hosts), and
  `pkexec /usr/bin/chairlift-updex-helper` (`internal/updex.HelperPath`, actions
  `org.frostyard.ChairLift.updex.{enable-feature,disable-feature,update}`) —
  always that fixed absolute path, matching the
  `org.freedesktop.policykit.exec.path` annotation, with the updex subcommand
  matching `org.freedesktop.policykit.exec.argv1`. The helper must strictly
  reject unsupported argv because PolicyKit does not validate arguments after
  action selection. ChairLift ships no passwordless PolicyKit rules; normal
  administrator authentication applies. Homebrew tap trust (`brew trust`) is
  deliberately per-user and does **not** use pkexec. Firmware installs are the
  other unprivileged mutation: they call
  `fwupdmgr update --no-reboot-check <DeviceId>` directly and fwupd's own
  PolicyKit actions authorize the daemon
  ([ADR-0012](docs/adr/0012-firmware-updates-use-fwupds-own-polkit.md)), so
  never route them through pkexec or add a helper or policy for them. Do not
  add arbitrary privileged command execution, broaden what pkexec runs, or
  route new mutations around the fixed helper/policy pair. The 1 existing
  exception: a maintenance action with `sudo: true` runs `pkexec <script>`
  with the script path from the configuration
  (`pageview.MaintenanceCommand`), under PolicyKit's default `pkexec` action
  and its administrator authentication (see "Known limits" in
  `docs/design/overview.md`). Do not widen it.
- **First-run mode stays user-level, pure at its gate, and opt-in per image**
  ([ADR-0014](docs/adr/0014-first-run-mode.md)). `cmd/chairlift-first-run`
  imports no UI toolkit (no MyGo, no frostui, no GTK binding;
  guarded by `internal/installcheck.TestFirstRunGateImportsNoGTK`) and only
  calls `internal/firstrun.Gate`; it ships in `frostyard-chairlift` only.
  `chairlift --first-run` uses the application ID
  `org.frostyard.ChairLift.FirstRun` (set with `frostui.SetAppID` and its own
  single-instance lock) and the wizard in `internal/window/firstrun*.go`; the
  six pages and `internal/navigation` do not know it. While its actions run,
  the app vetoes the window's close (`FirstRun.CanClose`). `firstrun.MarkDone` is the only writer of the markers
  (`chairlift/first-run.done` and, until ADR-0014's dated end rule, the legacy
  `snow-first-setup.done`), reached only through `firstrun.Complete`; reading
  the legacy marker never ends. The first run uses no pkexec and no sudo, and
  ChairLift ships no autostart entry: the image owns the trigger. The
  `first-run: window presented` log line is a contract. First-run text goes
  through `internal/i18n`; a new msgid goes into `internal/i18n/msgids.go`.
  No `.po` catalog ships
  ([ADR-0018](docs/adr/0018-relicense-mit-and-drop-inherited-translations.md)):
  ChairLift is MIT, so a catalog added later is contributed under MIT and
  copies nothing from First Setup or another GPL-licensed source. The msgids'
  sentences are ChairLift's own (`TestNoMsgidCopiesFirstSetup`).
- **OS staging execution has one owner.** `internal/stageexec` is the pure-Go
  leaf package that owns the progress event contract, merged stdout/stderr
  streaming, direct-child cancellation, error classification, completion event,
  and channel closure for both `internal/bootc` and `internal/sysupdate`.
  Provider packages retain their fixed paths, host detection, dry-run logging,
  and public error adapters; do not copy the process loop back into either one.
- **System-integration split.** The
  `frostyard-chairlift-system-integration` nFPM package contains the fixed-path
  updex helper, all three PolicyKit policies, and package-maintainer config,
  but not the GUI or an OS staging implementation. Distributions pairing it
  with a user-scoped ChairLift install must provide their trusted stage helper
  at `/usr/libexec/bootc-update-stage` before enabling `bootc_updates_group`;
  native A/B hosts ship `/usr/libexec/snosi-sysupdate-stage` (and the
  `/usr/lib/snosi/native-ab` marker) with the OS image, which
  `sysupdate_updates_group` requires. Do not make the privileged path
  configurable from ChairLift's user-writable configuration.
- **UI-thread safety.** Views and every state struct in `internal/views`
  and `internal/window` belong to the UI thread. All external tool calls run
  in goroutines that copy their inputs first, never read state, and hand
  results back with `Env.Post` (MyGo's `Window.Update`, through the app's
  `poster`, which keeps posts made before the window exists). So do desktop
  app launches (`Env.launchApp`): `frostui.LaunchDesktopApp`'s GIO lookup
  and launch are synchronous and may block. The monitor lookup uses MyGo's
  menu-visible desktop entries and their localized names, selecting the first
  installed `health_group.app_id` candidate. Console output
  wakes the window through `frostui.Waker` (at most every 60 ms), because
  `Window.Update` rebuilds the view on every call. The live sampler buffers
  at most one minute of samples behind a mutex; only the UI thread applies
  them to chart history. It posts for each sample while Overview is selected,
  otherwise only for a displayed disk-space change, with at most one post
  pending. Navigation publishes the buffered history when Overview returns.
- **Changes run in the serial job queue.** In the command center, every
  mutation a page starts (install, update, stage, enable, clean, trust, flash)
  is a `frostui.Job` on `Env.Jobs`: one runs at a time, `Run` on a goroutine,
  `Done` on the UI thread. Only problem dismissals and report sends stay off
  the queue (ADR-0015's own lock and pacing); the first-run wizard keeps
  ADR-0014's own sequence of steps. Pages ask first through `Env.Dialogs`. A
  job whose work a cancel cannot stop (pkexec, firmware, updex) is
  `Unstoppable` and has no deadline: Stop drops the jobs queued after it and
  lets it finish, and a timeout would kill only the client and free the
  queue while the work goes on. For the same reason the main window refuses
  to close while such a job runs (`Main.CanClose`, wired in `openMain`
  through `attachMain`) and a toast says why: MyGo fires `OnClose` for the
  close button and for `App.Quit`, which SIGINT and SIGTERM call, so one
  guard covers them. A stoppable job, or an `Unstoppable` one that only
  waits in the queue, does not hold the window.
- **Every command shows in the console.** The backends announce each
  external command and its end through `internal/cmdlog`
  (`Start`/`End`, `DryRun` for previews) and tee its output when it is
  human-readable (`Stdout`/`Stderr`); machine output such as journal records
  and lsblk's JSON is not teed. A new `exec.Command` under `internal/` must do
  the same (`internal/installcheck.TestEveryCommandIsTraced` parses each
  file and counts `cmdlog.Start` calls per function, closures included). The
  frostui module is outside that walk, and its `LaunchDesktopApp` runs no
  command: it launches the desktop entry in process through GIO. The views
  report each launch with `cmdlog.Launch` (`launchDesktopApp`), which the
  console shows as an in-process launch, not as a command, and the same
  test requires a `cmdlog.Launch` beside every `frostui.LaunchDesktopApp`
  call. The rail's job card and the palette stop the queue
  (`Jobs.CancelAll`); a job that finished despite the cancel keeps its result.
- **The UI kit is a module.** ChairLift depends on
  `github.com/frostyard/frostui` (version in go.mod) and holds no copy of it;
  never add one back under `internal/`. The module guards its own imports. Put
  ChairLift-specific UI in `internal/views` or `internal/window`, and propose
  a view helper that is not ChairLift-specific to the module. See
  [docs/design/frostui.md](docs/design/frostui.md).
- **The window draws in the kit's Frostyard look.** `frostui.Adapt` sets
  Frostyard or Frostyard light each frame, without the desktop's accent.
  Views take corners, text sizes and surfaces from the kit (its metrics,
  text styles, `frostui.TextSize`, `frostui.ColorsOf`), never a literal
  radius or font size, and draw no shadow
  (`internal/installcheck.TestViewsDrawInKitMetrics`). See
  [the Frostyard look](docs/design/frostui.md#the-frostyard-look).
- **Views are tested headless.** `internal/views`, `internal/window` and
  `internal/app` hold tests that render with `ui.NewTester`; backends are injected as function fields, so no test runs
  brew, flatpak, bootc, fwupdmgr, updex or pkexec. A test that stands in for
  the UI thread posts to a `frostuitest.Loop` (`github.com/frostyard/frostui/frostuitest`). Decisions still live in the
  pure leaf packages (`pageview`, `actionmsg`, `actionstate`, `bundleview`,
  `featurestatus`, `flatpakstatus`, `trustmsg`). No package may load a UI
  library at init (`TestNoGTK4Bindings` keeps puregotk out).
- **The Problems page is local until the user opts in, and checks every
  value** ([ADR-0015](docs/adr/0015-problems-page-and-opt-in-reporting.md),
  [page spec](docs/specs/problems-page.md),
  [reporting spec](docs/specs/problem-reporting.md)). Its data lives in the pure
  `internal/problems` package, which imports no UI package and no network
  package (`net`, `net/...`, `crypto/tls`; guarded by
  `TestNoNetworkImports` and `TestNoNetworkDependencies`). The snosi-health
  paths (`/var/lib/snosi-health/problems.json`, `details.json`) are fixed
  constants, never configuration. ChairLift re-validates every detail value
  against `frostyard.problem/1` and never trusts the file's `reportable` flag
  alone; details.json text, `MESSAGE`, paths and trusted journald fields are
  local display text and never enter `BuildPayload`. The live journal read
  is capped (`readJournal`, 8 MiB, kill at the cap); do not use std's
  unbounded `problems.Read`. A number or boolean must be that JSON token
  type, never JSON text. The vendored
  `internal/problems/testdata/schema/v1.json` is a pinned byte copy of core's
  schema: a new schema version is a new copy with new enums and hints, never
  an edit. Every enum value of the hinted fields and every known code needs a
  hint. Dismiss goes through `actionmsg.ProblemDismiss` (dry run writes and
  hides nothing). `CHAIRLIFT_PROBLEMS_FIXTURES` is honored only with
  `--dry-run`, and `problems: page built ...` is a log contract (ADR-0008).
  ChairLift's own problem records go through `internal/selfreport`: its code
  constants are the only spelling of each code, each code has 1 tested call
  site (`TestCodesAreRecordedAtTheirPaths`), and `main` defers
  `RecoverPanic` after `selfreport.Init(version.Version)`.
- **`internal/reporting` is the only sender.** Nothing is sent until the
  per-user consent (off by default, `reporting.Consent`) allows it for the
  current schema data generation, and in Ask mode only the Report dialog
  sends. The dialog shows `Report.Text()` and posts the same `Report.Body`
  bytes. `internal/reporting` is the only production importer of `net/http`
  and the only non-test Go file that names `report.frostyard.org`
  (`internal/installcheck/reporting_test.go`); `internal/problems` must not
  import it. One lock (`reports.lock`) keeps the GUI and `chairlift
  --send-pending-reports` apart; the budget is 20 reports in 24 hours, 1
  weekly count, and the first failure pauses automatic sending for 24 hours.
  A kind `crash` report is never sent in phase 2: the sender refuses it
  before any network call and the page offers no Report button for it. The
  endpoint comes from `problems_reporting_group.endpoint` only in an
  administrator-owned file (`config.Config.ReportEndpoint`); the
  user-writable development fallback must never choose where a report goes.
  `--send-pending-reports` runs before `app.Run` and loads no GTK (MyGo loads
  it only in `App.Run`). The
  first-run Reporting step is optional, keeps ADR-0014's `Steps` and markers
  unchanged, and is guarded by `internal/firstrun` tests in new files.
- **Navigation behavior has one authority.** Page order, titles, icons, and
  the advertised and bound shortcuts live in the pure `internal/navigation`
  package. It also decides page visibility from static group configuration:
  omit a page when all of its groups are disabled, always keep the Overview,
  and compact Alt+number over visible pages. Help is a dialog (F1), not a
  page, and `help_page` configures its links. The rail, the palette's "Go
  to" commands, the pages' links (`Env.Navigate`) and the shortcuts all go
  through `Main.Navigate`, which applies `navigation.Resolve`. `Main.View`
  calls `Palette.RunPending` first, before it takes the toasts or captures
  the page, so a palette command shows in the frame that runs it. The shortcuts
  dialog lists `navigation.Shortcuts` of the same visible inventory, and
  `TestEveryAdvertisedShortcutWorks` presses every one. Do not reintroduce a
  second page or shortcut inventory in `internal/window` or `internal/app`.
- **Homebrew update actions preserve known state.** Per-package upgrades and
  the top-level metadata update use `internal/views/actionstate` gates before
  spawning work. Failures and dry-run previews restore their controls without
  changing rows or counts. A live package success removes its row, decrements
  the count/badge, and refreshes; a failed refresh preserves that last known
  row/count state instead of replacing it with an invented zero.
- **Flatpak update reloads are generation-guarded and preserve known
  state.** `Flatpak.loadUpdates` (`internal/views/flatpak.go`) takes an `actionstate.RefreshGate` generation
  on the main thread and every publication checks it, so a stale reload
  never replaces newer rows or counts. A load is usable when at least one
  installation succeeds or returns one or more updates with an error. A load
  in which no installation returned a usable result
  (`flatpakstatus.Result.Authoritative == false`) keeps the
  last known rows and count through `actionstate.OutdatedRefresh` and only
  reports that the check failed. Because such a load keeps the old rows, the
  per-app update button never waits for a reload to replace it: a failure or
  dry-run restores it, and a live success removes its row and decrements the
  Flatpak count before the reload. A live uninstall removes the app's update
  row too and, when an update check is in flight, starts a newer one, so the
  older check (begun before the uninstall) cannot list the app again.
- **Homebrew application actions are typed and refresh-safe.** Search queries
  both formula and cask namespaces and carries the result kind into
  `brew install [--cask]`. Search and installed-package refreshes use separate
  `actionstate.RefreshGate` generations; stale workers must not replace newer
  rows. Confirmed installs use an `actionstate.Gate`, restore controls on
  failure/dry-run, and refresh installed rows only after a live success.
  Installed formula/cask rows likewise confirm uninstall, formula rows confirm
  pin/unpin, and every row shares one gate across its mutation controls so
  actions cannot overlap. A live success completes the old controls and starts
  a generation-guarded inventory refresh; failure or dry-run restores them.
  A refresh whose list of that kind failed keeps the completed controls
  (`brewKeepRows`); only a list that loaded resets them.
- **Update counts have one owner per source.** Each update source
  (`views.UpdateSource`: system image, features, Flatpak, Homebrew, firmware)
  owns its items; the Updates badge, the Overview figure and the status bar
  read `Store.UpdateCount()`, the sum over the sources that apply, held items
  included. A live success removes its item before any reload; a failed
  reload keeps the last known items. Features retire only `ClassUpdated`
  outcomes of a successful live run; a failed or dry run retires nothing.
  Their completion markers survive failed checks and reset only on a
  successful current-generation check. Do not keep a separate counter.
- **Config-driven visibility is real.** Any group can be disabled in config
  (`config.IsGroupEnabled(page, group)`), so its widgets may never be
  constructed. Code that runs after an async action must not assume a widget
  from another group exists — nil-guard cross-group widget access. In
  particular, `brew_bundles_group` is independent of `brew_group`; bundle
  discovery and installs must not assume the formulae/casks expanders exist.
- **Configuration precedence fails closed.** Only a missing candidate advances
  to the next configuration search path. A dangling symlink is present, not
  missing (`Load` checks it with `Lstat`), so it is authoritative and fails
  closed. The first file that exists is authoritative: read, YAML, or schema errors must disable every configurable
  group, emit the `CONFIGURATION ERROR` diagnostic, and remain visible in the
  UI as a persistent toast until the file is fixed and ChairLift is restarted.
- **CI actions are immutable.** Every external `uses:` reference under
  `.github/workflows/` must use a full 40-character commit SHA. Keep the
  human-readable version or source ref in a trailing comment and update both
  intentionally. Local actions referenced with `./` are exempt. The
  `internal/installcheck` workflow scan enforces this across every workflow.
- **Debian packages publish only through frostyard/apt-publisher.**
  `.github/workflows/release.yml` runs on tag pushes. GoReleaser creates the
  GitHub release, then `actions/attest-build-provenance` attests
  `checksums.txt`, the archives and every package. The last step sends a
  `publish-deb` `repository_dispatch` to `frostyard/apt-publisher` with the
  repository and tag, authenticated by `APT_PUBLISH_TOKEN`. That step has no
  guard and is not `continue-on-error`
  ([frostyard/core ADR-0055](https://github.com/frostyard/core/blob/main/docs/adr/0055-publish-debian-packages-through-the-apt-publisher.md)).
  apt-publisher verifies each `.deb` against this tag's provenance and
  publishes it. It then dispatches `build` to the image repositories
  registered for chairlift
  ([ADR-0056](https://github.com/frostyard/core/blob/main/docs/adr/0056-rebuild-images-after-apt-publication.md)).
  No chairlift workflow holds signing or R2 credentials or dispatches `build`
  itself, and `snapshot.yml` publishes only the rolling `dev` prerelease, never
  to the APT repository. apt-publisher refuses a release containing a `.deb`
  whose package name is not registered for `frostyard/chairlift` in its
  `config/producers.tsv`, so register a new deb-producing `nfpms` entry there
  first. `internal/installcheck/release_workflow_test.go` pins this contract.

## Documentation

All documentation lives in the `docs/` tree, in frostyard/core's
four-category shape (core ADR-0025; the former `yeti/` AI-docs directory is
folded in). `docs/README.md` carries the category table, the index of every
doc, and the conventions — new docs start from their category's
`TEMPLATE.md`, and adding a doc means indexing it there:

- `docs/adr/` — why: repo-local decisions, immutable once accepted. Org-wide
  decisions go to frostyard/core instead, per
  [docs/org-adrs.md](docs/org-adrs.md).
- `docs/design/` — how it fits together: living architecture docs. The entry
  point is `docs/design/overview.md` (formerly `yeti/OVERVIEW.md`); read it
  and `docs/design/package-managers.md` (formerly `yeti/package-managers.md`)
  for architecture, patterns, and decision rationale before working. Write
  them to be maximally useful to an AI agent understanding the codebase —
  detailed architecture and rationale rather than user-facing guides.
- `docs/specs/` — exact contracts, changed only alongside implementing code.
- `docs/plans/` — phased plans with "Done when" outcomes.

After any change to source code, update relevant documentation in `AGENTS.md`,
`README.md`, and `docs/`. A task is not complete without reviewing
and updating relevant documentation. For behavior, configuration, dependency,
or install-layout changes, also follow
`docs/documentation-consistency.md`; current-state claims must be checked
against source/config/go.mod rather than copied from historical plans.

**.knowledge/ directory** is the repository's cross-session knowledge index.
Read `.knowledge/README.md` before working so prior corrections, handoffs,
durable lessons, and architecture guidance are discovered from their canonical
locations instead of duplicated into competing stores.

**.memory/ directory** is the repository's committed correction store for AI
agents. Read `.memory/README.md` and any learning artifacts in that directory
before working. Record verified corrections there when a session establishes
that a prior belief about ChairLift was wrong, and promote stable rules into
this file, `docs/design/engineering-lessons.md`, or `docs/design/` as appropriate. Never record
secrets or personal data because the directory is version-controlled.

## Learned lessons and skills

- **Lessons.** Read
  [docs/design/engineering-lessons.md](docs/design/engineering-lessons.md)
  before planning, implementing, or reviewing changes. Its rules (no test file
  in a puregotk package, gates that run only `internal/` tests, tag-aware YAML
  key identity, a check for every collection entry, and others) are binding
  guidance, not suggestions. Code comments link its anchors; keep them stable.
  Add a lesson there when a defect or review shows a rule that a test cannot
  enforce; record the correction itself in `.memory/corrections.jsonl`.
- **Skills.** `.agents/skills/` holds the skills that frostyard/core syncs
  (core ADR-0026; each has a `.synced-from-core` marker: edit it in core, not
  here) and `TEMPLATE/` for a repo-local skill. `.claude/skills` is a symlink
  to it. `.agents` stays a real directory: core's sync follows an `.agents`
  symlink to its target. A sync pull request whose branch predates a skills
  layout change writes to the old path, so update it from `main` and check
  CI again before the merge.
  `internal/installcheck.TestSyncedSkillsLiveInAgentsSkills` fails when
  `.agents` is a symlink or a `.synced-from-core` marker lies outside
  `.agents/skills/<name>/`.
- **Working rules.** Run focused tests while you iterate and `make ci` before
  you push. Review `git status` and `git diff` for unrelated or generated
  files. When a review names a class of defect, fix every instance of the
  class, not only the one it quotes. Report exactly which checks ran; when a
  check cannot run, give the command and the reason. A fact carried from an
  old document or a research report becomes current-state text only after
  you read every call site it describes (`git grep` the function); a
  plausible claim about where a value comes from is not evidence.

## Security and risk paths

- Report vulnerabilities as `SECURITY.md` says (GitHub private vulnerability
  reporting).
- `.github/risk-paths.json` lists the high-risk paths (core ADR-0068, amended
  by ADR-0078): schema version 2, where each entry is a glob with a level, and
  `default_level` (L3) covers a path that no glob matches. The governance files
  (`Makefile`, workflows, the template gate, the risk list) are L2; the tool
  pins, `.svu.yml`, `.goreleaser.yaml`, the PolicyKit policies
  (`data/*.policy`), the installed wrapper script and the root helper
  (`cmd/chairlift-updex-helper/`, `internal/updexhelper/`) are L1. The
  `risk-paths.yml` caller, workflow name `PR triage`, runs core's
  `pr-triage.yml` on every pull request (core ADR-0077, ADR-0078). It adds one
  level label `level:L1` to `level:L4`, from the list at the base, and
  `risk:high` for L1 and L2. It also removes `ready-for-human` when the head
  moves after the label. Keep the list in step when a release, install or
  privileged path is added.
- The PR template's `## Risk tier` section takes the tier from
  [docs/risk-tiers.md](docs/risk-tiers.md).

## Org-wide decisions

Org-level conventions this repo follows are recorded as ADRs in
frostyard/core — see [docs/org-adrs.md](docs/org-adrs.md) for the list that
binds this repo. Change the ADR (in core) before changing behavior it covers.
