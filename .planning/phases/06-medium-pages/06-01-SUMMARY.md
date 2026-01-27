---
phase: 06-medium-pages
plan: 01
subsystem: pages
tags: [maintenance, refactor, page-extraction]

dependency-graph:
  requires: [04-03]
  provides: [maintenance-page-package]
  affects: [07-xx]

tech-stack:
  added: []
  patterns: [logic-ui-separation, dependency-injection, context-lifecycle]

key-files:
  created:
    - internal/pages/maintenance/logic.go
    - internal/pages/maintenance/logic_test.go
    - internal/pages/maintenance/page.go
  modified:
    - internal/views/userhome.go

decisions:
  - id: maintenance-logic-separation
    choice: "Logic layer has no GTK imports for testability"
    reason: "Follows Phase 4 pattern, enables testing without GTK runtime"
  - id: maintenance-context-lifecycle
    choice: "Context-based goroutine lifecycle with cancel on Destroy"
    reason: "Consistent with system page pattern, prevents goroutine leaks"
  - id: maintenance-operations-tracking
    choice: "All actions tracked via operations.Start with RetryFunc"
    reason: "Enables retry from operations popover, consistent with Phase 3"

metrics:
  duration: "3 min"
  completed: "2026-01-27"
---

# Phase 6 Plan 1: Maintenance Page Extraction Summary

**One-liner:** Maintenance page extracted to internal/pages/maintenance with logic/UI separation, operations tracking, and retry support

## What Was Done

### Task 1: Create maintenance logic layer (5e13dba)
- Created `internal/pages/maintenance/logic.go` with:
  - `Action` struct (Title, Description, Script, Sudo)
  - `ScriptExecutor` interface for testable execution
  - `DefaultExecutor` using pkexec for sudo scripts
  - `ParseActions()` function to extract actions from config
- Created `internal/pages/maintenance/logic_test.go` with tests for:
  - nil config handling
  - empty actions slice
  - populated actions with correct field mapping

### Task 2: Create maintenance UI layer (360c5f0)
- Created `internal/pages/maintenance/page.go` implementing Page interface
- `New(deps pages.Deps)` constructor with dependency injection
- `Widget()` and `Destroy()` methods for lifecycle management
- Context-based goroutine lifecycle with cancel propagation
- All 4 maintenance groups implemented:
  - System Cleanup: Custom actions from config
  - Homebrew Cleanup: pm.HomebrewCleanup()
  - Flatpak Cleanup: pm.FlatpakUninstallUnused()
  - Optimization: Placeholder (coming soon)
- Operations tracking with `RetryFunc` wired for all actions
- Loop variable capture to avoid closure bugs

### Task 3: Integrate into userhome.go (4d5d0ac)
- Added maintenance package import
- Added `maintenancePagePkg *maintenance.Page` field
- Created page using `maintenance.New(deps)` in constructor
- Routed `GetPage("maintenance")` to `maintenancePagePkg.Widget()`
- Removed dead code:
  - `buildMaintenancePage()` function (~115 lines)
  - `runMaintenanceAction()` function
  - `onBrewCleanupClicked()` function
  - `onFlatpakCleanupClicked()` function
  - `maintenancePage` and `maintenancePrefsPage` fields
- Updated Destroy TODO comment to include maintenancePagePkg

## Key Decisions Made

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Logic/UI separation | Logic layer has no GTK imports | Enables testing without GTK runtime, follows Phase 4 pattern |
| Context lifecycle | Context-based goroutine management | Consistent with system page, prevents leaks on Destroy |
| Operations tracking | All actions use operations.Start | Enables retry from popover, consistent with Phase 3 |

## Deviations from Plan

None - plan executed exactly as written.

## Commit Log

| Task | Commit | Description |
|------|--------|-------------|
| 1 | 5e13dba | feat(06-01): create maintenance logic layer |
| 2 | 360c5f0 | feat(06-01): add maintenance page UI layer |
| 3 | 4d5d0ac | feat(06-01): integrate maintenance page into userhome |

## Verification Results

- `go build ./...` - Pass
- `go test ./internal/pages/maintenance/...` - Pass (3 tests)
- Logic layer has no GTK imports - Verified
- userhome.go reduced by ~179 lines

## Next Phase Readiness

**Phase 6 Plan 2 (Extensions Page):**
- Extensions page should follow identical pattern
- Logic layer: Extension struct, ListInstalled, Install, IsAvailable
- UI layer: Installed group, Discover group with repository URL entry

**Open Items:**
- None - all success criteria met
