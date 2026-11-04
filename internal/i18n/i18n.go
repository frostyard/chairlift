package i18n

import (
	"embed"
	"log"
	"os"
	"path"
	"strings"
	"sync"
)

// poFiles holds the translations ported from frostyard/first-setup. See
// po/README.md for the source, the license and the port rule.
//
//go:embed po/*.po
var poFiles embed.FS

// chineseScripts maps the region forms of Chinese locales to the script file
// names that first-setup's translations use.
var chineseScripts = map[string]string{
	"zh_CN": "zh_Hans",
	"zh_SG": "zh_Hans",
	"zh_TW": "zh_Hant",
	"zh_HK": "zh_Hant",
	"zh_MO": "zh_Hant",
}

// Candidates returns the catalog names to try for the environment, in order,
// with gettext's rules. The locale is the first non-empty value of LC_ALL,
// LC_MESSAGES and LANG. When it is empty, "C" or "POSIX", the result is nil
// (English) and LANGUAGE is ignored. Otherwise each entry of the
// colon-separated LANGUAGE list comes first, then the locale itself. For each
// entry the codeset (".UTF-8") and the modifier ("@latin") are removed, then
// "ll_CC" is tried before "ll". Chinese region forms map to the zh_Hans and
// zh_Hant file names. Duplicates are removed.
func Candidates(getenv func(string) string) []string {
	locale := ""
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := getenv(name); value != "" {
			locale = value
			break
		}
	}
	if base := stripLocale(locale); base == "" || base == "C" || base == "POSIX" {
		return nil
	}

	var entries []string
	for _, entry := range strings.Split(getenv("LANGUAGE"), ":") {
		if entry != "" {
			entries = append(entries, entry)
		}
	}
	entries = append(entries, locale)

	seen := make(map[string]bool)
	var candidates []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			candidates = append(candidates, name)
		}
	}
	for _, entry := range entries {
		base := stripLocale(entry)
		if script, ok := chineseScripts[base]; ok {
			add(script)
		}
		add(base)
		if language, _, found := strings.Cut(base, "_"); found {
			add(language)
		}
	}
	return candidates
}

// stripLocale removes the codeset and the modifier from a locale name:
// "de_DE.UTF-8@euro" becomes "de_DE".
func stripLocale(locale string) string {
	locale, _, _ = strings.Cut(locale, "@")
	locale, _, _ = strings.Cut(locale, ".")
	return locale
}

// Translator looks a msgid up in an ordered list of catalogs. The first
// catalog with a translation wins, so a string that one language lacks falls
// back to the next language of LANGUAGE, then to the English msgid.
type Translator struct {
	languages []string
	catalogs  []map[string]string
}

// NewTranslator loads the embedded catalogs that Candidates names for the
// environment. A candidate with no embedded file is skipped. A file that does
// not parse is logged and skipped (the unit tests prove every embedded file
// parses).
func NewTranslator(getenv func(string) string) *Translator {
	t := &Translator{}
	for _, name := range Candidates(getenv) {
		data, err := poFiles.ReadFile(path.Join("po", name+".po"))
		if err != nil {
			continue
		}
		catalog, err := ParsePO(data)
		if err != nil {
			log.Printf("i18n: ignoring po/%s.po: %v", name, err)
			continue
		}
		t.languages = append(t.languages, name)
		t.catalogs = append(t.catalogs, catalog)
	}
	return t
}

// Languages returns the names of the catalogs the translator loaded, in
// lookup order. It is empty for English.
func (t *Translator) Languages() []string {
	return append([]string(nil), t.languages...)
}

// T returns the translation of msgid, or msgid itself when no loaded catalog
// translates it.
func (t *Translator) T(msgid string) string {
	for _, catalog := range t.catalogs {
		if translated, ok := catalog[msgid]; ok {
			return translated
		}
	}
	return msgid
}

// processTranslator is the translator for the process environment, made on
// first use.
var processTranslator = sync.OnceValue(func() *Translator {
	return NewTranslator(os.Getenv)
})

// T translates msgid for the locale of the process environment.
func T(msgid string) string {
	return processTranslator().T(msgid)
}

// Format replaces each "{}" placeholder of a translated string with the next
// argument, in order, as first-setup's Python str.format calls did. Extra
// placeholders stay as they are; extra arguments are ignored.
func Format(translated string, args ...string) string {
	var out strings.Builder
	rest := translated
	for _, arg := range args {
		before, after, found := strings.Cut(rest, "{}")
		if !found {
			break
		}
		out.WriteString(before)
		out.WriteString(arg)
		rest = after
	}
	out.WriteString(rest)
	return out.String()
}
