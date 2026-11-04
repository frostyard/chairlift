package firstrun

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
