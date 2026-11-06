# Engineering lessons

Living document. Rationale:
[ADR-0007](../adr/0007-pure-leaf-packages-route-around-untestable-gtk.md),
[ADR-0005](../adr/0005-config-schema-reflected-from-canonical-struct.md).
Contracts: the guards named in each section.

## Overview

The rules below come from defects and rejected reviews in earlier work on
ChairLift. Each rule still holds for the code at the time of writing; each
section names the code or guard it applies to. Code comments link these
sections by their anchor (`docs/design/engineering-lessons.md#<anchor>`), so
keep the anchors stable.

These lessons were first written as separate files in `docs/agents/skills/`
by an automated spec-to-PR harness (the mill, archived). The baseline cleanup
(ADR-0016) moved the lessons that still hold into this file and deleted the
rest; Git history keeps the originals.

## Design

### Tests

<a id="no-tests-in-puregotk-packages"></a>
#### No test file in a package that imports puregotk

puregotk loads GTK, Libadwaita, GLib and graphene with `dlopen` at package
initialization, and panics when a library is missing. The CI runners have
none of these libraries. So a test binary for any package that imports
puregotk, directly or transitively, panics before a test function runs, for
example:

```
panic: Path for library: graphene not found ...
FAIL github.com/frostyard/chairlift/internal/views
```

- `internal/views`, `internal/window` and `internal/app` import puregotk and
  must stay without `_test.go` files. A guard, a nil check or a zero-value
  struct in the test does not help: the panic comes at init.
- A host with GTK installed (for example through linuxbrew) runs such a test
  green, and `make ci` passes locally. Do not trust local success for a test
  in a puregotk package. The only safe signal: the package does not import
  puregotk.
- Put the decidable, widget-free logic in a small package that imports no
  puregotk (the nine leaf packages under `internal/views/`, such as
  `trustmsg`, `actionmsg` and `pageview`) and test it there, table-driven.
- The CI name filter (`-run "^Test[^I]" -skip "Integration"`) is not an
  escape hatch: a skipped test still lives in the binary that panics at init.
- A method of a puregotk-holding struct (for example a nil guard on a widget
  field) cannot have a headless unit test. Extract its decidable core when it
  needs a test.

Guard: the `pageview` wiring test covers all seven page builders (see
`AGENTS.md`, "Headless view coverage stays puregotk-free").

<a id="gates-run-only-internal-tests"></a>
#### The gates run only the tests under `internal/`

`make verify`, `make ci` and the Unit Tests and Race Detection jobs of
`.github/workflows/ci.yml` run `go test ./internal/...`. A test in `cmd/` or
at the root builds and passes with `go test ./...`, but no gate runs it. When
a `cmd/` package needs regression coverage, move its decidable logic into an
`internal/` package that the command calls (for example
`internal/updexhelper` for `cmd/chairlift-updex-helper`), and test it there.
Guard: `internal/installcheck.TestGateFilterHidesNoHermeticTest`. The E2E
tests in `test/e2e/` are the exception: `make e2e` runs them in their own job.

<a id="check-every-collection-entry"></a>
#### A consistency test checks every entry of a collection

A test that checks a property of a list in a real file (for example every
`nfpms[]` entry of `.goreleaser.yaml`) loops over all entries, with a
`t.Run` per entry, and fails when the list is empty. A check of index 0 only
stops protecting anything when a second entry is added or the entries are
reordered. Example: `TestGoreleaserNfpmLayoutMatchesUsrPrefix` and
`TestGoreleaserLicenseIsGPL` in `internal/installcheck/goreleaser_test.go`.

<a id="consistency-needs-a-gated-check"></a>
#### A consistency requirement needs a gated check, not an inspection

When two sources of truth must agree (for example the `Makefile` install
layout and the nFPM package layout), each half needs an executable check that
CI runs. A half that is correct today still gets a check: the check passes
now and fails when that half drifts later.

<a id="assert-the-exact-remaining-set"></a>
#### Prove a removal with the exact remaining set

A test that guards a removal asserts the exact set that remains (its length
and each member), not "does not contain the removed name". The exact set also
catches the removed thing when it comes back under another name, and the test
does not have to spell the removed literal, so a repository-wide grep for it
stays empty. Example: the release footer check in
`internal/installcheck/goreleaser_test.go`.

<a id="parse-reported-line-numbers"></a>
#### Parse a reported line number; do not match the substring

A test of an error detail that names a line (`"... (line 3)"`) extracts the
number and checks the expected value, or at least that it is positive. A
check for the substring `"line "` also accepts `line 0` and `line -1`. When
one assertion helper serves a whole table, make it fail once on a wrong
fixture before you trust it.

<a id="call-helpers-directly-in-tests"></a>
#### Direct calls to unexported helpers are an allowlist

`internal/config` limits which unexported production helpers a test may call
by name: `directCallAllowlist` in `internal/config/sourcesurface_test.go`
(`TestSourceHelperDirectCallSurface`). A helper on the list has a direct unit
test of its own in addition to the coverage through the exported entry point.
Every other helper is tested only through its exported entry point.

<a id="direct-call-allowlist-entries"></a>
#### Each allowlist entry is authorized on its own

A reason to add one helper to `directCallAllowlist` does not cover a second
helper. Before a test calls an unexported helper directly, read the list
literal and check that the name is on it; when it is not, test the helper
through an entry point that is on the list, or add the entry with its own
reason in the comment above the list.

### Configuration code

<a id="yaml-key-identity-uses-the-tag"></a>
#### A yaml.v3 key identity includes the tag

A `yaml.Node`'s `Value` is the raw text of a scalar, so the string key `"1"`
and the integer key `1` have the same `Value` and different tags (`!!str`,
`!!int`). Code that compares or deduplicates mapping keys for merge
precedence builds the identity from the resolved tag and the value
(`effectiveKeyIdentity` in `internal/config/effectivekeys.go`), and its tests
include two keys with the same `Value` and different tags. The duplicate-key
check in `internal/config/sourcegraph.go` is the exception on purpose: it
copies yaml.v3's own `uniqueKeys` predicate (see
[overview](overview.md)).

<a id="read-dependency-source-in-gomodcache"></a>
#### Match a dependency from its source, not from memory

When code must behave the same as a dependency (yaml.v3's merge-key
predicate, updex's update results, std's `problems` reader), read the
dependency's source in `$(go env GOMODCACHE)/<module>@<version>/` and cite
the function. For example, yaml.v3's merge-key test also accepts the tags
`!` and `tag:yaml.org,2002:merge`, which a version from memory misses.
`AGENTS.md` ("Releases and dependencies") names the reads for an updex or std
bump.

<a id="validate-discarded-merge-branches"></a>
#### Validate the merge branches that precedence discards

Configuration source-graph validation walks every branch that takes part in
merge precedence (`<<` merges and later keys), including the values a later
source overrides, and rejects malformed or over-limit data there too. Only
the winning values reach the effective result, but a malformed losing branch
is still an error (`internal/config/sourcegraph.go`, its tests in
`sourcegraph_test.go`).

<a id="derive-rules-from-the-canonical-struct"></a>
#### Derive validation rules from the canonical struct

Schema names (pages, groups, action fields) come from the yaml tags of the
canonical `Config` structs, not from a second hand-kept representation
([ADR-0005](../adr/0005-config-schema-reflected-from-canonical-struct.md)).
Where a mirror struct is needed (`rawGroupConfig`), a test proves it stays in
step with `GroupConfig`.

### Documentation

<a id="leaf-package-docs"></a>
#### Leaf-package docs enumerate, and change with the package

`docs/design/overview.md` and `docs/design/package-managers.md` name the
puregotk-free leaf packages under `internal/views/` by count and by name.
The change that adds or removes a leaf package also updates that count and
list. The prose for a leaf package names each distinct outcome and the
exported function that produces it (for example
`GroupDescriptionCheckFailed` for a failed check and `GroupDescription` for a
completed one), not a summary such as "handles every outcome", and says when
the call site composes no text of its own.

<a id="fix-contradicting-prose"></a>
#### Fix the old claim, not only add the new one

A documentation change for new behavior also searches the same file and its
sibling docs (`CONFIG.md`, `README.md`, `docs/design/`, `AGENTS.md`) for
sentences that state the old behavior, including restatements in notes and
FAQ sections, and changes them in the same change. See
[documentation-consistency.md](../documentation-consistency.md).

## Operational notes

- Add a lesson here when a defect or a review finding shows a rule that a
  test cannot enforce or that applies to more than one change. Record the
  correction itself in `.memory/corrections.jsonl` (ADR-0016).
- Remove a lesson when the code it names is gone.

## References

- Rationale: [ADR-0007](../adr/0007-pure-leaf-packages-route-around-untestable-gtk.md),
  [ADR-0005](../adr/0005-config-schema-reflected-from-canonical-struct.md),
  [ADR-0016](../adr/0016-adopt-the-repository-baseline.md)
- Related: [overview](overview.md), [package managers](package-managers.md),
  [documentation consistency](../documentation-consistency.md)
