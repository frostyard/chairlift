package stageexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunSuccessMergesOutputAndCompletes(t *testing.T) {
	script := writeScript(t, `echo "  stdout line  "
echo "stderr line" >&2
echo "   "
`)
	ch, done := startRun(context.Background(), script)
	events := collectEvents(ch)
	if err := waitErr(t, done); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []ProgressEvent{
		{Type: EventMessage, Message: "stdout line"},
		{Type: EventMessage, Message: "stderr line"},
		{Type: EventComplete, Message: "Staging complete"},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %+v, want %+v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event[%d] = %+v, want %+v", i, events[i], want[i])
		}
	}
}

func TestRunNonZeroExitReturnsLastOutputWithoutCompletion(t *testing.T) {
	script := writeScript(t, `echo "first"
echo "last failure" >&2
exit 4
`)
	ch, done := startRun(context.Background(), script)
	events := collectEvents(ch)
	err := waitErr(t, done)

	var runErr *Error
	if !errors.As(err, &runErr) {
		t.Fatalf("errors.As(*Error) = false; err = %T %v", err, err)
	}
	if !strings.Contains(runErr.Message, "exit 4") || !strings.Contains(runErr.Message, "last failure") {
		t.Errorf("error = %q, want exit code and last output", runErr.Message)
	}
	assertNoCompletion(t, events)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Errorf("exit error matches context sentinel: %v", err)
	}
}

func TestRunDeadlineClosesWithoutCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)
	ch, done := startRun(ctx, writeScript(t, "exec sleep 30\n"))
	events := collectEvents(ch)
	err := waitErr(t, done)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(DeadlineExceeded) = false; err = %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("deadline error matches Canceled: %v", err)
	}
	if strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("error leaks kill signal: %q", err)
	}
	assertNoCompletion(t, events)
}

func TestRunCancellationClosesWithoutCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, done := startRun(ctx, writeScript(t, "echo started\nexec sleep 30\n"))
	first := <-ch
	if first != (ProgressEvent{Type: EventMessage, Message: "started"}) {
		t.Fatalf("first event = %+v", first)
	}
	cancel()
	events := collectEvents(ch)
	err := waitErr(t, done)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(Canceled) = false; err = %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("cancellation error matches DeadlineExceeded: %v", err)
	}
	if strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("error leaks kill signal: %q", err)
	}
	assertNoCompletion(t, events)
}

func TestRunClassifiesContextDoneBeforeStart(t *testing.T) {
	tests := map[string]struct {
		context func() context.Context
		want    error
	}{
		"canceled": {
			context: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			want: context.Canceled,
		},
		"deadline": {
			context: func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				t.Cleanup(cancel)
				return ctx
			},
			want: context.DeadlineExceeded,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ch, done := startRun(test.context(), writeScript(t, "echo should-not-run\n"))
			events := collectEvents(ch)
			err := waitErr(t, done)
			if !errors.Is(err, test.want) {
				t.Errorf("errors.Is(%v) = false; err = %v", test.want, err)
			}
			assertNoCompletion(t, events)
		})
	}
}

func TestRunCancellationDoesNotWaitForInheritedOutputPipe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	script := writeScript(t, `parent=$$
(while kill -0 "$parent" 2>/dev/null; do sleep 0.05; done) &
echo started
wait
`)
	ch, done := startRun(ctx, script)
	if first := <-ch; first.Message != "started" {
		t.Fatalf("first event = %+v", first)
	}
	cancel()
	collectEvents(ch)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("errors.Is(Canceled) = false; err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run waited for a descendant retaining the output pipe")
	}
}

func TestRunMissingExecutableRecognizesNameAndPath(t *testing.T) {
	tests := map[string]string{
		"bare name": "chairlift-stage-command-that-does-not-exist",
		"path":      filepath.Join(t.TempDir(), "absent-stage-command"),
	}
	for name, executable := range tests {
		t.Run(name, func(t *testing.T) {
			ch, done := startRun(context.Background(), executable)
			events := collectEvents(ch)
			err := waitErr(t, done)
			var notFound *NotFoundError
			if !errors.As(err, &notFound) {
				t.Fatalf("errors.As(*NotFoundError) = false; err = %T %v", err, err)
			}
			assertNoCompletion(t, events)
		})
	}
}

func TestDryRunEmitsPreviewCompletesAndCloses(t *testing.T) {
	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- DryRun(context.Background(), ch, "/fixed/stage") }()
	events := collectEvents(ch)
	if err := waitErr(t, done); err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want preview and completion", events)
	}
	if events[0] != (ProgressEvent{Type: EventMessage, Message: "[DRY-RUN] would run /fixed/stage"}) {
		t.Errorf("preview = %+v", events[0])
	}
	if events[1] != (ProgressEvent{Type: EventComplete, Message: "Dry run complete"}) {
		t.Errorf("completion = %+v", events[1])
	}
}

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-stage")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func startRun(ctx context.Context, executable string) (<-chan ProgressEvent, <-chan error) {
	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- Run(ctx, ch, executable) }()
	return ch, done
}

func collectEvents(ch <-chan ProgressEvent) []ProgressEvent {
	var events []ProgressEvent
	for event := range ch {
		events = append(events, event)
	}
	return events
}

func assertNoCompletion(t *testing.T, events []ProgressEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type == EventComplete {
			t.Errorf("unexpected completion event: %+v", event)
		}
	}
}

func waitErr(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("stage executor did not return")
		return nil
	}
}
