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
| Pull request checks | Gate results attached to each proposed change, including reruns and logs | Open a pull request and select its **Checks** tab |
| Coverage | Line coverage produced by tests under `internal/...` | [Codecov](https://app.codecov.io/gh/frostyard/chairlift) |
| Build artifacts | Seven-day Linux binaries for the workflow's amd64 and arm64 matrix | Open a successful workflow run and view **Artifacts** |
| Release history | Published versions and release assets | [GitHub Releases](https://github.com/frostyard/chairlift/releases) |

Coverage upload is intentionally informational: `.github/workflows/test.yml`
sets Codecov failures to non-blocking. A missing Codecov report therefore does
not mean tests failed, and a green workflow does not prove that coverage was
uploaded. Use the workflow's **Unit Tests** log to distinguish those outcomes.

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
gate documented in `AGENTS.md`.

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

## Reviewing agent changes

For an agent-authored pull request, audit the signals in this order:

1. Read the issue and diff to confirm the change stays within scope.
2. Check the pull request's commit history and changed-files list for unrelated
   edits, generated artifacts, or missing documentation.
3. Require a green Tests workflow; inspect logs rather than relying only on the
   aggregate check mark.
4. Review coverage for changed pure-Go logic and verify regression tests cover
   the reported failure mode.
5. Apply the repository invariants and learned skills from `AGENTS.md`, then
   record concrete review findings on the pull request.

Reusable implementation and review prompts are available in the
[agent prompt catalog](prompts/index.md). They are aids only; repository
instructions and human review remain authoritative.
