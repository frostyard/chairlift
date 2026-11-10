# First-run translations

This directory holds the gettext `.po` catalogs that translate ChairLift's
first-run strings (`internal/i18n/msgids.go`). `internal/i18n` embeds the
directory with `go:embed` and reads each catalog with its own pure-Go parser:
there is no `msgfmt` step and no generated file.

## Current state

No translation is shipped. The first run shows its English strings in every
locale. The catalogs ChairLift once carried were the work of SNOW First
Setup's translators, under First Setup's copyleft license; ChairLift is
MIT-licensed, so they were removed ([ADR-0018](../../../docs/adr/0018-relicense-mit-and-drop-inherited-translations.md)).
The parser, the locale choice and the tests stay, so a translation can be
added at any time.

## Add a translation

1. Create `<language>.po` in this directory. The name is what `Candidates`
   (`internal/i18n/i18n.go`) looks for: `ll` (for example `de.po`) or
   `ll_CC` (for example `pt_BR.po`); Chinese uses `zh_Hans.po` and
   `zh_Hant.po`. The codeset and modifier of a locale are not part of the
   name.
2. Start with a header entry (`msgid ""`) whose `Content-Type` is
   `text/plain; charset=UTF-8`, then add one `msgid`/`msgstr` entry per
   string you translate. Each `msgid` is a constant of `Messages` in
   `internal/i18n/msgids.go`, copied byte for byte; keep a `{}` placeholder
   (in `MsgHello`) in the translation.
3. The parser supports comment lines, multi-line strings and C escapes. It
   skips entries with an empty `msgstr` and entries flagged `#, fuzzy`.
   `msgctxt` and plural forms are syntax errors.
4. Run `go test ./internal/i18n`.
   `TestEveryEmbeddedMsgidIsAFirstRunMsgid` fails when a catalog does not
   parse, holds no translation, or holds a msgid that is not in `Messages`.

A string the catalog does not translate shows the English msgid. A new or
changed first-run string starts untranslated in every catalog: add its msgid
to `Messages` and update the catalogs that should translate it.

## License of contributions

ChairLift is MIT-licensed (`LICENSE`). A translation is accepted only when
its author contributes it under the MIT license. Translate from the English
msgids: copy nothing from SNOW First Setup, Vanilla OS First Setup, or any
other source under a copyleft or otherwise incompatible license, including
their `.po` files and the translation memories built from them.
