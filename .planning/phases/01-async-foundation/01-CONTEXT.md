# Phase 1: Async Foundation - Context

**Gathered:** 2026-01-26
**Status:** Ready for planning

<domain>
## Phase Boundary

All async operations use a unified pattern with consistent threading, error handling, and GC safety. Specifically:
- All goroutine-to-UI communication routes through a single `RunOnMain()` function
- Error messages shown to users explain the problem and suggest what to do next
- Application runs without segfaults or random crashes from GC-related widget issues
- Callback references are held in a registry that prevents garbage collection

</domain>

<decisions>
## Implementation Decisions

### Error message style
- Show summary + technical hint (e.g., "Couldn't install Firefox: remote not reachable")
- Include suggested actions only when actionable (not generic advice)
- Tone: friendly but clear ("Couldn't install" not "Failed to install" or "Oops!")
- Offer expandable details ("Copy details" or "View log") for users who need full technical info

### Error notification UX
- Mixed approach: toast for minor errors, dialog for critical, inline for forms
- Critical = blocks functionality (user can't proceed without addressing it)
- Toast behavior: auto-dismiss for success messages, persistent for errors
- Multiple errors: stack them vertically, show all (don't queue or replace)

### Loading feedback patterns
- Brief delay (200-300ms) before showing spinner to avoid flicker
- Spinner is the default; progress bar only when explicit progress data exists
- Buttons show in-button spinner when their action is running
- Lists show skeleton loading rows matching expected layout

### Retry behavior defaults
- Error-type dependent: auto-retry for network issues, fail fast for other errors
- When auto-retrying, show "Retrying..." indicator (don't hide it from user)
- Timeouts are operation-specific (longer for installs, shorter for data fetches)
- Retry button offered when context-appropriate (not every error needs inline retry)

### Claude's Discretion
- Exact timing thresholds (200ms delay, specific timeout values per operation type)
- Skeleton row count and appearance
- Retry count (2-3) and backoff strategy
- Technical implementation of callback registry and RunOnMain

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches. User wants GNOME-style UX conventions followed where applicable.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 01-async-foundation*
*Context gathered: 2026-01-26*
