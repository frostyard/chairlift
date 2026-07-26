# A "rename-only" or "no production code changes" scope forbids new files too, even test-support ones

**When it applies:** Planning or reviewing any chunk whose spec/acceptance
criteria explicitly narrow the change to a rename, a doc fix, or otherwise say
"no production code changes" / "adding new test cases is out of scope" —
especially when the natural implementation instinct is to add a small helper
package, fixture, or shared constant to make the rename cleaner or more
DRY.

**What to do:** Take the scope restriction literally against the whole
working tree, not just against packages the spec mentions by name. A new file
under `internal/` — even one framed as "test-support," a names/constants
table, or a helper used only by `_test.go` files — is still source compiled
by `go build ./...` and still a new production file the moment it isn't
itself a `_test.go` file. If a rename or doc-only chunk starts to want a new
non-test file, that is a signal the chunk has grown scope, not a loophole in
the constraint. Either inline the needed value directly in each `_test.go`
file that uses it (accepting some duplication) or flag the scope conflict
back to the spec rather than planning around it.

**Learned from:** issue #76's mill run, plan round 1 — a plan chunk for a
five-test rename added `internal/testnames/testnames.go` plus two new tests
to centralize the old/new name mapping. The reviewer rejected it (high
severity) because the spec explicitly required a rename-only change with no
production code under `internal/` touched and no new test cases, and framing
the new file as "test support" didn't remove the conflict — it was still a
non-test `.go` file compiled by `go build ./...`.
