# Org-wide decisions (frostyard/core ADRs)

Conventions this repository follows that are decided at the org level are
recorded as ADRs in
[frostyard/core](https://github.com/frostyard/core/tree/main/docs/adr).
The ones that bind ChairLift:

- [ADR-0004 — Product-namespaced filesystem paths, split by lifetime tier](https://github.com/frostyard/core/blob/main/docs/adr/0004-product-namespaced-filesystem-tiers.md) — /etc/chairlift vs /usr/share/chairlift config precedence follows this
- [ADR-0005 — Transport discrimination by marker file and /run update-state contract](https://github.com/frostyard/core/blob/main/docs/adr/0005-native-ab-marker-and-update-state-files.md) — internal/sysupdate consumes /usr/lib/snosi/native-ab and /run/snosi/update-*
- [ADR-0010 — Publish packages through the shared repogen action](https://github.com/frostyard/core/blob/main/docs/adr/0010-publish-packages-via-repogen-to-r2.md) — release.yml publish step
- [ADR-0011 — Distro packages are named frostyard-<tool>](https://github.com/frostyard/core/blob/main/docs/adr/0011-frostyard-prefixed-package-names.md) — frostyard-chairlift / frostyard-chairlift-system-integration
- [ADR-0012 — svu-derived versions, make bump, and the rolling dev prerelease](https://github.com/frostyard/core/blob/main/docs/adr/0012-svu-versioning-and-rolling-dev-prerelease.md) — .svu.yaml, dev tag, snapshot concurrency group
- [ADR-0013 — Component releases trigger image rebuilds via repository_dispatch](https://github.com/frostyard/core/blob/main/docs/adr/0013-release-fanout-via-repository-dispatch.md) — snapshot.yml dispatches `build` to frostyard/snow
- [ADR-0015 — os-release is the image identity surface](https://github.com/frostyard/core/blob/main/docs/adr/0015-os-release-image-identity.md) — why transport detection uses the marker file, never IMAGE_ID
- [ADR-0016 — Reverse-DNS org.frostyard.* identifiers](https://github.com/frostyard/core/blob/main/docs/adr/0016-reverse-dns-org-frostyard-identifiers.md) — org.frostyard.ChairLift app ID, desktop file, polkit actions
- [ADR-0018 — Org-wide agent instruction and knowledge surfaces](https://github.com/frostyard/core/blob/main/docs/adr/0018-org-wide-agent-instruction-and-knowledge-surfaces.md) — AGENTS.md symlinks, docs/agents/skills, .memory/ (its yeti/ AI-docs directory is superseded on that point by ADR-0025)
- [ADR-0019 — Repository governance as machine-readable policy with risk tiers](https://github.com/frostyard/core/blob/main/docs/adr/0019-governance-as-code-and-risk-tiers.md) — .github/policies/, auto-qa-tuning, risk tiers
- [ADR-0020 — Trust boundaries for AI automation in CI](https://github.com/frostyard/core/blob/main/docs/adr/0020-ai-automation-trust-boundaries.md) — claude-code-review analyze/publish split, HTML idempotency markers
- [ADR-0021 — SHA-pinned actions and least-privilege CI workflows](https://github.com/frostyard/core/blob/main/docs/adr/0021-sha-pinned-actions-and-least-privilege-ci.md) — enforced by internal/installcheck/workflows_test.go
- [ADR-0022 — make ci is the canonical gate; TestI* is reserved](https://github.com/frostyard/core/blob/main/docs/adr/0022-make-ci-gate-and-test-naming-filter.md) — make ci, the Test[^I] filter, internal/installcheck pattern
- [ADR-0025 — One docs/ tree per repository, in core's four-category shape](https://github.com/frostyard/core/blob/main/docs/adr/0025-consolidate-repository-docs-into-docs.md) — docs/{adr,design,specs,plans} + indexed docs/README.md; yeti/ folded into docs/design/

When changing behavior covered by one of these, update or supersede the ADR
in frostyard/core first, then change this repo in the same effort.
