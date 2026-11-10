# 0017 — Draw ChairLift as a command center with MyGo

- **Status:** Accepted
- **Date:** 2026-11-10

## Context

ChairLift drew its window with GTK 4 and libadwaita through puregotk: seven
settings-style pages of preference rows. Two spikes (CUI through purego, then
MyGo) redrew it as a "command center": a top bar with a command palette, a
left rail with counts, one Overview of the whole machine, an Updates page
that runs every source as one pipeline, a console that shows every command
ChairLift runs, and a status bar. The owner chose MyGo
(`github.com/egoist/mygo`), a Go desktop framework that draws its own
widgets on GTK 3 and OpenGL, loaded at run time with purego:

- It is plain Go with no CGO and no bindings to maintain, and
  `Window.Update` is safe from any goroutine.
- It exposes a real AT-SPI tree on Linux (screen readers), which CUI lacks.
- It follows the desktop's accent, light or dark mode, text size and
  contrast, and the desktop's icon theme can be drawn on top.
- `ui.NewTester` renders a view headless in a unit test, with clicks, keys
  and an image of the frame, so the window's packages can be tested, which
  puregotk's init-time library loading never allowed
  ([ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md)).

Its costs: it is young (repository created 2026-09-26, one main author), its
binary is about 10 MB, it needs GTK 3, libepoxy and Pango at run time
instead of GTK 4 and libadwaita, and on Wayland it takes the app ID from the
program name. GTK 3 and GTK 4 cannot share a process.

## Decision

- **MyGo replaces GTK 4 everywhere**, the first-run window included. The
  puregotk and snowkit dependencies are removed;
  `internal/installcheck.TestNoGTK4Bindings` keeps them out.
- **The window is the command center**: six pages in the rail (Overview,
  Updates, Apps, Features, Problems, Maintenance; `internal/navigation`),
  Help as a dialog (F1 and the main menu), a command palette (Ctrl K), a
  console (Ctrl J) and a status bar. The configuration schema is unchanged:
  `system_page` feeds the Overview, `help_page` the Help dialog, and every
  group still gates its own card.
- **The console shows real output.** The backends report every command
  they start, its output lines and its end through `internal/cmdlog`; the
  window streams them, the log and dry-run previews into the console.
- **Changes run through one serial job queue** (`frostui.Jobs`): one
  mutation at a time, its output in the console, its progress in the rail
  and the status bar. Reads run concurrently.
- **The UI kit is frostyard's, not ChairLift's.** `internal/frostui` holds
  the shell, cards, dialogs, palette, console, job queue, pipeline and
  gauges, imports only MyGo, purego and the standard library
  (`TestFrostUIImportsNoChairLift`), and is written to move to its own
  module for frostmail, frostnotes and frostread
  ([design/frostui.md](../design/frostui.md)).
- **Views are tested.** Packages that import MyGo may hold tests: they use
  `ui.NewTester`, which needs Pango and cairo but no display. Pure leaf
  packages (`pageview`, `actionmsg`, `actionstate` and the rest) stay where
  the decisions are; ADR-0007's rule that window packages hold no tests no
  longer applies.
- **The app ID is set before the display opens**: `frostui.SetAppID` calls
  GLib's `g_set_prgname("org.frostyard.ChairLift")` (or
  `org.frostyard.ChairLift.FirstRun`), so the desktop matches the window to
  the installed `.desktop` file without renaming the binary.
- **Release builds are MyGo production builds** (`-tags mygo_noinspector`,
  `-X github.com/egoist/mygo.production=1`): no inspector, no automation
  plugin. `make build-automation` builds a development window that
  `mygo-automation` can drive and capture.
- **Only real data.** The mockups' invented content stays out: no 30-day
  service history, no "where the disk goes" map, no new privileged actions
  (container prune, journal vacuum, rollback, pinning) and no reboot
  button. Two small additions have real sources: live processor and memory
  use from `/proc` on the Overview (behind `health_group`), and ChairLift's
  own record of what it changed (`internal/activity`, under
  `$XDG_STATE_HOME/chairlift/activity.jsonl`, never written in dry run).

## Consequences

- The privilege boundary ([ADR-0001](0001-fixed-path-pkexec-privilege-boundary.md))
  is unchanged; the palette lists only actions the pages offer, through the
  same fixed paths.
- Runtime dependencies change from GTK 4 and libadwaita to GTK 3, libepoxy
  (OpenGL, with a CPU fallback) and Pango. The E2E job installs them, and the
  unit-test jobs install Pango and cairo for the Tester.
- Every UI state change happens on the UI thread; workers hand results back
  with `Env.Post` (`Window.Update`). `Window.Update` rebuilds the view on
  every call, so console output wakes the window at most every 60 ms.
- The E2E readiness markers ([ADR-0008](0008-e2e-readiness-is-a-log-marker-contract.md))
  and the first-run marker contract ([ADR-0014](0014-first-run-mode.md)) keep
  their exact text.
- MyGo is a young dependency with one main author. ChairLift pins it in
  go.mod, keeps the kit between its pages and MyGo, and would replace the
  kit's MyGo calls, not the pages', if it had to move.
- KDE (Sundog) is not yet tested.

## Alternatives considered

- **CUI through purego:** smaller (1.1 MB) and proven in a spike, but it has
  no accessibility tree, needs a mirrored C ABI and a string arena, and has
  no thread-safe wake.
- **Keep GTK 4 and restyle it:** libadwaita's preference rows and its
  untestable puregotk init are what the redesign set out to leave.
- **Two processes, a Go core and a separate front end:** every type twice
  and a protocol to own, for no gain once MyGo was pure Go.

## References

- Shapes: [design/overview.md](../design/overview.md),
  [design/frostui.md](../design/frostui.md)
- Builds on: [ADR-0001](0001-fixed-path-pkexec-privilege-boundary.md),
  [ADR-0003](0003-two-tier-config-with-fail-closed-semantics.md),
  [ADR-0008](0008-e2e-readiness-is-a-log-marker-contract.md),
  [ADR-0009](0009-dry-run-output-convention-and-single-decision-structs.md),
  [ADR-0014](0014-first-run-mode.md),
  [ADR-0015](0015-problems-page-and-opt-in-reporting.md)
- Supersedes: [ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md)
