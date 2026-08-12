# 0010 — Treat documentation as a CI-gated artifact split into current-state and historical

- **Status:** Accepted
- **Date:** 2026-08-12

## Context

ChairLift's documentation had accumulated the usual failure modes: version
numbers copied from old plans rather than `go.mod`, visibility claims that
no longer matched the page builders, a Go-port proposal that read like a
build guide, and "coming soon" promises that had long since shipped or been
abandoned. Prose has no compiler, so nothing failed when the code moved on.
The repository already had a mechanism for pinning cross-file facts —
`internal/installcheck`'s consistency tests over the Makefile, polkit
policies, and `.goreleaser.yaml` — which raises the question this ADR
answers: prose claims can be pinned the same way.

## Decision

Documentation is a CI-gated artifact with two classes and a test suite:

- **Classification.** `docs/documentation-consistency.md` defines the split:
  `README.md`, `CONFIG.md`, `docs/index.md`, `docs/reference.md`,
  `docs/design/`, and `docs/specs/` are *current-state* and must match the
  code; `README-go-port.md`, `docs/plans/` (except `TEMPLATE.md` files), and
  `docs/superpowers/` are *historical* — kept, but never sources of current
  behavior, and self-declared as such in their own text.
- **Enforcement.** `internal/installcheck/documentation_test.go` pins the
  rules as string-matching unit tests
  (`TestCurrentDocumentationMatchesSourceFacts`): required phrases must be
  present (the updex version in `docs/design/overview.md` is *computed from
  `go.mod`* at test time, never hardcoded; the config-fallback wording must
  appear in all three config docs; `README-go-port.md` must carry its
  historical banner), and banned phrases must stay absent (stale group keys,
  "Help page coming soon", the false runtime-visibility claim, stale build
  commands in the historical guide, the removed `EventError` name across
  code *and* design docs).
- **Process.** The checklist in `documentation-consistency.md` runs on every
  behavior/config/installation change; when docs and tests disagree, both
  are updated in the same commit. The tests run inside the ordinary
  `./internal/...` unit gate, so `make ci` fails on documentation drift
  exactly as it fails on code drift.

The decision, in one line: prose is testable, and current-state prose that
is worth keeping is worth pinning.

## Consequences

- Documented facts with a source of truth in the repo (versions, paths,
  keys, commands) cannot silently drift — CI names the file and the missing
  or forbidden phrase.
- Editing a pinned document has friction: rewording a pinned sentence means
  updating `documentation_test.go` in the same change. That is deliberate;
  the pin marks the sentence as a load-bearing claim.
- Historical documents can be kept honestly instead of deleted or
  half-updated: their banner and the test that requires it stop them from
  being mistaken for guides.
- Each pin is reactive — it exists because a claim drifted once. The suite
  grows with each incident rather than aspiring to pin everything.

## Alternatives considered

- **Delete stale docs instead of classifying:** rejected — the port guide
  and old plans have archival value; the historical banner keeps them
  without letting them lie.
- **A docs linter (vale/markdownlint rules):** rejected — style linters
  cannot cross-reference `go.mod` versions or code identifiers; the facts
  being pinned live in Go files and YAML, where a Go test reads them
  natively.
- **Generating docs from code:** rejected for these documents — they are
  narrative (why/how), not reference tables; generation would flatten them.
  Where derivation is possible, the tests already derive (the `go.mod`
  version check) instead of pinning a literal.

## References

- Shapes: [documentation-consistency.md](../documentation-consistency.md),
  [README.md (docs index)](../README.md), [design/overview.md](../design/overview.md)
- Builds on: [core ADR-0022 — make ci is the canonical gate](https://github.com/frostyard/core/blob/main/docs/adr/0022-make-ci-gate-and-test-naming-filter.md),
  [core ADR-0025 — One docs/ tree per repository](https://github.com/frostyard/core/blob/main/docs/adr/0025-consolidate-repository-docs-into-docs.md)
- Enforced by: `internal/installcheck/documentation_test.go`
