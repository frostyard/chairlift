// Package fwupd provides an interface to device firmware updates through the
// fwupdmgr command-line client of the Linux Vendor Firmware Service.
//
// Every call is unprivileged: fwupdmgr forwards the request to the fwupd
// daemon over D-Bus, and the daemon authorizes it with its own PolicyKit
// actions (for example org.freedesktop.fwupd.update-internal). ChairLift ships
// no PolicyKit policy and no helper binary for firmware, and never routes a
// firmware install through pkexec. See
// docs/adr/0012-firmware-updates-use-fwupds-own-polkit.md.
package fwupd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// readTimeout bounds the read-only update query.
	readTimeout = 60 * time.Second
	// mutationTimeout bounds a firmware install, which downloads a firmware
	// image and writes it to the device. It is far larger than the read
	// budget because a slow device can take many minutes to flash.
	mutationTimeout = 30 * time.Minute
	// waitDelay bounds how long Wait blocks after the process group has been
	// signalled, so a straggler holding the stdout/stderr pipes cannot keep
	// Wait open forever.
	waitDelay = 5 * time.Second
	// nothingToDoExitCode is fwupdmgr's exit status when there is no work to
	// do. Older versions use it for "no updatable devices", which is not an
	// error for an update query.
	nothingToDoExitCode = 2
)

var (
	dryRun = false

	// executable is the fwupdmgr command name. It is a variable only so tests
	// can substitute a fake script; production always uses "fwupdmgr".
	executable = "fwupdmgr"
)

// deviceIDPattern matches a fwupd device id: exactly 40 lowercase hexadecimal
// characters. ChairLift validates the id before it reaches argv, because
// PolicyKit does not inspect the arguments of the daemon's own actions and a
// malformed value must never be passed through to fwupdmgr.
var deviceIDPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// SetDryRun sets the dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("fwupd dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// Error represents a fwupd-related error. Err, when non-nil, carries the
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

// Update is one available firmware update, taken from the first (newest)
// release of a device that fwupd reports.
type Update struct {
	DeviceID       string
	DeviceName     string
	Vendor         string
	CurrentVersion string
	NewVersion     string
	Summary        string
	NeedsReboot    bool
	NeedsShutdown  bool
	// Installable is true only when fwupd reports the device as updatable
	// right now: the "updatable" flag is set, Problems is empty, and
	// UpdateError is empty. A device that fails any of those checks is still
	// listed, but `fwupdmgr update` would skip it and exit 0 without a flash.
	Installable bool
	// Reason explains why a device is not installable. It is empty when
	// Installable is true.
	Reason string
}

// The wire types mirror fwupd's JSON. Unknown keys are ignored on purpose:
// fwupdmgr emits far more fields than ChairLift uses, and a newer client must
// not break the parse.
type wireResponse struct {
	Devices []wireDevice `json:"Devices"`
}

type wireDevice struct {
	DeviceID    string        `json:"DeviceId"`
	Name        string        `json:"Name"`
	Vendor      string        `json:"Vendor"`
	Version     string        `json:"Version"`
	Flags       []string      `json:"Flags"`
	Problems    []string      `json:"Problems"`
	UpdateError string        `json:"UpdateError"`
	Releases    []wireRelease `json:"Releases"`
}

type wireRelease struct {
	Version string `json:"Version"`
	Summary string `json:"Summary"`
}

// ParseUpdates turns fwupdmgr's `get-updates --json` document into the list of
// updates the Updates page shows. It is pure, so it is tested without a
// fwupdmgr binary.
//
// A device with no releases offers nothing to install and is skipped, as is a
// device whose DeviceId is not a well-formed fwupd id. Releases are newest
// first, so the first release is the offered version. fwupd's device order is
// preserved.
//
// fwupd also lists devices that cannot update now — a closed laptop lid, a
// host on battery, or a recorded update error. Those devices stay in the list
// with Installable false and a Reason, because hiding them would remove the
// update without telling the user why it cannot be applied.
func ParseUpdates(data []byte) ([]Update, error) {
	var response wireResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, &Error{
			Message: fmt.Sprintf("Could not parse fwupdmgr output: %v", err),
			Err:     err,
		}
	}

	updates := make([]Update, 0, len(response.Devices))
	for _, device := range response.Devices {
		if len(device.Releases) == 0 || !validDeviceID(device.DeviceID) {
			continue
		}
		release := device.Releases[0]
		installable := deviceInstallable(device)
		reason := ""
		if !installable {
			reason = blockedReason(device)
		}
		updates = append(updates, Update{
			DeviceID:       device.DeviceID,
			DeviceName:     device.Name,
			Vendor:         device.Vendor,
			CurrentVersion: device.Version,
			NewVersion:     release.Version,
			Summary:        release.Summary,
			NeedsReboot:    hasFlag(device.Flags, "needs-reboot"),
			NeedsShutdown:  hasFlag(device.Flags, "needs-shutdown"),
			Installable:    installable,
			Reason:         reason,
		})
	}
	return updates, nil
}

// deviceInstallable reports whether `fwupdmgr update` would flash this device
// now. Without the "updatable" flag, with recorded problems, or with a
// recorded update error, fwupdmgr skips the device and exits 0 without a
// flash, so ChairLift must not offer the install.
func deviceInstallable(device wireDevice) bool {
	return hasFlag(device.Flags, "updatable") && len(device.Problems) == 0 && device.UpdateError == ""
}

// blockedReason explains why a device cannot be updated now. A recorded
// fwupd update error is the most specific answer, then the problem list, then
// a generic sentence.
func blockedReason(device wireDevice) string {
	if device.UpdateError != "" {
		return device.UpdateError
	}
	if len(device.Problems) > 0 {
		return "Needs attention: " + strings.Join(device.Problems, ", ")
	}
	return "This device cannot be updated now"
}

// hasFlag reports whether flags contains want.
func hasFlag(flags []string, want string) bool {
	for _, flag := range flags {
		if flag == want {
			return true
		}
	}
	return false
}

// validDeviceID reports whether deviceID is a well-formed fwupd device id.
func validDeviceID(deviceID string) bool {
	return deviceIDPattern.MatchString(deviceID)
}

// ListUpdates returns the available firmware updates. It never changes the
// host: `get-updates` only queries the daemon.
//
// fwupdmgr exits 2 when there is nothing to do (an older client prints "No
// updatable devices" to stderr), which is an empty list and not an error.
// Every other failure is returned as an *Error carrying stderr.
func ListUpdates() ([]Update, error) {
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()

	stdout, exitCode, err := runCommandAt(ctx, executable, "get-updates", "--json")
	if err != nil {
		if exitCode == nothingToDoExitCode {
			return nil, nil
		}
		return nil, err
	}

	return ParseUpdates([]byte(stdout))
}

// Install installs the firmware update for one device.
//
// The id is validated first: anything that is not 40 lowercase hexadecimal
// characters is rejected before a command is built, so no other value can
// reach fwupdmgr's argv. "--no-reboot-check" keeps fwupdmgr from asking about
// or starting a restart; ChairLift never restarts the computer itself.
func Install(deviceID string) error {
	if !validDeviceID(deviceID) {
		return &Error{
			Message: fmt.Sprintf("Invalid firmware device id %q: a fwupd device id is 40 lowercase hexadecimal characters", deviceID),
		}
	}

	args := installArgs(deviceID)

	if dryRun {
		log.Printf("[DRY-RUN] Would execute: %s %s", executable, strings.Join(args, " "))
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), mutationTimeout)
	defer cancel()

	_, _, err := runCommandAt(ctx, executable, args...)
	return err
}

// installArgs returns the argument list that installs one device's update.
func installArgs(deviceID string) []string {
	return []string{"update", "--no-reboot-check", deviceID}
}

// IsInstalled checks whether the fwupdmgr client is available.
func IsInstalled() bool {
	_, err := exec.LookPath(executable)
	return err == nil
}

var (
	installedOnce   sync.Once
	installedResult bool
)

// IsInstalledCached returns a cached result of IsInstalled, running the check
// at most once.
func IsInstalledCached() bool {
	installedOnce.Do(func() {
		installedResult = IsInstalled()
	})
	return installedResult
}

// runCommandAt runs exe with args under ctx and returns its stdout and exit
// code. The executable and context are parameters so tests can drive a fake
// script and control the deadline; ListUpdates and Install are the only
// production callers, and both pass the package executable.
//
// The command runs in its own process group and cancellation signals the
// whole group, so a helper process dies with fwupdmgr rather than being
// orphaned. cmd.Run still reaps the child.
func runCommandAt(ctx context.Context, exe string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = waitDelay

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		display := exe
		if len(args) > 0 {
			display += " " + strings.Join(args, " ")
		}
		// Classify the context outcome first: the process was killed by our
		// own Cancel func, so the raw error would otherwise read
		// "signal: killed".
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return "", -1, &Error{
				Message: fmt.Sprintf("Command '%s' timed out", display),
				Err:     context.DeadlineExceeded,
			}
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return "", -1, &Error{
				Message: fmt.Sprintf("Command '%s' was canceled", display),
				Err:     context.Canceled,
			}
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = err.Error()
			}
			return "", exitErr.ExitCode(), &Error{
				Message: fmt.Sprintf("fwupdmgr command failed: %s", message),
				Err:     err,
			}
		}
		// exec.ErrNotFound covers a bare name missing from $PATH;
		// fs.ErrNotExist covers an explicit path that does not exist.
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return "", -1, &Error{Message: "fwupdmgr not found. Please install fwupd first.", Err: err}
		}
		return "", -1, &Error{Message: err.Error(), Err: err}
	}

	return stdout.String(), 0, nil
}
