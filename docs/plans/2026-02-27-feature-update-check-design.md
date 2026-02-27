# Feature Update Check Design

## Problem

The Features page shows enabled/disabled features but no version or update information. Users can't see whether their enabled features have updates available without running CLI commands manually.

## Solution

Add `updex features check --json` integration to show version info and update availability on each enabled feature row. Uses sequential loading: feature list loads first, then check results enrich the UI progressively.

## `updex features check --json` Output

```json
[
  {
    "feature": "docker",
    "results": [
      {
        "component": "docker",
        "current_version": "5:29.2.1-1~debian.13~trixie",
        "newest_version": "5:29.2.1-1~debian.13~trixie",
        "update_available": false
      }
    ]
  }
]
```

Only returns results for enabled features. Read-only, no pkexec needed.

## `updex` Package Changes

### New types

```go
type CheckResult struct {
    Component       string `json:"component"`
    CurrentVersion  string `json:"current_version"`
    NewestVersion   string `json:"newest_version"`
    UpdateAvailable bool   `json:"update_available"`
}

type FeatureCheck struct {
    Feature string        `json:"feature"`
    Results []CheckResult `json:"results"`
}
```

### New function

- `CheckFeatures(ctx) ([]FeatureCheck, error)` — runs `updex features check --json` via `runCommand` (no pkexec)

## UI Changes

### Subtitle enrichment for enabled features

- No update: `"docker — v5:29.2.1-1~debian.13~trixie"`
- Update available: `"docker — v1.0.141 → v1.0.142 available"`
- Disabled features: subtitle stays as just the feature name

### Group description update

When updates are available, description changes from "9 features available" to "9 features available (2 updates)".

### Sequential loading flow

1. `loadFeatures()` loads feature list, renders rows (existing behavior)
2. While rendering, stores `map[string]*adw.ActionRow` mapping feature name → row reference
3. At end of render, kicks off `go uh.checkFeatureUpdates()`
4. `checkFeatureUpdates()` calls `updex.CheckFeatures(ctx)`, then on main thread updates subtitles and group description

### Error handling

If `CheckFeatures` fails, log the error silently. The feature list remains fully usable — version info is a progressive enhancement.

## Files Changed

- Modify: `internal/updex/updex.go` — add `CheckResult`, `FeatureCheck`, `CheckFeatures()`
- Modify: `internal/views/userhome.go` — add `featureRows` field, store refs in `loadFeatures()`, add `checkFeatureUpdates()`
