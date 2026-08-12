# 0004 — Give configuration failures a fixed, greppable diagnostic vocabulary

- **Status:** Accepted
- **Date:** 2026-08-12

## Context

[ADR-0003](0003-two-tier-config-with-fail-closed-semantics.md) makes a
present-but-broken configuration file disable every feature group. That is
only tolerable if the failure is unmissable and self-explanatory: an
administrator staring at an empty ChairLift window needs the exact file and
cause, support needs one string to grep logs for, and tooling needs error
classifications that do not shift wording between releases.

## Decision

Every authoritative configuration failure is reported through one
`*config.LoadError` with a fixed vocabulary:

- **Log line:** `LoadError.LogMessage()` (`internal/config/diagnostic.go:7-12`)
  renders the fixed, greppable prefix `CONFIGURATION ERROR:` followed by the
  structured error and the sentence stating that all feature groups were
  disabled and the file must be fixed.
- **Toast:** `LoadError.ToastMessage()` (`internal/config/diagnostic.go:16-21`)
  renders the user-facing equivalent, shown as a persistent toast — timeout 0,
  displayed until dismissed — as soon as the window's toast overlay exists
  (`internal/window/window.go:79-84`, `ShowErrorToast` at
  `internal/window/window.go:441-445`).
- **Classification:** `ErrorKind` (`internal/config/loaderror.go:8-28`) is a
  stable three-value enum — `read`, `parse/type`, `schema` — whose string
  values are documented as part of the diagnostic vocabulary and must not
  change. Within `read`, *absent* is distinguished from *unreadable* by the
  `fs.ErrNotExist` sentinel (via `LoadError.Unwrap`): absence advances the
  search order in `Load` (`internal/config/config.go:97-99`), while any
  other read failure is authoritative and fails closed.
- **Layout:** `LoadError.Error()` (`internal/config/loaderror.go:49-71`)
  renders `config <kind> error: <path>: <detail>: <cause>` in a fixed,
  documented field order, so the path and cause always appear and always in
  the same place.

## Consequences

- `grep "CONFIGURATION ERROR"` finds every configuration failure in any log,
  release after release; support documentation can rely on the phrase.
- The blank window that fail-closed produces is always accompanied by a
  toast naming the file and cause, so ADR-0003's severity does not strand
  users.
- The enum values and message layout are load-bearing: changing them is a
  breaking change to the diagnostic contract and requires updating the tests
  and documentation that pin them (`internal/config/diagnostic_test.go`,
  `internal/config/loaderror_test.go`, `CONFIG.md`, `docs/reference.md`).
- New failure modes must be expressed through the existing kinds (or a
  deliberate, documented enum extension), not ad-hoc log strings.

## Alternatives considered

- **Plain `log.Printf` at each failure site:** rejected — wording drifts,
  nothing is greppable, and the toast and log can disagree about the cause.
- **A transient toast:** rejected — a 3-second toast is missable, and the
  resulting all-groups-disabled window would look like a bug with no
  explanation left on screen.
- **Distinct `ErrorKind` values for absent vs unreadable:** rejected —
  absence is not an error the user sees (it advances the search order);
  encoding it as a kind would put a non-failure in the failure vocabulary.
  The `fs.ErrNotExist` sentinel already distinguishes the two precisely.

## References

- Shapes: [design/overview.md — Configuration](../design/overview.md#configuration),
  [CONFIG.md](../../CONFIG.md) (fail-closed notes), `docs/reference.md`
- Builds on: [ADR-0003](0003-two-tier-config-with-fail-closed-semantics.md)
- Enforced by: `internal/config/diagnostic_test.go`,
  `internal/config/loaderror_test.go`, `internal/config/runtime_test.go`,
  `internal/config/sourcesurface_test.go`
