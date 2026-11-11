# 0016 — Adopt the frostyard repository baseline

- **Status:** Accepted; the license its context names was replaced by MIT in [0018](0018-relicense-mit-and-drop-inherited-translations.md)
- **Date:** 2026-11-05

## Context

frostyard/core holds every code repository to one baseline
([core ADR-0071](https://github.com/frostyard/core/blob/main/docs/adr/0071-hold-every-code-repository-to-one-baseline.md),
rows B1 to B16 of `docs/specs/repository-baseline.md`). ChairLift already met
many rows (the instruction-file aliases, `.memory/`, the docs shape, pinned
actions, `.svu.yml`, the GPL-3.0 license, Dependabot). It did not have the
`verify` and `check` make targets, `mise.toml` and `mise.lock`, a risk-path
list, a `SECURITY.md`, a risk-tier heading in the PR template, or a CI
workflow named `ci.yml`. Its CI linted with whatever golangci-lint release was
latest, and the nightly compliance job with another pinned release.

The repository also kept the output of agent frameworks that are no longer
used: `.planning/codebase/` (a codebase map), `docs/superpowers/` and five
finished plans in `docs/plans/`, `.cursor/rules/`, a pointer skill in
`.claude/skills/chairlift/`, `.mill.toml`, and 24 lesson files in
`docs/agents/skills/` that the mill harvested (frostyard/mill is archived).
`.agents` was a symlink to `docs/agents`, so the skills that core syncs
(core ADR-0026) lived under `docs/`.

## Decision

- **Template gate.** ChairLift takes the gate of `bketelsen/agentic-template`
  (its ADR-0002 and ADR-0003): `scripts/check-template.mjs`, the vendored core
  files `scripts/check-corrections.mjs` and `scripts/lib/risk-paths.mjs`, and
  `test/check-template.test.mjs`, all byte-identical copies. The gate checks
  the instruction aliases, that every doc under `docs/` is indexed in
  `docs/README.md`, the risk-path list, `.memory/corrections.jsonl`, and the
  vendored hashes. (In those copies, "ADR-0002" and "ADR-0003" name the
  template's ADRs; in ChairLift this ADR records the same decision.)
- **Make triad and tool pins** (core ADR-0043, ADR-0044). `make verify` runs
  the non-mutating checks and the unit tests; `make check` formats and then
  runs `verify`; `make ci` runs `verify`, the corrections check against
  `BASE`, the race detector and the build matrix. `mise.toml` pins
  golangci-lint and Node; `mise.lock` holds their checksums; go.mod's `go`
  line stays the only Go pin. Every workflow that lints installs the pinned
  golangci-lint through `jdx/mise-action`, and `make lint` refuses another
  release.
- **CI entry.** `.github/workflows/test.yml` becomes `ci.yml`. The workflow
  name (`Tests`, which `snapshot.yml` follows) and every job name stay. The
  Verify job runs `make ci`.
- **Risk paths** (core ADR-0068). `.github/risk-paths.json` lists the
  governance files, the tool pins, the release configuration, the PolicyKit
  policies, the installed wrapper script and the root helper.
- **Skills and lessons.** `.agents/` is a real directory; core's synced skills
  live in `.agents/skills/`, and `.claude/skills` is a symlink to it. The
  harvested lessons that still hold move to
  [design/engineering-lessons.md](../design/engineering-lessons.md); the rest
  are deleted. The framework trees above are deleted after their facts that
  still hold were moved into `AGENTS.md` and `docs/`.
- **Historical docs.** This amends the classification of
  [ADR-0010](0010-docs-are-a-ci-gated-artifact.md): `docs/superpowers/` and
  the finished plans no longer exist, so the historical class is
  `README-go-port.md` and any plan in `docs/plans/` that is marked done.
  `docs/plans/` holds only plans for open work. The rest of ADR-0010 stays.

## Consequences

- `make verify` and `make ci` need Node.js 22 or later and the pinned
  golangci-lint (`mise install`); CI installs both.
- A new doc under `docs/` fails the gate until `docs/README.md` links it.
- A pull request that touches a listed risk path gets the `risk:high` label.
- A golangci-lint upgrade is a change to `mise.toml` and `mise.lock` only.
- Core's skill sync writes to `.agents/skills/`; it supports a directory and a
  symlink alike.
- Lessons now live in 1 design doc, not in a directory that every agent had
  to read file by file.

## Alternatives considered

- **Keep `.agents` as a symlink into `docs/`:** rejected. The docs-index check
  would require `docs/README.md` to link each of about 150 skill files, and
  skills are not documentation in the four categories.
- **Keep `test.yml`:** allowed by the baseline as a low-severity finding, but
  this change already edits the workflow, so the rename comes with it.
- **Keep the mill lessons as files:** rejected. The mill is archived, and half
  of the lessons are about its plan-chunk process, which no longer runs.

## References

- Shapes: [design/engineering-lessons.md](../design/engineering-lessons.md),
  [design/overview.md](../design/overview.md)
- Builds on: [ADR-0010](0010-docs-are-a-ci-gated-artifact.md),
  [ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md)
- Org: core ADR-0018, ADR-0021, ADR-0025, ADR-0026, ADR-0029, ADR-0043,
  ADR-0044, ADR-0068, ADR-0071 (see [org-adrs.md](../org-adrs.md))
