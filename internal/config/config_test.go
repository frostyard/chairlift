package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// pageNames lists every page key Config exposes, matching the switch
// statements in IsGroupEnabled/GetGroupConfig. Tests loop over this slice
// (and, within each page, every group defaultConfig() defines) instead of
// sampling a single page/group, per the repo's
// regression-tests-must-cover-every-collection-entry skill.
var pageNames = []string{
	"system_page",
	"updates_page",
	"applications_page",
	"maintenance_page",
	"features_page",
	"help_page",
}

// pagesOf returns cfg's pages keyed by the same page-name strings
// IsGroupEnabled/GetGroupConfig switch on, so tests can loop generically.
func pagesOf(cfg *Config) map[string]PageConfig {
	return map[string]PageConfig{
		"system_page":       cfg.SystemPage,
		"updates_page":      cfg.UpdatesPage,
		"applications_page": cfg.ApplicationsPage,
		"maintenance_page":  cfg.MaintenancePage,
		"features_page":     cfg.FeaturesPage,
		"help_page":         cfg.HelpPage,
	}
}

func groupsEqual(t *testing.T, page, name string, got, want GroupConfig) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("page %q group %q: got %+v, want %+v", page, name, got, want)
	}
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test config file: %v", err)
	}
	return path
}

// withConfigPaths points the package-level configPaths search list at paths
// (typically a single nonexistent path, to force the "no config file found"
// fallback) for the duration of the calling test, restoring the original
// list afterward. This exercises Load()'s real fallback logic rather than a
// test-only stand-in.
func withConfigPaths(t *testing.T, paths []string) {
	t.Helper()
	orig := configPaths
	t.Cleanup(func() { configPaths = orig })
	configPaths = paths
}

// TestLoadFromPathUnreadablePathReturnsError confirms loadFromPath surfaces
// an error for a nonexistent/unreadable path, which is what drives Load()'s
// defaultConfig() fallback exercised by TestLoadAbsentFileFallsBackToDefaultConfig.
func TestLoadFromPathUnreadablePathReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yml")
	if _, err := loadFromPath(path); err == nil {
		t.Fatalf("loadFromPath(%q): expected error for nonexistent file, got nil", path)
	}
}

// TestLoadAbsentFileFallsBackToDefaultConfig asserts that when no config
// file can be found, Load() (the real production entry point) returns a
// *Config equal to defaultConfig() field-for-field, across every page and
// every group defaultConfig() defines.
func TestLoadAbsentFileFallsBackToDefaultConfig(t *testing.T) {
	withConfigPaths(t, []string{filepath.Join(t.TempDir(), "does-not-exist.yml")})

	got := pagesOf(Load())
	want := pagesOf(defaultConfig())

	for _, page := range pageNames {
		gotGroups := got[page]
		wantGroups := want[page]
		if len(gotGroups) != len(wantGroups) {
			t.Errorf("page %q: got %d groups, want %d", page, len(gotGroups), len(wantGroups))
		}
		for name, wantGroup := range wantGroups {
			gotGroup, ok := gotGroups[name]
			if !ok {
				t.Errorf("page %q group %q: missing from fallback config", page, name)
				continue
			}
			groupsEqual(t, page, name, gotGroup, wantGroup)
		}
	}
}

// TestMaintenanceCleanupGroupDefaultConsistentAcrossAbsentAndOmitted pins
// down that maintenance_cleanup_group resolves to the identical
// GroupConfig{Enabled:false, Actions:[bls-gc entry]} whether the config file
// is entirely absent or present but silent about maintenance_cleanup_group
// specifically (while still mentioning maintenance_page and another of its
// groups).
func TestMaintenanceCleanupGroupDefaultConsistentAcrossAbsentAndOmitted(t *testing.T) {
	wantGroup := GroupConfig{
		Enabled: false,
		Actions: []ActionConfig{
			{
				Title:  "Clean Up Boot Old Entries",
				Script: "/usr/libexec/bls-gc",
				Sudo:   true,
			},
		},
	}

	// Sanity: wantGroup must match defaultConfig()'s actual value, not a
	// hand-duplicated guess that could drift from the real default.
	if actual := defaultConfig().MaintenancePage["maintenance_cleanup_group"]; !reflect.DeepEqual(actual, wantGroup) {
		t.Fatalf("test setup assumption violated: defaultConfig() maintenance_cleanup_group is %+v, want %+v", actual, wantGroup)
	}

	// Absent-file case.
	withConfigPaths(t, []string{filepath.Join(t.TempDir(), "does-not-exist.yml")})
	absentCfg := Load()
	groupsEqual(t, "maintenance_page", "maintenance_cleanup_group",
		absentCfg.MaintenancePage["maintenance_cleanup_group"], wantGroup)

	// Partial-file case: mentions maintenance_page and a sibling group, but
	// omits maintenance_cleanup_group entirely.
	path := writeConfigFile(t, "maintenance_page:\n  maintenance_brew_group:\n    enabled: false\n")
	partialCfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("loadFromPath(%q): %v", path, err)
	}
	groupsEqual(t, "maintenance_page", "maintenance_cleanup_group",
		partialCfg.MaintenancePage["maintenance_cleanup_group"], wantGroup)
}

// defaultBearingGroups lists every group in defaultConfig() that defines a
// non-Enabled default field (AppID, Website/Issues/Chat, Actions, or
// BundlesPaths). Enabled-only-overlay coverage loops over all of them, not
// just one, per the repo's regression-tests-must-cover-every-collection-entry
// skill.
var defaultBearingGroups = []struct {
	page  string
	group string
}{
	{"system_page", "health_group"},
	{"applications_page", "applications_installed_group"},
	{"help_page", "help_resources_group"},
	{"maintenance_page", "maintenance_cleanup_group"},
	{"applications_page", "brew_bundles_group"},
}

// TestEnabledOnlyOverlayPreservesOtherDefaultFields feeds a partial file
// that sets only `enabled` (true, then separately false) on a group, and
// asserts every other default-bearing field on that group is unchanged from
// defaultConfig(). Exercised for every group above, not a single sample.
func TestEnabledOnlyOverlayPreservesOtherDefaultFields(t *testing.T) {
	defPages := pagesOf(defaultConfig())

	for _, gc := range defaultBearingGroups {
		for _, enabledVal := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/%s/enabled=%v", gc.page, gc.group, enabledVal), func(t *testing.T) {
				content := fmt.Sprintf("%s:\n  %s:\n    enabled: %v\n", gc.page, gc.group, enabledVal)
				path := writeConfigFile(t, content)

				cfg, err := loadFromPath(path)
				if err != nil {
					t.Fatalf("loadFromPath(%q): %v", path, err)
				}

				got := pagesOf(cfg)[gc.page][gc.group]
				want := defPages[gc.page][gc.group]
				want.Enabled = enabledVal

				groupsEqual(t, gc.page, gc.group, got, want)
			})
		}
	}
}

// TestOmittedEnabledInheritsDocumentedDefault asserts that a group present
// in the file (with some other field set) but silent about `enabled`
// inherits the documented default Enabled value from defaultConfig() -- true
// for an ordinary group, false for maintenance_cleanup_group specifically --
// rather than the Go zero value false.
func TestOmittedEnabledInheritsDocumentedDefault(t *testing.T) {
	def := defaultConfig()

	t.Run("ordinary group inherits default enabled=true", func(t *testing.T) {
		if !def.SystemPage["health_group"].Enabled {
			t.Fatal("test setup assumption violated: health_group default is not enabled")
		}

		path := writeConfigFile(t, "system_page:\n  health_group:\n    app_id: com.example.Other\n")
		cfg, err := loadFromPath(path)
		if err != nil {
			t.Fatalf("loadFromPath(%q): %v", path, err)
		}

		got := cfg.SystemPage["health_group"]
		if !got.Enabled {
			t.Errorf("health_group: omitted `enabled` got %v, want true (default)", got.Enabled)
		}
		if got.AppID != "com.example.Other" {
			t.Errorf("health_group: AppID override not applied, got %q", got.AppID)
		}
	})

	t.Run("maintenance_cleanup_group inherits default enabled=false", func(t *testing.T) {
		if def.MaintenancePage["maintenance_cleanup_group"].Enabled {
			t.Fatal("test setup assumption violated: maintenance_cleanup_group default is enabled")
		}

		content := "maintenance_page:\n" +
			"  maintenance_cleanup_group:\n" +
			"    actions:\n" +
			"      - title: Custom\n" +
			"        script: /usr/libexec/custom\n" +
			"        sudo: true\n"
		path := writeConfigFile(t, content)
		cfg, err := loadFromPath(path)
		if err != nil {
			t.Fatalf("loadFromPath(%q): %v", path, err)
		}

		got := cfg.MaintenancePage["maintenance_cleanup_group"]
		if got.Enabled {
			t.Errorf("maintenance_cleanup_group: omitted `enabled` got %v, want false (default, not the Go zero-value coincidence)", got.Enabled)
		}
		wantActions := []ActionConfig{{Title: "Custom", Script: "/usr/libexec/custom", Sudo: true}}
		if !reflect.DeepEqual(got.Actions, wantActions) {
			t.Errorf("maintenance_cleanup_group: Actions override not applied, got %+v, want %+v", got.Actions, wantActions)
		}
	})
}

// TestExplicitEmptySliceOverlayClearsDefault asserts that an explicit empty
// slice (`actions: []`, `bundles_paths: []`) overlays to an empty (len==0)
// slice rather than restoring the default slice.
func TestExplicitEmptySliceOverlayClearsDefault(t *testing.T) {
	t.Run("actions: []", func(t *testing.T) {
		path := writeConfigFile(t, "maintenance_page:\n  maintenance_cleanup_group:\n    actions: []\n")
		cfg, err := loadFromPath(path)
		if err != nil {
			t.Fatalf("loadFromPath(%q): %v", path, err)
		}
		got := cfg.MaintenancePage["maintenance_cleanup_group"].Actions
		if len(got) != 0 {
			t.Errorf("maintenance_cleanup_group.Actions = %+v, want empty slice", got)
		}
	})

	t.Run("bundles_paths: []", func(t *testing.T) {
		path := writeConfigFile(t, "applications_page:\n  brew_bundles_group:\n    bundles_paths: []\n")
		cfg, err := loadFromPath(path)
		if err != nil {
			t.Fatalf("loadFromPath(%q): %v", path, err)
		}
		got := cfg.ApplicationsPage["brew_bundles_group"].BundlesPaths
		if len(got) != 0 {
			t.Errorf("brew_bundles_group.BundlesPaths = %+v, want empty slice", got)
		}
	})
}

// TestNonEmptySliceOverlayReplacesDefaultContents asserts that a non-empty
// actions/bundles_paths list in the file fully replaces the default list
// contents (exact-equal), not a superset/append of the default.
func TestNonEmptySliceOverlayReplacesDefaultContents(t *testing.T) {
	t.Run("actions replaces default list", func(t *testing.T) {
		content := "maintenance_page:\n" +
			"  maintenance_cleanup_group:\n" +
			"    actions:\n" +
			"      - title: Only Action\n" +
			"        script: /usr/libexec/only\n" +
			"        sudo: false\n"
		path := writeConfigFile(t, content)
		cfg, err := loadFromPath(path)
		if err != nil {
			t.Fatalf("loadFromPath(%q): %v", path, err)
		}

		got := cfg.MaintenancePage["maintenance_cleanup_group"].Actions
		want := []ActionConfig{{Title: "Only Action", Script: "/usr/libexec/only", Sudo: false}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("maintenance_cleanup_group.Actions = %+v, want %+v (exact replacement, not appended to default)", got, want)
		}
	})

	t.Run("bundles_paths replaces default list", func(t *testing.T) {
		content := "applications_page:\n" +
			"  brew_bundles_group:\n" +
			"    bundles_paths:\n" +
			"      - /custom/path/one\n" +
			"      - /custom/path/two\n"
		path := writeConfigFile(t, content)
		cfg, err := loadFromPath(path)
		if err != nil {
			t.Fatalf("loadFromPath(%q): %v", path, err)
		}

		got := cfg.ApplicationsPage["brew_bundles_group"].BundlesPaths
		want := []string{"/custom/path/one", "/custom/path/two"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("brew_bundles_group.BundlesPaths = %+v, want %+v (exact replacement, not appended to default)", got, want)
		}
	})
}

// TestIsGroupEnabledMatchesExpectedForEveryGroup calls IsGroupEnabled for
// every (page, group) pair defaultConfig() defines, both for the
// absent-file fallback config and for a partial-file config that overrides
// several groups across different pages, asserting each result matches the
// expected default/override. Looped, not sampled.
func TestIsGroupEnabledMatchesExpectedForEveryGroup(t *testing.T) {
	defPages := pagesOf(defaultConfig())

	t.Run("absent file", func(t *testing.T) {
		withConfigPaths(t, []string{filepath.Join(t.TempDir(), "does-not-exist.yml")})
		cfg := Load()

		for _, page := range pageNames {
			for name, group := range defPages[page] {
				if got := cfg.IsGroupEnabled(page, name); got != group.Enabled {
					t.Errorf("IsGroupEnabled(%q, %q) = %v, want %v (default)", page, name, got, group.Enabled)
				}
			}
		}
	})

	t.Run("partial file overrides several groups", func(t *testing.T) {
		content := "system_page:\n" +
			"  health_group:\n" +
			"    enabled: false\n" +
			"updates_page:\n" +
			"  brew_trust_group:\n" +
			"    enabled: false\n" +
			"maintenance_page:\n" +
			"  maintenance_cleanup_group:\n" +
			"    enabled: true\n" +
			"help_page:\n" +
			"  help_resources_group:\n" +
			"    enabled: false\n"
		path := writeConfigFile(t, content)
		cfg, err := loadFromPath(path)
		if err != nil {
			t.Fatalf("loadFromPath(%q): %v", path, err)
		}

		type pageGroup struct{ page, group string }
		overrides := map[pageGroup]bool{
			{"system_page", "health_group"}:                   false,
			{"updates_page", "brew_trust_group"}:              false,
			{"maintenance_page", "maintenance_cleanup_group"}: true,
			{"help_page", "help_resources_group"}:             false,
		}

		for _, page := range pageNames {
			for name, group := range defPages[page] {
				want := group.Enabled
				if override, ok := overrides[pageGroup{page, name}]; ok {
					want = override
				}
				if got := cfg.IsGroupEnabled(page, name); got != want {
					t.Errorf("IsGroupEnabled(%q, %q) = %v, want %v", page, name, got, want)
				}
			}
		}
	})
}
