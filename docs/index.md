# ChairLift

ChairLift is the command center of [Snow Linux](https://github.com/frostyard/snosi): a system-management app written in Go on [MyGo](https://mygo.egoist.dev), which draws its own widgets on GTK 3 and OpenGL loaded at run time — no CGO. One window manages the system image (bootc or native A/B), system features (via updex), Flatpak apps, Homebrew packages, device firmware (fwupd), the problems recorded on the computer, and cleanups.

The UI is YAML-configuration-driven, making it portable to other Linux distributions by toggling feature groups on or off ([CONFIG.md](https://github.com/frostyard/chairlift/blob/main/CONFIG.md), [reference](reference.md)).

This page is the user-facing docs-site home. Contributor- and agent-facing
documentation — architecture, ADRs, specs, and plans — is indexed in
[docs/README.md](README.md), with
[docs/design/overview.md](design/overview.md) as the architecture entry
point.

## The window

The window is a command center: a top bar with a command palette (Ctrl K),
a left rail of six pages, a console that shows the commands ChairLift runs
and their output (Ctrl J), and a status bar with the running image, the
running job, the updates and problems waiting and the free space. Every
change runs in one queue, one at a time. Installs, uninstalls, pins, feature
toggles, tap trust, bundles, cleanups and firmware ask first; updates start
when you click them.

| Page | Description |
|------|-------------|
| **Overview** | The system image with its deployment chain (rollback, booted, staged) and its one main action (Check now, Update everything, or Review), figures for updates, problems, features and free space, live processor and memory use, the disk, the system details, and what ChairLift changed recently. Always shown. |
| **Updates** | One update run across every source — the system image (staged with bootc or native A/B, applies on restart), features, Flatpak, Homebrew and firmware — drawn as a pipeline, then a card per source. "Update everything" leaves out held packages and firmware, which installs device by device after a confirmation. Untrusted Homebrew taps show as banners with a Trust tap action. |
| **Apps** | Installed: every Flatpak app and Homebrew formula and cask in one sortable, filterable table, with an inspector to update, open, pin or unpin, and uninstall. Bundles: curated Brewfiles to install. Find more: Homebrew search and install. A button opens the configured software center (Bazaar by default) for new Flatpak apps. |
| **Features** | Turn system features managed by updex on or off, after a confirmation, and update them. |
| **Problems** | Problems recorded on this computer — Snow's system checks (`snosi-health`), crashes of your programs, and frostyard app problems — each with a hint, Copy details, Dismiss and Report…. Reporting is opt-in and off by default; then each report is either shown before it is sent or sent automatically, at most 20 a day plus one weekly count ([ADR-0015](adr/0015-problems-page-and-opt-in-reporting.md), [specs/problem-reporting.md](specs/problem-reporting.md)). |
| **Maintenance** | Clean up Homebrew, remove unused Flatpak runtimes, and run the configured maintenance scripts, one at a time or with Clean everything; the free space and the deployments. |

A page is omitted when all of its groups are disabled; the Overview always
shows, so the window always has a page. **Help** is a dialog (F1, the main
menu or the palette) with the configured links to the project website, the
issue tracker and the community chat; **About ChairLift** and **Keyboard
shortcuts** are dialogs too.

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Alt+1` … `Alt+N` | Open the first through Nth visible page in rail order; omitted pages leave no gaps |
| `Ctrl+K` | Command palette |
| `Ctrl+J` | Show or hide the console |
| `Ctrl+R` | Check for updates |
| `Ctrl+?` | Keyboard shortcuts dialog |
| `F1` | Help |
| `Ctrl+Q` | Quit |

## Command-Line Flags

| Flag | Description |
|------|-------------|
| `--dry-run`, `-d` | Run without making any changes to the system. Propagated to all package manager wrappers; the console shows each command that would run. |
| `--first-run` | Open the first-run setup window (application ID `org.frostyard.ChairLift.FirstRun`) instead of the main window. `chairlift-first-run` starts it once per user; ChairLift ships no autostart entry ([ADR-0014](adr/0014-first-run-mode.md)). |
| `--send-pending-reports` | Send the problem reports that your consent allows, then exit; no window. |

## Optional Dependencies

Runtime visibility depends on the group:

- the system image hero, the image update source and Maintenance's
  deployments appear only on a bootc host (`bootc status` reports a booted
  deployment) or a native A/B host, and the image source only when the
  host's stage script is installed;
- the firmware source stays out of the Updates page until `fwupdmgr` is
  installed;
- the Flatpak and Homebrew update sources, their kinds in the Apps table,
  and their cleanups are left out when the tool is not installed;
- the Homebrew untrusted-tap banners show only when actionable taps exist;
- the Features page says that the feature manager is not available when
  updex has no features here.

Page omission is separate and static: it depends only on which groups
configuration enables, not on runtime tool availability.

| Tool | Used For |
|------|----------|
| Homebrew | Package management (formulae, casks, bundles) |
| Flatpak | Installed-application listing, updates and uninstall; new installs are delegated to the configured software center |
| fwupd (`fwupdmgr`) | Firmware updates |
| bootc + `/usr/libexec/bootc-update-stage` | Staged bootc system updates |
| `/usr/lib/snosi/native-ab` marker + `/usr/libexec/snosi-sysupdate-stage` | Staged native A/B (systemd-sysupdate) system updates |
| Updex | System feature toggles and updates |
| Mission Center | The system monitor the Overview opens (configurable) |

The window itself needs GTK 3, libepoxy with an OpenGL driver (Mesa), and
Pango at run time. MyGo loads them when the window opens and draws on the
CPU when OpenGL cannot run on a GPU.

## Installation

Each [ChairLift release](https://github.com/frostyard/chairlift/releases)
provides ready-to-install packages for 64-bit Intel/AMD and Arm systems:

| Distribution family | Full-package filename | Install command |
|---|---|---|
| Debian/Ubuntu | `frostyard-chairlift_<version>_<arch>.deb` (`amd64` or `arm64`) | `sudo apt install ./<downloaded-filename>` |
| Fedora/RHEL | `frostyard-chairlift-<version>-1.<arch>.rpm` (`x86_64` or `aarch64`) | `sudo dnf install ./<downloaded-filename>` |
| Alpine | `frostyard-chairlift_<version>_<arch>.apk` (`x86_64` or `aarch64`) | `sudo apk add --allow-untrusted ./<downloaded-filename>` |

Download `checksums.txt` from the same release and verify the selected package
before installing it:

```bash
package='<downloaded-package-filename>'
grep -F "  $package" checksums.txt | sha256sum --check -
```

Tag releases also publish the `.deb` packages to Frostyard's APT repository,
`https://repository.frostyard.org/debian/`, from which Snow Linux images
install ChairLift.

The full `frostyard-chairlift` package includes the GUI, the privileged
helper, desktop assets, PolicyKit policies, and package-maintainer
configuration. For distributions that install the GUI through a user-scoped
Homebrew cask, releases also provide a `frostyard-chairlift-system-integration`
deb/rpm/apk. It installs the fixed-path updex helper, PolicyKit policies, and
maintainer configuration without installing the GUI. It intentionally
conflicts with the self-contained `frostyard-chairlift` package. Both packages
depend on the system's pkexec provider. Privileged actions also need a running
PolicyKit authentication agent in the desktop session. Bootc staging
additionally requires the distribution to provide its trusted implementation
at `/usr/libexec/bootc-update-stage`; the integration package does not supply
one. Native A/B staging uses `/usr/libexec/snosi-sysupdate-stage`, which
ships with the OS image itself.

### From source

```bash
make build
sudo make install
```

`make build` produces three binaries in `build/`, all built with
`CGO_ENABLED=0`:

- `chairlift` — the main application (a MyGo production build: no inspector,
  no automation plugin)
- `chairlift-updex-helper` — privileged helper for updex write operations
- `chairlift-first-run` — pure-Go first-run gate; it loads no UI library

`make install` installs binaries, desktop file, icons, PolicyKit policies, the
updex helper, and maintainer configuration defaults to `PREFIX`
(default `/usr`), and no autostart entry. The maintainer configuration is installed at
`/usr/share/chairlift/config.yml`; `/etc/chairlift/config.yml` is reserved for
administrator overrides and is never created or overwritten by ChairLift's
source or nFPM packages.

PolicyKit integration requires the default prefix: `polkitd` reads `.policy`
files only from `/usr/share/polkit-1/actions`, and `pkexec` matches the
updex helper against the absolute path `/usr/bin/chairlift-updex-helper` in
the policy's `org.freedesktop.policykit.exec.path` annotation. Under any other
`PREFIX` (for example `make install PREFIX=$HOME/.local`) the policies land
where polkit never looks, so the updex, bootc staging and sysupdate staging
features stop working or fall back to pkexec's generic, more restrictive
authentication. ChairLift
installs no passwordless PolicyKit rules. `DESTDIR` layers underneath
`PREFIX` for staged installs (`make install DESTDIR=/path/to/stage
PREFIX=/usr`), which is what `.goreleaser.yaml`'s nFPM packaging uses. A
source install made before the `/usr` default is removed with
`sudo make uninstall PREFIX=/usr/local`.

### Development

The [quality dashboard](quality.md) links to live CI, coverage, build-artifact,
and release signals and explains how to reproduce the enforced checks locally.
The [public metrics catalog](metrics/) collects the public, read-only sources
and documents their interpretation and agent-provenance boundaries.

```bash
make run               # Build and run with --dry-run
make dev               # Debug build with race detector
make test              # Run tests (views headless with MyGo's Tester)
make lint              # Run golangci-lint
make build-automation  # A window that mygo-automation can drive and capture
```

Set `CHAIRLIFT_SNAPSHOT_DIR` to make the page snapshot tests write PNGs, for
example `CHAIRLIFT_SNAPSHOT_DIR=/tmp/shots go test ./internal/views -run Snapshot`.
