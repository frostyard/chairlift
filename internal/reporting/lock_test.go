package reporting

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestLockIsExclusive proves the send lock admits 1 holder at a time and is
// released again.
func TestLockIsExclusive(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if _, err := Lock(dir); !errors.Is(err, ErrBusy) {
		t.Fatalf("a second Lock = %v, want ErrBusy", err)
	}
	info, err := os.Stat(filepath.Join(dir, LockFile))
	if err != nil {
		t.Fatalf("the lock file was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("lock file mode = %o, want 600", perm)
	}
	unlock()
	second, err := Lock(dir)
	if err != nil {
		t.Fatalf("Lock after unlock: %v", err)
	}
	second()
}

// TestRunIsBusyWhileTheLockIsHeld proves a run sends nothing while another
// holder has the lock, and that the manual path refuses too.
func TestRunIsBusyWhileTheLockIsHeld(t *testing.T) {
	clk := newClock(testStart)
	statePath := newStatePath(t)
	unlock, err := Lock(filepath.Dir(statePath))
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	defer unlock()

	sender := &recordingSender{}
	in := input(clk, statePath, sender, reportableItems(2))
	res := Run(context.Background(), in)
	if sender.count() != 0 {
		t.Fatalf("%d requests were made while the lock was held, want 0", sender.count())
	}
	if res.Skipped != "busy" {
		t.Errorf("Skipped = %q, want %q", res.Skipped, "busy")
	}
	if _, err := SendReport(context.Background(), in, sentReport(t)); !errors.Is(err, ErrBusy) {
		t.Errorf("SendReport() = %v, want ErrBusy", err)
	}
	if sender.count() != 0 {
		t.Fatalf("%d requests were made, want 0", sender.count())
	}
}
