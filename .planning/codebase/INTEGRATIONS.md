# External Integrations

**Analysis Date:** 2026-01-26

## APIs & External Services

**Package Management (via frostyard/pm library):**
- Flatpak - Install, uninstall, update, list packages
  - SDK/Client: `github.com/frostyard/pm` (Lister, Installer, Uninstaller, Upgrader interfaces)
  - Auth: None (uses system flatpak command)
  - Wrapper: `internal/pm/wrapper.go`

- Snap - Install, list packages
  - SDK/Client: `github.com/frostyard/pm`
  - Auth: None (uses system snapd)
  - Wrapper: `internal/pm/wrapper.go`

- Homebrew - Install, uninstall, update, upgrade, search, cleanup
  - SDK/Client: `github.com/frostyard/pm`
  - Auth: None (uses system brew command)
  - Wrapper: `internal/pm/wrapper.go`

**NBC Bootc Container System:**
- NBC (`nbc`) - System status, update checking, updates, cache management
  - SDK/Client: `github.com/frostyard/nbc/pkg/types` (types only)
  - Implementation: `internal/nbc/nbc.go`
  - Commands executed: `nbc status`, `nbc update`, `nbc download`, `nbc list`, `nbc validate`, `nbc cache`
  - Auth: PolicyKit (pkexec) for state-changing operations
  - Progress: JSON Lines streaming for long operations

**Systemd-sysext Extensions:**
- updex - List installed systemd-sysext extensions
  - Implementation: `internal/updex/updex.go`
  - Commands executed: `updex list --json`, `updex --version`
  - Auth: None (read-only)

- instex - Discover and install extensions from remote repositories
  - Implementation: `internal/instex/instex.go`
  - Commands executed: `instex discover <url> --json`, `instex install <url> --component <name>`
  - Auth: PolicyKit (pkexec) for installation
  - Default repository: `https://repository.frostyard.org`

## Data Storage

**Databases:**
- None - No database used

**File Storage:**
- Configuration files only (YAML)
- Reads system files: `/etc/os-release`, `/run/nbc-booted`

**Caching:**
- In-memory caching for package manager availability status
- NBC image cache managed by `nbc` command

## Authentication & Identity

**Auth Provider:**
- PolicyKit (pkexec) for privileged operations
  - PolicyKit policies in `data/org.frostyard.ChairLift.*.policy`
  - PolicyKit rules in `data/org.frostyard.ChairLift.*.rules`
  - Used for: nbc operations, instex install, snap operations

**Operations Requiring Elevation:**
- NBC: update, download, install, cache clear/remove
- Extensions: instex install
- System cleanup scripts with `sudo: true` in config

## Monitoring & Observability

**Error Tracking:**
- None (logs to stdout/stderr via `log` package)

**Logs:**
- Standard Go logging: `log.Printf()`, `log.Println()`
- Log flags: `log.LstdFlags | log.Lshortfile`

## CI/CD & Deployment

**Hosting:**
- GitHub (source code, releases)
- Frostyard repository (package distribution via R2)

**CI Pipeline:**
- GitHub Actions
  - `.github/workflows/test.yml` - Lint, unit tests, race detection, build verification
  - `.github/workflows/release.yml` - GoReleaser builds on tag push
  - `.github/workflows/snapshot.yml` - Nightly/snapshot builds

**Release Process:**
1. Run `make bump` to create new tag
2. GitHub Actions triggers on tag push
3. GoReleaser Pro builds deb/rpm/apk packages
4. Packages published to Frostyard repository via R2

## Environment Configuration

**Required env vars:**
- None for runtime

**Build/Release env vars:**
- `GORELEASER_KEY` - GoReleaser Pro license key
- `GITHUB_TOKEN` - GitHub API access for releases
- `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY` - Cloudflare R2 storage
- `CLOUDFLARE_ZONE`, `CLOUDFLARE_API_TOKEN` - Cache purging
- `REPOGEN_GPG_KEY` - Package signing

**Secrets location:**
- GitHub Actions secrets for CI/CD
- Local `.envrc` for development (GORELEASER_KEY)

## Webhooks & Callbacks

**Incoming:**
- None

**Outgoing:**
- None

## External Commands Executed

**Package Managers (via exec.Command):**
- `flatpak` - via pm library
- `snap` - via pm library
- `brew` - via pm library, direct calls for cleanup/bundle/outdated

**System Tools:**
- `nbc` - NBC bootc operations (with pkexec for state changes)
- `updex` - Extension listing
- `instex` - Extension discovery/installation (with pkexec)
- `pkexec` - PolicyKit privilege escalation
- `gtk-launch` - Launch desktop applications

**Progress Reporting:**
- NBC commands stream JSON Lines (`--json` flag)
- PM library uses ProgressReporter interface

## Dry-Run Mode

**Supported by all integrations:**
- Command line flag: `--dry-run` or `-d`
- Each integration module has `SetDryRun(mode bool)` function
- State-changing operations logged but not executed
- Set in: `internal/app/app.go` startup

---

*Integration audit: 2026-01-26*
