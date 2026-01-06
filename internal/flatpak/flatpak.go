// Package flatpak provides an interface to the Flatpak package manager
package flatpak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

var (
	dryRun  = false
	timeout = 60 * time.Second
)

// SetDryRun sets the dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("Flatpak dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// Error represents a Flatpak-related error
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
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

// runFlatpakCommand executes a flatpak command and returns the output
func runFlatpakCommand(args ...string) (string, error) {
	if len(args) > 0 && stateChangingCommands[args[0]] && dryRun {
		msg := fmt.Sprintf("[DRY-RUN] Would execute: flatpak %s", strings.Join(args, " "))
		log.Println(msg)
		return msg, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "flatpak", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", &Error{Message: fmt.Sprintf("Command 'flatpak %s' timed out", strings.Join(args, " "))}
		}
		if _, ok := err.(*exec.ExitError); ok {
			return "", &Error{Message: fmt.Sprintf("Flatpak command failed: %s", stderr.String())}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "", &NotFoundError{Message: "Flatpak not found. Please install Flatpak first."}
		}
		return "", &Error{Message: err.Error()}
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
			fields = splitByWhitespace(line)
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

// splitByWhitespace splits a string by whitespace, handling multiple spaces
func splitByWhitespace(s string) []string {
	var result []string
	fields := strings.Fields(s)
	for _, f := range fields {
		if f != "" {
			result = append(result, f)
		}
	}
	return result
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

// ListUpdates returns available updates for Flatpak applications
func ListUpdates(user bool) ([]UpdateInfo, error) {
	args := []string{"remote-ls", "--updates", "--columns=name,application,version,branch,origin"}
	if user {
		args = append(args, "--user")
	} else {
		args = append(args, "--system")
	}

	output, err := runFlatpakCommand(args...)
	if err != nil {
		return nil, err
	}

	return parseUpdateList(output, user)
}

// parseUpdateList parses the tabular output from flatpak remote-ls --updates
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
			fields = splitByWhitespace(line)
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

// MarshalJSON implements json.Marshaler for Application
func (a Application) MarshalJSON() ([]byte, error) {
	type Alias Application
	return json.Marshal(&struct {
		Alias
	}{
		Alias: (Alias)(a),
	})
}

// UninstallUnused removes unused Flatpak runtimes and extensions
func UninstallUnused() (string, error) {
	return runFlatpakCommand("uninstall", "--unused", "-y")
}
