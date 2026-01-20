// Package updex provides an interface to systemd-sysext extension management via updex
package updex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"time"
)

const (
	updexCommand   = "updex"
	DefaultTimeout = 5 * time.Minute
)

var dryRun = false

// SetDryRun enables/disables dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("updex dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// DefaultContext returns a context with the default timeout
func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), DefaultTimeout)
}

// Error represents an updex-related error
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// NotFoundError is returned when updex is not installed
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// Extension represents a systemd-sysext extension version
type Extension struct {
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
	Available bool   `json:"available"`
	Current   bool   `json:"current"`
	Component string `json:"component"`
}

// IsInstalled checks if updex is installed and accessible
func IsInstalled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, updexCommand, "--version")
	err := cmd.Run()
	return err == nil
}

// runCommand executes an updex command and returns stdout, stderr, and any error
func runCommand(ctx context.Context, args ...string) (string, string, error) {
	if dryRun {
		log.Printf("[DRY-RUN] updex: %v", args)
	}

	cmd := exec.CommandContext(ctx, updexCommand, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if stderr.Len() > 0 {
		log.Printf("updex stderr: %s", stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", stderr.String(), &Error{Message: "Command timed out"}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "", stderr.String(), &NotFoundError{Message: "updex not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", stderr.String(), &Error{Message: fmt.Sprintf("command failed (exit %d): %s", exitErr.ExitCode(), stderr.String())}
		}
		return "", stderr.String(), &Error{Message: err.Error()}
	}

	return stdout.String(), stderr.String(), nil
}

// List returns all extensions (installed and available versions)
func List(ctx context.Context) ([]Extension, error) {
	output, _, err := runCommand(ctx, "list", "--json")
	if err != nil {
		return nil, err
	}

	// Parse JSON array output
	var extensions []Extension
	if err := json.Unmarshal([]byte(output), &extensions); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse JSON output: %v", err)}
	}

	return extensions, nil
}

// ListInstalled returns only installed extensions
func ListInstalled(ctx context.Context) ([]Extension, error) {
	extensions, err := List(ctx)
	if err != nil {
		return nil, err
	}

	var installed []Extension
	for _, ext := range extensions {
		if ext.Installed {
			installed = append(installed, ext)
		}
	}
	return installed, nil
}
