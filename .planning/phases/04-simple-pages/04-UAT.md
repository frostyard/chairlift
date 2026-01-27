---
status: complete
phase: 04-simple-pages
source: [04-01-SUMMARY.md, 04-02-SUMMARY.md, 04-03-SUMMARY.md]
started: 2026-01-27T04:05:00Z
updated: 2026-01-27T04:05:00Z
---

## Current Test
<!-- OVERWRITE each test - shows where we are -->

number: complete
name: UAT Complete
expected: ""
awaiting: none

## Tests

### 1. Help Page Displays Resource Links
expected: Navigate to the Help page. The "Help & Resources" group displays link rows from config. Clicking a link opens it in browser.
result: issue
reported: "they're clickable, a log message appears. No browser opens."
severity: major

### 2. System Page Shows OS Release Info
expected: Navigate to the System page. The "System Information" expander shows parsed OS info from /etc/os-release (Pretty Name, Version, etc.). URL entries are clickable.
result: issue
reported: "clickable, but no web page opens"
severity: major

### 3. System Page Shows NBC Status
expected: On NBC-booted system, System page shows "NBC Status" group with async-loaded status details. Shows spinner while loading, then populates with NBC info.
result: pass

### 4. System Page Shows Health Link
expected: System page shows "System Health" group with a link to launch Mission Center (or configured app). Clicking launches the app.
result: issue
reported: "click, logs say launching, no launch"
severity: major

### 5. Unit Tests Pass
expected: Run `go test ./internal/pages/... -v` — all 9 tests pass (6 help, 3 system). No GTK runtime required.
result: pass
reported: "9/9 tests pass (6 help, 3 system)"

### 6. Application Compiles and Runs
expected: Run `make build && make run` — application starts without errors. Navigate between pages without crashes.
result: pass

## Summary

total: 6
passed: 3
issues: 3
pending: 0
skipped: 0

## Gaps

- truth: "Clicking a link on Help page opens it in browser"
  status: diagnosed
  reason: "User reported: they're clickable, a log message appears. No browser opens."
  severity: major
  test: 1
  root_cause: "openURL() in userhome.go is a stub - only logs, no implementation"
  artifacts: [internal/views/userhome.go]
  missing: ["Implement openURL using xdg-open or exec.Command"]

- truth: "URL entries on System page open in browser when clicked"
  status: diagnosed
  reason: "User reported: clickable, but no web page opens"
  severity: major
  test: 2
  root_cause: "openURL() in userhome.go is a stub - only logs, no implementation"
  artifacts: [internal/views/userhome.go]
  missing: ["Implement openURL using xdg-open or exec.Command"]

- truth: "System Health link launches Mission Center app"
  status: diagnosed
  reason: "User reported: click, logs say launching, no launch"
  severity: major
  test: 4
  root_cause: "launchApp() uses gtk-launch but app ID may be incorrect or app not installed"
  artifacts: [internal/views/userhome.go, internal/pages/system/page.go]
  missing: ["Verify Mission Center app ID", "Add error feedback if app not found"]
