package sysupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeScript writes an executable shell script and returns its path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-stage")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func collectEvents(ch <-chan ProgressEvent) []ProgressEvent {
	var events []ProgressEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func TestRunStageStreamingSuccess(t *testing.T) {
	script := writeScript(t, `echo "Staging update: 20260810200801"
echo "Update staged; it will apply at the next reboot."`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- runStageStreaming(ctx, ch, script) }()

	events := collectEvents(ch)
	if err := <-done; err != nil {
		t.Fatalf("runStageStreaming: %v", err)
	}

	if len(events) != 3 { // 2 messages + 1 complete
		t.Fatalf("got %d events %+v, want 3", len(events), events)
	}
	if events[0].Type != EventMessage || events[0].Message != "Staging update: 20260810200801" {
		t.Errorf("event[0] = %+v", events[0])
	}
	if events[2].Type != EventComplete {
		t.Errorf("event[2] = %+v, want EventComplete", events[2])
	}
}

func TestRunStageStreamingFailure(t *testing.T) {
	script := writeScript(t, `echo "about to fail"
echo "boom" >&2
exit 3`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- runStageStreaming(ctx, ch, script) }()

	events := collectEvents(ch)
	err := <-done
	if err == nil {
		t.Fatal("runStageStreaming = nil error, want failure")
	}
	// stdout and stderr lines both stream as messages
	var sawStdout, sawStderr bool
	for _, e := range events {
		if e.Message == "about to fail" {
			sawStdout = true
		}
		if e.Message == "boom" {
			sawStderr = true
		}
		if e.Type == EventComplete {
			t.Error("got EventComplete on failure")
		}
	}
	if !sawStdout || !sawStderr {
		t.Errorf("missing streamed lines; events: %+v", events)
	}
}

func TestStageUpdateDryRun(t *testing.T) {
	SetDryRun(true)
	defer SetDryRun(false)

	ctx := context.Background()
	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- StageUpdate(ctx, ch) }()

	events := collectEvents(ch)
	if err := <-done; err != nil {
		t.Fatalf("dry-run StageUpdate: %v", err)
	}
	if len(events) == 0 || events[len(events)-1].Type != EventComplete {
		t.Errorf("dry-run should emit mock events ending in EventComplete; got %+v", events)
	}
}

func TestRunStageStreamingDeadline(t *testing.T) {
	// `exec sleep` replaces the shell, so killing the child kills the sleep
	// too and no stray process survives the test.
	script := writeScript(t, "exec sleep 30\n")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- runStageStreaming(ctx, ch, script) }()

	collectEvents(ch)
	err := waitErr(t, done)
	if err == nil {
		t.Fatal("runStageStreaming = nil error, want deadline failure")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, DeadlineExceeded) = false; err = %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("deadline error also matches context.Canceled: %v", err)
	}
	if strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("message leaks raw kill signal: %q", err.Error())
	}
}

func TestRunStageStreamingCanceled(t *testing.T) {
	script := writeScript(t, "echo \"downloading image\"\nexec sleep 30\n")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- runStageStreaming(ctx, ch, script) }()

	first, ok := <-ch
	if !ok {
		t.Fatal("channel closed before any progress event")
	}
	if first.Message != "downloading image" {
		t.Fatalf("first event = %+v, want the streamed line", first)
	}
	cancel()
	collectEvents(ch)

	err := waitErr(t, done)
	if err == nil {
		t.Fatal("runStageStreaming = nil error, want cancellation failure")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, Canceled) = false; err = %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("cancellation error also matches context.DeadlineExceeded: %v", err)
	}
	if strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("message leaks raw kill signal: %q", err.Error())
	}
}

func TestRunStageStreamingNonZeroExitIsNeitherContextOutcome(t *testing.T) {
	script := writeScript(t, `echo "checking for update"
echo "signed index cannot be fetched"
exit 4`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- runStageStreaming(ctx, ch, script) }()

	collectEvents(ch)
	err := waitErr(t, done)
	if err == nil {
		t.Fatal("runStageStreaming = nil error, want exit failure")
	}
	var stageErr *Error
	if !errors.As(err, &stageErr) {
		t.Fatalf("errors.As(*Error) = false; err = %T %v", err, err)
	}
	if !strings.Contains(stageErr.Message, "exit 4") {
		t.Errorf("message %q missing exit code", stageErr.Message)
	}
	if !strings.Contains(stageErr.Message, "signed index cannot be fetched") {
		t.Errorf("message %q missing last output line", stageErr.Message)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Errorf("plain exit failure matches a context sentinel: %v", err)
	}
}

func TestRunStageStreamingMissingExecutable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() {
		done <- runStageStreaming(ctx, ch, filepath.Join(t.TempDir(), "absent-helper"))
	}()

	collectEvents(ch)
	err := waitErr(t, done)
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("errors.As(*NotFoundError) = false; err = %T %v", err, err)
	}
}

func TestNativeABDetectionUsesMarkerConstant(t *testing.T) {
	// The gate must key off snosi's published marker contract, not a bootc
	// probe or an os-release field.
	if MarkerPath != "/usr/lib/snosi/native-ab" {
		t.Errorf("MarkerPath = %q, want the snosi native A/B marker", MarkerPath)
	}
	if StageScriptPath != "/usr/libexec/snosi-sysupdate-stage" {
		t.Errorf("StageScriptPath = %q, want the fixed snosi stager path", StageScriptPath)
	}
}

// waitErr returns the runner's error, failing the test rather than hanging if
// the runner never returns.
func waitErr(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("runStageStreaming did not return")
		return nil
	}
}
