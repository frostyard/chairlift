package pageview

import (
	"strings"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
)

// reportingConsent returns a consent for the current schema.
func reportingConsent(t *testing.T, enabled bool, mode reporting.Mode) reporting.Consent {
	t.Helper()
	return reporting.Decide(enabled, mode, time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC))
}

// TestReportingDescription proves the switch's description follows the
// consent: off, on with its mode, and the stale-format text.
func TestReportingDescription(t *testing.T) {
	tests := []struct {
		name    string
		consent reporting.Consent
		want    string
	}{
		{"off", reporting.Consent{}, ReportingOff},
		{"on asking", reportingConsent(t, true, reporting.ModeAsk), "On: " + ReportingModeAsk + "."},
		{"on automatic", reportingConsent(t, true, reporting.ModeAuto), "On: " + ReportingModeAuto + "."},
		{"stale format", reporting.Consent{Format: 1, Enabled: true, Mode: reporting.ModeAuto, Schema: "frostyard.problem/0"}, ReportingStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReportingDescription(tt.consent); got != tt.want {
				t.Errorf("ReportingDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReportingModeRow(t *testing.T) {
	if got := ReportingMode(reportingConsent(t, true, reporting.ModeAuto)); got != (Row{Title: ReportingModeTitle, Subtitle: ReportingModeAuto}) {
		t.Errorf("ReportingMode(auto) = %+v", got)
	}
	if got := ReportingMode(reportingConsent(t, true, reporting.ModeAsk)); got.Subtitle != ReportingModeAsk {
		t.Errorf("ReportingMode(ask).Subtitle = %q, want %q", got.Subtitle, ReportingModeAsk)
	}
}

// TestReportingWhat pins the privacy link.
func TestReportingWhat(t *testing.T) {
	got := ReportingWhat()
	if got.Title != ReportingWhatTitle {
		t.Errorf("ReportingWhat().Title = %q, want %q", got.Title, ReportingWhatTitle)
	}
	if !strings.HasPrefix(got.Subtitle, "https://") {
		t.Errorf("ReportingWhat().Subtitle = %q, want a URL", got.Subtitle)
	}
}

// TestSentRows proves the sent list: newest first, the date and the kind's
// title, the report ID, and at most 50 rows.
func TestSentRows(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	st := &reporting.State{}
	for i := 0; i < 3; i++ {
		st.Record(reporting.Entry{
			ReportID:  "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5" + string(rune('0'+i)),
			Kind:      problems.KindUnitFailed,
			ProblemID: "0123456789abcdef",
			Week:      "2026-W45",
			SentAt:    start.Add(time.Duration(i) * 24 * time.Hour),
		})
	}
	rows := SentRows(st, loc)
	if len(rows) != 3 {
		t.Fatalf("SentRows() = %d rows, want 3", len(rows))
	}
	if !strings.HasPrefix(rows[0].Title, "Nov 6, 2026 · ") {
		t.Errorf("the first row is %q, want the newest report", rows[0].Title)
	}
	if !strings.HasSuffix(rows[0].Title, SentKindTitle(problems.KindUnitFailed)) {
		t.Errorf("the first row is %q, want the kind's title", rows[0].Title)
	}
	if rows[0].Subtitle != "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a52" {
		t.Errorf("the first row's subtitle is %q, want the report ID", rows[0].Subtitle)
	}
	if got := SentRows(nil, loc); len(got) != 0 {
		t.Errorf("SentRows(nil) = %d rows, want 0", len(got))
	}

	many := &reporting.State{}
	for i := 0; i < MaxSentRows+10; i++ {
		many.Record(reporting.Entry{ReportID: "id", Kind: problems.KindCount, SentAt: start.Add(time.Duration(i) * time.Minute)})
	}
	if got := SentRows(many, loc); len(got) != MaxSentRows {
		t.Errorf("SentRows() = %d rows, want %d", len(got), MaxSentRows)
	}
	if got := SentKindTitle(problems.KindCount); got != "Weekly count" {
		t.Errorf("SentKindTitle(count) = %q", got)
	}
}

// TestReportingCountRow proves the count row: Review… only in Ask mode with
// the count due.
func TestReportingCountRow(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	ask := reportingConsent(t, true, reporting.ModeAsk)
	auto := reportingConsent(t, true, reporting.ModeAuto)

	row, offered := ReportingCountRow(ask, &reporting.State{}, now)
	if !offered || row.Title != ReportingCountTitle {
		t.Fatalf("ReportingCountRow(ask, due) = %+v, %v; want the row and Review…", row, offered)
	}
	if _, offered := ReportingCountRow(ask, &reporting.State{CountWeek: "2026-W45"}, now); offered {
		t.Error("ReportingCountRow() offered Review… for a week that was sent")
	}
	if _, offered := ReportingCountRow(auto, &reporting.State{}, now); offered {
		t.Error("ReportingCountRow() offered Review… in Auto mode")
	}
	if _, offered := ReportingCountRow(reporting.Consent{}, &reporting.State{}, now); offered {
		t.Error("ReportingCountRow() offered Review… with reporting off")
	}
}

// TestReportOffered proves the Report button's rule: allowed AND reportable
// AND not crash AND not sent in this week.
func TestReportOffered(t *testing.T) {
	item := problems.Item{ID: "0123456789abcdef", Kind: problems.KindUnitFailed, KnownKind: true, Week: "2026-W45", Reportable: true}
	crash := item
	crash.Kind = problems.KindCrash
	notReportable := item
	notReportable.Reportable = false
	sent := &reporting.State{}
	sent.Record(reporting.Entry{Kind: problems.KindUnitFailed, ProblemID: item.ID, Week: item.Week, SentAt: time.Now()})

	tests := []struct {
		name    string
		item    problems.Item
		allowed bool
		state   *reporting.State
		want    bool
	}{
		{"allowed", item, true, nil, true},
		{"not allowed", item, false, nil, false},
		{"not reportable", notReportable, true, nil, false},
		{"crash is local only", crash, true, nil, false},
		{"already sent", item, true, sent, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReportOffered(tt.item, tt.allowed, tt.state); got != tt.want {
				t.Errorf("ReportOffered() = %v, want %v", got, tt.want)
			}
		})
	}
}
