# Gates only run tests under `internal/...` — tests elsewhere are never enforced

**When it applies:** Adding or reviewing any new `_test.go` file, or a plan
that proposes one, anywhere in this repo — especially under `cmd/` or the
repo root.

**What to do:** Every gate that runs Go tests — `make ci` (the deep gate),
`.github/workflows/test.yml`'s Unit Tests and Race jobs, and `gates_chunk`'s
test line in `.mill/config.json` — scopes `go test` to `./internal/...`
specifically, not `./...`. (Plain `make test`, used only for ad hoc local
runs, does use `./...`, but that target is never what CI or the mill's gates
invoke — passing it locally proves nothing about what actually gets
enforced.) A test file placed in `cmd/chairlift-updex-helper`, `cmd/chairlift`,
or any other package outside `internal/` will build and even show green if
you run it directly with `go test ./...`, but no gate — local chunk gate,
deep gate, or GitHub CI — will ever execute it. It silently protects nothing.

If a spec requires regression coverage for logic that currently lives in a
`cmd/` package (e.g. a helper binary's subcommand dispatch), extract the
decidable logic into an `internal/` package the `cmd/` package calls into,
and put the table-driven test there. This mirrors the fix already required by
the `gtk-headless-tests` skill for puregotk packages — decidable logic
belongs in a small, dependency-light `internal/` package regardless of which
constraint (puregotk dlopen, or gate test scope) is pushing it out of the
package where it's easiest to write.

**Learned from:** issue #56's mill run, plan round 2 — a plan added
`cmd/chairlift-updex-helper/main_test.go` for helper subcommand coverage; the
reviewer objected that no gate exercises tests outside `./internal/...`. The
revised plan moved the logic into `internal/updexhelper` instead, which is
covered by `make ci` and CI.
