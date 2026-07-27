# Before planning a new package-level helper or var, grep existing `_test.go` files in that package

**When it applies:** Planning or reviewing any chunk that introduces a new
package-level function, var, or const — especially a small utility like a
name-lookup helper or a list of known keys — in a package that already has
one or more `_test.go` files, whether or not those test files are in the
chunk's `files` list.

**What to do:** Go test files compile into the same package as the
production code and share its identifier namespace, so a new
`func groupKeys(page PageConfig) ...` or `var pageNames = ...` collides with
an identically named helper or var already declared in
`internal/whatever/whatever_test.go`, even though that file is never touched
by the chunk and even though Go allows no overloading to disambiguate them.
This breaks the build the moment the chunk lands, and a plan that explicitly
restricts the chunk to only its listed new files will not resolve the
collision by renaming the pre-existing one. Before drafting a chunk that adds
any package-level identifier, grep every `_test.go` file already in the
target package for that exact name (and close variants) — not just the
package's non-test `.go` files — and pick a name or scope that avoids the
clash, or explicitly widen the chunk to include the rename.

**Learned from:** issue #69's mill run — plan round 1 proposed a
`groupKeys(page)` helper that collided with an existing `groupKeys(page
PageConfig)` already defined in `internal/config/config_test.go`; round 2's
fix (a `pageNames()` func) collided the same way with a pre-existing
package-level `var pageNames` at config_test.go line 18. Both rounds were
rejected because Go forbids the redeclaration and the chunk's file scope
excluded editing the test file.
