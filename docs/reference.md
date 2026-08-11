# Configuration Reference

ChairLift is configured via a YAML file that controls which UI groups are visible and their behavior.

## File Locations

Configuration files are searched in order (first found wins):

1. `/etc/chairlift/config.yml` — system-wide (highest priority)
2. `/usr/share/chairlift/config.yml` — package maintainer defaults
3. `config.yml` — beside the executable when present, otherwise relative to
   the current working directory (development fallback)

Only a missing candidate advances the search. The first existing candidate is
authoritative. If it cannot be read or fails YAML/schema validation, ChairLift
hides every feature group, logs a `CONFIGURATION ERROR`, and displays a
persistent toast with the path and cause. Fix the file and restart ChairLift.
If no file is found, built-in defaults apply: all groups are enabled except
`maintenance_cleanup_group`.

Source and nFPM installs provide the repository's maintainer defaults at
`/usr/share/chairlift/config.yml`. Package upgrades may replace that file.
Administrators should put local changes in `/etc/chairlift/config.yml`, which
has higher precedence and is never created or overwritten by ChairLift's
packages.

## Format

```yaml
page_name:
  group_name:
    enabled: true/false
    # Optional per-group fields (see below)
```

Groups with `enabled: false` are hidden from the UI. Missing entries inherit
their built-in values. Unknown page, group, or field names and wrong field
types are errors. Changes require restarting ChairLift.

When every builder-backed group on a functional page is disabled, the page is
also omitted from the sidebar, content stack, shortcuts dialog, and
Alt+number bindings. Alt+number is compacted over the remaining pages in the
order below. Help is always retained.

## Pages and Groups

### System Page (`system_page`)

| Group | Key | Description |
|-------|-----|-------------|
| OS Info | `system_info_group` | Displays fields from `/etc/os-release` |
| bootc Status | `bootc_status_group` | bootc deployment status (booted/staged/rollback image, version, digest); shown only when `bootc.IsBootcBootedCached()` reports a booted deployment |
| Health | `health_group` | Launches a system monitor application |

`health_group` supports:

- `app_id` — Flatpak application ID to launch (default: `io.missioncenter.MissionCenter`)

### Updates Page (`updates_page`)

| Group | Key | Description |
|-------|-----|-------------|
| bootc Updates | `bootc_updates_group` | Download and stage the next bootc system image update (applies on restart); shown only when bootc-booted and the fixed `/usr/libexec/bootc-update-stage` helper is present. Non-Snow distributions must provide a trusted implementation there before enabling this group; ChairLift's system-integration package does not supply one. |
| Native A/B Updates | `sysupdate_updates_group` | Download and stage the next native A/B (systemd-sysupdate) system image update (applies on restart), plus a read-only previous-version rollback row; shown only when the `/usr/lib/snosi/native-ab` marker and the fixed `/usr/libexec/snosi-sysupdate-stage` helper are present. The OS image ships both; ChairLift's system-integration package supplies only the PolicyKit policy. |
| Flatpak Updates | `flatpak_updates_group` | Pending Flatpak application updates |
| Homebrew Updates | `brew_updates_group` | Outdated Homebrew packages with upgrade buttons |
| Untrusted Taps | `brew_trust_group` | Untrusted Homebrew taps with installed packages (Homebrew 6 tap trust); trust a tap to resume its updates. Shown only when there is something to trust |

### Applications Page (`applications_page`)

| Group | Key | Description |
|-------|-----|-------------|
| Installed Apps | `applications_installed_group` | Launcher for the external Flatpak manager used for discovery and installation |
| User Flatpak | `flatpak_user_group` | User-installed Flatpak applications with uninstall actions |
| System Flatpak | `flatpak_system_group` | System-wide Flatpak applications with uninstall actions |
| Homebrew | `brew_group` | Installed Homebrew formulae/casks with uninstall actions and formula pin/unpin actions |
| Brew Search | `brew_search_group` | Search and install Homebrew formulae and casks with an explicit package-type confirmation |
| Brew Bundles | `brew_bundles_group` | Install packages from Brewfile bundles |

`applications_installed_group` supports:

- `app_id` — Flatpak application ID to launch (default: `io.github.kolunmi.Bazaar`)

ChairLift does not directly discover or install new Flatpak applications.
Those operations belong to the configured external manager.

`brew_bundles_group` supports:

- `bundles_paths` — directories searched, without recursion, for
  `*.Brewfile` bundles (default: `["/usr/share/snow/bundles"]`). Missing
  directories are ignored; other path errors are shown while readable
  directories still contribute rows. Exact duplicate paths are collapsed,
  but same-named Brewfiles in different directories remain separate and show
  their absolute paths. The first-line `#` comment, when present, is displayed
  as the bundle description.

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

Feature operations (enable, disable, update) require administrator
authentication through PolicyKit and are performed by the fixed
`/usr/bin/chairlift-updex-helper` binary. ChairLift installs no passwordless
authorization rule.

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
