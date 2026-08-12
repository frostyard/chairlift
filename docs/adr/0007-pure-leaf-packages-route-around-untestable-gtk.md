# 0007 — Put all decidable logic in headless leaf packages; keep GTK packages test-free

- **Status:** Accepted
- **Date:** 2026-08-12

## Context

ChairLift uses puregotk, a CGO-free GTK4/libadwaita binding that resolves the
GTK and graphene shared libraries at package init via purego. On a GTK-less
host — including CI's unit-test runners — a test binary that *imports* a
puregotk-importing package panics during init, before any test function
runs. `go test` over such packages therefore cannot work headlessly, no
matter how the tests are written. Yet the interesting decisions (what
subtitle a row gets, whether a dry-run click mutates a row, how update
badges count) must be unit-tested somewhere.

## Decision

The package layout routes all decidable logic around GTK:

- Exactly three surfaces import puregotk: `internal/app`, `internal/window`,
  and the page builders at the top level of `internal/views`
  (`applications_page.go`, `features_page.go`, `help_page.go`,
  `maintenance_page.go`, `system_page.go`, `updates_page.go`, `views.go`).
  These packages contain **zero** `_test.go` files, by policy: any logic
  worth testing must not live there.
- Every decidable computation lives in headless leaf packages that import no
  GTK: `internal/views/actionmsg`, `actionstate`, `badgestate`,
  `bundleview`, `featurestatus`, `flatpakstatus`, `pageview`, `rowset`, and
  `trustmsg`, plus non-view peers like `internal/updexhelper`. Each leaf
  ships table-driven unit tests that run on a GTK-less host.
- Wiring tests keep the GTK layer honest without importing it:
  `TestPageBuildersUsePurePresentations`
  (`internal/views/pageview/wiring_test.go:11`) reads the page builders'
  *source* and asserts every one calls its `pageview.*` presentation helpers
  and that retired inline presentation fragments do not reappear; the
  `actionstate` wiring tests (`internal/views/actionstate/wiring_test.go`,
  `applications_wiring_test.go`) do the same for action-state decisions.

This is what makes a CGO-free GTK application testable on GTK-less CI: the
GTK packages shrink to declarative wiring, and everything with a branch in it
is a pure function under test.

## Consequences

- `CGO_ENABLED=0 go test ./internal/...` runs the entire decidable surface
  on any host; CI needs no GTK, no display, no dbus for unit tests (the E2E
  smoke test, [ADR-0008](0008-e2e-readiness-is-a-log-marker-contract.md),
  covers the real GTK path separately).
- New view logic must be designed as a leaf function first and wired second;
  "just compute it in the page builder" is a reviewable violation, and the
  wiring tests make regressions mechanical.
- The wiring tests are source-string matches, so refactoring a page builder
  can require updating the expected fragments — a deliberate cost, since the
  fragments are the proof that presentation stays out of GTK code.
- More packages exist than a conventional layout would have; the naming
  (`internal/views/<concern>`) keeps each leaf adjacent to its consumer.

## Alternatives considered

- **Interface-mocking the GTK widgets:** rejected — the panic happens at
  package *init* of any puregotk importer, so even a fully mocked test file
  in those packages cannot run headlessly.
- **Build-tagging GTK code and testing behind a stub tag:** rejected — a
  stub GTK surface large enough to exercise page builders is a second
  implementation that drifts; the leaf split tests real logic instead of
  imitation widgets.
- **Running unit tests under xvfb with GTK installed:** rejected as the
  primary strategy — it makes every unit-test invocation host-dependent and
  slow; reserved for the single E2E smoke test.

## References

- Shapes: [design/package-managers.md — the View-layer sections](../design/package-managers.md#view-layer-page-presentation-internalviewspageview),
  [design/overview.md — Key Patterns](../design/overview.md#key-patterns)
- Related: [ADR-0008](0008-e2e-readiness-is-a-log-marker-contract.md),
  [ADR-0009](0009-dry-run-output-convention-and-single-decision-structs.md)
- Enforced by: `internal/views/pageview/wiring_test.go`,
  `internal/views/actionstate/wiring_test.go`,
  `internal/views/actionstate/applications_wiring_test.go`, and the per-leaf
  unit tests in each `internal/views/*` package
