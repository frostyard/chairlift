package bootc

import (
	"context"
	"os"
	"path/filepath"
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
	script := writeScript(t, `echo "Staging update: img"
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
	if events[0].Type != EventMessage || events[0].Message != "Staging update: img" {
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
