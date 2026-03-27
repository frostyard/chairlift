# Extensions → Features Redesign

## Problem

The Extensions tab is broken. It relies on two separate CLI tools (`updex` and `instex`) with an outdated command interface. The `updex` tool has been updated to consolidate functionality under `updex features` subcommands, making `instex` unnecessary.

## Solution

Replace the Extensions tab with a Features tab that uses the new `updex features` CLI interface. Remove `instex` entirely.

## New `updex features` CLI Interface

### `updex features list --json`

Returns all features (available and enabled) in a single call:

```json
[
  {
    "name": "docker",
    "description": "Docker Containers",
    "documentation": "https://frostyard.org",
    "enabled": true,
    "source": "/usr/lib/sysupdate.d/docker.feature",
    "transfers": ["docker"]
  }
]
```

### `updex features enable <name>`

Marks a feature for download. Requires pkexec. Does not download or apply — just enables the sysupdate definition.

### `updex features disable <name>`

Removes a feature from the download list. Requires pkexec.

### `updex features update`

Downloads enabled features. Requires pkexec. Changes apply after reboot.

## `updex` Package Changes

### New struct

```go
type Feature struct {
    Name          string   `json:"name"`
    Description   string   `json:"description"`
    Documentation string   `json:"documentation"`
    Enabled       bool     `json:"enabled"`
    Source        string   `json:"source"`
    Transfers     []string `json:"transfers"`
}
```

### New functions

- `ListFeatures(ctx) ([]Feature, error)` — runs `updex features list --json`
- `EnableFeature(ctx, name) error` — runs `pkexec updex features enable <name>`
- `DisableFeature(ctx, name) error` — runs `pkexec updex features disable <name>`
- `UpdateFeatures(ctx) error` — runs `pkexec updex features update`
- `runPrivilegedCommand(ctx, args...) (string, string, error)` — executes via pkexec, respects dry-run

### Removed

- `Extension` struct
- `List()`, `ListInstalled()`

## UI Design

### Features Page Layout

Single "Features" group with:

- **Header:** "Update" button (suggested-action style) — runs `pkexec updex features update`
- **Feature rows:** `ActionRow` for each feature with:
  - Title: feature description (human-readable, e.g., "Docker Containers")
  - Subtitle: feature name (technical, e.g., "docker")
  - Suffix: `gtk.Switch` reflecting `enabled` state

### Interaction Flow

1. Toggle switch ON → `pkexec updex features enable <name>` → toast: "Feature enabled. Update to download, reboot to apply."
2. Toggle switch OFF → `pkexec updex features disable <name>` → toast with similar guidance
3. Click "Update" → `pkexec updex features update` → progress indication → success/error toast: "Features updated. Changes apply after reboot."

### When updex not installed

Show "Feature Manager Not Available" message (same pattern as current code).

## Renaming

### Navigation (window.go)

- Nav item: `extensions` → `features`, title "Features"
- Icon stays `application-x-addon-symbolic`

### Keyboard Shortcuts (app.go)

- Action: `win.navigate-extensions` → `win.navigate-features` (Alt+5)
- Fix shortcuts window: add "Go to Features" at Alt+5, "Go to Help" at Alt+6

### Config

- `ExtensionsPage` → `FeaturesPage`, yaml tag `features_page`
- Single group: `features_group` (remove `discover_group`, `installed_group`)
- Update `IsGroupEnabled`/`GetGroupConfig` switch cases
- Update `config.yml`

### Views (userhome.go)

- Fields: `extensionsPage` → `featuresPage`, `extensionsPrefsPage` → `featuresPrefsPage`
- `extensionsGroup` → `featuresGroup`
- Remove: `discoverEntry`, `discoverResultsGroup`, `discoverResultRows`, `installedComponentsMap`
- Remove: `buildExtensionsPage`, `loadExtensions`, `onDiscoverClicked`, `displayDiscoveryResults`, `onInstallExtensionClicked`
- Add: `buildFeaturesPage`, `loadFeatures`, `onFeatureToggled`
- Remove `instex` import

### App (app.go)

- Remove `instex` import and `instex.SetDryRun(true)`

## Deletions

- `internal/instex/` — entire directory
- `data/org.frostyard.ChairLift.instex.policy`
- `data/org.frostyard.ChairLift.instex.rules`

## Kept As-Is

- `data/org.frostyard.ChairLift.updex.policy` — existing actions cover new commands
- `data/org.frostyard.ChairLift.updex.rules` — sudo group rules still apply
