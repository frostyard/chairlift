# External Integrations

**Analysis Date:** 2026-01-26
**Updated:** 2026-10-22 (removed obsolete package-manager library references, #234)

## APIs & External Services

**Package Management (direct CLI wrappers, no client library):**
- Flatpak - List applications, list updates, install, uninstall, update, remove unused runtimes, inspect applications and remotes
  - CLI: `flatpak list`, `flatpak remote-ls --updates`, `flatpak install -y`, `flatpak uninstall -y`, `flatpak update -y`, `flatpak uninstall --unused -y`, `flatpak info --show-metadata`, `flatpak remotes`
  - Implementation: `internal/flatpak/flatpak.go`
  - Auth: None (unprivileged, no pkexec)
  - Operations: `ListUserApplications`, `ListSystemApplications`, `ListUpdates`, `Install`, `Uninstall`, `Update`, `UninstallUnused`, `GetRemotes`, `Info`

- Homebrew - List installed formulae and casks, list outdated, search, install, uninstall, upgrade, update, pin/unpin, cleanup, Brew bundles, tap trust
  - CLI: `brew info --installed --json=v2`, `brew outdated --json=v2`, `brew search --formula|--cask`, `brew install|uninstall [--cask]`, `brew upgrade`, `brew update`, `brew pin|unpin`, `brew cleanup`, `brew bundle dump|install`, `brew trust`, `brew tap-info --installed --json`
  - Implementation: `internal/homebrew/homebrew.go`, `internal/homebrew/bundles.go`, `internal/homebrew/trust.go`
  - Auth: None (unprivileged, no pkexec); `brew trust` is per-user state in `~/.homebrew/trust.json`
  - Operations: `ListInstalledFormulae`, `ListInstalledCasks`, `ListOutdated`, `Search`, `Install`, `Uninstall`, `Upgrade`, `Update`, `Pin`, `Unpin`, `Cleanup`, `BundleDump`, `BundleInstall`, `AvailableBundles`, `ListUntrustedTaps`, `TrustPackages`

Snap support was removed (PR #52). ChairLift has no Snap operations.

See `../../docs/design/package-managers.md` for the canonical description of
both wrappers, including timeouts, error handling, and dry-run behavior.

**NBC Bootc Container System:**
- NBC (`nbc`) - System status, update checking, updates, cache management
  - SDK/Client: `github.com/frostyard/nbc/pkg/types` (types only)
  - Implementation: `internal/nbc/nbc.go`
  - Commands executed: `nbc status`, `nbc update`, `nbc download`, `nbc list`, `nbc validate`, `nbc cache`
  - Auth: PolicyKit (pkexec) for state-changing operations
  - Progress: JSON Lines streaming for long operations

**System Features (via updex):**
- updex - List, enable, disable, and update system features
  - Implementation: `internal/updex/updex.go`
  - Commands executed: `updex features list --json`, `updex features enable <name>`, `updex features disable <name>`, `updex features update`, `updex --version`
  - Auth: PolicyKit (pkexec) for enable/disable/update operations; read-only for listing

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
  - Used for: nbc operations, updex feature enable/disable/update

**Operations Requiring Elevation:**
- NBC: update, download, install, cache clear/remove
- Features: updex enable, disable, update
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

**Package Managers (direct `os/exec` calls):**
- `flatpak` - `internal/flatpak`, all operations
- `brew` - `internal/homebrew`, all operations including cleanup, bundles, tap trust, and outdated

**System Tools:**
- `nbc` - NBC bootc operations (with pkexec for state changes)
- `updex` - Feature listing, enable/disable/update (with pkexec for state changes)
- `pkexec` - PolicyKit privilege escalation
- `gtk-launch` - Launch desktop applications

**Progress Reporting:**
- NBC commands stream JSON Lines (`--json` flag)
- Flatpak and Homebrew commands return their final output; neither streams progress events

## Dry-Run Mode

**Supported by all integrations:**
- Command line flag: `--dry-run` or `-d`
- Each integration module has `SetDryRun(mode bool)` function
- State-changing operations logged but not executed
- Set in: `internal/app/app.go` startup

---

*Integration audit: 2026-01-26*
