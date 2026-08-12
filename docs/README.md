# Documentation

ChairLift keeps one documentation tree, `docs/`, in frostyard/core's
four-category shape
([core ADR-0025](https://github.com/frostyard/core/blob/main/docs/adr/0025-consolidate-repository-docs-into-docs.md);
the former `yeti/` AI-docs directory is folded in here). Docs are split by
the question they answer:

| Directory | Question | Contents |
| --- | --- | --- |
| [adr/](adr/) | **Why** did we choose this? | Repo-local Architecture Decision Records — immutable once accepted; superseded, never edited. Org-wide decisions go to frostyard/core instead (see [org-adrs.md](org-adrs.md)) |
| [design/](design/) | **How** does it fit together? | Living documents describing the current architecture; [design/overview.md](design/overview.md) is the entry point |
| [specs/](specs/) | **What exactly** is the contract? | Precise, testable interface definitions, changed only alongside implementing code |
| [plans/](plans/) | **When/in what order** do we build? | Phased plans with "Done when" outcomes |

The user-facing docs-site home is [index.md](index.md) (rendered by
`mkdocs.yml`); this file is the contributor-facing index of everything under
`docs/`.

## Index

### Decisions (ADRs)

*(none yet — repo-local decisions get the next free number from
[adr/TEMPLATE.md](adr/TEMPLATE.md); org-wide decisions that bind this repo
are listed in [org-adrs.md](org-adrs.md))*

### Design

- [design/overview.md](design/overview.md) — architecture entry point:
  purpose, dependency flow, key patterns, configuration, build and release
  (formerly `yeti/OVERVIEW.md`)
- [design/package-managers.md](design/package-managers.md) — the Homebrew,
  Flatpak, bootc, sysupdate, and updex wrappers and their view-layer leaf
  packages (formerly `yeti/package-managers.md`)

### Specs

*(none yet)*

### Plans

Historical plan/design artifacts from past automated runs (per
[documentation-consistency.md](documentation-consistency.md), not sources of
current behavior):

- [plans/2026-02-27-extensions-to-features-design.md](plans/2026-02-27-extensions-to-features-design.md)
- [plans/2026-02-27-extensions-to-features-plan.md](plans/2026-02-27-extensions-to-features-plan.md)
- [plans/2026-02-27-feature-update-check-design.md](plans/2026-02-27-feature-update-check-design.md)
- [plans/2026-03-04-puregotk-alignment-design.md](plans/2026-03-04-puregotk-alignment-design.md)
- [plans/2026-03-04-puregotk-alignment-plan.md](plans/2026-03-04-puregotk-alignment-plan.md)

### Process and policy docs (uncategorized, indexed in place)

- [index.md](index.md) — user-facing overview: pages, shortcuts, optional
  dependencies, building and installing (docs-site home)
- [reference.md](reference.md) — user-facing configuration/behavior reference
- [quality.md](quality.md) — quality dashboard: CI, coverage, release signals
- [metrics.md](metrics.md) and [metrics/README.md](metrics/README.md) —
  metrics definitions and the public metrics catalog
- [documentation-consistency.md](documentation-consistency.md) — checklist
  for keeping current-state docs in sync with source; defines which docs are
  current-state vs historical
- [risk-tiers.md](risk-tiers.md) — change risk classification used by PRs
- [review-rubric.md](review-rubric.md) — pull request review rubric
- [SECURITY-AI.md](SECURITY-AI.md) — AI security policy
- [org-adrs.md](org-adrs.md) — frostyard/core ADRs that bind this repository
- [prompts/index.md](prompts/index.md) — reusable agent prompt catalog
- [agents/skills/](agents/skills/) — durable learned lessons (mill harvest)
  and core-synced skills; binding guidance for agents
- [superpowers/](superpowers/) — historical plan/spec artifacts from past
  superpowers runs (not sources of current behavior)

## Conventions

- **New docs start from their category's `TEMPLATE.md`** (in each directory).
- New repo-local decision → new ADR with the next number; if it reverses an
  old one, mark the old one `Superseded by NNNN` rather than editing it.
  Org-wide decisions become ADRs in frostyard/core plus a line in
  [org-adrs.md](org-adrs.md).
- Design docs are updated in place to always reflect reality.
- Specs change only alongside the code that implements them.
- Cross-links between categories are mandatory in both directions.
- Adding a doc means adding it to the index above.
- Doc-consistency tests in `internal/installcheck` pin literal paths and
  claims in these docs; when docs and tests disagree, update both in the
  same commit.
