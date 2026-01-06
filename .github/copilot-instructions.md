# GitHub Copilot Instructions for ChairLift

## Project Overview

ChairLift is a modern GTK4/Libadwaita system management tool originally designed for Snow Linux but made portable to other distributions. It provides a unified interface for:

- Homebrew package management
- System health monitoring
- Flatpak application management
- NBC (bootc container) system updates
- System updates and maintenance

## Architecture

### Technology Stack

- **UI Framework**: GTK4 + Libadwaita via [puregotk](https://github.com/jwijenbergh/puregotk)
- **Language**: Go
- **Build System**: Make
- **Configuration**: YAML-based

### Key Design Patterns

1. **Async Operations**: All long-running operations (package installation, searches, updates) run in goroutines with `runOnMainThread()` helper for UI updates via `glib.IdleAdd()`

2. **Configuration-Driven UI**: The application reads `config.yml` to dynamically show/hide UI groups and configure application launchers, making it portable across distributions

3. **Toast Notifications**: User feedback is provided via `adw.Toast` notifications accessed through the `ToastAdder` interface

4. **Dry-Run Mode**: All packages (homebrew, flatpak, nbc) support `--dry-run` flag to preview changes without modifying the system

### Project Structure

```
chairlift/
├── cmd/
│   └── chairlift/
│       └── main.go           # Application entry point
├── internal/
│   ├── app/
│   │   └── app.go            # Main application setup
│   ├── config/
│   │   └── config.go         # YAML configuration loading
│   ├── flatpak/
│   │   └── flatpak.go        # Flatpak integration
│   ├── homebrew/
│   │   └── homebrew.go       # Homebrew integration
│   ├── nbc/
│   │   └── nbc.go            # NBC bootc integration
│   ├── views/
│   │   └── userhome.go       # Page content and UI logic
│   └── window/
│       └── window.go         # Main window and navigation
├── data/                     # Desktop files, icons, policies
├── config.yml                # Default configuration
├── CONFIG.md                 # Configuration documentation
└── Makefile                  # Build commands
```

## Development Workflow

### After Any Code Changes

**Always run these commands after making changes:**

```bash
make fmt    # Format code with gofmt
make lint   # Run golangci-lint
make build  # Verify it compiles
```

### Build Commands

- `make build` - Build the application
- `make run` - Build and run with `--dry-run` flag
- `make fmt` - Format all Go code
- `make lint` - Run linter (requires golangci-lint)
- `make test` - Run tests
- `make clean` - Clean build artifacts

## Configuration System

**Critical**: ChairLift uses a YAML configuration file to control UI behavior. Always maintain this pattern when adding new features.

### Config File Locations (searched in order)

1. `/etc/chairlift/config.yml` (system-wide - highest priority)
2. `/usr/share/chairlift/config.yml` (package maintainer defaults)
3. `config.yml` (development/source directory)

### Adding Configurable Features

When adding new preference groups or external app integrations:

1. **Add to config.yml**:

```yaml
page_name:
  group_name:
    enabled: true
    app_id: com.example.App # for external apps
```

2. **Add to internal/config/config.go** (default config):

```go
PageConfig{
    "group_name": GroupConfig{Enabled: true},
}
```

3. **Read in views code**:

```go
if uh.config.IsGroupEnabled("page_name", "group_name") {
    // Build and add the group
}

// For app IDs:
groupCfg := uh.config.GetGroupConfig("page_name", "group_name")
appID := "default.app"
if groupCfg != nil && groupCfg.AppID != "" {
    appID = groupCfg.AppID
}
```

4. **Update documentation**: Add entries to CONFIG.md

## Coding Conventions

### Go Style

- Follow standard Go conventions
- Use meaningful variable names, especially for UI widgets
- Add comments to exported functions
- Keep functions focused and small

### Threading Pattern

Always follow this pattern for async operations:

```go
func (uh *UserHome) onActionClicked(button *gtk.Button) {
    button.SetSensitive(false)
    button.SetLabel("Processing...")

    go func() {
        // Do work in goroutine
        result, err := someOperation()

        runOnMainThread(func() {
            button.SetSensitive(true)
            button.SetLabel("Original Label")

            if err != nil {
                uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed: %v", err))
                return
            }
            uh.toastAdder.ShowToast("Success!")
        })
    }()
}
```

The `runOnMainThread()` helper uses `glib.IdleAdd()` to safely update UI from goroutines.

### UI Widget Creation

- Use `adw.NewPreferencesGroup()` for logical groupings
- Use `adw.NewActionRow()` for list items
- Use `adw.NewExpanderRow()` for collapsible sections
- Add icons with `gtk.NewImageFromIconName()`
- Store widget references in the struct for later updates

### Adding New Packages (internal modules)

When adding a new package manager integration (like the flatpak package):

1. Create `internal/packagename/packagename.go`
2. Implement `SetDryRun(bool)` and `IsDryRun() bool` for dry-run support
3. Implement `IsInstalled() bool` to check availability
4. Add `SetDryRun()` call in `internal/app/app.go` when dry-run is enabled
5. Import and use in `internal/views/userhome.go`

## Package Integration Examples

### Homebrew (internal/homebrew)

```go
if !homebrew.IsInstalled() {
    return
}

packages, err := homebrew.ListInstalledFormulae()
if err != nil {
    // Handle error
}

// Install (respects dry-run)
err = homebrew.Install("package-name", false)
```

### Flatpak (internal/flatpak)

```go
if !flatpak.IsInstalled() {
    return
}

// List user/system apps
userApps, _ := flatpak.ListUserApplications()
systemApps, _ := flatpak.ListSystemApplications()

// List available updates
updates, _ := flatpak.ListUpdates(true) // true = user, false = system

// Update (respects dry-run)
err = flatpak.Update("com.app.Id", true)
```

### NBC (internal/nbc)

```go
ctx, cancel := nbc.DefaultContext()
defer cancel()

status, err := nbc.GetStatus(ctx)
updateCheck, err := nbc.CheckUpdate(ctx)

// Update with progress callback
progressCh := make(chan nbc.ProgressEvent)
go nbc.Update(ctx, nbc.UpdateOptions{Auto: true}, progressCh)
```

## Portability Guidelines

ChairLift should work on any Linux distribution. When adding features:

1. **Make it configurable**: Don't hardcode distribution-specific paths or apps
2. **Provide defaults**: Default to Snow Linux behavior but allow overrides
3. **Handle missing dependencies gracefully**: Check if tools exist before using them
4. **Document in CONFIG.md**: Explain how to adapt for other distributions

## Testing

When making changes:

1. Test with and without Homebrew installed
2. Test with and without Flatpak installed
3. Test with different config.yml settings
4. Verify goroutines don't block UI
5. Check that errors show appropriate toast notifications
6. Test with `--dry-run` flag to ensure state-changing operations are blocked
7. Run `make fmt` and `make lint` before committing

## Common Pitfalls to Avoid

1. **Don't hardcode paths** - Use config.yml
2. **Don't hardcode app IDs** - Make them configurable
3. **Don't block the UI** - Use goroutines for long operations
4. **Don't forget error handling** - Always handle errors from goroutines
5. **Don't skip `runOnMainThread()`** - Required for UI updates from goroutines
6. **Don't forget `IsInstalled()` checks** - Before using any package manager
7. **Don't forget dry-run support** - State-changing operations must check `dryRun` flag
8. **Don't forget to run `make fmt` and `make lint`** - After every change

## Adding New Pages

To add a new page to the navigation:

1. Add page fields in `UserHome` struct
2. Create `buildNewPage()` method
3. Call `buildNewPage()` in `New()` constructor
4. Add to `GetPage()` switch statement
5. Add nav item in `window.go` navItems slice
6. Add config section to config.yml and config.go
7. Update CONFIG.md

## File Naming and Organization

- Application entry: `cmd/chairlift/main.go`
- Core app logic: `internal/app/`
- Configuration: `internal/config/`
- Package integrations: `internal/<package>/`
- UI views: `internal/views/`
- Window/navigation: `internal/window/`
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
- [puregotk](https://github.com/jwijenbergh/puregotk) - Pure Go GTK4 bindings
- [Adwaita documentation](https://gnome.pages.gitlab.gnome.org/libadwaita/doc/)
- [GTK4 documentation](https://docs.gtk.org/gtk4/)
