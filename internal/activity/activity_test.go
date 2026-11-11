package activity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultPath(t *testing.T) {
	env := map[string]string{"HOME": "/home/frosty"}
	got, err := DefaultPath(func(k string) string { return env[k] })
	if err != nil || got != "/home/frosty/.local/state/chairlift/activity.jsonl" {
		t.Errorf("DefaultPath = %q, %v", got, err)
	}
	env["XDG_STATE_HOME"] = "/state"
	if got, _ := DefaultPath(func(k string) string { return env[k] }); got != "/state/chairlift/activity.jsonl" {
		t.Errorf("DefaultPath with XDG_STATE_HOME = %q", got)
	}
	env = map[string]string{"XDG_STATE_HOME": "relative"}
	if _, err := DefaultPath(func(k string) string { return env[k] }); err == nil {
		t.Error("relative paths gave no error")
	}
}

func TestAddWritesAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chairlift", FileName)
	l, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxEntries+5; i++ {
		if err := l.Add("flatpak", "Updated app", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Add("image", "Staged Snow 2026.11.08", "Applies on restart"); err != nil {
		t.Fatal(err)
	}
	again, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	recent := again.Recent(3)
	if len(recent) != 3 || recent[0].Title != "Staged Snow 2026.11.08" || recent[0].Detail != "Applies on restart" {
		t.Errorf("Recent = %+v", recent)
	}
	if n := len(again.Recent(1000)); n != MaxEntries {
		t.Errorf("kept %d entries, want %d", n, MaxEntries)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("state directory mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chairlift", FileName)
	l, _ := Open(path, true)
	if err := l.Add("features", "Turned on Docker", ""); err != nil {
		t.Fatal(err)
	}
	if len(l.Recent(5)) != 1 {
		t.Error("dry run did not keep the entry in memory")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Errorf("dry run created %s: %v", filepath.Dir(path), err)
	}
}

func TestOpenSkipsBadLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	data := `{"at":"2026-11-09T10:00:00Z","source":"brew","title":"Upgraded gh"}
not json
{"at":"2026-11-09T11:00:00Z","source":"brew"}
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := l.Recent(10); len(got) != 1 || got[0].Title != "Upgraded gh" {
		t.Errorf("Recent = %+v", got)
	}
	var nilLog *Log
	if nilLog.Add("x", "y", "") != nil || nilLog.Recent(3) != nil {
		t.Error("a nil log is not inert")
	}
}

func TestWhen(t *testing.T) {
	now := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		at   time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "just now"},
		{now.Add(-12 * time.Minute), "12 min ago"},
		{now.Add(-3 * time.Hour), "3 h ago"},
		{time.Date(2026, 11, 9, 22, 0, 0, 0, time.UTC), "Yesterday"},
		{time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC), "Fri"},
		{time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC), "Oct 3"},
	}
	for _, tc := range cases {
		if got := When(tc.at, now); got != tc.want {
			t.Errorf("When(%v) = %q, want %q", tc.at, got, tc.want)
		}
	}
	if !strings.HasSuffix(When(now.Add(-90*time.Minute), now), "h ago") {
		t.Error("90 minutes is not in hours")
	}
}
