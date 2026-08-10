# Quality dashboard

This page is the entry point for auditing ChairLift's current quality signals.
It links to live reports rather than copying pass rates or coverage percentages
that would immediately become stale.

[![Tests](https://github.com/frostyard/chairlift/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/frostyard/chairlift/actions/workflows/test.yml?query=branch%3Amain)
[![Codecov](https://codecov.io/gh/frostyard/chairlift/branch/main/graph/badge.svg)](https://app.codecov.io/gh/frostyard/chairlift)

## Live signals

| Signal | What it reports | Source |
|---|---|---|
| Tests workflow | Latest lint, unit-test, race-detection, verification, and cross-architecture build results | [GitHub Actions](https://github.com/frostyard/chairlift/actions/workflows/test.yml) |
| Nightly compliance | Daily full CI, E2E, and known-vulnerability scan results for the default branch | [GitHub Actions](https://github.com/frostyard/chairlift/actions/workflows/nightly-compliance.yml) |
| Issue triage | Deterministic labels applied from structured issue titles and bodies | [GitHub Actions](https://github.com/frostyard/chairlift/actions/workflows/triage.yml) |
| Pull request checks | Gate results attached to each proposed change, including reruns and logs | Open a pull request and select its **Checks** tab |
| PR acceptance | Accepted and closed pull request counts over a rolling 90-day cohort | [Metric definition and reproducible query](metrics.md) |
| Coverage | Line coverage produced by tests under `internal/...` | [Codecov](https://app.codecov.io/gh/frostyard/chairlift) |
| Build artifacts | Seven-day Linux binaries for the workflow's amd64 and arm64 matrix | Open a successful workflow run and view **Artifacts** |
| Release history | Published versions and release assets | [GitHub Releases](https://github.com/frostyard/chairlift/releases) |

`codecov.yml` compares project coverage with the pull request's base and fails
its project status only when coverage drops by more than one percentage point.
It deliberately has no fixed project target or patch target. The upload step
remains non-blocking, so a missing Codecov report does not mean tests failed,
and a green Tests workflow does not prove that coverage was uploaded. Use the
workflow's **Unit Tests** log to distinguish those outcomes.

## Enforced checks

The repository's `Tests` workflow runs on pushes and pull requests targeting
`main`. Its jobs provide these independent signals:

- **Lint** — `golangci-lint` over the Go source.
- **Unit Tests** — headless tests under `internal/...`, with atomic coverage.
- **Race Detection** — the same internal test scope under the race detector.
- **Verify** — tidy-module, `go vet`, and `gofmt` checks.
- **Build** — Linux builds for amd64 and arm64.

`make ci` mirrors those credential-free checks locally in fail-fast order and
also rebuilds the native binaries at the end. It is the pre-submission quality
gate documented in `AGENTS.md`; Codecov's remote project status is additional
and cannot be reproduced by that target.

```bash
make ci
```

To inspect the same coverage scope locally without relying on the external
upload, run:

```bash
go test ./internal/... \
  -coverprofile=coverage.out \
  -covermode=atomic \
  -run '^Test[^I]' \
  -skip Integration
go tool cover -func=coverage.out
```

The test-name filters are significant: ordinary unit tests must not begin with
`TestI` or contain `Integration`, because the gated command excludes them.
Tests for packages that import puregotk also cannot run headlessly; decidable
logic belongs in a pure package under `internal/`, as required by `AGENTS.md`.

## Nightly compliance

The `Nightly compliance` workflow runs every day at 04:17 UTC and can also be
started manually. It checks out the current default branch, runs the complete
host-independent `make ci` gate, installs the same GTK/Xvfb runtime used by the
hosted E2E job and runs `make e2e`, then runs `govulncheck ./...` against the
current Go vulnerability database. This catches dependency disclosures and
environment drift even when no pull request is active.

The workflow has read-only repository permission, persists no checkout
credentials, consumes no repository secrets, and publishes nothing. Its
third-party actions are pinned to commits and its Go compliance tools are
installed at explicit versions. A nightly failure is an investigation signal;
it does not replace pull-request checks or authorize an automatic merge.

## Reviewing agent changes

For an agent-authored pull request, audit the signals in this order:

1. Read the issue and diff to confirm the change stays within scope.
2. Check the pull request's commit history and changed-files list for unrelated
   edits, generated artifacts, or missing documentation.
3. Require a green Tests workflow; inspect logs rather than relying only on the
   aggregate check mark.
4. Review coverage for changed pure-Go logic and verify regression tests cover
   the reported failure mode.
5. Apply the [pull request review rubric](review-rubric.md), including the
   repository invariants and learned skills, then record concrete findings on
   the pull request.

## Automated Copilot feedback

`.github/workflows/copilot-review-apply.yml` closes the feedback loop for
Copilot pull request reviews. When the trusted Copilot reviewer submits a
commented or changes-requested review with inline findings on an open,
non-draft pull request, the workflow posts one deduplicated `@copilot` fix
request linked to that review. A review without inline findings does not start
a fix cycle.

`.github/workflows/ai-fix-requested.yml` handles explicit implementation
requests on issues. When the `ai-fix-requested` label is applied to an open
issue, it posts one deduplicated `@copilot` request linked to that issue.
Reapplying the label does not start a duplicate fix cycle.

The review workflow receives read-only contents and pull-requests write
permissions; the issue workflow receives only issues write permission. Each
uses its write permission only to create the request comment. Neither workflow
checks out or executes repository code, interpolates review or issue text into
a shell, approves, merges, or bypasses required checks. Copilot's resulting
changes must still pass the ordinary quality gates and human review; review
findings that should not be applied must be explained on the pull request.

## Automated issue triage

`.github/workflows/triage.yml` applies conservative, additive labels when an
issue is opened or edited. Its deterministic rules are:

| Structured input | Labels |
|---|---|
| An `[ACMM L…]` title plus an `acmm:` criterion field | `acmm`, `ai-fix-requested` |
| A `[guide]` title or guide-agent filing marker | `documentation`, `agent/guide` |
| A `[quality]` title or quality-agent filing marker | `agent/quality` |
| Explicit bug, documentation, feature/enhancement, or question title prefixes | The matching standard category label |
| A `Documentation Gap` or `Feature Request` body heading | `documentation` or `enhancement` |

Triage never removes a label, so an edit cannot erase a maintainer decision. It
checks that configured labels still exist, skips labels already present, and
leaves ambiguous issues for humans. Issue text is bounded and matched only as
data by a commit-pinned API action: the workflow checks out and executes no
issue-controlled code, receives no secrets, and has only read-only contents
plus issue-label write permission.

Reusable implementation and review prompts are available in the
[agent prompt catalog](prompts/index.md). They are aids only; repository
instructions and human review remain authoritative.
