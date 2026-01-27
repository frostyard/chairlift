---
phase: 01-async-foundation
plan: 02
subsystem: infra
tags: [glib, gtk4, async, threading, error-handling, migration]

# Dependency graph
requires:
  - phase: 01-01
    provides: RunOnMain() function and UserError type
provides:
  - userhome.go using centralized async.RunOnMain()
  - UserError pattern established at 4 key error points
affects: [02-widget-extraction, 03-operations-progress, wrapper-migration]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "All async UI updates via async.RunOnMain()"
    - "UserError for user-friendly error messaging with logging"

key-files:
  created: []
  modified:
    - internal/views/userhome.go

key-decisions:
  - "Removed local runOnMainThread and callback registry (replaced by async package)"
  - "Removed glib import from userhome.go (now encapsulated in async package)"
  - "Applied UserError to 4 prominent user-facing operations"

patterns-established:
  - "async.RunOnMain(func()) for all goroutine-to-UI updates in views"
  - "async.NewUserError() for friendly error messages, log technical details"

# Metrics
duration: 1.5min
completed: 2026-01-27
---

# Phase 1 Plan 02: Migrate userhome.go to async package Summary

**Migrated 59 runOnMainThread calls to async.RunOnMain and established UserError pattern at 4 key error handlers**

## Performance

- **Duration:** 1.5 min
- **Started:** 2026-01-27T02:15:29Z
- **Completed:** 2026-01-27T02:17:00Z
- **Tasks:** 3
- **Files modified:** 1

## Accomplishments

- Removed local runOnMainThread function and callback registry from userhome.go
- Migrated all 59 call sites to use centralized async.RunOnMain()
- Added async package import, removed now-unused glib import
- Established UserError pattern at 4 key error handlers (NBC update, Flatpak cleanup, Homebrew install, extension install)

## Task Commits

Each task was committed atomically:

1. **Tasks 1-2: Remove local runOnMainThread and migrate calls** - `f2e1ef2` (refactor)
2. **Task 3: Add UserError to key error handlers** - `bc9908e` (feat)

Note: Tasks 1 and 2 were committed together because Task 1 leaves the file in a non-compiling state.

## Files Created/Modified

- `internal/views/userhome.go` - Migrated to async.RunOnMain, added UserError usage

## Decisions Made

- **Combined Task 1+2 commit:** Tasks 1 and 2 are interdependent; committing separately would leave broken intermediate state
- **4 UserError points chosen:** NBC update, Flatpak cleanup, Homebrew install, extension install - all high-visibility user operations
- **Removed glib import:** No longer needed in userhome.go since async package encapsulates glib.IdleAdd

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- userhome.go fully migrated to async package
- UserError pattern established for future error handling improvements
- Ready for 01-03 (wrapper migration) or next phase
- No blockers or concerns

---
*Phase: 01-async-foundation*
*Completed: 2026-01-27*
