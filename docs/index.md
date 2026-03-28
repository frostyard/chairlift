# ChairLift

ChairLift is a GTK4/Libadwaita system management GUI for [Snow Linux](https://github.com/frostyard/snow), written in Go using [puregotk](https://codeberg.org/puregotk/puregotk) bindings (no CGO). It provides a unified interface for managing Homebrew packages, Flatpak and Snap applications, NBC bootc system updates, system features (via updex), and maintenance tasks.

The UI is YAML-configuration-driven, making it portable to other Linux distributions by toggling feature groups on or off.

## Pages

ChairLift organizes its functionality into six pages:

| Page | Description |
|------|-------------|
| **Applications** | Browse, search, install, and uninstall Flatpak (user and system), Snap, and Homebrew packages. Install curated Brewfile bundles. |
| **Maintenance** | Run cleanup tasks for Homebrew and Flatpak, and execute custom maintenance scripts. |
| **Updates** | Check and apply NBC system updates, Flatpak updates, and Homebrew package upgrades. |
| **System** | View OS information, NBC bootc status, and launch a system health monitor. |
| **Features** | Toggle system features managed by updex. |
| **Help** | Links to the project website, issue tracker, and community chat. |

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+Q` | Quit |
| `Ctrl+?` | Show shortcuts dialog |
| `Alt+1` | Applications |
| `Alt+2` | Maintenance |
| `Alt+3` | Updates |
| `Alt+4` | System |
| `Alt+5` | Features |
| `Alt+6` | Help |

## Command-Line Flags

| Flag | Description |
|------|-------------|
| `--dry-run`, `-d` | Run without making any changes to the system. Propagated to all package manager wrappers. |

## Optional Dependencies

ChairLift adapts to what is available on the system. Groups for unavailable tools are hidden automatically.

| Tool | Used For |
|------|----------|
| Homebrew | Package management (formulae, casks, bundles) |
| Flatpak | Application management and updates |
| Snap / snapd | Snap package management |
| NBC (`/usr/bin/nbc`) | Bootc system updates |
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

Installs binaries, desktop file, icons, PolicyKit policies, and the updex helper to `PREFIX` (default `/usr/local`).

### Development

```bash
make run      # Build and run with --dry-run
make dev      # Debug build with race detector
make test     # Run tests
make lint     # Run golangci-lint
```
