// Package nbc provides an interface to the nbc bootc container installer
package nbc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/frostyard/nbc/pkg/types"
)

const (
	nbcCommand     = "nbc"
	pkexecCommand  = "pkexec"
	DefaultTimeout = 30 * time.Minute
)

var dryRun = false

// SetDryRun enables/disables dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("nbc dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// DefaultContext returns a context with the default 30-minute timeout
func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), DefaultTimeout)
}

// Error represents an nbc-related error
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// NotFoundError is returned when nbc is not installed
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// --- Re-export types from nbc/pkg/types for convenience ---

type (
	// StatusOutput represents nbc status output
	StatusOutput = types.StatusOutput
	// UpdateCheck represents update availability info
	UpdateCheck = types.UpdateCheck
	// StagedUpdate represents a staged update in cache
	StagedUpdate = types.StagedUpdate
	// UpdateCheckOutput from `nbc update --check`
	UpdateCheckOutput = types.UpdateCheckOutput
	// ListOutput from `nbc list`
	ListOutput = types.ListOutput
	// DiskOutput represents a disk
	DiskOutput = types.DiskOutput
	// PartitionOutput represents a partition
	PartitionOutput = types.PartitionOutput
	// CacheListOutput from `nbc cache list`
	CacheListOutput = types.CacheListOutput
	// CachedImageMetadata represents a cached image
	CachedImageMetadata = types.CachedImageMetadata
	// DownloadOutput from `nbc download`
	DownloadOutput = types.DownloadOutput
	// ValidateOutput from `nbc validate`
	ValidateOutput = types.ValidateOutput
	// ProgressEvent represents a streaming progress event
	ProgressEvent = types.ProgressEvent
	// EventType represents the type of progress event
	EventType = types.EventType
)

// Re-export event type constants
const (
	EventTypeStep     = types.EventTypeStep
	EventTypeProgress = types.EventTypeProgress
	EventTypeMessage  = types.EventTypeMessage
	EventTypeWarning  = types.EventTypeWarning
	EventTypeError    = types.EventTypeError
	EventTypeComplete = types.EventTypeComplete
)

// --- Option Structs ---

// UpdateOptions for the Update command
type UpdateOptions struct {
	Image        string   // Container image reference (optional, uses saved config)
	Device       string   // Target disk device (optional, auto-detected)
	Force        bool     // Force reinstall even if up-to-date
	DownloadOnly bool     // Download without applying
	LocalImage   bool     // Apply from staged cache
	Auto         bool     // Use staged if available, else pull
	SkipPull     bool     // Skip pulling image
	KernelArgs   []string // Additional kernel arguments
}

// InstallOptions for the Install command
type InstallOptions struct {
	Image       string   // Container image reference
	LocalImage  string   // Local image digest (alternative to Image)
	Device      string   // Target disk device (required)
	Filesystem  string   // Filesystem type: ext4, btrfs (default: btrfs)
	Encrypt     bool     // Enable LUKS encryption
	Passphrase  string   // LUKS passphrase
	KeyFile     string   // Path to LUKS passphrase file
	TPM2        bool     // Enroll TPM2 for auto-unlock
	KernelArgs  []string // Additional kernel arguments
	RootPwdFile string   // Path to root password file
	SkipPull    bool     // Skip pulling image
}

// DownloadOptions for the Download command
type DownloadOptions struct {
	Image      string // Container image reference
	ForInstall bool   // Save to staged-install cache
	ForUpdate  bool   // Save to staged-update cache
}

// --- State-changing commands (respect dry-run) ---

// stateChangingCommands maps command names to whether they change state
var stateChangingCommands = map[string]bool{
	"install":  true,
	"update":   true,
	"download": true,
}

// stateChangingCacheSubcommands maps cache subcommand names to whether they change state
var stateChangingCacheSubcommands = map[string]bool{
	"clear":  true,
	"remove": true,
}

func isStateChanging(args []string) bool {
	if len(args) == 0 {
		return false
	}
	// Check top-level commands
	if stateChangingCommands[args[0]] {
		return true
	}
	// Handle cache subcommands specially
	if args[0] == "cache" && len(args) > 1 {
		return stateChangingCacheSubcommands[args[1]]
	}
	return false
}

// --- Command Execution ---

// runNbcCommandDirect executes nbc directly without pkexec (for read-only commands)
func runNbcCommandDirect(ctx context.Context, args ...string) (string, string, error) {
	if dryRun {
		log.Printf("[DRY-RUN] nbc (read-only): %v", args)
	}

	// Add --json flag to all commands
	args = append([]string{"--json"}, args...)

	cmd := exec.CommandContext(ctx, nbcCommand, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Log stderr for debugging
	if stderr.Len() > 0 {
		log.Printf("nbc stderr: %s", stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", stderr.String(), &Error{Message: "Command timed out"}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "", stderr.String(), &NotFoundError{Message: "nbc not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", stderr.String(), &Error{Message: fmt.Sprintf("nbc failed (exit %d): %s", exitErr.ExitCode(), stderr.String())}
		}
		return "", stderr.String(), &Error{Message: err.Error()}
	}

	return stdout.String(), stderr.String(), nil
}

// runNbcCommand executes nbc via pkexec and returns stdout, stderr, and error
func runNbcCommand(ctx context.Context, args ...string) (string, string, error) {
	// Add --json flag to all commands
	args = append([]string{"--json"}, args...)

	// Add --dry-run for state-changing commands if in dry-run mode
	if dryRun && isStateChanging(args) {
		args = append(args, "--dry-run")
	}

	fullArgs := append([]string{nbcCommand}, args...)
	cmd := exec.CommandContext(ctx, pkexecCommand, fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Log stderr for debugging
	if stderr.Len() > 0 {
		log.Printf("nbc stderr: %s", stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", stderr.String(), &Error{Message: "Command timed out after 30 minutes"}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "", stderr.String(), &NotFoundError{Message: "nbc or pkexec not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", stderr.String(), &Error{Message: fmt.Sprintf("nbc failed (exit %d): %s", exitErr.ExitCode(), stderr.String())}
		}
		return "", stderr.String(), &Error{Message: err.Error()}
	}

	return stdout.String(), stderr.String(), nil
}

// runNbcCommandStreaming executes nbc and streams JSON Lines to a channel
func runNbcCommandStreaming(ctx context.Context, progressCh chan<- ProgressEvent, args ...string) error {
	defer close(progressCh)

	// Add --json flag to all commands
	args = append([]string{"--json"}, args...)

	// Add --dry-run for state-changing commands if in dry-run mode
	if dryRun && isStateChanging(args) {
		args = append(args, "--dry-run")
	}

	fullArgs := append([]string{nbcCommand}, args...)
	cmd := exec.CommandContext(ctx, pkexecCommand, fullArgs...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &Error{Message: fmt.Sprintf("failed to create stdout pipe: %v", err)}
	}

	// Capture stderr for logging
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return &NotFoundError{Message: "nbc or pkexec not found"}
		}
		return &Error{Message: fmt.Sprintf("failed to start command: %v", err)}
	}

	// Read JSON Lines from stdout
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()

		var event ProgressEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Log malformed lines but continue
			log.Printf("nbc: non-JSON output: %s", line)
			continue
		}

		select {
		case progressCh <- event:
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return ctx.Err()
		}
	}

	// Log stderr
	if stderr.Len() > 0 {
		log.Printf("nbc stderr: %s", stderr.String())
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &Error{Message: "Command timed out after 30 minutes"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &Error{Message: fmt.Sprintf("nbc failed (exit %d): %s", exitErr.ExitCode(), stderr.String())}
		}
		return &Error{Message: err.Error()}
	}

	return nil
}

// --- Status and Information ---

// IsInstalled checks if nbc is installed and accessible
func IsInstalled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, nbcCommand, "--version")
	err := cmd.Run()
	return err == nil
}

// GetStatus returns the current system status
func GetStatus(ctx context.Context) (*StatusOutput, error) {
	output, _, err := runNbcCommandDirect(ctx, "status")
	if err != nil {
		return nil, err
	}

	var status StatusOutput
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse status JSON: %v", err)}
	}

	return &status, nil
}

// ListDisks returns all available physical disks
func ListDisks(ctx context.Context) (*ListOutput, error) {
	output, _, err := runNbcCommandDirect(ctx, "list")
	if err != nil {
		return nil, err
	}

	var result ListOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse disk list JSON: %v", err)}
	}

	return &result, nil
}

// CheckUpdate checks if a system update is available
func CheckUpdate(ctx context.Context) (*UpdateCheckOutput, error) {
	output, _, err := runNbcCommandDirect(ctx, "update", "--check")
	if err != nil {
		return nil, err
	}

	var result UpdateCheckOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse update check JSON: %v", err)}
	}

	return &result, nil
}

// Validate validates a disk for bootc installation
func Validate(ctx context.Context, device string) (*ValidateOutput, error) {
	output, _, err := runNbcCommand(ctx, "validate", "--device", device)
	if err != nil {
		// For validate, errors are expected - try to parse JSON error response
		var result ValidateOutput
		if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr == nil {
			return &result, nil
		}
		return nil, err
	}

	var result ValidateOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse validate JSON: %v", err)}
	}

	return &result, nil
}

// --- Cache Management ---

// ListCachedImages lists cached images
// cacheType: "install" or "update"
func ListCachedImages(ctx context.Context, cacheType string) (*CacheListOutput, error) {
	var flag string
	switch cacheType {
	case "install":
		flag = "--install-images"
	case "update":
		flag = "--update-images"
	default:
		return nil, &Error{Message: "cacheType must be 'install' or 'update'"}
	}

	output, _, err := runNbcCommand(ctx, "cache", "list", flag)
	if err != nil {
		return nil, err
	}

	var result CacheListOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse cache list JSON: %v", err)}
	}

	return &result, nil
}

// RemoveCachedImage removes a cached image by digest
// cacheType: "install", "update", or "" for auto-detect
func RemoveCachedImage(ctx context.Context, digest string, cacheType string) error {
	args := []string{"cache", "remove", digest}
	if cacheType != "" {
		args = append(args, "--type", cacheType)
	}

	_, _, err := runNbcCommand(ctx, args...)
	return err
}

// ClearCache clears all cached images
// cacheType: "install" or "update"
func ClearCache(ctx context.Context, cacheType string) error {
	var flag string
	switch cacheType {
	case "install":
		flag = "--install"
	case "update":
		flag = "--update"
	default:
		return &Error{Message: "cacheType must be 'install' or 'update'"}
	}

	_, _, err := runNbcCommand(ctx, "cache", "clear", flag)
	return err
}

// --- Streaming Operations ---

// Update updates the system to a new container image
// Streams progress events to progressCh (channel is closed when done)
func Update(ctx context.Context, opts UpdateOptions, progressCh chan<- ProgressEvent) error {
	args := []string{"update"}

	if dryRun {
		log.Printf("[DRY-RUN] nbc update with options: %+v", opts)
		args = append(args, "--dry-run")
	}
	if opts.Image != "" {
		args = append(args, "--image", opts.Image)
	}
	if opts.Device != "" {
		args = append(args, "--device", opts.Device)
	}
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.DownloadOnly {
		args = append(args, "--download-only")
	}
	if opts.LocalImage {
		args = append(args, "--local-image")
	}
	if opts.Auto {
		args = append(args, "--auto")
	}
	if opts.SkipPull {
		args = append(args, "--skip-pull")
	}
	for _, karg := range opts.KernelArgs {
		args = append(args, "--karg", karg)
	}

	return runNbcCommandStreaming(ctx, progressCh, args...)
}

// Download downloads a container image to cache
// Streams progress events to progressCh (channel is closed when done)
func Download(ctx context.Context, opts DownloadOptions, progressCh chan<- ProgressEvent) error {
	args := []string{"download"}

	if dryRun {
		log.Printf("[DRY-RUN] nbc download with options: %+v", opts)
		args = append(args, "--dry-run")

	}

	if opts.Image != "" {
		args = append(args, "--image", opts.Image)
	}
	if opts.ForInstall {
		args = append(args, "--for-install")
	}
	if opts.ForUpdate {
		args = append(args, "--for-update")
	}

	if !opts.ForInstall && !opts.ForUpdate {
		return &Error{Message: "either ForInstall or ForUpdate must be true"}
	}

	return runNbcCommandStreaming(ctx, progressCh, args...)
}
