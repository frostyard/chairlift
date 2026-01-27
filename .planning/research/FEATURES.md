# Feature Landscape: GTK4/Libadwaita Desktop Application Patterns

**Domain:** GTK4/Libadwaita desktop application (system management tool)
**Target Users:** Less technical users requiring clear feedback and understandable errors
**Researched:** 2026-01-26
**Overall Confidence:** HIGH (based on official GNOME documentation and HIG)

## Table Stakes

Features users expect from a production-ready GTK4/Libadwaita desktop application. Missing = product feels incomplete or amateurish.

### User Feedback & Progress

| Feature | Why Expected | Complexity | Current State | Notes |
|---------|--------------|------------|---------------|-------|
| **Toasts for transient feedback** | HIG standard for showing operation results; users expect non-blocking confirmation | Low | Present (ShowToast/ShowErrorToast) | Already implemented via AdwToastOverlay |
| **Spinners for loading states** | Users expect visual indication something is happening; prevents confusion | Low | Partial (used in some places) | Should be consistent across all async loads |
| **Progress bars for long operations** | Required for operations >30s per HIG; users need to know when things will complete | Medium | Present but inconsistent (NBC update has it, others don't) | Needs unified pattern |
| **Disabled state during operations** | Prevents double-submission; shows operation is in progress | Low | Present but inconsistent | Button.SetSensitive(false) used ad-hoc |
| **Clear error messages** | Less technical users need to understand what went wrong | Low | Present but raw (shows Go error strings) | Need user-friendly error messages |

### Accessibility

| Feature | Why Expected | Complexity | Current State | Notes |
|---------|--------------|------------|---------------|-------|
| **Keyboard navigation** | Required for accessibility; GNOME expectation | Low | Likely inherited from GTK | Verify works for all interactive elements |
| **Accessible labels for icon-only buttons** | Screen readers need text descriptions; GNOME accessibility requirement | Low | Partial (SetTooltipText used some places) | Use GTK_ACCESSIBLE_PROPERTY_LABEL |
| **High-contrast mode support** | Accessibility setting that must work | Low | Inherited from GTK/Adwaita | Test with high-contrast enabled |
| **Large text mode support** | Accessibility setting; UI must remain usable | Low | Inherited from GTK/Adwaita | Test with large text enabled |
| **Widget relationship labeling** | Screen readers need context; GNOME accessibility requirement | Medium | Not implemented | Use GTK_ACCESSIBLE_RELATION_LABELLED_BY |

### Application Structure

| Feature | Why Expected | Complexity | Current State | Notes |
|---------|--------------|------------|---------------|-------|
| **Consistent navigation** | Users expect predictable app structure | Low | Present (sidebar + stack) | Already using AdwNavigationSplitView |
| **Keyboard shortcuts** | Power users expect efficiency; GNOME standard | Low | Present | Already has Ctrl+Q, Alt+1-6, etc. |
| **Application menu** | Standard GNOME pattern for app-level actions | Low | Present | Hamburger menu implemented |
| **Window state persistence** | Users expect window size/position remembered | Low | Not implemented | Use GSettings or config file |
| **Responsive/adaptive layout** | Desktop apps should handle different window sizes | Medium | Partial (split view adapts) | NavigationSplitView provides this |

### Error Handling

| Feature | Why Expected | Complexity | Current State | Notes |
|---------|--------------|------------|---------------|-------|
| **Non-blocking error display** | Errors shouldn't stop user flow for recoverable issues | Low | Present (error toasts) | Good pattern already |
| **Error context preservation** | Users need to understand what failed and why | Medium | Missing | Errors lose context during goroutine transitions |
| **Graceful degradation** | Features should work even when dependencies unavailable | Low | Present | Already checks pm.FlatpakIsInstalled(), etc. |
| **Retry capability** | Users should be able to retry failed operations | Low | Missing | No retry buttons on failure |

## Differentiators

Features that elevate the app above average quality. Not expected, but valued when present.

### Unified Async Operation System

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Operation queue/tracker** | Shows all ongoing operations in one place; prevents confusion about what's happening | High | Current BottomSheet approach is good concept, needs unification |
| **Unified cancellation support** | All long operations should be cancellable; gives users control | Medium | Some operations have cancel, others don't |
| **Operation history** | Shows what completed recently; helpful for understanding what happened | Medium | Currently toasts are transient and disappear |
| **Automatic retry with backoff** | Network operations gracefully handle temporary failures | Medium | Not implemented |
| **Parallel operation coordination** | Multiple operations can run without blocking each other | High | Currently possible but no coordination |

### Enhanced User Feedback

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Contextual help text** | Explains what each action does before user clicks | Low | Some subtitles present, could be more comprehensive |
| **Confirmation dialogs for destructive actions** | Per HIG, destructive actions need confirmation OR undo | Medium | Uninstall buttons have no confirmation |
| **Undo support** | Better than confirmation per HIG; allows recovery from mistakes | High | Not implemented; complex for system operations |
| **Status banners** | Persistent state information (not transient like toasts) | Low | Not used; could show "Dry run mode active" |
| **Placeholder pages** | Better empty states with guidance | Low | Not implemented; uses basic "Loading..." text |

### Component Architecture

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Reusable async widget patterns** | Consistent loading/error/success states across views | Medium | Currently each view handles this differently |
| **Widget factory/builders** | Reduce code duplication for common patterns | Medium | Same ActionRow creation patterns repeated everywhere |
| **View-Model separation** | Business logic separated from UI; easier testing | High | Currently mixed in userhome.go monolith |
| **Component lifecycle management** | Proper cleanup of goroutines and callbacks | Medium | GC-prevention pattern in place, but ad-hoc |

### State Management

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Centralized app state** | Single source of truth for application state | Medium | Currently distributed across UserHome fields |
| **State change notifications** | UI updates automatically when state changes | High | Currently manual runOnMainThread calls |
| **Persistent preferences** | Remember user settings between sessions | Low | Config exists but limited |
| **State restoration** | Resume previous session state after restart | Medium | Not implemented |

### Polish Features

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Smooth animations** | Professional feel; GTK4 supports this well | Low | Inherited from Adwaita |
| **Desktop notifications** | Alert users when background operations complete | Low | Not implemented |
| **System tray/background mode** | Monitor for updates without window open | Medium | Not relevant for current use case |
| **Theming support** | Follows system light/dark mode | Low | Inherited from Adwaita |

## Anti-Features

Features to explicitly NOT build. Common mistakes in this domain that waste effort or harm UX.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| **Progress windows/dialogs** | Per HIG: consequence of closing unclear, obscures useful controls | Use inline progress bars, thin header bar progress, or BottomSheet |
| **Synchronous blocking operations** | Freezes UI, terrible UX, no cancellation possible | Use goroutines with runOnMainThread callback |
| **Alert dialogs for non-critical errors** | Per HIG: disruptive, breaks flow | Use toasts for simple errors; reserve dialogs for truly critical issues |
| **Raw technical error messages** | Less technical users can't understand them | Translate to user-friendly messages with actionable guidance |
| **Multiple simultaneous toasts** | Overwhelming, messages get lost | Queue toasts or use status banners for persistent states |
| **Activity mode progress (spinning indefinitely)** | Per HIG: avoid for long periods, users hate uncertainty | Calculate/estimate actual progress when possible |
| **Unexpected dialogs** | Per HIG: dialogs only in response to user action | Never pop up dialogs without user initiation |
| **Custom widget accessibility** | Very complex to get right | Use standard GTK4/Adwaita widgets that have built-in accessibility |
| **Over-engineering state management** | Redux/Elm patterns add complexity without GTK4 benefits | Go channels + runOnMainThread is appropriate for this scale |
| **Premature abstraction** | Widget factories before patterns are clear | Build concrete implementations first, then extract common patterns |

## Feature Dependencies

```
Core Dependencies:
├── runOnMainThread utility (exists)
│   └── All async UI updates depend on this
│
├── Config system (exists)
│   └── Persistent preferences
│   └── Window state persistence
│
└── ToastAdder interface (exists)
    └── All user feedback

Recommended Build Order:
1. Error handling improvements (no dependencies)
2. Unified async pattern (depends on error handling)
3. Accessibility improvements (can parallelize with above)
4. Component extraction (depends on stable async pattern)
5. Enhanced state management (depends on component architecture)
```

## MVP Recommendation for Refactoring

For the refactoring milestone, prioritize in this order:

### Phase 1: Foundation (Table Stakes)
1. **Unified async operation pattern** - Single way to handle all async operations
2. **Consistent error handling** - User-friendly messages, proper context preservation
3. **Spinner/loading states** - Consistent across all async loads

### Phase 2: Reliability
4. **Cancellation support** - For all long-running operations
5. **Progress indication** - Extend progress pattern to more operations
6. **Accessibility labels** - Screen reader support for icon buttons

### Phase 3: Structure
7. **Component extraction** - Break up userhome.go monolith into focused components
8. **View-specific files** - One file per page view
9. **Shared widget builders** - Extract common ActionRow creation patterns

### Defer to post-refactor:
- **Undo support** - Complex for system operations, questionable value
- **Desktop notifications** - Enhancement, not required for production-ready
- **State restoration** - Nice-to-have, not blocking

## Key Recommendations

### For Less Technical Users (Target Audience)

1. **Error messages should explain what to do**
   - Bad: "Error: context canceled"
   - Good: "Operation was cancelled. You can try again when ready."

2. **Use plain language**
   - Bad: "Flatpak uninstall failed: ENOENT"
   - Good: "Couldn't remove [App Name]. The application may have already been uninstalled."

3. **Provide next steps**
   - When an operation fails, suggest what to try
   - When an operation succeeds with side effects, explain them

4. **Show progress meaningfully**
   - "Downloading update (12 of 30 MB)" not just a spinning indicator
   - "Step 2 of 5: Verifying image" for multi-step operations

### For Production-Ready Quality

1. **Every async operation needs**:
   - Loading state (spinner or progress)
   - Success feedback (toast)
   - Error feedback (toast with user-friendly message)
   - Cancellation option (for operations >10s)
   - Disabled controls during operation

2. **Every destructive action needs**:
   - Either confirmation dialog OR undo capability (prefer undo per HIG)
   - Clear indication it's destructive (destructive-action CSS class)

3. **Every view needs**:
   - Loading state for initial data fetch
   - Empty state when no data (placeholder page)
   - Error state with retry option

## Sources

### Official GNOME Documentation (HIGH confidence)
- GNOME HIG - Feedback Patterns: https://developer.gnome.org/hig/patterns/feedback.html
- GNOME HIG - Toasts: https://developer.gnome.org/hig/patterns/feedback/toasts.html
- GNOME HIG - Progress Bars: https://developer.gnome.org/hig/patterns/feedback/progress-bars.html
- GNOME HIG - Dialogs: https://developer.gnome.org/hig/patterns/feedback/dialogs.html
- GNOME HIG - Placeholder Pages: https://developer.gnome.org/hig/patterns/feedback/placeholders.html
- GNOME HIG - Accessibility: https://developer.gnome.org/hig/guidelines/accessibility.html
- GNOME Developer - Accessibility Coding Guidelines: https://developer.gnome.org/documentation/guidelines/accessibility/coding-guidelines.html
- GNOME Developer - Asynchronous Programming: https://developer.gnome.org/documentation/tutorials/asynchronous-programming.html
- GNOME Developer - Threading: https://developer.gnome.org/documentation/tutorials/threading.html

### Codebase Analysis (HIGH confidence)
- internal/views/userhome.go - 2498 line monolith with current patterns
- internal/window/window.go - Application structure
- internal/app/app.go - Application initialization
