package pageview

import (
	"strings"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

var problemsNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func problemItem(kind string, count int, last time.Time, fields ...problems.Field) problems.Item {
	return problems.Item{Kind: kind, KnownKind: true, Count: count, LastSeen: last, Fields: fields}
}

func validField(name, value string) problems.Field {
	return problems.Field{Name: name, Display: value, Value: value, Valid: true}
}

func TestProblemWhen(t *testing.T) {
	tests := []struct {
		name  string
		last  time.Time
		count int
		want  string
	}{
		{"today", time.Date(2026, 11, 5, 3, 56, 0, 0, time.UTC), 1, "Today at 03:56"},
		{"yesterday", time.Date(2026, 11, 4, 23, 5, 0, 0, time.UTC), 1, "Yesterday at 23:05"},
		{"this year", time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC), 1, "On Oct 1 at 08:00"},
		{"last year", time.Date(2025, 12, 31, 8, 0, 0, 0, time.UTC), 1, "On Dec 31, 2025 at 08:00"},
		{"more than once", time.Date(2026, 11, 5, 3, 56, 0, 0, time.UTC), 3, "3 times, last today at 03:56"},
		{"zero time", time.Time{}, 1, "Time unknown"},
	}
	for _, tt := range tests {
		item := problemItem(problems.KindCount, tt.count, tt.last)
		if got := ProblemWhen(item, problemsNow, time.UTC); got != tt.want {
			t.Errorf("%s: ProblemWhen = %q, want %q", tt.name, got, tt.want)
		}
	}
	// The location decides the day.
	berlin := time.FixedZone("UTC+2", 2*3600)
	late := problemItem(problems.KindCount, 1, time.Date(2026, 11, 4, 23, 0, 0, 0, time.UTC))
	if got := ProblemWhen(late, problemsNow, berlin); got != "Today at 01:00" {
		t.Errorf("ProblemWhen in UTC+2 = %q", got)
	}
	if got := ProblemWhen(late, problemsNow, nil); got != "Yesterday at 23:00" {
		t.Errorf("ProblemWhen with a nil location = %q", got)
	}
}

func TestProblemRow(t *testing.T) {
	last := time.Date(2026, 11, 5, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		item problems.Item
		want Row
	}{
		{"unit", problemItem(problems.KindUnitFailed, 1, last, validField("unit", "user@.service")),
			Row{"A system service failed", "Today at 03:00 · user@.service"}},
		{"unit from local details", func() problems.Item {
			it := problemItem(problems.KindUnitFailed, 1, last)
			it.Local.Units = []string{"brian-backup.service"}
			return it
		}(), Row{"A system service failed", "Today at 03:00 · brian-backup.service"}},
		{"feature", problemItem(problems.KindSysextUpdateFailed, 2, last, problems.Field{Name: "feature", Display: "docker"}),
			Row{"A system feature could not be updated", "2 times, last today at 03:00 · docker"}},
		{"stage", problemItem(problems.KindUpdateStageFailed, 1, last, validField("target_version", "20261104000000")),
			Row{"An update could not be prepared", "Today at 03:00 · version 20261104000000"}},
		{"older image", problemItem(problems.KindBootedOlderImage, 1, last, validField("newer_version", "20261104000000")),
			Row{"The computer started an older system version", "Today at 03:00 · newer version 20261104000000"}},
		{"app code", problemItem(problems.KindAppProblem, 1, last, validField("app_id", "org.frostyard.Frostmail"),
			validField("class", "error"), validField("code", "store.migrate_failed")),
			Row{"Frostmail reported an error", "Today at 03:00 · store.migrate_failed"}},
		{"no context", problemItem(problems.KindIntegrityError, 1, last), Row{"System files failed an integrity check", "Today at 03:00"}},
		{"unknown kind", problems.Item{Kind: "disk_full", Count: 1, LastSeen: last},
			Row{"An unknown problem was recorded", "Today at 03:00 · kind disk_full"}},
	}
	for _, tt := range tests {
		if got := Problem(tt.item, problemsNow, time.UTC); got != tt.want {
			t.Errorf("%s: Problem = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// TestProblemsSystemStatus covers every problems.json outcome.
func TestProblemsSystemStatus(t *testing.T) {
	generated := time.Date(2026, 11, 5, 3, 56, 0, 0, time.UTC)
	tests := []struct {
		name      string
		outcome   problems.SourceOutcome
		generated time.Time
		shown     int
		hidden    int
		wantDesc  string
		wantEmpty *Row
	}{
		{"missing", problems.SourceMissing, time.Time{}, 0, 0, ProblemsSystemDescription,
			&Row{Title: "System checks are not available on this system"}},
		{"unreadable", problems.SourceUnreadable, time.Time{}, 0, 0, ProblemsSystemDescription,
			&Row{"System checks could not be read", "See the log for details."}},
		{"malformed", problems.SourceMalformed, time.Time{}, 0, 0, ProblemsSystemDescription,
			&Row{"System checks could not be read", "See the log for details."}},
		{"ok and empty", problems.SourceOK, generated, 0, 0, ProblemsSystemDescription + ". Last checked today at 03:56",
			&Row{"No problems found", "Last checked today at 03:56"}},
		{"ok with items", problems.SourceOK, generated, 4, 0, ProblemsSystemDescription + ". Last checked today at 03:56", nil},
		{"ok with 1 hidden", problems.SourceOK, generated, 4, 1, ProblemsSystemDescription + ". Last checked today at 03:56. 1 dismissed problem is hidden", nil},
		{"ok with 2 hidden", problems.SourceOK, generated, 0, 2, ProblemsSystemDescription + ". Last checked today at 03:56. 2 dismissed problems are hidden",
			&Row{"No problems found", "Last checked today at 03:56"}},
		{"ok with no generated time", problems.SourceOK, time.Time{}, 0, 0, ProblemsSystemDescription + ". Last check time unknown",
			&Row{"No problems found", "Last check time unknown"}},
	}
	for _, tt := range tests {
		got := ProblemsSystemStatus(tt.outcome, tt.generated, tt.shown, tt.hidden, problemsNow, time.UTC)
		if got.Description != tt.wantDesc {
			t.Errorf("%s: description = %q, want %q", tt.name, got.Description, tt.wantDesc)
		}
		if (got.Empty == nil) != (tt.wantEmpty == nil) || got.Empty != nil && *got.Empty != *tt.wantEmpty {
			t.Errorf("%s: empty row = %+v, want %+v", tt.name, got.Empty, tt.wantEmpty)
		}
	}
}

func TestProblemsAppsStatus(t *testing.T) {
	empty := ProblemsAppsStatus(0, 0)
	if empty.Empty == nil || empty.Empty.Title != "No problems found" || empty.Description != ProblemsAppsDescription {
		t.Errorf("empty apps status = %+v", empty)
	}
	shown := ProblemsAppsStatus(3, 2)
	if shown.Empty != nil || !strings.HasSuffix(shown.Description, ". 2 dismissed problems are hidden") {
		t.Errorf("apps status = %+v", shown)
	}
}

// TestProblemsPhase1PromiseIsGone proves the phase-1 row that promised
// nothing is sent cannot come back: reporting is now opt-in, and its rows
// live in pageview/reporting.go.
func TestProblemsPhase1PromiseIsGone(t *testing.T) {
	if ReportingTitle != "Reporting" {
		t.Errorf("ReportingTitle = %q, want Reporting", ReportingTitle)
	}
	if ReportingOff != "Off: nothing leaves this computer." {
		t.Errorf("ReportingOff = %q", ReportingOff)
	}
}

func TestProblemDetails(t *testing.T) {
	it := problemItem(problems.KindCrash, 1, problemsNow,
		problems.Field{Name: "exe", Display: "tool", Reason: "not a program under /usr"},
		validField("signal", "SIGABRT"))
	it.Missing = []string{"package"}
	it.Local = problems.Local{
		Values:   []problems.Field{{Name: "exe", Display: "tool"}},
		Units:    []string{"a.service", "b.service"},
		ExePath:  "/home/brian/tool",
		Comm:     "tool",
		Unit:     "app.scope",
		Message:  "boom",
		Messages: []string{"log 1", "log 2"},
	}
	rows := ProblemDetails(it)
	if len(rows) != 3 {
		t.Fatalf("rows = %+v, want hint, values, local", rows)
	}
	if rows[0].Title != "What it means" || rows[0].Subtitle != problems.Hint(it) {
		t.Errorf("hint row = %+v", rows[0])
	}
	if want := "exe: tool (not valid: not a program under /usr)\nsignal: SIGABRT\npackage: (not recorded)"; rows[1] != (Row{"Recorded values", want}) {
		t.Errorf("values row = %+v", rows[1])
	}
	if want := "exe: tool\nUnits: a.service, b.service\nProgram: /home/brian/tool\nProcess: tool\nUnit: app.scope\nMessage: boom\nlog 1\nlog 2"; rows[2] != (Row{"On this computer", want}) {
		t.Errorf("local row = %+v", rows[2])
	}
	if rows := ProblemDetails(problems.Item{Kind: "x"}); len(rows) != 1 {
		t.Errorf("an item with no values has rows %+v", rows)
	}
}

func TestCapitalize(t *testing.T) {
	if capitalize("") != "" || capitalize("today") != "Today" {
		t.Error("capitalize")
	}
}
