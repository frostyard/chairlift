# 0008 — Define E2E startup readiness as an exact log-marker contract

- **Status:** Accepted
- **Date:** 2026-08-12

## Context

The unit-test layer deliberately never touches GTK
([ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md)), so one
end-to-end smoke test must prove the real application actually boots: GTK
initializes, the window is constructed and presented, dry-run mode engaged.
A GUI booted under CI has no user to observe it, and puregotk exposes no
convenient headless introspection hook — the only observable channel the
test controls is the process's stdout/stderr. "Started successfully" needs
a definition a test can poll.

## Decision

Startup readiness is a contract of three exact stdout markers, emitted by
`internal/app/app.go` and polled verbatim by the E2E test:

- `"Running in dry-run mode"` (`internal/app/app.go:83`)
- `"ChairLift activated"` (`internal/app/app.go:104`)
- `"app: window presented"` (`internal/app/app.go:118`; the test matches the
  line's fixed prefix)

`TestApplicationStartsInDryRun` (`test/e2e/e2e_test.go:48-118`) launches the
built binary with `--dry-run` under `dbus-run-session -- xvfb-run -a`, in a
sanitized environment (`LANG=C`, `LC_ALL=C`, `NO_AT_BRIDGE=1`,
`GTK_A11Y=none`, `GSETTINGS_BACKEND=memory`, `HOME=` a temp dir), polls
combined output every 50ms for all three markers with a 30-second timeout,
requires one further second of stability (the process must not exit), then
terminates its private process group.

The test binary location is a convention, not a search: `make e2e` builds
and exports `CHAIRLIFT_E2E_BUILD_DIR` (`Makefile:76-77`), and the test skips
with an explanatory message when the variable is unset
(`test/e2e/e2e_test.go:256-268`). The E2E package imports no puregotk
package and runs only via the explicit `make e2e` target, outside the
`./internal/...` unit filter (core ADR-0022).

## Consequences

- The three log lines are a public API: renaming, rewording, or "tidying"
  any of them breaks CI until the test's `want` list is updated in the same
  change. That friction is the point — startup observability cannot decay
  silently.
- Readiness is defined by application-authored events, not by sleeps or
  window-system queries, so the test is fast when the app is fast and the
  30s ceiling only binds on genuine hangs.
- The sanitized environment plus private D-Bus/X server keeps the test
  hermetic on shared runners (no session bus leakage, no a11y bus attempts,
  no user dconf writes).
- Anyone adding a startup phase that can hang should add a marker for it and
  extend the contract, rather than relying on the stability window.

## Alternatives considered

- **Fixed sleep then "process still alive" check:** rejected — proves only
  that the process did not crash, not that a window was presented; and it is
  slow on fast hosts, flaky on slow ones.
- **Window-system introspection (xdotool/AT-SPI):** rejected — a11y is
  deliberately disabled for hermeticity, and X-level queries assert less
  than the app's own "window presented" event while being far more fragile.
- **A dedicated readiness IPC (file/socket/D-Bus signal):** rejected —
  production code would grow a test-only channel; the log lines already
  exist for humans and cost nothing to keep exact.

## References

- Shapes: [design/overview.md — Build and Release](../design/overview.md#build-and-release)
  (the CI-mirror paragraph describing `make e2e`)
- Builds on: [ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md),
  [core ADR-0022 — make ci is the canonical gate; TestI* is reserved](https://github.com/frostyard/core/blob/main/docs/adr/0022-make-ci-gate-and-test-naming-filter.md)
- Enforced by: `test/e2e/e2e_test.go` (`TestApplicationStartsInDryRun`,
  `TestApplicationHelp`, `TestInstalledBundleAndHelperBoundary`), wired by
  the `e2e` target in `Makefile`
