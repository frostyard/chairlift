# A doc chunk must grep for and fix existing contradictory claims, not just add new prose

**When it applies:** Planning or reviewing any chunk that documents new or
changed behavior in a `.md` file (`CONFIG.md`, `README.md`, `yeti/*.md`,
`AGENTS.md`) — especially when the behavior already has *some* prose written
about it (an old default, an old fallback rule, an old "how this works"
paragraph) that the change makes wrong or incomplete.

**What to do:** Before scoping a doc chunk as "add a paragraph describing the
new behavior," grep the target file (and any sibling doc that describes the
same subsystem, e.g. `CONFIG.md` vs. `yeti/OVERVIEW.md`) for existing
sentences that state the old, now-contradicted behavior — including
restatements in a "Notes" or FAQ-style section, which tend to repeat a claim
in different words rather than reference the primary description once. Add
those exact locations to the chunk's acceptance criteria as concrete greps
(e.g. `grep -n 'all features enabled' CONFIG.md` returning nothing) so the
chunk is checkable rather than relying on a reviewer's careful re-read.
Writing accurate new prose next to stale old prose still leaves the doc
self-contradictory and will be rejected on plan review even though the
acceptance criterion asked only for the new behavior to be "documented."

**Learned from:** issue #60's mill run, plan round 1 — the plan added overlay
prose to `CONFIG.md`'s Notes section but left three pre-existing sentences
claiming "all features enabled" applies unconditionally (lines 13, 153-154),
even though `defaultConfig()` ships `maintenance_cleanup_group` disabled. The
reviewer rejected the plan (medium severity) because those exact-checkable
factual errors were left untouched. Round 2 fixed it by widening the chunk to
rewrite all three locations and adding grep-based acceptance criteria.
