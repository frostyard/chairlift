// Package trustmsg builds the user-facing text shown when a Homebrew upgrade
// is blocked by an untrusted tap.
//
// It is deliberately free of any puregotk/GTK import so its logic can be
// unit-tested on a headless host. A test binary for a package that imports
// puregotk panics while resolving GTK/graphene shared libraries at package
// init — before any test function runs — so logic that must be tested cannot
// live in the view packages. See docs/agents/skills/gtk-headless-tests.md.
package trustmsg

import "fmt"

// UpgradeMessage returns the toast text for a Homebrew upgrade that failed with
// an untrusted-tap error. When the Untrusted Homebrew Taps group is present in
// the UI (trustGroupAvailable), the message points the user there; otherwise it
// is self-contained and references no UI section that may not exist — the case
// where brew_trust_group is disabled or its group has not been built.
func UpgradeMessage(pkgName string, trustGroupAvailable bool) string {
	if trustGroupAvailable {
		return fmt.Sprintf("%s comes from an untrusted tap — see Untrusted Homebrew Taps below", pkgName)
	}
	return fmt.Sprintf("%s comes from an untrusted tap and cannot be upgraded until the tap is trusted", pkgName)
}
