# ChairLift Configuration

ChairLift can be configured to show or hide specific feature groups, making it more portable across different Linux distributions.

ChairLift's window is a command center: a left rail of six pages (Overview,
Updates, Apps, Features, Problems, Maintenance), a command palette, a console
and a status bar, with Help as a dialog. The configuration keeps its page
names (`system_page`, `applications_page`, ...), which no longer match the
window's pages one to one; each section below says where its groups show.

## Configuration File Location

ChairLift searches for the configuration file in the following locations (in order):

1. `/etc/chairlift/config.yml` (system-wide configuration - highest priority)
2. `/usr/share/chairlift/config.yml` (package maintainer defaults installed by
   source and nFPM packages)
3. `config.yml` beside the ChairLift executable, or in the current working
   directory when no executable-relative file exists (development fallback)

If no configuration file is found, all features default to enabled, except
`maintenance_cleanup_group`, which defaults to disabled.

Packages own and may replace the `/usr/share` defaults during an upgrade.
Administrators should put local changes in `/etc/chairlift/config.yml`;
ChairLift's install and packaging paths never create or overwrite that file.
The `frostyard-chairlift-system-integration` package also provides the
`/usr/share` defaults for a user-scoped GUI installation.

The first file that exists in this order is authoritative. ChairLift does not
fall through to a lower-priority file when that file is unreadable, malformed,
or fails schema validation. A symlink whose target is missing counts as an
existing, unreadable file: it fails closed rather than falling through. Instead, it disables every feature group, logs a
high-signal `CONFIGURATION ERROR`, and displays a persistent error toast naming
the file and cause. Fix the authoritative file and restart ChairLift.

These semantics are decision records
[ADR-0003](docs/adr/0003-two-tier-config-with-fail-closed-semantics.md)
(search order and fail-closed behavior) and
[ADR-0004](docs/adr/0004-configuration-error-diagnostic-vocabulary.md)
(the diagnostic vocabulary).

## Configuration Format

The configuration file uses YAML format with a simple structure:

```yaml
page_name:
  group_name:
    enabled: true/false
```

Every group gates its own card, tab or update source. When every group that
a page of the window shows is disabled, that page is omitted from the rail,
the command palette's "Go to" commands, the shortcuts dialog and the
Alt+number bindings, and the remaining Alt+number shortcuts compact in rail
order. The Overview always shows, so the window always has a page. Help is a
dialog (F1), not a page: it is always reachable, and with
`help_resources_group` disabled it lists no links.

## Available Pages and Groups

### System Page (`system_page`): the Overview

- `system_info_group`: The System details card: this computer (host name,
  model, kernel, Secure Boot, uptime, image) and the fields of
  `/etc/os-release`
- `bootc_status_group`: The system image hero with the deployment chain
  (rollback, booted, staged) on a bootc or native A/B host, and the
  read-only deployment chain on the Maintenance page
- `health_group`: The live processor and memory chart and the button that
  opens the system monitor
  - `app_id`: A desktop application ID or an ordered list. The default tries
    GNOME System Monitor, then KDE System Monitor. The first
    installed application visible in the desktop menu is used. An empty
    string or list uses the default order.

### Updates Page (`updates_page`): the Updates page

Each group is an update source of the Updates page; the Features source
follows `features_page`'s `features_group`. A source shows only where its
tool applies, and "Update everything" runs every source except firmware.

- `bootc_updates_group`: The system image on bootc installs, staged for the
  next restart
- `sysupdate_updates_group`: The system image on native A/B (systemd-sysupdate) installs, staged for the next restart; shown only on native A/B installs
- `flatpak_updates_group`: Available Flatpak application updates (user and system)
- `brew_updates_group`: Outdated Homebrew packages, and the update of Homebrew itself
- `brew_trust_group`: Untrusted Homebrew taps with installed packages (Homebrew 6 tap trust), as banners on the Homebrew card; only shown when there is something to trust
- `firmware_updates_group`: Device firmware updates from the Linux Vendor
  Firmware Service (fwupd), installed one device at a time after a
  confirmation; only shown when the `fwupdmgr` client is installed

### Applications Page (`applications_page`): the Apps page

- `applications_installed_group`: The Apps page's button that opens the
  external software center for discovering and installing new
  applications; ChairLift itself lists, updates, opens and uninstalls
  installed Flatpaks but does not directly install them
  - `app_id`: Application ID for the software center (default: `io.github.kolunmi.Bazaar`)
- `flatpak_user_group`: User-installed Flatpak applications in the Installed table
- `flatpak_system_group`: System-wide Flatpak applications in the Installed table
- `brew_group`: Installed Homebrew formulae/casks in the Installed table, with
  uninstall actions, formula pin/unpin actions, and the export to `~/Brewfile`
- `brew_search_group`: The Find more tab: search and install Homebrew formulae and casks
- `brew_bundles_group`: The Bundles tab: curated Homebrew package bundles
  - `bundles_paths`: Array of directories searched for immediate
    `*.Brewfile` entries (default: `['/usr/share/snow/bundles']`). Missing
    directories are ignored so one configuration can cover multiple
    distribution variants. Other unreadable paths are reported in the group
    while bundles from readable paths remain available. Repeating the same
    path does not duplicate a row; same-named Brewfiles in different
    directories remain distinct and show their full paths.

### Maintenance Page (`maintenance_page`): the Maintenance page

Each group is a row of the Clean up card; every cleanup asks first, and
"Clean everything" runs them one after the other.

- `maintenance_cleanup_group`: System cleanup scripts (disabled by default)
  - `actions`: Array of maintenance scripts that can be executed
    - `title`: Display name for the action
    - `script`: Absolute path to the script to execute
    - `sudo`: Boolean indicating if the script requires administrator privileges (uses pkexec)
- `maintenance_brew_group`: Homebrew cleanup (runs `brew cleanup` to remove old versions and cache)
- `maintenance_flatpak_group`: Flatpak cleanup (runs `flatpak uninstall --unused` with no scope flag, which covers every installation, to remove unused runtimes)
- `maintenance_optimization_group`: **Deprecated.** It showed only a "Coming soon"
  placeholder and now renders nothing. The key stays valid, so an existing
  file that names it still loads; remove it from your configuration.

### Features Page (`features_page`): the Features page

- `features_group`: System features managed by updex: the Features page and
  the Updates page's features source (requires features configured for
  updex)

### Problems Page (`problems_page`): the Problems page

The Problems page shows problems recorded on this computer
([ADR-0015](docs/adr/0015-problems-page-and-opt-in-reporting.md)). It sits
between Features and Maintenance in the rail. Each group can be turned off; with all 3
off the page is omitted, as after a configuration error. The snosi-health
paths are fixed and cannot be configured, and the report endpoint is the only
field of this page (`endpoint`, administrator-owned files only).

- `problems_system_group`: Problems that Snow's `snosi-health` checks
  recorded in `/var/lib/snosi-health/problems.json` (and their local details
  from `details.json` when the user may read it). Without that file the
  group shows "System checks are not available on this system".
- `problems_apps_group`: Crashes of the user's own programs (from
  `coredumpctl`, the last 90 days) and problems that frostyard apps recorded
  in the user journal.
- `problems_reporting_group`: Opt-in problem reporting, the reporting card
  beside the problem lists. Nothing is sent until the user turns the switch
  on; the choice and the mode live in the user's own
  consent file, not in this configuration
  ([specs/problem-reporting.md](docs/specs/problem-reporting.md)). The
  optional first-run Reporting step shows only while this group is enabled and
  `/var/lib/snosi-health/problems.json` exists.
  - `endpoint`: the report intake URL (default
    `https://report.frostyard.org/v1/report`). Only
    `/etc/chairlift/config.yml` and `/usr/share/chairlift/config.yml` may set
    it: ChairLift ignores the value from the development fallback beside the
    executable or in the working directory (logged once), because a
    user-writable file must not choose where a report goes.

### Help Page (`help_page`): the Help dialog

Help is a dialog (F1, the main menu, the command palette), not a page.

- `help_resources_group`: The Help dialog's links
  - `website`: URL to the project website
  - `issues`: URL to the issue tracker for bug reports and feature requests
  - `chat`: URL to community chat or discussions

### First-Run Page (`first_run_page`)

This page is not in the rail. It configures `chairlift --first-run`
([ADR-0014](docs/adr/0014-first-run-mode.md)).

- `first_run_apps_group`: The first run's Apps step (application picker)
  - `apps_path`: Path of the application list (default:
    `/usr/share/snow/first-run/apps.json`). The list is JSON in SNOW First
    Setup's shape: `{"<category>": [{"name": "...", "id": "<Flatpak app ID>",
    "active": false}]}`; an absent `active` means the app starts selected.

The Apps step shows only when this group is enabled, the list is valid, and
the connection check found the internet. A disabled group, a configuration
error, or a missing or invalid list hides the step; the user can still finish
the first run.

## Example: Disabling Homebrew Features

To create a distribution-specific configuration that disables all Homebrew features:

```yaml
updates_page:
  bootc_updates_group:
    enabled: true
  flatpak_updates_group:
    enabled: true # Keep Flatpak updates
  brew_updates_group:
    enabled: false # Hide Homebrew updates

applications_page:
  applications_installed_group:
    enabled: true
  flatpak_user_group:
    enabled: true
  flatpak_system_group:
    enabled: true
  brew_group:
    enabled: false # Hide Homebrew packages
  brew_search_group:
    enabled: false # Hide Homebrew search
  brew_bundles_group:
    enabled: false # Hide Homebrew bundles

# Other pages remain fully enabled
system_page:
  system_info_group:
    enabled: true
  health_group:
    enabled: true

maintenance_page:
  maintenance_cleanup_group:
    enabled: true
  maintenance_brew_group:
    enabled: true
  maintenance_flatpak_group:
    enabled: true

help_page:
  help_resources_group:
    enabled: true
```

## Deployment

### For System Administrators

To customize ChairLift for your distribution:

1. Create a custom `config.yml` with your desired settings
2. Install it to `/etc/chairlift/config.yml` for system-wide configuration
3. Package maintainers can include it in `/usr/share/chairlift/config.yml`

### For Package Maintainers

When packaging ChairLift for different distributions:

1. Copy the default `config.yml` to your package
2. Modify it to match your distribution's available features
3. Install it to the appropriate location during package installation

Example for Debian packaging:

```bash
install -D -m 644 config.yml debian/tmp/usr/share/chairlift/config.yml
```

## Notes

- Groups with `enabled: false` are completely hidden from the UI, and their data is never loaded
- A configuration file is applied as an overlay on top of the built-in
  defaults, field by field, not as a full replacement:
  - An omitted `enabled` key inherits that group's documented default
    (`true` for every group except `maintenance_cleanup_group`, which
    defaults to `false`)
  - An omitted optional field (`app_id`, `website`, `issues`, `chat`,
    `actions`, `bundles_paths`, `apps_path`) inherits its documented default value
  - An explicit empty list (e.g. `actions: []`) clears the field
  - A non-empty list, or an explicitly set scalar value, replaces the
    default outright
- Page names, group names, and field names must match the documented schema;
  unknown names and values of the wrong type are configuration errors (the
  schema is derived from the canonical config struct — decision record
  [ADR-0005](docs/adr/0005-config-schema-reflected-from-canonical-struct.md))
- An unreadable (including a dangling symlink), malformed, or schema-invalid
  authoritative file fails closed:
  all feature groups are hidden, lower-priority files are ignored, and
  ChairLift reports the path and cause in both its log and a persistent toast
- Changes require restarting ChairLift to take effect
