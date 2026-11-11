// Package trustmsg builds the user-facing text shown when a Homebrew upgrade
// is blocked by an untrusted tap.
//
// It imports no UI package, so the text is tested as a plain function. See
// docs/design/engineering-lessons.md#no-tests-in-puregotk-packages.
package trustmsg

import "fmt"

// UpgradeMessage returns the toast text for a Homebrew upgrade that failed with
// an untrusted-tap error. When the tap-trust banner can show
// (trustGroupAvailable: brew_trust_group is on), the message points the user
// to it, in the Homebrew card of the Updates page; otherwise it
// is self-contained and references no UI section that may not exist — the case
// where brew_trust_group is disabled or its group has not been built.
func UpgradeMessage(pkgName string, trustGroupAvailable bool) string {
	if trustGroupAvailable {
		return fmt.Sprintf("%s comes from an untrusted tap — trust the tap in the Homebrew card of Updates", pkgName)
	}
	return fmt.Sprintf("%s comes from an untrusted tap and cannot be upgraded until the tap is trusted", pkgName)
}
