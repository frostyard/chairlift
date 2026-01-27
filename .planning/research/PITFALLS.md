# Domain Pitfalls

**Domain:** GTK4/Go application refactoring (2500-line monolith to library extraction)
**Researched:** 2026-01-26
**Confidence:** HIGH (based on codebase analysis + GTK4/Go domain knowledge)

## Critical Pitfalls

Mistakes that cause rewrites, UI breakage, or major regressions.

### Pitfall 1: GTK Main Thread Violations During Refactoring

**What goes wrong:** Moving code between files or packages breaks the careful `runOnMainThread()` pattern. GTK4 crashes or hangs when UI is modified from background goroutines.

**Why it happens:** 
- Chairlift has async operations everywhere (12+ `go func()` calls in userhome.go)
- When extracting components, developers copy callback code but miss the thread-marshaling wrapper
- puregotk doesn't provide runtime warnings—just segfaults or silent corruption

**Consequences:**
- App crashes unpredictably, hard to reproduce
- Flatpak/Homebrew operations appear to complete but UI doesn't update
- Race conditions that appear weeks later in production

**Prevention:**
- **Rule: Every UI mutation must trace back to `runOnMainThread()`**
- Extract `runOnMainThread()` to shared package FIRST before any refactoring
- Add comments `// UI-SAFE: Called via runOnMainThread` to every callback
- Consider wrapper type that enforces main-thread-only access: `type UIAction func()` with single entry point

**Detection:**
- Warning sign: Code moves between packages without updating imports of `runOnMainThread`
- Warning sign: Direct widget method calls in goroutines
- Test: Run with `-race` flag, though GTK issues won't always surface as races

**Phase relevance:** Address in infrastructure phase (earliest). Must be solved before any component extraction.

---

### Pitfall 2: Widget Reference Lifetime/GC Problems

**What goes wrong:** Go garbage collector destroys GTK widgets that are still displayed or referenced by GTK's internal C structures.

**Why it happens:**
- Chairlift already has `idleCallbackRegistry` (lines 29-33) to prevent GC of callbacks
- When extracting components, references stored in `UserHome` struct get split across packages
- New component structs may not hold references correctly
- The 30+ widget fields in `UserHome` all exist specifically to prevent GC

**Consequences:**
- Random crashes when clicking buttons/rows that were garbage collected
- Memory corruption in the GTK layer
- Incredibly hard to debug—crashes in C code, not Go

**Prevention:**
- **Rule: Every extracted component must hold references to all widgets it creates**
- Keep the same pattern: struct fields for widgets, never just local variables for long-lived widgets
- Document why each field exists (GC prevention vs state access)
- When in doubt, keep the reference

**Detection:**
- Warning sign: Extracted component creates widgets in function, returns parent without storing children
- Warning sign: Callbacks reference widgets not stored in struct
- Test: Stress test UI with rapid navigation and button clicks

**Phase relevance:** Must be understood before any component extraction. Document patterns in architecture phase.

---

### Pitfall 3: Breaking User-Facing UX During Internal Refactoring

**What goes wrong:** Refactoring changes visible behavior—button placement, loading states, error messages, keyboard shortcuts—that confuse less-technical users.

**Why it happens:**
- Chairlift targets users who find CLI intimidating (per PROJECT.md)
- Developers focus on code structure, accidentally change UX
- No tests to catch behavioral regressions
- "Clean up" refactoring removes perceived redundancy that was actually user-visible

**Consequences:**
- Users report "it's broken" when technically it works differently
- Loss of trust from target user base
- Support burden increases

**Prevention:**
- **Rule: UX parity is mandatory for internal refactoring**
- Document current behavior BEFORE refactoring each page (screenshots, click-through flows)
- Compare before/after for every page during review
- Extract components without changing their API or visible behavior first
- Resist "while we're here" improvements—they belong in separate phases

**Detection:**
- Warning sign: Refactoring PR includes "also improved X" changes
- Warning sign: Loading states, error messages, or toast text changed
- Test: Manual regression testing against documented flows

**Phase relevance:** Every phase. Establish documentation/screenshot baseline before each refactoring phase.

---

### Pitfall 4: Mutex Deadlocks When Splitting State

**What goes wrong:** `UserHome` has 11+ mutexes protecting different state. When splitting into components, mutex acquisition order becomes inconsistent, causing deadlocks.

**Why it happens:**
- Current code has implicit ordering (caller always acquires in same order)
- Extracting components creates new call patterns
- Components calling back into parent can invert lock order
- GTK main thread + multiple mutexes = complex deadlock scenarios

**Consequences:**
- App hangs completely—no crash, no error, just frozen
- Happens under specific timing, hard to reproduce
- Users have to kill the app

**Prevention:**
- **Map all mutexes and their acquisition patterns before refactoring**
- Current mutexes: `idleCallbackMu`, `updateCountMu`, `currentProgressMu`, plus `flatpakMu`, `snapMu`, `brewMu`, `flatpakAvailMu`, `snapAvailMu`, `brewAvailMu`, `dryRunMu` in pm/wrapper.go
- Establish lock hierarchy: UI locks < PM wrapper locks
- Never hold UI mutex while calling into PM layer
- Consider whether components need their own mutexes vs sharing parent's

**Detection:**
- Warning sign: Component takes mutex, then calls function that takes another mutex
- Warning sign: Callback function that's called from multiple contexts with different lock states
- Test: Static analysis for lock acquisition patterns

**Phase relevance:** Address in infrastructure phase alongside `runOnMainThread` extraction. Document lock hierarchy.

---

### Pitfall 5: Progress System Fragmentation

**What goes wrong:** The progress tracking system (6 maps in `UserHome`) becomes inconsistent when components manage their own progress separately.

**Why it happens:**
- Current progress is "bolted on" (per PROJECT.md) with complex state tracking
- Each component handling its own progress leads to different patterns
- `progressExpanders`, `progressGroups`, `progressRows`, `progressSpinners`, `progressActions`, `progressTasks` all interrelated
- Cleanup logic (`cleanupProgressUI`) depends on this centralized structure

**Consequences:**
- Progress gets stuck showing "in progress" forever
- Bottom sheet never closes automatically
- Memory leak from orphaned progress entries
- Inconsistent progress UX across different operations

**Prevention:**
- **Design unified progress system BEFORE component extraction**
- Current pattern works but is fragile; don't replicate fragility in each component
- Options:
  - Central progress manager that components register with
  - Observable pattern where components publish, central UI subscribes
  - Structured progress context passed through call chain
- Make progress a core infrastructure, not per-component responsibility

**Detection:**
- Warning sign: Each extracted component implements its own progress map
- Warning sign: Bottom sheet operations duplicated in multiple places
- Test: Trigger multiple concurrent operations, verify all complete cleanly

**Phase relevance:** Design in architecture phase. Implement in infrastructure phase BEFORE feature extraction.

---

## Moderate Pitfalls

Mistakes that cause delays, technical debt, or require partial rewrites.

### Pitfall 6: Extracting Too Early Without Patterns Emerging

**What goes wrong:** Creating a reusable library from the first file touched, before understanding what's actually reusable across the codebase.

**Why it happens:**
- Natural desire to "do it right" from the start
- First component extracted becomes template—bad template = bad library
- Patterns in system page may not apply to extensions page

**Prevention:**
- **Extract 2-3 components as internal packages BEFORE designing library**
- Let patterns emerge from actual extraction work
- Keep extracted code in `internal/` initially, only promote to library after validation
- Document what worked and what didn't after each extraction

**Phase relevance:** Defer library design until after substantial internal refactoring complete.

---

### Pitfall 7: Async Operation Inconsistency

**What goes wrong:** Different components implement async operations differently—some use channels, some use callbacks, some use both.

**Why it happens:**
- Chairlift already has mixed patterns:
  - NBC uses channels (`progressCh <- event`)
  - PM uses callbacks (`ProgressCallback func(...)`)
  - Direct goroutines with `runOnMainThread` closures
- When extracting, developers pick whichever seems easiest for that component

**Prevention:**
- **Standardize async pattern before extraction**
- Current candidates:
  - Channels (NBC pattern) - Good for streaming progress
  - Callbacks (PM pattern) - Good for integration with external libs
  - Context with observers - Good for cancellation
- Recommendation: Design unified pattern that wraps all three internally but presents one interface

**Phase relevance:** Solve in infrastructure phase. Part of unified async/progress framework.

---

### Pitfall 8: Configuration Coupling

**What goes wrong:** Extracted components still depend on full `config.Config` struct, making them non-reusable.

**Why it happens:**
- Current code passes `*config.Config` everywhere
- Easy to keep passing same struct to extracted components
- Components end up needing entire config even if they use one field

**Prevention:**
- **Components receive only what they need**
- Instead of `func NewSystemPage(cfg *config.Config)`, use `func NewSystemPage(enabled bool, healthAppID string)`
- Let caller do config lookup, component stays pure
- Config parsing stays in app layer, components are config-agnostic

**Phase relevance:** Apply during each component extraction. Design principle, not separate phase.

---

### Pitfall 9: Losing Dry-Run Support

**What goes wrong:** Extracted components don't properly propagate dry-run mode, causing real operations during testing.

**Why it happens:**
- Dry-run is currently a module-level variable in multiple packages
- When extracting components, need to ensure they check dry-run
- Easy to add new operation that bypasses the check

**Prevention:**
- **Centralize dry-run as injectable dependency, not global**
- Pass `dryRun bool` to constructors, store in component
- Every state-changing function checks instance field
- Alternatively: wrapper functions that check once, all operations go through wrapper

**Phase relevance:** Part of infrastructure design. Address with async/progress framework.

---

### Pitfall 10: Test Strategy Paralysis

**What goes wrong:** Wanting comprehensive tests before refactoring, but tests are hard to write for 2500-line monolith, so nothing gets tested.

**Why it happens:**
- Testing GTK code is genuinely hard (no puregotk testing utilities exist)
- Testing without changing code structure requires complex mocking
- Waiting for "perfect" test infrastructure blocks actual work

**Prevention:**
- **Accept that full UI testing isn't practical; focus on testable layers**
- Test what CAN be tested without GTK: config parsing, PM wrapper logic, command builders
- Extract testable logic from UI handlers FIRST, test that
- Use characterization tests for refactoring: capture current behavior, verify it doesn't change
- Integration tests that run actual commands in dry-run mode

**Phase relevance:** Early phase. Get testing infrastructure working for non-GTK code before major refactoring.

---

## Minor Pitfalls

Mistakes that cause annoyance but are straightforward to fix.

### Pitfall 11: Package Naming Conflicts

**What goes wrong:** Internal packages named same as imported external packages cause confusion.

**Why it happens:**
- `internal/pm` wraps `github.com/frostyard/pm`
- Already requires aliasing: `pm "github.com/frostyard/pm"` in wrapper.go
- More wrappers could create more conflicts

**Prevention:**
- Use distinct names for wrapper packages (e.g., `pmwrap`, `nbcwrap`)
- Or keep current pattern but document the aliasing convention

---

### Pitfall 12: Incomplete Stub Migration

**What goes wrong:** Stubs (`openURL`, `runMaintenanceAction`, `ListFlatpakUpdates`) don't get implemented when code moves.

**Why it happens:**
- Easy to copy stubs during refactoring without implementing
- Stubs are "working" (they compile and don't crash)
- Technical debt accumulates silently

**Prevention:**
- **Audit stubs before refactoring, decide: implement or remove**
- Current stubs per CONCERNS.md:
  - `openURL()` - implement with xdg-open
  - `runMaintenanceAction()` - implement with exec
  - `ListFlatpakUpdates()` - implement or remove feature
  - `FlatpakUninstallUnused()` - implement or remove feature
- Add `// TODO(refactor): Implement before extraction` comments

**Phase relevance:** Address before or during extraction of relevant pages.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|----------------|------------|
| Infrastructure extraction | Main-thread violations (#1), Mutex hierarchy (#4) | Extract utilities first, document threading rules |
| Progress unification | Fragmentation (#5), Async inconsistency (#7) | Design central progress manager before splitting |
| System page extraction | Widget GC (#2), UX regression (#3) | Keep all widget refs, screenshot before/after |
| Updates page extraction | All of the above + dry-run (#9) | This page has most async complexity |
| Applications page extraction | Large list performance, duplicate patterns | Consider virtualization, extract common patterns |
| Extensions page extraction | updex/instex wrapper complexity | May need to update wrapper layer first |
| Library extraction | Early extraction (#6), Configuration coupling (#8) | Wait for patterns to emerge from internal work |
| Testing | Paralysis (#10) | Start with non-GTK code, characterization tests |

## Chairlift-Specific Pitfall Matrix

Based on codebase analysis, risk of each pitfall by current file:

| File | Highest Risks |
|------|---------------|
| `views/userhome.go` | #1 (threading), #2 (GC), #4 (mutexes), #5 (progress) |
| `pm/wrapper.go` | #7 (async), #9 (dry-run), #1 (threading—has own runOnMainThread) |
| `window/window.go` | #2 (GC—holds view refs), #3 (UX—navigation) |
| `nbc/nbc.go` | #7 (channel pattern different from pm) |
| `config/config.go` | Lowest risk—pure data, no GTK |

## Sources

- Codebase analysis: `internal/views/userhome.go` (2499 lines), `internal/pm/wrapper.go` (1048 lines)
- Project context: `.planning/PROJECT.md`, `.planning/codebase/CONCERNS.md`
- GTK4/puregotk threading model: Inferred from `runOnMainThread` implementation and `glib.IdleAdd` usage
- Confidence: HIGH for pitfalls derived from actual code patterns; MEDIUM for prevention strategies (validated approaches but context-dependent)

---

*Pitfalls research: 2026-01-26*
