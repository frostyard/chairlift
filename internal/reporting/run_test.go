package reporting

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/problems/schemacheck"
)

// TestRunConsentAndGroupStopEverything proves that nothing is sent when the
// reporting group is off, when the consent is off, or when the consent names
// a schema with another data generation.
func TestRunConsentAndGroupStopEverything(t *testing.T) {
	stale := Consent{Format: consentFormat, Enabled: true, Mode: ModeAuto, Schema: "frostyard.problem/0"}
	off := Consent{Format: consentFormat, Enabled: false, Mode: ModeAuto, Schema: CurrentSchema}
	tests := []struct {
		name    string
		groupOn bool
		consent Consent
		skipped string
	}{
		{"group off", false, autoConsent(), "group off"},
		{"consent off", true, off, "consent"},
		{"stale schema", true, stale, "consent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newClock(testStart)
			sender := &recordingSender{}
			in := input(clk, newStatePath(t), sender, reportableItems(3))
			in.GroupOn = tt.groupOn
			in.Consent = fixedConsent(tt.consent)
			res := Run(context.Background(), in)
			if sender.count() != 0 {
				t.Fatalf("%d requests were made, want 0", sender.count())
			}
			if res.Skipped != tt.skipped {
				t.Errorf("Skipped = %q, want %q", res.Skipped, tt.skipped)
			}
		})
	}
}

// TestRunNeverSendsInAskMode proves Run is the automatic path only: in Ask
// mode it sends nothing, even with the count due and 3 reportable items.
func TestRunNeverSendsInAskMode(t *testing.T) {
	clk := newClock(testStart)
	sender := &recordingSender{}
	in := input(clk, newStatePath(t), sender, reportableItems(3))
	in.Consent = fixedConsent(askConsent())
	in.InstallInfo = someInstallInfo
	res := Run(context.Background(), in)
	if sender.count() != 0 {
		t.Fatalf("%d requests were made in ask mode, want 0", sender.count())
	}
	if res.Skipped != "ask mode" {
		t.Errorf("Skipped = %q, want %q", res.Skipped, "ask mode")
	}
}

// TestRunSendsNothingWithoutAReleaseClient proves a dev build never sends.
func TestRunSendsNothingWithoutAReleaseClient(t *testing.T) {
	clk := newClock(testStart)
	sender := &recordingSender{}
	in := input(clk, newStatePath(t), sender, reportableItems(3))
	in.Client = ""
	res := Run(context.Background(), in)
	if sender.count() != 0 {
		t.Fatalf("%d requests were made, want 0", sender.count())
	}
	if res.Skipped != "not a release build" {
		t.Errorf("Skipped = %q", res.Skipped)
	}
}

// TestRunDailyLimit proves the 20-a-day budget: 25 pending items send 20
// reports, a second run the same day sends none, and 24 hours later the
// remaining 5 go.
func TestRunDailyLimit(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{}
	in := input(clk, statePath, sender, reportableItems(25))

	res := Run(context.Background(), in)
	if got := sender.count(); got != MaxPerDay {
		t.Fatalf("%d requests were made, want %d", got, MaxPerDay)
	}
	if len(res.Sent) != MaxPerDay {
		t.Fatalf("Sent holds %d entries, want %d", len(res.Sent), MaxPerDay)
	}
	seen := map[string]bool{}
	for _, e := range res.Sent {
		if seen[e.ProblemID] {
			t.Fatalf("item %s was sent twice", e.ProblemID)
		}
		seen[e.ProblemID] = true
	}

	second := Run(context.Background(), in)
	if got := sender.count(); got != MaxPerDay {
		t.Fatalf("a second run the same day made %d more requests, want 0", got-MaxPerDay)
	}
	if len(second.Sent) != 0 {
		t.Errorf("the second run sent %d reports, want 0", len(second.Sent))
	}

	clk.Advance(24 * time.Hour)
	third := Run(context.Background(), in)
	if got := sender.count(); got != MaxPerDay+5 {
		t.Fatalf("24 h later the total is %d requests, want %d", got, MaxPerDay+5)
	}
	if len(third.Sent) != 5 {
		t.Errorf("24 h later the run sent %d reports, want 5", len(third.Sent))
	}
}

// TestRunNeverSendsAnItemTwice proves a problem is sent at most once in its
// week.
func TestRunNeverSendsAnItemTwice(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{}
	in := input(clk, statePath, sender, []problems.Item{reportableItem("0123456789abcdef")})
	Run(context.Background(), in)
	if got := sender.count(); got != 1 {
		t.Fatalf("%d requests were made, want 1", got)
	}
	clk.Advance(time.Minute)
	Run(context.Background(), in)
	if got := sender.count(); got != 1 {
		t.Fatalf("%d requests were made after a second run, want 1", got)
	}
}

// TestRunWeeklyCount proves 1 count a week: 2 runs in a week send 1 count,
// and the next ISO week sends another.
func TestRunWeeklyCount(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{}
	in := input(clk, statePath, sender, nil)
	in.InstallInfo = someInstallInfo

	Run(context.Background(), in)
	if got := sender.count(); got != 1 {
		t.Fatalf("%d requests were made, want 1 count", got)
	}
	if kinds := sender.kinds(t); len(kinds) != 1 || kinds[0] != problems.KindCount {
		t.Fatalf("sent kinds = %v, want [count]", kinds)
	}
	clk.Advance(time.Hour)
	Run(context.Background(), in)
	if got := sender.count(); got != 1 {
		t.Fatalf("%d requests were made in the same week, want 1 count", got)
	}
	clk.Advance(7 * 24 * time.Hour)
	Run(context.Background(), in)
	if got := sender.count(); got != 2 {
		t.Fatalf("%d requests were made over 2 weeks, want 2 counts", got)
	}
}

// TestRunCountSkipsAnInvalidEnvelope proves a non-snosi host sends no count:
// without image.id and image.version there is nothing the schema accepts.
func TestRunCountSkipsAnInvalidEnvelope(t *testing.T) {
	clk := newClock(testStart)
	sender := &recordingSender{}
	in := input(clk, newStatePath(t), sender, nil)
	in.InstallInfo = someInstallInfo
	in.Envelope = problems.Envelope{}
	Run(context.Background(), in)
	if sender.count() != 0 {
		t.Fatalf("%d requests were made without a valid envelope, want 0", sender.count())
	}
}

// TestRunSkipsNonReportableItems proves a host without a valid envelope sends
// nothing, even in auto mode: its items are not reportable.
func TestRunSkipsNonReportableItems(t *testing.T) {
	clk := newClock(testStart)
	sender := &recordingSender{}
	items := reportableItems(3)
	for i := range items {
		items[i].Reportable = false
	}
	in := input(clk, newStatePath(t), sender, items)
	res := Run(context.Background(), in)
	if sender.count() != 0 {
		t.Fatalf("%d requests were made for non-reportable items, want 0", sender.count())
	}
	if len(res.Sent) != 0 {
		t.Errorf("Sent holds %d entries, want 0", len(res.Sent))
	}
}

// TestRunNeverSendsACrashReport proves kind crash stays local: Run skips it,
// ForItem refuses it, and no crash body ever reaches a server.
func TestRunNeverSendsACrashReport(t *testing.T) {
	clk := newClock(testStart)
	srv := newReportServer(t, http.StatusAccepted)
	sender := &capturingSender{inner: NewHTTPSender(srv.URL)}
	items := append(reportableItems(1), crashItem())
	in := input(clk, newStatePath(t), sender, items)
	in.InstallInfo = someInstallInfo
	res := Run(context.Background(), in)
	if res.Err != nil {
		t.Fatalf("Run: %v", res.Err)
	}
	for _, body := range sender.bodies() {
		if kind, err := KindOf(body); err == nil && kind == problems.KindCrash {
			t.Fatal("a crash report was sent")
		}
	}
	if srv.badKinds.Load() != 0 {
		t.Fatalf("the server received %d crash reports, want 0", srv.badKinds.Load())
	}
	if _, err := ForItem(crashItem(), "chairlift/0.14.0"); !errors.Is(err, ErrLocalOnly) {
		t.Errorf("ForItem(crash) = %v, want ErrLocalOnly", err)
	}
	payload, err := problemsCrashPayload(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewHTTPSender(srv.URL).Send(context.Background(), payload); !errors.Is(err, ErrLocalOnly) {
		t.Errorf("Send(crash) = %v, want ErrLocalOnly", err)
	}
	if srv.badKinds.Load() != 0 {
		t.Fatalf("the server received %d crash reports after a direct send, want 0", srv.badKinds.Load())
	}
	if got := len(sender.bodies()); got != 2 {
		t.Errorf("the sender was asked for %d bodies, want 2 (1 report and 1 count)", got)
	}
}

// TestRunConsentTurnedOffBetweenRequestsStops proves the consent is read
// again before each request, so turning reporting off stops a run at once.
func TestRunConsentTurnedOffBetweenRequestsStops(t *testing.T) {
	clk := newClock(testStart)
	sender := &recordingSender{}
	in := input(clk, newStatePath(t), sender, reportableItems(5))
	// Run reads the consent before each report and again after the pace, so
	// the first report's 3 reads allow and every later read refuses.
	calls := 0
	in.Consent = func() Consent {
		calls++
		if calls > 3 {
			return Consent{}
		}
		return autoConsent()
	}
	res := Run(context.Background(), in)
	if got := sender.count(); got != 1 {
		t.Fatalf("%d requests were made, want 1", got)
	}
	if res.Err != nil {
		t.Fatalf("Run: %v", res.Err)
	}
}

// TestRunFailedSendPauses24Hours proves a 503, a 500 and a transport error all
// pause automatic sending for 24 hours, and that the pause ends.
func TestRunFailedSendPauses24Hours(t *testing.T) {
	failures := map[string]error{
		"503":        &StatusError{Code: http.StatusServiceUnavailable},
		"500":        &StatusError{Code: http.StatusInternalServerError},
		"transport":  errors.New("dial tcp: connection refused"),
		"unexpected": errors.New("something else"),
	}
	for name, failure := range failures {
		t.Run(name, func(t *testing.T) {
			clk := newClock(testStart)
			statePath := newStatePath(t)
			sender := &recordingSender{err: failure}
			in := input(clk, statePath, sender, reportableItems(2))
			res := Run(context.Background(), in)
			if res.Err == nil {
				t.Fatal("Run() = nil, want the send failure")
			}
			st, err := LoadState(statePath)
			if err != nil {
				t.Fatalf("LoadState: %v", err)
			}
			if !st.RetryAfter.Equal(testStart.Add(RetryAfter)) {
				t.Fatalf("RetryAfter = %v, want %v", st.RetryAfter, testStart.Add(RetryAfter))
			}

			clk.Advance(time.Hour)
			paused := Run(context.Background(), in)
			if got := sender.count(); got != 1 {
				t.Fatalf("%d requests were made during the pause, want 1 (only the failed one)", got)
			}
			if !strings.HasPrefix(paused.Skipped, "paused until ") {
				t.Errorf("Skipped = %q, want a pause", paused.Skipped)
			}

			clk.Advance(24 * time.Hour)
			sender.mu.Lock()
			sender.err = nil
			sender.mu.Unlock()
			// The failed report was not recorded, so it is sent again with
			// the other pending report.
			Run(context.Background(), in)
			if got := sender.count(); got != 3 {
				t.Fatalf("%d requests were made after the pause, want 3", got)
			}
		})
	}
}

// TestRunUnreadableStateSendsNothing proves a broken sent log stops the
// automatic path: it cannot enforce the caps or record a send.
func TestRunUnreadableStateSendsNothing(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	if err := writeFile(statePath, []byte("{"), ".reports-*.json"); err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{}
	in := input(clk, statePath, sender, reportableItems(2))
	res := Run(context.Background(), in)
	if sender.count() != 0 {
		t.Fatalf("%d requests were made with an unreadable sent log, want 0", sender.count())
	}
	if res.Skipped != "state unreadable" {
		t.Errorf("Skipped = %q, want %q", res.Skipped, "state unreadable")
	}
}

// TestRunParallelRunsSendNothingTwice proves the lock keeps 2 runs apart: no
// item is sent twice, and the total never exceeds the daily budget.
func TestRunParallelRunsSendNothingTwice(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{}
	results := make([]RunResult, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := input(clk, statePath, sender, reportableItems(25))
			results[i] = Run(context.Background(), in)
		}(i)
	}
	wg.Wait()
	if got := sender.count(); got > MaxPerDay {
		t.Fatalf("%d requests were made, want at most %d", got, MaxPerDay)
	}
	seen := map[string]bool{}
	for _, res := range results {
		for _, e := range res.Sent {
			if seen[e.ProblemID] {
				t.Fatalf("item %s was sent by both runs", e.ProblemID)
			}
			seen[e.ProblemID] = true
		}
	}
	st, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Sent) != sender.count() {
		t.Errorf("the sent log holds %d entries, the sender saw %d requests", len(st.Sent), sender.count())
	}
}

// TestRunBodiesAreExactlyTheSentReports proves the bytes: the server receives
// exactly Report.Body, Text is the body, and every sent body validates
// against the vendored v1 schema.
func TestRunBodiesAreExactlyTheSentReports(t *testing.T) {
	clk := newClock(testStart)
	srv := newReportServer(t, http.StatusAccepted)
	sender := &capturingSender{inner: NewHTTPSender(srv.URL)}
	in := input(clk, newStatePath(t), sender, reportableItems(2))
	in.InstallInfo = someInstallInfo
	if res := Run(context.Background(), in); res.Err != nil {
		t.Fatalf("Run: %v", res.Err)
	}
	bodies := sender.bodies()
	if len(bodies) != 3 {
		t.Fatalf("the sender was asked for %d bodies, want 3", len(bodies))
	}
	received := srv.received()
	if len(received) != len(bodies) {
		t.Fatalf("the server answered %d requests, the sender sent %d bodies", len(received), len(bodies))
	}
	validator := loadValidator(t)
	for i, body := range bodies {
		// The server receives exactly the bytes the sender was given, which
		// are exactly the bytes the dialog shows.
		if received[i] != string(body) {
			t.Errorf("body %d: the server received %q, want the report body", i, received[i])
		}
		if r := (Report{Body: body}); r.Text() != string(body) {
			t.Errorf("body %d: Text() is not the body", i)
		}
		if errs := validateBody(t, validator, body); len(errs) > 0 {
			t.Errorf("body %d is not valid: %v\n%s", i, errs, body)
		}
	}
}

// TestEveryKindBodyValidates proves each kind's report body is valid: the
// schema is the contract of what leaves this computer.
func TestEveryKindBodyValidates(t *testing.T) {
	validator := loadValidator(t)
	for _, kind := range problems.Kinds {
		if kind == problems.KindCount {
			continue
		}
		t.Run(kind, func(t *testing.T) {
			item, ok := kindItem(kind)
			if !ok {
				t.Fatalf("no item for kind %s", kind)
			}
			reportID := "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b"
			body, err := problems.BuildPayload(item, reportID, "chairlift/0.14.0")
			if err != nil {
				t.Fatalf("BuildPayload: %v", err)
			}
			if errs := validateBody(t, validator, body); len(errs) > 0 {
				t.Errorf("%s body is not valid: %v\n%s", kind, errs, body)
			}
			r := Prepare(kind, item.ID, item.Week, reportID, body)
			if r.Text() != string(r.Body) {
				t.Errorf("%s: Text() is not the body", kind)
			}
			if errs := validateBody(t, validator, r.Body); len(errs) > 0 {
				t.Errorf("%s prepared body is not valid: %v", kind, errs)
			}
		})
	}
	t.Run(problems.KindCount, func(t *testing.T) {
		body, err := BuildCount(validEnvelope(), week, "<1m", "latest",
			"3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "chairlift/0.14.0")
		if err != nil {
			t.Fatalf("BuildCount: %v", err)
		}
		r := Prepare(problems.KindCount, "", week, "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", body)
		if errs := validateBody(t, validator, r.Body); len(errs) > 0 {
			t.Errorf("count body is not valid: %v\n%s", errs, r.Body)
		}
	})
}

// TestCountBodyKeepsReadableEnums proves the count body holds the enum text
// itself, not JSON escapes such as \u003c1w: the dialog shows readable JSON,
// and the body still validates against the vendored schema.
func TestCountBodyKeepsReadableEnums(t *testing.T) {
	validator := loadValidator(t)
	for _, age := range []string{"<1w", "<1m", "<6m", ">=6m"} {
		t.Run(age, func(t *testing.T) {
			body, err := BuildCount(validEnvelope(), week, age, "latest",
				"3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "chairlift/0.14.0")
			if err != nil {
				t.Fatalf("BuildCount: %v", err)
			}
			if !strings.Contains(string(body), `"`+age+`"`) {
				t.Errorf("the count body does not hold the literal value %q:\n%s", age, body)
			}
			if strings.Contains(string(body), `\u003c`) || strings.Contains(string(body), `\u003e`) {
				t.Errorf("the count body escapes the enum value:\n%s", body)
			}
			r := Prepare(problems.KindCount, "", week, "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", body)
			if !strings.Contains(r.Text(), `"`+age+`"`) {
				t.Errorf("the dialog text does not hold the literal value %q:\n%s", age, r.Text())
			}
			if errs := validateBody(t, validator, r.Body); len(errs) > 0 {
				t.Errorf("count body of %q is not valid: %v", age, errs)
			}
		})
	}
}

// TestDryRunSendsNothingAndWritesNothing proves a dry run builds every
// report, sends none, and leaves no sent log.
func TestDryRunSendsNothingAndWritesNothing(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{}
	in := input(clk, statePath, sender, reportableItems(3))
	in.InstallInfo = someInstallInfo
	in.DryRun = true
	in.Log = nil
	var lines []string
	in.Log = func(line string) { lines = append(lines, line) }
	res := Run(context.Background(), in)
	if sender.count() != 0 {
		t.Fatalf("%d requests were made in a dry run, want 0", sender.count())
	}
	if len(res.Previews) != 4 {
		t.Fatalf("%d previews, want 4 (3 reports and 1 count)", len(res.Previews))
	}
	if len(res.Sent) != 0 {
		t.Errorf("a dry run recorded %d sends, want 0", len(res.Sent))
	}
	if _, err := LoadState(statePath); err != nil {
		t.Errorf("LoadState after a dry run: %v", err)
	}
	if _, err := os.Stat(statePath); err == nil {
		t.Errorf("a dry run wrote the sent log %s", statePath)
	}
	want := 0
	for _, line := range lines {
		if line == "[DRY-RUN] would send report "+problems.KindUnitFailed {
			want++
		}
	}
	if want != 3 {
		t.Errorf("%d unit_failed preview log lines, want 3 (%v)", want, lines)
	}
	for _, r := range res.Previews {
		if len(r.Body) == 0 {
			t.Errorf("a preview of kind %s has no body", r.Kind)
		}
	}
}

// loadValidator loads the vendored v1 schema.
func loadValidator(t *testing.T) *schemacheck.Validator {
	t.Helper()
	v, err := schemacheck.Load(filepath.Join("..", "problems", "testdata", "schema", "v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// validateBody returns the schema errors of a report body.
func validateBody(t *testing.T, v *schemacheck.Validator, body []byte) []string {
	t.Helper()
	errs, err := v.Validate(schemacheck.Decode(body))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	return errs
}

// TestRunWithdrawDuringThePaceStopsTheNextRequest proves the consent is read
// again after the pace wait: turning reporting off during the wait stops the
// next request, and that is not a failure (no pause).
func TestRunWithdrawDuringThePaceStopsTheNextRequest(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{}
	off := false
	in := input(clk, statePath, sender, reportableItems(3))
	in.Pace = 50 * time.Millisecond
	in.Consent = func() Consent {
		if off {
			return Consent{}
		}
		return autoConsent()
	}
	// Turn reporting off during the wait before the second request.
	sender.onSend = func(n int) {
		if n == 1 {
			off = true
		}
	}
	res := Run(context.Background(), in)
	if got := sender.count(); got != 1 {
		t.Fatalf("%d requests were made, want 1", got)
	}
	if res.Err != nil {
		t.Fatalf("Run() error = %v, want a normal stop", res.Err)
	}
	st, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st.Paused(clk.Now()) {
		t.Error("a withdrawal paused automatic sending, want no pause")
	}
	if len(st.Sent) != 1 {
		t.Errorf("the sent log holds %d entries, want 1", len(st.Sent))
	}
}

// TestRunWithoutASenderSendsNothing proves a missing endpoint stops the run
// before it reads or writes anything.
func TestRunWithoutASenderSendsNothing(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	in := input(clk, statePath, nil, reportableItems(2))
	res := Run(context.Background(), in)
	if res.Skipped != "no endpoint" {
		t.Fatalf("Skipped = %q, want no endpoint", res.Skipped)
	}
	if len(res.Sent) != 0 {
		t.Errorf("Sent holds %d entries, want 0", len(res.Sent))
	}
	if _, err := os.Stat(statePath); err == nil {
		t.Errorf("the run wrote the sent log %s", statePath)
	}
}

// TestRunSaveFailureMakesNoRequest proves the order record, save, then send: a
// sent log that cannot be written stops the run with 0 requests.
func TestRunSaveFailureMakesNoRequest(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a root user may write into a read-only folder")
	}
	clk := newClock(testStart)
	// The sent log exists and reads, but its folder admits no new file, so the
	// save before the request fails.
	statePath := newStatePath(t)
	if err := (&State{}).Save(statePath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	folder := filepath.Dir(statePath)
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o700) })
	sender := &recordingSender{}
	in := input(clk, statePath, sender, reportableItems(2))
	in.Sender = sender
	res := Run(context.Background(), in)
	if sender.count() != 0 {
		t.Fatalf("%d requests were made without a written sent log, want 0", sender.count())
	}
	if res.Err == nil {
		t.Fatal("Run() error = nil, want the save failure")
	}
	if len(res.Sent) != 0 {
		t.Errorf("Sent holds %d entries, want 0", len(res.Sent))
	}
}

// TestRunFailedRequestRemovesTheEntry proves a failed request leaves no entry
// for that report and pauses automatic sending.
func TestRunFailedRequestRemovesTheEntry(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{err: &StatusError{Code: http.StatusServiceUnavailable}}
	in := input(clk, statePath, sender, reportableItems(2))
	in.Sender = sender
	res := Run(context.Background(), in)
	if res.Err == nil {
		t.Fatal("Run() error = nil, want the send failure")
	}
	st, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Sent) != 0 {
		t.Errorf("the sent log holds %d entries after a failed request, want 0", len(st.Sent))
	}
	if !st.Paused(clk.Now()) {
		t.Error("a failed request did not pause automatic sending")
	}
	// The report is still pending, so a run after the pause sends it again.
	clk.Advance(25 * time.Hour)
	sender.mu.Lock()
	sender.err = nil
	sender.mu.Unlock()
	after := Run(context.Background(), in)
	if len(after.Sent) != 2 {
		t.Errorf("after the pause %d reports were sent, want 2", len(after.Sent))
	}
}

// TestRunKeepsTheEntryOfASuccessfulRequest proves the entry is in the file
// after a successful send.
func TestRunKeepsTheEntryOfASuccessfulRequest(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	sender := &recordingSender{}
	in := input(clk, statePath, sender, reportableItems(1))
	in.Sender = sender
	res := Run(context.Background(), in)
	if len(res.Sent) != 1 {
		t.Fatalf("Sent holds %d entries, want 1", len(res.Sent))
	}
	st, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Sent) != 1 || st.Sent[0] != res.Sent[0] {
		t.Errorf("the file holds %+v, want the sent entry", st.Sent)
	}
}
