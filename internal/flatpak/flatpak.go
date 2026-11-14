// Package flatpak provides an interface to the Flatpak package manager
package flatpak

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
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
// exec.Cmd exists. See runFlatpakCommandEnv.
func runFlatpakCommandCtx(ctx context.Context, args ...string) (string, error) {
	return runFlatpakCommandEnv(ctx, nil, args...)
}

// runFlatpakCommandEnv executes a flatpak command under the caller's ctx with
// the command's timeout class (commandTimeout) layered on top, so whichever
// deadline is nearer wins and cancelling ctx stops the command. It is the
// single dry-run gate: a state-changing command is skipped before any
// exec.Cmd exists. env entries are added to the inherited process
// environment; a nil env inherits it unchanged. The alt-id query passes
// LC_ALL=C, because flatpak translates the labels it prints with gettext.
func runFlatpakCommandEnv(ctx context.Context, env []string, args ...string) (string, error) {
	if len(args) > 0 && stateChangingCommands[args[0]] && dryRun {
		msg := fmt.Sprintf("[DRY-RUN] Would execute: flatpak %s", strings.Join(args, " "))
		log.Println(msg)
		cmdlog.DryRun("flatpak", args...)
		return msg, nil
	}

	runCtx, cancel := context.WithTimeout(ctx, commandTimeout(args))
	defer cancel()

	return runFlatpakCommandAt(runCtx, "flatpak", env, args...)
}

// runFlatpakCommandAt runs exe with args under ctx and returns its stdout. The
// executable, extra environment entries and context are parameters so tests
// can drive a fake script, hand it a locale and control the deadline;
// runFlatpakCommandEnv is the only production caller and always passes
// "flatpak". A nil env leaves the child's environment exactly as inherited;
// otherwise the entries are appended to it, so a later value of a variable
// wins over the inherited one.
//
// The command runs in its own process group and cancellation signals the
// whole group, so flatpak's helper processes (download workers, ostree pulls)
// die with it rather than being orphaned. cmd.Run still reaps the child.
func runFlatpakCommandAt(ctx context.Context, exe string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
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
	_, err := runFlatpakCommand("install", "-y", installationFlag(user), appID)
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
	_, err := runFlatpakCommand("uninstall", "-y", installationFlag(user), appID)
	return err
}

// Update updates a Flatpak application or all applications
func Update(appID string, user bool) error {
	args := []string{"update", "-y", installationFlag(user)}
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

// updateRemote is one remote an update query covers. oci is true when the
// remote's options include "oci", which makes it an OCI registry rather than
// an OSTree repository: its refs are named by image digest (see
// isInstalledImage).
type updateRemote struct {
	name string
	oci  bool
}

// parseUpdateRemotes returns the remotes an update query covers, in listed
// order: every listed remote except one whose options include "disabled",
// which is the set flatpak's own remote-less `remote-ls` visits. Each remote
// reports whether its options also include "oci".
func parseUpdateRemotes(output string) []updateRemote {
	var remotes []updateRemote
	for _, line := range strings.Split(output, "\n") {
		name, options, _ := strings.Cut(strings.TrimSpace(line), "\t")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		optionList := strings.Split(strings.TrimSpace(options), ",")
		if slices.Contains(optionList, "disabled") {
			continue
		}
		remotes = append(remotes, updateRemote{name: name, oci: slices.Contains(optionList, "oci")})
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
// never appear as updates. The commit column is last and only read for OCI
// remotes, where it carries the leading hex digits of the image digest; the
// remote stays the final argument, which the fake scripts in the tests rely
// on.
func updateListArgs(user bool, remote string) []string {
	return []string{"remote-ls", "--updates", "--app", "--columns=name,application,version,branch,origin,commit", installationFlag(user), remote}
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
		output, err := runFlatpakCommandCtx(ctx, updateListArgs(user, remote.name)...)
		if err != nil {
			if abortsUpdateQuery(ctx, err) {
				return nil, err
			}
			failed = append(failed, &RemoteError{Remote: remote.name, Installation: installation, Err: err})
			continue
		}
		for _, row := range parseUpdateRows(output, user) {
			// A non-OCI remote (Flathub) is listed exactly as before: no
			// extra query, no filtering.
			if remote.oci {
				installed, err := isInstalledOCIUpdate(ctx, user, row)
				if err != nil {
					return nil, err
				}
				if installed {
					continue
				}
			}
			updates = append(updates, row.info)
		}
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

// updateRow is one parsed row of a remote's update query: the update itself
// plus the commit the remote offers, which is only meaningful for an OCI
// remote. UpdateInfo stays free of it, so the exported contract does not
// change.
type updateRow struct {
	info   UpdateInfo
	commit string
}

// parseUpdateRows parses the tabular output of one remote's update query. The
// commit is the sixth tab-separated field; the whitespace fallback cannot
// tell an empty field from a missing one, so it leaves the commit empty
// (unknown), which keeps the row.
func parseUpdateRows(output string, user bool) []updateRow {
	var rows []updateRow
	installation := installationName(user)

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split by tab (flatpak uses tabs as column separators)
		fields := strings.Split(line, "\t")
		tabbed := true
		if len(fields) < 5 {
			// Try splitting by multiple spaces for systems that might use spaces
			tabbed = false
			fields = strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
		}

		row := updateRow{info: UpdateInfo{Installation: installation}}

		if len(fields) >= 1 {
			row.info.Name = strings.TrimSpace(fields[0])
		}
		if len(fields) >= 2 {
			row.info.ApplicationID = strings.TrimSpace(fields[1])
		}
		if len(fields) >= 3 {
			row.info.NewVersion = strings.TrimSpace(fields[2])
		}
		if len(fields) >= 4 {
			row.info.Branch = strings.TrimSpace(fields[3])
		}
		if len(fields) >= 5 {
			row.info.Origin = strings.TrimSpace(fields[4])
		}
		if tabbed && len(fields) >= 6 {
			row.commit = strings.TrimSpace(fields[5])
		}

		rows = append(rows, row)
	}

	return rows
}

// altIDInfoArgs builds the argument list of the `flatpak info` call that reads
// one installed application's Alt-id. The ref is spelled <application>//<branch>
// so that it names the deployment the update row is about when several
// branches are installed; with no branch, flatpak chooses the installed one.
func altIDInfoArgs(user bool, appID, branch string) []string {
	ref := appID
	if branch != "" {
		ref = appID + "//" + branch
	}
	return []string{"info", installationFlag(user), ref}
}

// parseAltID returns the value of the `Alt-id:` line of `flatpak info` output,
// or "" when the output has none. flatpak prints labels padded on the left, so
// the line's trimmed text is what is matched; the label itself goes through
// gettext, which is why the query runs with LC_ALL=C.
func parseAltID(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, "Alt-id:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// isInstalledImage reports whether a listed update commit is the image that
// installation already holds, named by its Alt-id. An OCI remote names each
// ref by its image digest, which flatpak keeps as the deployment's Alt-id
// after importing the image, so the listed commit is a prefix of the Alt-id
// (flatpak prints the first 12 characters of a commit). Both values may carry
// a "sha256:" prefix. Every unknown case returns false, because wrongly
// dropping a row would hide a real update.
func isInstalledImage(commit, altID string) bool {
	commit = strings.TrimPrefix(strings.TrimSpace(commit), "sha256:")
	altID = strings.TrimPrefix(strings.TrimSpace(altID), "sha256:")
	if len(commit) < 12 || altID == "" {
		return false
	}
	return strings.HasPrefix(altID, commit)
}

// isInstalledOCIUpdate reports whether one row of an OCI remote's update query
// is the image the installation already holds, and so not an update at all. A
// row whose commit is unknown is kept, and a failed `flatpak info` affects only
// its own row: only a cancelled caller context or a missing executable ends the
// whole query, exactly as for a failed remote query (abortsUpdateQuery).
//
// Workaround for Flatpak releases before flatpak/flatpak@a819741d13
// (https://github.com/flatpak/flatpak/pull/6875): `remote-ls --updates`
// compares the remote commit with the installed commit only, while
// `flatpak update` also compares it with the deployment's Alt-id. Remove this
// check once the oldest Flatpak that ChairLift supports includes that commit.
func isInstalledOCIUpdate(ctx context.Context, user bool, row updateRow) (bool, error) {
	if row.commit == "" {
		return false, nil
	}
	// LC_ALL=C keeps the "Alt-id:" label untranslated.
	output, err := runFlatpakCommandEnv(ctx, []string{"LC_ALL=C"}, altIDInfoArgs(user, row.info.ApplicationID, row.info.Branch)...)
	if err != nil {
		if abortsUpdateQuery(ctx, err) {
			return false, err
		}
		log.Printf("flatpak: cannot read the installed image of %s in the %s installation, keeping its update: %v", row.info.ApplicationID, row.info.Installation, err)
		return false, nil
	}
	if isInstalledImage(row.commit, parseAltID(output)) {
		log.Printf("flatpak: %s in the %s installation is the installed OCI image, not an update", row.info.ApplicationID, row.info.Installation)
		return true, nil
	}
	return false, nil
}

// UninstallUnused removes unused Flatpak runtimes and extensions. It passes
// no scope flag on purpose: flatpak parses `uninstall` with
// FLATPAK_BUILTIN_FLAG_ALL_DIRS, so a scope-less `--unused` cleans the user
// installation, the default system installation and every extra system
// installation. A --user or --system flag would narrow that.
func UninstallUnused() (string, error) {
	return runFlatpakCommand("uninstall", "--unused", "-y")
}
