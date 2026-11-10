package pageview

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
)

func TestProblemCardText(t *testing.T) {
	unit := problemItem(problems.KindUnitFailed, 3, time.Date(2026, 11, 5, 3, 56, 0, 0, time.UTC),
		validField("unit", "user@.service"), validField("result", "exit-code"))
	unit.FirstSeen = time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	got := ProblemCardText(unit, problemsNow, time.UTC)
	want := ProblemCard{
		Title:     "A system service failed",
		Subtitle:  "user@.service · last seen today at 03:56",
		Count:     "3 times",
		FirstSeen: "First seen yesterday at 08:00",
		Severity:  SeverityWarning,
	}
	if got != want {
		t.Errorf("ProblemCardText = %+v, want %+v", got, want)
	}

	// No subject and no first time.
	bare := problemItem(problems.KindAppProblem, 1, time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC), validField("class", "panic"))
	got = ProblemCardText(bare, problemsNow, time.UTC)
	if got.Subtitle != "Last seen on Oct 1 at 08:00" || got.Count != "once" || got.FirstSeen != "" || got.Severity != SeverityWarning {
		t.Errorf("ProblemCardText(bare) = %+v", got)
	}
}

func TestProblemCount(t *testing.T) {
	for n, want := range map[int]string{0: "once", 1: "once", 2: "2 times", 17: "17 times"} {
		if got := ProblemCount(n); got != want {
			t.Errorf("ProblemCount(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestProblemSubject(t *testing.T) {
	last := problemsNow
	tests := []struct {
		name string
		item problems.Item
		want string
	}{
		{"unit", problemItem(problems.KindUnitFailed, 1, last, validField("unit", "a.service")), "a.service"},
		{"crash signal", problemItem(problems.KindCrash, 1, last, validField("signal", "SIGSEGV")), "SIGSEGV"},
		{"integrity subsystem", problemItem(problems.KindIntegrityError, 1, last, validField("subsystem", "composefs")), "composefs"},
		{"tpm code", problemItem(problems.KindTPMUnlock, 1, last, validField("tpm_rc", "PCR_CHANGED")), "TPM PCR_CHANGED"},
		{"stage version first", problemItem(problems.KindUpdateStageFailed, 1, last,
			validField("target_version", "20261105100000"), validField("error_class", "network")), "version 20261105100000"},
		{"stage class", problemItem(problems.KindUpdateStageFailed, 1, last, validField("error_class", "network")), "network"},
		{"app code", problemItem(problems.KindAppProblem, 1, last, validField("code", "store.migrate_failed")), "store.migrate_failed"},
		{"invalid value", problemItem(problems.KindCrash, 1, last, problems.Field{Name: "signal", Display: "SIGX"}), ""},
		{"unknown kind", problems.Item{Kind: "new_kind", LastSeen: last}, "kind new_kind"},
	}
	for _, tt := range tests {
		if got := ProblemSubject(tt.item); got != tt.want {
			t.Errorf("%s: ProblemSubject = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestProblemSeverity(t *testing.T) {
	tests := []struct {
		name string
		item problems.Item
		want Severity
	}{
		{"integrity", problemItem(problems.KindIntegrityError, 1, problemsNow), SeverityDanger},
		{"unit", problemItem(problems.KindUnitFailed, 1, problemsNow), SeverityWarning},
		{"crash", problemItem(problems.KindCrash, 1, problemsNow), SeverityWarning},
		{"tpm after retry", problemItem(problems.KindTPMUnlock, 1, problemsNow, validField("outcome", "ok_after_retry")), SeverityInfo},
		{"tpm recovery key", problemItem(problems.KindTPMUnlock, 1, problemsNow, validField("outcome", "recovery_key")), SeverityWarning},
		{"app error", problemItem(problems.KindAppProblem, 1, problemsNow, validField("class", "error")), SeverityInfo},
		{"app panic", problemItem(problems.KindAppProblem, 1, problemsNow, validField("class", "panic")), SeverityWarning},
		{"unknown kind", problems.Item{Kind: "new_kind"}, SeverityInfo},
	}
	for _, tt := range tests {
		if got := ProblemSeverity(tt.item); got != tt.want {
			t.Errorf("%s: ProblemSeverity = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestProblemsSubtitle(t *testing.T) {
	ask := reporting.Decide(true, reporting.ModeAsk, problemsNow)
	auto := reporting.Decide(true, reporting.ModeAuto, problemsNow)
	stale := ask
	stale.Schema = "frostyard.problem/0"
	tests := []struct {
		name                        string
		shown                       int
		loaded, dataOn, reportingOn bool
		consent                     reporting.Consent
		want                        string
	}{
		{"loading", 0, false, true, true, reporting.Consent{}, ProblemsLoading},
		{"off", 7, true, true, true, reporting.Consent{}, "7 recorded on this computer, in plain words. Nothing leaves it unless you turn reports on."},
		{"stale is off", 7, true, true, true, stale, "7 recorded on this computer, in plain words. Nothing leaves it unless you turn reports on."},
		{"ask", 1, true, true, true, ask, "1 recorded on this computer, in plain words. Reports are on: you see each one before it is sent."},
		{"auto", 2, true, true, true, auto, "2 recorded on this computer, in plain words. Reports are on: they are sent automatically."},
		{"none", 0, true, true, true, reporting.Consent{}, "No problems recorded on this computer. Nothing leaves it unless you turn reports on."},
		{"no reporting group", 3, true, true, false, auto, "3 recorded on this computer, in plain words. Nothing leaves it."},
		{"no lists", 0, false, false, true, reporting.Consent{}, "The problem lists are turned off on this computer. Nothing leaves it unless you turn reports on."},
	}
	for _, tt := range tests {
		if got := ProblemsSubtitle(tt.shown, tt.loaded, tt.dataOn, tt.reportingOn, tt.consent); got != tt.want {
			t.Errorf("%s: ProblemsSubtitle = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestProblemsSectionNotes(t *testing.T) {
	generated := time.Date(2026, 11, 5, 3, 56, 0, 0, time.UTC)
	if got := ProblemsSystemNotes(problems.SourceOK, generated, 2, 1, problemsNow, time.UTC); !reflect.DeepEqual(got,
		[]string{"Last checked today at 03:56", "1 dismissed problem is hidden"}) {
		t.Errorf("system notes = %q", got)
	}
	// With no shown problem, ProblemsSystemStatus's row says when.
	if got := ProblemsSystemNotes(problems.SourceOK, generated, 0, 3, problemsNow, time.UTC); !reflect.DeepEqual(got,
		[]string{"3 dismissed problems are hidden"}) {
		t.Errorf("system notes with none shown = %q", got)
	}
	if got := ProblemsSystemNotes(problems.SourceMissing, time.Time{}, 0, 0, problemsNow, time.UTC); got != nil {
		t.Errorf("system notes of a missing file = %q", got)
	}
	if got := ProblemsAppsNotes(0); got != nil {
		t.Errorf("apps notes = %q", got)
	}
	if got := ProblemsAppsNotes(2); !reflect.DeepEqual(got, []string{"2 dismissed problems are hidden"}) {
		t.Errorf("apps notes = %q", got)
	}
}

func TestReportingLimitsNameTheBudget(t *testing.T) {
	if got := ReportingLimits(); !strings.HasPrefix(got, "At most 20 reports a day") {
		t.Errorf("ReportingLimits = %q", got)
	}
}

func TestReportingSentSummary(t *testing.T) {
	if got := ReportingSentSummary(nil); got != ReportingSentEmpty {
		t.Errorf("nil = %q", got)
	}
	state := &reporting.State{}
	state.Record(reporting.Entry{ReportID: "a", Kind: problems.KindUnitFailed, ProblemID: "p", Week: "2026-W45", SentAt: problemsNow})
	if got := ReportingSentSummary(state); got != "1 report sent" {
		t.Errorf("one = %q", got)
	}
	state.Record(reporting.Entry{ReportID: "b", Kind: problems.KindUnitFailed, ProblemID: "q", Week: "2026-W45", SentAt: problemsNow})
	if got := ReportingSentSummary(state); got != "2 reports sent" {
		t.Errorf("two = %q", got)
	}
}

func TestReportingRunToast(t *testing.T) {
	tests := []struct {
		name   string
		res    reporting.RunResult
		dryRun bool
		want   string
	}{
		{"preview", reporting.RunResult{Previews: make([]reporting.Report, 2)}, true, "[DRY-RUN] Preview: 2 reports — no changes made"},
		{"sent", reporting.RunResult{Sent: make([]reporting.Entry, 1)}, false, "Sent 1 report"},
		{"skipped", reporting.RunResult{Skipped: "ask mode"}, false, "No report was sent: ask mode"},
		{"nothing", reporting.RunResult{}, false, "No report is pending"},
		{"error is the caller's", reporting.RunResult{Err: errors.New("503")}, false, "No report is pending"},
	}
	for _, tt := range tests {
		if got := ReportingRunToast(tt.res, tt.dryRun); got != tt.want {
			t.Errorf("%s: ReportingRunToast = %q, want %q", tt.name, got, tt.want)
		}
	}
}
