# ChairLift Configuration

ChairLift can be configured to show or hide specific feature groups, making it more portable across different Linux distributions.

## Configuration File Location

ChairLift searches for the configuration file in the following locations (in order):

1. `chairlift/config.yml` (in the application source directory)
2. `/etc/chairlift/config.yml` (system-wide configuration)
3. `/usr/share/chairlift/config.yml` (installed configuration)

If no configuration file is found, all features are enabled by default.

## Configuration Format

The configuration file uses YAML format with a simple structure:

```yaml
page_name:
  group_name:
    enabled: true/false
```

## Available Pages and Groups

### System Page (`system_page`)
- `system_info_group`: Operating system information from /etc/os-release
- `health_group`: System health monitoring and performance tools
  - `app_id`: Application ID for the system monitoring tool (default: `io.missioncenter.MissionCenter`)

### Updates Page (`updates_page`)
- `updates_status_group`: System-wide updates
- `brew_updates_group`: Homebrew package updates and outdated packages
- `updates_settings_group`: Update preferences and settings

### Applications Page (`applications_page`)
- `applications_installed_group`: Flatpak application management
  - `app_id`: Application ID for the Flatpak manager (default: `io.github.kolunmi.Bazaar`)
- `brew_group`: Installed Homebrew formulae and casks
- `brew_search_group`: Search and install Homebrew packages
- `brew_bundles_group`: Curated Homebrew package bundles
  - `bundles_paths`: Array of directory paths to search for Brewfile bundles (default: `['/usr/share/snow/bundles']`)

### Maintenance Page (`maintenance_page`)
- `maintenance_cleanup_group`: System cleanup utilities
- `maintenance_optimization_group`: System optimization tools

### Help Page (`help_page`)
- `help_resources_group`: Help and support resources
  - `website`: URL to the project website
  - `issues`: URL to the issue tracker for bug reports and feature requests
  - `chat`: URL to community chat or discussions

## Example: Disabling Homebrew Features

To create a distribution-specific configuration that disables all Homebrew features:

```yaml
updates_page:
  updates_status_group:
    enabled: true
  brew_updates_group:
    enabled: false  # Hide Homebrew updates
  updates_settings_group:
    enabled: true

applications_page:
  applications_installed_group:
    enabled: true
  brew_group:
    enabled: false  # Hide Homebrew packages
  brew_search_group:
    enabled: false  # Hide Homebrew search
  brew_bundles_group:
    enabled: false  # Hide Homebrew bundles

# Other pages remain fully enabled
system_page:
  system_info_group:
    enabled: true
  health_group:
    enabled: true

maintenance_page:
  maintenance_cleanup_group:
    enabled: true
  maintenance_optimization_group:
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
install -D -m 644 config.yml debian/tmp/etc/chairlift/config.yml
```

## Notes

- Groups with `enabled: false` will be completely hidden from the UI
- Missing entries default to `enabled: true`
- Invalid or unreadable configuration files will fall back to all features enabled
- Changes require restarting ChairLift to take effect
