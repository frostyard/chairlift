// Package stageexec runs the fixed privileged staging commands used by OS
// update providers. It owns their widget-free progress and process contract.
package stageexec

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"strings"
)

// EventType classifies a ProgressEvent.
type EventType string

const (
	EventMessage  EventType = "message"
	EventComplete EventType = "complete"
)

// ProgressEvent is one line of staging progress or the final completion event.
type ProgressEvent struct {
	Type    EventType
	Message string
}

// Error describes a staging execution failure.
type Error struct {
	Message string
	Err     error
}

func (e *Error) Error() string {
	return e.Message
}

// Unwrap exposes the underlying process or context error.
func (e *Error) Unwrap() error {
	return e.Err
}

// NotFoundError reports that the requested executable does not exist.
type NotFoundError struct {
	Message string
	Err     error
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// Unwrap exposes the executable lookup error.
func (e *NotFoundError) Unwrap() error {
	return e.Err
}

// DryRun emits the standard preview and completion events without starting a
// process. It always closes progressCh.
func DryRun(ctx context.Context, progressCh chan<- ProgressEvent, scriptPath string) error {
	defer close(progressCh)

	if err := send(ctx, progressCh, ProgressEvent{
		Type:    EventMessage,
		Message: "[DRY-RUN] would run " + scriptPath,
	}); err != nil {
		return contextError(err)
	}
	if err := send(ctx, progressCh, ProgressEvent{
		Type:    EventComplete,
		Message: "Dry run complete",
	}); err != nil {
		return contextError(err)
	}
	return nil
}

// Run executes name with args, streams merged stdout and stderr as trimmed,
// non-empty EventMessage values, emits one EventComplete after a successful
// exit, and always closes progressCh. Cancellation kills and reaps only the
// direct child: current callers execute through pkexec, whose privileged child
// cannot be signaled as an unprivileged process group.
func Run(ctx context.Context, progressCh chan<- ProgressEvent, name string, args ...string) error {
	defer close(progressCh)

	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &Error{Message: fmt.Sprintf("failed to create stdout pipe: %v", err), Err: err}
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return contextError(ctxErr)
		}
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return &NotFoundError{Message: name + " not found", Err: err}
		}
		return &Error{Message: fmt.Sprintf("failed to start %s: %v", name, err), Err: err}
	}

	lines := scanLines(ctx, stdout)
	var lastLine string
	for {
		select {
		case <-ctx.Done():
			stop(cmd, stdout)
			return contextError(ctx.Err())
		case result, ok := <-lines:
			if !ok {
				goto scanned
			}
			if result.err != nil {
				stop(cmd, stdout)
				if ctxErr := ctx.Err(); ctxErr != nil {
					return contextError(ctxErr)
				}
				return &Error{
					Message: fmt.Sprintf("failed to read staging output: %v", result.err),
					Err:     result.err,
				}
			}
			lastLine = result.line
			if err := send(ctx, progressCh, ProgressEvent{Type: EventMessage, Message: result.line}); err != nil {
				stop(cmd, stdout)
				return contextError(err)
			}
		}
	}

scanned:
	if ctxErr := ctx.Err(); ctxErr != nil {
		stop(cmd, stdout)
		return contextError(ctxErr)
	}
	if err := cmd.Wait(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return contextError(ctxErr)
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

	if err := send(ctx, progressCh, ProgressEvent{
		Type:    EventComplete,
		Message: "Staging complete",
	}); err != nil {
		return contextError(err)
	}
	return nil
}

type scanResult struct {
	line string
	err  error
}

func scanLines(ctx context.Context, output io.Reader) <-chan scanResult {
	results := make(chan scanResult)
	go func() {
		defer close(results)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			select {
			case results <- scanResult{line: line}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case results <- scanResult{err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return results
}

func stop(cmd *exec.Cmd, output io.Closer) {
	_ = cmd.Process.Kill()
	_ = output.Close()
	_ = cmd.Wait()
}

func send(ctx context.Context, progressCh chan<- ProgressEvent, event ProgressEvent) error {
	select {
	case progressCh <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func contextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Message: "Update staging timed out", Err: context.DeadlineExceeded}
	}
	return &Error{Message: "Update staging was canceled", Err: context.Canceled}
}
