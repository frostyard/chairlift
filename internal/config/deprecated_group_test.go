package config

import (
	"testing"

	"github.com/frostyard/chairlift/internal/navigation"
)

// deprecatedOptimizationGroup was the "System Optimization" placeholder of
// the Maintenance page. It renders nothing now, but existing configuration
// files name it, and an unknown group is a schema error that disables every
// group. These tests keep the key valid and keep it from holding the
// Maintenance page open on its own.
const deprecatedOptimizationGroup = "maintenance_optimization_group"

func TestDeprecatedOptimizationGroupStillLoads(t *testing.T) {
	path := writeConfigFile(t, "maintenance_page:\n"+
		"  maintenance_brew_group:\n    enabled: true\n"+
		"  "+deprecatedOptimizationGroup+":\n    enabled: true\n")

	cfg, loadErr := loadFromPath(path)
	if loadErr != nil {
		t.Fatalf("loadFromPath() error = %v, want nil for a file that names %s", loadErr, deprecatedOptimizationGroup)
	}
	if !cfg.IsGroupEnabled("maintenance_page", "maintenance_brew_group") {
		t.Fatal("maintenance_brew_group disabled: the deprecated key made the configuration fail closed")
	}
	if !cfg.IsGroupEnabled("system_page", "system_info_group") {
		t.Fatal("system_info_group disabled: the deprecated key made the configuration fail closed")
	}
}

func TestDeprecatedOptimizationGroupDoesNotKeepMaintenanceVisible(t *testing.T) {
	path := writeConfigFile(t, "maintenance_page:\n"+
		"  maintenance_cleanup_group:\n    enabled: false\n"+
		"  maintenance_brew_group:\n    enabled: false\n"+
		"  maintenance_flatpak_group:\n    enabled: false\n"+
		"  "+deprecatedOptimizationGroup+":\n    enabled: true\n")

	cfg, loadErr := loadFromPath(path)
	if loadErr != nil {
		t.Fatalf("loadFromPath() error = %v, want nil", loadErr)
	}

	visible := navigation.VisibleItems(cfg.IsGroupEnabled)
	for _, item := range visible {
		if item.Name == "maintenance" {
			t.Fatalf("Maintenance page visible with only %s enabled; it renders nothing", deprecatedOptimizationGroup)
		}
	}
	if len(visible) == 0 || visible[0].Name != "overview" {
		t.Fatalf("visible pages = %+v, want the other pages starting with the Overview", visible)
	}
}
