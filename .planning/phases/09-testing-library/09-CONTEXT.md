# Phase 9: Testing & Library - Context

**Gathered:** 2026-01-28
**Status:** Ready for planning

<domain>
## Phase Boundary

Add test coverage to business logic (config parsing, command builders, PM wrappers, page logic layers) and extract reusable GTK4/Go patterns into a library package. Tests enable confident refactoring; library enables pattern reuse in future projects.

</domain>

<decisions>
## Implementation Decisions

### Test scope & coverage
- Primary goal: regression safety — catch breaking changes during future refactors
- Test logic layers (pure Go, no GTK) in each page package
- Test PM wrapper functions (config parsing, command building)
- Claude's discretion: coverage targets and table-driven vs individual function style

### Integration test approach
- Verify widget structure exists after page construction (buttons, rows, etc.)
- Skip gracefully if no DISPLAY/WAYLAND_DISPLAY — tests work locally, skip in headless CI
- Use mocked dependencies — inject mock pm/nbc/updex for fast, isolated, predictable tests
- Test fixtures live in testdata/ folders (standard Go convention)

### Library extraction
- Location: same repo, /pkg/adwutil/ — easier iteration, extract to separate repo when stable
- Scope: full toolkit — widgets, async utilities, operations tracking, page interface
- Package name: adwutil (emphasizes libadwaita focus)
- Module: shared go.mod with chairlift — no submodule, simpler for now

### Documentation
- Format: full docs — godoc comments + README.md + examples/ folder
- Examples: working mini-apps showing patterns in context (buildable, runnable)
- No migration guides from raw GTK/puregotk — document the library itself
- License: MIT

### Claude's Discretion
- Coverage percentage targets per module
- Table-driven vs individual test functions per case
- Specific test helper utilities needed
- Example app complexity and count

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 09-testing-library*
*Context gathered: 2026-01-28*
