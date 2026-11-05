package problems

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSeenPath(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	tests := []struct {
		name string
		vars map[string]string
		want string
		err  bool
	}{
		{"state home", map[string]string{"XDG_STATE_HOME": "/s", "HOME": "/h"}, "/s/chairlift/problems-seen.json", false},
		{"home", map[string]string{"HOME": "/h"}, "/h/.local/state/chairlift/problems-seen.json", false},
		{"relative state home", map[string]string{"XDG_STATE_HOME": "s", "HOME": "/h"}, "/h/.local/state/chairlift/problems-seen.json", false},
		{"nothing", map[string]string{"HOME": "h"}, "", true},
	}
	for _, tt := range tests {
		got, err := SeenPath(env(tt.vars))
		if got != tt.want || (err != nil) != tt.err {
			t.Errorf("%s: SeenPath = %q, %v; want %q, error %v", tt.name, got, err, tt.want, tt.err)
		}
	}
}

// TestDismissHidesUntilTheProblemComesBack: a dismissed item stays hidden
// until its last time is newer than the dismissal.
func TestDismissHidesUntilTheProblemComesBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "chairlift", "problems-seen.json")
	seen, err := LoadSeen(path)
	if err != nil || seen.Len() != 0 {
		t.Fatalf("LoadSeen(missing) = %d entries, %v", seen.Len(), err)
	}
	last := time.Date(2026, 11, 5, 2, 0, 0, 0, time.UTC)
	item := Item{ID: "00000000000000aa", LastSeen: last}
	at := last.Add(time.Hour)
	if err := seen.Dismiss(item.ID, at); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("store mode = %v, want 0600", info.Mode().Perm())
	}
	if dir, _ := os.Stat(filepath.Dir(path)); dir.Mode().Perm() != 0o700 {
		t.Errorf("folder mode = %v, want 0700", dir.Mode().Perm())
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".problems-seen-*"))
	if len(leftovers) != 0 {
		t.Errorf("temporary files left: %v", leftovers)
	}

	reloaded, err := LoadSeen(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		last   time.Time
		hidden bool
	}{
		{"same last time", last, true},
		{"last time equals the dismissal", at, true},
		{"seen again after the dismissal", at.Add(time.Second), false},
	} {
		it := item
		it.LastSeen = tt.last
		if got := reloaded.Hidden(it); got != tt.hidden {
			t.Errorf("%s: Hidden = %v, want %v", tt.name, got, tt.hidden)
		}
	}
	if reloaded.Hidden(Item{ID: "00000000000000bb", LastSeen: last}) {
		t.Error("an item that was never dismissed is hidden")
	}
	visible, hidden := FilterSeen([]Item{item, {ID: "00000000000000bb"}}, reloaded)
	if len(visible) != 1 || hidden != 1 || visible[0].ID != "00000000000000bb" {
		t.Errorf("FilterSeen = %v, %d", visible, hidden)
	}
	if visible, hidden := FilterSeen([]Item{item}, nil); len(visible) != 1 || hidden != 0 {
		t.Error("a nil store hid an item")
	}
}

func TestDismissKeepsAtMost1000(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	seen := NewSeen(path)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < MaxSeen; i++ {
		seen.at[fmt.Sprintf("%016x", i)] = base.Add(time.Duration(i) * time.Minute)
	}
	if err := seen.Dismiss("crash-00000000000000ff", base.Add(time.Hour*1000)); err != nil {
		t.Fatal(err)
	}
	if seen.Len() != MaxSeen {
		t.Fatalf("store holds %d ids, want %d", seen.Len(), MaxSeen)
	}
	if seen.Hidden(Item{ID: fmt.Sprintf("%016x", 0), LastSeen: base}) {
		t.Error("the oldest dismissal was kept")
	}
	reloaded, err := LoadSeen(path)
	if err != nil || reloaded.Len() != MaxSeen {
		t.Fatalf("reloaded = %d, %v", reloaded.Len(), err)
	}
}

func TestDismissFailureKeepsTheStore(t *testing.T) {
	dir := t.TempDir()
	blocker := writeTestFile(t, dir, "file", "x")
	seen := NewSeen(filepath.Join(blocker, "seen.json"))
	if err := seen.Dismiss("00000000000000aa", time.Now()); err == nil {
		t.Fatal("Dismiss under a file gave no error")
	}
	if seen.Len() != 0 {
		t.Error("a failed write changed the store")
	}
	if err := NewSeen("").Dismiss("00000000000000aa", time.Now()); err == nil {
		t.Error("Dismiss with no path gave no error")
	}
	if err := seen.Dismiss("../x", time.Now()); err == nil {
		t.Error("Dismiss of a bad id gave no error")
	}
}

func TestLoadSeenBadFiles(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"bad.json":    "{",
		"format.json": `{"format": 2, "dismissed": {}}`,
		"large.json":  `{"format": 1, "dismissed": {}}` + strings.Repeat(" ", maxSeenBytes),
	} {
		seen, err := LoadSeen(writeTestFile(t, dir, name, content))
		if err == nil || seen == nil || seen.Len() != 0 {
			t.Errorf("%s: LoadSeen = %v, %v; want an empty store and an error", name, seen, err)
		}
	}
	if seen, err := LoadSeen(dir); err == nil || seen.Len() != 0 {
		t.Error("LoadSeen(directory) gave no error")
	}
	path := writeTestFile(t, dir, "ids.json", `{"format": 1, "dismissed": {"../x": "2026-01-01T00:00:00Z", "app-00000000000000aa": "2026-01-01T00:00:00Z"}}`)
	seen, err := LoadSeen(path)
	if err != nil || seen.Len() != 1 {
		t.Fatalf("LoadSeen = %d ids, %v; want only the valid id", seen.Len(), err)
	}
}

func TestDismissAt(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	if got := DismissAt(Item{LastSeen: now.Add(-time.Hour)}, now); !got.Equal(now) {
		t.Errorf("DismissAt(older) = %v", got)
	}
	future := now.Add(time.Hour)
	if got := DismissAt(Item{LastSeen: future}, now); !got.Equal(future) {
		t.Errorf("DismissAt(future) = %v, want the item's last time", got)
	}
}
