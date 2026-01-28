---
phase: 09-testing-library
plan: 06
subsystem: ui
tags: [gtk4, libadwaita, widgets, toast, destructive-action]

# Dependency graph
requires:
  - phase: 09-04
    provides: Widget helpers with signal callback registry
provides:
  - NewButtonRowWithIcon helper for rows with icon + button suffix
  - NewButtonRowWarning convenience wrapper for destructive actions
  - Toast notification demo in basic example
affects: [applications-using-adwutil]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Icon suffix before button for visual indicators"
    - "ToastOverlay wrapping scrolled content for notifications"

key-files:
  created: []
  modified:
    - pkg/adwutil/widgets.go
    - pkg/adwutil/examples/basic/main.go

key-decisions:
  - "Icon added as suffix before button for visual order [Icon] [Button]"
  - "NewButtonRowWithIconAndClass for maximum flexibility with defaults"

patterns-established:
  - "Warning button: icon suffix + destructive-action CSS class + toast feedback"

# Metrics
duration: 3min
completed: 2026-01-28
---

# Phase 9 Plan 6: Warning Button with Toast Demo Summary

**NewButtonRowWithIcon and NewButtonRowWarning helpers added to adwutil with toast notification demo in basic example**

## Performance

- **Duration:** 3 min
- **Started:** 2026-01-28T15:00:00Z
- **Completed:** 2026-01-28T15:03:00Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Added NewButtonRowWithIcon helper for rows with icon suffix before button
- Added NewButtonRowWarning convenience wrapper for destructive action patterns
- Basic example demonstrates toast notification with warning button

## Task Commits

Each task was committed atomically:

1. **Task 1: Add NewButtonRowWithIcon helper to adwutil** - `53df124` (feat)
2. **Task 2: Add toast and warning button demo to basic example** - `da0c012` (feat)

## Files Created/Modified

- `pkg/adwutil/widgets.go` - Added NewButtonRowWithIcon, NewButtonRowWithIconAndClass, NewButtonRowWarning helpers
- `pkg/adwutil/examples/basic/main.go` - Added ToastOverlay and Warning Actions group with Clear Cache demo

## Decisions Made

- Icon added as suffix before button creates visual order: [Title/Subtitle] ... [Icon] [Button]
- Created NewButtonRowWithIconAndClass for maximum flexibility (custom icon + custom CSS class)
- NewButtonRowWithIcon defaults to suggested-action, NewButtonRowWarning uses destructive-action

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- UAT gap closed: basic example now demonstrates button with warning icon suffix and toast notification
- All adwutil widget helpers documented and demonstrated

---
*Phase: 09-testing-library*
*Completed: 2026-01-28*
