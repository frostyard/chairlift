package problems

import (
	"context"
	"errors"
	"io"
	"os/exec"
)

// Output caps of the page's commands. A command whose standard output
// reaches its cap is killed; ChairLift never holds more than the cap.
const (
	// MaxJournalOutput caps the journalctl output of the apps group.
	MaxJournalOutput = 8 << 20
	// maxCommandOutput caps the coredumpctl output.
	maxCommandOutput = 4 << 20
	// maxStderr caps the standard error that ChairLift keeps of a command.
	maxStderr = 4 << 10
	// readChunk is the most that 1 read asks of a command's output.
	readChunk = 32 << 10
)

// streamFunc starts a command and returns its standard output and a wait
// function. wait(kill) kills the command first when kill is true, then
// waits for it and returns its standard error (at most maxStderr bytes) and
// its exit error. Tests replace it to give oversized output.
type streamFunc func(ctx context.Context, name string, args []string) (io.Reader, func(kill bool) (string, error), error)

// execStream is the production streamFunc.
func execStream(ctx context.Context, name string, args []string) (io.Reader, func(kill bool) (string, error), error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stderr := &cappedBuffer{max: maxStderr}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return stdout, func(kill bool) (string, error) {
		if kill {
			_ = cmd.Process.Kill()
		}
		err := cmd.Wait()
		return stderr.String(), err
	}, nil
}

// cappedOutput is the result of runCapped.
type cappedOutput struct {
	// out holds at most the cap of standard output.
	out []byte
	// truncated is true when the output reached the cap; the command was
	// then killed, and its exit error is not an error of the read.
	truncated bool
	stderr    string
	err       error
}

// runCapped runs name with run and reads at most limit bytes of its
// standard output. When the output reaches the limit it stops reading and
// kills the command.
func runCapped(ctx context.Context, run streamFunc, name string, args []string, limit int) cappedOutput {
	stdout, wait, err := run(ctx, name, args)
	if err != nil {
		return cappedOutput{err: err}
	}
	out, truncated, readErr := readUpTo(stdout, limit)
	stderr, waitErr := wait(truncated || readErr != nil)
	res := cappedOutput{out: out, truncated: truncated, stderr: stderr}
	switch {
	case truncated:
	case readErr != nil:
		res.err = readErr
	default:
		res.err = waitErr
	}
	return res
}

// readUpTo reads r into a buffer of at most limit bytes. It returns true
// when r holds more than limit bytes; it then stops after 1 more byte. The
// buffer never grows past limit.
func readUpTo(r io.Reader, limit int) ([]byte, bool, error) {
	buf := make([]byte, 0, min(readChunk, limit))
	for len(buf) < limit {
		if len(buf) == cap(buf) {
			grown := make([]byte, len(buf), min(2*cap(buf)+readChunk, limit))
			copy(grown, buf)
			buf = grown
		}
		n, err := r.Read(buf[len(buf):min(cap(buf), len(buf)+readChunk)])
		buf = buf[:len(buf)+n]
		if errors.Is(err, io.EOF) {
			return buf, false, nil
		}
		if err != nil {
			return buf, false, err
		}
	}
	var one [1]byte
	for {
		n, err := r.Read(one[:])
		if n > 0 {
			return buf, true, nil
		}
		if errors.Is(err, io.EOF) {
			return buf, false, nil
		}
		if err != nil {
			return buf, false, err
		}
	}
}
