# Configuration Reference

ChairLift is configured via a YAML file that controls which UI groups are visible and their behavior.
The configuration keeps its page names (`system_page`, `applications_page`,
...); each section below says where in the command-center window its groups
show.

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

Groups with `enabled: false` are hidden from the UI, and their data is never loaded. Missing entries inherit
their built-in values. Unknown page, group, or field names and wrong field
types are errors. Changes require restarting ChairLift.

When every group that a page of the window shows is disabled, the page is
omitted from the rail, the command palette's "Go to" commands, the shortcuts
dialog, and the Alt+number bindings. Alt+number is compacted over the
remaining pages in rail order (Overview, Updates, Apps, Features, Problems,
Maintenance). The Overview always shows. Help is a dialog, not a page, and is
always reachable.

## Pages and Groups

### System Page (`system_page`): the Overview

| Group | Key | Description |
|-------|-----|-------------|
| System details | `system_info_group` | The System details card: this computer (host name, model, kernel, Secure Boot, uptime, image, build time, digest) and the fields of `/etc/os-release` |
| Image status | `bootc_status_group` | The system image hero: OS name, channel, Secure Boot and the deployment chain (rollback, booted, staged: version, digest, pin) on a bootc host (`bootc.IsBootcBootedCached()` reports a booted deployment) or a native A/B host; also Maintenance's read-only deployment chain |
| Health | `health_group` | The live processor and memory chart (sampled each second) and the button that launches a system monitor application |

`health_group` supports:

- `app_id` — desktop application ID or ordered list. The default tries GNOME
  System Monitor, then KDE System Monitor. The first installed
  entry visible in the desktop menu is used. An empty string or list uses
  the default order.

### Updates Page (`updates_page`): the Updates page

Each group is an update source of the Updates page; the features source
follows `features_page`'s `features_group`.

| Group | Key | Description |
|-------|-----|-------------|
| bootc Updates | `bootc_updates_group` | Download and stage the next bootc system image update (applies on restart); shown only when bootc-booted and the fixed `/usr/libexec/bootc-update-stage` helper is present. Non-Snow distributions must provide a trusted implementation there before enabling this group; ChairLift's system-integration package does not supply one. |
| Native A/B Updates | `sysupdate_updates_group` | Download and stage the next native A/B (systemd-sysupdate) system image update (applies on restart), plus a read-only previous-version line; shown only when the `/usr/lib/snosi/native-ab` marker and the fixed `/usr/libexec/snosi-sysupdate-stage` helper are present. The OS image ships both; ChairLift's system-integration package supplies only the PolicyKit policy. |
| Flatpak Updates | `flatpak_updates_group` | Pending Flatpak application updates |
| Homebrew Updates | `brew_updates_group` | Outdated Homebrew packages with upgrade buttons, Update all, and the update of Homebrew itself; pinned formulae and packages of untrusted taps are held |
| Untrusted Taps | `brew_trust_group` | Banners on the Homebrew card for untrusted Homebrew taps with installed packages (Homebrew 6 tap trust); trust a tap to resume its updates. Shown only when there is something to trust |
| Firmware Updates | `firmware_updates_group` | Device firmware updates from the Linux Vendor Firmware Service (fwupd), installed one device at a time after a confirmation that warns about power and the restart or shutdown the device needs, and never by "Update everything"; only one install runs at a time. A device that fwupd will not update now (for example a closed lid, a host on battery, or a recorded update error) stays in the list as held, with its reason and without an install button; held devices count in the Updates badge like every other item. After each install ChairLift reads the device's own state, not the update offer, so an update that fwupd did not apply is reported as an error and the device stays in the list; a staged update that needs a restart says so. Every call is unprivileged: the fwupd daemon authorizes the install with its own PolicyKit actions. Shown only when `fwupdmgr` is installed; enabled by default |

### Applications Page (`applications_page`): the Apps page

| Group | Key | Description |
|-------|-----|-------------|
| Software center | `applications_installed_group` | The button that opens the external software center used for discovery and installation |
| User Flatpak | `flatpak_user_group` | User-installed Flatpak applications in the Installed table, with update, open and uninstall actions |
| System Flatpak | `flatpak_system_group` | System-wide Flatpak applications in the Installed table, with update, open and uninstall actions |
| Homebrew | `brew_group` | Installed Homebrew formulae/casks in the Installed table with update and uninstall actions, formula pin/unpin actions, and the export to `~/Brewfile` |
| Brew Search | `brew_search_group` | The Find more tab: search and install Homebrew formulae and casks with an explicit package-type confirmation |
| Brew Bundles | `brew_bundles_group` | The Bundles tab: install packages from Brewfile bundles |

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

### Maintenance Page (`maintenance_page`): the Maintenance page

Each group is a row of the Clean up card; every cleanup asks first, and
shows its command and, as it runs, its output in the console.

| Group | Key | Description |
|-------|-----|-------------|
| Cleanup | `maintenance_cleanup_group` | Custom cleanup scripts |
| Homebrew Cleanup | `maintenance_brew_group` | `brew cleanup` (remove old versions and cache) |
| Flatpak Cleanup | `maintenance_flatpak_group` | `flatpak uninstall --unused` with no scope flag, so every installation (remove unused runtimes) |
| (deprecated) | `maintenance_optimization_group` | Renders nothing; the key stays valid so existing files still load |

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

### Features Page (`features_page`): the Features page

| Group | Key | Description |
|-------|-----|-------------|
| Features | `features_group` | Toggle and update system features managed by updex; also the Updates page's features source |

Feature operations (enable, disable, update) require administrator
authentication through PolicyKit and are performed by the fixed
`/usr/bin/chairlift-updex-helper` binary. ChairLift installs no passwordless
authorization rule.

### Problems Page (`problems_page`): the Problems page

Local problem records, with opt-in reporting that is off until the user turns
it on
([ADR-0015](adr/0015-problems-page-and-opt-in-reporting.md),
[specs/problems-page.md](specs/problems-page.md),
[specs/problem-reporting.md](specs/problem-reporting.md)). The page sits
between Features and Maintenance, so with every page visible it is `Alt+5`.

| Group | Key | Description |
|-------|-----|-------------|
| System | `problems_system_group` | Problems from `/var/lib/snosi-health/problems.json` (fixed path); "System checks are not available on this system" without the file |
| Apps | `problems_apps_group` | The user's crashes (`coredumpctl`, last 90 days) and frostyard app problem records (user journal) |
| Reporting | `problems_reporting_group` | The reporting card: the opt-in switch, the Ask/Auto mode, "Sent reports", "What is sent" and the weekly count; `endpoint` may be set only in an administrator-owned file |

Each problem card expands to a hint, the recorded values, the local details,
**Copy details**, **Dismiss**, and **Report…** while reporting is on. A
dismissed problem is kept in `$XDG_STATE_HOME/chairlift/problems-seen.json`
and shows again when it happens again. Under `--dry-run`, Dismiss changes
nothing, and **Report…** shows the exact report without sending it.

The reporting card:

| Control | What it does |
|-----|--------------|
| Send problem reports | The per-user consent switch, saved in `$XDG_CONFIG_HOME/chairlift/reporting.json`. Off by default; every failure reads as off |
| When a problem is found | **Ask each time** (only the Report dialog sends) or **Send automatically** (at most 20 reports in 24 hours, paced at 7 seconds) |
| Sent reports | Each sent report with its date, kind and `report_id`; newest first, at most 50 |
| What is sent | Opens `https://frostyard.org/privacy/problem-reports/` |
| Weekly count | In Ask mode, while this ISO week's count was not sent: **Review…** shows the count (install age bucket and update channel only) |

`chairlift --send-pending-reports` runs the automatic send with no window and
exits 1 for a configuration error or a failed send; `--dry-run` prints the
reports instead of sending them.

### Help Page (`help_page`): the Help dialog

Help is a dialog (F1, the main menu, the command palette), not a page.

| Group | Key | Description |
|-------|-----|-------------|
| Resources | `help_resources_group` | The Help dialog's links to project resources |

`help_resources_group` supports:

| Field | Description |
|-------|-------------|
| `website` | Project website URL |
| `issues` | Issue tracker URL |
| `chat` | Community chat or discussions URL |

### First-Run Page (`first_run_page`)

Not a page of the rail: it configures `chairlift --first-run`
([ADR-0014](adr/0014-first-run-mode.md)).

| Group | Key | Description |
|-------|-----|-------------|
| Apps step | `first_run_apps_group` | The first run's application picker |

`first_run_apps_group` supports:

| Field | Description |
|-------|-------------|
| `apps_path` | Application list (JSON); default `/usr/share/snow/first-run/apps.json` |

A disabled group, a configuration error, or a missing or invalid list hides
the Apps step; the user can still finish the first run.

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

All other groups keep their built-in state since they are not listed: enabled,
except `maintenance_cleanup_group`.
