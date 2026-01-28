---
phase: 09-testing-library
plan: 03
subsystem: library
tags: [adwutil, gtk4, async, errors, re-export]

# Dependency graph
requires:
  - phase: 09-02
    provides: UserError tests in internal/async
provides:
  - pkg/adwutil library with RunOnMain and UserError
  - Backward-compatible internal/async re-exports
affects: [future-library-users, widget-extraction]

# Tech tracking
tech-stack:
  added: []
  patterns: [library-extraction, type-alias-re-export]

key-files:
  created:
    - pkg/adwutil/doc.go
    - pkg/adwutil/async.go
    - pkg/adwutil/errors.go
    - pkg/adwutil/errors_test.go
  modified:
    - internal/async/scheduler.go
    - internal/async/errors.go

key-decisions:
  - "pkg/adwutil has zero internal chairlift imports - pure library"
  - "Type alias (UserError = adwutil.UserError) maintains backward compatibility"
  - "Re-export pattern allows gradual migration to adwutil"

patterns-established:
  - "Library extraction: Extract to pkg/, update internal/ to re-export"
  - "Type alias: Use type alias for seamless backward compatibility"

# Metrics
duration: 2min
completed: 2026-01-28
---

# Phase 9 Plan 3: Extract Async Utilities Summary

**pkg/adwutil library with RunOnMain and UserError for reuse in GTK4/Go projects**

## Performance

- **Duration:** 2 min
- **Started:** 2026-01-28T17:18:04Z
- **Completed:** 2026-01-28T17:19:52Z
- **Tasks:** 3
- **Files modified:** 6

## Accomplishments
- Created pkg/adwutil library package with comprehensive documentation
- Extracted RunOnMain function with callback registry for GTK thread safety
- Extracted UserError type with constructors and formatting methods
- Updated internal/async to re-export from adwutil (backward compatible)

## Task Commits

Each task was committed atomically:

1. **Task 1: Create pkg/adwutil with doc and async** - `99b7489` (feat)
2. **Task 2: Extract UserError to adwutil** - `8942e76` (feat)
3. **Task 3: Update internal/async to re-export from adwutil** - `3a3dccf` (refactor)

## Files Created/Modified
- `pkg/adwutil/doc.go` - Package documentation with usage examples
- `pkg/adwutil/async.go` - RunOnMain function and callback registry
- `pkg/adwutil/errors.go` - UserError type and constructors
- `pkg/adwutil/errors_test.go` - Full test coverage for UserError
- `internal/async/scheduler.go` - Re-exports RunOnMain from adwutil
- `internal/async/errors.go` - Re-exports UserError from adwutil

## Decisions Made
- **Zero internal imports in pkg/adwutil**: Library is completely standalone, only importing puregotk and standard library
- **Type alias for UserError**: Using `type UserError = adwutil.UserError` allows existing code to continue working without changes
- **Re-export pattern**: internal/async delegates to adwutil, enabling gradual migration

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- pkg/adwutil ready for widget extraction in future plans
- Library pattern established for extracting additional utilities
- All existing code continues to work via internal/async re-exports

---
*Phase: 09-testing-library*
*Completed: 2026-01-28*
