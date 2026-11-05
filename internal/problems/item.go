// Package problems loads the local problem records that ChairLift's
// Problems page shows (ADR-0015, docs/specs/problems-page.md): the system
// problems that snosi-health writes to /var/lib/snosi-health, the user's
// own crashes from coredumpctl, and the frostyard app problem records of the
// user journal (read through github.com/frostyard/std/problems).
//
// The package imports no GTK binding and no network package: phase 1 of the
// Problems page only shows records on this computer. Every value that can
// reach a report (BuildPayload) is validated here against schema
// frostyard.problem/1, whatever the file or record says about itself.
package problems

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Origin says where an item comes from.
type Origin int

const (
	// OriginSystem is an item of /var/lib/snosi-health/problems.json.
	OriginSystem Origin = iota
	// OriginUserCrash is a group of the user's own coredumpctl entries.
	OriginUserCrash
	// OriginAppRecord is a group of app problem records of the user journal.
	OriginAppRecord
)

// String returns the origin's name for logs.
func (o Origin) String() string {
	switch o {
	case OriginSystem:
		return "system"
	case OriginUserCrash:
		return "user-crash"
	case OriginAppRecord:
		return "app-record"
	default:
		return "unknown"
	}
}

// Envelope is the image identity of the host, from problems.json. Valid is
// true only when the image ID is in the closed v1 enum, the version matches
// the v1 image version pattern, and secure is a boolean.
type Envelope struct {
	ImageID      string
	ImageVersion string
	Secure       bool
	Valid        bool
}

// Local holds text that stays on this computer: it is shown and copied, and
// it never enters a payload.
type Local struct {
	// Values are the "local" fields of details.json (the detail before
	// snosi-health sanitized it), sorted by name.
	Values []Field
	// Units are the full unit names of details.json's extra.units.
	Units []string
	// ExePath is the executable path: details.json's extra.exe_path, or the
	// path that coredumpctl reported.
	ExePath string
	// Messages are up to 5 journal texts of details.json.
	Messages []string
	// Message is the MESSAGE of the newest app problem record.
	Message string
	// Comm and Unit are the trusted journald fields of the newest app
	// problem record (_COMM, and _SYSTEMD_USER_UNIT or _SYSTEMD_UNIT).
	Comm string
	Unit string
}

// Item is 1 problem as the Problems page shows it.
type Item struct {
	// ID is the snosi-health id (16 hex digits) of a system item, or
	// "crash-" or "app-" and 16 hex digits for a user item. Dismiss keys on
	// it.
	ID     string
	Origin Origin
	// Kind is the problem kind. KnownKind is false for a kind that is not
	// one of the 9 kinds of schema v1; such an item is never reportable.
	Kind      string
	KnownKind bool
	// Fields are the validated detail fields; Missing names the required
	// fields that are absent.
	Fields  []Field
	Missing []string
	// Week is the ISO week of the record; WeekValid says it matches the v1
	// week pattern.
	Week      string
	WeekValid bool
	FirstSeen time.Time
	LastSeen  time.Time
	Count     int
	// FileReportable is the "reportable" flag of problems.json. ChairLift
	// does not trust it alone: Reportable is true only when the flag, the
	// own validation of every field, and the envelope all pass.
	FileReportable bool
	Reportable     bool
	Envelope       Envelope
	Local          Local

	// times holds the entry times of a user crash group, for the merge rule.
	times []time.Time
}

// Field returns the field name, or false when the item has no such field.
func (it Item) Field(name string) (Field, bool) {
	for _, f := range it.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// ValidString returns the value of a valid string field, or "".
func (it Item) ValidString(name string) string {
	f, ok := it.Field(name)
	if !ok || !f.Valid {
		return ""
	}
	s, _ := f.Value.(string)
	return s
}

// DetailValid reports whether every detail field passed its rule and no
// required field is missing.
func (it Item) DetailValid() bool {
	if !it.KnownKind || len(it.Missing) > 0 {
		return false
	}
	for _, f := range it.Fields {
		if !f.Valid {
			return false
		}
	}
	return true
}

// isoWeek returns the ISO 8601 week of t in UTC, such as "2026-W45".
func isoWeek(t time.Time) string {
	year, week := t.UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

// hashID returns prefix and the first 16 hex digits of the sha256 of parts.
func hashID(prefix string, parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(h.Sum(nil))[:16]
}
