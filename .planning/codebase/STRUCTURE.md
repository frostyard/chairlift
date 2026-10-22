# Codebase Structure

**Analysis Date:** 2026-01-26
**Updated:** 2026-10-22 (removed obsolete package-manager library references, #234)

## Directory Layout

```
chairlift/
├── cmd/                    # Application entry points
│   └── chairlift/          # Main binary
├── internal/               # Internal packages (not importable externally)
│   ├── app/                # GTK Application lifecycle
│   ├── config/             # YAML configuration loading
│   ├── nbc/                # nbc CLI wrapper (bootc updates)
│   ├── flatpak/            # flatpak CLI wrapper (applications, updates)
│   ├── homebrew/           # brew CLI wrapper (packages, bundles, tap trust)
│   ├── updex/              # updex CLI wrapper (feature management)
│   ├── version/            # Build version info
│   ├── views/              # Page content and UI logic
│   └── window/             # Main window composition
├── data/                   # Static data files for installation
│   └── icons/              # Application icons (scalable/symbolic)
├── build/                  # Build output directory
├── dist/                   # Release artifacts
├── docs/                   # Documentation
├── .github/                # GitHub workflows
│   └── workflows/          # CI/CD definitions
└── .planning/              # Planning and analysis documents
    └── codebase/           # Codebase analysis (this file)
```

## Directory Purposes

**cmd/chairlift/:**
- Purpose: Binary entry point
- Contains: `main.go` only
- Key files: `cmd/chairlift/main.go`

**internal/app/:**
- Purpose: GTK Application wrapper and lifecycle
- Contains: Application struct, keyboard shortcuts, command-line options
- Key files: `internal/app/app.go`

**internal/config/:**
- Purpose: Configuration loading and defaults
- Contains: Config struct, YAML parsing, group enablement
- Key files: `internal/config/config.go`

**internal/views/:**
- Purpose: Page content builders and event handlers
- Contains: UserHome struct with all page building methods
- Key files: `internal/views/userhome.go` (large file, ~2000+ lines)

**internal/window/:**
- Purpose: Main window UI composition
- Contains: Window struct, navigation sidebar, content stack, dialogs
- Key files: `internal/window/window.go`

**internal/nbc/:**
- Purpose: NBC bootc installer CLI wrapper
- Contains: Command execution, JSON parsing, streaming progress
- Key files: `internal/nbc/nbc.go`

**internal/updex/:**
- Purpose: Updex feature manager CLI wrapper
- Contains: Feature listing, enable/disable, and update operations
- Key files: `internal/updex/updex.go`

**internal/flatpak/:**
- Purpose: Flatpak CLI wrapper
- Contains: Application and update listing, install/uninstall/update, unused removal, info and remotes with dry-run support
- Key files: `internal/flatpak/flatpak.go`

**internal/homebrew/:**
- Purpose: Homebrew CLI wrapper
- Contains: Formulae/cask listing, search, install/uninstall/upgrade/pin, cleanup, Brew bundle discovery, and tap trust with dry-run support
- Key files: `internal/homebrew/homebrew.go`, `internal/homebrew/bundles.go`, `internal/homebrew/trust.go`

**internal/version/:**
- Purpose: Build-time version information
- Contains: Version, Commit, Date variables
- Key files: `internal/version/version.go`

**data/:**
- Purpose: Static files installed with the application
- Contains: Desktop file, icons, PolicyKit policies/rules
- Key files: 
  - `data/org.frostyard.ChairLift.desktop`
  - `data/org.frostyard.ChairLift.nbc.policy`
  - `data/chairlift-wrapper.sh`

## Key File Locations

**Entry Points:**
- `cmd/chairlift/main.go`: Binary entry, version injection

**Configuration:**
- `config.yml`: Application configuration (development)
- `config.nbc-example.yml`: Example config for NBC-based systems
- `config.bootc-example.yml`: Example config for bootc systems
- `/etc/chairlift/config.yml`: Production config location

**Core Logic:**
- `internal/app/app.go`: Application lifecycle (129 lines)
- `internal/window/window.go`: Window composition (444 lines)
- `internal/views/userhome.go`: Page content and actions (2000+ lines)

**Tool Wrappers:**
- `internal/nbc/nbc.go`: NBC wrapper (515 lines)
- `internal/flatpak/flatpak.go`: Flatpak wrapper (488 lines)
- `internal/homebrew/homebrew.go`: Homebrew wrapper (473 lines)
- `internal/updex/updex.go`: Updex wrapper (178 lines)

**Build:**
- `Makefile`: Build, install, release targets
- `.goreleaser.yaml`: GoReleaser configuration
- `go.mod`: Go module definition

## Naming Conventions

**Files:**
- Single-file packages use package name: `app.go`, `config.go`
- Wrapper pattern: `wrapper.go` for facade packages

**Directories:**
- Lowercase, short names: `app`, `nbc`, `flatpak`
- Match package name exactly

**Go Packages:**
- Package name = directory name
- Exported types use PascalCase: `Application`, `UserHome`, `Config`
- Unexported functions use camelCase: `buildUI`, `loadOSRelease`

**Configuration Keys:**
- Snake_case in YAML: `system_page`, `nbc_updates_group`
- Page names: lowercase with underscores

## Where to Add New Code

**New Page:**
1. Add page field to `UserHome` struct in `internal/views/userhome.go`
2. Create `build{PageName}Page()` method
3. Call builder in `views.New()` goroutine
4. Add to `GetPage()` switch
5. Add NavItem in `internal/window/window.go`
6. Add page config section in `internal/config/config.go`

**New External Tool Integration:**
1. Create new package in `internal/{toolname}/`
2. Implement command execution with dry-run support
3. Add SetDryRun/IsDryRun functions
4. Call SetDryRun from `internal/app/app.go` during init
5. Use from views via goroutines with `runOnMainThread()` callbacks

**New Package Manager Support:**
1. Create a package in `internal/{tool}/` following `internal/flatpak` and `internal/homebrew`
2. Add `SetDryRun`/`IsDryRun`, and an availability check with a cached variant (`IsInstalledCached` using `sync.Once`)
3. Implement `commandTimeout(args)` from a `stateChangingCommands` map: 30s for reads, 30m for mutations
4. Call `SetDryRun` from `internal/app/app.go` during init
5. Use from views via goroutines with `sgtk.RunOnMainThread()` callbacks

**New Configuration Option:**
1. Add field to appropriate struct in `internal/config/config.go`
2. Set default in `defaultConfig()`
3. Use via `config.IsGroupEnabled()` or `config.GetGroupConfig()`

**Utilities:**
- Shared GTK helpers: Add to `internal/views/userhome.go` (e.g., `runOnMainThread`)
- Package-specific helpers: Keep in respective package

## Special Directories

**build/:**
- Purpose: Build output for local development
- Generated: Yes (`make build`)
- Committed: No (in .gitignore)

**dist/:**
- Purpose: Release artifacts from GoReleaser
- Generated: Yes (CI/release process)
- Committed: Partial (may contain release assets)

**data/icons/hicolor/:**
- Purpose: FreeDesktop icon theme structure
- Generated: No (manually created)
- Committed: Yes

**.planning/codebase/:**
- Purpose: Architecture and analysis documentation
- Generated: By GSD analysis tools
- Committed: Yes

---

*Structure analysis: 2026-01-26*
