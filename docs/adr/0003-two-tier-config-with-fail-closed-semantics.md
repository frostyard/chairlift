# 0003 — Load configuration from two ownership tiers and fail closed

- **Status:** Accepted
- **Date:** 2026-08-12

## Context

ChairLift's feature groups are configured per distribution: an image builder
ships defaults, an administrator overrides them locally, and a developer runs
from a checkout with neither installed. Package upgrades must be able to
replace maintainer defaults without ever clobbering administrator edits
(core ADR-0004's lifetime tiers). Separately, a configuration file that
exists but cannot be used is ambiguous: silently falling through to a
lower-priority file would run the app with settings its administrator
explicitly replaced, and silently using built-in defaults would enable
feature groups the broken file may have been written to disable.

## Decision

Configuration is searched in a fixed order of ownership tiers
(`internal/config/config.go:75-79`):

1. `/etc/chairlift/config.yml` — administrator-owned; never created,
   overwritten, or packaged by ChairLift's install paths.
2. `/usr/share/chairlift/config.yml` — package-owned maintainer defaults,
   replaceable on upgrade.
3. `config.yml` beside the executable, else under the current working
   directory — development fallback (`internal/config/paths.go:20`).

Only genuine absence advances the search: `Load`
(`internal/config/config.go:89-108`) continues past a candidate only when
the read fails with `fs.ErrNotExist`. The first existing file is
authoritative. If it is present but unusable — unreadable, unparseable, or
schema-invalid — `Load` returns `disabledConfig()`
(`internal/config/config.go:139-156`), which keeps the canonical pages and
groups but forces every feature group off, together with the diagnostic
described in [ADR-0004](0004-configuration-error-diagnostic-vocabulary.md).
Built-in defaults apply only when every candidate is absent.

Packaging honors the tiers: `make install` and both nFPM packages install
only the `/usr/share` copy and are test-forbidden from touching
`/etc/chairlift/config.yml` (`internal/installcheck/makefile_test.go:104-112`,
`internal/installcheck/goreleaser_test.go:144-147`).

## Consequences

- Administrator edits survive every package upgrade; maintainer defaults
  remain freely replaceable.
- A broken authoritative file is loud and safe: every group disappears
  until the file is fixed, rather than the app guessing which settings were
  intended. The cost is deliberate — a single typo in `/etc` blanks the UI
  until corrected, which is why the diagnostic (ADR-0004) must name the file
  and cause.
- Lower-priority files can never mask a broken higher-priority file, so
  "why is my /etc change ignored" cannot happen silently.
- Development checkouts work with zero installation via the relative
  fallback.

## Alternatives considered

- **Fall through to the next tier on any error:** rejected — runs with
  settings the administrator explicitly replaced, hiding the breakage.
- **Fall back to built-in defaults on error:** rejected — enables groups the
  broken file may exist to disable; fail-open on a file that plainly intends
  restrictions.
- **Merge all tiers (later files override earlier keys):** rejected — makes
  effective config the union of files no one can read in one place; the
  single-authoritative-file rule keeps "which file am I running" answerable.
- **XDG per-user config tier:** not needed — ChairLift configures
  system-scoped features; per-user config would let unprivileged users
  re-enable groups the administrator disabled.

## References

- Shapes: [design/overview.md — Configuration](../design/overview.md#configuration),
  [CONFIG.md](../../CONFIG.md) (search order, deployment, fail-closed notes)
- Related: [ADR-0004](0004-configuration-error-diagnostic-vocabulary.md),
  [ADR-0005](0005-config-schema-reflected-from-canonical-struct.md)
- Builds on: [core ADR-0004 — Product-namespaced filesystem paths, split by lifetime tier](https://github.com/frostyard/core/blob/main/docs/adr/0004-product-namespaced-filesystem-tiers.md)
- Enforced by: `internal/config/config_test.go`, `internal/config/paths_test.go`,
  `internal/installcheck/makefile_test.go`,
  `internal/installcheck/goreleaser_test.go`, and the wording pins in
  `internal/installcheck/documentation_test.go`
  ("configuration fallback wording is consistent")
