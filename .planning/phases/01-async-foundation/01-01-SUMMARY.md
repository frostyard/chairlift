---
phase: 01-async-foundation
plan: 01
subsystem: infra
tags: [glib, gtk4, async, threading, error-handling]

# Dependency graph
requires: []
provides:
  - RunOnMain() function for thread-safe GTK updates from goroutines
  - UserError type for user-friendly error messaging
  - Callback registry pattern preventing GC collection
affects: [02-widget-extraction, 03-operations-progress, userhome-migration, wrapper-migration]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Callback registry for GC safety with glib.IdleAdd"
    - "UserError separation of user message from technical details"

key-files:
  created:
    - internal/async/scheduler.go
    - internal/async/errors.go
  modified: []

key-decisions:
  - "RunOnMain uses exact pattern from userhome.go for proven reliability"
  - "UserError uses 'Couldn't' tone per CONTEXT.md decisions"

patterns-established:
  - "async.RunOnMain(func()) for all goroutine-to-UI updates"
  - "async.UserError for all user-facing errors with technical details"

# Metrics
duration: 2min
completed: 2026-01-27
---

# Phase 1 Plan 01: Create async package Summary

**Thread-safe RunOnMain() function and UserError type for GTK4/Go async operations**

## Performance

- **Duration:** 2 min
- **Started:** 2026-01-27T02:08:56Z
- **Completed:** 2026-01-27T02:10:49Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Created `internal/async/scheduler.go` with `RunOnMain()` function (97 lines)
- Created `internal/async/errors.go` with `UserError` type (107 lines)
- Callback registry pattern prevents GC collection of scheduled callbacks
- UserError implements error interface with Unwrap() for errors.Is/As support

## Task Commits

Each task was committed atomically:

1. **Task 1: Create async/scheduler.go with RunOnMain()** - `91eaffd` (feat)
2. **Task 2: Create async/errors.go with UserError type** - `4f46b19` (feat)

## Files Created/Modified

- `internal/async/scheduler.go` - Thread-safe RunOnMain() with callback registry
- `internal/async/errors.go` - UserError type with formatting methods and constructors

## Decisions Made

- **Exact pattern match:** RunOnMain() follows the proven pattern from userhome.go lines 36-55 exactly, including the lock/unlock sequence
- **Tone compliance:** UserError documentation uses "Couldn't" not "Failed to" per CONTEXT.md decisions
- **No additional features:** Kept scope minimal - only RunOnMain() and UserError as specified

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- async package is ready for use
- Next plan (01-02) will migrate userhome.go to use async.RunOnMain()
- No blockers or concerns

---
*Phase: 01-async-foundation*
*Completed: 2026-01-27*
