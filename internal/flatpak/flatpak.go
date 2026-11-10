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
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/frostyard/chairlift/internal/cmdlog"
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
	"install":    true,
	"uninstall":  true,
	"remove":     true,
	"update":     true,
	"remote-add": true,
}

// commandTimeout returns the timeout class for a flatpak invocation: the
// mutation timeout for state-changing commands, the read timeout otherwise.
func commandTimeout(args []string) time.Duration {
	if len(args) > 0 && stateChangingCommands[args[0]] {
		return mutationTimeout
	}
	return readTimeout
}

// runFlatpakCommand executes a flatpak command under its own timeout class
// and returns the output.
func runFlatpakCommand(args ...string) (string, error) {
	return runFlatpakCommandCtx(context.Background(), args...)
}

// runFlatpakCommandCtx executes a flatpak command under the caller's ctx with
// the command's timeout class (commandTimeout) layered on top, so whichever
// deadline is nearer wins and cancelling ctx stops the command. It is the
// single dry-run gate: a state-changing command is skipped before any
// exec.Cmd exists.
func runFlatpakCommandCtx(ctx context.Context, args ...string) (string, error) {
	if len(args) > 0 && stateChangingCommands[args[0]] && dryRun {
		msg := fmt.Sprintf("[DRY-RUN] Would execute: flatpak %s", strings.Join(args, " "))
		log.Println(msg)
		cmdlog.DryRun("flatpak", args...)
		return msg, nil
	}

	runCtx, cancel := context.WithTimeout(ctx, commandTimeout(args))
	defer cancel()

	return runFlatpakCommandAt(runCtx, "flatpak", args...)
}

// runFlatpakCommandAt runs exe with args under ctx and returns its stdout. The
// executable and context are parameters so tests can drive a fake script and
// control the deadline; runFlatpakCommandCtx is the only production caller
// and always passes "flatpak".
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
	trace := cmdlog.Start(exe, args...)
	cmd.Stdout = trace.Stdout(&stdout)
	cmd.Stderr = trace.Stderr(&stderr)

	err := cmd.Run()
	trace.End(err)
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
	trace := cmdlog.Start("flatpak", "--version")
	err := cmd.Run()
	trace.End(err)
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

// FlathubRemoteName and FlathubRepoURL name the remote the first run adds
// before it installs applications (first-setup's setup-flatpak-remote).
const (
	FlathubRemoteName = "flathub"
	FlathubRepoURL    = "https://dl.flathub.org/repo/flathub.flatpakrepo"
)

// AddRemote adds a remote to the user or system installation with
// --if-not-exists, so an existing remote of that name is kept and is not an
// error. remote-add is a state-changing command: under dry-run it is logged
// and skipped.
func AddRemote(name, url string, user bool) error {
	_, err := runFlatpakCommand("remote-add", installationFlag(user), "--if-not-exists", name, url)
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

// installationFlag returns the scope flag every per-installation query passes.
func installationFlag(user bool) string {
	if user {
		return "--user"
	}
	return "--system"
}

// installationName returns the Installation value ("user" or "system") for a
// scope.
func installationName(user bool) string {
	if user {
		return "user"
	}
	return "system"
}

// updateRemoteListArgs lists the installation's remotes with their options.
// flatpak already omits disabled remotes unless --show-disabled is given; the
// options column lets parseUpdateRemotes check that rather than depend on it.
func updateRemoteListArgs(user bool) []string {
	return []string{"remotes", installationFlag(user), "--columns=name,options"}
}

// parseUpdateRemotes returns the remotes an update query covers, in listed
// order: every listed remote except one whose options include "disabled",
// which is the set flatpak's own remote-less `remote-ls` visits.
func parseUpdateRemotes(output string) []string {
	var remotes []string
	for _, line := range strings.Split(output, "\n") {
		name, options, _ := strings.Cut(strings.TrimSpace(line), "\t")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if slices.Contains(strings.Split(strings.TrimSpace(options), ","), "disabled") {
			continue
		}
		remotes = append(remotes, name)
	}
	return remotes
}

// originListArgs lists the origin remote of every installed application in
// the installation. "--app" matches the update query's own filter: runtimes
// never appear in the update inventory, so a remote that serves only
// runtimes (the usual leftover when an application is uninstalled without
// `--unused`) cannot hide an update the inventory would list.
func originListArgs(user bool) []string {
	return []string{"list", installationFlag(user), "--app", "--columns=origin"}
}

// parseOrigins returns the set of remotes named in `flatpak list
// --columns=origin` output.
func parseOrigins(output string) map[string]bool {
	origins := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		if origin := strings.TrimSpace(line); origin != "" {
			origins[origin] = true
		}
	}
	return origins
}

// updateListArgs builds the flatpak argument list used to query one remote's
// available updates. "--app" restricts the query to applications so runtimes
// never appear as updates.
func updateListArgs(user bool, remote string) []string {
	return []string{"remote-ls", "--updates", "--app", "--columns=name,application,version,branch,origin", installationFlag(user), remote}
}

// RemoteError reports a remote whose update query failed while an installed
// application in its installation still comes from it, so updates for that
// application are unknown.
type RemoteError struct {
	Remote       string
	Installation string // "user" or "system"
	Err          error
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("Flatpak remote %q in the %s installation could not be checked: %v", e.Remote, e.Installation, e.Err)
}

// Unwrap exposes the failed query's cause to errors.Is/errors.As.
func (e *RemoteError) Unwrap() error {
	return e.Err
}

// abortsUpdateQuery reports whether a query error is not one remote's own
// failure and must therefore abort the whole update query: a cancelled
// caller context or a missing flatpak executable.
func abortsUpdateQuery(ctx context.Context, err error) bool {
	var notFound *NotFoundError
	return ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.As(err, &notFound)
}

// ListUpdates returns available updates for Flatpak applications in one
// installation. See listUpdates for the per-remote contract.
func ListUpdates(user bool) ([]UpdateInfo, error) {
	return listUpdates(context.Background(), user)
}

// listUpdates queries each enabled remote of the installation separately,
// because a single remote-less `remote-ls --updates` fails outright when any
// one remote is unreachable, so a leftover remote from an uninstalled
// application hid every other remote's updates. A remote whose query fails is
// logged and ignored when no installed application in the installation comes
// from it; otherwise it is reported as a *RemoteError. When the returned
// error is a remote failure, the slice still holds the updates the healthy
// remotes reported. A cancelled ctx or a missing executable aborts the whole
// query with a nil slice. Every query runs under ctx with the read timeout
// applied per query, so one slow remote cannot starve the others.
func listUpdates(ctx context.Context, user bool) ([]UpdateInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	remotesOutput, err := runFlatpakCommandCtx(ctx, updateRemoteListArgs(user)...)
	if err != nil {
		return nil, err
	}

	installation := installationName(user)
	var updates []UpdateInfo
	var failed []*RemoteError
	for _, remote := range parseUpdateRemotes(remotesOutput) {
		output, err := runFlatpakCommandCtx(ctx, updateListArgs(user, remote)...)
		if err != nil {
			if abortsUpdateQuery(ctx, err) {
				return nil, err
			}
			failed = append(failed, &RemoteError{Remote: remote, Installation: installation, Err: err})
			continue
		}
		remoteUpdates, _ := parseUpdateList(output, user)
		updates = append(updates, remoteUpdates...)
	}
	if len(failed) == 0 {
		return updates, nil
	}

	originsOutput, err := runFlatpakCommandCtx(ctx, originListArgs(user)...)
	if err != nil {
		if abortsUpdateQuery(ctx, err) {
			return nil, err
		}
		// Without the origins no failed remote can be shown unused.
		errs := []error{err}
		for _, remoteErr := range failed {
			errs = append(errs, remoteErr)
		}
		return updates, errors.Join(errs...)
	}
	origins := parseOrigins(originsOutput)
	var errs []error
	for _, remoteErr := range failed {
		if !origins[remoteErr.Remote] {
			log.Printf("flatpak: ignoring %s remote %q, which no installed application uses: %v", installation, remoteErr.Remote, remoteErr.Err)
			continue
		}
		errs = append(errs, remoteErr)
	}
	return updates, errors.Join(errs...)
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

// UninstallUnused removes unused Flatpak runtimes and extensions. It passes
// no scope flag on purpose: flatpak parses `uninstall` with
// FLATPAK_BUILTIN_FLAG_ALL_DIRS, so a scope-less `--unused` cleans the user
// installation, the default system installation and every extra system
// installation. A --user or --system flag would narrow that.
func UninstallUnused() (string, error) {
	return runFlatpakCommand("uninstall", "--unused", "-y")
}
