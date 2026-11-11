package featurestatus

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFeaturesPageUsesSummarizeAndRechecks is the wiring half of this
// package's contract: it reads the Features domain's source
// (internal/views/features.go) and asserts the fragments that make the rows
// show the state after an update rather than the startup check's state.
// internal/views has behavior tests of its own (features_test.go); this
// keeps the ordering guarantees that a behavior test cannot see.
func TestFeaturesPageUsesSummarizeAndRechecks(t *testing.T) {
	text := readViews(t, "features.go")

	for _, required := range []string{
		`generation := f.featureCheck.Begin()`,
		`f.checkFeatureUpdates(generation, r.checks, r.err)`,
		`!f.featureCheck.IsCurrent(generation)`,
		`featurestatus.Summarize(`,
		`f.baseSubtitles[`,
		`if decision.Recheck`,
		`f.recheckFeatureUpdates()`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("features wiring does not contain %q", required)
		}
	}

	// The page must ask this package for the whole check at once. Calling
	// Feature per check entry is what left a row's subtitle from an earlier
	// check in place and counted components twice.
	for _, name := range []string{"features.go", "features_page.go"} {
		if strings.Contains(readViews(t, name), `featurestatus.Feature(`) {
			t.Errorf("%s calls featurestatus.Feature( per check entry, want featurestatus.Summarize(", name)
		}
	}

	// Order inside checkFeatureUpdates: the generation guard must come
	// before every publication (the failed flag the description reads, and
	// the summary the rows read).
	body := functionBody(t, text, "func (f *Features) checkFeatureUpdates(")
	guard := indexOrFail(t, body, `if !f.featureCheck.IsCurrent(generation) {`)
	for _, publication := range []string{
		`f.checking = false`,
		`f.checkFailed = true`,
		`f.summary = featurestatus.Summarize(names, checks)`,
		`f.checks = make(`,
	} {
		if at := indexOrFail(t, body, publication); at < guard {
			t.Errorf("checkFeatureUpdates publishes %q before the generation guard", publication)
		}
	}

	// A row the check did not answer for reads its base subtitle: the
	// summary replaces the last one whole, and subtitle falls back.
	subtitle := functionBody(t, text, "func (f *Features) subtitle(")
	rows := indexOrFail(t, subtitle, `if status, ok := f.summary.Rows[name]; ok {`)
	if base := indexOrFail(t, subtitle, `return f.baseSubtitles[name]`); base < rows {
		t.Error("subtitle reads the base subtitle before the check's rows")
	}

	// The description comes from this package for every outcome.
	description := functionBody(t, text, "func (f *Features) groupDescription(")
	indexOrFail(t, description, `featurestatus.GroupDescriptionCheckFailed(total)`)
	indexOrFail(t, description, `featurestatus.GroupDescription(total, f.summary.Updates, f.summary.Waiting)`)

	// The re-check after an update must sit in the update's completion,
	// after its toast, and only behind the decision's Recheck.
	update := functionBody(t, text, "func (f *Features) updated(")
	recheck := indexOrFail(t, update, "if decision.Recheck {\n\t\tf.recheckFeatureUpdates()")
	if toast := indexOrFail(t, update, `f.env.Toast(decision.Toast)`); recheck < toast {
		t.Error("updated re-checks before it shows the toast")
	}

	// A confirmed toggle lists and checks again; a dry run returns first.
	toggled := functionBody(t, text, "func (f *Features) toggled(")
	confirm := indexOrFail(t, toggled, "if !decision.Confirm {\n\t\t// Nothing changed under dry run")
	if reload := indexOrFail(t, toggled, "f.load()"); reload < confirm {
		t.Error("toggled lists again before it checks decision.Confirm")
	}

	// load stores the base subtitle of every row it builds and then checks.
	load := functionBody(t, text, "func (f *Features) load(")
	base := indexOrFail(t, load, `f.baseSubtitles[feat.Name] = baseSubtitle(row)`)
	if check := indexOrFail(t, load, `f.recheckFeatureUpdates()`); check < base {
		t.Error("load checks before it stores the base subtitles")
	}
}

// readViews returns the source of one file of internal/views.
func readViews(t *testing.T, name string) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	path := filepath.Join(repoRoot, "internal", "views", name)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(source)
}

// functionBody returns the source of the top-level function that starts with
// signature, up to the next top-level func declaration or the end of the file.
func functionBody(t *testing.T, text, signature string) string {
	t.Helper()
	start := strings.Index(text, signature)
	if start < 0 {
		t.Fatalf("features.go has no %q", signature)
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
