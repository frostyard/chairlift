# Documentation Consistency Checklist

The current-state vs historical classification below, and its enforcement by
string-matching unit tests in `internal/installcheck/documentation_test.go`,
are decision record
[ADR-0010](adr/0010-docs-are-a-ci-gated-artifact.md).

Use this checklist whenever behavior, configuration, dependencies, or
installation layout changes:

- Compare documented page/group keys with `config.yml`, `Config`, and the
  actual `IsGroupEnabled` guards. Run
  `go test ./internal/config ./internal/navigation`.
- Copy dependency versions from `go.mod`; do not retain a version claim from a
  plan or release note.
- Trace optional-tool visibility in the page builder and its async loader.
  Distinguish static config-driven page omission from runtime group hiding,
  unavailable placeholders, and visible error/status rows.
- Check commands and installation destinations against `Makefile`,
  `.goreleaser.yaml`, fixed helper constants, and PolicyKit annotations. Run a
  `make -n install` dry run when installation text changes.
- Treat `README-go-port.md` and files under `docs/plans/` (except category
  `TEMPLATE.md` files) and `docs/superpowers/` as historical. Current-state
  claims belong in `README.md`, `CONFIG.md`, `docs/index.md`,
  `docs/reference.md`, `docs/design/` (formerly `yeti/`), and `docs/specs/`.
- Run `make ci`, then grep current-state documentation for the obsolete term,
  key, version, or path that prompted the change.
