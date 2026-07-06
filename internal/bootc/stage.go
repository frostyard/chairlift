package bootc

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// StageScriptPath is the snow-shipped workaround script that pulls the OS
// image via podman and stages it with `bootc switch --transport
// containers-storage`. See the design spec for why plain `bootc upgrade`
// is not used.
const StageScriptPath = "/usr/libexec/bootc-update-stage"

// EventType classifies a ProgressEvent.
type EventType string

const (
	EventMessage  EventType = "message"
	EventError    EventType = "error"
	EventComplete EventType = "complete"
)

// ProgressEvent is a single line of progress from the stage script.
type ProgressEvent struct {
	Type    EventType
	Message string
}

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
		progressCh <- ProgressEvent{Type: EventMessage, Message: "[DRY-RUN] would run " + StageScriptPath}
		progressCh <- ProgressEvent{Type: EventComplete, Message: "Dry run complete"}
		close(progressCh)
		return nil
	}
	return runStageStreaming(ctx, progressCh, pkexecCommand, StageScriptPath)
}

// runStageStreaming runs a command, streaming stdout+stderr lines to
// progressCh. It closes progressCh before returning. Separated from
// StageUpdate so tests can run a local fake script without pkexec.
func runStageStreaming(ctx context.Context, progressCh chan<- ProgressEvent, name string, args ...string) error {
	defer close(progressCh)

	cmd := exec.CommandContext(ctx, name, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &Error{Message: fmt.Sprintf("failed to create stdout pipe: %v", err)}
	}
	// Merge stderr into the same stream so podman/bootc messages surface.
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return &NotFoundError{Message: name + " not found"}
		}
		return &Error{Message: fmt.Sprintf("failed to start %s: %v", name, err)}
	}

	var lastLine string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lastLine = line
		select {
		case progressCh <- ProgressEvent{Type: EventMessage, Message: line}:
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			_ = cmd.Wait() // reap the killed child; error is expected here
			return ctx.Err()
		}
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &Error{Message: "Update staging timed out"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			msg := fmt.Sprintf("update staging failed (exit %d)", exitErr.ExitCode())
			if lastLine != "" {
				msg += ": " + lastLine
			}
			return &Error{Message: msg}
		}
		return &Error{Message: err.Error()}
	}

	progressCh <- ProgressEvent{Type: EventComplete, Message: "Staging complete"}
	return nil
}
