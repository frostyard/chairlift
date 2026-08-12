package sysupdate

import (
	"context"
	"errors"
	"testing"

	"github.com/frostyard/chairlift/internal/stageexec"
)

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
	if StageScriptPath != "/usr/libexec/snosi-sysupdate-stage" {
		t.Errorf("StageScriptPath = %q", StageScriptPath)
	}
	if len(events) != 2 ||
		events[0] != (ProgressEvent{Type: EventMessage, Message: "[DRY-RUN] would run " + StageScriptPath}) ||
		events[1].Type != EventComplete {
		t.Errorf("events = %+v, want fixed-path preview and completion", events)
	}
}

func TestStageErrorAdapterPreservesProviderTypes(t *testing.T) {
	canceled := adaptStageError(&stageexec.Error{
		Message: "Update staging was canceled",
		Err:     context.Canceled,
	})
	var updateErr *Error
	if !errors.As(canceled, &updateErr) || !errors.Is(canceled, context.Canceled) {
		t.Errorf("cancellation adapter = %T %v", canceled, canceled)
	}

	missing := adaptStageError(&stageexec.NotFoundError{Message: "pkexec not found"})
	var notFound *NotFoundError
	if !errors.As(missing, &notFound) {
		t.Errorf("missing adapter = %T %v", missing, missing)
	}
}

func TestNativeABDetectionUsesMarkerConstant(t *testing.T) {
	if MarkerPath != "/usr/lib/snosi/native-ab" {
		t.Errorf("MarkerPath = %q, want the snosi native A/B marker", MarkerPath)
	}
}
