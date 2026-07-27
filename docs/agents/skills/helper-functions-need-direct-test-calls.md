# "Every added helper must be called from the test file" means a literal, direct call — not coverage through the public entrypoint

**When it applies:** Any plan chunk whose acceptance criteria require that
every unexported helper function added in an implementation file be exercised
by its companion `_test.go` file. This is common in chunks that introduce
several small internal helpers (parsing, key-identity, traversal) behind one
exported function.

**What to do:** Reviewers check this criterion literally: if `foo.go` adds
`resolveNode`, `resolveMapping`, `mappingKeyID`, etc., the test file must
contain at least one call site naming each of those identifiers directly
(e.g. `mappingKeyID(node)` in a unit test), not merely a call into the
top-level exported function that happens to invoke them internally.
End-to-end coverage through the public API does not satisfy this criterion
even when it exercises every line — write a small direct unit test per
helper (or a table test that calls the helper by name) alongside the
integration-style tests. When drafting the chunk, list each helper's expected
direct-call test up front so it isn't missed during implementation.

**Learned from:** issue #69 phase-1 validator mill run — chunk 1 added several
resolve.go helpers (`resolveNode`, `resolveDocument`, `resolveAlias`,
`resolveMapping`, `mappingKeyID`, `parseTypeErr`, `resolveMergeSources`) but
`resolve_test.go` only called `resolveEffective` and `isMergeKey` directly.
The same "helpers not called directly from tests" objection recurred in
review rounds 1, 2, and 3, contributing to the run exhausting its review-round
limit and terminating as FAILED.
