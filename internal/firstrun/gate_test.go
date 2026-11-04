package firstrun

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/i18n"
)

func TestDecideCompletion(t *testing.T) {
	p := Paths{Marker: "/c/chairlift/first-run.done", LegacyMarker: "/c/snow-first-setup.done"}
	tests := []struct {
		dryRun bool
		want   Completion
	}{
		{true, Completion{
			WriteMarkers:    false,
			Log:             "[DRY-RUN] would write first-run markers: /c/chairlift/first-run.done, /c/snow-first-setup.done",
			DoneDescription: i18n.MsgDryRunFinished,
		}},
		{false, Completion{
			WriteMarkers:    true,
			Log:             "first-run: markers written: /c/chairlift/first-run.done, /c/snow-first-setup.done",
			DoneDescription: i18n.MsgFinishedDesc,
		}},
	}
	for _, tt := range tests {
		if got := DecideCompletion(tt.dryRun, p); got != tt.want {
			t.Errorf("DecideCompletion(%v) = %+v, want %+v", tt.dryRun, got, tt.want)
		}
	}
	if !strings.HasPrefix(i18n.MsgDryRunFinished, "[DRY-RUN] Preview:") || !strings.HasSuffix(i18n.MsgDryRunFinished, "— no changes made") {
		t.Errorf("dry-run Done text %q breaks ADR-0009's preview wording", i18n.MsgDryRunFinished)
	}
}

func TestCompleteDryRunWritesNothing(t *testing.T) {
	p := testPaths(t)
	touch(t, p.LegacyAutostart)
	decision, err := Complete(true, p)
	if err != nil || decision.WriteMarkers {
		t.Fatalf("Complete(dry run) = %+v, %v; want no write and no error", decision, err)
	}
	if exists(t, p.Marker) || exists(t, p.LegacyMarker) {
		t.Fatal("dry run wrote a marker")
	}
	if !exists(t, p.LegacyAutostart) {
		t.Fatal("dry run removed the legacy autostart copy")
	}
}

func TestCompleteWritesMarkers(t *testing.T) {
	p := testPaths(t)
	decision, err := Complete(false, p)
	if err != nil || !decision.WriteMarkers {
		t.Fatalf("Complete() = %+v, %v; want a write and no error", decision, err)
	}
	if !exists(t, p.Marker) || !exists(t, p.LegacyMarker) {
		t.Fatal("Complete() did not write both markers")
	}
}

// gateEnvFor is a regular user's gate environment for the config home.
func gateEnvFor(configHome string) GateEnv {
	return GateEnv{UID: 1000, Getenv: envOf(map[string]string{"XDG_CONFIG_HOME": configHome}), Home: "/nonexistent-home"}
}

// assertGateExecs checks that the gate starts the first run again.
func assertGateExecs(t *testing.T, env GateEnv) {
	t.Helper()
	got := Gate(env)
	if got.ExecPath != ChairLiftPath || len(got.ExecArgv) < 2 || got.ExecArgv[1] != "--first-run" {
		t.Errorf("Gate() = %+v, want an exec of %s --first-run (the next login runs the first run again)", got, ChairLiftPath)
	}
}

// TestCompleteReportsMarkDoneFailure is a total failure: the config home is
// a file, so no marker can be written, the Done step says the first run
// starts again, and the gate does start it again.
func TestCompleteReportsMarkDoneFailure(t *testing.T) {
	p := testPaths(t)
	touch(t, p.ConfigHome) // a file where a directory must be
	decision, err := Complete(false, p)
	if err == nil {
		t.Fatal("Complete() error = nil, want MarkDone's error")
	}
	if !strings.HasPrefix(decision.Log, "first-run: could not record completion: ") {
		t.Fatalf("Complete() log = %q", decision.Log)
	}
	// The failure reaches the presentation: no "ready to use" claim, the
	// warning icon, and an error-log entry with the error text.
	assertCompletionFailure(t, decision, err)
	if CompleteAt(false, p, nil).Err == nil {
		t.Error("CompleteAt() lost MarkDone's error")
	}
	if got := GateStateAt(p, nil); got != GateRunsAgain {
		t.Fatalf("GateStateAt() = %v after a total failure, want GateRunsAgain", got)
	}
	if done := DoneState(nil, decision, GateStateAt(p, nil)); done.Description != i18n.MsgCompletionFailedDesc {
		t.Errorf("DoneState().Description = %q, want %q", done.Description, i18n.MsgCompletionFailedDesc)
	}
	assertGateExecs(t, gateEnvFor(p.ConfigHome))
}

// TestCompletePartialFailureDoesNotPromiseAnotherRun: the legacy autostart
// path is a non-empty directory, so its remove fails after both markers were
// written. The gate then skips the next login, so the Done step must not say
// that the first run starts again.
func TestCompletePartialFailureDoesNotPromiseAnotherRun(t *testing.T) {
	p := testPaths(t)
	touch(t, filepath.Join(p.LegacyAutostart, "keep"))

	decision := CompleteAt(false, p, nil)
	if decision.Err == nil || !decision.ShowWarning {
		t.Fatalf("CompleteAt() = %+v, want the remove error", decision)
	}
	if !IsDone(p) || GateStateAt(p, nil) != GateDone {
		t.Fatal("IsDone() = false; the markers were written before the remove failed")
	}
	got := Gate(gateEnvFor(p.ConfigHome))
	if got.Code != 0 || got.ExecPath != "" || !strings.Contains(got.Stderr, "first-run: already done") {
		t.Fatalf("Gate() = %+v, want exit 0 \"already done\"", got)
	}
	done := DoneState(nil, decision, GateStateAt(p, nil))
	if done.Description == i18n.MsgCompletionFailedDesc || done.Description != i18n.MsgFinishedWithErrors {
		t.Errorf("DoneState().Description = %q, want %q, not the \"starts again\" text", done.Description, i18n.MsgFinishedWithErrors)
	}
	if !done.Warning || len(done.Entries) != 1 || !strings.Contains(done.Entries[0].Message, p.LegacyAutostart) {
		t.Errorf("DoneState() = %+v, want the warning and the remove error in the log", done)
	}
}

// TestCompleteAtReportsMissingConfigHome covers the window's path error: a
// live run with no config home records nothing and must say so.
func TestCompleteAtReportsMissingConfigHome(t *testing.T) {
	pathsErr := errors.New("neither XDG_CONFIG_HOME nor HOME is set")
	decision := CompleteAt(false, Paths{}, pathsErr)
	if decision.Err == nil || !errors.Is(decision.Err, pathsErr) {
		t.Fatalf("CompleteAt() Err = %v, want it to wrap %v", decision.Err, pathsErr)
	}
	assertCompletionFailure(t, decision, pathsErr)
	if got := GateStateAt(Paths{}, pathsErr); got != GateUnknown {
		t.Fatalf("GateStateAt() = %v with unknown paths, want GateUnknown", got)
	}
	// The gate in this environment cannot find a config home either: it
	// exits 1 and starts nothing, so the Done step must not promise another
	// run.
	if broken := Gate(GateEnv{UID: 1000, Getenv: envOf(nil)}); broken.Code != 1 || broken.ExecPath != "" {
		t.Errorf("Gate(no config home) = %+v, want exit 1 and no exec", broken)
	}
	done := DoneState(nil, decision, GateStateAt(Paths{}, pathsErr))
	if done.Description == i18n.MsgCompletionFailedDesc || done.Description != i18n.MsgFinishedWithErrors {
		t.Errorf("DoneState().Description = %q, want %q, not the \"starts again\" text", done.Description, i18n.MsgFinishedWithErrors)
	}
	if !done.Warning || len(done.Entries) != 1 {
		t.Errorf("DoneState() = %+v, want the warning and the error entry", done)
	}

	// In dry run nothing would be written, so a missing config home is no
	// failure.
	if dry := CompleteAt(true, Paths{}, pathsErr); dry.Err != nil || dry.ShowWarning || dry.DoneDescription != i18n.MsgDryRunFinished {
		t.Fatalf("CompleteAt(dry run) = %+v, want the dry-run preview", dry)
	}
	// Success has no warning and no error entry.
	ok := CompleteAt(false, testPaths(t), nil)
	if ok.Err != nil || ok.ShowWarning || ok.DoneDescription != i18n.MsgFinishedDesc || ok.ErrorLog() != nil {
		t.Fatalf("CompleteAt(success) = %+v", ok)
	}
}

func assertCompletionFailure(t *testing.T, decision Completion, cause error) {
	t.Helper()
	if !decision.ShowWarning || decision.Err == nil {
		t.Errorf("failure outcome = %+v, want Err and ShowWarning", decision)
	}
	if decision.DoneDescription != i18n.MsgCompletionFailedDesc {
		t.Errorf("DoneDescription = %q, want %q (not the success text)", decision.DoneDescription, i18n.MsgCompletionFailedDesc)
	}
	entries := decision.ErrorLog()
	if len(entries) != 1 || entries[0].Title != "Setup failed: Could not record that the first run is finished" ||
		!strings.Contains(entries[0].Message, cause.Error()) {
		t.Errorf("ErrorLog() = %+v, want one translated entry with %q", entries, cause)
	}
	done := DoneState(nil, decision, GateRunsAgain)
	if !done.Warning || len(done.Entries) != 1 || done.Description != i18n.MsgCompletionFailedDesc {
		t.Errorf("DoneState(GateRunsAgain) = %+v, want the warning, the entry and the retry text", done)
	}
}

// TestGateStateAt covers the 3 gate states and that the zero value never
// promises another run.
func TestGateStateAt(t *testing.T) {
	if GateState(0) != GateUnknown {
		t.Fatal("the zero GateState is not GateUnknown")
	}
	p := testPaths(t)
	if got := GateStateAt(p, nil); got != GateRunsAgain {
		t.Errorf("GateStateAt(no marker) = %v, want GateRunsAgain", got)
	}
	if got := GateStateAt(p, errors.New("no home")); got != GateUnknown {
		t.Errorf("GateStateAt(paths error) = %v, want GateUnknown", got)
	}
	touch(t, p.LegacyMarker)
	if got := GateStateAt(p, nil); got != GateDone {
		t.Errorf("GateStateAt(legacy marker) = %v, want GateDone", got)
	}
}

func TestDoneState(t *testing.T) {
	action := ErrorEntry{Title: "Setup failed: Installing Firefox", Message: "no network"}
	failed := Completion{DoneDescription: i18n.MsgCompletionFailedDesc, Err: errors.New("disk full"), ShowWarning: true}
	finished := Completion{DoneDescription: i18n.MsgFinishedDesc}
	dry := Completion{DoneDescription: i18n.MsgDryRunFinished}
	tests := []struct {
		name            string
		actions         []ErrorEntry
		completion      Completion
		gate            GateState
		wantWarning     bool
		wantEntries     int
		wantDescription string
	}{
		{"all good", nil, finished, GateDone, false, 0, i18n.MsgFinishedDesc},
		{"dry run", nil, dry, GateRunsAgain, false, 0, i18n.MsgDryRunFinished},
		{"dry run, unknown paths", nil, dry, GateUnknown, false, 0, i18n.MsgDryRunFinished},
		{"an action failed", []ErrorEntry{action}, finished, GateDone, true, 1, i18n.MsgFinishedDesc},
		{"completion failed, no marker", nil, failed, GateRunsAgain, true, 1, i18n.MsgCompletionFailedDesc},
		{"completion failed, a marker exists", nil, failed, GateDone, true, 1, i18n.MsgFinishedWithErrors},
		{"completion failed, gate state unknown", nil, failed, GateUnknown, true, 1, i18n.MsgFinishedWithErrors},
		{"both failed: action entry first", []ErrorEntry{action}, failed, GateRunsAgain, true, 2, i18n.MsgCompletionFailedDesc},
	}
	for _, tt := range tests {
		got := DoneState(tt.actions, tt.completion, tt.gate)
		if got.Warning != tt.wantWarning || len(got.Entries) != tt.wantEntries || got.Description != tt.wantDescription {
			t.Errorf("%s: DoneState() = %+v, want warning %v, %d entries, %q", tt.name, got, tt.wantWarning, tt.wantEntries, tt.wantDescription)
		}
		if tt.wantEntries > 0 && len(tt.actions) > 0 && got.Entries[0] != action {
			t.Errorf("%s: first entry = %+v, want the action entry", tt.name, got.Entries[0])
		}
	}
}

// TestRunProgressDrainsSchemeWriterBeforeCompletion: the first color-scheme
// write is still running and a newer choice is pending when the actions
// end. The completion step must run only after both writes, and the last
// write must be the newer choice.
func TestRunProgressDrainsSchemeWriterBeforeCompletion(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	var mu sync.Mutex
	var writes []ColorScheme
	writer := NewSchemeWriter(func(scheme ColorScheme) error {
		started <- struct{}{}
		mu.Lock()
		first := len(writes) == 0
		mu.Unlock()
		if first {
			<-release
		}
		mu.Lock()
		writes = append(writes, scheme)
		mu.Unlock()
		return nil
	})
	writer.Request(SchemeDark)
	<-started
	writer.Request(SchemeLight) // pending while the Dark write blocks

	var order []string
	var seenAtComplete []ColorScheme
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(release)
	}()
	_, done := RunProgress(PlanActions([]App{firefox}), ProgressSteps{
		Run:   func(Action) error { order = append(order, "action"); return nil },
		Emit:  func(Event) {},
		Drain: writer.Wait,
		Complete: func() Completion {
			order = append(order, "complete")
			mu.Lock()
			seenAtComplete = append([]ColorScheme(nil), writes...)
			mu.Unlock()
			return Completion{DoneDescription: i18n.MsgFinishedDesc}
		},
		Gate: func() GateState { order = append(order, "gate"); return GateDone },
	})
	if want := []string{"action", "action", "complete", "gate"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	if want := []ColorScheme{SchemeDark, SchemeLight}; !reflect.DeepEqual(seenAtComplete, want) {
		t.Fatalf("writes when completion ran = %v, want %v (the writer was not drained first)", seenAtComplete, want)
	}
	if done.Description != i18n.MsgFinishedDesc || done.Warning {
		t.Fatalf("RunProgress() Done = %+v", done)
	}
}

func TestRunProgressReportsFailedActions(t *testing.T) {
	_, done := RunProgress(PlanActions([]App{firefox}), ProgressSteps{
		Run: func(a Action) error {
			if a.Kind == ActionInstall {
				return errors.New("no space")
			}
			return nil
		},
		Emit:     func(Event) {},
		Complete: func() Completion { return Completion{DoneDescription: i18n.MsgFinishedDesc} },
		Gate:     func() GateState { return GateDone },
	})
	if !done.Warning || len(done.Entries) != 1 || done.Entries[0].Message != "no space" {
		t.Fatalf("RunProgress() Done = %+v, want the install failure in the log", done)
	}
}

func TestSetDryRun(t *testing.T) {
	t.Cleanup(func() { SetDryRun(false) })
	SetDryRun(true)
	if !IsDryRun() {
		t.Fatal("IsDryRun() = false after SetDryRun(true)")
	}
	SetDryRun(false)
	if IsDryRun() {
		t.Fatal("IsDryRun() = true after SetDryRun(false)")
	}
}

func TestParseGateArgs(t *testing.T) {
	tests := []struct {
		args    []string
		want    gateOptions
		wantErr bool
	}{
		{nil, gateOptions{}, false},
		{[]string{"--help"}, gateOptions{help: true}, false},
		{[]string{"-h"}, gateOptions{help: true}, false},
		{[]string{"--dry-run"}, gateOptions{dryRun: true}, false},
		{[]string{"--mark-done", "--dry-run", "--mark-done"}, gateOptions{dryRun: true, markDone: true}, false},
		{[]string{"--first-run"}, gateOptions{}, true},
		{[]string{"-d"}, gateOptions{}, true},
		{[]string{"--mark-done", "extra"}, gateOptions{}, true},
	}
	for _, tt := range tests {
		got, err := parseGateArgs(tt.args)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("parseGateArgs(%v) = %+v, %v; want %+v, wantErr %v", tt.args, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestGate(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		uid         int
		setup       func(t *testing.T, p Paths)
		wantCode    int
		wantExec    []string
		wantStdout  string
		wantStderr  string
		wantMarkers bool
	}{
		{name: "help", args: []string{"--help"}, uid: 1000, wantStdout: "--mark-done"},
		{name: "short help", args: []string{"-h"}, uid: 1000, wantStdout: "Usage: chairlift-first-run"},
		{name: "help as root", args: []string{"--help"}, uid: 0, wantStdout: "Usage: chairlift-first-run"},
		{name: "unknown argument", args: []string{"--bogus"}, uid: 1000, wantCode: 2, wantStderr: "unknown argument \"--bogus\"\nUsage: chairlift-first-run"},
		{name: "root", uid: 0, wantStderr: "first-run: skipped for root"},
		{name: "root mark-done writes nothing", args: []string{"--mark-done"}, uid: 0, wantStderr: "first-run: skipped for root"},
		{name: "not done execs chairlift", uid: 1000, wantExec: []string{"chairlift", "--first-run"}, wantStderr: "first-run: starting /usr/bin/chairlift --first-run"},
		{name: "dry run passes", args: []string{"--dry-run"}, uid: 1000, wantExec: []string{"chairlift", "--first-run", "--dry-run"}},
		{name: "chairlift marker", uid: 1000, setup: func(t *testing.T, p Paths) { touch(t, p.Marker) }, wantStderr: "first-run: already done"},
		{name: "legacy marker", uid: 1000, setup: func(t *testing.T, p Paths) { touch(t, p.LegacyMarker) }, wantStderr: "first-run: already done"},
		{name: "mark-done", args: []string{"--mark-done"}, uid: 1000, wantStderr: "first-run: markers written: ", wantMarkers: true},
		{name: "mark-done dry run", args: []string{"--mark-done", "--dry-run"}, uid: 1000, wantStderr: "[DRY-RUN] would write first-run markers: "},
		{name: "mark-done failure", args: []string{"--mark-done"}, uid: 1000, setup: func(t *testing.T, p Paths) { touch(t, p.ConfigHome) }, wantCode: 1, wantStderr: "first-run: could not record completion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "cfg")
			p, err := ResolvePaths(envOf(map[string]string{"XDG_CONFIG_HOME": cfg}), "")
			if err != nil {
				t.Fatal(err)
			}
			if tt.setup != nil {
				tt.setup(t, p)
			}
			got := Gate(GateEnv{Args: tt.args, UID: tt.uid, Getenv: envOf(map[string]string{"XDG_CONFIG_HOME": cfg}), Home: "/nonexistent-home"})
			if got.Code != tt.wantCode {
				t.Errorf("Code = %d, want %d (stderr %q)", got.Code, tt.wantCode, got.Stderr)
			}
			if tt.wantExec != nil {
				if got.ExecPath != "/usr/bin/chairlift" || !reflect.DeepEqual(got.ExecArgv, tt.wantExec) {
					t.Errorf("exec = %q %v, want /usr/bin/chairlift %v", got.ExecPath, got.ExecArgv, tt.wantExec)
				}
			} else if got.ExecPath != "" || got.ExecArgv != nil {
				t.Errorf("exec = %q %v, want none", got.ExecPath, got.ExecArgv)
			}
			if !strings.Contains(got.Stdout, tt.wantStdout) || (tt.wantStdout == "" && got.Stdout != "") {
				t.Errorf("Stdout = %q, want %q", got.Stdout, tt.wantStdout)
			}
			if !strings.Contains(got.Stderr, tt.wantStderr) {
				t.Errorf("Stderr = %q, want %q", got.Stderr, tt.wantStderr)
			}
			if tt.setup == nil {
				// Only --mark-done without --dry-run, as a regular user, writes.
				if gotNew, gotLegacy := exists(t, p.Marker), exists(t, p.LegacyMarker); gotNew != tt.wantMarkers || gotLegacy != tt.wantMarkers {
					t.Errorf("markers written = %v/%v, want %v", gotNew, gotLegacy, tt.wantMarkers)
				}
			}
		})
	}
}

func TestGateUsesHomeWithoutXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	got := Gate(GateEnv{Args: []string{"--mark-done"}, UID: 1000, Getenv: envOf(nil), Home: home})
	if got.Code != 0 {
		t.Fatalf("Code = %d, stderr %q", got.Code, got.Stderr)
	}
	for _, path := range []string{
		filepath.Join(home, ".config", "chairlift", "first-run.done"),
		filepath.Join(home, ".config", "snow-first-setup.done"),
	} {
		if !exists(t, path) {
			t.Errorf("--mark-done did not write %s", path)
		}
	}
	again := Gate(GateEnv{UID: 1000, Getenv: envOf(nil), Home: home})
	if again.Code != 0 || again.ExecPath != "" || !strings.Contains(again.Stderr, "already done") {
		t.Fatalf("gate after --mark-done = %+v, want exit 0 with no exec", again)
	}
}

func TestGateWithoutConfigHome(t *testing.T) {
	got := Gate(GateEnv{UID: 1000, Getenv: envOf(nil)})
	if got.Code != 1 || got.ExecPath != "" || !strings.Contains(got.Stderr, "cannot find the config home") {
		t.Fatalf("Gate() = %+v, want exit 1 with no exec", got)
	}
}
