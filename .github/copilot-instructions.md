# GitHub Copilot Instructions for ChairLift

## Project Overview

ChairLift is a modern GTK4/Libadwaita system management tool originally designed for Snow Linux but made portable to other distributions. It provides a unified interface for:

- Homebrew package management
- System health monitoring
- Flatpak application management
- System updates and maintenance

## Architecture

### Technology Stack

- **UI Framework**: GTK4 + Libadwaita
- **Language**: Python 3
- **Build System**: Meson
- **Configuration**: YAML-based

### Key Design Patterns

1. **Async Operations**: All long-running operations (package installation, searches, updates) run in background threads using Python's `threading` module with `GLib.idle_add()` for UI updates

2. **Configuration-Driven UI**: The application reads `config.yml` to dynamically show/hide UI groups and configure application launchers, making it portable across distributions

3. **Toast Notifications**: User feedback is provided via `Adw.Toast` notifications accessed through `self.__window.add_toast()`

### Project Structure

```
chairlift/
├── chairlift/
│   ├── core/
│   │   └── homebrew.py       # Homebrew integration library
│   ├── views/
│   │   └── user_home.py      # Main UI implementation
│   ├── gtk/                  # UI templates
│   ├── config.yml            # Default configuration
│   └── application.py        # Main application class
├── data/                     # Desktop files, icons, policies
├── CONFIG.md                 # Configuration documentation
└── Justfile                  # Development commands
```

## Configuration System

**Critical**: ChairLift uses a YAML configuration file to control UI behavior. Always maintain this pattern when adding new features.

### Config File Locations (searched in order)

1. `/etc/chairlift/config.yml` (system-wide - highest priority)
2. `/usr/share/chairlift/config.yml` (package maintainer defaults)
3. `chairlift/config.yml` (development/source directory)

### Adding Configurable Features

When adding new preference groups or external app integrations:

1. **Add to config.yml**:

```yaml
page_name:
  group_name:
    enabled: true
    app_id: com.example.App # for external apps
```

2. **Read in code**:

```python
if self.__is_group_enabled('page_name', 'group_name'):
    self.page.add(group)

# For app IDs:
app_id = self.__config.get('page_name', {}).get('group_name', {}).get('app_id', 'default.app')
```

3. **Update documentation**: Add entries to CONFIG.md and config.example.yml

## Coding Conventions

### Python Style

- Use double underscore prefix for private methods: `__method_name()`
- Use meaningful variable names, especially for UI widgets
- Add docstrings to all methods
- Use `_()` function for all user-facing strings (internationalization)

### Threading Pattern

Always follow this pattern for async operations:

```python
def __on_action_clicked(self, button):
    button.set_sensitive(False)
    button.set_label(_("Processing..."))

    def action_in_thread():
        try:
            # Do work
            return {'success': True, 'message': _("Success")}
        except Exception as e:
            return {'success': False, 'message': str(e)}

    def on_action_complete(result):
        button.set_sensitive(True)
        button.set_label(_("Original Label"))

        if hasattr(self.__window, 'add_toast'):
            toast = Adw.Toast.new(result['message'])
            toast.set_timeout(3)
            self.__window.add_toast(toast)

    import threading
    def run():
        result = action_in_thread()
        GLib.idle_add(lambda: on_action_complete(result))

    thread = threading.Thread(target=run, daemon=True)
    thread.start()
```

### UI Widget Creation

- Use Adw.PreferencesGroup for logical groupings
- Use Adw.ActionRow for list items
- Use Adw.ExpanderRow for collapsible sections
- Add icons with `Gtk.Image.new_from_icon_name()`
- Make URLs clickable with `row.connect("activated", self.__on_url_row_activated, url)`

## Homebrew Integration

The `chairlift.core.homebrew` module provides all Homebrew functionality:

- `is_homebrew_installed()` - Check if Homebrew is available
- `list_installed_packages()` - Get installed formulae/casks
- `search_formula()` - Search for packages
- `install_bundle()` - Install from Brewfile
- `pin_package()` / `unpin_package()` - Pin management
- All functions may raise `homebrew.HomebrewError`

**Always check** `homebrew.is_homebrew_installed()` before Homebrew operations.

## Portability Guidelines

ChairLift should work on any Linux distribution. When adding features:

1. **Make it configurable**: Don't hardcode distribution-specific paths or apps
2. **Provide defaults**: Default to Snow Linux behavior but allow overrides
3. **Handle missing dependencies gracefully**: Check if apps/tools exist before using them
4. **Document in CONFIG.md**: Explain how to adapt for other distributions

## Development Environment

- Uses Distrobox with Debian Trixie container
- Just command runner for common tasks
- Run `just setup` to create dev environment
- Run `just enter` to enter the container
- Run `just build` and `just run` to test

## Testing

When making changes:

1. Test with and without Homebrew installed
2. Test with different config.yml settings
3. Verify threading doesn't block UI
4. Check that errors show appropriate toast notifications
5. Test on a system without Snow Linux-specific features

## Common Pitfalls to Avoid

1. **Don't hardcode paths** - Use config.yml
2. **Don't hardcode app IDs** - Make them configurable
3. **Don't block the UI** - Use threading for long operations
4. **Don't forget error handling** - Always catch exceptions in threads
5. **Don't skip `GLib.idle_add()`** - Required for UI updates from threads
6. **Don't forget to check `is_homebrew_installed()`** before Homebrew calls
7. **Don't forget internationalization** - Wrap strings in `_()`

## Adding New Pages

To add a new page to the navigation:

1. Create `__build_new_page()` method
2. Add page widget in `__init__`: `self.new_page_widget = self.__create_page()`
3. Add to `get_page()` dictionary
4. Add config section to config.yml
5. Register in window.py navigation
6. Update CONFIG.md

## Package Dependencies

When adding features that require new Python packages:

1. Add to `debian/control` Dependencies
2. Add to README.md Dependencies section
3. Add to distrobox.ini for development
4. Import at the top of the file (not lazy imports)

## File Naming and Organization

- UI implementation: `chairlift/views/`
- Core logic: `chairlift/core/`
- UI templates: `chairlift/gtk/` (if using .ui files)
- Assets: `chairlift/assets/`
- Keep files focused on single responsibility

## Commit Message Convention

Use conventional commits format:

- `feat:` - New features
- `fix:` - Bug fixes
- `docs:` - Documentation changes
- `chore:` - Maintenance tasks
- `refactor:` - Code refactoring
- Always sign commits with `-s` flag

## Additional Resources

- CONFIG.md - Complete configuration documentation
- README.md - User and developer documentation
- Adwaita documentation: https://gnome.pages.gitlab.gnome.org/libadwaita/doc/
- GTK4 documentation: https://docs.gtk.org/gtk4/
