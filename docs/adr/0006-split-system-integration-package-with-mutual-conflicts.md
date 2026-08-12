# 0006 — Publish a GUI-less system-integration package that conflicts with the full package

- **Status:** Accepted
- **Date:** 2026-08-12

## Context

ChairLift's privileged pieces — the updex helper binary, the three PolicyKit
policies, and the maintainer config defaults — must be root-owned files at
fixed `/usr` paths ([ADR-0001](0001-fixed-path-pkexec-privilege-boundary.md),
[ADR-0002](0002-usr-prefix-is-the-only-supported-install-prefix.md)). But the
GUI itself can arrive user-scoped, e.g. via a Homebrew cask, which cannot
install root-owned polkit policies. Such an installation needs the system
half from a distro package without pulling in a second copy of the GUI; and
two packages owning the same `/usr` files must never be co-installable.

## Decision

GoReleaser publishes two mutually exclusive nFPM package shapes from the same
release (`.goreleaser.yaml`, nfpms):

- **`frostyard-chairlift`** — self-contained: both binaries (build ids
  `chairlift`, `chairlift-updex-helper`), desktop file, icons, maintainer
  config, and all three policies.
- **`frostyard-chairlift-system-integration`** — root-owned companion for a
  user-scoped GUI delivery such as the Homebrew cask: build id
  `chairlift-updex-helper` only, and contents of exactly the three policies
  plus `/usr/share/chairlift/config.yml`. No GUI, no desktop assets.

Each declares `conflicts:` on the other, because they intentionally own the
same privileged files. Both ship as `deb`, `rpm`, and `apk`. Neither ships
`bootc-update-stage` or `snosi-sysupdate-stage` — those stage scripts are
distro policy, provided by the image at the fixed `/usr/libexec` paths, and
the UI hides the corresponding groups when the script is absent.

`TestGoreleaserPublishesSystemIntegrationPackage`
(`internal/installcheck/goreleaser_test.go:156-196`) pins the shape: unique
ids, exact build-id lists per package, the mutual `conflicts:` pair, the
format list, and content-set equality for the integration package (exactly
the four files, no more, no fewer).
`TestGoreleaserNfpmLayoutMatchesUsrPrefix` additionally holds every entry —
current and future — to the `/usr` layout.

## Consequences

- A Homebrew-cask (or other user-scoped) GUI install has a supported path to
  working privileged operations: install the integration package once as
  root.
- The package manager, not documentation, prevents the two shapes from
  fighting over `/usr/bin/chairlift-updex-helper` and the policies.
- Every privileged-surface change (new policy, renamed helper) must now be
  made in both packages; the content-set equality test turns a forgotten
  side into a CI failure.
- The integration package tracks ChairLift releases, so a cask user's GUI
  and system halves can skew in version between upgrades; the helper argv
  contract (ADR-0001) is the compatibility boundary.

## Alternatives considered

- **One package only:** rejected — forces a full GUI install (with desktop
  files and icons) onto systems whose GUI arrives via cask, and the two GUI
  copies would shadow each other.
- **`depends:`/`recommends:` from a cask onto the full package:** rejected —
  Homebrew casks cannot depend on distro packages; and the full package
  would still collide with the cask's GUI.
- **Letting the cask install the polkit files itself:** rejected — Homebrew
  installs are user-owned and unprivileged; polkit policies and the helper
  must be root-owned at fixed paths.
- **No `conflicts:` (rely on identical file contents):** rejected — dpkg/rpm
  file-conflict behavior differs by format and flags; explicit conflicts
  make the exclusivity a stated contract.

## References

- Shapes: [design/overview.md — Privileged operations](../design/overview.md#privileged-operations)
  ("System-integration delivery"),
  [design/package-managers.md](../design/package-managers.md) (release
  packaging notes)
- Builds on: [ADR-0001](0001-fixed-path-pkexec-privilege-boundary.md),
  [ADR-0002](0002-usr-prefix-is-the-only-supported-install-prefix.md),
  [core ADR-0011 — Distro packages are named frostyard-&lt;tool&gt;](https://github.com/frostyard/core/blob/main/docs/adr/0011-frostyard-prefixed-package-names.md)
- Enforced by: `internal/installcheck/goreleaser_test.go`
  (`TestGoreleaserPublishesSystemIntegrationPackage`,
  `TestGoreleaserNfpmLayoutMatchesUsrPrefix`)
