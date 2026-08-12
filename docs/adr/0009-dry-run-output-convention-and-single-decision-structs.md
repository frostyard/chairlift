# 0009 — Gate dry-run behavior through fixed [DRY-RUN] prefixes and single decision structs

- **Status:** Accepted
- **Date:** 2026-08-12

## Context

ChairLift's `--dry-run` flag must guarantee two things at once: no
state-changing command runs, and the UI never *claims* a change happened.
Those are separate failure modes — a wrapper can correctly skip its command
while the view still shows "installed", removes a row, or confirms a switch
flip. When the toast text and the UI mutation are gated by two independent
`if dryRun` conditionals in a GTK page builder, they can drift, and
[ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md) means the
page builder itself is untestable, so the drift would be invisible to CI.

## Decision

Three rules, applied uniformly:

1. **Every wrapper carries the flag.** `SetDryRun`/`IsDryRun` exist on each
   wrapper package — `internal/homebrew/homebrew.go:35`,
   `internal/flatpak/flatpak.go:34`, `internal/bootc/bootc.go:29`,
   `internal/sysupdate/sysupdate.go:35`, `internal/updex/updex.go:39` — and
   on `internal/views/dryrun.go:16` for configured custom maintenance
   scripts, which have no wrapper package of their own. `app.New()` sets all
   six once at startup. Wrappers skip their state-changing commands entirely
   under dry-run (e.g. `updex.runHelper` returns before ever invoking
   pkexec, `internal/updex/updex.go:160-164`).
2. **Dry-run output is prefix-fixed.** Skipped executions log
   `[DRY-RUN] would execute: ...`-style lines, and preview toasts begin with
   `[DRY-RUN] Preview:` and end with `— no changes made`
   (`internal/views/actionmsg/actionmsg.go`), so dry-run output is
   greppable and unmistakable in logs and on screen.
3. **One decision, never two conditionals.** Where a dry-run-aware handler
   would also mutate UI state on success, the toast and the mutation derive
   from a single tested decision struct returned by
   `internal/views/actionmsg`: `ScriptDecision.Execute` (whether a custom
   maintenance script runs at all), `BundleInstallDecision.Complete`,
   `TapTrustDecision.MutateUI`, and `FeatureToggleDecision.Confirm`. The
   view computes `IsDryRun()` exactly once, builds the decision, and
   branches solely on it for both the mutation and the toast; the caller
   must not independently recompute the condition. Actions with no second
   mutation to gate (upgrade, cleanup, Brewfile dump, bootc/sysupdate stage
   toasts, …) use plain string functions instead — the wrapper already made
   and tested the skip decision.

## Consequences

- A table-driven test in `actionmsg_test.go` proves the mutation gate and
  its toast cannot disagree, on a GTK-less host, for every gated action.
- `[DRY-RUN]` is a stable vocabulary: users, docs, and the E2E readiness
  marker ("Running in dry-run mode",
  [ADR-0008](0008-e2e-readiness-is-a-log-marker-contract.md)) can rely on
  it, and rewording a preview toast is a contract change with a test to
  update.
- Adding a new state-changing action requires classifying it: decision
  struct (it also mutates UI) or plain string (it does not). The `actionmsg`
  package documentation records the classification per function.
- Defense in depth is intentional: the updex helper binary also honors
  `--dry-run` even though the wrapper never reaches it in dry-run mode.

## Alternatives considered

- **A single global dry-run flag:** rejected — wrappers are independently
  usable and independently tested; each package owning its flag keeps its
  tests hermetic. The cost (six `SetDryRun` calls at startup) is one line
  each in `app.New()`.
- **Two conditionals per handler (one for toast, one for mutation):**
  rejected — this is the drift bug the decision structs exist to prevent;
  it shipped real inconsistencies (e.g. dry-run tap trust removing rows)
  before the structs were introduced.
- **Deriving the toast from the mutation result after the fact:** rejected —
  under dry-run there is no mutation result to inspect; the decision has to
  be made once, up front, and shared.

## References

- Shapes: [design/overview.md — Dry-run mode](../design/overview.md#dry-run-mode),
  [design/package-managers.md — View-layer toast and decision helpers](../design/package-managers.md#view-layer-toast-and-decision-helpers-internalviewsactionmsg-internalviewstrustmsg)
- Builds on: [ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md)
- Enforced by: `internal/views/actionmsg/actionmsg_test.go`, the wrapper
  packages' dry-run tests (`internal/homebrew`, `internal/flatpak`,
  `internal/bootc/stage_test.go`, `internal/sysupdate/stage_test.go`,
  `internal/updex/updex_test.go`), and the wiring tests of
  [ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md)
