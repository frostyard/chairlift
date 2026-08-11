// Package sysupdate provides an interface to Snow Linux native A/B
// (systemd-sysupdate) OS updates. Status reads parse the world-readable
// state files under /run/snosi written by the OS-shipped stager
// (unprivileged). Update staging is delegated to the snosi-shipped script
// /usr/libexec/snosi-sysupdate-stage via pkexec; the script checks for and
// downloads a newer image into the inactive root/verity slots in one
// idempotent run and never reboots — the staged version applies at the next
// natural reboot via systemd-boot entry selection.
package sysupdate

import (
	"context"
	"log"
	"os"
	"sync"
	"time"
)

const (
	// MarkerPath is snosi's canonical native A/B marker. Its presence is the
	// OS's own published contract for "this host updates via
	// systemd-sysupdate": every snosi unit and script gates on it
	// (ConditionPathExists=/usr/lib/snosi/native-ab). The bootc binary is
	// absent on native A/B hosts, so this marker — not a bootc probe — is
	// the host-type gate.
	MarkerPath = "/usr/lib/snosi/native-ab"

	pkexecCommand  = "pkexec"
	DefaultTimeout = 30 * time.Minute
)

var dryRun = false

// SetDryRun enables/disables dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("sysupdate dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// DefaultContext returns a context with the default 30-minute timeout
func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), DefaultTimeout)
}

// Error represents a sysupdate-related error. Err, when non-nil, carries the
// underlying cause (for example context.DeadlineExceeded or
// context.Canceled) so callers can classify it with errors.Is.
type Error struct {
	Message string
	Err     error
}

func (e *Error) Error() string {
	return e.Message
}

// Unwrap exposes the underlying cause to errors.Is/errors.As.
func (e *Error) Unwrap() error {
	return e.Err
}

// NotFoundError is returned when a required executable is not installed
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// IsNativeAB reports whether this host is a native A/B (systemd-sysupdate)
// install, by the presence of snosi's marker file.
func IsNativeAB() bool {
	_, err := os.Stat(MarkerPath)
	return err == nil
}

var (
	nativeABOnce   sync.Once
	nativeABResult bool
)

// IsNativeABCached returns a cached result of IsNativeAB, running the check
// at most once. Safe to call from view goroutines during async startup.
func IsNativeABCached() bool {
	nativeABOnce.Do(func() {
		nativeABResult = IsNativeAB()
	})
	return nativeABResult
}
