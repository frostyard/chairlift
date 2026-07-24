# Consistency-check tests must iterate every entry, not just index 0

**When it applies:** Writing or revising any regression test that asserts a
property against a real config file or generated collection with a slice/list
field — e.g. `.goreleaser.yaml`'s `nfpms[]`, `contents[]`, or any other
`[]T` parsed from a repo asset — where the spec's acceptance criterion is a
consistency invariant ("layout matches `/usr`", "all entries agree with X")
rather than a single fixed value.

**What to do:** Index into element `[0]` only as a first draft, then
immediately generalize before submitting for review: loop over the whole
slice (`for i, n := range cfg.Nfpms { ... }`) and assert the property on every
entry, not just the first. A test that hard-codes `cfg.Nfpms[0]` (or
equivalent) passes today but silently stops protecting anything the moment a
second entry is added or an existing one is reordered — which is exactly the
gap a reviewer checking the acceptance criterion will flag. If a review
objection says "only checks the first/one instance, iterate all", treat that
as a request to change the loop bound, not to add a comment or an extra
single-index assertion next to the existing one — a partial fix that still
special-cases index 0 will read as the same bug on the next review pass and
costs a full extra review round for no progress. When the collection could
legitimately be empty, also assert `len(...) > 0` (or otherwise fail loudly)
so the loop can't silently no-op.

**Learned from:** issue #59's mill run — `TestGoreleaserNfpmLayoutMatchesUsrPrefix`
in `internal/installcheck/goreleaser_test.go` checked only `cfg.Nfpms[0]`
against the acceptance criterion "the nFPM layout is tested for consistency
across all packages." The reviewer raised the identical objection across
three consecutive revision rounds (medium, then medium, then escalated to
high) because each revision left the single-index check in place instead of
looping over `cfg.Nfpms`; the chunk exhausted its review-round budget and the
run was terminated as failed without ever generalizing the loop.
