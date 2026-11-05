package config

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// problemsGroups is the exact group set of the Problems page (ADR-0015).
var problemsGroups = []string{
	"problems_apps_group",
	"problems_reporting_group",
	"problems_system_group",
}

// TestProblemsPageDefaultGroupSetIsExact pins the Problems page's groups:
// exactly the three groups the page builder guards, each enabled by default.
func TestProblemsPageDefaultGroupSetIsExact(t *testing.T) {
	cfg := defaultConfig()
	got := make([]string, 0, len(cfg.ProblemsPage))
	for name, group := range cfg.ProblemsPage {
		got = append(got, name)
		if !group.Enabled {
			t.Errorf("%s is disabled by default, want enabled", name)
		}
	}
	sort.Strings(got)
	if len(got) != len(problemsGroups) {
		t.Fatalf("problems_page groups = %v, want %v", got, problemsGroups)
	}
	for i := range got {
		if got[i] != problemsGroups[i] {
			t.Fatalf("problems_page groups = %v, want %v", got, problemsGroups)
		}
	}
	for _, name := range problemsGroups {
		if !cfg.IsGroupEnabled("problems_page", name) {
			t.Errorf("IsGroupEnabled(problems_page, %s) = false, want true", name)
		}
		if cfg.GetGroupConfig("problems_page", name) == nil {
			t.Errorf("GetGroupConfig(problems_page, %s) = nil", name)
		}
	}
}

// TestProblemsGroupsOverlay proves each Problems group can be turned off in
// a config file on its own, and that an unknown group is a schema error.
func TestProblemsGroupsOverlay(t *testing.T) {
	for _, off := range problemsGroups {
		t.Run(off, func(t *testing.T) {
			cfg, err := loadFromPath(writeConfigFile(t, "problems_page:\n  "+off+":\n    enabled: false\n"))
			if err != nil {
				t.Fatalf("loadFromPath: %v", err)
			}
			for _, name := range problemsGroups {
				if got, want := cfg.IsGroupEnabled("problems_page", name), name != off; got != want {
					t.Errorf("IsGroupEnabled(problems_page, %s) = %v, want %v", name, got, want)
				}
			}
		})
	}
	t.Run("unknown group", func(t *testing.T) {
		cfg, err := loadFromPath(writeConfigFile(t, "problems_page:\n  problems_send_group:\n    enabled: true\n"))
		if cfg != nil || err == nil || err.Kind != KindSchema {
			t.Fatalf("loadFromPath = (%v, %v), want nil config and a schema error", cfg, err)
		}
	})
}

// TestConfigErrorDisablesProblemsGroups proves the fail-closed rule
// (ADR-0003) reaches the Problems page: a configuration error turns every
// Problems group off, so the page is omitted.
func TestConfigErrorDisablesProblemsGroups(t *testing.T) {
	path := writeConfigFile(t, "problems_page: [\n")
	withConfigPaths(t, []string{path})
	cfg, loadErr := Load()
	if loadErr == nil {
		t.Fatal("Load() error = nil, want a parse error")
	}
	for _, name := range problemsGroups {
		if cfg.IsGroupEnabled("problems_page", name) {
			t.Errorf("%s is enabled after a configuration error, want disabled", name)
		}
	}
}

// TestShippedConfigsEnableProblemsGroups asserts both shipped configuration
// files keep every Problems group enabled, as the defaults do.
func TestShippedConfigsEnableProblemsGroups(t *testing.T) {
	for _, name := range []string{"config.yml", "config.bootc-example.yml"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(repoRoot(), name)
			cfg, err := loadFromPath(path)
			if err != nil {
				t.Fatalf("loadFromPath(%s): %v", name, err)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			raw, parseErr := parseAndValidate(path, data)
			if parseErr != nil {
				t.Fatalf("parseAndValidate(%s): %v", name, parseErr)
			}
			for _, group := range problemsGroups {
				if !cfg.IsGroupEnabled("problems_page", group) {
					t.Errorf("%s disables %s", name, group)
				}
				if _, ok := raw.ProblemsPage[group]; !ok {
					t.Errorf("%s does not name %s", name, group)
				}
			}
		})
	}
}
