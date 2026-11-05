package reporting

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

// The sent log's limits and caps (docs/specs/problem-reporting.md).
const (
	// stateFormat is the only sent log format this package writes.
	stateFormat = 1
	// MaxPerDay bounds the reports that 1 machine sends in 24 hours.
	MaxPerDay = 20
	// MaxEntries bounds the kept entries of the sent log; the oldest go
	// first. It also bounds the "Sent reports" list the page shows.
	MaxEntries = 500
	// RetainFor is how long an entry stays in the sent log.
	RetainFor = 90 * 24 * time.Hour
	// RetryAfter is how long a failed send pauses every automatic send.
	RetryAfter = 24 * time.Hour
	// maxStateBytes bounds the sent log that LoadState reads.
	maxStateBytes = 1 << 20
)

// ErrStateUnreadable is returned by LoadState for a file that cannot be
// read or parsed. Callers then send nothing automatically: they cannot
// enforce the daily limit or record the send.
var ErrStateUnreadable = errors.New("reporting: the sent log is unreadable")

// Entry is 1 sent report.
type Entry struct {
	ReportID string `json:"report_id"`
	Kind     string `json:"kind"`
	// ProblemID is the item ID for a problem report, and empty for a count.
	ProblemID string    `json:"problem_id"`
	Week      string    `json:"week"`
	SentAt    time.Time `json:"sent_at"`
}

// stateFile is the JSON form of the sent log.
type stateFile struct {
	Format     int       `json:"format"`
	Sent       []Entry   `json:"sent"`
	CountWeek  string    `json:"count_week"`
	RetryAfter time.Time `json:"retry_after,omitzero"`
}

// State is the sent log: what this user sent, the week of the last weekly
// count, and the end of a pause after a failed send.
type State struct {
	// Sent holds the entries, oldest first.
	Sent []Entry
	// CountWeek is the ISO week of the last count; empty means none.
	CountWeek string
	// RetryAfter is the end of the pause after a failed send.
	RetryAfter time.Time
}

// StatePath returns the sent log of this user:
// $XDG_STATE_HOME/chairlift/reports.json, else
// $HOME/.local/state/chairlift/reports.json. Only absolute values count.
func StatePath(getenv func(string) string) (string, error) {
	if dir := getenv("XDG_STATE_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "chairlift", "reports.json"), nil
	}
	if home := getenv("HOME"); filepath.IsAbs(home) {
		return filepath.Join(home, ".local", "state", "chairlift", "reports.json"), nil
	}
	return "", errors.New("reporting: neither XDG_STATE_HOME nor HOME is an absolute path")
}

// LoadState reads the sent log at path. A missing file is an empty state
// with no error. A file that cannot be read, is larger than 1 MiB, or is
// not format 1 gives an empty state and ErrStateUnreadable.
func LoadState(path string) (*State, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &State{}, nil
	}
	if err != nil {
		return &State{}, fmt.Errorf("%w: %v", ErrStateUnreadable, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxStateBytes+1))
	if err != nil {
		return &State{}, fmt.Errorf("%w: %v", ErrStateUnreadable, err)
	}
	if len(data) > maxStateBytes {
		return &State{}, fmt.Errorf("%w: %s is larger than %d bytes", ErrStateUnreadable, path, maxStateBytes)
	}
	var file stateFile
	if err := json.Unmarshal(data, &file); err != nil || file.Format != stateFormat {
		return &State{}, fmt.Errorf("%w: %s is not a format %d sent log", ErrStateUnreadable, path, stateFormat)
	}
	st := &State{Sent: file.Sent, CountWeek: file.CountWeek, RetryAfter: file.RetryAfter}
	st.trim(time.Time{})
	return st, nil
}

// Save writes the sent log to path: a temporary file in the same folder
// (mode 0600), then a rename. The folder is made with mode 0700.
func (s *State) Save(path string) error {
	if path == "" {
		return errors.New("reporting: the sent log has no path")
	}
	data, err := json.MarshalIndent(stateFile{
		Format:     stateFormat,
		Sent:       s.Sent,
		CountWeek:  s.CountWeek,
		RetryAfter: s.RetryAfter,
	}, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, data, ".reports-*.json")
}

// SentInLast24h counts the entries sent within 24 hours before now.
func (s *State) SentInLast24h(now time.Time) int {
	if s == nil {
		return 0
	}
	since := now.Add(-24 * time.Hour)
	count := 0
	for _, e := range s.Sent {
		if e.SentAt.After(since) {
			count++
		}
	}
	return count
}

// Remaining returns how many reports this machine may still send: MaxPerDay
// minus the reports of the last 24 hours, never below 0.
func (s *State) Remaining(now time.Time) int {
	left := MaxPerDay - s.SentInLast24h(now)
	if left < 0 {
		return 0
	}
	return left
}

// WasSent reports whether the problem problemID was already sent in week.
// A count has no problem ID and is never "already sent".
func (s *State) WasSent(problemID, week string) bool {
	if s == nil || problemID == "" {
		return false
	}
	for _, e := range s.Sent {
		if e.ProblemID == problemID && e.Week == week {
			return true
		}
	}
	return false
}

// CountDue reports whether this week's count was not sent yet.
func (s *State) CountDue(now time.Time) bool {
	if s == nil {
		return true
	}
	return s.CountWeek != problems.ISOWeek(now)
}

// Paused reports whether a failed send pauses automatic sending now.
func (s *State) Paused(now time.Time) bool {
	if s == nil {
		return false
	}
	return now.Before(s.RetryAfter)
}

// Record adds a problem report entry and trims the log.
func (s *State) Record(e Entry) {
	if s == nil {
		return
	}
	s.Sent = append(s.Sent, e)
	s.trim(e.SentAt)
}

// RecordCount records the count of week and its entry. A count is recorded
// even when its entry is trimmed later; the week is what stops a second
// count in the same week.
func (s *State) RecordCount(week string, e Entry) {
	if s == nil {
		return
	}
	s.CountWeek = week
	s.Sent = append(s.Sent, e)
	s.trim(e.SentAt)
}

// Fail pauses automatic sending for RetryAfter from now.
func (s *State) Fail(now time.Time) {
	if s == nil {
		return
	}
	s.RetryAfter = now.UTC().Add(RetryAfter)
}

// trim drops the oldest entries past MaxEntries and every entry older than
// RetainFor before the newest entry's time.
func (s *State) trim(now time.Time) {
	if len(s.Sent) == 0 {
		return
	}
	sort.SliceStable(s.Sent, func(i, j int) bool { return s.Sent[i].SentAt.Before(s.Sent[j].SentAt) })
	newest := now
	if newest.IsZero() {
		newest = s.Sent[len(s.Sent)-1].SentAt
	}
	cutoff := newest.Add(-RetainFor)
	kept := s.Sent[:0]
	for _, e := range s.Sent {
		if e.SentAt.Before(cutoff) {
			continue
		}
		kept = append(kept, e)
	}
	s.Sent = kept
	if len(s.Sent) > MaxEntries {
		s.Sent = append([]Entry(nil), s.Sent[len(s.Sent)-MaxEntries:]...)
	}
}
