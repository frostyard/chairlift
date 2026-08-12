package bootc

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/frostyard/chairlift/internal/stageexec"
)

// StageScriptPath is the snow-shipped workaround script that pulls the OS
// image via podman and stages it with `bootc switch --transport
// containers-storage`. See the design spec for why plain `bootc upgrade`
// is not used.
const StageScriptPath = "/usr/libexec/bootc-update-stage"

// EventType classifies a ProgressEvent.
type EventType = stageexec.EventType

const (
	EventMessage  = stageexec.EventMessage
	EventComplete = stageexec.EventComplete
)

// ProgressEvent is a single line of progress from the stage script.
type ProgressEvent = stageexec.ProgressEvent

// StageScriptAvailable reports whether the stage script is installed.
func StageScriptAvailable() bool {
	_, err := os.Stat(StageScriptPath)
	return err == nil
}

// StageUpdate checks for and stages a system update by running the stage
// script via pkexec. Output lines stream to progressCh as EventMessage
// events; EventComplete is sent on success. progressCh is closed when done.
// The script is idempotent: it exits 0 without staging when already current.
func StageUpdate(ctx context.Context, progressCh chan<- ProgressEvent) error {
	if dryRun {
		log.Printf("[DRY-RUN] would execute: pkexec %s", StageScriptPath)
		return adaptStageError(stageexec.DryRun(ctx, progressCh, StageScriptPath))
	}
	return adaptStageError(stageexec.Run(ctx, progressCh, pkexecCommand, StageScriptPath))
}

func adaptStageError(err error) error {
	if err == nil {
		return nil
	}
	var notFound *stageexec.NotFoundError
	if errors.As(err, &notFound) {
		return &NotFoundError{Message: notFound.Message}
	}
	var stageErr *stageexec.Error
	if errors.As(err, &stageErr) {
		return &Error{Message: stageErr.Message, Err: stageErr.Err}
	}
	return &Error{Message: err.Error(), Err: err}
}
