# A spec that authorizes widening one frozen allowlist/list for exactly one named entry does not cover any other identifier

**When it applies:** Planning or reviewing any chunk that calls an unexported
helper directly from a test file guarded by a frozen allowlist/list constant
(e.g. `directCallAllowlist` in `internal/config/sourcesurface_test.go`), where
the spec text explicitly authorizes adding exactly one new named entry to that
list for one new function.

**What to do:** Treat that authorization as scoped to the single identifier
named in the spec, not as blanket permission for the chunk (or a later chunk)
to call any other unexported helper directly. Before writing a chunk that has
a test directly call a second unexported function (e.g. `mergeConfig`) and
claims "it's already in the allowlist," grep the actual list literal in the
test file to confirm that identifier is really present. If it isn't, either
add a new, separately-justified authorization to the plan (the spec is
frozen, so this usually means restructuring the chunk to exercise that helper
indirectly through the authorized entry point) rather than asserting it's
already covered.

**Learned from:** issue #69 phase1c mill run — plan round 1's chunk c5 called
the unexported `mergeConfig` from `validatemerge_test.go`, claiming it was
already in `directCallAllowlist`. The spec had authorized only
`"parseAndValidate": true` for that list; `mergeConfig` was never added,
`TestSourceHelperDirectCallSurface` would fail, and the chunk explicitly
forbade changing `sourcesurface_test.go` to fix it, making the chunk
unplannable as written.
