# Contributing to ChairLift

Thank you for helping improve ChairLift.

## Before you start

- Search the [issue tracker](https://github.com/frostyard/chairlift/issues) for
  existing work.
- Open an issue before making a large behavioral or architectural change.
- Review [AGENTS.md](AGENTS.md) for repository invariants and development
  guidance.

## Development setup

ChairLift requires Go and builds without CGO. Runtime dependencies and
installation details are listed in the [README](README.md).

```sh
git clone https://github.com/frostyard/chairlift.git
cd chairlift
make deps
make build
```

## Making changes

1. Create a focused branch from the current default branch.
2. Keep changes small and limited to one concern.
3. Add or update tests for changed behavior.
4. Update relevant documentation when behavior, configuration, dependencies,
   or installation changes.
5. Format Go changes with `make fmt`.

Tests for packages that import puregotk cannot run on headless systems because
GTK libraries are loaded during package initialization. Extract testable logic
into a pure-Go package under `internal/` rather than adding tests to those
packages. Name ordinary unit tests so they do not begin with `TestI` or contain
`Integration`; CI reserves those names for tests requiring a real environment.

## Validation

Run the same checks used by CI before submitting:

```sh
make ci
```

For a quicker iteration loop, use `make test` and `make lint`, then run
`make ci` before opening or updating a pull request.

## Pull requests

- Link the relevant issue.
- Explain what changed and why.
- Describe how the change was tested.
- Keep unrelated refactors out of the pull request.
- Ensure all CI checks pass.

By contributing, you agree that your contributions are licensed under the
project's [GPL-3.0-or-later license](LICENSE).
