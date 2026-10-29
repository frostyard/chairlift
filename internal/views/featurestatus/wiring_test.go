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

	// Order and scope inside checkFeatureUpdates: the generation guard must
	// come before BOTH publication branches (the check-failed description and
	// the row subtitles), and the base-subtitle reset must be the real
	// assignment inside this function, not only the map's initialization.
	body := functionBody(t, text, "func (uh *UserHome) checkFeatureUpdates(")
	guard := indexOrFail(t, body, `if !uh.featureCheck.IsCurrent(generation) {`)
	for _, publication := range []string{
		`featurestatus.GroupDescriptionCheckFailed(totalFeatures)`,
		`featurestatus.Summarize(names, checks)`,
		`row.SetSubtitle(status.Subtitle)`,
		`row.SetSubtitle(uh.featureBaseSubtitles[name])`,
		`featurestatus.GroupDescription(totalFeatures, summary.Updates, summary.Waiting)`,
	} {
		if at := indexOrFail(t, body, publication); at < guard {
			t.Errorf("checkFeatureUpdates publishes %q before the generation guard", publication)
		}
	}

	// The re-check after an update must sit in the update handler, after its
	// toast, and only behind the decision's Recheck.
	update := functionBody(t, text, "func (uh *UserHome) onUpdateFeaturesClicked(")
	recheck := indexOrFail(t, update, "if decision.Recheck {\n\t\t\t\tuh.recheckFeatureUpdates()")
	if toast := indexOrFail(t, update, `uh.toastAdder.ShowToast(decision.Toast)`); recheck < toast {
		t.Error("onUpdateFeaturesClicked re-checks before it shows the toast")
	}

	// loadFeatures stores the base subtitle of every row it builds.
	load := functionBody(t, text, "func (uh *UserHome) loadFeatures(")
	indexOrFail(t, load, `uh.featureBaseSubtitles[feat.Name] = presentation.Subtitle`)
}

// functionBody returns the source of the top-level function that starts with
// signature, up to the next top-level func declaration or the end of the file.
func functionBody(t *testing.T, text, signature string) string {
	t.Helper()
	start := strings.Index(text, signature)
	if start < 0 {
		t.Fatalf("features_page.go has no %q", signature)
	}
	rest := text[start:]
	if end := strings.Index(rest[len(signature):], "\nfunc "); end >= 0 {
		return rest[:len(signature)+end]
	}
	return rest
}

// indexOrFail returns the position of fragment in body, and fails the test
// when body does not contain it.
func indexOrFail(t *testing.T, body, fragment string) int {
	t.Helper()
	at := strings.Index(body, fragment)
	if at < 0 {
		t.Fatalf("function body does not contain %q", fragment)
	}
	return at
}
