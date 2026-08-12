# 0005 — Reflect the config schema from the canonical struct and validate strictly

- **Status:** Accepted
- **Date:** 2026-08-12

## Context

ChairLift's configuration is a two-level `<page>.<group>` YAML document whose
valid keys already exist in code: `Config`'s yaml tags name the pages and
`defaultConfig()`'s maps name the groups. Maintaining a second, hand-written
schema (a validation table, a JSON Schema file) would drift from the structs
the same way documentation does. At the same time, lenient YAML decoding is
dangerous here: a typo like `brew_updates_groop:` would be silently ignored,
leaving a group enabled that the file plainly meant to disable — the exact
failure [ADR-0003](0003-two-tier-config-with-fail-closed-semantics.md)'s
fail-closed rule exists to prevent. Finally, a config file must be able to
*clear* a default (e.g. `actions: []`) as well as inherit one, which plain
struct decoding cannot distinguish.

## Decision

The schema is derived by reflection from the canonical structs, in
`internal/config/schema.go`: `SchemaPages()` reads `Config`'s yaml tags,
`SchemaGroups(page)` reads the group names out of the value returned by
`defaultConfig()`, `SchemaGroupFields()` reads `rawGroupConfig`'s yaml tags
(the type yaml.v3 actually decodes into, held to `GroupConfig` by a parity
test in `schema_test.go`), and `SchemaActionFields()` reads `ActionConfig`'s.
There is no second schema artifact to update.

Validation is strict: `parseAndValidate` (`internal/config/validate.go`)
walks the parsed document and classifies every entry against the reflected
schema — an unknown page, group, or field name is a hard `KindSchema` error,
and a value of the wrong shape or type is a hard `KindParseType` error, both
of which fail closed per ADR-0003. Unknown keys are never skipped.

Files are field-by-field overlays, not replacements: `rawGroupConfig`
(`internal/config/config.go:64-72`) uses pointer-typed fields so yaml.v3
distinguishes *omitted* (nil — `mergeGroup` keeps `defaultConfig()`'s value)
from *present, possibly empty* (non-nil — the file's value replaces the
default outright, so an explicit `actions: []` clears the default)
(`internal/config/config.go:158-227`).

## Consequences

- Adding a page, group, or field is one edit to the canonical structs and
  `defaultConfig()`; the schema, validation, and `SchemaGroups` consumers
  follow automatically.
- Typos in config files are startup errors naming the unknown key, not
  silently ignored keys.
- `CONFIG.md`'s documented pages/groups must track `defaultConfig()`; the
  documentation-consistency checklist and its tests
  ([ADR-0010](0010-docs-are-a-ci-gated-artifact.md)) hold the prose to the
  code.
- Strictness cuts both ways: an old config naming a since-removed group
  fails closed after an upgrade rather than being ignored, so removing a
  group from the schema is a compatibility decision, not a cleanup.
- The overlay semantics (`omitted inherits, explicit empty clears`) are part
  of the file-format contract and documented in `CONFIG.md`'s Notes.

## Alternatives considered

- **Hand-maintained schema table or JSON Schema file:** rejected — a second
  source of truth that drifts from the structs; reflection makes drift
  impossible.
- **`yaml.KnownFields(true)` strict decoding alone:** rejected — it reports
  yaml.v3's error wording rather than the stable classified vocabulary of
  [ADR-0004](0004-configuration-error-diagnostic-vocabulary.md), and cannot
  express the per-level page/group/field diagnostics or the null-value
  no-op semantics `validate.go` implements.
- **Non-pointer decoding with zero-value sentinels:** rejected — cannot
  distinguish "omitted" from "explicitly empty", so a file could never clear
  a defaulted list like `maintenance_cleanup_group.actions`.

## References

- Shapes: [design/overview.md — Configuration](../design/overview.md#configuration),
  [CONFIG.md](../../CONFIG.md) (Available Pages and Groups; Notes)
- Builds on: [ADR-0003](0003-two-tier-config-with-fail-closed-semantics.md),
  [ADR-0004](0004-configuration-error-diagnostic-vocabulary.md)
- Enforced by: `internal/config/schema_test.go` (including the
  `rawGroupConfig`/`GroupConfig` tag-parity test),
  `internal/config/validate_test.go`, `internal/config/effectivekeys_test.go`,
  `internal/config/config_test.go` (merge/overlay semantics)
