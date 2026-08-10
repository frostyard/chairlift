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
| Adaptive Auto-QA | Outcome- and risk-tuned uncached test repetitions for pull requests | [GitHub Actions](https://github.com/frostyard/chairlift/actions/workflows/auto-qa.yml) |
| Nightly compliance | Daily full CI, E2E, and known-vulnerability scan results for the default branch | [GitHub Actions](https://github.com/frostyard/chairlift/actions/workflows/nightly-compliance.yml) |
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

## Adaptive Auto-QA

`.github/workflows/auto-qa.yml` adjusts additional QA intensity from observed
outcomes without weakening the required Tests workflow. For each non-draft
pull request, it reads up to 20 completed default-branch push runs of
`test.yml` and selects a bounded number of uncached, shuffled internal test
runs:

| Recent default-branch failure rate | Repetitions |
|---|---:|
| No history | 2 |
| Below 5% | 1 |
| 5% to below 20% | 2 |
| 20% or higher | 3 |

Changes to workflows, dependency manifests, packaging/install files,
PolicyKit policies, configuration validation, bootc, or updex raise the result
to the maximum three repetitions regardless of recent success. The selected
history inputs, sensitive-path count/sample, repetition count, and reason are
recorded in the workflow summary so the tuning decision is auditable.

The range is deliberately fixed at one through three: missing history fails
conservatively to the middle tier, a healthy period cannot remove the extra
uncached run, and bad outcomes can increase cost only to a known bound. The
workflow has read-only API/repository permissions, uses commit-pinned actions,
persists no checkout credentials, and receives no secrets. It supplements but
never changes, skips, approves, or merges around the ordinary quality gates.

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

The workflow has read-only contents access and pull-request comment access. It
does not check out or execute pull request code, interpolate review text into a
shell, approve, merge, or bypass required checks. Copilot's resulting changes
must still pass the ordinary quality gates and human review; findings that
should not be applied must be explained on the pull request.

Reusable implementation and review prompts are available in the
[agent prompt catalog](prompts/index.md). They are aids only; repository
instructions and human review remain authoritative.
