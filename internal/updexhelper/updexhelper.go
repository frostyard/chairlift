// Package updexhelper holds the argv-parsing and Options-building logic for
// cmd/chairlift-updex-helper, the privileged helper binary invoked via
// pkexec to perform updex write operations. It is deliberately free of any
// puregotk/GTK import (only stdlib plus github.com/frostyard/updex/v2/updex and
// github.com/frostyard/updex/v2/sysext),
// so its logic can be unit-tested on a headless host — a test binary for a
// package that imports puregotk panics while resolving GTK/graphene shared
// libraries at package init, before any test function runs. See
// docs/agents/skills/gtk-headless-tests.md.
//
// cmd/chairlift-updex-helper/main.go is reduced to dispatching the validated
// Invocation returned here, then passing the per-subcommand Options struct to
// the corresponding updex client call.
package updexhelper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/frostyard/updex/v2/sysext"
	"github.com/frostyard/updex/v2/updex"
)

const (
	CommandEnableFeature  = "enable-feature"
	CommandDisableFeature = "disable-feature"
	CommandUpdate         = "update"
)

// Exit codes of cmd/chairlift-updex-helper. Exit 0 means success. The GUI
// (internal/updex) maps these codes to its error types, and an older GUI
// treats every non-zero exit as a failure, so each new code must stay a
// failure code. pkexec passes the helper's exit code through unchanged.
const (
	// ExitFailure is any failure: invalid argv, an SDK error, an encode error.
	ExitFailure = 1
	// ExitActivationIncomplete means the update's links and extensions are
	// in place and merged, but the activation did not finish
	// (sysext.ErrActivationIncomplete); a reboot finishes it.
	ExitActivationIncomplete = 3
)

// Invocation is a validated privileged-helper command. PolicyKit selects an
// action from the executable path and first argument; ParseInvocation is the
// second boundary and rejects every argv shape the helper does not support.
type Invocation struct {
	Command string
	Feature string
	DryRun  bool
}

// SupportedCommands returns the complete first-argument set accepted by the
// privileged helper. The returned slice is a fresh value so callers cannot
// mutate the package's command surface.
func SupportedCommands() []string {
	return []string{CommandEnableFeature, CommandDisableFeature, CommandUpdate}
}

// ParseInvocation accepts only the three argv shapes ChairLift emits:
//
//	enable-feature <name> [--dry-run]
//	disable-feature <name> [--dry-run]
//	update [--dry-run]
//
// pkexec does not validate arguments after selecting an action, so the
// privileged helper must reject extra, misplaced, and unknown arguments.
func ParseInvocation(args []string) (Invocation, error) {
	if len(args) == 0 {
		return Invocation{}, fmt.Errorf("usage: chairlift-updex-helper <command> [args...]")
	}

	switch args[0] {
	case CommandEnableFeature, CommandDisableFeature:
		if len(args) != 2 && len(args) != 3 {
			return Invocation{}, fmt.Errorf("usage: chairlift-updex-helper %s <name> [--dry-run]", args[0])
		}
		if args[1] == "" || args[1] == "--dry-run" {
			return Invocation{}, fmt.Errorf("usage: chairlift-updex-helper %s <name> [--dry-run]", args[0])
		}
		dryRun := len(args) == 3
		if dryRun && args[2] != "--dry-run" {
			return Invocation{}, fmt.Errorf("usage: chairlift-updex-helper %s <name> [--dry-run]", args[0])
		}
		return Invocation{Command: args[0], Feature: args[1], DryRun: dryRun}, nil
	case CommandUpdate:
		if len(args) > 2 || (len(args) == 2 && args[1] != "--dry-run") {
			return Invocation{}, fmt.Errorf("usage: chairlift-updex-helper update [--dry-run]")
		}
		return Invocation{Command: CommandUpdate, DryRun: len(args) == 2}, nil
	default:
		return Invocation{}, fmt.Errorf("unknown command: %s", args[0])
	}
}

// EnableOptions builds the updex.EnableFeatureOptions for the
// enable-feature subcommand, with DryRun set to exactly dryRun.
func EnableOptions(dryRun bool) updex.EnableFeatureOptions {
	return updex.EnableFeatureOptions{DryRun: dryRun}
}

// DisableOptions builds the updex.DisableFeatureOptions for the
// disable-feature subcommand, with DryRun set to exactly dryRun.
func DisableOptions(dryRun bool) updex.DisableFeatureOptions {
	return updex.DisableFeatureOptions{DryRun: dryRun}
}

// UpdateOptions builds the updex.UpdateFeaturesOptions for the update
// subcommand, with DryRun set to exactly dryRun. Previously main.go passed
// a zero-value updex.UpdateFeaturesOptions{} here, silently dropping the
// parsed --dry-run flag for this one subcommand.
func UpdateOptions(dryRun bool) updex.UpdateFeaturesOptions {
	return updex.UpdateFeaturesOptions{DryRun: dryRun}
}

// WriteResult serializes a successful SDK result and preserves every SDK
// error for the helper's fatal path, including final sysext refresh failures.
func WriteResult(w io.Writer, value any, err error) error {
	if err != nil {
		return err
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return nil
}

// ExitCode maps an SDK error to the helper's process exit code: 0 for nil,
// ExitActivationIncomplete when err wraps sysext.ErrActivationIncomplete
// (through %w and errors.Join alike), and ExitFailure otherwise.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, sysext.ErrActivationIncomplete):
		return ExitActivationIncomplete
	default:
		return ExitFailure
	}
}

// WriteUpdateResult serializes the update command's results. Unlike
// WriteResult it also writes non-nil results when err is non-nil: updex
// returns the per-feature results together with an aggregate error when a
// transfer failed or the final refresh left activation incomplete, and the
// GUI needs those results to say what happened. It writes nothing when
// results is nil. It returns err when non-nil, else any encode error, so the
// caller's fatal path and exit code still reflect the SDK failure.
func WriteUpdateResult(w io.Writer, results []updex.UpdateFeaturesResult, err error) error {
	if results != nil {
		if encErr := json.NewEncoder(w).Encode(results); encErr != nil && err == nil {
			return fmt.Errorf("failed to encode JSON: %w", encErr)
		}
	}
	return err
}
