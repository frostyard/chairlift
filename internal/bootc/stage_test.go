package bootc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/frostyard/chairlift/internal/stageexec"
)

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-script")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStageUpdateDryRunUsesFixedPath(t *testing.T) {
	SetDryRun(true)
	t.Cleanup(func() { SetDryRun(false) })

	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- StageUpdate(context.Background(), ch) }()

	var events []ProgressEvent
	for event := range ch {
		events = append(events, event)
	}
	if err := <-done; err != nil {
		t.Fatalf("StageUpdate: %v", err)
	}
	if StageScriptPath != "/usr/libexec/bootc-update-stage" {
		t.Errorf("StageScriptPath = %q", StageScriptPath)
	}
	if len(events) != 2 ||
		events[0] != (ProgressEvent{Type: EventMessage, Message: "[DRY-RUN] would run " + StageScriptPath}) ||
		events[1].Type != EventComplete {
		t.Errorf("events = %+v, want fixed-path preview and completion", events)
	}
}

func TestStageErrorAdapterPreservesProviderTypes(t *testing.T) {
	deadline := adaptStageError(&stageexec.Error{
		Message: "Update staging timed out",
		Err:     context.DeadlineExceeded,
	})
	var bootcErr *Error
	if !errors.As(deadline, &bootcErr) || !errors.Is(deadline, context.DeadlineExceeded) {
		t.Errorf("deadline adapter = %T %v", deadline, deadline)
	}

	missing := adaptStageError(&stageexec.NotFoundError{Message: "pkexec not found"})
	var notFound *NotFoundError
	if !errors.As(missing, &notFound) {
		t.Errorf("missing adapter = %T %v", missing, missing)
	}
}
