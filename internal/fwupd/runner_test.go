package fwupd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// killPID force-kills a recorded PID and its process group, ignoring the
// expected "no such process" outcome.
func killPID(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// readPID reads the PID a fake script recorded.
func readPID(pidFile string) (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// runInBackground runs runCommandAt in a goroutine so the test can cancel its
// context while the command is still running.
func runInBackground(ctx context.Context, exe string, args ...string) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, _, err := runCommandAt(ctx, exe, args...)
		done <- err
	}()
	return done
}

// awaitRun waits for a backgrounded run to return, failing the test rather
// than hanging if the runner never comes back.
func awaitRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(15 * time.Second):
		t.Fatal("runCommandAt did not return")
		return nil
	}
}

func TestRunFwupdCommandAt(t *testing.T) {
	t.Run("success captures stdout and exit code", func(t *testing.T) {
		script := fakeFwupdmgr(t, `echo "hello from fwupdmgr"`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		out, exitCode, err := runCommandAt(ctx, script)
		if err != nil {
			t.Fatalf("runCommandAt: %v", err)
		}
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
		if strings.TrimSpace(out) != "hello from fwupdmgr" {
			t.Errorf("stdout = %q, want %q", out, "hello from fwupdmgr")
		}
	})

	t.Run("non-zero exit reports stderr and the exit code", func(t *testing.T) {
		script := fakeFwupdmgr(t, `echo "boom: device not found" >&2
exit 3`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, exitCode, err := runCommandAt(ctx, script, "update", "nope")
		if err == nil {
			t.Fatal("runCommandAt = nil error, want failure")
		}
		if exitCode != 3 {
			t.Errorf("exit code = %d, want 3", exitCode)
		}
		var fwupdErr *Error
		if !errors.As(err, &fwupdErr) {
			t.Fatalf("err = %T (%v), want *Error", err, err)
		}
		if !strings.Contains(fwupdErr.Error(), "boom: device not found") {
			t.Errorf("message = %q, want it to contain the script's stderr", fwupdErr.Error())
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Error("plain non-zero exit must not classify as a deadline")
		}
		if errors.Is(err, context.Canceled) {
			t.Error("plain non-zero exit must not classify as a cancellation")
		}
	})

	t.Run("exit status 2 is reported with its exit code", func(t *testing.T) {
		script := fakeFwupdmgr(t, `echo "No updatable devices" >&2
exit 2`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, exitCode, err := runCommandAt(ctx, script, "get-updates", "--json")
		if err == nil {
			t.Fatal("runCommandAt = nil error, want failure")
		}
		if exitCode != nothingToDoExitCode {
			t.Errorf("exit code = %d, want %d", exitCode, nothingToDoExitCode)
		}
	})

	t.Run("missing executable path yields an error", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "definitely-not-here")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, _, err := runCommandAt(ctx, missing, "--version")
		var fwupdErr *Error
		if !errors.As(err, &fwupdErr) {
			t.Fatalf("err = %T (%v), want *Error", err, err)
		}
	})

	t.Run("deadline unwraps to context.DeadlineExceeded", func(t *testing.T) {
		script := fakeFwupdmgr(t, `sleep 30`)

		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()

		err := awaitRun(t, runInBackground(ctx, script, "update"))
		if err == nil {
			t.Fatal("runCommandAt = nil error, want deadline failure")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want errors.Is(err, context.DeadlineExceeded)", err)
		}
		if errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, must not classify as a cancellation", err)
		}
		if strings.Contains(err.Error(), "signal: killed") {
			t.Errorf("err = %q, must not surface the raw kill signal", err.Error())
		}
	})

	t.Run("cancellation unwraps to context.Canceled", func(t *testing.T) {
		script := fakeFwupdmgr(t, `sleep 30`)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := runInBackground(ctx, script, "update")
		time.Sleep(100 * time.Millisecond)
		cancel()

		err := awaitRun(t, done)
		if err == nil {
			t.Fatal("runCommandAt = nil error, want cancellation failure")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want errors.Is(err, context.Canceled)", err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, must not classify as a deadline", err)
		}
		if strings.Contains(err.Error(), "signal: killed") {
			t.Errorf("err = %q, must not surface the raw kill signal", err.Error())
		}
	})
}

// TestRunFwupdCommandAtKillsProcessGroup proves that a helper process spawned
// by the command dies with the command when the context is cancelled, rather
// than being orphaned once the direct child is gone.
func TestRunFwupdCommandAtKillsProcessGroup(t *testing.T) {
	helperPID := filepath.Join(t.TempDir(), "helper.pid")
	script := fakeFwupdmgr(t, `sleep 300 &
echo $! > "$1"
sleep 300`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := runInBackground(ctx, script, helperPID)

	pid := waitForPID(t, helperPID)
	t.Cleanup(func() { killPID(pid) })

	cancel()
	if err := awaitRun(t, done); err == nil {
		t.Fatal("runCommandAt = nil error, want cancellation failure")
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return // the helper process is gone
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper pid %d survived cancellation (kill(pid, 0) = %v)", pid, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitForPID polls pidFile until the fake script has recorded its background
// helper's PID, failing rather than hanging if it never does.
func waitForPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if pid, err := readPID(pidFile); err == nil && pid > 0 {
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("fake script never wrote a PID to %s", pidFile)
	return 0
}
