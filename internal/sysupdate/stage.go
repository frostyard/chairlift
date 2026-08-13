package sysupdate

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/frostyard/chairlift/internal/stageexec"
)

// StageScriptPath is the snosi-shipped native A/B stager. It checks the
// image's sysupdate target for a newer version and downloads it into the
// inactive root/verity slots via systemd-sysupdate; it never reboots, and
// it is idempotent (exits 0 without staging when already current or when
// the candidate is already in the inactive slot).
const StageScriptPath = "/usr/libexec/snosi-sysupdate-stage"

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
