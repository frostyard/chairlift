# Phase 3: Operations & Progress - Context

**Gathered:** 2026-01-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can see, track, and cancel all ongoing operations with consistent progress feedback. This includes a central location to view operations, cancellation for long-running operations, history of completed operations, spinners for async loads, progress bars for extended operations, and disabled controls during operations.

</domain>

<decisions>
## Implementation Decisions

### Operations panel location
- Header popover (icon in header opens dropdown with operations)
- Icon always visible in header, badge with count appears when operations are active
- Operations grouped by type (Installs, Updates, Loading) inside popover
- Only show groups that have active operations (no empty groups)

### Progress display style
- Single consistent spinner style everywhere for short operations (<30s)
- Long operations (>30s) show progress bar + percentage + descriptive label
- Unknown progress starts as indeterminate (pulsing) bar, transitions to determinate when percentage known
- Progress appears inline (next to triggering button/row) as primary location, popover as secondary for background operations

### Completed operations history
- Keep all completed operations from current session (since app launch)
- Separate tab/toggle within the popover to view history (not mixed with active)
- Each completed operation shows: name, outcome (success/failed), timestamp, duration
- Failed operations stay in active list with retry option (not auto-moved to history)

### Cancellation behavior
- Only long operations (>5s) are cancellable
- Cancel button appears immediately when operation starts (for cancellable operations)
- Cancellation always requires confirmation dialog
- After cancellation confirmed, show toast notification "Cancelled"

### Claude's Discretion
- Specific spinner component/animation choice
- Exact popover dimensions and styling
- Progress bar styling details
- Grouping category names and icons
- Confirmation dialog wording

</decisions>

<specifics>
## Specific Ideas

No specific product references mentioned — open to standard GTK4/libadwaita approaches for popovers, progress bars, and toasts.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 03-operations-progress*
*Context gathered: 2026-01-26*
