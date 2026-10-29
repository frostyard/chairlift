package featurestatus

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFeaturesPageUsesSummarizeAndRechecks is the wiring half of this package's
// contract: internal/views cannot host a test file, so this reads the page
// builder's source instead and asserts the fragments that make the rows show
// the state after an update rather than the startup check's state.
func TestFeaturesPageUsesSummarizeAndRechecks(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	path := filepath.Join(repoRoot, "internal", "views", "features_page.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(source)

	for _, required := range []string{
		`generation := uh.featureCheck.Begin()`,
		`go uh.checkFeatureUpdates(generation, len(features))`,
		`!uh.featureCheck.IsCurrent(generation)`,
		`featurestatus.Summarize(`,
		`uh.featureBaseSubtitles[`,
		`if decision.Recheck`,
		`uh.recheckFeatureUpdates()`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("features-page wiring does not contain %q", required)
		}
	}

	// The page must ask this package for the whole check at once. Calling
	// Feature per check entry is what left a row's subtitle from an earlier
	// check in place and counted components twice.
	if strings.Contains(text, `featurestatus.Feature(`) {
		t.Error("features-page wiring still calls featurestatus.Feature( per check entry, want featurestatus.Summarize(")
	}
}
