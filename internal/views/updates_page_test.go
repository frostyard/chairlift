package views

import "testing"

// TestLoadOutdatedPackagesNilExpander is a regression test for the panic
// described in AGENTS.md/yeti/package-managers.md's tap-trust nil-safety
// note: trustTap unconditionally refreshes outdated packages after a
// successful "brew trust", but outdatedExpander is only built when
// brew_updates_group is enabled. A host that enables brew_trust_group while
// disabling brew_updates_group must not panic on a nil *adw.ExpanderRow.
//
// This constructs a zero-value &UserHome{} — no widgets allocated, so
// nothing dlopen's GTK — with outdatedExpander left nil, the direct analog
// of brew_updates_group being disabled. loadOutdatedPackages must return
// before touching homebrew.IsInstalledCached(), homebrew.ListOutdated(),
// uh.updateBadgeCount(), or sgtk.RunOnMainThread, all of which are unsafe or
// unreachable in this headless test environment (see
// docs/agents/skills/gtk-headless-tests.md).
func TestLoadOutdatedPackagesNilExpander(t *testing.T) {
	uh := &UserHome{}

	uh.loadOutdatedPackages()
}
