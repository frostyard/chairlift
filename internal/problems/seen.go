package problems

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Dismiss store limits.
const (
	// MaxSeen bounds the dismissed ids; the oldest dismissals go first.
	MaxSeen = 1000
	// maxSeenBytes bounds the file that LoadSeen reads.
	maxSeenBytes = 1 << 20
)

// itemIDRe matches every item ID: a system id, or a user item id.
var itemIDRe = regexp.MustCompile(`^((crash-|app-)?[0-9a-f]{16})$`)

// SeenPath returns the dismiss store of this user:
// $XDG_STATE_HOME/chairlift/problems-seen.json, else
// $HOME/.local/state/chairlift/problems-seen.json. Only absolute values
// count. getenv is os.Getenv in production.
func SeenPath(getenv func(string) string) (string, error) {
	if dir := getenv("XDG_STATE_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "chairlift", "problems-seen.json"), nil
	}
	if home := getenv("HOME"); filepath.IsAbs(home) {
		return filepath.Join(home, ".local", "state", "chairlift", "problems-seen.json"), nil
	}
	return "", errors.New("problems: neither XDG_STATE_HOME nor HOME is an absolute path")
}

// seenFile is the JSON form of the dismiss store.
type seenFile struct {
	Format    int                  `json:"format"`
	Dismissed map[string]time.Time `json:"dismissed"`
}

// Seen holds the items this user dismissed: item ID to dismiss time. It is
// safe for concurrent use.
type Seen struct {
	path string
	mu   sync.Mutex
	at   map[string]time.Time
}

// NewSeen returns an empty store that writes to path.
func NewSeen(path string) *Seen {
	return &Seen{path: path, at: map[string]time.Time{}}
}

// LoadSeen reads the store at path. A missing file is an empty store. A
// file that cannot be read or parsed gives an empty store and the error; a
// later Dismiss replaces it. Entries with an id that is not an item ID are
// ignored.
func LoadSeen(path string) (*Seen, error) {
	s := NewSeen(path)
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxSeenBytes+1))
	if err != nil {
		return s, err
	}
	if len(data) > maxSeenBytes {
		return s, fmt.Errorf("%s: larger than %d bytes", path, maxSeenBytes)
	}
	var file seenFile
	if err := json.Unmarshal(data, &file); err != nil || file.Format != 1 {
		return s, fmt.Errorf("%s: not a format 1 dismiss store", path)
	}
	for id, at := range file.Dismissed {
		if itemIDRe.MatchString(id) {
			s.at[id] = at
		}
	}
	s.trim()
	return s, nil
}

// Hidden reports whether it is dismissed: it was dismissed, and its last
// time is not newer than the dismissal. A problem that comes back after the
// dismissal shows again.
func (s *Seen) Hidden(it Item) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	at, ok := s.at[it.ID]
	return ok && !it.LastSeen.After(at)
}

// Dismiss records id as dismissed at at and writes the store: a temporary
// file in the same folder, mode 0600, then a rename. The folder is made
// with mode 0700. On a write failure the store in memory keeps the
// dismissal out, so a retry can succeed and Hidden stays as the file says.
func (s *Seen) Dismiss(id string, at time.Time) error {
	if !itemIDRe.MatchString(id) {
		return fmt.Errorf("problems: %q is not an item id", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]time.Time, len(s.at)+1)
	for k, v := range s.at {
		next[k] = v
	}
	next[id] = at.UTC()
	trimSeen(next)
	if err := writeSeen(s.path, next); err != nil {
		return err
	}
	s.at = next
	return nil
}

// Len returns the number of dismissed ids.
func (s *Seen) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.at)
}

func (s *Seen) trim() {
	s.mu.Lock()
	defer s.mu.Unlock()
	trimSeen(s.at)
}

// trimSeen removes the oldest dismissals past MaxSeen.
func trimSeen(at map[string]time.Time) {
	if len(at) <= MaxSeen {
		return
	}
	ids := make([]string, 0, len(at))
	for id := range at {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if !at[ids[i]].Equal(at[ids[j]]) {
			return at[ids[i]].Before(at[ids[j]])
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids[:len(ids)-MaxSeen] {
		delete(at, id)
	}
}

// writeSeen writes the store by a temporary file and a rename.
func writeSeen(path string, at map[string]time.Time) error {
	if path == "" {
		return errors.New("problems: the dismiss store has no path")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(seenFile{Format: 1, Dismissed: at}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".problems-seen-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		return cleanup(err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// DismissAt returns the dismiss time of it: now, or the item's last time
// when that is later (a clock that runs behind must not make the item come
// back at once).
func DismissAt(it Item, now time.Time) time.Time {
	if it.LastSeen.After(now) {
		return it.LastSeen
	}
	return now
}

// FilterSeen returns the items that are not hidden and the number hidden.
func FilterSeen(items []Item, s *Seen) ([]Item, int) {
	visible := make([]Item, 0, len(items))
	hidden := 0
	for _, it := range items {
		if s.Hidden(it) {
			hidden++
			continue
		}
		visible = append(visible, it)
	}
	return visible, hidden
}
