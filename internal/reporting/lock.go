package reporting

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrBusy is returned by Lock when another process holds the send lock: the
// GUI and `chairlift --send-pending-reports` must never send at the same
// time (one sender per host).
var ErrBusy = errors.New("reporting: another process is sending reports")

// LockFile is the name of the lock file inside the state folder.
const LockFile = "reports.lock"

// Lock takes the send lock of the state folder dir. The lock file is
// created with mode 0600 in a folder with mode 0700, and the lock is a
// non-blocking exclusive flock: a second holder gets ErrBusy at once
// instead of waiting. unlock releases it; it is safe to call once.
func Lock(dir string) (unlock func(), err error) {
	if dir == "" {
		return nil, errors.New("reporting: the send lock has no folder")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, LockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("reporting: take the send lock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
