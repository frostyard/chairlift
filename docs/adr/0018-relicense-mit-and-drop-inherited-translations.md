# 0018 — Relicense ChairLift under MIT and drop the inherited translations

- **Status:** Accepted
- **Date:** 2026-11-10

## Context

ChairLift began as an adaptation of Vanilla OS First Setup and carried that
project's license, GPL-3.0-or-later. It has since been rewritten. The window is Go on MyGo
([ADR-0017](0017-mygo-command-center-window.md)), the first-run mode is Go
([ADR-0014](0014-first-run-mode.md)), and none of First Setup's code or
sentences remain (decision 5).
Brian Ketelsen holds the copyright on all of ChairLift's code, so he can
relicense it.

One part of the repository was not his work. ADR-0014's decision 8 ported the
first-run translations of SNOW First Setup (frostyard/first-setup commit
`b5f7de3`) into `internal/i18n/po/*.po`: 51 languages, each written by First
Setup's translators and licensed by them under First Setup's license,
GPL-3.0. Those files cannot be relicensed by ChairLift's owner, and the
binary embeds them.

The rest of the translation machinery is ChairLift's own: the `.po` parser
(`ParsePO`), gettext's locale choice (`Candidates`), the `Translator`,
`Format`, and the msgid list (`internal/i18n/msgids.go`).

## Decision

1. **ChairLift is MIT-licensed.** `LICENSE` holds the MIT text with
   `Copyright (c) 2025-2026 Brian Ketelsen`. The package metadata says the
   same: `metadata.license` and every `nfpms[].license` in
   `.goreleaser.yaml` are `MIT`, pinned by
   `internal/installcheck.TestGoreleaserLicenseIsMIT`. The About dialog
   (`Main.showAbout`) names "MIT".
2. **The inherited translations are deleted.** Every
   `internal/i18n/po/*.po` file is removed, and no translation ships. The
   first run shows its English strings in every locale. This amends
   ADR-0014's decision 8: the embedded `.po` machinery stays, but nothing is
   ported from First Setup, and a msgid no longer has to match First Setup's.
3. **The machinery stays ready for new catalogs.** `internal/i18n` embeds
   the `po` directory (`//go:embed po`), because a `po/*.po` pattern that
   matches no file does not build; `po/README.md` is embedded with it. Each
   catalog is a `<name>.po` file at the root of that directory.
   `NewTranslator` takes the catalog filesystem (`fs.FS`): `T` reads the
   embedded one, and the tests write their own `fstest.MapFS` catalogs.
   `TestEveryEmbeddedMsgidIsAFirstRunMsgid` passes with no catalog and
   checks every catalog that is added.
4. **Contributions are MIT, and nothing is copied from a copyleft source.**
   A translation, like any contribution, is contributed under the MIT
   license and written for ChairLift. Nothing is copied from SNOW First
   Setup, Vanilla OS First Setup or any other copyleft or incompatibly
   licensed source. `CONTRIBUTING.md` and `internal/i18n/po/README.md` say
   so.
5. **The first run's sentences are written again.** The English msgids that
   ADR-0014 ported were First Setup's text too. Every sentence-length msgid in
   `internal/i18n/msgids.go` is now in ChairLift's own words, and
   `TestNoMsgidCopiesFirstSetup` fails when a msgid equals one of First
   Setup's sentences; it compares SHA-256 hashes, so the test does not carry
   them. Single words and short labels (Back, Next, Light, Dark Mode, Open
   Settings, Applications and the like) stay: they are common UI words, the
   ordinary names of those controls. One test fixture also stays:
   `internal/firstrun/testdata/first-setup-apps.json` is first-setup's
   `apps.json`, a list of application names and Flatpak IDs. It is factual
   data that proves ChairLift reads the file the images ship, and no binary
   embeds it.

## Consequences

- No file in the repository carries the earlier license. Releases tagged
  before this change keep the license they shipped with; Git history keeps
  the earlier `LICENSE` and the deleted catalogs.
- Users with a non-English locale lose the first-run translations they had
  and see English until MIT translations are contributed.
- No first-run sentence had to keep First Setup's wording for a translation
  to apply, because no catalog ships, so the new wording costs no translation.
- The `internal/i18n` tests no longer exercise real catalogs. They prove the
  parser, the locale choice and the per-message fallback with catalogs that
  the tests write.
- A reviewer of a translation pull request must check that it is original
  work contributed under MIT. A test cannot check that.
- The package license and `LICENSE` must still agree;
  `TestGoreleaserLicenseIsMIT` fails when they drift.

## Alternatives considered

- **Keep the earlier license and the translations:** rejected — the owner
  chose MIT for the code he wrote, and the translations are the only part
  that blocked it.
- **Ask First Setup's translators to relicense their work:** rejected — the
  51 languages have many contributors, often known only by a
  translation-platform name; each consent would have to be found and
  recorded, for a few dozen short strings that can be translated again.
- **Keep the catalogs under their own license beside MIT code:** rejected —
  the binary embeds them, so every release would combine GPL-3.0 and MIT
  work, and the repository would carry two licenses.
- **Remove `internal/i18n` too:** rejected — it is ChairLift's own small,
  tested code, and keeping it lets MIT translations be added without a new
  design.

## References

- Shapes: [design/overview.md — First-run mode](../design/overview.md#first-run-mode)
  (Translations),
  [design/package-managers.md — Install-path consistency](../design/package-managers.md),
  `internal/i18n/po/README.md`, [CONTRIBUTING.md](../../CONTRIBUTING.md),
  [README.md](../../README.md)
- Amends: [ADR-0014](0014-first-run-mode.md) (decision 8, the translations)
- Builds on: [ADR-0016](0016-adopt-the-repository-baseline.md) (the
  repository baseline's license row, which the MIT `LICENSE` meets)
- Enforced by: `internal/installcheck.TestGoreleaserLicenseIsMIT`,
  `internal/i18n` tests (`TestEveryEmbeddedMsgidIsAFirstRunMsgid`,
  `TestNoMsgidCopiesFirstSetup` and the translator tests)
