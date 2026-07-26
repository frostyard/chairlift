# Grep-based acceptance criteria over test names must account for subtests and for other chunks' own criteria

**When it applies:** Planning or reviewing any chunk whose acceptance
criteria include (a) a grep-based count of `go test -list`/`RUN` lines
expected to change by some exact number, or (b) a `grep -P '\bWORD\b'`-style
"this name must not appear" check — especially in a multi-chunk plan where a
later chunk's acceptance criteria also grep for text containing that same
word (e.g. documenting the convention by naming the pattern in prose).

**What to do:** For RUN-line/test-count deltas: `go test -list`/`-v` output
includes one line per top-level test *and* one per subtest (`t.Run(...)`), so
renaming or adding parent tests that already contain table-driven or
`t.Run`-based subtests changes the count by more than the number of renamed
identifiers. Compute the expected delta by actually listing tests before and
after (or reasoning subtest-by-subtest), not by counting renamed function
names. For `\b`-anchored greps: GNU grep's `\b` treats an ellipsis character
and most punctuation as a word boundary, so a criterion like
`grep -P '\bTestInstall\b'` returning nothing can directly conflict with a
sibling chunk's requirement that some doc literally contain the string
`TestInstall…` (three dots) as a worked example — both are technically true
readings of "the old name must be gone" vs. "the doc must explain the
mistake," but they cannot both hold over the same tree. When a plan spans
multiple chunks, cross-check every grep-based acceptance criterion against
every other chunk's criteria for the same identifiers before finalizing —
don't verify each chunk's checks only in isolation.

**Learned from:** issue #76's mill run, plan round 2 — round 2 was rejected
twice: once because a RUN-line-count criterion assumed +5 lines for five
renamed tests but two of them had two subtests each (actual delta +9), and
once because c1's `grep -P '\bTestInstall\b'` (must return nothing) directly
contradicted c2's requirement that `AGENTS.md` name the exact accident
`TestInstall…`, since GNU grep's word-boundary matches at the ellipsis.
