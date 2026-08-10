package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrewBundleGroupConfigControlsRuntimeWiring(t *testing.T) {
	if !defaultConfig().IsGroupEnabled("applications_page", "brew_bundles_group") {
		t.Fatal("default brew_bundles_group is disabled, want enabled")
	}

	path := writeConfigFile(t, "applications_page:\n  brew_bundles_group:\n    enabled: false\n")
	cfg, loadErr := loadFromPath(path)
	if loadErr != nil {
		t.Fatalf("loadFromPath(%q): %v", path, loadErr)
	}
	if cfg.IsGroupEnabled("applications_page", "brew_bundles_group") {
		t.Fatal("explicitly disabled brew_bundles_group remains enabled")
	}

	sourcePath := filepath.Join(repoRoot(), "internal", "views", "applications_page.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	text := string(source)
	for _, required := range []string{
		`if uh.config.IsGroupEnabled("applications_page", "brew_bundles_group") {`,
		`uh.config.GetGroupConfig("applications_page", "brew_bundles_group")`,
		`groupCfg.BundlesPaths`,
		`go uh.loadBrewBundles(bundlePaths)`,
		`homebrew.AvailableBundles(paths)`,
		`bundleview.Present(len(bundles), warning, homebrewAvailable)`,
		`pageview.BrewBundle(bundle.Name, bundle.Description, bundle.Path)`,
		`if !gate.TryStart()`,
		`homebrew.BundleInstall(bundle.Path)`,
		`actionmsg.BundleInstall(homebrew.IsDryRun(), bundle.Name)`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("Applications-page bundle wiring does not contain %q", required)
		}
	}
}
