# A "prove it's fully removed" grep criterion must exclude `.mill/` and must not fight a regression test that names the removed thing

**When it applies:** Planning or reviewing any chunk whose acceptance
criteria include a repo-wide `grep -rn <removed-identifier> .`-style check
asserting some name, key, flag, or symbol is now completely gone — especially
when the same chunk also adds a regression test meant to prevent that exact
thing from silently coming back.

**What to do:** A bare `grep -rn <name> .` (even with `--include`/`.git/`
filters) also walks `.mill/` — and `.mill/spec.md`, `.mill/plan.md`, and
`.mill/objections.json` necessarily keep naming the removed thing as the
historical record of *why* it was removed. That makes the criterion
unsatisfiable as literally written. Exclude `.mill/` (and `.git/`) from the
start: `grep -rn <name> . --exclude-dir=.git --exclude-dir=.mill`, rather than
enumerating an explicit allowlist of product/doc paths — an allowlist has to
be kept in sync as new files are added, while excluding the two fixed
tooling directories does not.

Separately, check whether the chunk's own new regression test needs to spell
the removed identifier to prove its absence (e.g. a test named after the key,
or a string-literal assertion quoting it) — if so, that test file will also
trip the same grep, since it lives outside `.mill/`. Resolve this by writing
the test so it never needs the literal name at all: assert the *exact*
remaining set (`len(x) == N` plus membership of every surviving entry) rather
than "does not contain `<removed-name>`". An exact-set check is strictly
stronger anyway — it also catches the same thing being re-added under a
*different* name — and it removes the conflict without touching the grep's
scope or the test's location.

**Learned from:** issue #58's mill run — plan round 1 was rejected because
its grep-based removal criterion walked `.mill/spec.md`/`plan.md`. Round 2's
fix (an explicit path allowlist) was itself rejected because the allowlist
still included `internal/` wholesale, which the same chunk's new
`config_test.go` also lives under and was written to name the removed key.
Round 3 converged only after fixing both at once: excluding `.mill/`
structurally instead of allowlisting, and rewriting the test as an exact-set
assertion that never spells the removed key's name.
