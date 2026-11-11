# Documentation

ChairLift keeps one documentation tree, `docs/`, in frostyard/core's
four-category shape
([core ADR-0025](https://github.com/frostyard/core/blob/main/docs/adr/0025-consolidate-repository-docs-into-docs.md);
the former `yeti/` AI-docs directory is folded in here). Docs are split by
the question they answer:

| Directory | Question | Contents |
| --- | --- | --- |
| [adr/](adr/) | **Why** did we choose this? | Repo-local Architecture Decision Records — immutable once accepted; superseded, never edited. Org-wide decisions go to frostyard/core instead (see [org-adrs.md](org-adrs.md)) |
| [design/](design/) | **How** does it fit together? | Living documents describing the current architecture; [design/overview.md](design/overview.md) is the entry point |
| [specs/](specs/) | **What exactly** is the contract? | Precise, testable interface definitions, changed only alongside implementing code |
| [plans/](plans/) | **When/in what order** do we build? | Phased plans with "Done when" outcomes |

The user-facing docs-site home is [index.md](index.md) (rendered by
`mkdocs.yml`); this file is the contributor-facing index of everything under
`docs/`.

## Index

### Decisions (ADRs)

Repo-local decisions get the next free number from
[adr/TEMPLATE.md](adr/TEMPLATE.md); org-wide decisions that bind this repo
are listed in [org-adrs.md](org-adrs.md).

- [adr/0001-fixed-path-pkexec-privilege-boundary.md](adr/0001-fixed-path-pkexec-privilege-boundary.md)
  — every root mutation goes through pkexec at hardcoded absolute paths
  matching the polkit `exec.path`/`exec.argv1` annotations; the helper
  re-validates full argv; no passwordless rules ever ship
- [adr/0002-usr-prefix-is-the-only-supported-install-prefix.md](adr/0002-usr-prefix-is-the-only-supported-install-prefix.md)
  — `PREFIX=/usr` is the only supported install prefix (polkitd's fixed
  actions directory, pkexec's absolute path match); `DESTDIR` layers under it
- [adr/0003-two-tier-config-with-fail-closed-semantics.md](adr/0003-two-tier-config-with-fail-closed-semantics.md)
  — `/etc/chairlift` → `/usr/share/chairlift` → dev fallback; only absence
  advances the search; a present-but-broken file disables every feature group
- [adr/0004-configuration-error-diagnostic-vocabulary.md](adr/0004-configuration-error-diagnostic-vocabulary.md)
  — fixed greppable `CONFIGURATION ERROR` log prefix, persistent toast, and
  the stable `ErrorKind` classification vocabulary
- [adr/0005-config-schema-reflected-from-canonical-struct.md](adr/0005-config-schema-reflected-from-canonical-struct.md)
  — the config schema is reflected from `Config`/`defaultConfig()` yaml tags;
  unknown keys hard-error; files are field-by-field overlays (explicit empty
  clears, omitted inherits)
- [adr/0006-split-system-integration-package-with-mutual-conflicts.md](adr/0006-split-system-integration-package-with-mutual-conflicts.md)
  — self-contained `frostyard-chairlift` vs GUI-less
  `frostyard-chairlift-system-integration`, conflicting both ways, for
  user-scoped GUI installs such as the Homebrew cask
- [adr/0007-pure-leaf-packages-route-around-untestable-gtk.md](adr/0007-pure-leaf-packages-route-around-untestable-gtk.md)
  — puregotk-importing packages stay test-free; all decidable logic lives in
  headless leaf packages with wiring tests proving the page builders use them
  (superseded by ADR-0017)
- [adr/0008-e2e-readiness-is-a-log-marker-contract.md](adr/0008-e2e-readiness-is-a-log-marker-contract.md)
  — E2E startup readiness is three exact stdout markers polled under
  dbus-run-session + xvfb-run; the log lines are a public API
- [adr/0009-dry-run-output-convention-and-single-decision-structs.md](adr/0009-dry-run-output-convention-and-single-decision-structs.md)
  — per-wrapper `SetDryRun`/`IsDryRun`, fixed `[DRY-RUN]` message prefixes,
  and single tested decision structs gating toast + UI mutation together
- [adr/0010-docs-are-a-ci-gated-artifact.md](adr/0010-docs-are-a-ci-gated-artifact.md)
  — documentation splits into current-state vs historical and is enforced by
  string-matching unit tests; prose is testable
- [adr/0011-unprivileged-feature-check-compares-with-the-booted-image.md](adr/0011-unprivileged-feature-check-compares-with-the-booted-image.md)
  — the unprivileged feature check gives updex a bootc runner that turns a
  root-only `bootc status` failure into the existing os-release fallback, so a
  row can show that it waits for a newer OS image; the root helper keeps the
  real bootc and fails closed
- [adr/0012-firmware-updates-use-fwupds-own-polkit.md](adr/0012-firmware-updates-use-fwupds-own-polkit.md)
  — firmware installs run the unprivileged `fwupdmgr update
  --no-reboot-check <DeviceId>` and fwupd's own PolicyKit actions authorize
  the daemon; ChairLift ships no policy file and no helper for firmware, and
  the group was off by default until a hardware test (that default is
  superseded by ADR-0013)
- [adr/0013-firmware-updates-are-on-by-default.md](adr/0013-firmware-updates-are-on-by-default.md)
  — `firmware_updates_group` is enabled by default after the test on real
  hardware passed (2026-11-03); the rest of ADR-0012 stays
- [adr/0014-first-run-mode.md](adr/0014-first-run-mode.md)
  — `chairlift --first-run` replaces SNOW First Setup: a pure-Go
  `chairlift-first-run` gate, its own application ID, one marker writer that
  also writes first-setup's marker until the dated end rule, no autostart in
  the deb, the `first_run_page` config, and embedded `.po` translations (the
  translations are amended by ADR-0018: the ported catalogs are deleted)
- [adr/0015-problems-page-and-opt-in-reporting.md](adr/0015-problems-page-and-opt-in-reporting.md)
  — a 7th page, Problems, shows snosi-health's system problems, the user's
  crashes and frostyard app records; every value is checked against
  `frostyard.problem/1` again; reporting is opt-in per user (ask each time or
  send automatically, 20 a day, one weekly count, an admin-only endpoint, and
  no `crash` report in phase 2); ChairLift records its own panics and 4 coded
  errors through std `problems`
- [adr/0016-adopt-the-repository-baseline.md](adr/0016-adopt-the-repository-baseline.md)
  — core's repository baseline: the template gate, `make verify`/`check`/`ci`
  with `mise.toml` pins, `ci.yml`, risk paths, core skills in
  `.agents/skills/`, the mill lessons folded into one design doc, and the
  framework trees deleted (amends ADR-0010's historical-docs list)
- [adr/0017-mygo-command-center-window.md](adr/0017-mygo-command-center-window.md)
  — the window is a MyGo command center (Overview, Updates, Apps, Features,
  Problems, Maintenance, a palette, a console and a status bar) on GTK 3;
  puregotk is gone, views are tested with MyGo's Tester, the console streams
  real command output through `internal/cmdlog`, changes run in one serial
  job queue, and the UI kit is the `github.com/frostyard/frostui` module
  (supersedes ADR-0007)
- [adr/0018-relicense-mit-and-drop-inherited-translations.md](adr/0018-relicense-mit-and-drop-inherited-translations.md)
  — ChairLift is MIT-licensed (`LICENSE`, the package metadata, the About
  dialog); the first-run translations ported from SNOW First Setup are
  deleted because their translators' license cannot be changed, and the
  first run's sentences are written again; the `.po`
  machinery stays, reads an injectable `fs.FS`, and new catalogs must be
  contributed under MIT (amends ADR-0014's translations)

### Design

- [design/overview.md](design/overview.md) — architecture entry point:
  purpose, the command-center window and its six pages, dependency flow, key
  patterns (the UI thread, the serial job queue, dry run, the console, the
  palette, the status bar, live figures, recent activity), the Problems page,
  the first-run mode, configuration, build and release (formerly
  `yeti/OVERVIEW.md`)
- [design/package-managers.md](design/package-managers.md) — the Homebrew,
  Flatpak, fwupd, bootc, sysupdate, and updex wrappers and their view-layer
  leaf packages (formerly `yeti/package-managers.md`)
- [design/frostui.md](design/frostui.md) — where the frostyard desktop UI
  kit on MyGo lives now (the `github.com/frostyard/frostui` module), and what
  ChairLift draws itself
- [design/engineering-lessons.md](design/engineering-lessons.md) — the
  durable test, configuration-code and documentation rules learned from past
  defects and reviews (formerly the lesson files in `docs/agents/skills/`);
  code comments link its anchors

### Specs

- [specs/problems-page.md](specs/problems-page.md) — the Problems page's
  sources and limits, the problems.json outcomes, the validation and
  reportable rules, the merge rule, the dismiss store, the reporting card
  and its Report dialog, the dry-run fixture hook, and the
  `problems: page built` log marker
- [specs/problem-reporting.md](specs/problem-reporting.md) — the opt-in
  sender: the per-user consent file and its schema generations, the sent log,
  the HTTP request and its limits, the automatic and manual run rules, the
  20-a-day budget, the 24-hour pause after a failure, the one-sender lock, the
  admin-only endpoint, and the `--send-pending-reports` exit codes

### Plans

No open plans. A new plan starts from [plans/TEMPLATE.md](plans/TEMPLATE.md).
The finished plans of past automated runs were deleted with
[ADR-0016](adr/0016-adopt-the-repository-baseline.md); Git history keeps them.

### Process and policy docs (uncategorized, indexed in place)

- [index.md](index.md) — user-facing overview: pages, shortcuts, optional
  dependencies, building and installing (docs-site home)
- [reference.md](reference.md) — user-facing configuration/behavior reference
- [quality.md](quality.md) — quality dashboard: CI, coverage, release signals
- [metrics.md](metrics.md) and [metrics/README.md](metrics/README.md) —
  metrics definitions and the public metrics catalog
- [documentation-consistency.md](documentation-consistency.md) — checklist
  for keeping current-state docs in sync with source; defines which docs are
  current-state vs historical
- [risk-tiers.md](risk-tiers.md) — change risk classification used by PRs
- [review-rubric.md](review-rubric.md) — pull request review rubric
- [SECURITY-AI.md](SECURITY-AI.md) — AI security policy
- [org-adrs.md](org-adrs.md) — frostyard/core ADRs that bind this repository
- [prompts/index.md](prompts/index.md) — reusable agent prompt catalog:
  [implement-issue.md](prompts/implement-issue.md),
  [review-change.md](prompts/review-change.md),
  [reconcile-documentation.md](prompts/reconcile-documentation.md)

## Conventions

- **New docs start from their category's `TEMPLATE.md`** (in each directory).
- New repo-local decision → new ADR with the next number; if it reverses an
  old one, mark the old one `Superseded by NNNN` rather than editing it.
  Org-wide decisions become ADRs in frostyard/core plus a line in
  [org-adrs.md](org-adrs.md).
- Design docs are updated in place to always reflect reality.
- Specs change only alongside the code that implements them.
- Cross-links between categories are mandatory in both directions.
- Adding a doc means adding it to the index above.
- Doc-consistency tests in `internal/installcheck` pin literal paths and
  claims in these docs; when docs and tests disagree, update both in the
  same commit.
