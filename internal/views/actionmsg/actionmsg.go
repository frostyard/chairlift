// Package actionmsg builds the toast text (and, where the action itself is
// gated by dry-run, the execution decision) for maintenance-page,
// applications-page, and updates-page actions: Homebrew Brewfile dumps,
// Homebrew/Flatpak cleanup, Homebrew package installs/upgrades/self-updates,
// Flatpak application uninstalls/updates, Homebrew tap trust, and configured
// custom maintenance scripts.
//
// It is deliberately free of any puregotk/GTK import, following the
// internal/views/trustmsg pattern, so its logic can be unit-tested on a
// headless host. A test binary for a package that imports puregotk panics
// while resolving GTK/graphene shared libraries at package init — before any
// test function runs — so logic that must be tested cannot live in the view
// packages. See docs/agents/skills/gtk-headless-tests.md.
//
// Functions whose result only selects display text (BundleDump, Cleanup,
// Install, Uninstall, Upgrade, Update, SelfUpdate) return a plain string:
// the state-changing/no-op decision for those actions is already made and
// already tested inside their wrapper package (internal/homebrew,
// internal/flatpak). Functions whose result gates a further decision that
// has no wrapper package of its own to make it (MaintenanceScript, for
// configured custom scripts) return a decision struct instead of a plain
// string, precisely so the gated decision — not just the wording of the
// toast that follows it — is what a table-driven test in actionmsg_test.go
// asserts. TapTrust also returns a decision struct even though
// homebrew.TrustPackages already gates the underlying `brew trust` exec
// itself: the thing that needs gating here is a second, UI-side decision —
// whether the Untrusted Homebrew Taps view mutates (row removal, group
// visibility, refresh) — which has no wrapper-package equivalent to decide
// it, so TapTrustDecision.MutateUI is what actionmsg_test.go asserts on.
package actionmsg

import "fmt"

// BundleDump returns the toast text for a Homebrew Brewfile dump. When dryRun
// is true, homebrew.BundleDump itself never runs `brew bundle dump` (bundle
// is one of homebrew's stateChangingCommands, skipped entirely under dry-run)
// so path is never actually written, and the toast must say so rather than
// unconditionally claiming the file was saved.
func BundleDump(dryRun bool, path string) string {
	if dryRun {
		return fmt.Sprintf("[DRY-RUN] Preview: Brewfile would be saved to %s — no changes made", path)
	}
	return fmt.Sprintf("Brewfile saved to %s", path)
}

// Cleanup returns the toast text for a Homebrew or Flatpak cleanup action.
// The wrapper package (internal/homebrew or internal/flatpak) already skips
// the state-changing cleanup command under dry-run and returns a mock
// message as output — this function only selects which string to show: the
// wrapper's mock output when dryRun is true, or a fixed completion message
// when the cleanup actually ran.
func Cleanup(dryRun bool, tool string, output string) string {
	if dryRun {
		return output
	}
	return fmt.Sprintf("%s cleanup completed", tool)
}

// Install returns the toast text for a Homebrew package install. The
// wrapper package (internal/homebrew) already skips the state-changing
// `brew install` command under dry-run — install is one of homebrew's
// stateChangingCommands — so this function only selects which string to
// show: a preview when dryRun is true, or a fixed completion message when
// the install actually ran.
func Install(dryRun bool, pkgName string) string {
	if dryRun {
		return fmt.Sprintf("[DRY-RUN] Preview: %s would be installed — no changes made", pkgName)
	}
	return fmt.Sprintf("%s installed", pkgName)
}

// Uninstall returns the toast text for a Flatpak application uninstall. The
// wrapper package (internal/flatpak) already skips the state-changing
// `flatpak uninstall` command under dry-run, so this function only selects
// which string to show: a preview when dryRun is true, or a fixed completion
// message when the uninstall actually ran.
func Uninstall(dryRun bool, appID string) string {
	if dryRun {
		return fmt.Sprintf("[DRY-RUN] Preview: %s would be uninstalled — no changes made", appID)
	}
	return fmt.Sprintf("%s uninstalled", appID)
}

// Upgrade returns the toast text for a per-package Homebrew upgrade. The
// wrapper package (internal/homebrew) already skips the state-changing
// `brew upgrade` command under dry-run — upgrade is one of homebrew's
// stateChangingCommands — so this function only selects which string to
// show: a preview when dryRun is true, or a fixed completion message when
// the upgrade actually ran.
func Upgrade(dryRun bool, pkgName string) string {
	if dryRun {
		return fmt.Sprintf("[DRY-RUN] Preview: %s would be upgraded — no changes made", pkgName)
	}
	return fmt.Sprintf("%s upgraded", pkgName)
}

// Update returns the toast text for a per-app Flatpak update. The wrapper
// package (internal/flatpak) already skips the state-changing
// `flatpak update` command under dry-run, so this function only selects
// which string to show: a preview when dryRun is true, or a fixed completion
// message when the update actually ran.
func Update(dryRun bool, appID string) string {
	if dryRun {
		return fmt.Sprintf("[DRY-RUN] Preview: %s would be updated — no changes made", appID)
	}
	return fmt.Sprintf("%s updated", appID)
}

// SelfUpdate returns the toast text for a package manager self-update (e.g.
// Homebrew's own `brew update`). The wrapper package already skips the
// state-changing update command under dry-run, so this function only selects
// which string to show: a preview when dryRun is true, or a fixed completion
// message when the update actually ran.
func SelfUpdate(dryRun bool, tool string) string {
	if dryRun {
		return fmt.Sprintf("[DRY-RUN] Preview: %s would be updated — no changes made", tool)
	}
	return fmt.Sprintf("%s updated successfully", tool)
}

// TapTrustDecision is the result of deciding whether trusting a Homebrew tap
// should mutate the Untrusted Homebrew Taps UI (remove the tap's row, hide
// the group when empty, refresh outdated packages), and what toast to show
// for that decision.
type TapTrustDecision struct {
	// MutateUI is true when the tap was actually trusted (homebrew.
	// TrustPackages ran `brew trust` for real) and the Untrusted Homebrew
	// Taps UI should reflect that: remove the row, hide the group if empty,
	// and refresh outdated packages. It is exactly !dryRun — under dry-run,
	// TrustPackages's underlying `brew trust` call never runs (trust is one
	// of homebrew's stateChangingCommands), so the tap is not actually
	// trusted and the UI must not act as though it were.
	MutateUI bool
	// Toast is the completion message to show immediately.
	Toast string
}

// TapTrust decides whether trusting a Homebrew tap (trustTap in
// internal/views/updates_page.go, following a successful
// homebrew.TrustPackages call) should mutate the Untrusted Homebrew Taps UI,
// and what toast to show. MutateUI is exactly !dryRun; the caller must not
// independently recompute that condition. Under dry-run, TrustPackages's
// `brew trust` call is skipped entirely by homebrew's stateChangingCommands
// gate, so nothing was actually trusted — removing the row, hiding the
// group, or refreshing outdated packages would make the tap disappear from
// the untrusted list as if it were now trusted, with no way to undo it from
// the UI. This function is what actionmsg_test.go asserts on, precisely so
// that decision — not just the wording of the toast that follows it — is
// tested.
func TapTrust(dryRun bool, tapName string) TapTrustDecision {
	if dryRun {
		return TapTrustDecision{
			MutateUI: false,
			Toast:    fmt.Sprintf("[DRY-RUN] Preview: %s would be trusted — no changes made", tapName),
		}
	}
	return TapTrustDecision{
		MutateUI: true,
		Toast:    fmt.Sprintf("Trusted %s. Its packages can update again.", tapName),
	}
}

// ScriptDecision is the result of deciding whether a configured custom
// maintenance script should actually execute, and what toast to show for
// that decision.
type ScriptDecision struct {
	// Execute is true when the script should actually be run (cmd.Run()
	// invoked, whether direct or via pkexec). It is false under dry-run, in
	// which case no exec.Cmd may be constructed or run at all.
	Execute bool
	// Toast is the completion message to show immediately (dry-run) or once
	// the script's cmd.Run() returns successfully (live run).
	Toast string
}

// MaintenanceScript decides whether a configured custom maintenance script
// (config.yml's `actions` entries, run by runMaintenanceAction in
// internal/views/maintenance_page.go) should execute. Custom scripts have no
// wrapper package of their own to gate their execution the way homebrew,
// flatpak, bootc, and updex do, so this is the one place that decision is
// made and tested. Execute is exactly !dryRun; the caller must not
// independently recompute that condition.
func MaintenanceScript(dryRun bool, title string) ScriptDecision {
	if dryRun {
		return ScriptDecision{
			Execute: false,
			Toast:   fmt.Sprintf("[DRY-RUN] Preview: %s would run — no changes made", title),
		}
	}
	return ScriptDecision{
		Execute: true,
		Toast:   fmt.Sprintf("%s completed", title),
	}
}
