# Phase 7: Complex Pages - Context

**Gathered:** 2026-01-28
**Status:** Ready for planning

<domain>
## Phase Boundary

Extract Applications and Updates pages from userhome.go, completing the monolith split. After this phase, userhome.go should be under 300 lines — a thin shell that composes pages. All existing functionality must work identically to before refactoring.

</domain>

<decisions>
## Implementation Decisions

### Package Manager Handling
- One shared `applications` package handles all three PMs (Flatpak, Homebrew, Snap) with internal separation
- PM unification approach: Claude's discretion based on existing frostyard/pm patterns
- Unavailable PMs: Show the section but indicate PM not installed (disabled state)
- Progress display: Operations system only — track in operations popover, button shows spinner

### Search & Discovery
- Search functionality is in scope for Phase 7
- Scope: Search + discovery — both installed packages AND new packages to install (flathub, brew formulae, snap store)
- Unified search: One search box, results from all available PMs
- Results presentation: Grouped by PM (results under Flatpak, Homebrew, Snap headers)

### Updates Page Scope
- Coverage: All updates — NBC system updates + Flatpak/Homebrew/Snap package updates
- Organization: Sectioned by source (System/NBC, Flatpak, Homebrew, Snap)
- Update All: Both global "Update All" button AND per-section buttons
- Reboot communication: Post-update prompt after NBC update completes
- Refresh behavior: Auto-check on page load + manual refresh button
- Progress display: Uniform indeterminate progress for all update types (consistency over granularity)
- No updates state: Use empty state widget pattern from Phase 5
- Empty sections: Show all sections, empty ones say "No updates available"

### Applications Page Structure
- Navigation: Sidebar navigation — left sidebar with PM list, content area on right
- All Apps view: Claude's discretion based on search functionality decision
- Per-package actions: Remove + Launch + Info/Details view
- Info/Details content: Claude's discretion based on what each PM provides

### userhome.go Remnants
- Shell contents: Claude's discretion — determine minimal shell based on extractions
- Navigation code: Claude's discretion — keep in shell or extract based on complexity
- File rename: Rename to app.go or shell.go to reflect new composition role
- Remaining code distribution: Claude's discretion — distribute to appropriate existing packages

### Claude's Discretion
- PM interface vs shared UI approach for code unification
- Whether to include "All Apps" view in sidebar
- Package details content per PM
- What remains in the shell file
- Navigation code location
- Distribution of non-page-specific code

</decisions>

<specifics>
## Specific Ideas

- Sidebar navigation pattern aligns with Libadwaita navigation patterns
- Search results grouped by PM keeps clear separation while enabling unified discovery
- Post-update reboot prompt (not inline notice) is cleaner UX

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 07-complex-pages*
*Context gathered: 2026-01-28*
