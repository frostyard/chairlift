# 0011 — The unprivileged feature check compares a staged image with the booted image

- **Status:** Accepted
- **Date:** 2026-10-29

## Context

The Features page shows a row per updex feature, and its subtitle comes from
`updex.CheckFeatures`. One of the states that check reports is a wait: a
component's newest staged image is not linked, and the next `features update`
will not link it, because its `UPDEX_MIN_IMAGE_VERSION` is above the
`IMAGE_VERSION` of the image that boots next (updex ADR-0016). updex reads
that version with `imagecompat.NextBootImageVersion`, which runs
`bootc status --json`.

ChairLift runs the check as the ordinary, unprivileged GUI user. On Snow Linux
(bootc 1.17.1, tested as uid 1001) that command fails for such a user:

```
This command must be executed as the root user
```

The failure is not a missing bootc, so updex does not fall back. It puts an
error on every component whose staged image sets the key and leaves
`NeedsImage` unset. ChairLift's `partitionFeatureChecks` drops error-bearing
components to keep updex v1's partial-failure behavior, so the wait vanished
before it reached the page: on a bootc host the "waiting for a newer OS
image" state added in PR #254 never appeared. Even the re-check after an
update (added in the same change as this ADR) would drop the component, and
the row would show its plain description instead of naming the wait.

The check only reads. It decides what text a row shows; it changes no state.

## Decision

The read-only client in `internal/updex` gives updex a bootc runner that
answers an unprivileged bootc failure the way updex answers a missing bootc:
`internal/updex/bootcrunner.go`'s `checkBootcRunner` calls the real
`imagecompat.DefaultBootcRunner`, and returns its result unchanged when bootc
succeeded, when the error already wraps `exec.ErrNotFound`, when the context is
cancelled, or when the process is root. In every other case it writes one log
line and returns an error that wraps `exec.ErrNotFound` and keeps the inner
error text. `getClient` installs it:
`updexapi.ClientConfig{BootcRunner: newCheckBootcRunner()}`.

`imagecompat.NextBootImageVersion` treats a wrapped `exec.ErrNotFound` as "not
an image-based host" and falls back to `IMAGE_VERSION` of the booted OS image
from os-release. The unprivileged check therefore compares a staged image with
the booted image, which is the read-only question it can answer.

The privileged side does not change. `cmd/chairlift-updex-helper` still builds
its client with `updex.ClientConfig{}` and the real bootc, so the link gate
that decides whether a link may move keeps failing closed where root can read
bootc but bootc fails. No PolicyKit action, helper path, or argv surface is
added, so [ADR-0001](0001-fixed-path-pkexec-privilege-boundary.md)'s
privilege boundary is untouched: the fallback widens what an unprivileged
*read* reports, never what a privileged write may do.

## Consequences

- On a bootc host the Features page shows the wait it always meant to show: a
  row reads "waiting for a newer OS image" after an update that staged an image
  which needs a newer OS.
- The known limit: when bootc has staged a newer *OS* deployment, the booted
  os-release is older than the image that boots next, so a row can read
  "waiting for a newer OS image" although the next update would link. The
  reboot into that OS clears it. This is the price of an unprivileged read;
  the alternative was reporting nothing at all.
- Root keeps fail-closed behavior, so a genuine bootc failure still stops a
  link move rather than silently relinking.
- The check side now depends on `github.com/frostyard/updex/v2/imagecompat`
  (`internal/updex/bootcrunner.go` and its test). The frozen import allowlist
  in `internal/installcheck/updex_dependency_test.go` names both files, so a
  new import of the module cannot slip in unnoticed.
- One log line per failed check on a bootc host explains the fallback, so a
  reader of the log is not left wondering why bootc was not used.

## Alternatives considered

- **Run the check through pkexec:** rejected — it would add a PolicyKit action
  for a read, prompt the user for a password to draw a row subtitle, and widen
  the privilege boundary for a purely informational question.
- **Ask the root helper for the next-boot version:** rejected for the same
  reason plus a new helper subcommand and its own `exec.argv1` annotation;
  nothing about the change needs root if the booted image is an acceptable
  comparison.
- **Leave the failure and show a component error:** rejected — it reports an
  unprivileged read as a defect of the feature, and the row that PR #254 added
  stays invisible on every bootc host.
- **Fall back inside a fork of the updex client:** rejected — the SDK already
  defines the fallback and its trigger (`exec.ErrNotFound`); duplicating that
  logic in ChairLift would drift from it. The SDK carries the policy, this ADR
  records why ChairLift feeds it that answer.

## References

- Shapes: [design/package-managers.md — View-layer feature update status](../design/package-managers.md#view-layer-feature-update-status-internalviewsfeaturestatus),
  [design/package-managers.md — Updex](../design/package-managers.md), [design/overview.md](../design/overview.md)
- Builds on: [ADR-0001](0001-fixed-path-pkexec-privilege-boundary.md),
  [ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md)
- Related: [ADR-0009](0009-dry-run-output-convention-and-single-decision-structs.md) (`FeatureUpdateDecision.Recheck`)
- Enforced by: `internal/updex/bootcrunner_test.go` (the pass-through and
  fallback table, an end-to-end `imagecompat.NextBootImageVersion` call, and
  the source check that only the check client gets the runner) and
  `internal/views/featurestatus/wiring_test.go`
