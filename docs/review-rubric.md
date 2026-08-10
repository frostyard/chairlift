# Pull request review rubric

Use this rubric for every ChairLift pull request. Review the issue, read
`AGENTS.md` and every file in `docs/agents/skills/`, then inspect the changed
code in context rather than reviewing the diff alone.

## Review criteria

| Area | Approval standard |
|---|---|
| Scope | The change satisfies the linked issue, contains no unrelated refactors or generated artifacts, and preserves behavior outside the requested scope. |
| Risk classification | The pull request selects the highest applicable [change risk tier](risk-tiers.md), and its review and validation evidence meet that tier's requirements. |
| Correctness | Success, failure, dry-run, retry, and disabled-feature paths behave consistently. Errors remain visible instead of being silently converted into success or invented state. |
| Repository invariants | The privilege boundary, GTK main-thread rule, async generation guards, config-driven visibility, navigation authority, and known-state ownership rules in `AGENTS.md` remain intact. |
| Tests | Changed behavior has regression coverage in a CI-enforced scope. Tests avoid puregotk packages, do not begin with `TestI` or contain `Integration` unless they intentionally require the separately enforced environment, and cover the reported failure mode rather than only a happy path. |
| Quality gates | Required pull request checks pass. For source changes, `make ci` is the local equivalent of the host-independent checks; review Codecov separately for an unexpected coverage regression. |
| Documentation | User-visible behavior, configuration, dependencies, installation layout, and repository invariants are documented where relevant. Current-state claims agree with source, config, `go.mod`, and packaging files. |
| Maintainability | The change follows existing package boundaries and naming, reuses established helpers, keeps type and error handling explicit, and avoids adding a second source of truth. |

Apply the durable lessons in `docs/agents/skills/` whenever their stated
conditions match the change. A green workflow does not override a violated
repository invariant or missing scenario coverage.

## Findings and decisions

Classify a finding as **blocking** when it can cause incorrect or unsafe
behavior, violates a repository invariant, leaves required behavior untested,
contradicts current-state documentation, or prevents a required quality gate
from passing. Include the file and line, the concrete failure mode, and the
smallest safe correction.

Classify optional cleanup or future hardening as **non-blocking** and explain
why approval does not depend on it. Do not block on personal style preferences
that are not enforced by repository conventions.

Approve only when no blocking findings remain and the available evidence
supports the change. If evidence is unavailable or a relevant scenario cannot
be exercised, state that testing gap explicitly instead of treating it as a
successful result.
