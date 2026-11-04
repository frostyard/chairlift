// Package i18n translates ChairLift's first-run strings. It reads the
// gettext .po files embedded from internal/i18n/po with a small pure-Go
// parser, so the build needs no msgfmt step, no generated files and no CGO.
//
// The package is puregotk-free (ADR-0007): the first-run window in
// internal/window calls T, and every decision here is unit-tested headlessly.
package i18n

import (
	"bytes"
	"fmt"
	"strings"
)

// ParsePO parses the text of a gettext .po file and returns its usable
// translations, keyed by msgid. It supports the subset the ported first-setup
// files use: comment lines, "msgid" and "msgstr" keywords, multi-line strings
// and C escapes. It skips the header entry (empty msgid), entries with an
// empty msgstr and entries flagged "#, fuzzy", as gettext does. msgctxt and
// plural forms are not supported and are syntax errors, as are a string with
// no keyword, a duplicate msgid and an unknown escape.
func ParsePO(data []byte) (map[string]string, error) {
	p := poParser{translations: make(map[string]string)}
	for i, line := range bytes.Split(data, []byte("\n")) {
		if err := p.line(strings.TrimRight(string(line), "\r"), i+1); err != nil {
			return nil, err
		}
	}
	if err := p.flush(len(bytes.Split(data, []byte("\n")))); err != nil {
		return nil, err
	}
	return p.translations, nil
}

// poParser is the state of ParsePO between lines.
type poParser struct {
	translations map[string]string
	fuzzy        bool   // the current entry carries the fuzzy flag
	key          string // "", "msgid" or "msgstr": the keyword of the open string
	msgid        strings.Builder
	msgstr       strings.Builder
	hasMsgid     bool
	hasMsgstr    bool
}

// line consumes one line of a .po file.
func (p *poParser) line(line string, number int) error {
	trimmed := strings.TrimSpace(line)
	switch {
	case trimmed == "":
		return p.flush(number)
	case strings.HasPrefix(trimmed, "#"):
		if p.hasMsgstr {
			// A comment after a complete entry starts the next entry.
			if err := p.flush(number); err != nil {
				return err
			}
		}
		if strings.HasPrefix(trimmed, "#,") {
			for _, flag := range strings.Split(trimmed[2:], ",") {
				if strings.TrimSpace(flag) == "fuzzy" {
					p.fuzzy = true
				}
			}
		}
		return nil
	case strings.HasPrefix(trimmed, `"`):
		if p.key == "" {
			return fmt.Errorf("line %d: string with no msgid or msgstr keyword", number)
		}
		return p.appendString(trimmed, number)
	}

	keyword, rest, _ := strings.Cut(trimmed, " ")
	switch keyword {
	case "msgid":
		if p.hasMsgstr {
			if err := p.flush(number); err != nil {
				return err
			}
		}
		if p.hasMsgid {
			return fmt.Errorf("line %d: msgid without a msgstr for the previous msgid", number)
		}
		p.key, p.hasMsgid = "msgid", true
	case "msgstr":
		if !p.hasMsgid || p.hasMsgstr {
			return fmt.Errorf("line %d: msgstr without a msgid", number)
		}
		p.key, p.hasMsgstr = "msgstr", true
	default:
		return fmt.Errorf("line %d: unsupported keyword %q", number, keyword)
	}
	return p.appendString(strings.TrimSpace(rest), number)
}

// appendString unquotes one quoted string and appends it to the open keyword.
func (p *poParser) appendString(quoted string, number int) error {
	value, err := unquotePO(quoted)
	if err != nil {
		return fmt.Errorf("line %d: %w", number, err)
	}
	if p.key == "msgid" {
		p.msgid.WriteString(value)
	} else {
		p.msgstr.WriteString(value)
	}
	return nil
}

// flush ends the current entry and stores it when it is a usable
// translation.
func (p *poParser) flush(number int) error {
	defer p.reset()
	if !p.hasMsgid && !p.hasMsgstr {
		return nil
	}
	if !p.hasMsgstr {
		return fmt.Errorf("line %d: msgid %q has no msgstr", number, p.msgid.String())
	}
	msgid, msgstr := p.msgid.String(), p.msgstr.String()
	if _, dup := p.translations[msgid]; dup && msgid != "" {
		return fmt.Errorf("line %d: duplicate msgid %q", number, msgid)
	}
	if msgid == "" || msgstr == "" || p.fuzzy {
		return nil
	}
	p.translations[msgid] = msgstr
	return nil
}

// reset clears the entry state.
func (p *poParser) reset() {
	p.fuzzy, p.key, p.hasMsgid, p.hasMsgstr = false, "", false, false
	p.msgid.Reset()
	p.msgstr.Reset()
}

// unquotePO removes the double quotes of a .po string and decodes its C
// escapes.
func unquotePO(quoted string) (string, error) {
	if len(quoted) < 2 || quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
		return "", fmt.Errorf("malformed string %s", quoted)
	}
	body := quoted[1 : len(quoted)-1]
	var out strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '"' {
			return "", fmt.Errorf("unescaped quote in string %s", quoted)
		}
		if c != '\\' {
			out.WriteByte(c)
			continue
		}
		i++
		if i >= len(body) {
			return "", fmt.Errorf("trailing backslash in string %s", quoted)
		}
		switch body[i] {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case 'a':
			out.WriteByte('\a')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'v':
			out.WriteByte('\v')
		case '\\':
			out.WriteByte('\\')
		case '"':
			out.WriteByte('"')
		default:
			return "", fmt.Errorf("unsupported escape \\%c in string %s", body[i], quoted)
		}
	}
	return out.String(), nil
}
