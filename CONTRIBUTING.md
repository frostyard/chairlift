# Contributing to ChairLift

Thank you for helping improve ChairLift.

## Before you start

- Search the [issue tracker](https://github.com/frostyard/chairlift/issues) for
  existing work.
- Open an issue before making a large behavioral or architectural change.
- Review [AGENTS.md](AGENTS.md) for repository invariants and development
  guidance.
- Follow the [AI security policy](docs/SECURITY-AI.md) for AI-assisted work,
  including its data, tool, automation, and human-review boundaries.

## Local setup

ChairLift requires Git, Make, the Go version declared in
[`go.mod`](go.mod), and `golangci-lint` for the complete local quality gate.
The application builds without CGO, but the race-detector gate requires a
working CGO toolchain. GTK4 and Libadwaita are runtime dependencies and are
needed for E2E testing, not ordinary builds.

Fork the repository, clone your fork, and register the upstream repository:

```sh
git clone git@github.com:YOUR-USER/chairlift.git
cd chairlift
git remote add upstream https://github.com/frostyard/chairlift.git
git fetch upstream
make deps
make build
```

Installation details and optional runtime dependencies are listed in the
[README](README.md).

## Development workflow

1. Start a focused branch from the current upstream default branch:

   ```sh
   git fetch upstream
   git switch -c TYPE/ISSUE-DESCRIPTION upstream/main
   ```

2. Keep the change limited to one issue and avoid unrelated refactors or
   generated artifacts.
3. Add or update regression tests for changed behavior. Use `make test` and
   `make lint` for a quick local iteration loop, and format Go changes with
   `make fmt`.
4. Update the relevant current-state documentation. Changes to behavior,
   configuration, dependencies, or installation layout must follow the
   [documentation consistency checklist](docs/documentation-consistency.md).
5. Run `make ci` before pushing. Rebase onto the latest `upstream/main`, rerun
   the gate, then push the branch to your fork:

   ```sh
   git fetch upstream
   git rebase upstream/main
   make ci
   git push --set-upstream origin HEAD
   ```

Use an EditorConfig-compatible editor so new text follows the repository's
line-ending, indentation, final-newline, and whitespace conventions.

## Testing constraints

Tests for packages that import puregotk cannot run on headless systems because
GTK libraries are loaded during package initialization. Extract testable logic
into a pure-Go package under `internal/` rather than adding tests to those
packages. Name ordinary unit tests so they do not begin with `TestI` or contain
`Integration`; CI reserves those names for tests requiring a real environment.

The enforced unit and race gates run tests under `internal/...`. Logic moved
outside that tree needs a separately enforced test path; do not rely on an
ad-hoc local test that CI never executes.

Run `make e2e` when a change affects application startup, GTK integration,
installation staging, command-line behavior, or the privileged helper. This
target requires GTK4, Libadwaita, `dbus-run-session`, GNU `timeout`, and Xvfb.

## Quality gates

`make ci` is required before opening or updating a pull request. It mirrors the
host-independent checks in the repository's `Tests` workflow:

| Gate | What it checks |
|---|---|
| Verify | `go.mod`/`go.sum` remain tidy, `go vet` passes, and Go files are formatted |
| Lint | `golangci-lint` reports no violations |
| Unit tests | Headless tests under `internal/...` pass with the CI name filters |
| Race detection | The same internal test scope passes with the race detector |
| Build | Linux amd64, Linux arm64, and the native binaries compile |

The hosted workflow additionally runs `make e2e` with its runtime dependencies.
Codecov reports the same internal test scope and rejects project coverage
regressions greater than one percentage point; that remote signal is not
reproduced by `make ci`. See the [quality dashboard](docs/quality.md) for the
canonical description of every signal.

## Pull requests

- Target `main` from the branch on your fork and use a closing keyword for an
  issue the pull request fully resolves.
- Complete every section of the pull request template: explain what changed
  and why, list exact validation commands and results, and describe regression
  coverage and documentation changes.
- Keep the commit history and changed-files list focused on the issue.
- Ensure every required GitHub check passes, and inspect a failed job's logs
  rather than relying on the aggregate status.
- Review proposed changes against the
  [pull request review rubric](docs/review-rubric.md).

By contributing, you agree that your contributions are licensed under the
project's [GPL-3.0-or-later license](LICENSE).
