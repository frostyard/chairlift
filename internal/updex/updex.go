// Package updex provides an interface to system feature management via the updex API.
// Read operations use the updex Go library directly. Write operations that require
// root are delegated to the chairlift-updex-helper binary via pkexec.
package updex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/updexhelper"
	updexapi "github.com/frostyard/updex/v2/updex"
)

const (
	// HelperPath is the fixed, absolute installed path of the privileged
	// updex helper binary. It must match the
	// org.freedesktop.policykit.exec.path annotation on all three actions in
	// data/org.frostyard.ChairLift.updex.policy exactly. Each action also
	// selects one supported helper command through the exec.argv1 annotation.
	// A path mismatch (e.g. a bare, $PATH-resolved name) makes pkexec fall
	// back to the generic, more restrictive
	// org.freedesktop.policykit.pkexec.run-program action instead. This
	// constant is installed at $(PREFIX)/bin/chairlift-updex-helper by the
	// Makefile, which requires PREFIX=/usr (the default) to match.
	HelperPath = "/usr/bin/chairlift-updex-helper"

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

// ActivationIncompleteError is returned by UpdateFeatures when the helper
// exits with updexhelper.ExitActivationIncomplete: the feature links are in
// place and the extensions merged, but the activation did not finish, and a
// reboot finishes it. It is a failure to an older caller, which only sees a
// non-nil error.
type ActivationIncompleteError struct {
	Message string
}

func (e *ActivationIncompleteError) Error() string {
	return e.Message
}

// ResultsUnreadableError is returned by UpdateFeatures when the helper exited
// 0 but its stdout held no readable results array. The update outcome is
// unknown: the caller must not report success.
type ResultsUnreadableError struct {
	Message string
}

func (e *ResultsUnreadableError) Error() string {
	return e.Message
}

// NotFoundError is returned when updex features are not configured
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// Type aliases to the updex API types
type (
	Feature      = updexapi.FeatureInfo
	CheckResult  = updexapi.CheckResult
	FeatureCheck = updexapi.CheckFeaturesResult
	// FeatureUpdate is one feature's result of the update command.
	FeatureUpdate = updexapi.UpdateFeaturesResult
	// TransferUpdate is one transfer's result inside a FeatureUpdate.
	TransferUpdate = updexapi.UpdateResult
)

// Singleton API client
var (
	clientOnce sync.Once
	apiClient  *updexapi.Client
)

func getClient() *updexapi.Client {
	clientOnce.Do(func() {
		apiClient = updexapi.NewClient(updexapi.ClientConfig{})
	})
	return apiClient
}

// IsInstalled checks if updex features are configured on this system
func IsInstalled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := getClient().Features(ctx)
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

// ListFeatures returns all available features
func ListFeatures(ctx context.Context) ([]Feature, error) {
	features, err := getClient().Features(ctx)
	if err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to list features: %v", err)}
	}
	return features, nil
}

// CheckFeatures checks enabled features for available updates
func CheckFeatures(ctx context.Context) ([]FeatureCheck, error) {
	checks, err := getClient().CheckFeatures(ctx, updexapi.CheckFeaturesOptions{})
	checks, componentErrors, err := partitionFeatureChecks(checks, err)
	for _, componentErr := range componentErrors {
		log.Printf(
			"Feature update component check failed: feature=%q component=%q: %s",
			componentErr.Feature,
			componentErr.Component,
			componentErr.Message,
		)
	}
	if err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to check features: %v", err)}
	}
	return checks, nil
}

type componentCheckError struct {
	Feature   string
	Component string
	Message   string
}

// partitionFeatureChecks preserves Updex v1's partial-failure behavior while
// retaining v2's structured component errors for diagnostics. A request-level
// error is returned only when no component result explains it.
func partitionFeatureChecks(checks []FeatureCheck, err error) ([]FeatureCheck, []componentCheckError, error) {
	filtered := make([]FeatureCheck, 0, len(checks))
	var componentErrors []componentCheckError

	for _, check := range checks {
		results := make([]CheckResult, 0, len(check.Results))
		for _, result := range check.Results {
			if result.Error == "" {
				results = append(results, result)
				continue
			}
			componentErrors = append(componentErrors, componentCheckError{
				Feature:   check.Feature,
				Component: result.Component,
				Message:   result.Error,
			})
		}
		check.Results = results
		filtered = append(filtered, check)
	}

	if len(componentErrors) == 0 {
		return filtered, nil, err
	}
	return filtered, componentErrors, nil
}

// EnableFeature enables a feature for download
func EnableFeature(ctx context.Context, name string) error {
	_, _, err := runHelper(ctx, pkexecCommand, updexhelper.CommandEnableFeature, name)
	return err
}

// DisableFeature disables a feature
func DisableFeature(ctx context.Context, name string) error {
	_, _, err := runHelper(ctx, pkexecCommand, updexhelper.CommandDisableFeature, name)
	return err
}

// UpdateFeatures downloads enabled features and returns the helper's
// per-feature results. A nil error does not mean every feature updated: a
// feature held by the next-boot image gate or a rollback record reports so in
// its results (see SummarizeUpdate).
//
// The helper prints its results on stdout and exits 0 on success. On failure
// the results still come back next to the error when the helper printed them:
// a *ActivationIncompleteError (exit 3) means the extensions merged but
// activation did not finish; any other exit gives *Error. A helper from an
// older ChairLift prints nothing on failure, so the results are nil then.
// When the helper exits 0 but stdout has no readable results array, the
// result is nil and a *ResultsUnreadableError (see parseUpdateResults for the
// accepted forms). Under dry-run no helper runs, and the results and the
// error are both nil.
func UpdateFeatures(ctx context.Context) ([]FeatureUpdate, error) {
	return updateFeatures(ctx, pkexecCommand)
}

// updateFeatures is UpdateFeatures with the pkexec binary as a test seam.
//
// The helper error always wins: with a non-nil error it is returned with
// whatever results parsed (or nil). When the helper exits 0 and stdout holds
// no readable results array, the update outcome is unknown, so the result is
// nil and a *ResultsUnreadableError, never a success.
func updateFeatures(ctx context.Context, pkexecPath string) ([]FeatureUpdate, error) {
	stdout, _, err := runHelper(ctx, pkexecPath, updexhelper.CommandUpdate)
	if dryRun {
		return nil, err
	}
	results, parseErr := parseUpdateResults(stdout)
	if err != nil {
		if parseErr != nil {
			log.Printf("updex helper: cannot read update results of a failed run: %v", parseErr)
		}
		return results, err
	}
	if parseErr != nil {
		log.Printf("updex helper: cannot read update results: %v", parseErr)
		return nil, &ResultsUnreadableError{Message: fmt.Sprintf("cannot read update results: %v", parseErr)}
	}
	return results, nil
}

// parseUpdateResults decodes the helper's JSON array of feature results. It
// decodes the whole trimmed stdout first. When that fails it tries the last
// non-empty line: a helper from an older ChairLift could have child-process
// text before its JSON, which json.Encoder ends with a newline. Empty stdout
// and stdout without a decodable array give an error and nil results.
func parseUpdateResults(stdout string) ([]FeatureUpdate, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, errors.New("helper printed no results")
	}
	var results []FeatureUpdate
	err := json.Unmarshal([]byte(trimmed), &results)
	if err == nil && results != nil {
		return results, nil
	}
	if err == nil {
		err = errors.New("helper printed null instead of a results array")
	}
	if idx := strings.LastIndexByte(trimmed, '\n'); idx >= 0 {
		var last []FeatureUpdate
		if lastErr := json.Unmarshal([]byte(strings.TrimSpace(trimmed[idx+1:])), &last); lastErr == nil && last != nil {
			return last, nil
		}
	}
	return nil, err
}

// runHelper executes HelperPath via pkexec for privileged operations. pkexecPath
// is the pkexec binary to invoke — always pkexecCommand in production, but an
// explicit parameter (mirroring internal/stageexec.Run's executable seam) so
// tests can substitute a fake pkexec stand-in without invoking the real
// pkexec/polkit stack or requiring root. HelperPath itself is never overridden: it is the fixed
// absolute path that must match the policy's exec.path annotation, so tests
// assert it by inspecting the fake pkexec's captured argv rather than by
// substituting a different one in.
//
// When the helper exits non-zero, the captured stdout is still returned next
// to the error: the update command prints its results also on failure. Exit
// code updexhelper.ExitActivationIncomplete gives *ActivationIncompleteError,
// any other code *Error.
func runHelper(ctx context.Context, pkexecPath string, args ...string) (string, string, error) {
	if dryRun {
		args = append(args, "--dry-run")
		log.Printf("[DRY-RUN] would execute: %s %s %v", pkexecPath, HelperPath, args)
		return "", "", nil
	}

	fullArgs := append([]string{HelperPath}, args...)
	cmd := exec.CommandContext(ctx, pkexecPath, fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if stderr.Len() > 0 {
		log.Printf("updex helper stderr: %s", stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", stderr.String(), &Error{Message: "command timed out"}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "", stderr.String(), &NotFoundError{Message: "pkexec or chairlift-updex-helper not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			msg := fmt.Sprintf("command failed (exit %d): %s", exitErr.ExitCode(), stderr.String())
			if exitErr.ExitCode() == updexhelper.ExitActivationIncomplete {
				return stdout.String(), stderr.String(), &ActivationIncompleteError{Message: msg}
			}
			return stdout.String(), stderr.String(), &Error{Message: msg}
		}
		return "", stderr.String(), &Error{Message: err.Error()}
	}

	return stdout.String(), stderr.String(), nil
}
