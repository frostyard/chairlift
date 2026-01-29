---
phase: 07-complex-pages
plan: 03
subsystem: ui
tags: [gtk4, adw, flatpak, homebrew, snap, search, package-manager]

# Dependency graph
requires:
  - phase: 07-02
    provides: Applications page foundation with NavigationSplitView sidebar
  - phase: 04-simple-pages
    provides: pages.Deps dependency injection pattern
provides:
  - Full Applications page with Flatpak/Homebrew/Snap content sections
  - Search functionality for Homebrew packages
  - PM list/uninstall/install capabilities
  - userhome.go reduced to thin shell (shell.go)
affects: [07-04, 08-polish]

# Tech tracking
tech-stack:
  added: []
  patterns: [Category-specific content pages, PM availability-gated UI, Search with async results]

key-files:
  created: []
  modified:
    - internal/pages/applications/logic.go
    - internal/pages/applications/page.go
    - internal/views/shell.go (renamed from userhome.go)

key-decisions:
  - "SearchResult type provides unified structure for multi-PM search results"
  - "Content pages use addXxxGroupToPage helpers for reusable group building"
  - "All async operations check p.ctx.Done() before UI updates"
  - "userhome.go renamed to shell.go reflecting its composition role"
  - "Destroy() method added to UserHome for proper lifecycle cleanup"

patterns-established:
  - "Search pattern: async search with context cancellation and UI update guards"
  - "PM content helpers: addFlatpakGroupsToPage, addSnapGroupToPage, addHomebrewGroupToPage"
  - "Shell composition: views package composes page packages via Deps injection"

# Metrics
duration: 5min
completed: 2026-01-28
---

# Phase 7 Plan 03: Applications Page Content Summary

**Complete Applications page with Flatpak/Homebrew/Snap list/uninstall/search, userhome.go reduced to 194-line shell**

## Performance

- **Duration:** 5 min
- **Started:** 2026-01-28T15:48:56Z
- **Completed:** 2026-01-28T15:53:50Z
- **Tasks:** 3
- **Files modified:** 4

## Accomplishments
- Added SearchResult type and SearchHomebrew function to logic layer
- Implemented full PM content sections (Flatpak user/system, Snap, Homebrew formulae/casks)
- Homebrew search with install capability working
- userhome.go cleaned and renamed to shell.go (194 lines from ~1100)
- Destroy() lifecycle method added for proper goroutine cleanup

## Task Commits

Each task was committed atomically:

1. **Task 1: Extend logic layer with search** - `f70eaa0` (feat)
2. **Task 2: Add PM content sections to page** - `135647c` (feat)
3. **Task 3: Remove migrated code from userhome.go** - `a4bc592` (refactor)
   - Additional cleanup: `869e4f2` (feat - Destroy method)
   - File rename: `f2db9a8` (chore - userhome.go -> shell.go)

## Files Created/Modified
- `internal/pages/applications/logic.go` - SearchResult type, SearchHomebrew, HasSearchCapability
- `internal/pages/applications/page.go` - Full PM content with async loading functions
- `internal/views/shell.go` - Thin composition shell (renamed from userhome.go)
- `internal/window/window.go` - Progress bottom sheet removed (cleanup)

## Decisions Made
- SearchResult uses PMCategory enum for type-safe PM identification
- Each PM has dedicated addXxxGroupToPage helper for reusable content building
- Context cancellation checks added at every async.RunOnMain call site
- Loop variable capture (`app := app`) used consistently to avoid closure bugs
- userhome.go renamed to shell.go per CONTEXT.md decision on shell file naming

## Deviations from Plan

### Commit Labeling

**Task 3 commits used wrong plan label (07-04 instead of 07-03)**
- Commits a4bc592, 869e4f2, f2db9a8 labeled as 07-04
- This was due to concurrent execution environment state
- Functional work is correct, only commit message labels differ
- No impact on code correctness

### Additional Work

**1. [Rule 2 - Missing Critical] Added Destroy() lifecycle method**
- **Found during:** Task 3 (userhome.go cleanup)
- **Issue:** No Destroy() method for proper goroutine cleanup on page packages
- **Fix:** Added Destroy() method that calls Destroy on all page packages
- **Files modified:** internal/views/shell.go
- **Committed in:** 869e4f2

**2. File rename: userhome.go -> shell.go**
- Per CONTEXT.md: "File rename: Rename to app.go or shell.go to reflect new composition role"
- **Committed in:** f2db9a8

---

**Total deviations:** 2 (1 missing critical, 1 planned rename)
**Impact on plan:** Deviations were beneficial - Destroy() ensures clean shutdown, rename clarifies file purpose.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Applications page fully functional with all PM sections
- Shell composition pattern complete - ready for final polish phase
- userhome.go significantly reduced - Plan 04 can focus on any remaining cleanup

---
*Phase: 07-complex-pages*
*Completed: 2026-01-28*
