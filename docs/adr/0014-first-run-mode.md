# 0014 — Run the first-login setup as a ChairLift first-run mode

- **Status:** Accepted; its decision 8 (translations) is amended by [0018](0018-relicense-mit-and-drop-inherited-translations.md) (the ported First Setup catalogs are deleted and no translation ships; the `.po` machinery stays)
- **Date:** 2026-11-04

## Context

SNOW First Setup (frostyard/first-setup, the `snow-first-setup` deb) is a
Python/GTK app that Snow and Snowfield start at each graphical login through
`/etc/xdg/autostart/org.frostyard.FirstSetup.autostart.desktop`. In its
first-login mode it greets the user, checks the network, sets the color
scheme, installs user Flatpaks from a curated list, and writes
`${XDG_CONFIG_HOME:-~/.config}/snow-first-setup.done` so it never runs again.
Plan T843 retires it and moves the flow into ChairLift.

Facts that constrain the move:

- puregotk dlopens libgtk-4 during package initialization, and
  `internal/app` registers its GType in `init`. A check inside
  `cmd/chairlift` runs only after GTK has loaded, so a done user would pay a
  GTK start at every login.
- `internal/app` uses one application ID with `G_APPLICATION_FLAGS_NONE`. A
  second `chairlift` process with the same ID only activates the running one:
  if the user has ChairLift open, a first-run window in that process would
  never show.
- Hosts that ran first-setup have `snow-first-setup.done`. A bootc rollback
  can boot an older image that still ships first-setup.
- ChairLift has users on other distributions. It must not start anything on
  its own there.
- first-setup has 51 gettext translations; ChairLift had none.

## Decision

1. **A separate pure-Go gate binary.** `cmd/chairlift-first-run`
   (`/usr/bin/chairlift-first-run`, built `CGO_ENABLED=0`) imports only the
   standard library and the puregotk-free packages `internal/firstrun`,
   `internal/i18n`, `internal/config` and `internal/flatpak`. Its logic is
   `firstrun.Gate`: an unknown argument exits 2; `--help` exits 0; UID 0
   exits 0 (the first run and its markers are per-user); an existing marker
   exits 0; otherwise it execs the fixed path `/usr/bin/chairlift
   --first-run` (plus `--dry-run` when given). Each skip logs one
   `first-run: ...` line to stderr for the user journal. The binary ships
   only in the GUI package `frostyard-chairlift`, never in
   `frostyard-chairlift-system-integration` (ADR-0006).
2. **A separate application ID.** `chairlift --first-run` (detected in
   `os.Args` before the GObject exists, and registered as a GApplication
   option) uses `org.frostyard.ChairLift.FirstRun`, so an open ChairLift does
   not swallow it. On activate it builds `internal/window`'s first-run window,
   not the main window. The six pages and `internal/navigation` do not
   change.
3. **The marker contract.** The config home is `$XDG_CONFIG_HOME` when it is
   not empty, else `$HOME/.config` (first-setup's rule). ChairLift's marker is
   `<config home>/chairlift/first-run.done`. ChairLift counts the run as done
   when that marker **or** first-setup's `<config home>/snow-first-setup.done`
   exists. One function, `firstrun.MarkDone`, writes the markers: it creates
   `<config home>/chairlift` (0700), writes both markers as empty 0644 files
   (idempotent), and removes the legacy per-user copy
   `<config home>/autostart/org.frostyard.FirstSetup.autostart.desktop`. The
   Done step and `chairlift-first-run --mark-done` (for the lab, no GTK) both
   reach it through `firstrun.Complete` (the window through
   `firstrun.CompleteAt`, which also carries a missing config home). The
   window calls it after the Progress actions end, also when some failed,
   and before the Done step shows, as first-setup did. A completion that
   cannot be recorded (no config home, or a `MarkDone` error) is not hidden:
   `firstrun.Completion` carries `Err` and `ShowWarning`, and the Done step
   shows the warning icon and an error-log entry ("Setup failed: Could not
   record that the first run is finished" and the error text). The text
   follows the real gate state after the failure (`firstrun.GateStateAt`):
   "The first run starts again at your next login" only for
   `GateRunsAgain` (the paths are known and no marker exists). For
   `GateDone` (`MarkDone` wrote a marker before a later step failed, so the
   gate skips the next login) and `GateUnknown` (no config home, so the gate
   exits 1 and starts nothing) the text is "Setup finished with errors".
   There is no retry button.
4. **The Progress step cannot be cut short.** While the Progress actions run
   (`Flow.CanClose` is false), the window blocks `close-request` and holds
   the GApplication, so the Flatpak actions and the marker write finish. The
   hold is released and close works again when Progress ends.
   `firstrun.RunProgress` fixes the order on the worker goroutine: the
   actions, then a drain of the color-scheme writer, then the completion, so
   the last style choice is written before Exit can end the process.
5. **The legacy marker end rule.** ChairLift always **reads**
   `snow-first-setup.done`; that never ends. During the transition it also
   **writes** it (the `writeLegacyMarker` constant in
   `internal/firstrun/markers.go`), so a rollback to a first-setup image does
   not run first-setup again. The write ends no earlier than 90 days after
   the first snosi main image without first-setup publishes. The change that
   removes the write records that date and the ChairLift release that drops
   it in `docs/design/overview.md` ("First-run mode").
6. **No autostart in the deb.** ChairLift ships no autostart entry and no
   first-run `.desktop` file. The image owns the trigger: snosi ships
   `/etc/xdg/autostart/org.frostyard.ChairLift.FirstRun.desktop` with
   `Exec=/usr/bin/chairlift-first-run` on Snow and Snowfield (snosi
   ADR-0028). On any other distribution nothing starts on its own.
7. **The `first_run_page` config.** A non-navigation config page with one
   group, `first_run_apps_group` (`enabled: true`, `apps_path:
   /usr/share/snow/first-run/apps.json`, image data from snosi in
   first-setup's JSON shape). A disabled group, a configuration error
   (ADR-0003 fail-closed), or a missing or invalid list hides the Apps step;
   the user can still finish. A configuration error also stays on screen as
   a persistent plain-text toast (`LoadError.ToastMessage`, timeout 0), as in
   the main window.
8. **Translations through embedded `.po` files.** First-run strings go
   through `internal/i18n`: a small pure-Go `.po` parser over files embedded
   with `go:embed`, gettext's locale choice (`LC_ALL`, `LC_MESSAGES`, `LANG`,
   then `LANGUAGE`), no `msgfmt` step, no generated file, no new dependency.
   Unchanged first-setup strings keep their msgids and translations (ported
   from first-setup commit `b5f7de3`); new strings start untranslated.
9. **Offline users can finish.** The connectivity step is status only (a 5 s
   TCP dial to `dl.flathub.org:443`). Offline, the Apps step is skipped and
   the user can install applications later from ChairLift.
10. **Dropped first-setup functions.** The live-session check (no live
   desktop exists any more); the legacy `gtk-theme` write (libadwaita ignores
   it; only `color-scheme` is written); the `~/.bashrc` Homebrew line (moves
   to snosi as `/etc/profile.d/snosi-brew.sh`); the world-writable
   `/tmp/first-setup.log` (a symlink risk; ChairLift logs to its journal);
   the hidden rerun launcher; the script override hook; network icon
   fetches. The settings buttons run the fixed commands
   `gnome-control-center universal-access` and `gnome-control-center wifi`
   and show only when `gnome-control-center` is on `PATH`; they have no
   config keys, because the first run is GNOME-only (snosi runs it only on
   GNOME images).
11. **Dry run.** Under `--dry-run` the first run writes no marker
    (`[DRY-RUN] would write first-run markers: ...`), runs no gsettings, and
    the Flatpak wrapper skips `remote-add` and `install`. The completion is
    one decision struct, `firstrun.Completion` (ADR-0009).
12. **The color scheme is written serially.** One `firstrun.SchemeWriter`
    writes the Style step's choices one at a time, and the latest choice
    wins, so quick toggles cannot leave the scheme the user did not choose
    last.

## Consequences

- A done user's login costs one small static process and no GTK start.
- The first run is user-level only: no pkexec, no sudo, no PolicyKit change
  (ADR-0001). A guard test fails on `pkexec` or `sudo` in a first-run file.
- ChairLift now owns a marker contract that snosi and the lab rely on;
  renaming a marker or the `first-run: window presented` log line is a
  contract change (ADR-0008 style).
- The legacy marker write is a dated obligation: someone must remove it
  after the end rule is met, and record the date and release.
- Changing a ported English string loses its translations; new strings need
  translators. The rest of ChairLift is still English-only.
- Images that want a first run must ship the autostart entry and the apps
  list; ChairLift alone shows no first run.

## Alternatives considered

- **Gate inside `cmd/chairlift`:** rejected — puregotk loads GTK in package
  `init`, before any gate code runs.
- **One application ID:** rejected — a running ChairLift would receive the
  activation and the wizard would never show.
- **Autostart entry in the ChairLift deb:** rejected — every user of every
  distribution would get the first run.
- **A systemd user unit as the trigger:** rejected — XDG autostart is what
  GNOME runs at session start, needs no preset, and is what first-setup used.
- **A hard connectivity gate (first-setup's behavior):** rejected — an
  offline user would meet the wizard at every login and never finish.
- **A gettext library or `msgfmt` build step:** rejected — a third-party
  dependency or generated `.mo` files for about 40 strings; the embedded
  `.po` reader is small and tested.
- **Config keys for the settings commands:** rejected — the first run is
  GNOME-only, and a configurable command is a new execution surface for no
  user.

## References

- Shapes: [design/overview.md — First-run mode](../design/overview.md#first-run-mode),
  [design/package-managers.md — Flatpak](../design/package-managers.md),
  [CONFIG.md](../../CONFIG.md), `internal/i18n/po/README.md`
- Builds on: [ADR-0001](0001-fixed-path-pkexec-privilege-boundary.md),
  [ADR-0003](0003-two-tier-config-with-fail-closed-semantics.md),
  [ADR-0006](0006-split-system-integration-package-with-mutual-conflicts.md),
  [ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md),
  [ADR-0008](0008-e2e-readiness-is-a-log-marker-contract.md),
  [ADR-0009](0009-dry-run-output-convention-and-single-decision-structs.md)
- Enforced by: `internal/firstrun` tests (gate, markers, flow, apps list,
  actions, completion, wiring), `internal/i18n` tests,
  `internal/config/firstrun_config_test.go`,
  `internal/installcheck/firstrun_test.go`, and the first-run tests in
  `test/e2e/e2e_test.go`
- Plan: T843 (chairlift takes over first-setup); snosi ADR-0028 (the
  trigger and the apps list)
