// Package bootc provides an interface to bootc-based system updates.
// Status reads call `bootc status --format json` directly (unprivileged).
// Update staging is delegated to the snow-shipped workaround script
// /usr/libexec/bootc-update-stage via pkexec, because bootc's own
// registry-transport pull currently fails on snow images.
package bootc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"
)

const (
	bootcCommand   = "bootc"
	pkexecCommand  = "pkexec"
	DefaultTimeout = 30 * time.Minute
)

var dryRun = false

// SetDryRun enables/disables dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("bootc dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// DefaultContext returns a context with the default 30-minute timeout
func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), DefaultTimeout)
}

// Error represents a bootc-related error
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// NotFoundError is returned when bootc is not installed
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// ImageReference identifies a container image (org.containers.bootc/v1).
type ImageReference struct {
	Image     string `json:"image"`
	Transport string `json:"transport"`
}

// ImageStatus describes a deployed image.
type ImageStatus struct {
	Image       ImageReference `json:"image"`
	Version     string         `json:"version"`
	Timestamp   string         `json:"timestamp"`
	ImageDigest string         `json:"imageDigest"`
}

// Deployment is one entry in status (booted, staged, or rollback).
type Deployment struct {
	Image  *ImageStatus `json:"image"`
	Pinned bool         `json:"pinned"`
}

// ImageRef returns the deployment's image reference, or "" if unknown.
func (d *Deployment) ImageRef() string {
	if d == nil || d.Image == nil {
		return ""
	}
	return d.Image.Image.Image
}

// Version returns the deployment's image version, or "".
func (d *Deployment) Version() string {
	if d == nil || d.Image == nil {
		return ""
	}
	return d.Image.Version
}

// Timestamp returns the deployment's image timestamp, or "".
func (d *Deployment) Timestamp() string {
	if d == nil || d.Image == nil {
		return ""
	}
	return d.Image.Timestamp
}

// Digest returns the deployment's image digest, or "".
func (d *Deployment) Digest() string {
	if d == nil || d.Image == nil {
		return ""
	}
	return d.Image.ImageDigest
}

// SpecInfo is the host spec section of bootc status.
type SpecInfo struct {
	Image *ImageReference `json:"image"`
}

// StatusInfo is the status section of bootc status.
type StatusInfo struct {
	Booted   *Deployment `json:"booted"`
	Staged   *Deployment `json:"staged"`
	Rollback *Deployment `json:"rollback"`
}

// Status is the parsed output of `bootc status --format json`.
type Status struct {
	Spec   SpecInfo   `json:"spec"`
	Status StatusInfo `json:"status"`
}

// Booted reports whether the host is booted from a bootc deployment.
func (s *Status) Booted() bool {
	return s != nil && s.Status.Booted != nil
}

// parseStatus parses `bootc status --format json` output.
func parseStatus(data []byte) (*Status, error) {
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse bootc status JSON: %v", err)}
	}
	return &s, nil
}

// GetStatus returns the current bootc host status. Runs unprivileged.
func GetStatus(ctx context.Context) (*Status, error) {
	cmd := exec.CommandContext(ctx, bootcCommand, "status", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &Error{Message: "bootc status timed out"}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return nil, &NotFoundError{Message: "bootc not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, &Error{Message: fmt.Sprintf("bootc status failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))}
		}
		return nil, &Error{Message: err.Error()}
	}
	return parseStatus(output)
}

// IsBootcBooted reports whether this host is booted from a bootc deployment.
// Note: `bootc status` exits 0 with a null booted entry on non-bootc hosts,
// so the gate is the booted field, not the exit code.
func IsBootcBooted(ctx context.Context) bool {
	status, err := GetStatus(ctx)
	if err != nil {
		return false
	}
	return status.Booted()
}

var (
	bootedOnce   sync.Once
	bootedResult bool
)

// IsBootcBootedCached returns a cached result of IsBootcBooted, running the
// check at most once. Safe to call from view goroutines during async startup.
func IsBootcBootedCached() bool {
	bootedOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		bootedResult = IsBootcBooted(ctx)
	})
	return bootedResult
}
