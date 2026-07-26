// Package flatpak provides an interface to the Flatpak package manager
package flatpak

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// readTimeout bounds read-only flatpak commands (listing, info, remotes).
	readTimeout = 30 * time.Second
	// mutationTimeout bounds state-changing flatpak commands, which may
	// download large application images and therefore need a far larger budget.
	mutationTimeout = 30 * time.Minute
	// waitDelay bounds how long Wait blocks after the process group has been
	// signalled. flatpak's helpers (download workers, ostree pulls) inherit
	// the stdout/stderr pipes, so a straggler could otherwise hold Wait open
	// forever even though the command itself is gone.
	waitDelay = 5 * time.Second
)

var dryRun = false

// SetDryRun sets the dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("Flatpak dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// Error represents a Flatpak-related error. Err, when non-nil, carries the
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

// NotFoundError is returned when Flatpak is not installed
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// Application represents an installed Flatpak application
type Application struct {
	Name          string `json:"name"`
	ApplicationID string `json:"application"`
	Version       string `json:"version"`
	Branch        string `json:"branch"`
	Origin        string `json:"origin"`
	Installation  string `json:"installation"` // "user" or "system"
	Ref           string `json:"ref"`
}

// stateChangingCommands are commands that modify system state
var stateChangingCommands = map[string]bool{
	"install":   true,
	"uninstall": true,
	"remove":    true,
	"update":    true,
}

// commandTimeout returns the timeout class for a flatpak invocation: the
// mutation timeout for state-changing commands, the read timeout otherwise.
func commandTimeout(args []string) time.Duration {
	if len(args) > 0 && stateChangingCommands[args[0]] {
		return mutationTimeout
	}
	return readTimeout
}

// runFlatpakCommand executes a flatpak command and returns the output
func runFlatpakCommand(args ...string) (string, error) {
	if len(args) > 0 && stateChangingCommands[args[0]] && dryRun {
		msg := fmt.Sprintf("[DRY-RUN] Would execute: flatpak %s", strings.Join(args, " "))
		log.Println(msg)
		return msg, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout(args))
	defer cancel()

	return runFlatpakCommandAt(ctx, "flatpak", args...)
}

// runFlatpakCommandAt runs exe with args under ctx and returns its stdout. The
// executable and context are parameters so tests can drive a fake script and
// control the deadline; runFlatpakCommand is the only production caller and
// always passes "flatpak".
//
// The command runs in its own process group and cancellation signals the
// whole group, so flatpak's helper processes (download workers, ostree pulls)
// die with it rather than being orphaned. cmd.Run still reaps the child.
func runFlatpakCommandAt(ctx context.Context, exe string, args ...string) (string, error) {
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
			return "", &Error{
				Message: fmt.Sprintf("Command '%s' timed out", display),
				Err:     context.DeadlineExceeded,
			}
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return "", &Error{
				Message: fmt.Sprintf("Command '%s' was canceled", display),
				Err:     context.Canceled,
			}
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", &Error{Message: fmt.Sprintf("Flatpak command failed: %s", stderr.String()), Err: err}
		}
		// exec.ErrNotFound covers a bare name missing from $PATH;
		// fs.ErrNotExist covers an explicit path that does not exist.
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return "", &NotFoundError{Message: "Flatpak not found. Please install Flatpak first."}
		}
		return "", &Error{Message: err.Error(), Err: err}
	}

	return stdout.String(), nil
}

// IsInstalled checks if Flatpak is installed and accessible
func IsInstalled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "flatpak", "--version")
	err := cmd.Run()
	return err == nil
}

var (
	installedOnce   sync.Once
	installedResult bool
)

// IsInstalledCached returns a cached result of IsInstalled, running the check at most once.
func IsInstalledCached() bool {
	installedOnce.Do(func() {
		installedResult = IsInstalled()
	})
	return installedResult
}

// ListUserApplications returns all user-installed Flatpak applications
func ListUserApplications() ([]Application, error) {
	return listApplications("--user")
}

// ListSystemApplications returns all system-installed Flatpak applications
func ListSystemApplications() ([]Application, error) {
	return listApplications("--system")
}

// listApplications lists installed applications for a given installation type
func listApplications(installFlag string) ([]Application, error) {
	// Use columns format for structured output
	output, err := runFlatpakCommand("list", installFlag, "--app", "--columns=name,application,version,branch,origin,ref")
	if err != nil {
		return nil, err
	}

	return parseApplicationList(output, installFlag)
}

// parseApplicationList parses the tabular output from flatpak list
func parseApplicationList(output string, installFlag string) ([]Application, error) {
	var apps []Application
	lines := strings.Split(strings.TrimSpace(output), "\n")

	installation := "system"
	if installFlag == "--user" {
		installation = "user"
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split by tab (flatpak uses tabs as column separators)
		fields := strings.Split(line, "\t")
		if len(fields) < 6 {
			// Try splitting by multiple spaces for systems that might use spaces
			fields = strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
		}

		app := Application{
			Installation: installation,
		}

		if len(fields) >= 1 {
			app.Name = strings.TrimSpace(fields[0])
		}
		if len(fields) >= 2 {
			app.ApplicationID = strings.TrimSpace(fields[1])
		}
		if len(fields) >= 3 {
			app.Version = strings.TrimSpace(fields[2])
		}
		if len(fields) >= 4 {
			app.Branch = strings.TrimSpace(fields[3])
		}
		if len(fields) >= 5 {
			app.Origin = strings.TrimSpace(fields[4])
		}
		if len(fields) >= 6 {
			app.Ref = strings.TrimSpace(fields[5])
		}

		apps = append(apps, app)
	}

	return apps, nil
}

// Install installs a Flatpak application
func Install(appID string, user bool) error {
	args := []string{"install", "-y"}
	if user {
		args = append(args, "--user")
	} else {
		args = append(args, "--system")
	}
	args = append(args, appID)

	_, err := runFlatpakCommand(args...)
	return err
}

// Uninstall removes a Flatpak application
func Uninstall(appID string, user bool) error {
	args := []string{"uninstall", "-y"}
	if user {
		args = append(args, "--user")
	} else {
		args = append(args, "--system")
	}
	args = append(args, appID)

	_, err := runFlatpakCommand(args...)
	return err
}

// Update updates a Flatpak application or all applications
func Update(appID string, user bool) error {
	args := []string{"update", "-y"}
	if user {
		args = append(args, "--user")
	} else {
		args = append(args, "--system")
	}
	if appID != "" {
		args = append(args, appID)
	}

	_, err := runFlatpakCommand(args...)
	return err
}

// UpdateInfo represents an available Flatpak update
type UpdateInfo struct {
	Name          string `json:"name"`
	ApplicationID string `json:"application"`
	NewVersion    string `json:"new_version"`
	Branch        string `json:"branch"`
	Origin        string `json:"origin"`
	Installation  string `json:"installation"` // "user" or "system"
}

// updateListArgs builds the flatpak argument list used to query available
// updates. "--app" restricts the query to applications so runtimes never
// appear as updates.
func updateListArgs(user bool) []string {
	args := []string{"remote-ls", "--updates", "--app", "--columns=name,application,version,branch,origin"}
	if user {
		args = append(args, "--user")
	} else {
		args = append(args, "--system")
	}
	return args
}

// ListUpdates returns available updates for Flatpak applications
func ListUpdates(user bool) ([]UpdateInfo, error) {
	output, err := runFlatpakCommand(updateListArgs(user)...)
	if err != nil {
		return nil, err
	}

	return parseUpdateList(output, user)
}

// parseUpdateList parses the tabular output from the flatpak update query
func parseUpdateList(output string, user bool) ([]UpdateInfo, error) {
	var updates []UpdateInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	installation := "system"
	if user {
		installation = "user"
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split by tab (flatpak uses tabs as column separators)
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			// Try splitting by multiple spaces for systems that might use spaces
			fields = strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
		}

		update := UpdateInfo{
			Installation: installation,
		}

		if len(fields) >= 1 {
			update.Name = strings.TrimSpace(fields[0])
		}
		if len(fields) >= 2 {
			update.ApplicationID = strings.TrimSpace(fields[1])
		}
		if len(fields) >= 3 {
			update.NewVersion = strings.TrimSpace(fields[2])
		}
		if len(fields) >= 4 {
			update.Branch = strings.TrimSpace(fields[3])
		}
		if len(fields) >= 5 {
			update.Origin = strings.TrimSpace(fields[4])
		}

		updates = append(updates, update)
	}

	return updates, nil
}

// GetRemotes returns the list of configured remotes
func GetRemotes(user bool) ([]string, error) {
	args := []string{"remotes", "--columns=name"}
	if user {
		args = append(args, "--user")
	} else {
		args = append(args, "--system")
	}

	output, err := runFlatpakCommand(args...)
	if err != nil {
		return nil, err
	}

	var remotes []string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			remotes = append(remotes, line)
		}
	}

	return remotes, nil
}

// ApplicationInfo represents detailed info about a Flatpak application
type ApplicationInfo struct {
	Application
	Description string            `json:"description"`
	Runtime     string            `json:"runtime"`
	Permissions map[string]string `json:"permissions"`
}

// Info gets detailed information about a Flatpak application
func Info(appID string, user bool) (*ApplicationInfo, error) {
	args := []string{"info", "--show-metadata"}
	if user {
		args = append(args, "--user")
	} else {
		args = append(args, "--system")
	}
	args = append(args, appID)

	output, err := runFlatpakCommand(args...)
	if err != nil {
		return nil, err
	}

	// Parse the metadata output
	info := &ApplicationInfo{
		Application: Application{
			ApplicationID: appID,
			Installation:  "system",
		},
		Permissions: make(map[string]string),
	}

	if user {
		info.Installation = "user"
	}

	// Parse key=value pairs from the output
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				switch key {
				case "name":
					info.Name = value
				case "version":
					info.Version = value
				case "branch":
					info.Branch = value
				case "origin":
					info.Origin = value
				case "runtime":
					info.Runtime = value
				}
			}
		}
	}

	return info, nil
}

// UninstallUnused removes unused Flatpak runtimes and extensions
func UninstallUnused() (string, error) {
	return runFlatpakCommand("uninstall", "--unused", "-y")
}
