package reporting

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

// testStart is the fixed clock of these tests: a Wednesday in ISO week
// 2026-W45.
var testStart = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

// clock is a fixed clock that a test moves.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock(t time.Time) *clock {
	return &clock{now: t}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// recordingSender counts Send calls and keeps each body. err, when set, is
// returned for every send; onSend runs after the body is kept.
type recordingSender struct {
	mu     sync.Mutex
	bodies [][]byte
	errs   []error
	err    error
	onSend func(n int)
}

func (s *recordingSender) Send(_ context.Context, body []byte) error {
	s.mu.Lock()
	s.bodies = append(s.bodies, append([]byte(nil), body...))
	n := len(s.bodies)
	hook := s.onSend
	err := s.err
	if len(s.errs) >= n {
		err = s.errs[n-1]
	}
	s.mu.Unlock()
	if hook != nil {
		hook(n)
	}
	return err
}

func (s *recordingSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func (s *recordingSender) kinds(t *testing.T) []string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.bodies))
	for i, body := range s.bodies {
		kind, err := KindOf(body)
		if err != nil {
			t.Fatalf("body %d: %v", i, err)
		}
		out[i] = kind
	}
	return out
}

// capturingSender records the body it is asked to send, then delegates.
type capturingSender struct {
	inner Sender
	mu    sync.Mutex
	sent  [][]byte
}

func (s *capturingSender) Send(ctx context.Context, body []byte) error {
	s.mu.Lock()
	s.sent = append(s.sent, append([]byte(nil), body...))
	s.mu.Unlock()
	return s.inner.Send(ctx, body)
}

func (s *capturingSender) bodies() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.sent))
	copy(out, s.sent)
	return out
}

// validEnvelope is a valid snosi image envelope.
func validEnvelope() problems.Envelope {
	return problems.Envelope{ImageID: "snow", ImageVersion: "20261105100000", Secure: true, Valid: true}
}

// week is ISO week 2026-W45, the week of testStart.
const week = "2026-W45"

// reportableItem returns a fully valid unit_failed item with the given ID.
func reportableItem(id string) problems.Item {
	return problems.Item{
		ID:        id,
		Kind:      problems.KindUnitFailed,
		KnownKind: true,
		Fields: []problems.Field{
			{Name: "unit", Display: "nginx.service", Value: "nginx.service", Valid: true},
			{Name: "result", Display: "exit-code", Value: "exit-code", Valid: true},
			{Name: "shipped", Display: "true", Value: true, Valid: true},
		},
		Week:           week,
		WeekValid:      true,
		FirstSeen:      testStart.Add(-time.Hour),
		LastSeen:       testStart.Add(-time.Minute),
		Count:          1,
		FileReportable: true,
		Reportable:     true,
		Envelope:       validEnvelope(),
	}
}

// reportableItems returns n reportable items with distinct IDs.
func reportableItems(n int) []problems.Item {
	items := make([]problems.Item, n)
	for i := range items {
		items[i] = reportableItem(fmt.Sprintf("%016x", i+1))
	}
	return items
}

// crashItem returns a reportable item of kind crash.
func crashItem() problems.Item {
	return problems.Item{
		ID:        "00000000000000ff",
		Kind:      problems.KindCrash,
		KnownKind: true,
		Fields: []problems.Field{
			{Name: "exe", Display: "gnome-shell", Value: "gnome-shell", Valid: true},
			{Name: "signal", Display: "SIGSEGV", Value: "SIGSEGV", Valid: true},
		},
		Week:           week,
		WeekValid:      true,
		FirstSeen:      testStart.Add(-time.Hour),
		LastSeen:       testStart.Add(-time.Minute),
		Count:          1,
		FileReportable: true,
		Reportable:     true,
		Envelope:       validEnvelope(),
	}
}

// kindItem returns a fully valid reportable item of kind, for the schema
// check of every kind's sent body.
func kindItem(kind string) (problems.Item, bool) {
	it := problems.Item{
		ID:             "00000000000000aa",
		Kind:           kind,
		KnownKind:      true,
		Week:           week,
		WeekValid:      true,
		FirstSeen:      testStart.Add(-time.Hour),
		LastSeen:       testStart.Add(-time.Minute),
		Count:          1,
		FileReportable: true,
		Reportable:     true,
		Envelope:       validEnvelope(),
	}
	field := func(name string, value any) problems.Field {
		return problems.Field{Name: name, Display: fmt.Sprint(value), Value: value, Valid: true}
	}
	switch kind {
	case problems.KindUnitFailed:
		it.Fields = []problems.Field{field("unit", "nginx.service"), field("result", "exit-code"), field("shipped", true)}
	case problems.KindUpdateStageFailed:
		it.Fields = []problems.Field{field("target_version", "20261105100000"), field("error_class", "network"), field("bootc_version", "1.2.3")}
	case problems.KindBootedOlderImage:
		it.Fields = []problems.Field{field("newer_version", "20261105100000"), field("how", "manual_rollback")}
	case problems.KindSysextUpdateFailed:
		it.Fields = []problems.Field{field("feature", "docker"), field("version", "29.0.0"), field("outcome", "failed")}
	case problems.KindTPMUnlock:
		it.Fields = []problems.Field{field("outcome", "ok_after_retry"), field("tpm_rc", "PCR_CHANGED"), field("retries", 2)}
	case problems.KindIntegrityError:
		it.Fields = []problems.Field{field("subsystem", "composefs"), field("count", "2-10")}
	case problems.KindCrash:
		it.Fields = []problems.Field{field("exe", "gnome-shell"), field("signal", "SIGSEGV")}
	case problems.KindAppProblem:
		it.Fields = []problems.Field{
			field("app_id", "org.frostyard.ChairLift"),
			field("app_version", "0.14.0"),
			field("runtime", "go"),
			field("component", "gui"),
			field("class", "error"),
			field("code", "config.load_failed"),
		}
	default:
		return problems.Item{}, false
	}
	return it, true
}

// autoConsent is a consent that sends automatically for the current schema.
func autoConsent() Consent {
	return Decide(true, ModeAuto, testStart)
}

// askConsent is a consent that sends only from the dialog.
func askConsent() Consent {
	return Decide(true, ModeAsk, testStart)
}

// fixedConsent returns a Consent func that always returns c.
func fixedConsent(c Consent) func() Consent {
	return func() Consent { return c }
}

// newStatePath returns a fresh sent log path in a temporary folder.
func newStatePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "chairlift", "reports.json")
}

// input is a RunInput for the automatic path with the test defaults.
func input(clk *clock, statePath string, sender Sender, items []problems.Item) RunInput {
	return RunInput{
		Now:         clk.Now,
		GroupOn:     true,
		Consent:     fixedConsent(autoConsent()),
		StatePath:   statePath,
		Items:       items,
		Envelope:    validEnvelope(),
		Client:      "chairlift/0.14.0",
		InstallInfo: noInstallInfo,
		Sender:      sender,
		Log:         func(string) {},
	}
}

// noInstallInfo gives no install age, so a run sends no weekly count.
func noInstallInfo(context.Context) (string, string, bool) {
	return "", "", false
}

// someInstallInfo gives an install age and a channel, so a run may send the
// weekly count.
func someInstallInfo(context.Context) (string, string, bool) {
	return "<1m", "latest", true
}
