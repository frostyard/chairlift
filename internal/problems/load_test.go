package problems

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecideFixtures(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		dryRun bool
		want   FixtureDecision
	}{
		{"unset", "", true, FixtureDecision{}},
		{"unset live", "", false, FixtureDecision{}},
		{"absolute in dry run", "/tmp/fixtures/", true, FixtureDecision{Use: true, Dir: "/tmp/fixtures"}},
		{"live run", "/tmp/fixtures", false, FixtureDecision{Log: "problems: CHAIRLIFT_PROBLEMS_FIXTURES ignored: it needs --dry-run"}},
		{"relative", "fixtures", true, FixtureDecision{Log: "problems: CHAIRLIFT_PROBLEMS_FIXTURES ignored: not an absolute path"}},
		{"relative live", "fixtures", false, FixtureDecision{Log: "problems: CHAIRLIFT_PROBLEMS_FIXTURES ignored: it needs --dry-run"}},
	}
	for _, tt := range tests {
		if got := DecideFixtures(tt.value, tt.dryRun); got != tt.want {
			t.Errorf("%s: DecideFixtures(%q, %v) = %+v, want %+v", tt.name, tt.value, tt.dryRun, got, tt.want)
		}
	}
	if FixturesEnv != "CHAIRLIFT_PROBLEMS_FIXTURES" {
		t.Errorf("FixturesEnv = %q", FixturesEnv)
	}
}

// fixtureDir copies the snosi and user fixtures into 1 folder, in the
// layout of the dry-run hook.
func fixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range map[string]string{
		"problems.json":  fixturePath("snosi", "problems.json"),
		"details.json":   fixturePath("snosi", "details.json"),
		"coredumps.json": fixturePath("user", "coredumps.json"),
		"journal.json":   fixturePath("user", "journal.json"),
	} {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, name, string(data))
	}
	return dir
}

func TestLoadFromFixtures(t *testing.T) {
	dir := fixtureDir(t)
	seen := NewSeen(filepath.Join(t.TempDir(), "seen.json"))
	if err := seen.Dismiss("2b4cfb6309b48807", time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	snap := Load(context.Background(), Options{
		Now: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC), UID: fixtureUID,
		System: true, Apps: true, Seen: seen,
		Fixtures:    FixtureDecision{Use: true, Dir: dir},
		Coredumpctl: "chairlift-test-must-not-run", Journalctl: "chairlift-test-must-not-run",
	})
	if snap.System.Outcome != SourceOK || snap.DetailsErr != nil || snap.CoredumpErr != nil || snap.JournalErr != nil {
		t.Fatalf("snapshot errors: %s %v %v %v %v", snap.System.Outcome, snap.System.Err, snap.DetailsErr, snap.CoredumpErr, snap.JournalErr)
	}
	// 17 system items: 1 merged into the apps group, 1 dismissed.
	if len(snap.SystemItems) != 15 || snap.Merged != 1 || snap.SystemHidden != 1 {
		t.Errorf("system = %d, merged = %d, hidden = %d; want 15, 1, 1", len(snap.SystemItems), snap.Merged, snap.SystemHidden)
	}
	// 4 app record groups and 3 crash groups.
	if len(snap.AppItems) != 7 || snap.JournalDropped != 3 || snap.AppsHidden != 0 {
		t.Errorf("apps = %d, dropped = %d; want 7 and 3", len(snap.AppItems), snap.JournalDropped)
	}
	for i := 1; i < len(snap.AppItems); i++ {
		if snap.AppItems[i].LastSeen.After(snap.AppItems[i-1].LastSeen) {
			t.Fatal("app items are not sorted newest first")
		}
	}
	unit := itemByID(t, snap.SystemItems, "a6d8d298c66718de")
	if len(unit.Local.Messages) == 0 {
		t.Error("details.json was not applied")
	}
	lines := strings.Join(snap.LogLines(), "\n")
	for _, want := range []string{
		"problems: dropped 3 journal records that failed validation",
		"problems: 1 system crashes shown once in the apps group",
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("log lines do not contain %q:\n%s", want, lines)
		}
	}
	if snap.SourceInvalid() != nil {
		t.Error("SourceInvalid for a valid file")
	}
}

func TestLoadGroups(t *testing.T) {
	dir := fixtureDir(t)
	opts := Options{Fixtures: FixtureDecision{Use: true, Dir: dir}, UID: fixtureUID}

	if snap := Load(context.Background(), opts); snap.System.Outcome != SourceMissing || snap.SystemItems != nil || snap.AppItems != nil {
		t.Errorf("no group: %+v", snap)
	}
	opts.System = true
	snap := Load(context.Background(), opts)
	if len(snap.SystemItems) != 17 || snap.AppItems != nil || snap.Merged != 0 {
		t.Errorf("system only: system = %d, apps = %d, merged = %d", len(snap.SystemItems), len(snap.AppItems), snap.Merged)
	}
	opts.System, opts.Apps = false, true
	snap = Load(context.Background(), opts)
	if snap.SystemItems != nil || len(snap.AppItems) != 7 {
		t.Errorf("apps only: system = %d, apps = %d", len(snap.SystemItems), len(snap.AppItems))
	}
	for _, it := range snap.AppItems {
		if it.Origin == OriginAppRecord && !it.Envelope.Valid {
			t.Error("user items did not get the problems.json envelope")
		}
	}
}

func TestLoadFixtureFailures(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "problems.json", `{"format": 1, "schema": "frostyard.problem/2", "problems": []}`)
	writeTestFile(t, dir, "coredumps.json", `[{"time": "x"}]`)
	if err := os.Mkdir(filepath.Join(dir, "journal.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	snap := Load(context.Background(), Options{System: true, Apps: true, Fixtures: FixtureDecision{Use: true, Dir: dir}})
	if snap.System.Outcome != SourceMalformed || snap.SourceInvalid() == nil {
		t.Errorf("outcome = %s, SourceInvalid = %v", snap.System.Outcome, snap.SourceInvalid())
	}
	if snap.CoredumpErr == nil || snap.JournalErr == nil {
		t.Errorf("coredump err = %v, journal err = %v", snap.CoredumpErr, snap.JournalErr)
	}
	lines := strings.Join(snap.LogLines(), "\n")
	for _, want := range []string{"problems: system checks malformed:", "problems: crash list not read:", "problems: app problem records not read:"} {
		if !strings.Contains(lines, want) {
			t.Errorf("log lines do not contain %q:\n%s", want, lines)
		}
	}

	// Missing fixture files are empty sources; a too large one is an error.
	empty := t.TempDir()
	snap = Load(context.Background(), Options{Apps: true, Fixtures: FixtureDecision{Use: true, Dir: empty}})
	if snap.CoredumpErr != nil || snap.JournalErr != nil || len(snap.AppItems) != 0 {
		t.Errorf("empty fixtures: %v %v %d", snap.CoredumpErr, snap.JournalErr, len(snap.AppItems))
	}
	writeTestFile(t, empty, "coredumps.json", strings.Repeat(" ", maxCommandOutput+1))
	if snap = Load(context.Background(), Options{Apps: true, Fixtures: FixtureDecision{Use: true, Dir: empty}}); snap.CoredumpErr == nil {
		t.Error("a too large coredumps fixture gave no error")
	}
}

// TestLoadLiveRunsTheCommands runs Load without fixtures, with fake
// coredumpctl and journalctl commands that print the fixtures.
func TestLoadLiveRunsTheCommands(t *testing.T) {
	coredumps, err := filepath.Abs(fixturePath("user", "coredumps.json"))
	if err != nil {
		t.Fatal(err)
	}
	journal, err := filepath.Abs(fixturePath("user", "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(t.TempDir(), "args")
	snap := Load(context.Background(), Options{
		Now: time.Now(), UID: fixtureUID, Apps: true,
		Coredumpctl: fakeCommand(t, "cat "+coredumps),
		Journalctl:  fakeCommand(t, `echo "$@" > `+argsFile+`; cat `+journal),
	})
	if snap.CoredumpErr != nil || snap.JournalErr != nil {
		t.Fatalf("errors: %v %v", snap.CoredumpErr, snap.JournalErr)
	}
	if len(snap.AppItems) != 7 || snap.JournalDropped != 3 {
		t.Errorf("apps = %d, dropped = %d; want 7 and 3", len(snap.AppItems), snap.JournalDropped)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--user", "-n 500", "--since=@", "MESSAGE_ID=8915bdd215c84cbab0c31c05a1a33e7e"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("journalctl args %q do not contain %q", args, want)
		}
	}

	failing := Load(context.Background(), Options{
		Apps:        true,
		Coredumpctl: fakeCommand(t, "echo boom >&2; exit 3"),
		Journalctl:  fakeCommand(t, "echo boom >&2; exit 3"),
	})
	if failing.CoredumpErr == nil || failing.JournalErr == nil || len(failing.AppItems) != 0 {
		t.Errorf("failing commands: %v %v %d", failing.CoredumpErr, failing.JournalErr, len(failing.AppItems))
	}
}

func TestLogLinesOfSystemDropsAndDetails(t *testing.T) {
	snap := Snapshot{System: SystemFile{Dropped: 2, Truncated: 1}, DetailsErr: os.ErrInvalid}
	lines := strings.Join(snap.LogLines(), "\n")
	for _, want := range []string{"problems: problems.json: dropped 2 invalid and 1 past the limit of 200", "problems: details.json not used:"} {
		if !strings.Contains(lines, want) {
			t.Errorf("log lines do not contain %q:\n%s", want, lines)
		}
	}
	if len((Snapshot{}).LogLines()) != 0 {
		t.Error("an empty snapshot logs lines")
	}
}

func TestBuiltMarker(t *testing.T) {
	if got := BuiltMarker(16, 7, "ok"); got != "problems: page built system=16 apps=7 source=ok" {
		t.Errorf("BuiltMarker = %q", got)
	}
}
