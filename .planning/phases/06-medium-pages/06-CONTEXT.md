# Phase 6: Medium Pages - Context

**Gathered:** 2026-01-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Extract Maintenance and Extensions pages into their own packages following the pattern established in Phase 4 (Help/System pages). The Extensions page transitions from CLI-based updex calls to direct library integration via frostyard/updex.

</domain>

<decisions>
## Implementation Decisions

### Maintenance page logic separation
- Pure Go logic layer for config parsing, returns typed structs to UI
- Typed action structs: name, script, sudo flag, description
- Logic layer executes scripts, UI layer tracks via operations package
- Mock executor interface for testability (inject executor so tests can verify without running scripts)

### Extensions page library integration
- Evolve internal/updex from CLI wrapper to importing frostyard/updex library directly
- Use pm/progress interface directly (same pattern as Flatpak/Homebrew)
- Callback-based progress reporting for install/remove operations
- Library is compile-time dependency, always available (static binary)
- Page follows established pattern from Phase 4 (logic/UI separation)

### Discovery UX
- Manual button click triggers discovery (not auto-discover on load)
- Card grid display for discovered extensions (name, description, version, install action)
- Inline spinner while fetching available extensions
- StatusPage empty state when no extensions found

### Error presentation
- Errors appear in operations popover + toast notification
- Expandable error details showing stderr/error message
- Convert to friendly UserError pattern with action suggestion
- Failed operations offer retry via operations popover

### Claude's Discretion
- sysext availability detection approach
- Card grid layout specifics (spacing, columns)
- Loading skeleton design if used
- Exact error message wording

</decisions>

<specifics>
## Specific Ideas

- updex implements the pm/progress interface, so progress integration should flow through the same channel as other package managers (Flatpak, Homebrew)
- Follow Phase 4 pattern exactly: logic layer has no GTK dependencies, page packages receive dependencies via injection

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 06-medium-pages*
*Context gathered: 2026-01-27*
