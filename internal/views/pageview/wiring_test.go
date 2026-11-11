package pageview

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// viewDomains maps each domain of internal/views to its files. Every
// non-test Go file of the package must belong to one domain, so a new file
// cannot escape the checks below.
var viewDomains = map[string][]string{
	"shared":      {"env.go", "source.go", "store.go"},
	"apps":        {"apps.go", "flatpak.go", "brew.go", "brew_apps.go", "brew_bundles.go", "brew_search.go", "brew_source.go"},
	"updates":     {"updates.go", "firmware.go"},
	"overview":    {"overview.go", "system.go", "live.go"},
	"features":    {"features.go", "features_page.go"},
	"problems":    {"problems.go", "problems_reporting.go", "problems_view.go"},
	"maintenance": {"maintenance.go", "maintenance_view.go"},
}

// TestPageBuildersUsePurePresentations checks that the views draw their
// text and decisions from the pure helpers (ADR-0009), and that retired
// presentation logic does not come back inline.
func TestPageBuildersUsePurePresentations(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	viewsDir := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))

	owned := map[string]bool{}
	for _, files := range viewDomains {
		for _, f := range files {
			owned[f] = true
		}
	}
	entries, err := os.ReadDir(viewsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !owned[name] {
			t.Errorf("internal/views/%s belongs to no domain of the wiring test", name)
		}
	}

	read := func(domain string) string {
		var b strings.Builder
		for _, f := range viewDomains[domain] {
			data, err := os.ReadFile(filepath.Join(viewsDir, f))
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			b.Write(data)
		}
		return b.String()
	}

	tests := []struct {
		domain   string
		required []string
		retired  []string
	}{
		{
			domain: "apps",
			required: []string{
				"pageview.BrewBundle(",
				"pageview.SearchResult(",
				"pageview.UntrustedTap(",
				"pageview.FlatpakUpdate(",
				"flatpakstatus.Subtitle(",
				"bundleview.Present(",
				"trustmsg.UpgradeMessage(",
				"actionmsg.TapTrust(",
				"actionmsg.BundleInstall(",
				"actionmsg.Install(",
			},
			retired: []string{
				`fmt.Sprintf("%s — %s", bundle.Description, bundle.Path)`,
				`fmt.Sprintf("%s → %s", update.ApplicationID, update.NewVersion)`,
				`"pkexec"`,
			},
		},
		{
			domain: "updates",
			required: []string{
				"pageview.FirmwareUpdate(",
				"pageview.FirmwareBlockedUpdate(",
				"pageview.FirmwareUpdatesSubtitle(",
				"pageview.FirmwareInstallConfirmation(",
				"pageview.LastChecked(",
				"actionmsg.FirmwareInstall(",
			},
			retired: []string{`"pkexec"`},
		},
		{
			domain:   "maintenance",
			required: []string{"pageview.MaintenanceCommand(", "actionmsg.MaintenanceScript(", "actionmsg.Cleanup("},
			retired: []string{
				`exec.CommandContext(ctx, "pkexec", script)`,
				"exec.CommandContext(ctx, script)",
			},
		},
		{
			domain: "features",
			required: []string{
				"pageview.Feature(",
				"pageview.FeatureGroupDescription(",
				"featurestatus.Summarize(",
				"actionmsg.FeatureToggle(",
				"actionmsg.FeatureUpdate(",
			},
			retired: []string{`fmt.Sprintf("%d features available", len(features))`},
		},
		{
			domain: "problems",
			required: []string{
				"pageview.ProblemDetails(",
				"pageview.ProblemsSystemStatus(",
				"pageview.ProblemsAppsStatus(",
				"pageview.ReportingDescription(",
				"pageview.ReportingWhat(",
				"pageview.ReportingCountRow(",
				"pageview.SentRows(",
				"pageview.ReportOffered(",
				"pageview.ReportSummary(",
				"actionmsg.ProblemDismiss(",
				"actionmsg.Report(",
				"actionmsg.ReportingConsent(",
				"problems.DetailsText(",
				"problems.BuiltMarker(",
			},
			retired: []string{
				`"System checks are not available on this system"`,
				"problems.Title(",
				"problems.Hint(",
				"exec.Command",
				// The phase-1 promise that nothing is ever sent must not come
				// back: reporting is opt-in now.
				"pageview.ProblemsReporting(",
				"ChairLift does not send anything",
			},
		},
		{
			domain: "overview",
			required: []string{
				"pageview.ParseOSRelease(",
				"pageview.ShortDigest(",
			},
			retired: []string{"cases.Title(", "digest[:19]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			text := read(tt.domain)
			for _, fragment := range tt.required {
				if !strings.Contains(text, fragment) {
					t.Errorf("%s does not use %q", tt.domain, fragment)
				}
			}
			for _, fragment := range tt.retired {
				if strings.Contains(text, fragment) {
					t.Errorf("%s still owns presentation logic %q", tt.domain, fragment)
				}
			}
		})
	}
}
