package views

import (
	"strings"
	"testing"
)

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

// TestUntrustedTapUpgradeMessage covers both trustGroupAvailable states for
// the toast text shown when a Homebrew upgrade fails with an
// *homebrew.UntrustedTapError (internal/views/updates_page.go's per-row click
// handler in loadOutdatedPackages). When the Untrusted Homebrew Taps group
// isn't available (brew_trust_group disabled, or not yet built), the message
// must not point the user at a section that doesn't exist: no "below" and no
// "Untrusted Homebrew Taps" reference. When the group is available, the
// existing pointer-to-the-section wording must be preserved so current
// behavior is unchanged.
//
// untrustedTapUpgradeMessage is a standalone, pure function (no widget
// access), so this test needs no GTK/GLib and is safe to run headlessly per
// docs/agents/skills/gtk-headless-tests.md.
func TestUntrustedTapUpgradeMessage(t *testing.T) {
	tests := []struct {
		name                string
		pkgName             string
		trustGroupAvailable bool
		wantContains        []string
		forbidContains      []string
	}{
		{
			name:                "trust group available points to section",
			pkgName:             "example-pkg",
			trustGroupAvailable: true,
			wantContains:        []string{"example-pkg", "untrusted tap", "Untrusted Homebrew Taps", "below"},
		},
		{
			name:                "trust group unavailable is self-contained",
			pkgName:             "example-pkg",
			trustGroupAvailable: false,
			wantContains:        []string{"example-pkg", "untrusted tap", "trusted"},
			forbidContains:      []string{"below", "Untrusted Homebrew Taps"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := untrustedTapUpgradeMessage(tt.pkgName, tt.trustGroupAvailable)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("untrustedTapUpgradeMessage(%q, %v) = %q, want it to contain %q", tt.pkgName, tt.trustGroupAvailable, got, want)
				}
			}
			for _, forbid := range tt.forbidContains {
				if strings.Contains(got, forbid) {
					t.Errorf("untrustedTapUpgradeMessage(%q, %v) = %q, must not contain %q", tt.pkgName, tt.trustGroupAvailable, got, forbid)
				}
			}
		})
	}
}
