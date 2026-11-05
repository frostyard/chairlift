package reporting

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/problems"
)

// cliEnv is a fake environment with a config home and a state home in a
// temporary folder.
type cliEnv struct {
	t        *testing.T
	root     string
	home     string
	config   string
	state    string
	snapshot problems.Snapshot
}

func newCLIEnv(t *testing.T) *cliEnv {
	t.Helper()
	root := t.TempDir()
	return &cliEnv{t: t, root: root, home: filepath.Join(root, "home"),
		config: filepath.Join(root, "config"), state: filepath.Join(root, "state")}
}

func (e *cliEnv) getenv(name string) string {
	switch name {
	case "XDG_CONFIG_HOME":
		return e.config
	case "XDG_STATE_HOME":
		return e.state
	case "HOME":
		return e.home
	}
	return ""
}

// writeConsent writes the consent file of the fake environment.
func (e *cliEnv) writeConsent(c Consent) {
	e.t.Helper()
	path, err := ConsentPath(e.getenv)
	if err != nil {
		e.t.Fatalf("ConsentPath: %v", err)
	}
	if err := SaveConsent(path, c); err != nil {
		e.t.Fatalf("SaveConsent: %v", err)
	}
}

// run runs SendPending with the environment and returns its exit code and
// both streams.
func (e *cliEnv) run(in CLIInput) (int, string, string) {
	e.t.Helper()
	var stdout, stderr bytes.Buffer
	in.Getenv = e.getenv
	in.Stdout = &stdout
	in.Stderr = &stderr
	if in.Now == nil {
		clk := newClock(testStart)
		in.Now = clk.Now
	}
	if in.InstallInfo == nil {
		in.InstallInfo = noInstallInfo
	}
	if in.Load == nil {
		snap := e.snapshot
		in.Load = func(context.Context, bool, bool, *problems.Seen) problems.Snapshot { return snap }
	}
	code := SendPending(context.Background(), in)
	return code, stdout.String(), stderr.String()
}

// testConfig returns a configuration with the 3 Problems groups set the way
// the test needs. It is built here rather than loaded, so no test reads the
// host's configuration files.
func testConfig(reportingGroup bool, endpoint string) *config.Config {
	return &config.Config{ProblemsPage: config.PageConfig{
		"problems_system_group":    config.GroupConfig{Enabled: true},
		"problems_apps_group":      config.GroupConfig{Enabled: true},
		"problems_reporting_group": config.GroupConfig{Enabled: reportingGroup, Endpoint: endpoint},
	}}
}

// TestSendPendingSendsNothingWhenOff proves the 4 off cases make no request
// and exit 0: no consent, the reporting group off, ask mode, and a version
// that is not a release.
func TestSendPendingSendsNothingWhenOff(t *testing.T) {
	tests := []struct {
		name    string
		consent *Consent
		version string
		groupOn bool
		message string
	}{
		{name: "no consent", consent: nil, version: "0.15.0", groupOn: true, message: "problem reporting is off"},
		{name: "consent off", consent: &Consent{Format: 1, Enabled: false, Mode: ModeAuto, Schema: CurrentSchema}, version: "0.15.0", groupOn: true, message: "problem reporting is off"},
		{name: "group off", consent: ptr(autoConsent()), version: "0.15.0", groupOn: false, message: "problem reporting is off"},
		{name: "ask mode", consent: ptr(askConsent()), version: "0.15.0", groupOn: true, message: "Ask each time"},
		{name: "dev build", consent: ptr(autoConsent()), version: "dev", groupOn: true, message: "is not a release build"},
		{name: "stale schema", consent: &Consent{Format: 1, Enabled: true, Mode: ModeAuto, Schema: "frostyard.problem/0"}, version: "0.15.0", groupOn: true, message: "problem reporting is off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newCLIEnv(t)
			if tt.consent != nil {
				env.writeConsent(*tt.consent)
			}
			cfg := testConfig(tt.groupOn, "")
			sender := &recordingSender{}
			code, stdout, stderr := env.run(CLIInput{
				Config:    cfg,
				Version:   tt.version,
				NewSender: func(string) Sender { return sender },
			})
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if code != 0 {
				t.Fatalf("SendPending() = %d, want 0 (stderr %q)", code, stderr)
			}
			if sender.count() != 0 {
				t.Fatalf("%d requests were made, want 0", sender.count())
			}
			if !strings.Contains(stderr, tt.message) {
				t.Errorf("stderr = %q, want %q", stderr, tt.message)
			}
			if ff, err := StatePath(env.getenv); err == nil {
				if _, err := os.Stat(ff); err == nil {
					t.Errorf("the mode wrote a sent log at %s", ff)
				}
			}
		})
	}
}

// TestSendPendingSendsInAutoMode proves the mode sends the pending reports in
// auto mode and exits 0.
func TestSendPendingSendsInAutoMode(t *testing.T) {
	env := newCLIEnv(t)
	env.writeConsent(autoConsent())
	env.snapshot = problems.Snapshot{System: problems.SystemFile{Envelope: validEnvelope()}, SystemItems: reportableItems(2)}
	sender := &recordingSender{}
	code, _, stderr := env.run(CLIInput{
		Config:    testConfig(true, ""),
		Version:   "0.15.0",
		NewSender: func(string) Sender { return sender },
	})
	if code != 0 {
		t.Fatalf("SendPending() = %d, want 0 (stderr %q)", code, stderr)
	}
	if sender.count() != 2 {
		t.Fatalf("%d requests were made, want 2", sender.count())
	}
}

// TestSendPendingExitsOnAFailedSend proves a failed send exits 1.
func TestSendPendingExitsOnAFailedSend(t *testing.T) {
	env := newCLIEnv(t)
	env.writeConsent(autoConsent())
	env.snapshot = problems.Snapshot{System: problems.SystemFile{Envelope: validEnvelope()}, SystemItems: reportableItems(1)}
	sender := &recordingSender{err: &StatusError{Code: 503}}
	code, _, stderr := env.run(CLIInput{
		Config:    testConfig(true, ""),
		Version:   "0.15.0",
		NewSender: func(string) Sender { return sender },
	})
	if code != 1 {
		t.Fatalf("SendPending() = %d, want 1 (stderr %q)", code, stderr)
	}
}

// TestSendPendingConfigurationErrorExits1 proves a configuration error stops
// the mode before it reads anything else.
func TestSendPendingConfigurationErrorExits1(t *testing.T) {
	env := newCLIEnv(t)
	env.writeConsent(autoConsent())
	sender := &recordingSender{}
	code, _, stderr := env.run(CLIInput{
		ConfigErr: errFake{},
		Version:   "0.15.0",
		NewSender: func(string) Sender { return sender },
	})
	if code != 1 {
		t.Fatalf("SendPending() = %d, want 1", code)
	}
	if sender.count() != 0 {
		t.Fatalf("%d requests were made, want 0", sender.count())
	}
	if !strings.Contains(stderr, "configuration error") {
		t.Errorf("stderr = %q, want a configuration error line", stderr)
	}
}

// TestSendPendingDryRunPrintsPreviews proves a dry run prints the reports,
// sends nothing and writes no sent log.
func TestSendPendingDryRunPrintsPreviews(t *testing.T) {
	env := newCLIEnv(t)
	env.writeConsent(autoConsent())
	env.snapshot = problems.Snapshot{System: problems.SystemFile{Envelope: validEnvelope()}, SystemItems: reportableItems(1)}
	sender := &recordingSender{}
	code, stdout, stderr := env.run(CLIInput{
		Config:    testConfig(true, ""),
		Version:   "0.15.0",
		DryRun:    true,
		NewSender: func(string) Sender { return sender },
	})
	if code != 0 {
		t.Fatalf("SendPending() = %d, want 0 (stderr %q)", code, stderr)
	}
	if sender.count() != 0 {
		t.Fatalf("%d requests were made in a dry run, want 0", sender.count())
	}
	if !strings.Contains(stdout, `"kind": "unit_failed"`) {
		t.Errorf("stdout does not hold the preview:\n%s", stdout)
	}
	if !strings.Contains(stderr, "[DRY-RUN] would send report unit_failed") {
		t.Errorf("stderr does not hold the dry-run preview line:\n%s", stderr)
	}
	statePath, err := StatePath(env.getenv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); err == nil {
		t.Errorf("a dry run wrote the sent log %s", statePath)
	}
}

// TestSendPendingUsesTheConfiguredEndpoint proves the sender gets the
// endpoint of an administrator-owned configuration file, and the default
// otherwise.
func TestSendPendingUsesTheConfiguredEndpoint(t *testing.T) {
	env := newCLIEnv(t)
	env.writeConsent(autoConsent())
	env.snapshot = problems.Snapshot{System: problems.SystemFile{Envelope: validEnvelope()}, SystemItems: reportableItems(1)}
	var asked []string
	cfg := testConfig(true, "https://admin.example/v1/report")
	newSender := func(endpoint string) Sender {
		asked = append(asked, endpoint)
		return &recordingSender{}
	}
	env.run(CLIInput{Config: cfg, Version: "0.15.0", NewSender: newSender})
	want := []string{DefaultEndpoint}
	if len(asked) != 1 || asked[0] != want[0] {
		t.Fatalf("endpoints = %v, want %v: a user-writable source must not set the endpoint", asked, want)
	}
}

// TestSendPendingSkipsDismissedProblems proves the windowless mode sends no
// problem that the user dismissed: it loads the same dismiss store the page
// uses, so the 2 paths send the same set.
func TestSendPendingSkipsDismissedProblems(t *testing.T) {
	env := newCLIEnv(t)
	env.writeConsent(autoConsent())
	items := reportableItems(2)
	env.snapshot = problems.Snapshot{
		System:      problems.SystemFile{Envelope: validEnvelope()},
		SystemItems: items,
	}
	// The user dismissed the first item after it happened.
	seenPath, err := problems.SeenPath(env.getenv)
	if err != nil {
		t.Fatalf("SeenPath: %v", err)
	}
	seen := problems.NewSeen(seenPath)
	if err := seen.Dismiss(items[0].ID, testStart.Add(time.Hour)); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	sender := &recordingSender{}
	code, _, stderr := env.run(CLIInput{
		Config:    testConfig(true, ""),
		Version:   "0.15.0",
		NewSender: func(string) Sender { return sender },
		Load: func(_ context.Context, _, _ bool, got *problems.Seen) problems.Snapshot {
			if got == nil {
				t.Fatal("SendPending passed no dismiss store to problems.Load")
			}
			snap := env.snapshot
			snap.SystemItems, _ = problems.FilterSeen(snap.SystemItems, got)
			return snap
		},
	})
	if code != 0 {
		t.Fatalf("SendPending() = %d, want 0 (stderr %q)", code, stderr)
	}
	if sender.count() != 1 {
		t.Fatalf("%d requests were made, want 1: a dismissed problem must not be sent", sender.count())
	}
	sent := string(sender.bodies[0])
	if strings.Contains(sent, items[0].ID) {
		t.Errorf("the dismissed problem %s was sent:\n%s", items[0].ID, sent)
	}
}

// TestSendPendingUnreadableDismissStoreSendsNothing proves a broken dismiss
// store stops the mode with a line that says why.
func TestSendPendingUnreadableDismissStoreSendsNothing(t *testing.T) {
	env := newCLIEnv(t)
	env.writeConsent(autoConsent())
	env.snapshot = problems.Snapshot{
		System:      problems.SystemFile{Envelope: validEnvelope()},
		SystemItems: reportableItems(1),
	}
	seenPath, err := problems.SeenPath(env.getenv)
	if err != nil {
		t.Fatalf("SeenPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(seenPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seenPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{}
	code, _, stderr := env.run(CLIInput{
		Config:    testConfig(true, ""),
		Version:   "0.15.0",
		NewSender: func(string) Sender { return sender },
	})
	if code != 0 {
		t.Fatalf("SendPending() = %d, want 0", code)
	}
	if sender.count() != 0 {
		t.Fatalf("%d requests were made with an unreadable dismiss store, want 0", sender.count())
	}
	if !strings.Contains(stderr, "the dismiss store could not be read") {
		t.Errorf("stderr = %q, want the dismiss store line", stderr)
	}
}

// errFake is a configuration error.
type errFake struct{}

func (errFake) Error() string { return "fake configuration error" }

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }
