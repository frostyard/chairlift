# Roadmap: Chairlift Refactoring

## Overview

This refactoring transforms Chairlift from a 2500-line monolith into a well-structured GTK4/Go application with reusable patterns. The journey follows a dependency-driven order: async infrastructure first (everything depends on it), then widgets (pages depend on them), then pages from simple to complex (establish patterns before tackling complexity), and finally accessibility, testing, and library extraction (polish what works).

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Async Foundation** - Centralized async utilities, threading safety, error handling
- [x] **Phase 2: Widget Extraction** - Reusable UI components (AsyncExpanderRow, ActionButton, LoadingRow)
- [x] **Phase 3: Operations & Progress** - Unified operation tracking, cancellation, progress display
- [x] **Phase 4: Simple Pages** - Extract System & Help pages, establish page interface pattern
- [x] **Phase 5: Feedback Polish** - Empty states, status banners, retry capability
- [x] **Phase 6: Medium Pages** - Extract Maintenance & Extensions pages, updex library integration
- [ ] **Phase 7: Complex Pages** - Extract Applications & Updates pages, complete monolith split
- [ ] **Phase 8: Accessibility** - Screen reader support, keyboard navigation, widget relationships
- [ ] **Phase 9: Testing & Library** - Unit/integration tests, pattern documentation, library extraction

## Phase Details

### Phase 1: Async Foundation
**Goal**: All async operations use a unified pattern with consistent threading, error handling, and GC safety
**Depends on**: Nothing (first phase)
**Requirements**: INFR-01, INFR-02, INFR-03, INFR-04
**Success Criteria** (what must be TRUE):
  1. All goroutine-to-UI communication routes through a single `RunOnMain()` function
  2. Error messages shown to users explain the problem and suggest what to do next
  3. Application runs without segfaults or random crashes from GC-related widget issues
  4. Callback references are held in a registry that prevents garbage collection
**Plans**: 3 plans

Plans:
- [x] 01-01-PLAN.md — Create async package with RunOnMain and UserError
- [x] 01-02-PLAN.md — Migrate userhome.go to async.RunOnMain
- [x] 01-03-PLAN.md — Migrate pm/wrapper.go to async.RunOnMain

### Phase 2: Widget Extraction
**Goal**: Common UI patterns are extracted into reusable widget components
**Depends on**: Phase 1 (widgets use async patterns)
**Requirements**: ARCH-02, ARCH-03, ARCH-04, ARCH-05
**Success Criteria** (what must be TRUE):
  1. AsyncExpanderRow exists and handles loading state, error display, and content population
  2. ActionButton exists and disables itself during its operation, shows spinner
  3. LoadingRow exists and displays consistent loading state with spinner
  4. Common ActionRow patterns have builder functions reducing boilerplate
  5. Widgets are used in at least one place in existing code (validated by use)
**Plans**: 3 plans

Plans:
- [x] 02-01-PLAN.md — Create widgets package with AsyncExpanderRow
- [x] 02-02-PLAN.md — Create ActionButton, LoadingRow, and row builders
- [x] 02-03-PLAN.md — Validate widgets with userhome.go migration

### Phase 3: Operations & Progress
**Goal**: Users can see, track, and cancel all ongoing operations with consistent progress feedback
**Depends on**: Phase 1 (async patterns), Phase 2 (widgets for display)
**Requirements**: INFR-05, INFR-06, INFR-07, FDBK-01, FDBK-02, FDBK-03
**Success Criteria** (what must be TRUE):
  1. User can see all ongoing operations in a central location (header popover)
  2. User can cancel any operation that takes more than a few seconds
  3. User can see recently completed operations with their outcomes
  4. All async data loads show a spinner while loading
  5. Operations longer than 30 seconds show a progress bar with percentage
  6. Buttons and controls are disabled while their associated operations run
**Plans**: 5 plans

Plans:
- [x] 03-01-PLAN.md — Create operations package with registry and operation types
- [x] 03-02-PLAN.md — Create ProgressRow widget with spinner/progress bar transition
- [x] 03-03-PLAN.md — Create operations popover UI and cancellation dialogs
- [x] 03-04-PLAN.md — Wire operations button to header and integrate ActionButton
- [x] 03-05-PLAN.md — Validate with migration of one userhome.go operation

### Phase 4: Simple Pages
**Goal**: System and Help pages are extracted as separate packages, establishing the page interface pattern
**Depends on**: Phase 2 (widgets), Phase 3 (operations)
**Requirements**: ARCH-06, ARCH-07
**Success Criteria** (what must be TRUE):
  1. System page exists in its own package with clear interface boundary
  2. Help page exists in its own package with clear interface boundary
  3. Page packages expose a constructor that receives dependencies (toaster, config)
  4. Business logic (config reading, command building) is testable without GTK runtime
  5. Goroutines started by pages are cleaned up when pages are destroyed
**Plans**: 3 plans

Plans:
- [x] 04-01-PLAN.md — Create page interface and Help page package
- [x] 04-02-PLAN.md — Create System page package with goroutine lifecycle
- [x] 04-03-PLAN.md — Integrate page packages into userhome.go and add tests

### Phase 5: Feedback Polish
**Goal**: Users get clear guidance in empty states, see persistent mode indicators, and can retry failed operations
**Depends on**: Phase 3 (operations for retry), Phase 4 (pages for consistent application)
**Requirements**: FDBK-04, FDBK-05, FDBK-06
**Success Criteria** (what must be TRUE):
  1. Empty lists show placeholder pages with guidance text (not just blank space)
  2. Dry-run mode displays a persistent banner visible throughout the app
  3. Failed operations show a "Retry" button that re-attempts the operation
  4. Status banners are dismissible and don't obstruct primary content
**Plans**: 2 plans

Plans:
- [x] 05-01-PLAN.md — Create empty state widget and upgrade popover
- [x] 05-02-PLAN.md — Add dry-run banner and validate retry wiring

### Phase 6: Medium Pages
**Goal**: Maintenance and Extensions pages are extracted, with updex using library instead of CLI
**Depends on**: Phase 4 (page pattern established), Phase 5 (feedback patterns)
**Requirements**: INTG-01
**Success Criteria** (what must be TRUE):
  1. Maintenance page exists in its own package following established pattern
  2. Extensions page exists in its own package following established pattern
  3. Extensions page calls updex Go library directly (no subprocess/CLI)
  4. sysext operations have proper progress and error handling via library API
**Plans**: 3 plans

Plans:
- [x] 06-01-PLAN.md — Extract Maintenance page with logic/UI separation
- [x] 06-02-PLAN.md — Create Extensions logic layer with updex library
- [x] 06-03-PLAN.md — Create Extensions UI layer and integrate pages

### Phase 7: Complex Pages
**Goal**: Applications and Updates pages are extracted, completing the monolith split
**Depends on**: Phase 6 (medium pages validate patterns at scale)
**Requirements**: ARCH-01
**Success Criteria** (what must be TRUE):
  1. Applications page exists in its own package handling all three package managers
  2. Updates page exists in its own package with NBC progress integration
  3. userhome.go is reduced to a thin shell that composes pages (under 300 lines)
  4. All existing functionality works identically to before refactoring
  5. Package manager progress (Flatpak, Homebrew, Snap) routes through unified progress system
**Plans**: TBD

Plans:
- [ ] 07-01: TBD

### Phase 8: Accessibility
**Goal**: Users with assistive technologies can navigate and operate the full application
**Depends on**: Phase 7 (all pages extracted and stable)
**Requirements**: ACCS-01, ACCS-02, ACCS-03
**Success Criteria** (what must be TRUE):
  1. Icon-only buttons have accessible labels that screen readers announce
  2. All interactive elements can be reached and activated via keyboard alone
  3. Screen readers announce relationship context (e.g., "Install button for Firefox")
  4. Tab order follows logical visual flow
**Plans**: TBD

Plans:
- [ ] 08-01: TBD

### Phase 9: Testing & Library
**Goal**: Business logic has test coverage and reusable GTK4/Go patterns are extracted for future projects
**Depends on**: Phase 8 (all features complete and stable)
**Requirements**: TEST-01, TEST-02, LIBR-01, LIBR-02
**Success Criteria** (what must be TRUE):
  1. Config parsing, command builders, and PM wrapper logic have unit tests
  2. Page construction has integration tests that verify no panics on create
  3. Extractable patterns are documented with usage examples
  4. Reusable GTK4/Go utilities exist in a separate package (same or separate repo)
  5. Test coverage enables confident future refactoring
**Plans**: TBD

Plans:
- [ ] 09-01: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Async Foundation | 3/3 | Complete | 2026-01-27 |
| 2. Widget Extraction | 3/3 | Complete | 2026-01-27 |
| 3. Operations & Progress | 5/5 | Complete | 2026-01-27 |
| 4. Simple Pages | 3/3 | Complete | 2026-01-27 |
| 5. Feedback Polish | 2/2 | Complete | 2026-01-27 |
| 6. Medium Pages | 3/3 | Complete | 2026-01-27 |
| 7. Complex Pages | 0/TBD | Not started | - |
| 8. Accessibility | 0/TBD | Not started | - |
| 9. Testing & Library | 0/TBD | Not started | - |

---
*Roadmap created: 2026-01-26*
*Last updated: 2026-01-27 — Phase 6 complete*
