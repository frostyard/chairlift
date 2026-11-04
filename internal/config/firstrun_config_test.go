package config

import (
	"path/filepath"
	"testing"
)

// TestFirstRunAppsGroupDefault pins the first-run page's only group: enabled,
// with the Snow images' application list as its apps_path (ADR-0014).
func TestFirstRunAppsGroupDefault(t *testing.T) {
	cfg := defaultConfig()
	if len(cfg.FirstRunPage) != 1 {
		t.Fatalf("defaultConfig().FirstRunPage has %d groups, want exactly 1", len(cfg.FirstRunPage))
	}
	group := cfg.GetGroupConfig("first_run_page", "first_run_apps_group")
	if group == nil {
		t.Fatal("GetGroupConfig(first_run_page, first_run_apps_group) = nil")
	}
	if !group.Enabled {
		t.Error("first_run_apps_group is disabled by default, want enabled")
	}
	if defaultFirstRunAppsPath != "/usr/share/snow/first-run/apps.json" {
		t.Errorf("defaultFirstRunAppsPath = %q, want /usr/share/snow/first-run/apps.json", defaultFirstRunAppsPath)
	}
	if group.AppsPath != defaultFirstRunAppsPath {
		t.Errorf("first_run_apps_group.AppsPath = %q, want %q", group.AppsPath, defaultFirstRunAppsPath)
	}
	if !cfg.IsGroupEnabled("first_run_page", "first_run_apps_group") {
		t.Error("IsGroupEnabled(first_run_page, first_run_apps_group) = false, want true")
	}
}

// TestFirstRunAppsPathOverlay covers the overlay rule for apps_path: a file
// value replaces the default, an explicit empty value clears it, and an
// omitted value inherits it.
func TestFirstRunAppsPathOverlay(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantPath    string
		wantEnabled bool
	}{
		{"replaced", "first_run_page:\n  first_run_apps_group:\n    apps_path: /etc/snow/apps.json\n", "/etc/snow/apps.json", true},
		{"explicit empty", "first_run_page:\n  first_run_apps_group:\n    apps_path: \"\"\n", "", true},
		{"omitted inherits", "first_run_page:\n  first_run_apps_group:\n    enabled: false\n", defaultFirstRunAppsPath, false},
		{"other page only", "help_page:\n  help_resources_group:\n    enabled: true\n", defaultFirstRunAppsPath, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadFromPath(writeConfigFile(t, tt.content))
			if err != nil {
				t.Fatalf("loadFromPath: %v", err)
			}
			group := cfg.GetGroupConfig("first_run_page", "first_run_apps_group")
			if group == nil {
				t.Fatal("first_run_apps_group missing")
			}
			if group.AppsPath != tt.wantPath {
				t.Errorf("AppsPath = %q, want %q", group.AppsPath, tt.wantPath)
			}
			if got := cfg.IsGroupEnabled("first_run_page", "first_run_apps_group"); got != tt.wantEnabled {
				t.Errorf("IsGroupEnabled = %v, want %v", got, tt.wantEnabled)
			}
		})
	}
}

// TestFirstRunAppsPathRejectsWrongType proves apps_path takes part in the
// strict schema: a non-string value is a parse/type error, and an unknown
// group on the first-run page is a schema error.
func TestFirstRunAppsPathRejectsWrongType(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    ErrorKind
	}{
		{"sequence value", "first_run_page:\n  first_run_apps_group:\n    apps_path: [a, b]\n", KindParseType},
		{"unknown group", "first_run_page:\n  first_run_style_group:\n    enabled: true\n", KindSchema},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadFromPath(writeConfigFile(t, tt.content))
			if cfg != nil || err == nil || err.Kind != tt.want {
				t.Fatalf("loadFromPath = (%v, %v), want nil config and %q error", cfg, err, tt.want)
			}
		})
	}
}

// TestConfigErrorDisablesFirstRunApps proves the fail-closed rule (ADR-0003)
// reaches the first run: a configuration error hides the Apps step.
func TestConfigErrorDisablesFirstRunApps(t *testing.T) {
	path := writeConfigFile(t, "first_run_page: [\n")
	withConfigPaths(t, []string{path})
	cfg, loadErr := Load()
	if loadErr == nil {
		t.Fatal("Load() error = nil, want a parse error")
	}
	if cfg.IsGroupEnabled("first_run_page", "first_run_apps_group") {
		t.Error("first_run_apps_group is enabled after a configuration error, want disabled")
	}
	if group := cfg.GetGroupConfig("first_run_page", "first_run_apps_group"); group == nil || group.AppsPath != defaultFirstRunAppsPath {
		t.Errorf("first_run_apps_group after error = %+v, want the default apps_path kept", group)
	}
}

// TestShippedConfigsMatchFirstRunDefaults asserts both shipped configuration
// files keep first_run_apps_group enabled with the default list path, so a
// host with the packaged file and a host with no file agree.
func TestShippedConfigsMatchFirstRunDefaults(t *testing.T) {
	for _, name := range []string{"config.yml", "config.bootc-example.yml"} {
		t.Run(name, func(t *testing.T) {
			cfg, err := loadFromPath(filepath.Join(repoRoot(), name))
			if err != nil {
				t.Fatalf("loadFromPath(%s): %v", name, err)
			}
			if !cfg.IsGroupEnabled("first_run_page", "first_run_apps_group") {
				t.Errorf("%s disables first_run_apps_group", name)
			}
			if got := cfg.GetGroupConfig("first_run_page", "first_run_apps_group").AppsPath; got != defaultFirstRunAppsPath {
				t.Errorf("%s apps_path = %q, want %q", name, got, defaultFirstRunAppsPath)
			}
		})
	}
}
