# Configuration Reference

ChairLift is configured via a YAML file that controls which UI groups are visible and their behavior.

## File Locations

Configuration files are searched in order (first found wins):

1. `/etc/chairlift/config.yml` — system-wide (highest priority)
2. `/usr/share/chairlift/config.yml` — package maintainer defaults
3. `chairlift/config.yml` — relative to executable (development)

If no file is found, all features default to enabled.

## Format

```yaml
page_name:
  group_name:
    enabled: true/false
    # Optional per-group fields (see below)
```

Groups with `enabled: false` are hidden from the UI. Missing entries default to `enabled: true`. Changes require restarting ChairLift.

## Pages and Groups

### System Page (`system_page`)

| Group | Key | Description |
|-------|-----|-------------|
| OS Info | `system_info_group` | Displays fields from `/etc/os-release` |
| NBC Status | `nbc_status_group` | NBC bootc status (image, slot, staged updates) |
| Health | `health_group` | Launches a system monitor application |

`health_group` supports:

- `app_id` — Flatpak application ID to launch (default: `io.missioncenter.MissionCenter`)

### Updates Page (`updates_page`)

| Group | Key | Description |
|-------|-----|-------------|
| NBC Updates | `nbc_updates_group` | Check, download, and apply NBC system updates |
| Flatpak Updates | `flatpak_updates_group` | Pending Flatpak application updates |
| Homebrew Updates | `brew_updates_group` | Outdated Homebrew packages with upgrade buttons |
| Settings | `updates_settings_group` | Update preferences |

### Applications Page (`applications_page`)

| Group | Key | Description |
|-------|-----|-------------|
| Installed Apps | `applications_installed_group` | Launcher for a Flatpak manager |
| User Flatpak | `flatpak_user_group` | User-installed Flatpak applications |
| System Flatpak | `flatpak_system_group` | System-wide Flatpak applications |
| Snap | `snap_group` | Snap packages and Snap Store management |
| Homebrew | `brew_group` | Installed Homebrew formulae and casks |
| Brew Search | `brew_search_group` | Search and install Homebrew packages |
| Brew Bundles | `brew_bundles_group` | Install packages from Brewfile bundles |

`applications_installed_group` supports:

- `app_id` — Flatpak application ID to launch (default: `io.github.kolunmi.Bazaar`)

`brew_bundles_group` supports:

- `bundles_paths` — list of directories to search for Brewfile bundles (default: `["/usr/share/snow/bundles"]`)

### Maintenance Page (`maintenance_page`)

| Group | Key | Description |
|-------|-----|-------------|
| Cleanup | `maintenance_cleanup_group` | Custom cleanup scripts |
| Homebrew Cleanup | `maintenance_brew_group` | `brew cleanup` (remove old versions and cache) |
| Flatpak Cleanup | `maintenance_flatpak_group` | `flatpak uninstall --unused` (remove unused runtimes) |
| Optimization | `maintenance_optimization_group` | System optimization (placeholder) |

`maintenance_cleanup_group` supports:

- `actions` — list of scripts to offer:

```yaml
actions:
  - title: "Clean Up Boot Old Entries"
    script: "/usr/libexec/bls-gc"
    sudo: true
```

Each action has:

| Field | Description |
|-------|-------------|
| `title` | Display name |
| `script` | Absolute path to the script |
| `sudo` | If `true`, runs via `pkexec` for elevated privileges |

### Features Page (`features_page`)

| Group | Key | Description |
|-------|-----|-------------|
| Features | `features_group` | Toggle system features managed by updex |

Feature operations (enable, disable, update) require PolicyKit authentication and are performed by the `chairlift-updex-helper` binary.

### Help Page (`help_page`)

| Group | Key | Description |
|-------|-----|-------------|
| Resources | `help_resources_group` | Links to project resources |

`help_resources_group` supports:

| Field | Description |
|-------|-------------|
| `website` | Project website URL |
| `issues` | Issue tracker URL |
| `chat` | Community chat or discussions URL |

## Example

A configuration that disables all Homebrew features:

```yaml
applications_page:
  brew_group:
    enabled: false
  brew_search_group:
    enabled: false
  brew_bundles_group:
    enabled: false

updates_page:
  brew_updates_group:
    enabled: false

maintenance_page:
  maintenance_brew_group:
    enabled: false
```

All other groups remain enabled by default since they are not listed.
