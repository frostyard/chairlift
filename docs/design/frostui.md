# frostui: frostyard's desktop UI kit

ChairLift's window is drawn with `github.com/frostyard/frostui`, a Go module
on MyGo ([ADR-0017](../adr/0017-mygo-command-center-window.md)). The kit began
in this repository as `internal/frostui` and moved out; ChairLift requires the
module (version in `go.mod`) and keeps no copy.

The kit's parts, rules, tests and API changes are documented in the module:
[its design document](https://github.com/frostyard/frostui/blob/main/docs/design/frostui.md).

## What ChairLift adds

- **`Pipeline`'s name.** The module names a pipeline "Pipeline" for screen
  readers. The Updates page chains `.Label("Update run")`.
- **Launch tracing.** The kit cannot import `internal/cmdlog`, so a view that
  launches a desktop app through it reports the launch itself
  (`launchDesktopApp`, with `cmdlog.Launch`). Since frostui v0.5.1,
  `LaunchDesktopApp` starts no launcher program: it resolves the desktop
  entry and launches it in process through GIO, detached from ChairLift's
  stdout and stderr, and fails with GIO's reason when the entry is missing
  or cannot be launched. The lookup and the launch are synchronous, so the
  views call it on a goroutine (`Env.launchApp`) and post the result back.
- **The waker.** MyGo's `Window.Update` rebuilds the view on every call, so
  ChairLift wakes the window from a burst of console lines through
  `frostui.Waker`.
- **What stays here.** `internal/cmdlog` (the backends' command tracer),
  `internal/sysinfo` (host facts), `internal/activity` (ChairLift's own
  record), everything under `internal/views`, and the first run's frame and
  app lists in `internal/window`.

## The Frostyard look

ChairLift draws in the kit's two themes, Frostyard and Frostyard light
(frostui's ADR-0011, from frostui v0.5.0). `Shell.View`, which `Main.View`
calls, and the first run's `FirstRun.View` call `frostui.Adapt` first in
every frame: it picks the theme that matches the desktop's light or dark
setting and applies the desktop's high contrast and text size, but not its
accent. ChairLift sets no theme of its own. The first run's Style step
changes MyGo's theme source (`forceAppearance`), and `Adapt` turns that
choice into Frostyard or Frostyard light.

- **Corners, text and shadows.** Views take corners from the kit
  (`frostui.RadiusCard`, 0; `frostui.RadiusChip`, 2; the theme's `Radius`,
  4, for tiles) and text from its styles (`Title`, `Heading`, `Strong`,
  `Body`, `Muted`, `Caption`, `Mono`). Where no style has the size, they
  use `frostui.TextSize(c, n)`, so the desktop's text size reaches them. No
  view draws a shadow: a hairline (`Border`) separates.
  `internal/installcheck.TestViewsDrawInKitMetrics` fails on a `.Radius` or
  `.FontSize` call with a number literal, on a `FontSize` field with one (a
  `frostui.KeyValuesStyle`), and on any `.Shadow` call under `internal/`.
  `TestOverviewTextFollowsTheDesktopTextSize` draws the Overview at 1.5
  times the usual text size and checks that its own sizes grow.
- **Surfaces.** Where a view paints its own surface, it takes the kit's
  role:
  - the Overview's booted deployment: `ToneFill(Accent)` inside an accent
    hairline;
  - its Recent activity icons: `frostui.IconTile`;
  - the hero's snowflake tile: the Frostyard theme's `PanelFrom` in both
    themes, so the pale snowflake reads, at the theme's `Radius`, in the
    accent's line;
  - a problem's recorded values: `ColorsOf(t).Code` inside a hairline, as
    the console;
  - a Brewfile bundle's package names: a tag, `TagText` inside the
    neutral line. A kit pill would show the names in capitals.
- **Key-value rows.** The kit's `KeyValues` default size (13) does not
  scale, so the Overview's System details pass
  `KeyValuesStyle{FontSize: frostui.TextSize(c, 13)}` (`detailsStyle`), and
  its os-release rows use the same style. The Apps inspector's facts scale
  their 12.5 the same way.
- **The first run.** Its card is `frostui.Card`: the panel face over a
  hairline, square, with no shadow. The Style step's tiles sketch a window
  in `frostui.FrostyardTheme(dark)` with the desktop's preferences minus
  its accent (`schemePreview`), as `Adapt` will draw the window; the chosen
  tile is the selected fill inside an accent hairline.
  `TestFirstRunSchemeTilesDrawTheFrostyardThemes` reads both sketches'
  grounds.

**Known gaps, which belong to the module.** The views' plain MyGo buttons
(`ui.Button`, `ui.PrimaryButton`) keep MyGo's faces, a surface and the flat
accent, not the brand's ghost and gradient. The kit's branded buttons
(`IconButton`, `LabeledButton`) carry an icon, and it exports no text-only
one. Some of the kit's own sizes are literal too, as `KeyValues`' default
and the `Figures` notes, and do not follow the desktop's text size.
