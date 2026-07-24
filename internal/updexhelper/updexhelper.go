// Package updexhelper holds the argv-parsing and Options-building logic for
// cmd/chairlift-updex-helper, the privileged helper binary invoked via
// pkexec to perform updex write operations. It is deliberately free of any
// puregotk/GTK import (only stdlib plus github.com/frostyard/updex/updex),
// so its logic can be unit-tested on a headless host — a test binary for a
// package that imports puregotk panics while resolving GTK/graphene shared
// libraries at package init, before any test function runs. See
// docs/agents/skills/gtk-headless-tests.md.
//
// cmd/chairlift-updex-helper/main.go is reduced to argv dispatch only: it
// calls HasDryRunFlag to parse the shared --dry-run flag, then passes the
// per-subcommand Options struct built here to the corresponding updex
// client call.
package updexhelper

import "github.com/frostyard/updex/updex"

// HasDryRunFlag reports whether args (the helper's subcommand arguments,
// i.e. os.Args[2:]) contains the --dry-run flag. It takes an args slice
// rather than reading os.Args directly so it is a pure function, testable
// without process-global state.
func HasDryRunFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--dry-run" {
			return true
		}
	}
	return false
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
