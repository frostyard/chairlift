package config

import (
	"testing"
)

func TestLoadFromPath_ValidConfig(t *testing.T) {
	cfg, err := loadFromPath("testdata/valid.yml")
	if err != nil {
		t.Fatalf("loadFromPath failed: %v", err)
	}

	// Verify system_page structure
	group := cfg.SystemPage["system_info_group"]
	if !group.Enabled {
		t.Error("system_info_group should be enabled")
	}

	// Verify nbc_status_group
	nbcGroup := cfg.SystemPage["nbc_status_group"]
	if !nbcGroup.Enabled {
		t.Error("nbc_status_group should be enabled")
	}

	// Verify health_group has app_id
	healthGroup := cfg.SystemPage["health_group"]
	if healthGroup.AppID != "io.missioncenter.MissionCenter" {
		t.Errorf("health_group AppID = %q, want %q", healthGroup.AppID, "io.missioncenter.MissionCenter")
	}
}

func TestLoadFromPath_MinimalConfig(t *testing.T) {
	cfg, err := loadFromPath("testdata/minimal.yml")
	if err != nil {
		t.Fatalf("loadFromPath failed: %v", err)
	}

	group := cfg.SystemPage["system_info_group"]
	if group.Enabled {
		t.Error("system_info_group should be disabled")
	}
}

func TestLoadFromPath_NonExistent(t *testing.T) {
	_, err := loadFromPath("testdata/nonexistent.yml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadFromPath_HelpPageConfig(t *testing.T) {
	cfg, err := loadFromPath("testdata/valid.yml")
	if err != nil {
		t.Fatalf("loadFromPath failed: %v", err)
	}

	group := cfg.HelpPage["help_resources_group"]
	if !group.Enabled {
		t.Error("help_resources_group should be enabled")
	}
	if group.Website != "https://example.com" {
		t.Errorf("Website = %q, want %q", group.Website, "https://example.com")
	}
	if group.Issues != "https://github.com/example/issues" {
		t.Errorf("Issues = %q, want %q", group.Issues, "https://github.com/example/issues")
	}
	if group.Chat != "https://discord.gg/example" {
		t.Errorf("Chat = %q, want %q", group.Chat, "https://discord.gg/example")
	}
}

func TestLoadFromPath_MaintenanceActions(t *testing.T) {
	cfg, err := loadFromPath("testdata/valid.yml")
	if err != nil {
		t.Fatalf("loadFromPath failed: %v", err)
	}

	group := cfg.MaintenancePage["maintenance_cleanup_group"]
	if len(group.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(group.Actions))
	}

	// First action
	if group.Actions[0].Title != "Clean Up Temp Files" {
		t.Errorf("first action Title = %q, want %q", group.Actions[0].Title, "Clean Up Temp Files")
	}
	if group.Actions[0].Sudo {
		t.Error("first action should not require sudo")
	}

	// Second action
	if group.Actions[1].Title != "System Maintenance" {
		t.Errorf("second action Title = %q, want %q", group.Actions[1].Title, "System Maintenance")
	}
	if !group.Actions[1].Sudo {
		t.Error("second action should require sudo")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	// Verify system_page defaults
	if !cfg.SystemPage["system_info_group"].Enabled {
		t.Error("system_info_group should be enabled by default")
	}
	if !cfg.SystemPage["nbc_status_group"].Enabled {
		t.Error("nbc_status_group should be enabled by default")
	}

	// Verify help_page defaults
	helpGroup := cfg.HelpPage["help_resources_group"]
	if !helpGroup.Enabled {
		t.Error("help_resources_group should be enabled by default")
	}
	if helpGroup.Website == "" {
		t.Error("help_resources_group should have default website")
	}
}

func TestIsGroupEnabled_EnabledGroup(t *testing.T) {
	cfg, err := loadFromPath("testdata/valid.yml")
	if err != nil {
		t.Fatalf("loadFromPath failed: %v", err)
	}

	if !cfg.IsGroupEnabled("system_page", "system_info_group") {
		t.Error("system_info_group should be enabled")
	}
}

func TestIsGroupEnabled_DisabledGroup(t *testing.T) {
	cfg, err := loadFromPath("testdata/minimal.yml")
	if err != nil {
		t.Fatalf("loadFromPath failed: %v", err)
	}

	if cfg.IsGroupEnabled("system_page", "system_info_group") {
		t.Error("system_info_group should be disabled")
	}
}

func TestIsGroupEnabled_UnknownPage(t *testing.T) {
	cfg := defaultConfig()

	// Unknown pages should default to enabled
	if !cfg.IsGroupEnabled("unknown_page", "some_group") {
		t.Error("unknown pages should default to enabled")
	}
}

func TestIsGroupEnabled_UnknownGroup(t *testing.T) {
	cfg := defaultConfig()

	// Unknown groups should default to enabled
	if !cfg.IsGroupEnabled("system_page", "nonexistent_group") {
		t.Error("unknown groups should default to enabled")
	}
}

func TestGetGroupConfig_ReturnsConfig(t *testing.T) {
	cfg, err := loadFromPath("testdata/valid.yml")
	if err != nil {
		t.Fatalf("loadFromPath failed: %v", err)
	}

	group := cfg.GetGroupConfig("help_page", "help_resources_group")
	if group == nil {
		t.Fatal("expected non-nil group config")
	}
	if group.Website != "https://example.com" {
		t.Errorf("Website = %q, want %q", group.Website, "https://example.com")
	}
}

func TestGetGroupConfig_ReturnsNilForUnknownGroup(t *testing.T) {
	cfg := defaultConfig()

	group := cfg.GetGroupConfig("help_page", "nonexistent_group")
	if group != nil {
		t.Error("expected nil for nonexistent group")
	}
}

func TestGetGroupConfig_ReturnsNilForUnknownPage(t *testing.T) {
	cfg := defaultConfig()

	group := cfg.GetGroupConfig("unknown_page", "some_group")
	if group != nil {
		t.Error("expected nil for unknown page")
	}
}

func TestGetGroupConfig_AllPages(t *testing.T) {
	cfg := defaultConfig()

	// Test all valid page names
	pageTests := []struct {
		pageName  string
		groupName string
	}{
		{"system_page", "system_info_group"},
		{"updates_page", "nbc_updates_group"},
		{"applications_page", "flatpak_user_group"},
		{"maintenance_page", "maintenance_brew_group"},
		{"extensions_page", "installed_group"},
		{"help_page", "help_resources_group"},
	}

	for _, tc := range pageTests {
		group := cfg.GetGroupConfig(tc.pageName, tc.groupName)
		if group == nil {
			t.Errorf("GetGroupConfig(%q, %q) returned nil, expected config", tc.pageName, tc.groupName)
		}
	}
}

func TestConfigStructure_PageConfigIsMap(t *testing.T) {
	cfg := defaultConfig()

	// Verify PageConfig is a map type
	if cfg.SystemPage == nil {
		t.Error("SystemPage should not be nil")
	}

	// Verify we can iterate over it
	count := 0
	for range cfg.SystemPage {
		count++
	}
	if count == 0 {
		t.Error("SystemPage should have entries")
	}
}

func TestGroupConfig_OptionalFields(t *testing.T) {
	// Test that optional fields work correctly
	group := GroupConfig{
		Enabled: true,
	}

	// Optional fields should be zero values
	if group.AppID != "" {
		t.Error("AppID should be empty by default")
	}
	if group.Website != "" {
		t.Error("Website should be empty by default")
	}
	if len(group.Actions) != 0 {
		t.Error("Actions should be empty by default")
	}
	if len(group.BundlesPaths) != 0 {
		t.Error("BundlesPaths should be empty by default")
	}
}
