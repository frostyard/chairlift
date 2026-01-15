// Package instex provides an interface to discover and install systemd-sysext extensions
package instex

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
	instexCommand  = "instex"
	pkexecCommand  = "pkexec"
	DefaultTimeout = 5 * time.Minute
)

var dryRun = false

// SetDryRun enables/disables dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("instex dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// DefaultContext returns a context with the default timeout
func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), DefaultTimeout)
}

// Error represents an instex-related error
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// NotFoundError is returned when instex is not installed
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// DiscoveredExtension represents an extension available in a repository
type DiscoveredExtension struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

// DiscoverOutput represents the output of instex discover
type DiscoverOutput struct {
	URL        string                `json:"url"`
	Extensions []DiscoveredExtension `json:"extensions"`
}

// IsInstalled checks if instex is installed and accessible
func IsInstalled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, instexCommand, "--version")
	err := cmd.Run()
	return err == nil
}

// runCommand executes an instex command and returns stdout, stderr, and any error
func runCommand(ctx context.Context, args ...string) (string, string, error) {
	if dryRun {
		log.Printf("[DRY-RUN] instex: %v", args)
	}

	cmd := exec.CommandContext(ctx, instexCommand, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if stderr.Len() > 0 {
		log.Printf("instex stderr: %s", stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", stderr.String(), &Error{Message: "Command timed out"}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "", stderr.String(), &NotFoundError{Message: "instex not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", stderr.String(), &Error{Message: fmt.Sprintf("command failed (exit %d): %s", exitErr.ExitCode(), stderr.String())}
		}
		return "", stderr.String(), &Error{Message: err.Error()}
	}

	return stdout.String(), stderr.String(), nil
}

// runPrivilegedCommand executes an instex command via pkexec for privileged operations
func runPrivilegedCommand(ctx context.Context, args ...string) (string, string, error) {
	// Block state-changing operations in dry-run mode
	if dryRun {
		log.Printf("[DRY-RUN] would execute: pkexec %s %v", instexCommand, args)
		return "", "", nil
	}

	fullArgs := append([]string{instexCommand}, args...)
	cmd := exec.CommandContext(ctx, pkexecCommand, fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if stderr.Len() > 0 {
		log.Printf("instex stderr: %s", stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", stderr.String(), &Error{Message: "Command timed out"}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "", stderr.String(), &NotFoundError{Message: "pkexec or instex not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", stderr.String(), &Error{Message: fmt.Sprintf("command failed (exit %d): %s", exitErr.ExitCode(), stderr.String())}
		}
		return "", stderr.String(), &Error{Message: err.Error()}
	}

	return stdout.String(), stderr.String(), nil
}

// Discover retrieves available extensions from a remote repository
func Discover(ctx context.Context, url string) (*DiscoverOutput, error) {
	output, _, err := runCommand(ctx, "discover", url, "--json")
	if err != nil {
		return nil, err
	}

	var result DiscoverOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse discover JSON: %v", err)}
	}

	return &result, nil
}

// Install installs an extension from a remote repository
func Install(ctx context.Context, url, component string) error {
	_, _, err := runPrivilegedCommand(ctx, "install", url, "--component", component)
	return err
}
