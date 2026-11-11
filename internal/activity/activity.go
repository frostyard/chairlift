// Package activity keeps ChairLift's own record of what it changed on this
// computer for this user: "Updated Firefox", "Staged Snow 2026.11.08",
// "Turned on Docker". The Overview page lists the newest entries.
//
// It records only what ChairLift itself did and saw succeed; it is not a
// history of the system. The record is a small JSON Lines file under
// $XDG_STATE_HOME/chairlift (default ~/.local/state/chairlift), bounded to
// MaxEntries. In dry-run mode the log keeps entries in memory and writes
// nothing (ADR-0009). The package imports no UI code.
package activity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MaxEntries is how many entries the file keeps.
const MaxEntries = 200

// FileName is the record's name inside the state directory.
const FileName = "activity.jsonl"

// Entry is one thing ChairLift did.
type Entry struct {
	At time.Time `json:"at"`
	// Source is the area: "image", "features", "flatpak", "brew",
	// "firmware", "maintenance", "problems".
	Source string `json:"source"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// Log is the activity record. Its methods are safe for concurrent use.
type Log struct {
	mu      sync.Mutex
	path    string
	dryRun  bool
	entries []Entry
	now     func() time.Time
}

// DefaultPath is the record's path for the environment getenv reads.
func DefaultPath(getenv func(string) string) (string, error) {
	dir := getenv("XDG_STATE_HOME")
	if dir == "" || !filepath.IsAbs(dir) {
		home := getenv("HOME")
		if home == "" || !filepath.IsAbs(home) {
			return "", errors.New("activity: neither XDG_STATE_HOME nor HOME is an absolute path")
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "chairlift", FileName), nil
}

// Open reads the record at path, if there is one. A missing file is an
// empty record; an unreadable line is skipped. With dryRun the log never
// writes. An empty path keeps the record in memory only.
func Open(path string, dryRun bool) (*Log, error) {
	l := &Log{path: path, dryRun: dryRun, now: time.Now}
	if path == "" {
		return l, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // the user's own state file
	if errors.Is(err, fs.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return l, fmt.Errorf("read activity: %w", err)
	}
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		var e Entry
		if json.Unmarshal(s.Bytes(), &e) == nil && e.Title != "" && !e.At.IsZero() {
			l.entries = append(l.entries, e)
		}
	}
	if len(l.entries) > MaxEntries {
		l.entries = l.entries[len(l.entries)-MaxEntries:]
	}
	return l, nil
}

// Add records an entry now and writes the record, unless in dry run.
func (l *Log) Add(source, title, detail string) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, Entry{At: l.now(), Source: source, Title: title, Detail: detail})
	if len(l.entries) > MaxEntries {
		l.entries = l.entries[len(l.entries)-MaxEntries:]
	}
	if l.dryRun || l.path == "" {
		return nil
	}
	return l.write()
}

// write replaces the file atomically. The caller holds mu.
func (l *Log) write() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("create activity directory: %w", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, e := range l.entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), ".activity-*.tmp")
	if err != nil {
		return fmt.Errorf("write activity: %w", err)
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write activity: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write activity: %w", err)
	}
	if err := os.Rename(tmp.Name(), l.path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write activity: %w", err)
	}
	return nil
}

// Recent returns the newest n entries, newest first.
func (l *Log) Recent(n int) []Entry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if n > len(l.entries) {
		n = len(l.entries)
	}
	out := make([]Entry, 0, n)
	for i := len(l.entries) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, l.entries[i])
	}
	return out
}

// When says how long ago at was, as the Overview lists it: "just now",
// "12 min ago", "3 h ago", "Yesterday", "Mon" within a week, else "Nov 3".
func When(at, now time.Time) string {
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d/time.Minute))
	}
	y1, m1, d1 := at.Date()
	y2, m2, d2 := now.Date()
	today := time.Date(y2, m2, d2, 0, 0, 0, 0, now.Location())
	day := time.Date(y1, m1, d1, 0, 0, 0, 0, at.Location())
	switch {
	case !day.Before(today):
		return fmt.Sprintf("%d h ago", int(d/time.Hour))
	case !day.Before(today.AddDate(0, 0, -1)):
		return "Yesterday"
	case !day.Before(today.AddDate(0, 0, -6)):
		return at.Format("Mon")
	}
	return at.Format("Jan 2")
}
