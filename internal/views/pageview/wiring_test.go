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
				"pageview.SysupdateUpdateSubtitle(",
				"pageview.SysupdateStageResultSubtitle(",
				"pageview.SysupdateRollbackSubtitle(",
				"pageview.FirmwareUpdate(",
				"pageview.FirmwareBlockedUpdate(",
				"pageview.FirmwareUpdatesSubtitle(",
				"pageview.FirmwareInstallConfirmation(",
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
		{
			file: "problems_page.go",
			required: []string{
				"pageview.Problem(",
				"pageview.ProblemDetails(",
				"pageview.ProblemsSystemStatus(",
				"pageview.ProblemsAppsStatus(",
				"pageview.ProblemsReporting(",
				"glib.MarkupEscapeText(text.Title, -1)",
				"glib.MarkupEscapeText(text.Subtitle, -1)",
				"actionmsg.ProblemDismiss(IsDryRun(), item.ID, text.Title)",
				"problems.DetailsText(item)",
				"problems.BuiltMarker(",
				"sgtk.RunOnMainThread(",
			},
			retired: []string{
				`"System checks are not available on this system"`,
				`"No problems found"`,
				"problems.Title(",
				"problems.Hint(",
				"exec.Command",
			},
		},
	}

	// Every page builder file must be in the table above.
	pageFiles, err := filepath.Glob(filepath.Join(viewsDir, "*_page.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pageFiles) != 7 || len(pageFiles) != len(tests) {
		t.Fatalf("found %d page builders, the table covers %d; want 7 each", len(pageFiles), len(tests))
	}
	covered := map[string]bool{}
	for _, tt := range tests {
		covered[tt.file] = true
	}
	for _, path := range pageFiles {
		if !covered[filepath.Base(path)] {
			t.Errorf("page builder %s is not covered by the wiring test", filepath.Base(path))
		}
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
