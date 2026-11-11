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

	var text strings.Builder
	for _, name := range []string{"brew.go", "brew_bundles.go"} {
		source, err := os.ReadFile(filepath.Join(repoRoot(), "internal", "views", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text.Write(source)
	}
	for _, required := range []string{
		`groupBrewBundles = "brew_bundles_group"`,
		`g.BundlesPaths`,
		`bundles:       homebrew.AvailableBundles,`,
		`bundleInstall: homebrew.BundleInstall,`,
		`bundleview.Present(len(r.bundles), r.warning, r.available)`,
		`pageview.BrewBundle(item.Name, item.Description, item.Path)`,
		`bd.gate.TryStart()`,
		`actionmsg.BundleInstall(dryRun, name)`,
	} {
		if !strings.Contains(text.String(), required) {
			t.Errorf("Homebrew bundle wiring does not contain %q", required)
		}
	}
}
