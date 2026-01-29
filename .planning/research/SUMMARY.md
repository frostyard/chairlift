# Project Research Summary

**Project:** Chairlift Refactoring Milestone
**Domain:** GTK4/Go desktop application refactoring
**Researched:** 2026-01-26
**Confidence:** HIGH

## Executive Summary

Chairlift is a GTK4/Libadwaita desktop application written in Go using puregotk, currently structured as a 2500-line monolith (`userhome.go`) that needs decomposition for maintainability, testability, and eventual library extraction. The key insight from research: **puregotk enables standard Go patterns** (interfaces, composition, channels) rather than fighting GTK's C-oriented idioms. The refactoring should leverage Go's strengths while respecting GTK4's threading model.

The recommended approach is **infrastructure-first extraction**: pull out `runOnMainThread` and async utilities, then standardize progress tracking, then systematically extract pages from simplest to most complex. This order is dictated by dependencies—every page needs the async framework, and the most complex pages (updates, applications) depend on patterns proven in simpler pages. Feature-based package organization with interface boundaries enables testing of business logic without GTK runtime.

The critical risks are GTK main-thread violations during refactoring (causes segfaults), widget reference GC issues (causes random crashes), and progress system fragmentation (causes stuck UI states). All three are mitigated by establishing infrastructure patterns before any component extraction. The existing callback registry pattern for GC prevention is correct—it must be consistently applied across all extracted components.

## Key Findings

### Recommended Stack

The existing stack is appropriate—no technology changes recommended. The focus is on code organization patterns within the current GTK4/Go/puregotk architecture.

**Core patterns to adopt:**
- **Feature-based packages:** `internal/pages/{system,updates,applications,extensions,maintenance,help}` — each page as separate package with clear boundaries
- **Shared widget infrastructure:** `internal/widgets/` for reusable components (AsyncExpanderRow, ActionButton, LoadingRow)
- **Unified async framework:** `internal/async/` centralizing `runOnMainThread`, operation management, and progress reporting
- **Interface-based testability:** Define `Toaster`, `WindowServices`, `PackageManager` interfaces for mock-based testing

### Expected Features

**Must have (table stakes):**
- Consistent progress indication for all async operations (currently only NBC has proper progress)
- Clear error messages for less-technical users (current shows raw Go errors)
- Spinner/loading states on all async loads
- Disabled controls during operations (prevent double-clicks)
- Cancellation support for operations >10 seconds

**Should have (differentiators):**
- Unified operation queue/tracker (current BottomSheet concept, needs consistency)
- Accessibility labels for icon-only buttons (screen reader support)
- Confirmation dialogs for destructive actions (uninstall has none currently)
- Retry capability on failure

**Defer to post-refactor:**
- Desktop notifications for background operation completion
- Undo support (complex for system operations)
- State restoration between sessions
- Window state persistence

### Architecture Approach

The architecture follows GTK's natural component boundaries with Go package organization. Each page becomes a self-contained package implementing a common `Page` interface, receiving dependencies (toaster, config values) through constructors. Widgets package provides reusable composite widgets that encapsulate common patterns (expander with async loading, button that disables during operation).

**Major components:**
1. **async/scheduler.go** — Centralized `RunOnMain()`, callback registry, prevents GC issues
2. **async/operation.go** — Cancellable operation wrapper with progress, unified across all async work
3. **widgets/** — AsyncExpanderRow, ActionButton, LoadingRow (patterns that repeat 10-20x currently)
4. **pages/** — Six page packages, each ~100-700 lines extracted from `userhome.go`
5. **backends/** — Renamed wrappers for pm, nbc, updex, instex (existing, reorganized)

### Critical Pitfalls

1. **GTK Main-Thread Violations** — Every UI mutation must trace to `runOnMainThread()`. Extract this utility FIRST before any other refactoring. Add `// UI-SAFE` comments to callbacks.

2. **Widget Reference GC** — Go GC destroys widgets if references not held. Every extracted component must hold references to widgets it creates. The existing 40+ widget fields in `UserHome` exist specifically for this.

3. **UX Regression** — Target users are less technical; any visible behavior change causes confusion. Document current behavior (screenshots) before each page extraction, verify parity after.

4. **Mutex Deadlocks** — 11+ mutexes across `userhome.go` and `pm/wrapper.go`. Map lock acquisition patterns before splitting state. Establish hierarchy: UI locks < PM wrapper locks.

5. **Progress System Fragmentation** — 6 interrelated progress maps in `UserHome`. Design unified progress manager before component extraction, not after.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Infrastructure Foundation
**Rationale:** All other work depends on centralized async/threading utilities. Currently duplicated in `userhome.go` and `pm/wrapper.go`.
**Delivers:** `internal/async/` package with `RunOnMain()`, callback registry, operation wrapper
**Addresses:** Unified async pattern (Features), consistent threading
**Avoids:** Main-thread violations (#1), mutex deadlock setup (#4)

### Phase 2: Widget Extraction
**Rationale:** Pages are built from widgets; extract reusable patterns before pages. Currently same patterns repeated 20+ times.
**Delivers:** `internal/widgets/` with AsyncExpanderRow, ActionButton, LoadingRow, ProgressDisplay
**Uses:** async/scheduler from Phase 1
**Avoids:** Code duplication, inconsistent patterns

### Phase 3: Simple Page Extraction (System, Help)
**Rationale:** Smallest pages (~100-170 lines each), lowest risk, validates page interface and extraction pattern
**Delivers:** `internal/pages/system/`, `internal/pages/help/`, verified Page interface
**Addresses:** View-model separation, feature-based structure
**Avoids:** Widget GC issues (#2), UX regression (#3) — establish pattern with low stakes

### Phase 4: Progress System Unification
**Rationale:** Before extracting complex pages, unified progress is required. Current 6-map progress system is fragile.
**Delivers:** Central progress manager, unified ProgressEvent type, refactored BottomSheet integration
**Addresses:** Progress indication (Features), async consistency
**Avoids:** Progress fragmentation (#5), async inconsistency

### Phase 5: Medium Page Extraction (Maintenance, Extensions)
**Rationale:** Medium complexity (~150-200 lines each), applies proven patterns from Phase 3
**Delivers:** `internal/pages/maintenance/`, `internal/pages/extensions/`
**Uses:** Widgets, async, progress infrastructure
**Implements:** Page interface pattern at medium complexity

### Phase 6: Complex Page Extraction (Applications)
**Rationale:** ~600 lines with three package manager integrations. Highest widget count but no complex progress.
**Delivers:** `internal/pages/applications/`
**Avoids:** Widget sprawl, configuration coupling (#8)

### Phase 7: Complex Page Extraction (Updates)
**Rationale:** Most complex (~700 lines), depends on all prior infrastructure. Has NBC progress, PM progress, badge updates.
**Delivers:** `internal/pages/updates/`, fully refactored userhome.go (minimal shell)
**Avoids:** All critical pitfalls—this is where they'd surface if not handled earlier

### Phase 8: Testing and Library Preparation
**Rationale:** After patterns stable, add tests and evaluate library extraction. Premature extraction is a pitfall.
**Delivers:** Unit tests for async/backends, integration tests for pages, documented patterns for potential library
**Addresses:** Testability goals, library extraction strategy
**Avoids:** Premature abstraction (#6), test paralysis (#10)

### Phase Ordering Rationale

- **Dependencies drive order:** Async → Widgets → Pages (simple → complex)
- **Risk gradient:** Start with low-risk extractions, accumulate confidence
- **Infrastructure before features:** Progress system must be unified before pages that use it heavily
- **Complexity gradient:** System/Help → Maintenance/Extensions → Applications → Updates
- **Library extraction last:** See patterns emerge from real use, not speculation

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 4 (Progress Unification):** Complex state management, current system is "bolted on"—needs careful design
- **Phase 7 (Updates Page):** Most complexity, multiple async patterns converge—may need spike/prototype

Phases with standard patterns (skip research-phase):
- **Phase 1 (Infrastructure):** Well-understood Go patterns, existing code shows correct approach
- **Phase 2 (Widgets):** Pure UI extraction, patterns clear from current code analysis
- **Phase 3, 5, 6 (Page Extractions):** Mechanical extraction following established pattern

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | No technology changes; patterns from official puregotk + Go best practices |
| Features | HIGH | Based on GNOME HIG official documentation + codebase analysis |
| Architecture | MEDIUM | Synthesized from GTK4 patterns and Go idioms; limited public GTK4+Go examples |
| Pitfalls | HIGH | Derived directly from codebase analysis; threading/GC issues well-understood |

**Overall confidence:** HIGH

### Gaps to Address

- **Testing infrastructure:** No existing tests; unclear how to mock GTK for unit tests. Address in Phase 1 by focusing on testable layers (backends, config parsing) and accepting UI code is hard to unit test.

- **Library extraction scope:** Unclear what's truly reusable until patterns emerge. Defer decision until Phase 8; keep candidate code in `internal/` initially.

- **Dry-run propagation:** Currently module-level variable in multiple packages. Needs design decision—pass via constructor or context? Address in Phase 1 infrastructure.

## Sources

### Primary (HIGH confidence)
- Chairlift codebase analysis — `userhome.go`, `pm/wrapper.go`, `window.go`
- puregotk GitHub repository — examples, README, threading model
- GNOME HIG — Feedback patterns, accessibility, dialogs, progress bars

### Secondary (MEDIUM confidence)
- GTK4 official documentation — C-focused but patterns translate
- frostyard/pm library — progress callback pattern

### Tertiary (Context from project docs)
- `.planning/PROJECT.md` — User context, goals
- `.planning/codebase/CONCERNS.md` — Known issues, stubs

---
*Research completed: 2026-01-26*
*Ready for roadmap: yes*
