package sysupdate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"strings"
)

// StageScriptPath is the snosi-shipped native A/B stager. It checks the
// image's sysupdate target for a newer version and downloads it into the
// inactive root/verity slots via systemd-sysupdate; it never reboots, and
// it is idempotent (exits 0 without staging when already current or when
// the candidate is already in the inactive slot).
const StageScriptPath = "/usr/libexec/snosi-sysupdate-stage"

// EventType classifies a ProgressEvent.
type EventType string

const (
	EventMessage  EventType = "message"
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
//
// Failures classify into three mutually exclusive outcomes: deadline
// (*Error unwrapping to context.DeadlineExceeded), cancellation (*Error
// unwrapping to context.Canceled, or a bare context.Canceled when the stream
// send loses the race), and a non-zero exit (*Error carrying the last output
// line, matching neither context sentinel). A missing executable yields a
// *NotFoundError.
func runStageStreaming(ctx context.Context, progressCh chan<- ProgressEvent, name string, args ...string) error {
	defer close(progressCh)

	cmd := exec.CommandContext(ctx, name, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &Error{Message: fmt.Sprintf("failed to create stdout pipe: %v", err)}
	}
	// Merge stderr into the same stream so systemd-sysupdate's progress
	// output (which the script passes through on stderr) surfaces.
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		// exec.ErrNotFound covers a bare name missing from $PATH;
		// fs.ErrNotExist covers an explicit path that does not exist.
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
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
			// Direct kill of the child only, deliberately: StageUpdate runs
			// through pkexec, so the child is root-owned and this
			// unprivileged process cannot signal it as a process group the
			// way the homebrew and flatpak runners do (making the privileged
			// path group-killable is a privilege-model change).
			_ = cmd.Process.Kill()
			_ = cmd.Wait() // reap the killed child; error is expected here
			return ctx.Err()
		}
	}

	if err := cmd.Wait(); err != nil {
		// Classify the context outcome first: the child is killed when the
		// context ends, so the raw wait error would otherwise read
		// "signal: killed".
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return &Error{Message: "Update staging timed out", Err: context.DeadlineExceeded}
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return &Error{Message: "Update staging was canceled", Err: context.Canceled}
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg := fmt.Sprintf("update staging failed (exit %d)", exitErr.ExitCode())
			if lastLine != "" {
				msg += ": " + lastLine
			}
			return &Error{Message: msg, Err: err}
		}
		return &Error{Message: err.Error(), Err: err}
	}

	progressCh <- ProgressEvent{Type: EventComplete, Message: "Staging complete"}
	return nil
}
