package problems

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SchemaID is the schema version of every problem record and report that
// this package reads or builds (frostyard/core docs/specs/problem-report.md).
const SchemaID = "frostyard.problem/1"

// The 9 kinds of schema v1, in schema order.
const (
	KindUnitFailed         = "unit_failed"
	KindUpdateStageFailed  = "update_stage_failed"
	KindBootedOlderImage   = "booted_older_image"
	KindSysextUpdateFailed = "sysext_update_failed"
	KindTPMUnlock          = "tpm_unlock"
	KindIntegrityError     = "integrity_error"
	KindCrash              = "crash"
	KindAppProblem         = "app_problem"
	KindCount              = "count"
)

// Kinds lists the kinds of schema v1 in schema order. The list is closed: a
// kind that is not in it is an unknown kind and is never reportable.
var Kinds = []string{
	KindUnitFailed,
	KindUpdateStageFailed,
	KindBootedOlderImage,
	KindSysextUpdateFailed,
	KindTPMUnlock,
	KindIntegrityError,
	KindCrash,
	KindAppProblem,
	KindCount,
}

// Patterns of schema v1. Each source string is the exact "pattern" of
// core's v1.json; TestPatternsMatchSchema compares them byte for byte.
var (
	imageVersionRe   = regexp.MustCompile(`^[0-9]{14}$`)
	semverRe         = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})(-(alpha|beta|rc)\.(0|[1-9][0-9]{0,3}))?$`)
	reportIDRe       = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	weekRe           = regexp.MustCompile(`^20[2-9][0-9]-W(0[1-9]|[1-4][0-9]|5[0-3])$`)
	clientRe         = regexp.MustCompile(`^(chairlift/(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})(-(alpha|beta|rc)\.(0|[1-9][0-9]{0,3}))?|snosi-health/[0-9]{14})$`)
	unitRe           = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_.-]{0,95}@?\.(service|socket|timer|mount|automount|swap|path|target|slice|scope)$`)
	featureRe        = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	sysextVersionRe  = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+~_-]{0,63}$`)
	exeRe            = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
	packageRe        = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]{1,99}$`)
	packageVersionRe = regexp.MustCompile(`^([0-9]{1,4}:)?[0-9][A-Za-z0-9.+~-]{0,63}$`)
	codeRe           = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,63}$`)
	frameRe          = regexp.MustCompile(`^(other|(github\.com/frostyard(/[A-Za-z0-9_-]+){1,6}|(github\.com/emersion/go-imap/v2|archive|bufio|bytes|cmp|compress|container|context|crypto|database|debug|embed|encoding|errors|expvar|flag|fmt|go|hash|html|image|index|internal|io|iter|log|main|maps|math|mime|net|os|path|plugin|reflect|regexp|runtime|slices|sort|strconv|strings|structs|sync|syscall|testing|text|time|unicode|unique|unsafe|vendor|weak)(/[a-z0-9_]+){0,4})\.[A-Za-z0-9_.()*\[\]-]+)$`)
	locationRe       = regexp.MustCompile(`^src/([a-z0-9_]+/){0,8}[a-z0-9_]+\.rs:[1-9][0-9]{0,5}$`)

	// idRe and kindNameRe are snosi-health's problems.json rules
	// (snosi ADR-0029), not schema fields: an item whose id or kind fails
	// them is dropped.
	idRe       = regexp.MustCompile(`^[0-9a-f]{16}$`)
	kindNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

// Closed enums of the envelope.
var (
	imageIDs = []string{"snow", "snowfield", "sundog", "floe"}
	appIDs   = []string{
		"org.frostyard.ChairLift",
		"org.frostyard.nsl",
		"org.frostyard.intuneme",
		"org.frostyard.pilothouse",
		"org.frostyard.Frostmail",
		"org.frostyard.Frostnotes",
		"org.frostyard.Frostread",
	}
)

// valueType is the JSON type of a detail field.
type valueType int

const (
	typeString valueType = iota
	typeBool
	typeInt
	typeStrings
)

// fieldRule is the v1 rule of 1 detail field.
type fieldRule struct {
	name      string
	typ       valueType
	required  bool
	pattern   *regexp.Regexp
	enum      []string
	constTrue bool
	min, max  int // typeInt
	maxLen    int // typeString and the items of typeStrings, in characters
	minItems  int // typeStrings
	maxItems  int // typeStrings
}

// detailRules holds the detail fields of each kind, in schema order.
var detailRules = map[string][]fieldRule{
	KindUnitFailed: {
		{name: "unit", typ: typeString, required: true, pattern: unitRe},
		{name: "result", typ: typeString, required: true, enum: []string{"exit-code", "signal", "timeout", "core-dump", "start-limit-hit", "other"}},
		{name: "shipped", typ: typeBool, required: true, constTrue: true},
	},
	KindUpdateStageFailed: {
		{name: "target_version", typ: typeString, required: true, pattern: imageVersionRe},
		{name: "error_class", typ: typeString, required: true, enum: []string{"network", "registry", "signature", "disk_space", "lock", "other"}},
		{name: "bootc_version", typ: typeString, required: true, pattern: semverRe},
	},
	KindBootedOlderImage: {
		{name: "newer_version", typ: typeString, required: true, pattern: imageVersionRe},
		{name: "how", typ: typeString, required: true, enum: []string{"manual_rollback", "boot_menu", "unknown", "other"}},
	},
	KindSysextUpdateFailed: {
		{name: "feature", typ: typeString, required: true, pattern: featureRe},
		{name: "version", typ: typeString, required: true, pattern: sysextVersionRe},
		{name: "outcome", typ: typeString, required: true, enum: []string{"failed", "held_by_rollback", "needs_image", "skipped_unreachable", "skipped_busy", "other"}},
		{name: "error_class", typ: typeString, enum: []string{"network", "http_status", "signature", "checksum", "disk_space", "link", "config", "other"}},
	},
	KindTPMUnlock: {
		{name: "outcome", typ: typeString, required: true, enum: []string{"ok_after_retry", "recovery_key", "passphrase", "other"}},
		{name: "tpm_rc", typ: typeString, required: true, enum: []string{"PCR_CHANGED", "POLICY_FAIL", "LOCKOUT", "other"}},
		{name: "retries", typ: typeInt, required: true, min: 0, max: 10},
	},
	KindIntegrityError: {
		{name: "subsystem", typ: typeString, required: true, enum: []string{"composefs", "fs-verity", "dm-verity", "other"}},
		{name: "count", typ: typeString, required: true, enum: []string{"1", "2-10", ">10"}},
	},
	KindCrash: {
		{name: "exe", typ: typeString, required: true, pattern: exeRe},
		{name: "package", typ: typeString, pattern: packageRe},
		{name: "package_version", typ: typeString, pattern: packageVersionRe},
		{name: "signal", typ: typeString, required: true, enum: []string{"SIGSEGV", "SIGABRT", "SIGBUS", "SIGILL", "SIGFPE", "SIGTRAP", "SIGSYS", "other"}},
	},
	KindAppProblem: {
		{name: "app_id", typ: typeString, required: true, enum: appIDs},
		{name: "app_version", typ: typeString, required: true, pattern: semverRe},
		{name: "runtime", typ: typeString, required: true, enum: []string{"go", "rust"}},
		{name: "component", typ: typeString, required: true, enum: []string{"daemon", "cli", "gui", "shell", "other"}},
		{name: "class", typ: typeString, required: true, enum: []string{"panic", "error"}},
		{name: "code", typ: typeString, pattern: codeRe},
		{name: "frames", typ: typeStrings, pattern: frameRe, maxLen: 160, minItems: 1, maxItems: 32},
		{name: "location", typ: typeString, pattern: locationRe, maxLen: 128},
	},
	KindCount: {
		{name: "install_age", typ: typeString, required: true, enum: []string{"<1w", "<1m", "<6m", ">=6m"}},
		{name: "channel", typ: typeString, required: true, enum: []string{"latest", "other"}},
	},
}

// Field is 1 detail field of an item after validation. Value holds the
// typed value (string, bool, int or []string) only when Valid is true; a
// field that fails its rule keeps only its Display text, for the local view.
type Field struct {
	Name    string
	Display string
	Value   any
	Valid   bool
	// Reason says why the field is not valid ("not an allowed value", "does
	// not match its pattern", ...); it is empty for a valid field.
	Reason string
}

// maxDisplay bounds the display text of a field that failed its rule.
const maxDisplay = 160

// validateDetail checks raw (the detail object of 1 item) against the v1
// rules of kind. It returns the fields in schema order (unknown fields
// last, sorted), the names of required fields that are absent, and ok:
// true only when every field passed and none is missing. A kind with no
// rules gives no fields and ok false.
func validateDetail(kind string, raw map[string]json.RawMessage) (fields []Field, missing []string, ok bool) {
	rules, known := detailRules[kind]
	if !known {
		return nil, nil, false
	}
	ok = true
	named := make(map[string]bool, len(rules))
	for _, rule := range rules {
		named[rule.name] = true
		value, present := raw[rule.name]
		if !present {
			if rule.required {
				missing = append(missing, rule.name)
				ok = false
			}
			continue
		}
		field := checkField(rule, value)
		if !field.Valid {
			ok = false
		}
		fields = append(fields, field)
	}
	var extra []string
	for name := range raw {
		if !named[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		ok = false
		fields = append(fields, Field{
			Name:    cutDisplay(name),
			Display: rawDisplay(raw[name]),
			Reason:  "not a field of this kind",
		})
	}
	if kind == KindAppProblem {
		if !checkAppProblem(fields, &missing) {
			ok = false
		}
	}
	return fields, missing, ok
}

// checkField checks 1 present value against its rule.
func checkField(rule fieldRule, value json.RawMessage) Field {
	field := Field{Name: rule.name, Display: rawDisplay(value)}
	switch rule.typ {
	case typeBool:
		var b bool
		if !decodeStrict(value, &b) {
			field.Reason = "not true or false"
			return field
		}
		field.Display = strconv.FormatBool(b)
		if rule.constTrue && !b {
			field.Reason = "must be true"
			return field
		}
		field.Value = b
	case typeInt:
		n, ok := decodeNumber(value)
		if !ok {
			field.Reason = "not a whole number"
			return field
		}
		i, err := strconv.Atoi(n.String())
		if err != nil {
			field.Reason = "not a whole number"
			return field
		}
		field.Display = strconv.Itoa(i)
		if i < rule.min || i > rule.max {
			field.Reason = fmt.Sprintf("not between %d and %d", rule.min, rule.max)
			return field
		}
		field.Value = i
	case typeString:
		var s string
		if !decodeStrict(value, &s) {
			field.Reason = "not text"
			return field
		}
		field.Display = cutDisplay(s)
		if reason := checkString(rule, s); reason != "" {
			field.Reason = reason
			return field
		}
		field.Value = s
	case typeStrings:
		var list []string
		if !decodeStrict(value, &list) {
			field.Reason = "not a list of text"
			return field
		}
		field.Display = cutDisplay(strings.Join(list, ", "))
		if len(list) < rule.minItems || len(list) > rule.maxItems {
			field.Reason = fmt.Sprintf("must hold %d to %d entries", rule.minItems, rule.maxItems)
			return field
		}
		for _, s := range list {
			if reason := checkString(rule, s); reason != "" {
				field.Reason = reason
				return field
			}
		}
		field.Value = append([]string(nil), list...)
	}
	field.Valid = true
	return field
}

// checkString checks 1 string against a rule's length, enum and pattern.
func checkString(rule fieldRule, s string) string {
	if rule.maxLen > 0 && utf8.RuneCountInString(s) > rule.maxLen {
		return fmt.Sprintf("longer than %d characters", rule.maxLen)
	}
	if rule.enum != nil && !contains(rule.enum, s) {
		return "not an allowed value"
	}
	if rule.pattern != nil && !rule.pattern.MatchString(s) {
		return "does not match its pattern"
	}
	return ""
}

// checkAppProblem applies the S8 rules that join fields: class error needs
// a code and has no frames or location; class panic has no code; runtime go
// has no location; runtime rust has no frames. A field that breaks a rule is
// marked not valid.
func checkAppProblem(fields []Field, missing *[]string) bool {
	value := func(name string) string {
		for _, f := range fields {
			if f.Name == name && f.Valid {
				s, _ := f.Value.(string)
				return s
			}
		}
		return ""
	}
	present := func(name string) bool {
		for _, f := range fields {
			if f.Name == name {
				return true
			}
		}
		return false
	}
	class, runtime := value("class"), value("runtime")
	forbidden := map[string]string{}
	if class == "error" {
		forbidden["frames"] = "not allowed for class error"
		forbidden["location"] = "not allowed for class error"
	}
	if class == "panic" {
		forbidden["code"] = "not allowed for class panic"
	}
	if runtime == "go" {
		forbidden["location"] = "not allowed for runtime go"
	}
	if runtime == "rust" {
		forbidden["frames"] = "not allowed for runtime rust"
	}
	ok := true
	for i := range fields {
		if reason, bad := forbidden[fields[i].Name]; bad {
			fields[i].Valid = false
			fields[i].Value = nil
			fields[i].Reason = reason
			ok = false
		}
	}
	if class == "error" && !present("code") {
		*missing = append(*missing, "code")
		ok = false
	}
	return ok
}

// decodeStrict decodes value into v and rejects a JSON null.
func decodeStrict(value json.RawMessage, v any) bool {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	return dec.Decode(v) == nil
}

// decodeNumber returns value when it is a JSON number token. Decoding into
// json.Number alone also accepts a JSON string of digits ("1"), which the
// v1 integer type does not.
func decodeNumber(value json.RawMessage) (json.Number, bool) {
	var v any
	if !decodeStrict(value, &v) {
		return "", false
	}
	n, ok := v.(json.Number)
	return n, ok
}

// rawDisplay gives compact display text for a raw JSON value: the string
// itself for a JSON string, else the compact JSON, cut to maxDisplay.
func rawDisplay(value json.RawMessage) string {
	var s string
	if json.Unmarshal(value, &s) == nil {
		return cutDisplay(s)
	}
	var buf bytes.Buffer
	if json.Compact(&buf, value) == nil {
		return cutDisplay(buf.String())
	}
	return cutDisplay(string(value))
}

// cutDisplay cuts s to maxDisplay characters and replaces invalid UTF-8
// and control characters, so forged text cannot break the display.
func cutDisplay(s string) string {
	return cutText(s, maxDisplay)
}

// cutText cuts s to at most n characters, with invalid UTF-8 and control
// characters other than newline and tab replaced.
func cutText(s string, n int) string {
	var b strings.Builder
	count := 0
	for _, r := range strings.ToValidUTF8(s, "\uFFFD") {
		if count == n {
			b.WriteString("…")
			break
		}
		if (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f {
			r = '\uFFFD'
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ValidCode reports whether code matches the v1 code pattern.
func ValidCode(code string) bool {
	return codeRe.MatchString(code)
}

// ValidSemver reports whether version is a release version of the spec
// (<semver>: no "v" prefix, no build metadata, only alpha, beta or rc
// prereleases).
func ValidSemver(version string) bool {
	return semverRe.MatchString(version)
}
