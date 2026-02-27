// Package updex provides an interface to system feature management via updex
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
	pkexecCommand  = "pkexec"
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

// Feature represents a system feature managed by updex
type Feature struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Documentation string   `json:"documentation"`
	Enabled       bool     `json:"enabled"`
	Source        string   `json:"source"`
	Transfers     []string `json:"transfers"`
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

// runPrivilegedCommand executes an updex command via pkexec for privileged operations
func runPrivilegedCommand(ctx context.Context, args ...string) (string, string, error) {
	if dryRun {
		log.Printf("[DRY-RUN] would execute: pkexec %s %v", updexCommand, args)
		return "", "", nil
	}

	fullArgs := append([]string{updexCommand}, args...)
	cmd := exec.CommandContext(ctx, pkexecCommand, fullArgs...)

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
			return "", stderr.String(), &NotFoundError{Message: "pkexec or updex not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", stderr.String(), &Error{Message: fmt.Sprintf("command failed (exit %d): %s", exitErr.ExitCode(), stderr.String())}
		}
		return "", stderr.String(), &Error{Message: err.Error()}
	}

	return stdout.String(), stderr.String(), nil
}

// ListFeatures returns all available features
func ListFeatures(ctx context.Context) ([]Feature, error) {
	output, _, err := runCommand(ctx, "features", "list", "--json")
	if err != nil {
		return nil, err
	}

	var features []Feature
	if err := json.Unmarshal([]byte(output), &features); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse JSON output: %v", err)}
	}

	return features, nil
}

// EnableFeature enables a feature for download
func EnableFeature(ctx context.Context, name string) error {
	_, _, err := runPrivilegedCommand(ctx, "features", "enable", name)
	return err
}

// DisableFeature disables a feature
func DisableFeature(ctx context.Context, name string) error {
	_, _, err := runPrivilegedCommand(ctx, "features", "disable", name)
	return err
}

// UpdateFeatures downloads enabled features
func UpdateFeatures(ctx context.Context) error {
	_, _, err := runPrivilegedCommand(ctx, "features", "update")
	return err
}
