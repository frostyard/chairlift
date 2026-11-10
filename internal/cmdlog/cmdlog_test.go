package cmdlog

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
)

type recorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *recorder) sink(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.Kind == KindLine {
			out = append(out, e.Text)
		}
	}
	return out
}

func TestWithoutSinkEverythingIsANoOp(t *testing.T) {
	SetSink(nil)
	r := Start("brew", "list")
	if r != nil {
		t.Fatalf("Start without a sink = %v, want nil", r)
	}
	var buf bytes.Buffer
	if w := r.Stdout(&buf); w != &buf {
		t.Fatalf("nil Run Stdout did not return the writer itself")
	}
	r.Line("ignored")
	r.End(nil)
	DryRun("brew", "upgrade")
}

func TestRunReportsStartLinesAndEnd(t *testing.T) {
	rec := &recorder{}
	SetSink(rec.sink)
	t.Cleanup(func() { SetSink(nil) })

	r := Start("flatpak", "update", "org.example.App")
	var out, errBuf bytes.Buffer
	stdout := r.Stdout(&out)
	stderr := r.Stderr(&errBuf)
	_, _ = stdout.Write([]byte("Updating org.example.App\nDown"))
	_, _ = stdout.Write([]byte("loading 3 MB\r\n\n   \n"))
	_, _ = stderr.Write([]byte("warning: partial"))
	failure := errors.New("exit 1")
	r.End(failure)
	r.End(nil) // the second End is ignored

	if out.String() != "Updating org.example.App\nDownloading 3 MB\r\n\n   \n" {
		t.Errorf("stdout was not passed through unchanged: %q", out.String())
	}
	if errBuf.String() != "warning: partial" {
		t.Errorf("stderr was not passed through unchanged: %q", errBuf.String())
	}
	want := []string{"Updating org.example.App", "Downloading 3 MB", "warning: partial"}
	if got := rec.lines(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines = %q, want %q", got, want)
	}
	first, last := rec.events[0], rec.events[len(rec.events)-1]
	if first.Kind != KindStart || strings.Join(first.Argv, " ") != "flatpak update org.example.App" {
		t.Errorf("first event = %+v, want the start with the argv", first)
	}
	if last.Kind != KindEnd || !errors.Is(last.Err, failure) {
		t.Errorf("last event = %+v, want the end with the error", last)
	}
	ends := 0
	for _, e := range rec.events {
		if e.ID != first.ID {
			t.Errorf("event %+v has another command's ID", e)
		}
		if e.Kind == KindEnd {
			ends++
		}
	}
	if ends != 1 {
		t.Errorf("got %d end events, want 1", ends)
	}
}

func TestLongLinesAreClipped(t *testing.T) {
	rec := &recorder{}
	SetSink(rec.sink)
	t.Cleanup(func() { SetSink(nil) })

	r := Start("brew", "info")
	w := r.Stdout(nil)
	_, _ = w.Write([]byte(strings.Repeat("é", MaxLine))) // 2 bytes a rune, no newline
	r.End(nil)
	for _, line := range rec.lines() {
		if len(line) > MaxLine {
			t.Errorf("line of %d bytes exceeds MaxLine", len(line))
		}
		if !strings.HasSuffix(line, "…") && len(line) == MaxLine {
			t.Errorf("clipped line does not end with an ellipsis")
		}
	}
	if len(rec.lines()) == 0 {
		t.Fatal("no line reported")
	}
}

func TestDryRunReportsTheCommand(t *testing.T) {
	rec := &recorder{}
	SetSink(rec.sink)
	t.Cleanup(func() { SetSink(nil) })

	DryRun("pkexec", "/usr/bin/chairlift-updex-helper", "update")
	if len(rec.events) != 1 || rec.events[0].Kind != KindDryRun {
		t.Fatalf("events = %+v, want one dry-run event", rec.events)
	}
	if got := strings.Join(rec.events[0].Argv, " "); got != "pkexec /usr/bin/chairlift-updex-helper update" {
		t.Errorf("argv = %q", got)
	}
}
