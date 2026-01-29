---
phase: 07-complex-pages
plan: 01
subsystem: ui
tags: [gtk, adwaita, updates, nbc, flatpak, homebrew, pages]

# Dependency graph
requires:
  - phase: 04-simple-pages
    provides: Page interface, pages.Deps pattern, context-based goroutine lifecycle
  - phase: 06-medium-pages
    provides: Maintenance page pattern with operations.Start
provides:
  - Updates page package with logic/UI separation
  - NBC system update checking and installation
  - Flatpak update discovery and per-app updates
  - Homebrew update and outdated package management
  - Badge count callback for notifying parent of available updates
affects: [07-complex-pages, userhome-cleanup]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Updates page follows established page package pattern with context lifecycle
    - Badge count notification via callback (onBadgeUpdate func(int))

key-files:
  created:
    - internal/pages/updates/logic.go
    - internal/pages/updates/page.go
  modified:
    - internal/views/userhome.go

key-decisions:
  - "Badge count callback via onBadgeUpdate func(int) for parent notification"
  - "Logic layer (logic.go) has no GTK dependencies for testability"

patterns-established:
  - "Updates page uses same pattern as system/maintenance/extensions pages"
  - "NBCUpdateStatus struct wraps nbc.CheckUpdate result for logic layer"

# Metrics
duration: 5min
completed: 2026-01-28
---

# Phase 07 Plan 01: Updates Page Extraction Summary

**NBC/Flatpak/Homebrew updates page extracted to standalone package with logic/UI separation and badge count callback**

## Performance

- **Duration:** 5 min
- **Started:** 2026-01-28T15:40:00Z
- **Completed:** 2026-01-28T15:45:00Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- Created updates page logic layer with pure Go functions (no GTK dependencies)
- Built updates page UI with NBC, Flatpak, and Homebrew update sections
- Integrated updates page into userhome.go, reducing file by 694 lines
- Badge count updates correctly via callback to parent

## Task Commits

Each task was committed atomically:

1. **Task 1: Create updates logic layer** - `a9ba412` (feat)
2. **Task 2: Create updates page UI** - `0d24ddf` (feat)
3. **Task 3: Integrate updates page into userhome.go** - `69b1b6c` (refactor)

## Files Created/Modified
- `internal/pages/updates/logic.go` - Logic layer with NBCUpdateStatus, update checking functions
- `internal/pages/updates/page.go` - UI implementation with NBC/Flatpak/Homebrew groups
- `internal/views/userhome.go` - Integrated updates package, removed ~700 lines of code

## Decisions Made
- Badge count notification via onBadgeUpdate callback rather than interface
- Logic layer wraps nbc.CheckUpdate in local NBCUpdateStatus struct for isolation

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Removed unused `operations` import from userhome.go after extraction (detected by compiler)

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Updates page extraction complete
- userhome.go reduced from 1824 to 1130 lines
- Ready for Plan 02 (Applications page complex features)

---
*Phase: 07-complex-pages*
*Completed: 2026-01-28*
