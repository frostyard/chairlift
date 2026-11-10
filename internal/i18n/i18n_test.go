package i18n

import (
	"fmt"
	"io/fs"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

func envOf(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestParsePO(t *testing.T) {
	const data = `# translator comment
msgid ""
msgstr ""
"Language: de\n"
"Content-Type: text/plain; charset=UTF-8\n"

#: some/file.py:1
msgid "Next"
msgstr "Weiter"

msgid ""
"Multi "
"line"
msgstr ""
"Mehr"
"zeilig"

msgid "Escapes"
msgstr "a\"b\\c\nd\te"

#, fuzzy
msgid "Fuzzy"
msgstr "Unscharf"

#, python-format, fuzzy
msgid "Fuzzy two"
msgstr "Unscharf zwei"

msgid "Empty"
msgstr ""
#, fuzzy
msgid "After empty"
msgstr "Nach leer"
msgid "Adjacent"
msgstr "Benachbart"
`
	got, err := ParsePO([]byte(data))
	if err != nil {
		t.Fatalf("ParsePO() error = %v", err)
	}
	want := map[string]string{
		"Next":       "Weiter",
		"Multi line": "Mehrzeilig",
		"Escapes":    "a\"b\\c\nd\te",
		"Adjacent":   "Benachbart",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePO() = %#v, want %#v", got, want)
	}
}

func TestParsePORejectsBadSyntax(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"msgctxt", "msgctxt \"menu\"\nmsgid \"a\"\nmsgstr \"b\"\n", "unsupported keyword"},
		{"plural", "msgid \"a\"\nmsgid_plural \"as\"\nmsgstr[0] \"b\"\n", "unsupported keyword"},
		{"indexed msgstr", "msgid \"a\"\nmsgstr[0] \"b\"\n", "unsupported keyword"},
		{"string without keyword", "\"loose\"\n", "no msgid or msgstr keyword"},
		{"msgid without msgstr", "msgid \"a\"\n\n", "has no msgstr"},
		{"msgid without msgstr at end", "msgid \"a\"", "has no msgstr"},
		{"two msgids", "msgid \"a\"\nmsgid \"b\"\nmsgstr \"c\"\n", "without a msgstr"},
		{"msgstr without msgid", "msgstr \"b\"\n", "msgstr without a msgid"},
		{"two msgstrs", "msgid \"a\"\nmsgstr \"b\"\nmsgstr \"c\"\n", "msgstr without a msgid"},
		{"unknown escape", "msgid \"a\"\nmsgstr \"\\q\"\n", "unsupported escape"},
		{"unquoted value", "msgid a\nmsgstr \"b\"\n", "malformed string"},
		{"duplicate msgid", "msgid \"a\"\nmsgstr \"b\"\n\nmsgid \"a\"\nmsgstr \"c\"\n", "duplicate msgid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePO([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParsePO() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestUnquotePO(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: `""`, want: ""},
		{in: `"plain"`, want: "plain"},
		{in: `"\n\t\r\a\b\f\v\\\""`, want: "\n\t\r\a\b\f\v\\\""},
		{in: `"no end`, wantErr: true},
		{in: `"`, wantErr: true},
		{in: `"a"b"`, wantErr: true},
		{in: `"trailing\"`, wantErr: true},
		{in: `"\x41"`, wantErr: true},
	}
	for _, tt := range tests {
		got, err := unquotePO(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("unquotePO(%s) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("unquotePO(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStripLocale(t *testing.T) {
	for in, want := range map[string]string{
		"de_DE.UTF-8@euro": "de_DE",
		"sr@latin":         "sr",
		"pt_BR.utf8":       "pt_BR",
		"C.UTF-8":          "C",
		"":                 "",
	} {
		if got := stripLocale(in); got != want {
			t.Errorf("stripLocale(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCandidates(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"no locale is English", map[string]string{}, nil},
		{"C ignores LANGUAGE", map[string]string{"LANG": "C", "LANGUAGE": "de"}, nil},
		{"C.UTF-8 ignores LANGUAGE", map[string]string{"LANG": "C.UTF-8", "LANGUAGE": "de"}, nil},
		{"POSIX ignores LANGUAGE", map[string]string{"LC_ALL": "POSIX", "LANGUAGE": "de"}, nil},
		{"LANG with codeset", map[string]string{"LANG": "de_DE.UTF-8"}, []string{"de_DE", "de"}},
		{"LC_ALL wins", map[string]string{"LC_ALL": "fr_FR.UTF-8", "LC_MESSAGES": "it_IT", "LANG": "de_DE"}, []string{"fr_FR", "fr"}},
		{"LC_MESSAGES before LANG", map[string]string{"LC_MESSAGES": "it_IT", "LANG": "de_DE"}, []string{"it_IT", "it"}},
		{"modifier stripped", map[string]string{"LANG": "sr_RS.UTF-8@latin"}, []string{"sr_RS", "sr"}},
		{"region file name", map[string]string{"LANG": "pt_BR.UTF-8"}, []string{"pt_BR", "pt"}},
		{"LANGUAGE first", map[string]string{"LANG": "de_DE.UTF-8", "LANGUAGE": "fr:en_GB::"}, []string{"fr", "en_GB", "en", "de_DE", "de"}},
		{"zh_CN maps to Hans", map[string]string{"LANG": "zh_CN.UTF-8"}, []string{"zh_Hans", "zh_CN", "zh"}},
		{"zh_SG maps to Hans", map[string]string{"LANG": "zh_SG"}, []string{"zh_Hans", "zh_SG", "zh"}},
		{"zh_TW maps to Hant", map[string]string{"LANG": "zh_TW.UTF-8"}, []string{"zh_Hant", "zh_TW", "zh"}},
		{"zh_HK maps to Hant", map[string]string{"LANG": "zh_HK"}, []string{"zh_Hant", "zh_HK", "zh"}},
		{"zh_MO maps to Hant", map[string]string{"LANG": "zh_MO"}, []string{"zh_Hant", "zh_MO", "zh"}},
		{"duplicates removed", map[string]string{"LANG": "de_DE", "LANGUAGE": "de_DE:de"}, []string{"de_DE", "de"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Candidates(envOf(tt.env)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Candidates() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// catalogs returns a catalog filesystem with one "<name>.po" file per entry
// of entries, each mapping a msgid to its translation. The catalogs are
// written for these tests; ChairLift ships none (ADR-0018).
func catalogs(entries map[string]map[string]string) fstest.MapFS {
	files := fstest.MapFS{}
	for name, translations := range entries {
		var po strings.Builder
		po.WriteString("msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n")
		for msgid, msgstr := range translations {
			fmt.Fprintf(&po, "\nmsgid %s\nmsgstr %s\n", strconv.Quote(msgid), strconv.Quote(msgstr))
		}
		files[name+".po"] = &fstest.MapFile{Data: []byte(po.String())}
	}
	return files
}

func TestTranslatorResolvesString(t *testing.T) {
	files := catalogs(map[string]map[string]string{
		"de": {MsgNext: "Weiter", MsgHello: "Hallo {}!"},
	})
	de := NewTranslator(files, envOf(map[string]string{"LANG": "de_DE.UTF-8"}))
	if got := de.T(MsgNext); got != "Weiter" {
		t.Fatalf("de T(%q) = %q, want %q", MsgNext, got, "Weiter")
	}
	if got := de.Languages(); !reflect.DeepEqual(got, []string{"de"}) {
		t.Fatalf("de Languages() = %v, want [de]", got)
	}
	// A string the catalog does not translate shows the English msgid.
	if got := de.T(MsgOfflineDesc); got != MsgOfflineDesc {
		t.Fatalf("de T(untranslated msgid) = %q, want the msgid", got)
	}
	// One placeholder string works in one language.
	if got := Format(de.T(MsgHello), "Ada Lovelace"); got != "Hallo Ada Lovelace!" {
		t.Fatalf("de Format(T(%q)) = %q, want %q", MsgHello, got, "Hallo Ada Lovelace!")
	}
}

func TestTranslatorFallsBackPerMessage(t *testing.T) {
	// es has fewer translations than de: a string es lacks comes from de,
	// the next LANGUAGE entry, as gettext does; a string neither has stays
	// English.
	files := catalogs(map[string]map[string]string{
		"es": {MsgNext: "Siguiente"},
		"de": {MsgNext: "Weiter", MsgBack: "Zurück"},
	})
	tr := NewTranslator(files, envOf(map[string]string{"LANG": "de_DE.UTF-8", "LANGUAGE": "xx:es"}))
	if got := tr.Languages(); !reflect.DeepEqual(got, []string{"es", "de"}) {
		t.Fatalf("Languages() = %v, want [es de] (xx has no file)", got)
	}
	for msgid, want := range map[string]string{MsgNext: "Siguiente", MsgBack: "Zurück", MsgStart: MsgStart} {
		if got := tr.T(msgid); got != want {
			t.Errorf("T(%q) = %q, want %q", msgid, got, want)
		}
	}
}

func TestTranslatorSkipsCatalogThatDoesNotParse(t *testing.T) {
	files := catalogs(map[string]map[string]string{"fr": {MsgNext: "Suivant"}})
	files["de.po"] = &fstest.MapFile{Data: []byte("msgid \"Next\"\n")}
	tr := NewTranslator(files, envOf(map[string]string{"LANG": "de_DE.UTF-8", "LANGUAGE": "de:fr"}))
	if got := tr.Languages(); !reflect.DeepEqual(got, []string{"fr"}) {
		t.Fatalf("Languages() = %v, want [fr] (de.po does not parse)", got)
	}
	if got := tr.T(MsgNext); got != "Suivant" {
		t.Errorf("T(%q) = %q, want %q", MsgNext, got, "Suivant")
	}
}

func TestTranslatorEnglish(t *testing.T) {
	files := catalogs(map[string]map[string]string{"de": {MsgNext: "Weiter"}})
	tr := NewTranslator(files, envOf(map[string]string{"LANG": "C.UTF-8", "LANGUAGE": "de"}))
	if len(tr.Languages()) != 0 {
		t.Fatalf("Languages() = %v, want none", tr.Languages())
	}
	for _, msgid := range Messages {
		if got := tr.T(msgid); got != msgid {
			t.Errorf("T(%q) = %q, want the msgid", msgid, got)
		}
	}
	if got := T("not a first-run msgid"); got != "not a first-run msgid" {
		t.Errorf("T(unknown) = %q, want the msgid", got)
	}
}

func TestChineseScriptFiles(t *testing.T) {
	files := catalogs(map[string]map[string]string{
		"zh_Hans": {MsgNext: "zh_Hans Next"},
		"zh_Hant": {MsgNext: "zh_Hant Next"},
	})
	hans := NewTranslator(files, envOf(map[string]string{"LANG": "zh_CN.UTF-8"}))
	hant := NewTranslator(files, envOf(map[string]string{"LANG": "zh_TW.UTF-8"}))
	if got := hans.Languages(); !reflect.DeepEqual(got, []string{"zh_Hans"}) {
		t.Errorf("zh_CN Languages() = %v, want [zh_Hans]", got)
	}
	if got := hant.Languages(); !reflect.DeepEqual(got, []string{"zh_Hant"}) {
		t.Errorf("zh_TW Languages() = %v, want [zh_Hant]", got)
	}
	if got := hans.T(MsgNext); got != "zh_Hans Next" {
		t.Errorf("zh_CN T(%q) = %q, want the zh_Hans text", MsgNext, got)
	}
	if got := hant.T(MsgNext); got != "zh_Hant Next" {
		t.Errorf("zh_TW T(%q) = %q, want the zh_Hant text", MsgNext, got)
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		in   string
		args []string
		want string
	}{
		{"Hello {}!", []string{"Ada"}, "Hello Ada!"},
		{"{} and {}", []string{"a", "b"}, "a and b"},
		{"{} and {}", []string{"a"}, "a and {}"},
		{"no placeholder", []string{"a"}, "no placeholder"},
		{"Hello {}!", nil, "Hello {}!"},
	}
	for _, tt := range tests {
		if got := Format(tt.in, tt.args...); got != tt.want {
			t.Errorf("Format(%q, %v) = %q, want %q", tt.in, tt.args, got, tt.want)
		}
	}
}

// embeddedNames returns the names of the embedded catalogs. It may be empty:
// ChairLift ships no translation at the moment (ADR-0018).
func embeddedNames(t *testing.T) []string {
	t.Helper()
	matches, err := fs.Glob(embeddedCatalogs(), "*.po")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, strings.TrimSuffix(match, ".po"))
	}
	return names
}

// TestEveryEmbeddedMsgidIsAFirstRunMsgid parses every embedded catalog and
// fails when one does not parse, holds no translation, or holds a msgid the
// first run does not use. It also passes when no catalog is embedded.
func TestEveryEmbeddedMsgidIsAFirstRunMsgid(t *testing.T) {
	known := make(map[string]bool, len(Messages))
	for _, msgid := range Messages {
		if known[msgid] {
			t.Errorf("Messages lists %q twice", msgid)
		}
		known[msgid] = true
	}
	for _, name := range embeddedNames(t) {
		t.Run(name, func(t *testing.T) {
			data, err := fs.ReadFile(embeddedCatalogs(), name+".po")
			if err != nil {
				t.Fatalf("read embedded po/%s.po: %v", name, err)
			}
			catalog, err := ParsePO(data)
			if err != nil {
				t.Fatalf("po/%s.po does not parse: %v", name, err)
			}
			if len(catalog) == 0 {
				t.Fatalf("po/%s.po holds no translation; do not ship an empty catalog", name)
			}
			for msgid := range catalog {
				if !known[msgid] {
					t.Errorf("po/%s.po holds msgid %q, which is not in Messages", name, msgid)
				}
			}
		})
	}
}
