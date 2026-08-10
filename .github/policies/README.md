# Repository policies

This directory is the canonical home for machine-readable repository
governance.

`auto-qa-tuning.json` defines how ChairLift may react to changes in pull
request acceptance. It keeps required, security, and coverage checks fixed;
policy adjustments still require a pull request and human review. The
gate-enforced `internal/installcheck/autoqa_test.go` test verifies the schema
version, signal definition, and required/security guardrails.

The human-readable policy sources remain:

- [`AGENTS.md`](../../AGENTS.md) for repository invariants;
- [`docs/SECURITY-AI.md`](../../docs/SECURITY-AI.md) for AI security
  boundaries;
- [`docs/risk-tiers.md`](../../docs/risk-tiers.md) for change
  classification; and
- [`docs/review-rubric.md`](../../docs/review-rubric.md) for approval
  standards.

Machine-readable policy never authorizes automation to approve, merge,
release, or deploy its own changes.
