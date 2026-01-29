---
status: diagnosed
phase: 09-testing-library
source: 09-01-SUMMARY.md, 09-02-SUMMARY.md, 09-03-SUMMARY.md, 09-04-SUMMARY.md, 09-05-SUMMARY.md
started: 2026-01-28T17:45:00Z
updated: 2026-01-28T17:55:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Unit Tests Pass
expected: Run `go test ./...` in project root. All tests should pass with no failures.
result: pass

### 2. Basic Example Builds
expected: Run `go build ./pkg/adwutil/examples/basic` - builds without errors.
result: pass

### 3. Basic Example Runs
expected: Run the basic example binary. A window appears with a preferences group showing several rows (link row, info row, button row, icon row). Clicking the button with warning icon shows a toast.
result: issue
reported: "there's no button with a warning icon"
severity: major

### 4. Operations Example Builds
expected: Run `go build ./pkg/adwutil/examples/operations` - builds without errors.
result: pass

### 5. Operations Example Runs
expected: Run the operations example binary. A window shows an operations list. Click "Start" to begin a tracked operation. Progress bar fills over time. After ~6 seconds, operation auto-cancels demonstrating cancellation.
result: pass

### 6. README Documentation Exists
expected: `pkg/adwutil/README.md` exists and contains Quick Start section, API reference, and usage examples.
result: pass

### 7. Backward Compatibility
expected: Run main application `go run ./cmd/chairlift` - app starts normally, existing functionality works (internal packages re-export from adwutil).
result: issue
reported: "the flatpak applications section shows all applications as system flatpaks, but many of them are actually user flatpaks"
severity: major

## Summary

total: 7
passed: 5
issues: 2
pending: 0
skipped: 0

## Gaps

- truth: "Basic example shows button with warning icon that triggers toast"
  status: failed
  reason: "User reported: there's no button with a warning icon"
  severity: major
  test: 3
  root_cause: "Requirements mismatch - UAT expected button with warning icon and toast, but implementation plan (09-05-PLAN.md) never specified this feature. The example has icon rows but no clickable button that triggers a toast."
  artifacts:
    - path: "pkg/adwutil/examples/basic/main.go"
      issue: "Contains button rows, link rows, info rows, icon rows, but no button with warning icon that triggers a toast"
  missing:
    - "Add a button row with warning icon suffix that triggers a toast notification when clicked"
    - "May need to add toast helper to adwutil library"
  debug_session: ""

- truth: "Flatpak applications section correctly distinguishes user vs system flatpaks"
  status: failed
  reason: "User reported: the flatpak applications section shows all applications as system flatpaks, but many of them are actually user flatpaks"
  severity: major
  test: 7
  root_cause: "pm library Namespace field may not be populated correctly - wrapper.go:156 checks pkg.Ref.Namespace == 'user' but if Namespace is empty (fallback parsing path), all apps are classified as system"
  artifacts:
    - path: "internal/pm/wrapper.go"
      issue: "Line 156 - Uses simple string equality without defensive checking for empty Namespace"
    - path: "internal/pages/applications/page.go"
      issue: "Lines 316-325 - Apps separated into userApps/systemApps based on IsUser with no validation"
  missing:
    - "Debug logging to verify actual Namespace values returned by pm library"
    - "Defensive handling for empty or unexpected Namespace values"
    - "May be upstream pm library issue with flatpak output parsing"
  debug_session: ""
