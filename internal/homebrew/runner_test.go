package homebrew

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeBrew writes an executable shell script standing in for the brew
// executable, following the fake-script pattern of internal/bootc's
// stage_test.go. The script records its own PID to a file, and a t.Cleanup is
// registered that force-kills that PID (and its process group) if it is
// somehow still alive when the test ends, so no test leaves a stray process
// behind.
func fakeBrew(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "self.pid")
	script := filepath.Join(dir, "fake-brew")
	content := "#!/bin/sh\necho $$ > \"" + pidFile + "\"\n" + body + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pid, err := readPID(pidFile)
		if err != nil {
			return
		}
		killPID(pid)
	})
	return script
}

// killPID force-kills a recorded PID and its process group, ignoring the
// expected "no such process" outcome.
func killPID(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func readPID(pidFile string) (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// runInBackground runs runBrewCommandAt in a goroutine so the test can cancel
// its context while the command is still running.
func runInBackground(ctx context.Context, exe string, args ...string) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := runBrewCommandAt(ctx, exe, args...)
		done <- err
	}()
	return done
}

// awaitErr waits for a backgrounded run to return, failing the test rather
// than hanging if the runner never comes back.
func awaitErr(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(15 * time.Second):
		t.Fatal("runBrewCommandAt did not return")
		return nil
	}
}

// TestRunBrewCommandAtKeepsTapInfoOffTheGitHubAPI pins the workaround for
// Homebrew's intermittent "Error: Broken pipe" from tap-info on Linux: every
// tap-info runs with HOMEBREW_NO_GITHUB_API=1 and still inherits the caller's
// environment, while no other command loses GitHub API access.
func TestRunBrewCommandAtKeepsTapInfoOffTheGitHubAPI(t *testing.T) {
	t.Setenv("HOMEBREW_NO_GITHUB_API", "")
	t.Setenv("CHAIRLIFT_TEST_MARKER", "inherited")
	script := fakeBrew(t, `printf '%s|%s' "$HOMEBREW_NO_GITHUB_API" "$CHAIRLIFT_TEST_MARKER"`)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"tap-info", "--installed", "--json"}, "1|inherited"},
		{[]string{"tap-info", "--json", "vendor/tap"}, "1|inherited"},
		{[]string{"info", "--installed", "--json=v2"}, "|inherited"},
		{[]string{"outdated", "--json=v2"}, "|inherited"},
		{[]string{"trust", "--formula", "vendor/tap/pkg"}, "|inherited"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := runBrewCommandAt(ctx, script, tc.args...)
		cancel()
		if err != nil {
			t.Fatalf("runBrewCommandAt(%v): %v", tc.args, err)
		}
		if out != tc.want {
			t.Errorf("runBrewCommandAt(%v) saw env %q, want %q", tc.args, out, tc.want)
		}
	}
}

func TestRunBrewCommandAt(t *testing.T) {
	t.Run("success captures stdout", func(t *testing.T) {
		script := fakeBrew(t, `echo "hello from brew"`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		out, err := runBrewCommandAt(ctx, script)
		if err != nil {
			t.Fatalf("runBrewCommandAt: %v", err)
		}
		if strings.TrimSpace(out) != "hello from brew" {
			t.Errorf("stdout = %q, want %q", out, "hello from brew")
		}
	})

	t.Run("non-zero exit reports stderr", func(t *testing.T) {
		script := fakeBrew(t, `echo "boom: formula missing" >&2
exit 3`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := runBrewCommandAt(ctx, script, "info", "nope")
		if err == nil {
			t.Fatal("runBrewCommandAt = nil error, want failure")
		}
		var brewErr *Error
		if !errors.As(err, &brewErr) {
			t.Fatalf("err = %T (%v), want *Error", err, err)
		}
		if !strings.Contains(brewErr.Error(), "boom: formula missing") {
			t.Errorf("message = %q, want it to contain the script's stderr", brewErr.Error())
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Error("plain non-zero exit must not classify as a deadline")
		}
		if errors.Is(err, context.Canceled) {
			t.Error("plain non-zero exit must not classify as a cancellation")
		}
	})

	t.Run("untrusted tap stderr yields UntrustedTapError", func(t *testing.T) {
		script := fakeBrew(t, `echo "Error: formula comes from an untrusted tap" >&2
exit 1`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := runBrewCommandAt(ctx, script, "upgrade", "somepkg")
		if err == nil {
			t.Fatal("runBrewCommandAt = nil error, want failure")
		}
		var trustErr *UntrustedTapError
		if !errors.As(err, &trustErr) {
			t.Fatalf("err = %T (%v), want *UntrustedTapError", err, err)
		}
	})

	// `brew bundle install` replays a failing entry's own installer output on
	// stdout and prints only its restatement on stderr. Keeping stderr alone
	// threw away the only line that said why the entry failed.
	t.Run("failed mutation reports the cause from either stream", func(t *testing.T) {
		script := fakeBrew(t, `echo "Installing io.podman_desktop.PodmanDesktop"
echo 'error: Remote "flathub" not found'
echo "Error: Installing io.podman_desktop.PodmanDesktop has failed!" >&2
echo "Error: Homebrew Bundle failed! 1 Brewfile dependency failed to install." >&2
exit 1`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := runBrewCommandAt(ctx, script, "bundle", "install", "--file=/usr/share/snow/bundles/dev.Brewfile")
		if err == nil {
			t.Fatal("runBrewCommandAt = nil error, want failure")
		}
		want := `Brew command failed: error: Remote "flathub" not found`
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("failed mutation is summarized to one line", func(t *testing.T) {
		script := fakeBrew(t, `echo "==> Installing demo"
echo "==> Pouring demo.bottle.tar.gz"
echo "Error: bottle is corrupt" >&2
exit 1`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := runBrewCommandAt(ctx, script, "install", "demo")
		if got, want := fmt.Sprint(err), "Brew command failed: Error: bottle is corrupt"; got != want {
			t.Errorf("error = %q, want %q", got, want)
		}
		var brewErr *Error
		if !errors.As(err, &brewErr) {
			t.Fatalf("err = %T, want *Error", err)
		}
	})

	t.Run("silent failed mutation reports the exit status", func(t *testing.T) {
		script := fakeBrew(t, `exit 4`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := runBrewCommandAt(ctx, script, "upgrade", "demo")
		if got, want := fmt.Sprint(err), "Brew command failed: exit status 4"; got != want {
			t.Errorf("error = %q, want %q", got, want)
		}
	})

	t.Run("untrusted tap on mutation stdout yields UntrustedTapError", func(t *testing.T) {
		script := fakeBrew(t, `echo "Installing vendor/tap/tool"
echo "Error: Refusing to load formula tool from untrusted tap vendor/tap."
echo "Installing vendor/tap/tool has failed!" >&2
exit 1`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := runBrewCommandAt(ctx, script, "bundle", "install", "--file=x.Brewfile")
		var trustErr *UntrustedTapError
		if !errors.As(err, &trustErr) {
			t.Fatalf("err = %T (%v), want *UntrustedTapError", err, err)
		}
		want := "Brew command failed: Error: Refusing to load formula tool from untrusted tap vendor/tap."
		if trustErr.Error() != want {
			t.Errorf("message = %q, want %q", trustErr.Error(), want)
		}
	})

	// A read-only command keeps its whole stderr in the error: nothing else
	// preserves it, and searchKind matches on the full text.
	t.Run("failed read keeps full stderr and ignores stdout", func(t *testing.T) {
		script := fakeBrew(t, `echo "stdout-marker"
echo "Warning: tap is shallow" >&2
echo "Error: No formulae or casks found for \"demo\"." >&2
exit 1`)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := runBrewCommandAt(ctx, script, "search", "--formula", "demo")
		if err == nil {
			t.Fatal("runBrewCommandAt = nil error, want failure")
		}
		for _, want := range []string{"Warning: tap is shallow", `Error: No formulae or casks found for "demo".`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("read-only failure lost %q: %q", want, err.Error())
			}
		}
		if strings.Contains(err.Error(), "stdout-marker") {
			t.Errorf("read-only failure = %q, want stdout left out", err.Error())
		}
	})

	t.Run("missing executable path yields NotFoundError", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "definitely-not-here")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := runBrewCommandAt(ctx, missing, "--version")
		var notFound *NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("err = %T (%v), want *NotFoundError", err, err)
		}
	})

	t.Run("missing executable on PATH yields NotFoundError", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := runBrewCommandAt(ctx, "chairlift-no-such-brew-binary", "--version")
		var notFound *NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("err = %T (%v), want *NotFoundError", err, err)
		}
	})

	t.Run("deadline unwraps to context.DeadlineExceeded", func(t *testing.T) {
		script := fakeBrew(t, `sleep 30`)

		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()

		err := awaitErr(t, runInBackground(ctx, script, "update"))
		if err == nil {
			t.Fatal("runBrewCommandAt = nil error, want deadline failure")
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
		script := fakeBrew(t, `sleep 30`)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := runInBackground(ctx, script, "update")
		time.Sleep(100 * time.Millisecond)
		cancel()

		err := awaitErr(t, done)
		if err == nil {
			t.Fatal("runBrewCommandAt = nil error, want cancellation failure")
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

// TestRunBrewCommandAtDeadlineAndCancelMessagesDiffer proves the two context
// outcomes are distinguishable by message, not only by unwrapped cause.
func TestRunBrewCommandAtDeadlineAndCancelMessagesDiffer(t *testing.T) {
	// Both runs use the same script path so the executable name embedded in
	// each error is identical: any difference in the messages then comes from
	// the classification itself, not from differing temporary paths.
	script := fakeBrew(t, `sleep 30`)

	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer deadlineCancel()
	deadlineErr := awaitErr(t, runInBackground(deadlineCtx, script, "update"))
	if deadlineErr == nil {
		t.Fatal("deadline run = nil error, want failure")
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runInBackground(cancelCtx, script, "update")
	time.Sleep(100 * time.Millisecond)
	cancel()

	cancelErr := awaitErr(t, done)
	if cancelErr == nil {
		t.Fatal("cancelled run = nil error, want failure")
	}

	if deadlineErr.Error() == cancelErr.Error() {
		t.Errorf("deadline and cancellation share the message %q", deadlineErr.Error())
	}
	for _, err := range []error{deadlineErr, cancelErr} {
		if strings.Contains(err.Error(), "signal: killed") {
			t.Errorf("err = %q, must not surface the raw kill signal", err.Error())
		}
	}
}

// TestRunBrewCommandAtKillsProcessGroup proves that a helper process spawned
// by the command (brew's git/curl/download workers, here a background sleep)
// dies with the command when the context is cancelled, rather than being
// orphaned once the direct child is gone.
func TestRunBrewCommandAtKillsProcessGroup(t *testing.T) {
	helperPID := filepath.Join(t.TempDir(), "helper.pid")
	script := fakeBrew(t, `sleep 300 &
echo $! > "$1"
sleep 300`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := runInBackground(ctx, script, helperPID)

	pid := waitForPID(t, helperPID)
	t.Cleanup(func() { killPID(pid) })

	cancel()
	if err := awaitErr(t, done); err == nil {
		t.Fatal("runBrewCommandAt = nil error, want cancellation failure")
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
