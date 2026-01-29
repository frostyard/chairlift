---
phase: 07-complex-pages
plan: 04
subsystem: ui
tags: [gtk4, puregotk, lifecycle, composition, shell]

# Dependency graph
requires:
  - phase: 07-01
    provides: Updates page extracted to package
  - phase: 07-02
    provides: Applications page extracted to package
provides:
  - Thin shell (shell.go) composing all page packages
  - Proper Destroy() lifecycle for cancelling goroutines
  - PM progress bottom sheet removed (operations popover is unified display)
affects: [08-testing, 09-polish]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Shell composition pattern for page lifecycle
    - Destroy() method for goroutine cleanup

key-files:
  created: []
  modified:
    - internal/views/shell.go
    - internal/window/window.go

key-decisions:
  - "Remove PM progress bottom sheet - operations popover is unified progress display"
  - "Rename userhome.go to shell.go to reflect composition role"
  - "Destroy() implemented but window.go lacks close handler - noted for future"

patterns-established:
  - "Shell pattern: thin coordinator that creates/destroys page packages"

# Metrics
duration: 5min
completed: 2026-01-28
---

# Phase 07 Plan 04: Shell Reduction Summary

**Reduced userhome.go to thin shell.go (1130 to 194 lines) with proper Destroy() lifecycle management**

## Performance

- **Duration:** 5 min
- **Started:** 2026-01-28T15:48:48Z
- **Completed:** 2026-01-28T15:53:37Z
- **Tasks:** 3
- **Files modified:** 2

## Accomplishments
- Removed ~940 lines of redundant PM progress UI code
- Added Destroy() method for proper page package goroutine cleanup
- Renamed to shell.go to reflect composition role
- File now 194 lines (target was under 300)

## Task Commits

Each task was committed atomically:

1. **Task 1: Clean up remaining userhome.go code** - `a4bc592` (refactor)
2. **Task 2: Add proper Destroy() lifecycle management** - `869e4f2` (feat)
3. **Task 3: Rename userhome.go to shell.go** - `f2db9a8` (chore)

## Files Created/Modified
- `internal/views/shell.go` - Thin shell composing page packages (renamed from userhome.go)
- `internal/window/window.go` - Removed progress bottom sheet reference

## Decisions Made
- **Remove PM progress bottom sheet:** Operations popover (from Phase 3) is now the unified progress display. The redundant bottom sheet added unnecessary complexity
- **Keep UserHome struct name:** Renaming to Shell would require changes in window.go. Deferred to avoid breaking changes
- **Destroy() ready but not wired:** window.go lacks window close handling. Destroy() method is implemented and ready to be called when lifecycle management is added

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Shell is now a thin composition layer (194 lines)
- All page packages have Destroy() methods for proper cleanup
- Ready for Plan 05 (final integration) or testing phases
- Future work: Wire Destroy() to window close handler when lifecycle management is added

---
*Phase: 07-complex-pages*
*Completed: 2026-01-28*
