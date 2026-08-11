# ChairLift

ChairLift is a GTK4/Libadwaita system management GUI for [Snow Linux](https://github.com/frostyard/snosi), written in Go using [puregotk](https://codeberg.org/puregotk/puregotk) bindings (no CGO). It provides a unified interface for managing Homebrew and Flatpak applications, bootc system updates, system features (via updex), and maintenance tasks.

The UI is YAML-configuration-driven, making it portable to other Linux distributions by toggling feature groups on or off.

## Pages

ChairLift provides six configurable pages:

| Page | Description |
|------|-------------|
| **Applications** | Search/install Homebrew formulae and casks; uninstall installed formulae/casks; pin/unpin formulae; install curated Brewfile bundles. List/uninstall Flatpaks and launch the configured external manager for Flatpak discovery and installation. |
| **Maintenance** | Run cleanup tasks for Homebrew and Flatpak, and execute custom maintenance scripts. |
| **Updates** | Stage bootc or native A/B (systemd-sysupdate) system updates, apply Flatpak updates, upgrade Homebrew packages, and trust Homebrew taps. |
| **System** | View OS information, bootc deployment status, and launch a system health monitor. |
| **Features** | Toggle system features managed by updex. |
| **Help** | Links to the project website, issue tracker, and community chat. |

A functional page is omitted when all of its groups are disabled. Help is
always retained so the window always has a valid destination.

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+Q` | Quit |
| `Ctrl+?` | Show shortcuts dialog |
| `Alt+1` … `Alt+N` | Open the first through Nth visible page in sidebar order; omitted pages leave no gaps |
| `F1` | Help |

## Command-Line Flags

| Flag | Description |
|------|-------------|
| `--dry-run`, `-d` | Run without making any changes to the system. Propagated to all package manager wrappers. |

## Optional Dependencies

Runtime visibility depends on the group:

- bootc status/staging groups, the native A/B staging group, and unavailable
  Homebrew/Flatpak maintenance groups are hidden when their tool-specific
  runtime gates fail;
- the Homebrew untrusted-taps group stays hidden unless actionable taps exist;
- Features replaces its main group with an explicit unavailable message when
  Updex is not configured;
- Applications and Updates groups for Homebrew and Flatpak remain visible and
  report that the tool is unavailable.

Page omission is separate and static: it depends only on which builder-backed
groups configuration enables, not on runtime tool availability.

| Tool | Used For |
|------|----------|
| Homebrew | Package management (formulae, casks, bundles) |
| Flatpak | Installed-application listing/uninstall and updates; new installs are delegated to the configured external manager |
| bootc + `/usr/libexec/bootc-update-stage` | Staged bootc system updates |
| `/usr/lib/snosi/native-ab` marker + `/usr/libexec/snosi-sysupdate-stage` | Staged native A/B (systemd-sysupdate) system updates |
| Updex | System feature toggles |

## Building

```bash
make build
```

This produces two binaries in `build/`:

- `chairlift` — the main application
- `chairlift-updex-helper` — privileged helper for updex write operations

Both are built with `CGO_ENABLED=0`.

### Installation

```bash
sudo make install
```

Installs binaries, desktop file, icons, PolicyKit policies, the updex helper,
and maintainer configuration defaults to `PREFIX` (default `/usr`). The
maintainer configuration is installed at `/usr/share/chairlift/config.yml`;
`/etc/chairlift/config.yml` is reserved for administrator overrides and is
never created or overwritten by ChairLift's source or nFPM packages. PolicyKit
integration requires the default prefix.

For distributions that install the GUI through a user-scoped Homebrew cask,
releases also provide a `frostyard-chairlift-system-integration` deb/rpm/apk.
It installs the fixed-path updex helper, PolicyKit policies, and maintainer
configuration without installing the GUI. It intentionally conflicts with the
self-contained `frostyard-chairlift` package. Bootc staging additionally
requires the distribution to provide its trusted implementation at
`/usr/libexec/bootc-update-stage`; the integration package does not supply
one. Native A/B staging uses `/usr/libexec/snosi-sysupdate-stage`, which
ships with the OS image itself.

### Development

The [quality dashboard](quality.md) links to live CI, coverage, build-artifact,
and release signals and explains how to reproduce the enforced checks locally.
The [public metrics catalog](metrics/) collects the public, read-only sources
and documents their interpretation and agent-provenance boundaries.

```bash
make run      # Build and run with --dry-run
make dev      # Debug build with race detector
make test     # Run tests
make lint     # Run golangci-lint
```
