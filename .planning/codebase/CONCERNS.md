# Codebase Concerns

**Analysis Date:** 2026-01-26

## Tech Debt

**Massive View File (God Object):**
- Issue: `internal/views/userhome.go` is 2499 lines handling all page construction, callbacks, and state management
- Files: `internal/views/userhome.go`
- Impact: Hard to maintain, test, or modify without introducing bugs. High cognitive load for developers
- Fix approach: Split into separate files per page (system.go, updates.go, applications.go, etc.) or extract into sub-packages. Consider a presenter/controller pattern

**Unimplemented Features (Stubs):**
- Issue: Several placeholder functions that do nothing
- Files:
  - `internal/views/userhome.go:1345-1350` - `openURL()` logs but never opens URLs
  - `internal/views/userhome.go:1347-1350` - `runMaintenanceAction()` has `// TODO: Execute the script`
  - `internal/views/userhome.go:964-970` - "Optimization tools - Coming soon" placeholder
  - `internal/pm/wrapper.go:325-334` - `FlatpakUninstallUnused()` returns placeholder text
  - `internal/pm/wrapper.go:199-209` - `ListFlatpakUpdates()` returns empty list with TODO
- Impact: Users clicking "Run" buttons, URLs, or expecting Flatpak updates detection get no action
- Fix approach: Implement `xdg-open` for URLs, shell execution for scripts, use pm library for Flatpak updates

**Missing Update Detection:**
- Issue: `ListFlatpakUpdates()` always returns empty, waiting for pm library support
- Files: `internal/pm/wrapper.go:199-209`
- Impact: Flatpak update badge always shows 0, users don't know updates are available
- Fix approach: Either implement directly via `flatpak remote-ls --updates` or wait for pm library

**Duplicate `runOnMainThread` Functions:**
- Issue: Same function defined in two packages with identical logic
- Files:
  - `internal/views/userhome.go:36-55`
  - `internal/pm/wrapper.go:409-418`
- Impact: Code duplication, potential for divergence, harder maintenance
- Fix approach: Extract to shared utility package `internal/gtkutil/gtkutil.go`

## Known Bugs

**openURL Does Nothing:**
- Symptoms: Clicking any external link (website, issues, help URLs) does nothing
- Files: `internal/views/userhome.go:1342-1346`
- Trigger: Click any row with external link icon
- Workaround: None - URLs don't work
- Fix: Use `xdg-open` or `gtk_show_uri`:
  ```go
  func (uh *UserHome) openURL(url string) {
      cmd := exec.Command("xdg-open", url)
      _ = cmd.Start()
  }
  ```

**Maintenance Actions Not Executed:**
- Symptoms: Clicking "Run" on maintenance cleanup actions does nothing
- Files: `internal/views/userhome.go:1347-1350`
- Trigger: Configure maintenance actions in YAML, click Run button
- Workaround: None
- Fix: Implement script execution (consider `pkexec` for sudo scripts)

**Unused Function Warning Suppressed:**
- Symptoms: `cleanupProgressUI()` marked as `//nolint:unused` but never called
- Files: `internal/views/userhome.go:2479-2498`
- Trigger: N/A - dead code
- Workaround: N/A
- Fix: Either use it for cleanup after batch operations or remove it

## Security Considerations

**Shell Command Execution:**
- Risk: Maintenance actions execute arbitrary shell scripts from config file
- Files:
  - `internal/views/userhome.go:871-895` - builds maintenance action rows
  - `internal/config/config.go:36-41` - ActionConfig with Script field
- Current mitigation: Config file must be in `/etc/chairlift/` or `/usr/share/chairlift/` (admin-controlled locations)
- Recommendations:
  - Validate script paths exist and are not symlinks to sensitive locations
  - Consider allowlist of permitted scripts
  - Log all script executions

**Privilege Escalation via pkexec:**
- Risk: Several operations use `pkexec` for elevated privileges
- Files:
  - `internal/nbc/nbc.go:206-244` - system updates via pkexec
  - `internal/instex/instex.go:112-147` - extension install via pkexec
- Current mitigation: pkexec prompts for authentication, dry-run mode available
- Recommendations: Limit which commands can be run with pkexec, use polkit rules

**External Command Injection:**
- Risk: App IDs, package names passed to shell commands
- Files:
  - `internal/views/userhome.go:1327` - `exec.Command("gtk-launch", appID)`
  - `internal/pm/wrapper.go` - package names passed to brew/flatpak
- Current mitigation: IDs come from system queries, not user input
- Recommendations: Validate app IDs match expected patterns before execution

## Performance Bottlenecks

**Blocking Initialization:**
- Problem: PM manager availability checks block on external commands
- Files: `internal/pm/wrapper.go:81-93`, `internal/pm/wrapper.go:445-457`, `internal/pm/wrapper.go:619-631`
- Cause: Each manager calls out to its CLI to check availability
- Improvement path: Already async via goroutines, but `time.Sleep(200ms)` in `userhome.go:169` is a code smell. Consider using channels/sync.WaitGroup

**Serial Package Listing:**
- Problem: Loading Homebrew formulae and casks makes separate calls
- Files: `internal/views/userhome.go:1846-1890`
- Cause: Two separate `ListHomebrewFormulae()` and `ListHomebrewCasks()` calls
- Improvement path: pm library returns all in one call, but code filters twice; could optimize to single iteration

**Large List Rendering:**
- Problem: Adding hundreds of rows to GTK ListBox/ExpanderRow in loop
- Files: Multiple locations adding rows in `loadHomebrewPackages()`, `loadFlatpakApplications()`, etc.
- Cause: GTK operations on main thread for each row
- Improvement path: Consider virtualized lists or batch additions for large package counts

## Fragile Areas

**UserHome State Management:**
- Files: `internal/views/userhome.go`
- Why fragile: 30+ fields tracking UI state, 11+ mutexes, complex goroutine coordination
- Safe modification: Ensure all UI updates go through `runOnMainThread()`, lock order matters
- Test coverage: Zero test files exist

**Progress Tracking System:**
- Files: `internal/views/userhome.go:2301-2474`
- Why fragile: Complex map-based tracking with 6 maps (`progressExpanders`, `progressGroups`, `progressRows`, `progressSpinners`, `progressActions`, `progressTasks`)
- Safe modification: Always lock `currentProgressMu` before map access, cleanup after operations
- Test coverage: None

**Idle Callback Registry:**
- Files: `internal/views/userhome.go:28-55`
- Why fragile: Manual memory management to prevent GC collection of callbacks, uses raw uintptr
- Safe modification: Never remove callback from map before execution, always increment ID atomically
- Test coverage: None

## Scaling Limits

**Package List Memory:**
- Current capacity: Hundreds of packages display fine
- Limit: Thousands of packages may cause UI lag/memory issues
- Scaling path: Implement pagination or virtualized scrolling

**Concurrent Operations:**
- Current capacity: Multiple async operations work
- Limit: No throttling on concurrent package operations
- Scaling path: Add semaphore to limit concurrent installs/updates

## Dependencies at Risk

**puregotk Binding:**
- Risk: Uses development version (`v0.0.0-20260115100645-c78e1521129b`) - not stable release
- Impact: Breaking changes could require significant refactoring
- Migration plan: Monitor for stable release, pin version carefully

**frostyard/pm Library:**
- Risk: Incomplete API (no update detection, no unused removal for Flatpak)
- Impact: Features marked as TODO waiting on library updates
- Migration plan: Contribute upstream or implement workarounds

**frostyard/nbc Library:**
- Risk: Internal library, types imported directly
- Impact: Tightly coupled to specific nbc version
- Migration plan: Interface abstractions would help decouple

## Missing Critical Features

**URL Opening:**
- Problem: `openURL()` is a stub - no external links work
- Blocks: Help page links, OS release URLs, documentation access

**Maintenance Script Execution:**
- Problem: `runMaintenanceAction()` is a stub
- Blocks: Cleanup actions, admin maintenance tasks

**Flatpak Update Detection:**
- Problem: Always returns empty list
- Blocks: Users knowing Flatpak updates are available

## Test Coverage Gaps

**No Tests At All:**
- What's not tested: Entire codebase has zero test files
- Files: All files in `internal/` and `cmd/`
- Risk: Any change could break functionality unnoticed
- Priority: High

**Critical Paths Untested:**
- Package manager operations (install/uninstall/update)
- Configuration loading and parsing
- Progress event handling
- NBC system update flow

**GTK Integration:**
- What's not tested: All UI rendering and callbacks
- Files: `internal/views/`, `internal/window/`
- Risk: GTK crashes, memory leaks, threading issues
- Priority: Medium - harder to test GTK code

**Recommended Test Priority:**
1. `internal/config/config.go` - Pure Go, easy to test
2. `internal/pm/wrapper.go` - Mock pm.Manager interface
3. `internal/nbc/nbc.go` - Mock exec.Command
4. `internal/updex/updex.go` - Mock exec.Command
5. `internal/instex/instex.go` - Mock exec.Command

---

*Concerns audit: 2026-01-26*
