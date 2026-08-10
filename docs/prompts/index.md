# Agent prompt catalog

These reusable prompts give coding agents a consistent starting point for
common ChairLift tasks. Copy a prompt, replace its bracketed placeholders, and
include the relevant issue or diff.

The prompts supplement rather than replace repository instructions. Agents
must read and follow `AGENTS.md` and the files under `docs/agents/skills/`
before changing code. When a prompt conflicts with repository instructions,
the repository instructions take precedence.

## Catalog

| Prompt | Use it for |
|---|---|
| [Implement an issue](implement-issue.md) | Planning, coding, testing, and documenting a scoped issue |
| [Review a change](review-change.md) | Risk-focused review of a branch or pull request |
| [Reconcile documentation](reconcile-documentation.md) | Checking current-state docs against source and configuration |

All implementation work should finish with `make ci`. If the environment
cannot run a gate, report the exact command and reason instead of claiming it
passed.
