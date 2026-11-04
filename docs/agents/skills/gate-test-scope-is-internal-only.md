# Unit and race gates run every Go test under `./...`

**When it applies:** Adding or reviewing any new `_test.go` file, or a plan
that proposes one, anywhere in this repo — especially under `cmd/` or the
repo root.

**What to do:** The ordinary unit and race gates now run every Go package with
`go test ./...`. This includes `make ci`, the Tests workflow's Unit Tests job,
and the Tests workflow's Race Detection job. Neither the unit nor race command
uses `-run` or `-skip`. The only separate test package is `test/e2e/`, run by
`make e2e` and not by ordinary unit or race gates.

`internal/installcheck.TestUnitTestGatesRunEveryTest` guards the workflow and
Makefile commands. It fails on `-run`, `-skip`, or a package pattern narrower
than `./...`. The allowed Makefile exception is `./test/e2e` in the `e2e`
target. Do not move tests only to a package outside `internal/` without a
complete gate; packages outside `internal/` now execute under `./...`, but
puregotk packages still cannot host tests because package initialization
panics on headless CI hosts.

If logic lives in a `cmd/` or view package that cannot build a headless test
binary, extract its decidable core into a pure package under `internal/` and
table-test that package. This mirrors the `gtk-headless-tests` constraint;
it does not narrow the repository-wide unit or race scope.

**Learned from:** issue #56 originally found that gates scoped tests only to
`./internal/...`. The helper subcommand coverage moved to
`internal/updexhelper`. A later gate change widened unit and race tests to
`./...`, so all ordinary test packages are now exercised while the puregotk
package-init rule remains separately enforced.
