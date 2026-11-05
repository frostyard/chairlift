package problems

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

// processGone reports whether pid no longer runs: kill -0 fails with ESRCH,
// or the process is a zombie that waits to be reaped.
func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return true
	}
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return os.IsNotExist(err)
	}
	// The state follows the ")" that ends the command name.
	if i := strings.LastIndexByte(string(stat), ')'); i >= 0 && i+2 < len(stat) {
		return stat[i+2] == 'Z'
	}
	return false
}

// TestCappedReadKillsDescendantsThatHoldThePipes runs a real shell whose
// background child writes endless output to stdout, ignores SIGPIPE and
// holds stderr, while the shell waits for it. Killing only the shell would
// leave the child alive with the pipes open, and Wait would block forever.
// runCapped with execStream must return in bounded time, cut the output at
// the cap, and leave no descendant behind.
func TestCappedReadKillsDescendantsThatHoldThePipes(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is missing")
	}
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	// The child writes its own PID before any output, so the file exists
	// before the cap can be reached.
	script := `/bin/sh -c 'echo $$ > "$0"; trap "" PIPE; while :; do echo xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx; done' "$1" &
wait`
	const limit = 256 << 10

	done := make(chan cappedOutput, 1)
	start := time.Now()
	go func() {
		done <- runCapped(context.Background(), execStream, "/bin/sh", []string{"-c", script, "sh", pidFile}, limit)
	}()

	childPID := func() int {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return 0
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		return pid
	}
	t.Cleanup(func() {
		if pid := childPID(); pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})

	var res cappedOutput
	select {
	case res = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("runCapped did not return within 10 seconds: a descendant holds the pipes")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("runCapped took %s, want at most 5 seconds", elapsed)
	}
	if !res.truncated || res.err != nil {
		t.Errorf("truncated = %v, err = %v; want true and nil", res.truncated, res.err)
	}
	if len(res.out) > limit {
		t.Errorf("read %d bytes, over the cap %d", len(res.out), limit)
	}
	pid := childPID()
	if pid <= 0 {
		t.Fatal("the script wrote no child PID")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !processGone(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("the background child %d still runs after runCapped returned", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
