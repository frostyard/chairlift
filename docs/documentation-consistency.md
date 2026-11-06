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
- Treat `README-go-port.md` and any plan under `docs/plans/` that is marked
  done as historical ([ADR-0016](adr/0016-adopt-the-repository-baseline.md)
  deleted `docs/superpowers/` and the finished plans). Current-state
  claims belong in `README.md`, `CONFIG.md`, `docs/index.md`,
  `docs/reference.md`, `docs/design/` (formerly `yeti/`), and `docs/specs/`.
- Run `make ci`, then grep current-state documentation for the obsolete term,
  key, version, or path that prompted the change.
- Search the changed file and its sibling docs for sentences that state the
  old behavior, including restatements in notes and FAQ sections, and fix
  them in the same change
  ([engineering lessons](design/engineering-lessons.md#fix-contradicting-prose)).
