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
  counts batches ("Job 3 of 10"), lists the batch's jobs in queue order
  (`BatchJobs`), finds the job of an item (`Find`, `Pending`), cancels,
  recovers panics and logs each job's start and end. `Duration` formats a
  job's time ("1m 03s") and `RunSummary` how a batch ended ("9 done · 1
  failed").
- **Dialogs** (`dialogs.go`): `Dialogs.Ask(Confirm{...})` queues
  confirmations shown one at a time (destructive actions in the danger
  color, a note for administrator authentication); `Info` for content
  dialogs; `ShortcutsBody` and `AboutBody` build the usual two;
  `CodeDialog` shows text exactly as it is (a report before it is sent) in
  a scrollable monospaced box with Cancel and an action, and holds no
  state of its own.
- **Building blocks** (`components.go`, `text.go`): `Card`, `CardHeader`,
  `PageHeader`, `Section` and `SectionHeader` (icon, title, note and an
  aside at the end; `Quiet` mutes the title), `Figures` (a strip of
  clickable figures), `Pill`, `IconPill`, `ColorPill` and `DotPill` (in a
  color of the app's own, for kinds of things), `SquareAvatar` and
  `Initials` (initials on a color the name picks), `IconTile`,
  `IconButton`, `LabeledButton` (one button, plain or primary), `Banner` (a
  toned notice with an action and a close), `Empty`, `Loading`, `Problem`,
  `KeyValues` (with an optional `KeyValuesStyle` for a narrow side panel),
  `PillTabs` (tabs as pills), `SoftSelection` (a theme whose chosen table
  row is a tint, so pills on it still show), `RowDivider`, `Kbd`/`Shortcut`,
  and the text styles `Title`, `Heading`, `Strong`, `Body`, `Muted`,
  `Caption`, `Mono`, `Toned`.
- **Rows** (`rows.go`): `JobStatus(c, RowStatus{...})` draws a row's state
  at its end (a spinner while queued or running, a check, an error, or a
  note; `Compact` for dense rows: a caption, a pill, a bare error icon),
  `VersionChange` ("1.2 → 1.3"), `CommandRow` (an icon tile, a title with
  an optional "admin" pill, the command line in monospace, then a status
  cell and a button cell of fixed widths), `StepTile` (a captioned value
  in a chain such as deployments, the current one in the accent, an empty
  one dimmed), `SwitchRow` (a setting with its switch, which it returns),
  `DisclosureCard` (a card whose head opens it in the frame of the click
  and reports the click) and `TileGrid` (wrapping rows of tiles).
- **Wizard and toasts** (`wizard.go`, `toasts.go`): `Hero` (a large icon
  tile, logo or spinner over a title and a description), `StepDots` (a
  wizard's step indicator, a progress for screen readers) and `TopToasts`
  (toasts at the top of the window, each with a close button, so they cover
  no Back and Next and a lasting one can be closed).
- **Visualizations** (`viz.go`, `pipeline.go`): `LiveChart` (0–100 series,
  smoothed), `Legend`, `Gauge` (an open ring), `SegmentBar`, `ToggleTile`
  (a large on/off tile), `Pipeline` (stages of steps with states, one
  update run across sources), `JobCard` (the rail's running job) and
  `StateOf` (a job's state as a step's).
- **Desktop integration** (`theme.go`, `icons.go`, `appid.go`,
  `launch.go`): `Adapt` keeps the accent in inactive windows as GNOME and
  KDE do (MyGo fades it to gray, as macOS does) and must run first in every
  frame, since MyGo resets the theme each build pass; `Icons` finds
  freedesktop icons in the desktop's theme (Adwaita on GNOME, Breeze on
  KDE, falling back to Adwaita and hicolor) lazily and caches them;
  `SetAppID` sets GLib's program name before MyGo opens the display, so the
  Wayland app ID and the X11 `WM_CLASS` match the app's `.desktop` file;
  `LaunchDesktopApp` starts a desktop app by its ID with `gtk-launch` and
  does not wait. It is a launch, not a change, so it writes nothing to the
  console; an app injects it where its tests must not start anything
  (ChairLift's `Flatpak.launch`).
- **Test helpers** (`frostuitest`): `Loop` stands in for the UI thread in
  tests: workers `Post` from any goroutine, and the test runs what they
  posted on its own goroutine with `Flush`, `RunUntil(t, cond)` or
  `Settle(quiet)`. It imports the standard library only.

The look follows the desktop, not a palette of its own: MyGo reads the
accent, light or dark mode, contrast and text size from the desktop portal,
and frostui adds only metrics (`SpaceXS`…`SpaceXL`, `RadiusCard`,
`RadiusChip`, `RailWidth`, `TopBarHeight`), tones (`Neutral`, `Accent`,
`Success`, `Warning`, `Danger`) mapped onto the theme, and two derived
surfaces (`Chrome` for the bars and rail, `Panel` for cards). This keeps
high contrast and large text working; frostyard's web design system
(`frostyard/design-system`) stays the source for marketing and web apps.

## Rules

- **Imports.** frostui imports the standard library, MyGo, purego and its
  own packages only, and `frostuitest` the standard library only.
  `internal/installcheck.TestFrostUIImportsNoChairLift` enforces it over
  every package of the kit, tests included.
- **One helper, one place.** A view helper that is not about ChairLift (a
  pill, a row, a status, a dialog) belongs here, not as an unexported
  copy in a page. Pass colors from the page's theme where a subtree draws
  under another (`SoftSelection`); key a kit container from the caller
  with `c.Key` (`DisclosureCard(c.Key(id), …)`).
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

Every part has headless tests with `ui.NewTester` (MyGo renders on the
CPU; it needs Pango and cairo, no display). `frostui_test.go` covers
palette ranking and running, console filters and ring buffer, the job
queue's order, serialization, cancellation and panic recovery, dialogs in
order, and the shell's rail, status bar and console; `components_test.go`
covers the pieces promoted from the pages (each row status, the disclosure
card opening in the frame of its click, the key column of `KeyValuesStyle`,
`CodeDialog`'s action and Escape, `TopToasts`' close, `LaunchDesktopApp`
against a fake `gtk-launch` on `PATH`, and the rest); `frostuitest` tests
its own loop. `Tester.Image()` gives a PNG of a frame; ChairLift's tests
write them when `CHAIRLIFT_SNAPSHOT_DIR` is set. To prove a refactor of
the kit changes nothing on screen, render the snapshots before and after
and compare the PNGs pixel by pixel: spinners and wall-clock times
("checked today at 11:56") differ from one run to the next, nothing else.

## Moving it out

Target: `github.com/frostyard/frostui`, tagged like frostyard's other
libraries, with ChairLift requiring it.

1. Copy `internal/frostui` to the new repository at its root, with
   `frostuitest` under it; keep the tests and add a small gallery app (every
   component, light and dark, written with `Tester.Image()` into `docs/`).
2. Rename nothing: the package names are already `frostui` and
   `frostuitest`.
3. In ChairLift, replace the import paths, delete `internal/frostui`, and
   keep `TestFrostUIImportsNoChairLift`'s intent in the new repository as an
   import allow-list test (`frostuitest`: the standard library only).
4. Generalize the two ChairLift-flavored strings: the console pane's
   subtitle ("every command ChairLift runs, as it runs") becomes a field of
   `ConsoleView`, and the status bar's console item label stays.

What stays in ChairLift: `internal/cmdlog` (the backends' command tracer;
it could move too, as a separate tiny package, once another app runs
commands), `internal/sysinfo` (host facts), `internal/activity`
(ChairLift's own record) and everything under `internal/views`.

### What the pages still draw themselves

The page ports wrote their pieces as unexported helpers first; the general
ones are in the kit now (the pills, avatar, tabs, soft selection and
facts of Apps; the row status, version change, run summary and batch of
Updates; the cleanup row and deployment tile of Maintenance; the
disclosure card, switch row, section header and report dialog of
Problems; the hero, step dots and top toasts of the first run; the tile
grid of Features; the desktop-app launcher of Flatpak; the tests' UI-thread
loops). What stays, and why:

- **The Apps table with its inspector** and its "kind segments + filter
  field" toolbar: one user, and its parts are already kit pieces
  (`SoftSelection`, `SquareAvatar`, `ColorPill`, `DotPill`, `PillTabs`,
  `KeyValuesStyle`). The layout moves when a second app has a table with
  an inspector.
- **Two-column pages**: Problems (a list column that wraps above a 320 DIP
  reporting column) and Maintenance (cards that grow 2:1 above minimum
  widths) lay out differently; one helper would change one of them.
- **List rows** of Features (tile, title with the name in monospace, a
  toned line, a version column, a switch cell) and Updates (avatar, title
  and detail, `VersionChange`, a 170 DIP action cell): the same shape, but
  their paddings, gaps and columns differ by a few DIPs, so one `ListRow`
  would move pixels. `CommandRow` covers rows of commands.
- **The Apps "Working" cell** (a 14 DIP spinner and an 11.5 pt word inside
  a table cell) is smaller than `JobStatus`'s spinner.
- **The first run's frame and app lists**: the centered card, the keyed
  Back/Next footer (i18n labels, the flow's rules), the category and app
  rows with their switches (indented, padded inside a bordered column) and
  the progress list stay in `internal/window`; `Hero`, `StepDots`,
  `TopToasts` and `SwitchRow` (the Reporting step) moved.
- **Files other ports were still writing** (`overview.go`, `system.go`,
  `live.go`, `brew*.go`) were not touched. Duplicates seen there:
  `brew_search.go` builds an accent button with an icon by hand ("Search";
  `LabeledButton(…, true)` draws the same), its result rows are the
  Updates row's shape with `ui.Avatar`, and `brewChip` is a quiet tag close
  to a neutral `Pill` (other padding, radius and fill). `brew_test.go`
  keeps its own `uiLoop` (`frostuitest.Loop` does the same).

When a frostyard app on MyGo starts (frostmail, frostnotes, frostread), it
should begin from the shell: rail of mailboxes or notebooks, the palette
for commands, the console as an activity log of sync jobs, and the job
queue for long operations.
