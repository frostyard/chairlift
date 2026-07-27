# A test asserting an interim chunk's contract must be listed for update in the later chunk that changes that contract

**When it applies:** Planning or revising a multi-chunk plan where an early
chunk (e.g. c4) adds a test asserting some intermediate behavior — a value
"survives as an ordinary key," a field defaults to some placeholder, a
partial implementation returns a simplified result — and a later chunk (e.g.
c5) is explicitly designed to change that behavior into its final form.

**What to do:** When a later chunk supersedes behavior a test in an earlier
chunk asserts, that later chunk's `files` list must include the test file
that made the now-false assertion, and its acceptance criteria must say
whether the assertion is replaced or removed. Don't assume "the later chunk
implements the real behavior, so of course the old test gets fixed along the
way" — the chunk was reviewed and approved without naming the file, so
nothing forces it to happen, and the chunk fails its own test gate when the
now-stale assertion still runs. Before finalizing any chunk that changes
prior behavior, grep the test files touched by earlier chunks for assertions
about the specific case being changed, and add those files to the new
chunk explicitly.

**Learned from:** issue #69 phase-1b mill run, plan review round 4 — c4 added
a `resolve_test.go` assertion that a `<<` key survives as an ordinary key
when merge is only partially resolved; c5 makes the final result merge-free
(the `<<` key is fully consumed), which makes that exact assertion false, but
c5's plan neither listed `resolve_test.go` in `files` nor described replacing
the assertion. The reviewer caught it before implementation, but the same
gap recurred across earlier rounds too (rounds 1-3 each found a similar
"later chunk invalidates something the plan didn't track" issue), and the
plan exhausted its 4-round revision limit and the run terminated as FAILED
without ever reaching implementation.
