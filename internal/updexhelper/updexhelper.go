// Package updexhelper holds the argv-parsing and Options-building logic for
// cmd/chairlift-updex-helper, the privileged helper binary invoked via
// pkexec to perform updex write operations. It is deliberately free of any
// puregotk/GTK import (only stdlib plus github.com/frostyard/updex/updex),
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
	"fmt"

	"github.com/frostyard/updex/updex"
)

const (
	CommandEnableFeature  = "enable-feature"
	CommandDisableFeature = "disable-feature"
	CommandUpdate         = "update"
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
