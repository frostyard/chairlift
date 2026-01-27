---
phase: 01-async-foundation
plan: 03
subsystem: infra
tags: [glib, gtk4, async, threading, error-handling, package-manager]

# Dependency graph
requires:
  - phase: 01-01
    provides: async.RunOnMain() and async.UserError types
provides:
  - pm/wrapper.go using centralized async.RunOnMain()
  - User-friendly error messages for package manager operations
  - GC-safe callback handling for progress reporter
affects: [userhome-ui-updates, error-display, pm-operations]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "UserError for user-facing package manager errors"
    - "async.RunOnMain for progress reporter callbacks"

key-files:
  created: []
  modified:
    - internal/pm/wrapper.go

key-decisions:
  - "5 key error points converted to UserError (install/remove for Flatpak, Snap, Homebrew)"
  - "Hints follow 'Couldn't' tone per CONTEXT.md"

patterns-established:
  - "Package manager errors use async.UserError with actionable hints"

# Metrics
duration: 2min
completed: 2026-01-27
---

# Phase 1 Plan 03: Migrate pm/wrapper.go Summary

**pm/wrapper.go migrated from local runOnMainThread to async.RunOnMain with UserError integration for package manager errors**

## Performance

- **Duration:** 2 min
- **Started:** 2026-01-27T02:13:56Z
- **Completed:** 2026-01-27T02:16:10Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments

- Replaced 4 `runOnMainThread()` calls with `async.RunOnMain()` in progress reporter
- Deleted local `runOnMainThread` function that used naked `glib.IdleAdd` (no GC protection)
- Added 5 UserError usages at key user-facing error points
- All errors use "Couldn't" tone with actionable hints per CONTEXT.md

## Task Commits

Each task was committed atomically:

1. **Task 1: Replace local runOnMainThread with async.RunOnMain** - `186998e` (feat)
2. **Task 2: Add UserError to package manager operation errors** - `92ac63f` (feat)

## Files Created/Modified

- `internal/pm/wrapper.go` - Migrated to async package, added UserError for 5 error points

## Decisions Made

- **Error points selected:** Focused on install/remove operations for Flatpak, Snap, and Homebrew as these are the most user-visible operations
- **Hint style:** Each error includes actionable hints (connection issues, store availability, dependents)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- All three plans in Phase 1 now complete (01-01, 01-02, 01-03)
- async package provides centralized RunOnMain() and UserError
- All runOnMainThread implementations consolidated
- Ready for Phase 2: Widget Extraction

---
*Phase: 01-async-foundation*
*Completed: 2026-01-27*
