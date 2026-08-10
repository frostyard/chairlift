package pageview

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPageBuildersUsePurePresentations(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	viewsDir := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))

	tests := []struct {
		file     string
		required []string
		retired  []string
	}{
		{
			file: "applications_page.go",
			required: []string{
				"pageview.BrewBundle(",
				"pageview.HomebrewPackage(",
				"pageview.FlatpakApplication(",
				"pageview.SearchResult(",
			},
			retired: []string{
				`fmt.Sprintf("%s — %s", bundle.Description, bundle.Path)`,
				`fmt.Sprintf("%s (%s)", app.ApplicationID, app.Version)`,
				`row.SetSubtitle(result.Kind.DisplayName())`,
			},
		},
		{
			file: "updates_page.go",
			required: []string{
				"pageview.UntrustedTap(",
				"pageview.FlatpakUpdate(",
				"pageview.BootcUpdateSubtitle(",
				"pageview.BootcStageResultSubtitle(",
			},
			retired: []string{
				"strings.LastIndex(",
				`fmt.Sprintf("%s → %s", update.ApplicationID, update.NewVersion)`,
				`fmt.Sprintf("Update %s staged — restart to apply", version)`,
			},
		},
		{
			file:     "maintenance_page.go",
			required: []string{"pageview.MaintenanceCommand("},
			retired: []string{
				`exec.CommandContext(ctx, "pkexec", script)`,
				"exec.CommandContext(ctx, script)",
			},
		},
		{
			file: "features_page.go",
			required: []string{
				"pageview.Feature(",
				"pageview.FeatureGroupDescription(",
			},
			retired: []string{
				"row.SetTitle(feat.Description)",
				`fmt.Sprintf("%d features available", len(features))`,
			},
		},
		{
			file:     "help_page.go",
			required: []string{"pageview.HelpResources("},
			retired:  []string{`row.SetTitle("Website")`, `row.SetTitle("Report Issues")`},
		},
		{
			file: "system_page.go",
			required: []string{
				"pageview.ParseOSRelease(",
				"pageview.ShortDigest(",
			},
			retired: []string{"bufio.NewScanner(", "cases.Title(", "digest[:19]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			path := filepath.Join(viewsDir, tt.file)
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			text := string(source)
			for _, fragment := range tt.required {
				if !strings.Contains(text, fragment) {
					t.Errorf("%s does not use %q", tt.file, fragment)
				}
			}
			for _, fragment := range tt.retired {
				if strings.Contains(text, fragment) {
					t.Errorf("%s still owns presentation logic %q", tt.file, fragment)
				}
			}
		})
	}
}
