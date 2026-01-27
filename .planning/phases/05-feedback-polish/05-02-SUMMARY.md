# Phase 05 Plan 02: Dry-Run Banner & Retry Wiring Summary

**One-liner:** Dismissible dry-run mode banner at window top, RetryFunc wiring for failed Homebrew updates

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Add dry-run banner to window | 8fbaf5a | internal/window/window.go |
| 2 | Wire RetryFunc for Update Homebrew | b48e57e | internal/views/userhome.go |

## What Was Built

### Dry-Run Mode Banner
- Added `dryRunBanner *adw.Banner` field to Window struct
- Banner displays "Dry-Run Mode: Changes will be simulated only"
- Revealed based on `pm.IsDryRun()` at window creation
- Dismissible via "Understood" button (acknowledgment action)
- Widget hierarchy: banner → progressSheet → toasts

### RetryFunc Wiring
- `onUpdateHomebrewClicked` now sets `op.RetryFunc` after `operations.Start()`
- Self-referential function enables retry via operations popover
- When user clicks Retry, operation restarts from beginning

## Key Implementation Details

**Banner callback signature:**
```go
bannerClickedCb := func(banner adw.Banner) {
    w.dryRunBanner.SetRevealed(false)
}
w.dryRunBanner.ConnectButtonClicked(&bannerClickedCb)
```

**RetryFunc pattern:**
```go
op.RetryFunc = func() {
    uh.onUpdateHomebrewClicked(button)
}
```

## Deviations from Plan

None - plan executed exactly as written.

## Verification Results

- `go build ./...` ✓
- `go vet ./...` ✓
- Banner visibility controlled by pm.IsDryRun() ✓
- RetryFunc properly wired for Update Homebrew ✓

## Duration

~1 minute

---
*Completed: 2026-01-27*
