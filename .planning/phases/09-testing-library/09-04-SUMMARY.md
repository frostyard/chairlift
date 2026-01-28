---
phase: 09-testing-library
plan: 04
subsystem: library
tags: [adwutil, gtk4, widgets, operations, registry, re-export]

# Dependency graph
requires:
  - phase: 09-03
    provides: pkg/adwutil library with RunOnMain and UserError
provides:
  - Widget helpers (NewEmptyState, NewLinkRow, NewButtonRow, etc.) in adwutil
  - Operations tracking (Registry, Operation, State, Category) in adwutil
  - Complete adwutil library for GTK4/Libadwaita patterns
affects: [future-library-users, external-projects]

# Tech tracking
tech-stack:
  added: []
  patterns: [widget-helpers, operations-registry, signal-callback-gc-protection]

key-files:
  created:
    - pkg/adwutil/widgets.go
    - pkg/adwutil/empty_state.go
    - pkg/adwutil/operations.go
    - pkg/adwutil/operation.go
    - pkg/adwutil/operation_test.go
    - pkg/adwutil/operations_test.go
  modified:
    - internal/widgets/empty_state.go
    - internal/widgets/rows.go
    - internal/operations/operation.go
    - internal/operations/registry.go

key-decisions:
  - "Widget signal callbacks stored in registry to prevent GC collection"
  - "Operations tests moved to pkg/adwutil with implementation"
  - "Type aliases maintain full backward compatibility for existing code"

patterns-established:
  - "Signal callback registry: Store callbacks to prevent GC before signal fires"
  - "Operations tracking: Registry with listeners notified via RunOnMain"

# Metrics
duration: 4min
completed: 2026-01-28
---

# Phase 9 Plan 4: Extract Widgets and Operations Summary

**Complete adwutil library with widget helpers, operations tracking, and full test coverage for reuse in GTK4/Go projects**

## Performance

- **Duration:** 4 min
- **Started:** 2026-01-28T17:23:41Z
- **Completed:** 2026-01-28T17:27:17Z
- **Tasks:** 3
- **Files modified:** 10

## Accomplishments
- Extracted widget helpers (NewEmptyState, NewLinkRow, NewInfoRow, NewButtonRow, NewIconRow) to adwutil
- Extracted operations tracking (Registry, Operation, State, Category) to adwutil
- Updated internal packages to re-export from adwutil with backward compatibility
- Moved operations tests to pkg/adwutil alongside implementation

## Task Commits

Each task was committed atomically:

1. **Task 1: Extract widgets to adwutil** - `8e1f652` (feat)
2. **Task 2: Extract operations to adwutil** - `f32a661` (feat)
3. **Task 3: Update internal packages to re-export from adwutil** - `2be3a39` (refactor)

## Files Created/Modified
- `pkg/adwutil/empty_state.go` - NewEmptyState helper for GNOME HIG empty states
- `pkg/adwutil/widgets.go` - Row helpers with signal callback GC protection
- `pkg/adwutil/operation.go` - Operation type with State, Category, lifecycle methods
- `pkg/adwutil/operations.go` - Registry for tracking active/completed operations
- `pkg/adwutil/operation_test.go` - Tests for Operation type and methods
- `pkg/adwutil/operations_test.go` - Tests for Registry functionality
- `internal/widgets/empty_state.go` - Re-exports NewEmptyState from adwutil
- `internal/widgets/rows.go` - Re-exports row helpers from adwutil
- `internal/operations/operation.go` - Re-exports Operation, State, Category from adwutil
- `internal/operations/registry.go` - Re-exports Registry functions from adwutil

## Decisions Made
- **Signal callback registry in widgets.go**: GTK signal callbacks stored in registry to prevent GC collection before signal fires
- **Tests moved to pkg/adwutil**: Operations tests now live with implementation in pkg/adwutil for cleaner separation
- **Type aliases for backward compatibility**: `type Category = adwutil.Category` etc. ensures existing code continues to work

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- pkg/adwutil library complete with async, errors, widgets, and operations
- Library has zero imports from chairlift internal packages
- All existing code continues to work via internal package re-exports
- Ready for Phase 9-05 (documentation/finalization)

---
*Phase: 09-testing-library*
*Completed: 2026-01-28*
