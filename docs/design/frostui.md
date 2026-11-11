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
- **Command tracing.** The kit cannot import `internal/cmdlog`, so a view that
  launches through it traces the command itself (`launchDesktopApp`).
- **The waker.** MyGo's `Window.Update` rebuilds the view on every call, so
  ChairLift wakes the window from a burst of console lines through
  `frostui.Waker`.
- **What stays here.** `internal/cmdlog` (the backends' command tracer),
  `internal/sysinfo` (host facts), `internal/activity` (ChairLift's own
  record), everything under `internal/views`, and the first run's frame and
  app lists in `internal/window`.
