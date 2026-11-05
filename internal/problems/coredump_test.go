package problems

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCoredumpsShapes(t *testing.T) {
	array := `[{"time": 1793779200000000, "pid": 1, "uid": 1000, "gid": 1000, "sig": 11, "exe": "/usr/bin/a", "size": 0},
{"time": 1793779300000000, "pid": 2, "uid": 1000, "exe": "/usr/bin/b"},
{"pid": 3, "uid": 1000, "exe": "/usr/bin/no-time"},
{"time": 1793779400000000, "pid": 4, "exe": "/usr/bin/no-uid"}]`
	lines := `{"time": 1793779200000000, "pid": 1, "uid": 1000, "sig": 6, "exe": "/usr/bin/a"}
{"time": 1793779300000000, "pid": 2, "uid": 0, "sig": 11, "exe": "/usr/bin/b"}`
	tests := []struct {
		name    string
		data    string
		want    int
		wantErr bool
	}{
		{"array", array, 2, false},
		{"lines", lines, 2, false},
		{"empty", "  \n", 0, false},
		{"bad array", `[{"time": "x"}]`, 0, true},
		{"bad line", `{"time": 1}` + "\n{", 0, true},
		{"time as text", `[{"time": "1793779200000000", "uid": 1000, "exe": "/usr/bin/a"}]`, 0, true},
		{"uid as text", `[{"time": 1793779200000000, "uid": "1000", "exe": "/usr/bin/a"}]`, 0, true},
		{"sig as text", `[{"time": 1793779200000000, "uid": 1000, "sig": "11", "exe": "/usr/bin/a"}]`, 0, true},
		{"uid as text, 1 object a line", `{"time": 1793779200000000, "uid": "1000", "exe": "/usr/bin/a"}`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCoredumps([]byte(tt.data))
			if (err != nil) != tt.wantErr || len(got) != tt.want {
				t.Fatalf("ParseCoredumps = %d entries, %v; want %d, error %v", len(got), err, tt.want, tt.wantErr)
			}
		})
	}
	got, _ := ParseCoredumps([]byte(array))
	if want := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC); !got[0].Time.Equal(want) || got[0].Signal != 11 || got[0].PID != 1 {
		t.Errorf("entry = %+v, want time %v, signal 11, pid 1", got[0], want)
	}
	if got[1].Signal != 0 {
		t.Errorf("an absent sig = %d, want 0", got[1].Signal)
	}
}

func TestSignalNameCoversTheV1Enum(t *testing.T) {
	want := map[int]string{4: "SIGILL", 5: "SIGTRAP", 6: "SIGABRT", 7: "SIGBUS", 8: "SIGFPE", 11: "SIGSEGV", 31: "SIGSYS",
		0: "other", 9: "other", 15: "other", -1: "other"}
	for n, name := range want {
		if got := SignalName(n); got != name {
			t.Errorf("SignalName(%d) = %q, want %q", n, got, name)
		}
	}
}

func TestReportableExe(t *testing.T) {
	tests := []struct {
		path, want string
		ok         bool
	}{
		{"/usr/bin/gnome-shell", "gnome-shell", true},
		{"/usr/libexec/gnome-shell-calendar-server", "gnome-shell-calendar-server", true},
		{"/usr/lib/x86_64-linux-gnu/libexec/kf6/kioworker", "kioworker", true},
		{"/home/brian/bin/tool", "", false},
		{"/opt/app/bin/app", "", false},
		{"/app/bin/flatpak-app", "", false},
		{"/usr/bin/../../home/brian/x", "", false},
		{"/usr/bin/", "", false},
		{"/usr/bin/.hidden", "", false},
		{"usr/bin/relative", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := ReportableExe(tt.path)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ReportableExe(%q) = %q, %v; want %q, %v", tt.path, got, ok, tt.want, tt.ok)
		}
	}
}

func TestCrashItemsFixture(t *testing.T) {
	env := fixtureSystem(t).Envelope
	items := fixtureCrashes(t, env)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3 (gnome-shell, tool, foo; uid 1001 dropped)", len(items))
	}
	shell := items[0]
	if Title(shell) != "gnome-shell stopped unexpectedly" || shell.Count != 2 || !shell.Reportable {
		t.Errorf("gnome-shell item = %+v", shell)
	}
	if want := time.Date(2026, 11, 5, 2, 36, 50, 0, time.UTC); !shell.LastSeen.Equal(want) {
		t.Errorf("last = %v, want %v", shell.LastSeen, want)
	}
	if want := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC); !shell.FirstSeen.Equal(want) {
		t.Errorf("first = %v, want %v", shell.FirstSeen, want)
	}
	if shell.Week != "2026-W45" || shell.Origin != OriginUserCrash || !strings.HasPrefix(shell.ID, "crash-") {
		t.Errorf("gnome-shell week %q origin %s id %s", shell.Week, shell.Origin, shell.ID)
	}
	tool := items[1]
	if tool.Reportable || tool.ValidString("signal") != "SIGABRT" || tool.Local.ExePath != "/home/brian/bin/tool" {
		t.Errorf("tool item = %+v", tool)
	}
	if f, _ := tool.Field("exe"); f.Valid || f.Display != "tool" {
		t.Errorf("tool exe field = %+v, want shown and not valid", f)
	}
	foo := items[2]
	if foo.ValidString("signal") != "other" || !foo.Reportable {
		t.Errorf("foo item = %+v", foo)
	}
	// Without a valid envelope nothing is reportable.
	for _, it := range fixtureCrashes(t, Envelope{}) {
		if it.Reportable {
			t.Errorf("%s is reportable without an envelope", it.ID)
		}
	}
}

func TestCrashItemsAreCappedAndKeepNoExe(t *testing.T) {
	var entries []CoredumpEntry
	for i := 0; i < MaxItems+3; i++ {
		entries = append(entries, CoredumpEntry{Time: time.Unix(int64(i), 0), UID: 7, Signal: 11, Exe: fmt.Sprintf("/usr/bin/p%d", i)})
	}
	entries = append(entries, CoredumpEntry{Time: time.Unix(1000, 0), UID: 7, Signal: 6})
	items := CrashItems(entries, 7, Envelope{Valid: true, ImageID: "snow", ImageVersion: "20261105100000"})
	if len(items) != MaxItems {
		t.Fatalf("items = %d, want %d", len(items), MaxItems)
	}
	if items[0].Local.ExePath != "" || items[0].Reportable || Title(items[0]) != "A program stopped unexpectedly" {
		t.Errorf("an entry with no exe = %+v", items[0])
	}
}

// fakeCommand writes an executable shell script and returns its path.
func fakeCommand(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunCoredumpctl(t *testing.T) {
	ctx := context.Background()
	t.Run("args and output", func(t *testing.T) {
		bin := fakeCommand(t, `echo "$@" >&2; [ "$*" = "--json=short --no-pager --since=-90d" ] || exit 9; echo '[]'`)
		out, err := RunCoredumpctl(ctx, bin)
		if err != nil || strings.TrimSpace(string(out)) != "[]" {
			t.Fatalf("RunCoredumpctl = %q, %v", out, err)
		}
	})
	t.Run("no coredumps", func(t *testing.T) {
		bin := fakeCommand(t, `echo "No coredumps found." >&2; exit 1`)
		out, err := RunCoredumpctl(ctx, bin)
		if err != nil || out != nil {
			t.Fatalf("RunCoredumpctl = %q, %v; want no output and no error", out, err)
		}
	})
	t.Run("not installed", func(t *testing.T) {
		out, err := RunCoredumpctl(ctx, "chairlift-test-no-such-coredumpctl")
		if err != nil || out != nil {
			t.Fatalf("RunCoredumpctl = %q, %v; want no output and no error", out, err)
		}
	})
	t.Run("other failure", func(t *testing.T) {
		bin := fakeCommand(t, `echo "Failed to open journal" >&2; exit 1`)
		if _, err := RunCoredumpctl(ctx, bin); err == nil || !strings.Contains(err.Error(), "Failed to open journal") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("too large", func(t *testing.T) {
		bin := fakeCommand(t, `head -c 4194400 /dev/zero`)
		if _, err := RunCoredumpctl(ctx, bin); err == nil || !strings.Contains(err.Error(), "reached the cap") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		bin := fakeCommand(t, `exec sleep 5`)
		cctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		if _, err := RunCoredumpctl(cctx, bin); err == nil {
			t.Fatal("a killed command gave no error")
		}
	})
}
