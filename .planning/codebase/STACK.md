# Technology Stack

**Analysis Date:** 2026-01-26

## Languages

**Primary:**

- Go 1.25.6 - All application code

**Secondary:**

- YAML - Configuration files (`config.yml`, `.goreleaser.yaml`)
- Bash - Wrapper scripts (`data/chairlift-wrapper.sh`)

## Runtime

**Environment:**

- Linux only (GTK4/Libadwaita desktop environment required)
- No CGO required (uses puregotk for GTK bindings)

**Package Manager:**

- Go modules
- Lockfile: `go.sum` present

## Frameworks

**Core:**

- `github.com/jwijenbergh/puregotk` v0.0.0 - Pure Go GTK4/Libadwaita bindings (no CGO)
- GTK4 + Libadwaita (Adw) - Modern GNOME UI framework

**Configuration:**

- `gopkg.in/yaml.v3` v3.0.1 - YAML configuration parsing

**Internal Libraries (Frostyard):**

- `github.com/frostyard/nbc` v0.14.0 - NBC bootc container installer types
- `github.com/frostyard/pm` v0.2.1 - Package manager abstraction (Flatpak, Snap, Homebrew)
- `github.com/frostyard/pm/progress` v0.1.0 - Progress reporting for package operations

**Build/Release:**

- GoReleaser Pro v2 - Builds, packaging (deb, rpm, apk), and releases

**Linting:**

- golangci-lint (latest) - Go linting

## Key Dependencies

**Critical:**

- `puregotk` - Enables GTK4/Libadwaita UI without CGO
- `frostyard/nbc` - Provides types for NBC bootc system status and updates
- `frostyard/pm` - Unified package manager interface for Flatpak, Snap, Homebrew

**Infrastructure:**

- `google/uuid` v1.6.0 - UUID generation (indirect)
- `golang.org/x/text` v0.33.0 - Text processing, title casing

## Configuration

**Environment:**

- `GORELEASER_KEY` - Required for GoReleaser Pro releases (in `.envrc`)
- No other runtime environment variables required

**Application Configuration:**

- YAML-based configuration (`config.yml`)
- Search paths (in order):
  1. `/etc/chairlift/config.yml`
  2. `/usr/share/chairlift/config.yml`
  3. `config.yml` (relative to executable)
- Defaults to all features enabled if no config found

**Build Configuration:**

- `go.mod` - Go module definition
- `Makefile` - Build, test, install targets
- `.goreleaser.yaml` - Release packaging configuration
- `.svu.yaml` - Semantic versioning configuration

## Build Commands

```bash
make build          # Build binary to build/chairlift (CGO_ENABLED=0)
make run            # Build and run with --dry-run
make test           # Run all tests
make dev            # Build with race detector (requires CGO)
make fmt            # Format code
make lint           # Run golangci-lint
make install        # Install binary and data files
make uninstall      # Remove installed files
make bump           # Create new version tag and push
```

## Platform Requirements

**Development:**

- Go 1.25+ (CI) / 1.25.6 (go.mod)
- GTK4 and Libadwaita libraries installed
- make
- Optional: golangci-lint, svu

**Production:**

- Linux (x86_64 or arm64)
- GTK4 + Libadwaita runtime libraries
- PolicyKit (for privileged operations via pkexec)
- Optional external tools:
  - `nbc` - NBC bootc management
  - `updex` - systemd-sysext extension listing
  - `instex` - systemd-sysext extension installation
  - `flatpak` - Flatpak package management
  - `snap` - Snap package management
  - `brew` - Homebrew package management

**Deployment:**

- Native packages: deb, rpm, apk (via GoReleaser)
- Installation includes:
  - Binary: `/usr/local/bin/chairlift`
  - Wrapper: `/usr/local/bin/chairlift-wrapper`
  - Desktop file: `/usr/share/applications/org.frostyard.ChairLift.desktop`
  - Icons: `/usr/share/icons/hicolor/...`
  - PolicyKit policies: `/usr/share/polkit-1/actions/...`
  - PolicyKit rules: `/usr/share/polkit-1/rules.d/...`

## Version Information

- Build information injected via ldflags:
  - `main.buildVersion`
  - `main.buildCommit`
  - `main.buildDate`
  - `main.buildBy`
- Stored in `internal/version/version.go`

---

_Stack analysis: 2026-01-26_
