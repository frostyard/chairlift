# First-run translations

These gettext `.po` files translate ChairLift's first-run strings
(`internal/i18n/msgids.go`). `internal/i18n` embeds them with `go:embed` and
reads them with its own pure-Go parser: there is no `msgfmt` step and no
generated file.

## Source

- Repository: [frostyard/first-setup](https://github.com/frostyard/first-setup)
  (SNOW First Setup), directory `po/`.
- Commit: `b5f7de3` (`origin/main`, 2026-11-04).
- License: GPL-3.0, the license of first-setup. ChairLift is
  GPL-3.0-or-later, so the translations stay under the GPL. The translator
  credits are kept in the comment lines above each file's header entry.

## Port rule

The files were written once by a throwaway script that is not part of this
repository. For each language in first-setup's `po/LINGUAS`:

1. Keep the header entry (`msgid ""`) and the translator comment lines above
   it.
2. Keep only the entries whose msgid the first run uses, byte-identical to
   first-setup's msgid. Keep their translator comments; drop the `#:` source
   references (they name first-setup files).
3. Drop entries with an empty translation and entries flagged `fuzzy`.
4. Skip a language with 0 ported entries (`ne`).

One exception: the files `ar`, `be`, `ca`, `cs`, `da`, `de`, `eo`, `es`, `fa`,
`he`, `hu`, `ka`, `ko`, `ms`, `pl`, `pt`, `ro`, `sk` and `th` translate
"Connection available" with the text of an older msgid ("Connection
failed"). Their entry for that msgid is not ported, so those languages show
the English text. `TestConnectionAvailableIsNotPortedFromFailedTranslations`
pins this.

A new or changed first-run string starts untranslated: add its msgid to
`Messages` in `internal/i18n/msgids.go`. `TestEveryEmbeddedMsgidIsAFirstRunMsgid`
fails when a file holds a msgid that is not in `Messages`, or when a file does
not parse.
