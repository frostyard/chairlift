package reporting

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// sentReport returns the report of 1 reportable item.
func sentReport(t *testing.T) Report {
	t.Helper()
	r, err := ForItem(reportableItem("0123456789abcdef"), "chairlift/0.14.0")
	if err != nil {
		t.Fatalf("ForItem: %v", err)
	}
	return r
}

// TestSendReportSends1Report proves the dialog's send: 1 request, 1 entry in
// the sent log, and the report is not offered again for its week.
func TestSendReportSends1Report(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{}
	in := input(clk, statePath, sender, nil)
	in.Consent = fixedConsent(askConsent())
	r := sentReport(t)
	entry, err := SendReport(context.Background(), in, r)
	if err != nil {
		t.Fatalf("SendReport: %v", err)
	}
	if entry.ReportID != r.ReportID || entry.ProblemID != r.ProblemID || entry.Kind != r.Kind {
		t.Errorf("entry = %+v, want the report's identities", entry)
	}
	if sender.count() != 1 {
		t.Fatalf("%d requests were made, want 1", sender.count())
	}
	st, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !st.WasSent(r.ProblemID, r.Week) {
		t.Error("the sent report is not in the sent log")
	}
}

// TestSendReportRefusals proves every guard of the manual path makes no
// request.
func TestSendReportRefusals(t *testing.T) {
	r := sentReport(t)
	tests := []struct {
		name string
		in   func(clk *clock, statePath string, sender Sender) RunInput
		r    Report
		want error
	}{
		{
			name: "dry run",
			in: func(clk *clock, path string, sender Sender) RunInput {
				in := input(clk, path, sender, nil)
				in.DryRun = true
				return in
			},
			want: ErrDryRun,
		},
		{
			name: "group off",
			in: func(clk *clock, path string, sender Sender) RunInput {
				in := input(clk, path, sender, nil)
				in.GroupOn = false
				return in
			},
			want: ErrOff,
		},
		{
			name: "no consent",
			in: func(clk *clock, path string, sender Sender) RunInput {
				in := input(clk, path, sender, nil)
				in.Consent = fixedConsent(Consent{})
				return in
			},
			want: ErrNoConsent,
		},
		{
			name: "no release client",
			in: func(clk *clock, path string, sender Sender) RunInput {
				in := input(clk, path, sender, nil)
				in.Client = ""
				return in
			},
			want: ErrNoClient,
		},
		{
			name: "crash report",
			in: func(clk *clock, path string, sender Sender) RunInput {
				return input(clk, path, sender, nil)
			},
			r:    Report{Kind: "crash"},
			want: ErrLocalOnly,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newClock(testStart)
			sender := &recordingSender{}
			in := tt.in(clk, newStatePath(t), sender)
			report := r
			if tt.r.Kind != "" {
				report = tt.r
			}
			if _, err := SendReport(context.Background(), in, report); !errors.Is(err, tt.want) {
				t.Fatalf("SendReport() = %v, want %v", err, tt.want)
			}
			if sender.count() != 0 {
				t.Fatalf("%d requests were made, want 0", sender.count())
			}
		})
	}
}

// TestSendReportUnreadableStateSendsNothing proves a broken sent log stops the
// dialog's send too, and that nothing overwrites the file: the caps cannot be
// checked, so no request is made and no byte of the log changes.
func TestSendReportUnreadableStateSendsNothing(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	broken := []byte(`{"format":2,"sent":[`)
	if err := writeFile(statePath, broken, ".reports-*.json"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{}
	in := input(clk, statePath, sender, nil)
	_, err = SendReport(context.Background(), in, sentReport(t))
	if !errors.Is(err, ErrStateUnreadable) {
		t.Fatalf("SendReport() = %v, want ErrStateUnreadable", err)
	}
	if sender.count() != 0 {
		t.Fatalf("%d requests were made, want 0", sender.count())
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("the sent log disappeared: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("the send changed the sent log:\nbefore %q\nafter  %q", before, after)
	}
}

// TestSendReportDailyLimit proves the 20-a-day cap holds for the dialog too.
func TestSendReportDailyLimit(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	st := &State{}
	for i := 0; i < MaxPerDay; i++ {
		st.Record(Entry{Kind: "unit_failed", ProblemID: "aaaaaaaaaaaaaaaa", Week: week, SentAt: testStart.Add(-time.Minute)})
	}
	if err := st.Save(statePath); err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{}
	in := input(clk, statePath, sender, nil)
	if _, err := SendReport(context.Background(), in, sentReport(t)); !errors.Is(err, ErrDailyLimit) {
		t.Fatalf("SendReport() = %v, want ErrDailyLimit", err)
	}
	if sender.count() != 0 {
		t.Fatalf("%d requests were made, want 0", sender.count())
	}
}

// TestSendReportIgnoresAPauseAndRecordsAFailure proves the user's own request
// goes out during a pause, and that a failure pauses the automatic path.
func TestSendReportIgnoresAPauseAndRecordsAFailure(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	st := &State{}
	st.Fail(testStart)
	if err := st.Save(statePath); err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{err: &StatusError{Code: 503}}
	in := input(clk, statePath, sender, nil)
	if _, err := SendReport(context.Background(), in, sentReport(t)); err == nil {
		t.Fatal("SendReport() = nil, want the send failure")
	}
	if sender.count() != 1 {
		t.Fatalf("%d requests were made, want 1", sender.count())
	}
	after, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !after.RetryAfter.Equal(testStart.Add(RetryAfter)) {
		t.Errorf("RetryAfter = %v, want %v", after.RetryAfter, testStart.Add(RetryAfter))
	}
	if len(after.Sent) != 0 {
		t.Errorf("a failed send recorded %d entries, want 0", len(after.Sent))
	}
}

// TestSendReportWithoutASenderSendsNothing proves a missing endpoint stops
// the dialog's send with an error and no request.
func TestSendReportWithoutASenderSendsNothing(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	in := input(clk, statePath, nil, nil)
	if _, err := SendReport(context.Background(), in, sentReport(t)); !errors.Is(err, ErrNoEndpoint) {
		t.Fatalf("SendReport() = %v, want ErrNoEndpoint", err)
	}
	if _, err := os.Stat(statePath); err == nil {
		t.Errorf("the send wrote the sent log %s", statePath)
	}
}

// TestManualCountIsRecordedForItsWeek proves a count sent from the dialog
// stops the automatic path for that week, also after a restart.
func TestManualCountIsRecordedForItsWeek(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{}
	in := input(clk, statePath, sender, nil)
	in.Consent = fixedConsent(askConsent())
	in.InstallInfo = someInstallInfo

	count, err := NewCountReport(validEnvelope(), in.Client, "<1m", "latest", clk.Now())
	if err != nil {
		t.Fatalf("NewCountReport: %v", err)
	}
	if _, err := SendReport(context.Background(), in, count); err != nil {
		t.Fatalf("SendReport: %v", err)
	}
	st, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st.CountWeek != week {
		t.Fatalf("CountWeek = %q after a manual count, want %q", st.CountWeek, week)
	}

	// A reloaded state and an automatic run in the same week send no count.
	auto := input(clk, statePath, sender, nil)
	auto.InstallInfo = someInstallInfo
	res := Run(context.Background(), auto)
	if len(res.Sent) != 0 {
		t.Errorf("the automatic run sent %d reports, want 0", len(res.Sent))
	}
	if got := sender.count(); got != 1 {
		t.Errorf("%d requests were made, want 1 (only the manual count)", got)
	}

	// The dialog refuses a second count of the same week.
	again, err := NewCountReport(validEnvelope(), in.Client, "<1m", "latest", clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SendReport(context.Background(), in, again); !errors.Is(err, ErrAlreadySent) {
		t.Fatalf("a second manual count = %v, want ErrAlreadySent", err)
	}
	if got := sender.count(); got != 1 {
		t.Errorf("%d requests were made after a refused count, want 1", got)
	}
}

// TestManualReportIsSentOnce proves the dialog sends the same report once per
// week, also after a restart.
func TestManualReportIsSentOnce(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{}
	in := input(clk, statePath, sender, nil)
	in.Consent = fixedConsent(askConsent())
	r := sentReport(t)

	if _, err := SendReport(context.Background(), in, r); err != nil {
		t.Fatalf("SendReport: %v", err)
	}
	reloaded := input(clk, statePath, sender, nil)
	reloaded.Consent = fixedConsent(askConsent())
	if _, err := SendReport(context.Background(), reloaded, r); !errors.Is(err, ErrAlreadySent) {
		t.Fatalf("a second send = %v, want ErrAlreadySent", err)
	}
	if got := sender.count(); got != 1 {
		t.Fatalf("%d requests were made, want 1", got)
	}
}

// TestManualSendKeepsTheEntryBeforeTheRequest proves the dialog records and
// saves the entry before the request, and forgets it after a failure.
func TestManualSendKeepsTheEntryBeforeTheRequest(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	var seenAtSend []Entry
	sender := &recordingSender{}
	in := input(clk, statePath, sender, nil)
	in.Consent = fixedConsent(askConsent())
	sender.onSend = func(int) {
		st, err := LoadState(statePath)
		if err != nil {
			t.Errorf("the sent log is not readable during the request: %v", err)
			return
		}
		seenAtSend = st.Sent
	}
	r := sentReport(t)
	if _, err := SendReport(context.Background(), in, r); err != nil {
		t.Fatalf("SendReport: %v", err)
	}
	if len(seenAtSend) != 1 || seenAtSend[0].ReportID != r.ReportID {
		t.Errorf("the log held %+v during the request, want the entry", seenAtSend)
	}

	// A failed send leaves no entry and pauses automatic sending.
	failing := &recordingSender{err: &StatusError{Code: 503}}
	in2 := input(clk, newStatePath(t), failing, nil)
	in2.Consent = fixedConsent(askConsent())
	other := sentReport(t)
	if _, err := SendReport(context.Background(), in2, other); err == nil {
		t.Fatal("SendReport() = nil, want the send failure")
	}
	st, err := LoadState(in2.StatePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Sent) != 0 {
		t.Errorf("the failed send kept %d entries, want 0", len(st.Sent))
	}
	if !st.Paused(clk.Now()) {
		t.Error("the failed send did not pause automatic sending")
	}
}
