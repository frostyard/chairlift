# frostui: frostyard's desktop UI kit

Living design document for `internal/frostui`, the kit ChairLift's window is
built from ([ADR-0017](../adr/0017-mygo-command-center-window.md)). It is
written to leave ChairLift: frostmail, frostnotes and frostread can share it
once it is a module of its own.

## What it is

A thin layer over MyGo's native UI package (`github.com/egoist/mygo/ui`)
that gives frostyard's desktop apps one look and one set of behaviors:

- **The command-center shell** (`shell.go`): a top bar (brand, a field
  that opens the command palette, the app's buttons, a main menu), a left
  rail of pages with counts and a footer, the page area (scrolled, padded,
  at most 1320 DIPs wide), an optional console split under it, and a status
  bar of clickable items. `Shell.View(c, Frame{...})` draws it every frame;
  the `Frame` says what goes where.
- **Command palette** (`palette.go`): `Command{ID, Title, Group, Icon,
  Keys, Hint, Disabled, Reason, Run}`, fuzzy ranked by `Match`/`Filter`
  (ordered subsequence; word starts, runs and a match at the start count
  more), Up/Down/Enter/Esc.
- **Console** (`console.go`, `consoleview.go`): a thread-safe ring buffer of
  lines with levels (debug, output, info, command, dry run, warn, error),
  filters by level and text, follow, copy and clear. `Console.Writer`
  adapts it as a `log` output with a per-line classifier.
- **Job queue** (`jobs.go`): `Jobs` runs `Job`s one at a time, in order.
  `Run` works on a goroutine with a cancelable context and a `Progress`
  (fraction, status, console lines); `Done` runs on the UI thread. It
  counts batches ("Job 3 of 10"), finds the job of an item (`Find`,
  `Pending`), cancels, recovers panics and logs each job's start and end.
- **Dialogs** (`dialogs.go`): `Dialogs.Ask(Confirm{...})` queues
  confirmations shown one at a time (destructive actions in the danger
  color, a note for administrator authentication); `Info` for content
  dialogs; `ShortcutsBody` and `AboutBody` build the usual two.
- **Building blocks** (`components.go`, `text.go`): `Card`, `CardHeader`,
  `PageHeader`, `Section`, `Figures` (a strip of clickable figures),
  `Pill`, `IconPill`, `IconTile`, `IconButton`, `LabeledButton`, `Banner`
  (a toned notice with an action and a close), `Empty`, `Loading`,
  `Problem`, `KeyValues`, `RowDivider`, `Kbd`/`Shortcut`, and the text
  styles `Title`, `Heading`, `Strong`, `Body`, `Muted`, `Caption`, `Mono`,
  `Toned`.
- **Visualizations** (`viz.go`, `pipeline.go`): `LiveChart` (0–100 series,
  smoothed), `Legend`, `Gauge` (an open ring), `SegmentBar`, `ToggleTile`
  (a large on/off tile), `Pipeline` (stages of steps with states, one
  update run across sources), `JobCard` (the rail's running job) and
  `StateOf` (a job's state as a step's).
- **Desktop integration** (`theme.go`, `icons.go`, `appid.go`): `Adapt`
  keeps the accent in inactive windows as GNOME and KDE do (MyGo fades it to
  gray, as macOS does) and must run first in every frame, since MyGo resets
  the theme each build pass; `Icons` finds freedesktop icons in the
  desktop's theme (Adwaita on GNOME, Breeze on KDE, falling back to Adwaita
  and hicolor) lazily and caches them; `SetAppID` sets GLib's program name
  before MyGo opens the display, so the Wayland app ID and the X11
  `WM_CLASS` match the app's `.desktop` file.

The look follows the desktop, not a palette of its own: MyGo reads the
accent, light or dark mode, contrast and text size from the desktop portal,
and frostui adds only metrics (`SpaceXS`…`SpaceXL`, `RadiusCard`,
`RadiusChip`, `RailWidth`, `TopBarHeight`), tones (`Neutral`, `Accent`,
`Success`, `Warning`, `Danger`) mapped onto the theme, and two derived
surfaces (`Chrome` for the bars and rail, `Panel` for cards). This keeps
high contrast and large text working; frostyard's web design system
(`frostyard/design-system`) stays the source for marketing and web apps.

## Rules

- **Imports.** frostui imports the standard library, MyGo and purego only.
  `internal/installcheck.TestFrostUIImportsNoChairLift` enforces it, tests
  included.
- **Threads.** Views and every model belong to the UI thread, except
  `Console.Add`/`Addf`/`Writer` and `Progress.Set`/`Log`, which any
  goroutine may call. Apps hand work back with MyGo's `Window.Update`; since
  it rebuilds the view on every call, wake the window from a burst of
  console lines through a coalescing waker (ChairLift's `window.Waker`).
- **State outside the view.** `ListState`, `ScrollState`, dialog and
  palette state, and parsed SVGs live in structs (`Shell` holds the
  palette, console view, dialogs and info dialog), never in a view
  function.
- **Lists need a height.** A `ui.List` or `ui.Table` inside an unsized
  column gets almost none; size it or its column. Rows that hold controls
  must not set `ListState.Label` (the Tester then targets the row, not the
  control).
- **Clickable custom rows are buttons**: `ui.ButtonBase(c)`, for the
  keyboard and screen readers.

## Testing

Every part has headless tests in `frostui_test.go` with `ui.NewTester`
(MyGo renders on the CPU; it needs Pango and cairo, no display): palette
ranking and running, console filters and ring buffer, the job queue's
order, serialization, cancellation and panic recovery, dialogs in order, and
the shell's rail, status bar and console. `Tester.Image()` gives a PNG of a
frame; ChairLift's tests write them when `CHAIRLIFT_SNAPSHOT_DIR` is set.

## Moving it out

Target: `github.com/frostyard/frostui`, tagged like frostyard's other
libraries, with ChairLift requiring it.

1. Copy `internal/frostui` to the new repository at its root; keep the tests
   and add a small gallery app (every component, light and dark, written
   with `Tester.Image()` into `docs/`).
2. Rename nothing: the package name is already `frostui`.
3. In ChairLift, replace the import path, delete `internal/frostui`, and
   keep `TestFrostUIImportsNoChairLift`'s intent in the new repository as an
   import allow-list test.
4. Generalize the two ChairLift-flavored strings: the console pane's
   subtitle ("every command ChairLift runs, as it runs") becomes a field of
   `ConsoleView`, and the status bar's console item label stays.

What stays in ChairLift: `internal/cmdlog` (the backends' command tracer;
it could move too, as a separate tiny package, once another app runs
commands), `internal/sysinfo` (host facts), `internal/activity`
(ChairLift's own record) and everything under `internal/views`.

### Candidates found while porting the pages

The page ports wrote these as unexported helpers first; they belong in the
kit once a second app needs them:

- `Table` presets: a sortable, filterable table with an inspector panel
  beside it (Apps), and the "filter field + kind segments" toolbar.
- An expandable problem card (title, subtitle, count pill, details, actions)
  — a general "disclosure card".
- A two-column page layout (main column and a fixed-width side column).
- A launcher for a desktop app by its ID (`gtk-launch <id>`), used by the
  Overview, Apps and first run.
- A centered wizard frame with a step indicator and Back/Next (first run).

When a frostyard app on MyGo starts (frostmail, frostnotes, frostread), it
should begin from the shell: rail of mailboxes or notebooks, the palette
for commands, the console as an activity log of sync jobs, and the job
queue for long operations.
