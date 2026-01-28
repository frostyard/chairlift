# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-01-26)

**Core value:** Less technical users can confidently manage their immutable Linux desktop without needing to understand the underlying CLI tools or immutable filesystem concepts.
**Current focus:** Phase 9 - Testing & Library Extraction

## Current Position

Phase: 9 of 9 (Testing & Library)
Plan: 5 of 5 in current phase
Status: Phase complete
Last activity: 2026-01-28 - Completed 09-05-PLAN.md (Documentation & Examples)

Progress: [████████████████] 100%

## Performance Metrics

**Velocity:**
- Total plans completed: 32
- Average duration: 2.7 min
- Total execution time: 1.4 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1. Async Foundation | 3/3 | 6 min | 2 min |
| 2. Widget Extraction | 3/3 | 5 min | 1.7 min |
| 3. Operations & Progress | 5/5 | 8 min | 1.6 min |
| 4. Simple Pages | 3/3 | 7 min | 2.3 min |
| 5. Feedback Polish | 2/2 | 4 min | 2 min |
| 6. Medium Pages | 3/3 | 12 min | 4 min |
| 6.1 Remove instex | 1/1 | 2 min | 2 min |
| 7. Complex Pages | 5/5 | 35 min | 7 min |
| 9. Testing & Library | 5/5 | 14 min | 2.8 min |

**Recent Trend:**
- Last 5 plans: 3 min, 3 min, 2 min, 4 min
- Trend: Consistent

*Updated after each plan completion*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Extract reusable GTK4/Go library during refactor (Pending)
- Library location TBD until patterns emerge (Pending)
- Progress/async as core infrastructure (Pending)
- RunOnMain uses exact pattern from userhome.go (01-01)
- UserError uses 'Couldn't' tone per CONTEXT.md (01-01)
- Combined Task 1+2 commit for userhome.go migration (01-02)
- 4 key user-facing errors converted to UserError in userhome.go (01-02)
- 5 key package manager errors converted to UserError (01-03)
- AsyncExpanderRow uses composition (puregotk no inheritance) (02-01)
- StopLoading idempotent for simpler cleanup code (02-01)
- ActionButton stores originalLabel for restoration (02-02)
- LoadingRow.Stop() doesn't remove row, caller controls lifecycle (02-02)
- NewButtonRow defaults to suggested-action CSS class (02-02)
- Migrate loadNBCStatus as best example of full async lifecycle (02-03)
- Keep complex rows with buttons inline for now (02-03)
- Progress defaults to -1 (indeterminate) until set (03-01)
- Failed operations stay in active list for retry (03-01)
- IsCancellable requires cancellable flag AND >5s runtime (03-01)
- 30 second threshold for spinner-to-progress-bar transition (03-02)
- showingBar flag prevents flip-flopping between progress modes (03-02)
- Cancel confirmation uses AdwAlertDialog with Continue as default (03-03)
- Operations popover uses ViewSwitcher for Active/History tabs (03-03)
- Operations button packed left of hamburger menu in header (03-04)
- StartTrackedOperation returns both operation and done function (03-04)
- Migrated Update Homebrew as primary visible operation (03-05)
- Help page uses logic/UI layer separation for testability (04-01)
- BuildResourceLinks is pure Go with no GTK dependencies (04-01)
- System page logic layer has no GTK dependencies for testability (04-02)
- Page uses context.WithCancel for goroutine lifecycle management (04-02)
- FetchNBCStatus wraps nbc.GetStatus for future mock injection (04-02)
- Page packages integrated with dependency injection via pages.Deps (04-03)
- TODO added for Destroy() lifecycle when view cleanup is added (04-03)
- openURL implemented using xdg-open for browser links (04-UAT)
- launchApp detects flatpak apps and uses 'flatpak run' (04-UAT)
- Dry-run banner uses adw.Banner with "Understood" dismissal (05-02)
- RetryFunc wired on op after operations.Start() (05-02)
- StatusPage inline in popover.go due to import cycle (05-01)
- Maintenance page uses context-based goroutine lifecycle (06-01)
- All maintenance actions tracked via operations.Start with RetryFunc (06-01)
- IsAvailable uses exec.LookPath for systemd-sysext (06-02)
- Keep instex CLI for discovery (no library equivalent) (06-03) [SUPERSEDED by 06.1-01]
- Extensions page follows established page package pattern (06-03)
- Use p.ctx for Discover/Install calls to maintain goroutine lifecycle (06.1-01)
- IsDiscoverAvailable checks IsAvailable since updex SDK needs systemd-sysext (06.1-01)
- Badge count callback via onBadgeUpdate func(int) for parent notification (07-01)
- Updates page logic layer has no GTK dependencies for testability (07-01)
- Applications page uses NavigationSplitView for sidebar navigation (07-02)
- SearchResult type provides unified structure for multi-PM search results (07-03)
- Content pages use addXxxGroupToPage helpers for reusable group building (07-03)
- All async operations check p.ctx.Done() before UI updates (07-03)
- Remove PM progress bottom sheet - operations popover is unified display (07-04)
- Rename userhome.go to shell.go to reflect composition role (07-04)
- Destroy() implemented but window.go lacks close handler - noted for future (07-04)
- Test pure logic functions only, skip external service calls (09-01)
- Use isolated test registries to avoid singleton and async.RunOnMain (09-02)
- Table-driven tests for format method variations (09-02)
- Test nil-safety for all public methods (09-02)
- pkg/adwutil has zero internal chairlift imports - pure library (09-03)
- Type alias (UserError = adwutil.UserError) maintains backward compatibility (09-03)
- Re-export pattern allows gradual migration to adwutil (09-03)
- Widget signal callbacks stored in registry to prevent GC collection (09-04)
- Operations tests moved to pkg/adwutil with implementation (09-04)
- README covers all features with Quick Start samples (09-05)
- Examples follow pkg/adwutil/examples/{name}/main.go convention (09-05)

### Pending Todos

- Fix GTK markup parsing error: "•" character in operation status text (e.g., "Completed • <1s • Just now") is interpreted as markup. Need to escape or use plain text labels instead of markup-enabled ones.

### Blockers/Concerns

None yet.

### Roadmap Evolution

- Phase 6.1 inserted after Phase 6: Remove instex (COMPLETE)

## Session Continuity

Last session: 2026-01-28
Stopped at: Completed 09-05-PLAN.md (Documentation & Examples)
Resume with: Milestone complete - all phases finished

---
*State initialized: 2026-01-26*
*Last updated: 2026-01-28*
