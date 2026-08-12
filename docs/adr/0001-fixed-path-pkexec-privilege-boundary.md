# 0001 — Route every root mutation through pkexec at fixed absolute paths

- **Status:** Accepted
- **Date:** 2026-08-12

## Context

ChairLift is an unprivileged GTK application, but three of its operations
mutate the system as root: enabling/disabling/updating updex features,
staging a bootc image update, and staging a native A/B (systemd-sysupdate)
update. PolicyKit's `pkexec` selects an action by comparing the program's
resolved absolute path *textually* against the
`org.freedesktop.policykit.exec.path` annotation, optionally refined by
`org.freedesktop.policykit.exec.argv1`; a bare `$PATH`-resolved name can
resolve differently per invoking process, missing the comparison and falling
back to the generic, more restrictive `pkexec.run-program` action. Crucially,
PolicyKit validates *nothing* after action selection — argv beyond argv1
passes through to the privileged program unchecked. ChairLift also once
shipped passwordless polkit `.rules` files granting a login group blanket
access, which older installs may still carry.

## Decision

Every root mutation is invoked through `pkexec` at a hardcoded absolute path
that is a Go constant, matching the policy's `exec.path` annotation exactly:

- updex writes go through `internal/updex.HelperPath =
  "/usr/bin/chairlift-updex-helper"` (`internal/updex/updex.go:30`); `runHelper`
  always passes this constant, never a bare name (`internal/updex/updex.go:159`).
- bootc staging runs `internal/bootc.StageScriptPath =
  "/usr/libexec/bootc-update-stage"` (`internal/bootc/stage.go:18`).
- native A/B staging runs `internal/sysupdate.StageScriptPath =
  "/usr/libexec/snosi-sysupdate-stage"` (`internal/sysupdate/stage.go:20`).

The updex policy (`data/org.frostyard.ChairLift.updex.policy`) declares one
action per helper subcommand, each selecting its command through
`exec.argv1` (`enable-feature`, `disable-feature`, `update`). Because
PolicyKit does not validate the rest of argv, the helper is a second
boundary: `internal/updexhelper.ParseInvocation`
(`internal/updexhelper/updexhelper.go:51`) accepts only the three argv shapes
ChairLift emits and rejects extra, misplaced, and unknown arguments before
any updex call.

No passwordless `.rules` files are ever shipped. All actions default to
`auth_admin` / `auth_admin` / `auth_admin_keep`, and the source install
removes the legacy ChairLift `.rules` files so an old passwordless grant
cannot survive an upgrade.

The whole boundary is pinned by tests: `internal/installcheck/polkit_test.go`
cross-references the policy XML against the Go constants and
`updexhelper.SupportedCommands()` (`TestPolkitPoliciesMatchPrivilegedHelpers`),
asserts the `.rules` files stay absent (`TestPolkitPasswordlessRulesAreAbsent`),
and requires exactly one action per helper command
(`TestEveryUpdexCommandHasOnePolicyAction`). The staged-install E2E test
(`test/e2e/e2e_test.go`, `TestInstalledBundleAndHelperBoundary`) executes the
installed helper's argv-rejection paths.

## Consequences

- Moving or renaming any privileged executable is a coordinated change
  across the Go constant, the policy XML, the Makefile, and
  `.goreleaser.yaml`; the installcheck tests fail on any one-sided edit
  instead of letting the pieces drift.
- The fixed `/usr` paths make `/usr` the only workable install prefix
  ([ADR-0002](0002-usr-prefix-is-the-only-supported-install-prefix.md)).
- Users authenticate as an administrator for every privileged operation
  (kept for the session by `auth_admin_keep`); there is deliberately no
  passwordless path.
- Argv validation is intentionally duplicated at the helper: even a caller
  that passes polkit's argv1 check cannot smuggle extra arguments past
  `ParseInvocation`.
- The stage scripts are distro-provided at fixed paths; making them
  configurable would let GUI configuration redirect a root execution, so
  they never become config values.

## Alternatives considered

- **Passwordless polkit `.rules` for a login group:** rejected — blanket
  root access; ChairLift previously shipped these and now actively removes
  them at install.
- **A D-Bus system service performing the mutations:** heavier lifecycle
  (unit files, activation, its own polkit checks) for three narrow
  operations; the fixed-path helper is the smallest surface.
- **`$PATH`-resolved helper name:** breaks pkexec's textual `exec.path`
  comparison and silently degrades to the generic run-program action.
- **Validating argv only in the GUI:** PolicyKit does not re-validate argv
  after action selection, so the privileged side must not trust its caller.

## References

- Shapes: [design/overview.md — Privileged operations](../design/overview.md#privileged-operations),
  [design/package-managers.md](../design/package-managers.md)
- Related: [ADR-0002](0002-usr-prefix-is-the-only-supported-install-prefix.md),
  [ADR-0006](0006-split-system-integration-package-with-mutual-conflicts.md)
- Builds on: [core ADR-0016 — Reverse-DNS org.frostyard.* identifiers](https://github.com/frostyard/core/blob/main/docs/adr/0016-reverse-dns-org-frostyard-identifiers.md)
  (the `org.frostyard.ChairLift.*` action naming; this ADR is the boundary
  mechanics)
- Enforced by: `internal/installcheck/polkit_test.go`,
  `test/e2e/e2e_test.go` (`TestInstalledBundleAndHelperBoundary`)
