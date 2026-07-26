// Package homebrew provides an interface to the Homebrew package manager
package homebrew

import (
	"bytes"
	"context"
	"encoding/json"
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
	// readTimeout bounds read-only brew commands (listing, searching, info).
	readTimeout = 30 * time.Second
	// mutationTimeout bounds state-changing brew commands, which may download
	// and build packages and therefore need a far larger budget.
	mutationTimeout = 30 * time.Minute
	// waitDelay bounds how long Wait blocks after the process group has been
	// signalled. brew's helpers (git, curl, download workers) inherit the
	// stdout/stderr pipes, so a straggler could otherwise hold Wait open
	// forever even though the command itself is gone.
	waitDelay = 5 * time.Second
)

var dryRun = false

// SetDryRun sets the dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("Homebrew dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// Error represents a Homebrew-related error. Err, when non-nil, carries the
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

// NotFoundError is returned when Homebrew is not installed
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// Package represents an installed Homebrew package
type Package struct {
	Name               string   `json:"name"`
	Version            string   `json:"version"`
	InstalledOnRequest bool     `json:"installed_on_request"`
	Pinned             bool     `json:"pinned"`
	Outdated           bool     `json:"outdated"`
	Dependencies       []string `json:"dependencies,omitempty"`
}

// SearchResult represents a search result
type SearchResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
}

// stateChangingCommands are commands that modify system state
var stateChangingCommands = map[string]bool{
	"install":   true,
	"uninstall": true,
	"remove":    true,
	"upgrade":   true,
	"update":    true,
	"pin":       true,
	"unpin":     true,
	"bundle":    true,
	"cleanup":   true,
	"trust":     true,
}

// commandTimeout returns the timeout class for a brew invocation: the
// mutation timeout for state-changing commands, the read timeout otherwise.
func commandTimeout(args []string) time.Duration {
	if len(args) > 0 && stateChangingCommands[args[0]] {
		return mutationTimeout
	}
	return readTimeout
}

// runBrewCommand executes a brew command and returns the output
func runBrewCommand(args ...string) (string, error) {
	if len(args) > 0 && stateChangingCommands[args[0]] && dryRun {
		msg := fmt.Sprintf("[DRY-RUN] Would execute: brew %s", strings.Join(args, " "))
		log.Println(msg)
		return msg, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout(args))
	defer cancel()

	return runBrewCommandAt(ctx, "brew", args...)
}

// runBrewCommandAt runs exe with args under ctx and returns its stdout. The
// executable and context are parameters so tests can drive a fake script and
// control the deadline; runBrewCommand is the only production caller and
// always passes "brew".
//
// The command runs in its own process group and cancellation signals the
// whole group, so brew's helper processes (git, curl, download workers) die
// with it rather than being orphaned. cmd.Run still reaps the child.
func runBrewCommandAt(ctx context.Context, exe string, args ...string) (string, error) {
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
			if isUntrustedTapMessage(stderr.String()) {
				return "", &UntrustedTapError{Message: fmt.Sprintf("Brew command failed: %s", stderr.String())}
			}
			return "", &Error{Message: fmt.Sprintf("Brew command failed: %s", stderr.String()), Err: err}
		}
		// exec.ErrNotFound covers a bare name missing from $PATH;
		// fs.ErrNotExist covers an explicit path that does not exist.
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return "", &NotFoundError{Message: "Homebrew not found. Please install Homebrew first."}
		}
		return "", &Error{Message: err.Error(), Err: err}
	}

	return stdout.String(), nil
}

// IsInstalled checks if Homebrew is installed and accessible
func IsInstalled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "brew", "--version")
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

// ListInstalledFormulae returns all installed formulae
func ListInstalledFormulae() ([]Package, error) {
	output, err := runBrewCommand("info", "--installed", "--json=v2", "--formula")
	if err != nil {
		return nil, err
	}

	return parsePackagesJSON(output, true)
}

// ListInstalledCasks returns all installed casks
func ListInstalledCasks() ([]Package, error) {
	output, err := runBrewCommand("info", "--installed", "--json=v2", "--cask")
	if err != nil {
		return nil, err
	}

	return parsePackagesJSON(output, false)
}

// parsePackagesJSON parses the JSON output from brew info
func parsePackagesJSON(jsonData string, isFormula bool) ([]Package, error) {
	var data struct {
		Formulae []struct {
			Name     string `json:"name"`
			Versions struct {
				Stable string `json:"stable"`
			} `json:"versions"`
			Installed []struct {
				Version            string `json:"version"`
				InstalledOnRequest bool   `json:"installed_on_request"`
			} `json:"installed"`
			Pinned   bool `json:"pinned"`
			Outdated bool `json:"outdated"`
		} `json:"formulae"`
		Casks []struct {
			Token     string `json:"token"`
			Version   string `json:"version"`
			Installed string `json:"installed"`
			Outdated  bool   `json:"outdated"`
		} `json:"casks"`
	}

	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return nil, &Error{Message: fmt.Sprintf("Failed to parse JSON: %v", err)}
	}

	var packages []Package

	if isFormula {
		for _, f := range data.Formulae {
			if len(f.Installed) == 0 {
				continue
			}
			packages = append(packages, Package{
				Name:               f.Name,
				Version:            f.Installed[0].Version,
				InstalledOnRequest: f.Installed[0].InstalledOnRequest,
				Pinned:             f.Pinned,
				Outdated:           f.Outdated,
			})
		}
	} else {
		for _, c := range data.Casks {
			packages = append(packages, Package{
				Name:     c.Token,
				Version:  c.Installed,
				Outdated: c.Outdated,
			})
		}
	}

	return packages, nil
}

// ListOutdated returns all outdated packages
func ListOutdated() ([]Package, error) {
	output, err := runBrewCommand("outdated", "--json=v2")
	if err != nil {
		return nil, err
	}

	var data struct {
		Formulae []struct {
			Name              string   `json:"name"`
			InstalledVersions []string `json:"installed_versions"`
			CurrentVersion    string   `json:"current_version"`
			Pinned            bool     `json:"pinned"`
		} `json:"formulae"`
		Casks []struct {
			Name              string   `json:"name"`
			InstalledVersions []string `json:"installed_versions"`
			CurrentVersion    string   `json:"current_version"`
		} `json:"casks"`
	}

	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, &Error{Message: fmt.Sprintf("Failed to parse JSON: %v", err)}
	}

	var packages []Package
	for _, f := range data.Formulae {
		packages = append(packages, Package{
			Name:     f.Name,
			Version:  strings.Join(f.InstalledVersions, ", "),
			Outdated: true,
			Pinned:   f.Pinned,
		})
	}
	for _, c := range data.Casks {
		packages = append(packages, Package{
			Name:     c.Name,
			Version:  strings.Join(c.InstalledVersions, ", "),
			Outdated: true,
		})
	}

	return packages, nil
}

// Search searches for formulae matching the query
func Search(query string) ([]SearchResult, error) {
	output, err := runBrewCommand("search", "--formula", query)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "==>") {
			results = append(results, SearchResult{Name: line})
		}
	}

	return results, nil
}

// Install installs a package
func Install(name string, isCask bool) error {
	args := []string{"install"}
	if isCask {
		args = append(args, "--cask")
	}
	args = append(args, name)

	_, err := runBrewCommand(args...)
	return err
}

// Uninstall removes a package
func Uninstall(name string, isCask bool) error {
	args := []string{"uninstall"}
	if isCask {
		args = append(args, "--cask")
	}
	args = append(args, name)

	_, err := runBrewCommand(args...)
	return err
}

// Upgrade upgrades a package or all packages
func Upgrade(name string) error {
	args := []string{"upgrade"}
	if name != "" {
		args = append(args, name)
	}

	_, err := runBrewCommand(args...)
	return err
}

// Update updates Homebrew itself
func Update() error {
	_, err := runBrewCommand("update")
	return err
}

// Pin pins a package
func Pin(name string) error {
	_, err := runBrewCommand("pin", name)
	return err
}

// Unpin unpins a package
func Unpin(name string) error {
	_, err := runBrewCommand("unpin", name)
	return err
}

// BundleDump dumps installed packages to a Brewfile
func BundleDump(path string, force bool) error {
	args := []string{"bundle", "dump"}
	if path != "" {
		args = append(args, "--file="+path)
	}
	if force {
		args = append(args, "--force")
	}

	_, err := runBrewCommand(args...)
	return err
}

// BundleInstall installs packages from a Brewfile
func BundleInstall(path string) error {
	args := []string{"bundle", "install"}
	if path != "" {
		args = append(args, "--file="+path)
	}

	_, err := runBrewCommand(args...)
	return err
}

// Cleanup removes old versions, outdated downloads, and clears cache
func Cleanup() (string, error) {
	return runBrewCommand("cleanup")
}
