package problems

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"time"
)

// Fixed paths of snosi-health's files (snosi ADR-0029). They are not
// configurable: a user-writable setting must not choose the file whose
// values ChairLift may later report.
const (
	ProblemsPath = "/var/lib/snosi-health/problems.json"
	DetailsPath  = "/var/lib/snosi-health/details.json"
)

// Read limits. A larger file is malformed.
const (
	MaxProblemsBytes = 1 << 20 // 1 MiB
	MaxDetailsBytes  = 4 << 20 // 4 MiB
	// MaxItems bounds each list of items: system items, user crash groups
	// and app record groups.
	MaxItems = 200
)

// SourceOutcome is the outcome of reading problems.json.
type SourceOutcome int

const (
	// SourceMissing: the file does not exist. The host has no snosi-health;
	// the page says "System checks are not available on this system".
	SourceMissing SourceOutcome = iota
	// SourceUnreadable: the file exists but opening or reading it failed
	// (a permission or I/O error).
	SourceUnreadable
	// SourceMalformed: the file is not valid JSON, has a format other than
	// 1 or a schema other than frostyard.problem/1, has no problems list,
	// or is larger than MaxProblemsBytes.
	SourceMalformed
	// SourceOK: the file was read; its items are in SystemFile.Items.
	SourceOK
)

// String returns the outcome's name, as the "source=" value of the page's
// log marker.
func (o SourceOutcome) String() string {
	switch o {
	case SourceMissing:
		return "missing"
	case SourceUnreadable:
		return "unreadable"
	case SourceMalformed:
		return "malformed"
	case SourceOK:
		return "ok"
	default:
		return "unknown"
	}
}

// SystemFile is the parsed problems.json.
type SystemFile struct {
	Outcome SourceOutcome
	// Err says why the outcome is SourceUnreadable or SourceMalformed.
	Err error
	// Generated is the file's "generated" time; zero when it is absent or
	// not RFC 3339.
	Generated time.Time
	Envelope  Envelope
	// Client is the file's client string when it matches the v1 client
	// pattern, else "".
	Client string
	// Items are the system items, newest last_seen first, at most
	// MaxItems.
	Items []Item
	// Dropped counts problems that were dropped: not an object, a bad id, a
	// bad kind name, or a first_seen or last_seen that is not RFC 3339.
	Dropped int
	// Truncated counts valid problems past MaxItems.
	Truncated int
}

// ReadSystem reads and parses the problems.json file at path.
func ReadSystem(path string) SystemFile {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return SystemFile{Outcome: SourceMissing}
	}
	if err != nil {
		return SystemFile{Outcome: SourceUnreadable, Err: err}
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxProblemsBytes+1))
	if err != nil {
		return SystemFile{Outcome: SourceUnreadable, Err: err}
	}
	return ParseSystem(data)
}

// ParseSystem parses the content of a problems.json file (format 1).
// Unknown top-level keys are ignored.
func ParseSystem(data []byte) SystemFile {
	malformed := func(format string, args ...any) SystemFile {
		return SystemFile{Outcome: SourceMalformed, Err: fmt.Errorf("problems.json: "+format, args...)}
	}
	if len(data) > MaxProblemsBytes {
		return malformed("larger than %d bytes", MaxProblemsBytes)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top == nil {
		return malformed("not a JSON object")
	}
	var format int
	if !decodeStrict(top["format"], &format) || format != 1 {
		return malformed("format is not 1")
	}
	var schema string
	if !decodeStrict(top["schema"], &schema) || schema != SchemaID {
		return malformed("schema is not %s", SchemaID)
	}
	var list []json.RawMessage
	if !decodeStrict(top["problems"], &list) {
		return malformed("problems is not a list")
	}

	file := SystemFile{Outcome: SourceOK, Envelope: parseEnvelope(top)}
	var generated string
	if decodeStrict(top["generated"], &generated) {
		if t, err := time.Parse(time.RFC3339, generated); err == nil {
			file.Generated = t
		}
	}
	var client string
	if decodeStrict(top["client"], &client) && clientRe.MatchString(client) {
		file.Client = client
	}

	for _, raw := range list {
		item, ok := parseSystemItem(raw, file.Envelope)
		if !ok {
			file.Dropped++
			continue
		}
		file.Items = append(file.Items, item)
	}
	sortItems(file.Items)
	if len(file.Items) > MaxItems {
		file.Truncated = len(file.Items) - MaxItems
		file.Items = file.Items[:MaxItems]
	}
	return file
}

// parseEnvelope reads image.id, image.version and secure. Each may be null
// or wrong; the envelope is then not valid, and no item is reportable.
func parseEnvelope(top map[string]json.RawMessage) Envelope {
	var env Envelope
	var image map[string]json.RawMessage
	if decodeStrict(top["image"], &image) {
		_ = decodeStrict(image["id"], &env.ImageID)
		_ = decodeStrict(image["version"], &env.ImageVersion)
	}
	secureOK := decodeStrict(top["secure"], &env.Secure)
	env.Valid = secureOK && contains(imageIDs, env.ImageID) && imageVersionRe.MatchString(env.ImageVersion)
	env.ImageID = cutDisplay(env.ImageID)
	env.ImageVersion = cutDisplay(env.ImageVersion)
	return env
}

// parseSystemItem parses 1 entry of problems[]. It returns false for an
// entry that must be dropped.
func parseSystemItem(raw json.RawMessage, env Envelope) (Item, bool) {
	var entry map[string]json.RawMessage
	if !decodeStrict(raw, &entry) {
		return Item{}, false
	}
	var id, kind, firstSeen, lastSeen string
	if !decodeStrict(entry["id"], &id) || !idRe.MatchString(id) {
		return Item{}, false
	}
	if !decodeStrict(entry["kind"], &kind) || !kindNameRe.MatchString(kind) {
		return Item{}, false
	}
	if !decodeStrict(entry["first_seen"], &firstSeen) || !decodeStrict(entry["last_seen"], &lastSeen) {
		return Item{}, false
	}
	first, err1 := time.Parse(time.RFC3339, firstSeen)
	last, err2 := time.Parse(time.RFC3339, lastSeen)
	if err1 != nil || err2 != nil {
		return Item{}, false
	}

	item := Item{
		ID:        id,
		Origin:    OriginSystem,
		Kind:      kind,
		KnownKind: contains(Kinds, kind),
		FirstSeen: first.UTC(),
		LastSeen:  last.UTC(),
		Count:     1,
		Envelope:  env,
	}
	var occurrences int
	if decodeStrict(entry["occurrences"], &occurrences) && occurrences > 0 {
		item.Count = occurrences
	}
	var week string
	if decodeStrict(entry["week"], &week) {
		item.WeekValid = weekRe.MatchString(week)
		item.Week = cutDisplay(week)
	}
	var reportable bool
	if decodeStrict(entry["reportable"], &reportable) {
		item.FileReportable = reportable
	}

	detailOK := false
	if item.KnownKind {
		var detail map[string]json.RawMessage
		if decodeStrict(entry["detail"], &detail) {
			item.Fields, item.Missing, detailOK = validateDetail(kind, detail)
		} else {
			item.Missing = requiredFields(kind)
		}
	}
	item.Reportable = item.FileReportable && detailOK && item.WeekValid && env.Valid
	return item, true
}

// requiredFields returns the required field names of kind.
func requiredFields(kind string) []string {
	var names []string
	for _, rule := range detailRules[kind] {
		if rule.required {
			names = append(names, rule.name)
		}
	}
	return names
}

// sortItems orders items newest last_seen first, then by ID.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].LastSeen.Equal(items[j].LastSeen) {
			return items[i].LastSeen.After(items[j].LastSeen)
		}
		return items[i].ID < items[j].ID
	})
}
