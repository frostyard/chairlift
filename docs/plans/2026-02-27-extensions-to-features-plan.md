# Extensions → Features Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the broken Extensions tab with a Features tab using the new `updex features` CLI interface, and remove `instex` entirely.

**Architecture:** In-place refactor of the existing `updex` package and UI code. The `updex` package gets new `Feature` struct and functions for `updex features list/enable/disable/update`. The UI switches from expander rows with versions to simple action rows with toggle switches. All `instex` code and references are deleted.

**Tech Stack:** Go, puregotk (GTK4/libadwaita bindings), YAML config

---

### Task 1: Delete instex package and PolicyKit files

**Files:**
- Delete: `internal/instex/instex.go`
- Delete: `data/org.frostyard.ChairLift.instex.policy`
- Delete: `data/org.frostyard.ChairLift.instex.rules`

**Step 1: Remove instex files**

```bash
rm internal/instex/instex.go
rmdir internal/instex
rm data/org.frostyard.ChairLift.instex.policy
rm data/org.frostyard.ChairLift.instex.rules
```

**Step 2: Commit**

```bash
git add -u
git commit -m "chore: remove instex package and PolicyKit files"
```

---

### Task 2: Rewrite updex package

**Files:**
- Modify: `internal/updex/updex.go`

**Step 1: Replace the entire file**

Replace `internal/updex/updex.go` with this content:

```go
// Package updex provides an interface to system feature management via updex
package updex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"time"
)

const (
	updexCommand  = "updex"
	pkexecCommand = "pkexec"
	DefaultTimeout = 5 * time.Minute
)

var dryRun = false

// SetDryRun enables/disables dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("updex dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// DefaultContext returns a context with the default timeout
func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), DefaultTimeout)
}

// Error represents an updex-related error
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// NotFoundError is returned when updex is not installed
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// Feature represents a system feature managed by updex
type Feature struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Documentation string   `json:"documentation"`
	Enabled       bool     `json:"enabled"`
	Source        string   `json:"source"`
	Transfers     []string `json:"transfers"`
}

// IsInstalled checks if updex is installed and accessible
func IsInstalled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, updexCommand, "--version")
	err := cmd.Run()
	return err == nil
}

// runCommand executes an updex command and returns stdout, stderr, and any error
func runCommand(ctx context.Context, args ...string) (string, string, error) {
	if dryRun {
		log.Printf("[DRY-RUN] updex: %v", args)
	}

	cmd := exec.CommandContext(ctx, updexCommand, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if stderr.Len() > 0 {
		log.Printf("updex stderr: %s", stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", stderr.String(), &Error{Message: "Command timed out"}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "", stderr.String(), &NotFoundError{Message: "updex not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", stderr.String(), &Error{Message: fmt.Sprintf("command failed (exit %d): %s", exitErr.ExitCode(), stderr.String())}
		}
		return "", stderr.String(), &Error{Message: err.Error()}
	}

	return stdout.String(), stderr.String(), nil
}

// runPrivilegedCommand executes an updex command via pkexec for privileged operations
func runPrivilegedCommand(ctx context.Context, args ...string) (string, string, error) {
	if dryRun {
		log.Printf("[DRY-RUN] would execute: pkexec %s %v", updexCommand, args)
		return "", "", nil
	}

	fullArgs := append([]string{updexCommand}, args...)
	cmd := exec.CommandContext(ctx, pkexecCommand, fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if stderr.Len() > 0 {
		log.Printf("updex stderr: %s", stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", stderr.String(), &Error{Message: "Command timed out"}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "", stderr.String(), &NotFoundError{Message: "pkexec or updex not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", stderr.String(), &Error{Message: fmt.Sprintf("command failed (exit %d): %s", exitErr.ExitCode(), stderr.String())}
		}
		return "", stderr.String(), &Error{Message: err.Error()}
	}

	return stdout.String(), stderr.String(), nil
}

// ListFeatures returns all available features
func ListFeatures(ctx context.Context) ([]Feature, error) {
	output, _, err := runCommand(ctx, "features", "list", "--json")
	if err != nil {
		return nil, err
	}

	var features []Feature
	if err := json.Unmarshal([]byte(output), &features); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse JSON output: %v", err)}
	}

	return features, nil
}

// EnableFeature enables a feature for download
func EnableFeature(ctx context.Context, name string) error {
	_, _, err := runPrivilegedCommand(ctx, "features", "enable", name)
	return err
}

// DisableFeature disables a feature
func DisableFeature(ctx context.Context, name string) error {
	_, _, err := runPrivilegedCommand(ctx, "features", "disable", name)
	return err
}

// UpdateFeatures downloads enabled features
func UpdateFeatures(ctx context.Context) error {
	_, _, err := runPrivilegedCommand(ctx, "features", "update")
	return err
}
```

**Step 2: Verify it compiles (will fail until views/app are updated, but check syntax)**

Run: `cd internal/updex && go vet ./...`
Expected: PASS (the package itself should be valid)

**Step 3: Commit**

```bash
git add internal/updex/updex.go
git commit -m "feat: rewrite updex package for updex features subcommands"
```

---

### Task 3: Update config for features

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.yml`
- Modify: `CONFIG.md`

**Step 1: Update config.go**

In `internal/config/config.go`, make these changes:

1. Rename the struct field (line 18):
   - Old: `ExtensionsPage   PageConfig \`yaml:"extensions_page"\``
   - New: `FeaturesPage     PageConfig \`yaml:"features_page"\``

2. Update `defaultConfig()` (lines 139-142):
   - Old:
     ```go
     ExtensionsPage: PageConfig{
         "installed_group": GroupConfig{Enabled: true},
         "discover_group":  GroupConfig{Enabled: true},
     },
     ```
   - New:
     ```go
     FeaturesPage: PageConfig{
         "features_group": GroupConfig{Enabled: true},
     },
     ```

3. Update `IsGroupEnabled()` switch case (lines 166-167):
   - Old: `case "extensions_page": page = c.ExtensionsPage`
   - New: `case "features_page": page = c.FeaturesPage`

4. Update `GetGroupConfig()` switch case (lines 193-194):
   - Old: `case "extensions_page": page = c.ExtensionsPage`
   - New: `case "features_page": page = c.FeaturesPage`

**Step 2: Update config.yml**

Replace lines 61-65:
- Old:
  ```yaml
  extensions_page:
    installed_group:
      enabled: true
    discover_group:
      enabled: true
  ```
- New:
  ```yaml
  features_page:
    features_group:
      enabled: true
  ```

**Step 3: Update CONFIG.md**

Replace lines 66-69:
- Old:
  ```markdown
  ### Extensions Page (`extensions_page`)

  - `installed_group`: Installed systemd-sysext extensions (requires `updex` command)
  - `discover_group`: Discover and install extensions from remote repositories (requires `instex` command)
  ```
- New:
  ```markdown
  ### Features Page (`features_page`)

  - `features_group`: System features managed by updex (requires `updex` command)
  ```

**Step 4: Commit**

```bash
git add internal/config/config.go config.yml CONFIG.md
git commit -m "feat: rename extensions_page config to features_page"
```

---

### Task 4: Update app.go — remove instex

**Files:**
- Modify: `internal/app/app.go`

**Step 1: Remove instex import and dry-run call**

1. Remove import line 10: `"github.com/frostyard/chairlift/internal/instex"`
2. Remove line 47: `instex.SetDryRun(true)`
3. Update keyboard shortcut (line 86):
   - Old: `a.SetAccelsForAction("win.navigate-extensions", []string{"<Alt>5"})`
   - New: `a.SetAccelsForAction("win.navigate-features", []string{"<Alt>5"})`

**Step 2: Verify compilation**

Run: `go vet ./internal/app/...`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: remove instex from app, rename extensions shortcut to features"
```

---

### Task 5: Update window.go — rename navigation

**Files:**
- Modify: `internal/window/window.go`

**Step 1: Rename nav item**

Change line 46 in `navItems`:
- Old: `{Name: "extensions", Title: "Extensions", Icon: "application-x-addon-symbolic"},`
- New: `{Name: "features", Title: "Features", Icon: "application-x-addon-symbolic"},`

**Step 2: Fix keyboard shortcuts window**

In `onShowShortcuts()`, update `navShortcuts` (lines 334-343):
- Old:
  ```go
  navShortcuts := []struct {
      accel string
      title string
  }{
      {"Alt+1", "Go to Applications"},
      {"Alt+2", "Go to Maintenance"},
      {"Alt+3", "Go to Updates"},
      {"Alt+4", "Go to System"},
      {"Alt+5", "Go to Help"},
  }
  ```
- New:
  ```go
  navShortcuts := []struct {
      accel string
      title string
  }{
      {"Alt+1", "Go to Applications"},
      {"Alt+2", "Go to Maintenance"},
      {"Alt+3", "Go to Updates"},
      {"Alt+4", "Go to System"},
      {"Alt+5", "Go to Features"},
      {"Alt+6", "Go to Help"},
  }
  ```

**Step 3: Commit**

```bash
git add internal/window/window.go
git commit -m "feat: rename Extensions nav to Features, fix shortcuts display"
```

---

### Task 6: Rewrite features page in userhome.go

**Files:**
- Modify: `internal/views/userhome.go`

This is the largest task. Make the following changes:

**Step 1: Update imports**

Remove the `instex` import (line 17):
- Delete: `"github.com/frostyard/chairlift/internal/instex"`

**Step 2: Rename struct fields**

In the `UserHome` struct:

1. Rename page fields (lines 75, 83):
   - `extensionsPage` → `featuresPage`
   - `extensionsPrefsPage` → `featuresPrefsPage`

2. Replace extensions references section (lines 107-112):
   - Old:
     ```go
     // Extensions page references
     extensionsGroup        *adw.PreferencesGroup
     discoverEntry          *gtk.Entry
     discoverResultsGroup   *adw.PreferencesGroup
     discoverResultRows     []*adw.ActionRow
     installedComponentsMap map[string]bool
     ```
   - New:
     ```go
     // Features page references
     featuresGroup *adw.PreferencesGroup
     ```

**Step 3: Update New() function**

In `New()` (lines 133, 141):
- `uh.extensionsPage, uh.extensionsPrefsPage = uh.createPage()` → `uh.featuresPage, uh.featuresPrefsPage = uh.createPage()`
- `uh.buildExtensionsPage()` → `uh.buildFeaturesPage()`

**Step 4: Update GetPage()**

In `GetPage()` (lines 169-170):
- Old:
  ```go
  case "extensions":
      return uh.extensionsPage
  ```
- New:
  ```go
  case "features":
      return uh.featuresPage
  ```

**Step 5: Replace extensions page functions**

Delete these functions entirely:
- `buildExtensionsPage()` (lines 908-976)
- `loadExtensions()` (lines 978-1045)
- `onDiscoverClicked()` (lines 1047-1080)
- `displayDiscoveryResults()` (lines 1082-1142)
- `onInstallExtensionClicked()` (lines 1144-1177)

Replace with these new functions (insert at the same location, before `buildHelpPage()`):

```go
// buildFeaturesPage builds the Features page content
func (uh *UserHome) buildFeaturesPage() {
	page := uh.featuresPrefsPage
	if page == nil {
		return
	}

	// Features group - only show if updex is available
	if updex.IsInstalled() && uh.config.IsGroupEnabled("features_page", "features_group") {
		uh.featuresGroup = adw.NewPreferencesGroup()
		uh.featuresGroup.SetTitle("Features")
		uh.featuresGroup.SetDescription("Loading features...")

		// Add Update button as header suffix
		updateBtn := gtk.NewButtonWithLabel("Update")
		updateBtn.SetValign(gtk.AlignCenterValue)
		updateBtn.AddCssClass("suggested-action")
		updateClickedCb := func(btn gtk.Button) {
			uh.onUpdateFeaturesClicked(updateBtn)
		}
		updateBtn.ConnectClicked(&updateClickedCb)
		uh.featuresGroup.SetHeaderSuffix(&updateBtn.Widget)

		page.Add(uh.featuresGroup)

		// Load features asynchronously
		go uh.loadFeatures()
	} else if !updex.IsInstalled() {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Features")
		group.SetDescription("Manage system features")

		row := adw.NewActionRow()
		row.SetTitle("Feature Manager Not Available")
		row.SetSubtitle("The updex command is not installed on this system")
		group.Add(&row.Widget)
		page.Add(group)
	}
}

// loadFeatures loads feature information asynchronously
func (uh *UserHome) loadFeatures() {
	ctx, cancel := updex.DefaultContext()
	defer cancel()

	features, err := updex.ListFeatures(ctx)

	runOnMainThread(func() {
		if uh.featuresGroup == nil {
			return
		}

		if err != nil {
			uh.featuresGroup.SetDescription(fmt.Sprintf("Error: %v", err))
			return
		}

		if len(features) == 0 {
			uh.featuresGroup.SetDescription("No features available")
			return
		}

		uh.featuresGroup.SetDescription(fmt.Sprintf("%d features available", len(features)))

		for _, feat := range features {
			row := adw.NewActionRow()
			row.SetTitle(feat.Description)
			row.SetSubtitle(feat.Name)

			toggle := gtk.NewSwitch()
			toggle.SetActive(feat.Enabled)
			toggle.SetValign(gtk.AlignCenterValue)

			featName := feat.Name
			featEnabled := feat.Enabled
			stateSetCb := func(sw gtk.Switch, state bool) bool {
				if state == featEnabled {
					return false
				}
				uh.onFeatureToggled(featName, state)
				return false
			}
			toggle.ConnectStateSet(&stateSetCb)

			row.AddSuffix(&toggle.Widget)
			row.SetActivatableWidget(&toggle.Widget)
			uh.featuresGroup.Add(&row.Widget)
		}
	})
}

// onFeatureToggled handles enabling/disabling a feature
func (uh *UserHome) onFeatureToggled(name string, enabled bool) {
	go func() {
		ctx, cancel := updex.DefaultContext()
		defer cancel()

		var err error
		if enabled {
			err = updex.EnableFeature(ctx, name)
		} else {
			err = updex.DisableFeature(ctx, name)
		}

		runOnMainThread(func() {
			if err != nil {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to update %s: %v", name, err))
				return
			}

			if enabled {
				uh.toastAdder.ShowToast(fmt.Sprintf("%s enabled. Update to download, reboot to apply.", name))
			} else {
				uh.toastAdder.ShowToast(fmt.Sprintf("%s disabled. Update to apply, reboot to complete.", name))
			}
		})
	}()
}

// onUpdateFeaturesClicked handles the Update button click
func (uh *UserHome) onUpdateFeaturesClicked(button *gtk.Button) {
	button.SetSensitive(false)
	button.SetLabel("Updating...")

	go func() {
		ctx, cancel := updex.DefaultContext()
		defer cancel()

		err := updex.UpdateFeatures(ctx)

		runOnMainThread(func() {
			button.SetSensitive(true)
			button.SetLabel("Update")

			if err != nil {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", err))
				return
			}

			uh.toastAdder.ShowToast("Features updated. Changes apply after reboot.")
		})
	}()
}
```

**Step 6: Verify the full project compiles**

Run: `go build ./...`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/views/userhome.go
git commit -m "feat: replace extensions page with features page using updex features"
```

---

### Task 7: Update documentation

**Files:**
- Modify: `.planning/codebase/INTEGRATIONS.md`
- Modify: `.planning/codebase/STRUCTURE.md`
- Modify: `.planning/codebase/ARCHITECTURE.md`

**Step 1: Update INTEGRATIONS.md**

Find the instex section (lines ~31-41) and replace both updex and instex entries with:

```markdown
**System Features:**
- updex - Manage system features (list, enable, disable, update)
  - Implementation: `internal/updex/updex.go`
  - Commands executed: `updex features list --json`, `updex --version`
  - Auth: pkexec for enable/disable/update (read-only for list)
```

Remove any standalone instex entry.

**Step 2: Update STRUCTURE.md**

Remove `internal/instex/` references. Update `internal/updex/` description to reflect features.

**Step 3: Update ARCHITECTURE.md**

Remove instex from dependency list. Update updex description.

**Step 4: Commit**

```bash
git add .planning/
git commit -m "docs: update planning docs for features redesign"
```

---

### Task 8: Build and verify

**Step 1: Run gofmt**

Run: `gofmt -s -w .`
Expected: No changes (code should already be formatted)

**Step 2: Run go vet**

Run: `go vet ./...`
Expected: PASS

**Step 3: Run full build**

Run: `go build ./...`
Expected: PASS

**Step 4: Run with dry-run to verify startup**

Run: `go run ./cmd/chairlift --dry-run`
Expected: Application launches, Features tab visible in sidebar, no crashes

**Step 5: Commit any formatting fixes if needed**

```bash
git add -A
git commit -m "chore: formatting cleanup"
```
