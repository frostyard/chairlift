# Requirements: Chairlift Refactoring

**Defined:** 2026-01-26
**Core Value:** Less technical users can confidently manage their immutable Linux desktop without needing to understand the underlying CLI tools or immutable filesystem concepts.

## v1 Requirements

Requirements for this refactoring milestone. Each maps to roadmap phases.

### Infrastructure

- [ ] **INFR-01**: All async operations use a unified pattern with consistent progress, cancellation, and error handling
- [ ] **INFR-02**: Error messages are human-readable and explain what the user can do next
- [ ] **INFR-03**: runOnMainThread is consolidated into a shared async package
- [ ] **INFR-04**: Callback registry prevents GC collection of goroutine references
- [ ] **INFR-05**: Operation tracker shows all ongoing operations in one place
- [ ] **INFR-06**: All long-running operations are cancellable by the user
- [ ] **INFR-07**: Recently completed operations are visible in operation history

### Component Architecture

- [ ] **ARCH-01**: userhome.go monolith is split into separate packages per feature area (system, updates, applications, maintenance, extensions, help)
- [ ] **ARCH-02**: AsyncExpanderRow pattern is extracted as reusable widget
- [ ] **ARCH-03**: ActionButton pattern is extracted as reusable widget
- [ ] **ARCH-04**: LoadingRow pattern is extracted as reusable widget
- [ ] **ARCH-05**: Widget builder functions exist for common ActionRow creation patterns
- [ ] **ARCH-06**: Business logic is separated from UI code for testability
- [ ] **ARCH-07**: Component lifecycle is properly managed (goroutine cleanup, callback cleanup)

### User Feedback

- [ ] **FDBK-01**: All async data loads show consistent spinner/loading state
- [ ] **FDBK-02**: Operations longer than 30 seconds show progress bars
- [ ] **FDBK-03**: Interactive controls are disabled during their associated operations
- [ ] **FDBK-04**: Empty states use placeholder pages with guidance text
- [ ] **FDBK-05**: Persistent state (like dry-run mode) is shown via status banners
- [ ] **FDBK-06**: Failed operations show retry option

### Accessibility

- [ ] **ACCS-01**: Icon-only buttons have accessible labels for screen readers
- [ ] **ACCS-02**: Keyboard navigation works for all interactive elements
- [ ] **ACCS-03**: Widgets have proper relationship labeling for screen reader context

### Library Extraction

- [ ] **LIBR-01**: Extractable patterns are identified and documented during refactoring
- [ ] **LIBR-02**: Reusable GTK4/Go code is extracted into a separate library

### Testing

- [ ] **TEST-01**: Business logic (config, command builders, PM wrapper logic) has unit tests
- [ ] **TEST-02**: Page construction has integration tests that verify no panics

### Integration

- [ ] **INTG-01**: updex functionality uses the Go library directly instead of CLI wrapper

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Enhanced Features

- **FEAT-01**: Window state (size, position) persists between sessions
- **FEAT-02**: Desktop notifications when background operations complete
- **FEAT-03**: Undo support for destructive actions
- **FEAT-04**: State restoration after app restart
- **FEAT-05**: Confirmation dialogs for destructive actions (alternative to undo)

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Mobile/responsive layouts | Desktop-only GTK4 application |
| Web interface | Native desktop app only |
| Windows/macOS support | Linux-only, relies on GTK4/Libadwaita |
| Custom package manager backends | Uses existing frostyard/pm library |
| Progress dialogs/windows | Anti-pattern per GNOME HIG; use inline progress |
| Redux/Elm-style state management | Over-engineering for this scale; channels + runOnMainThread sufficient |
| Custom widget accessibility | Too complex; use standard GTK4/Adwaita widgets |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| INFR-01 | — | Pending |
| INFR-02 | — | Pending |
| INFR-03 | — | Pending |
| INFR-04 | — | Pending |
| INFR-05 | — | Pending |
| INFR-06 | — | Pending |
| INFR-07 | — | Pending |
| ARCH-01 | — | Pending |
| ARCH-02 | — | Pending |
| ARCH-03 | — | Pending |
| ARCH-04 | — | Pending |
| ARCH-05 | — | Pending |
| ARCH-06 | — | Pending |
| ARCH-07 | — | Pending |
| FDBK-01 | — | Pending |
| FDBK-02 | — | Pending |
| FDBK-03 | — | Pending |
| FDBK-04 | — | Pending |
| FDBK-05 | — | Pending |
| FDBK-06 | — | Pending |
| ACCS-01 | — | Pending |
| ACCS-02 | — | Pending |
| ACCS-03 | — | Pending |
| LIBR-01 | — | Pending |
| LIBR-02 | — | Pending |
| TEST-01 | — | Pending |
| TEST-02 | — | Pending |
| INTG-01 | — | Pending |

**Coverage:**
- v1 requirements: 28 total
- Mapped to phases: 0
- Unmapped: 28 ⚠️

---
*Requirements defined: 2026-01-26*
*Last updated: 2026-01-26 after initial definition*
