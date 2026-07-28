package badgestate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestViewsUseSharedBadgeCounts(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	viewsDir := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))

	checks := map[string][]string{
		filepath.Join(viewsDir, "views.go"): {
			`updateCounts badgestate.Counts`,
			`total := uh.updateCounts.Total()`,
		},
		filepath.Join(viewsDir, "updates_page.go"): {
			`uh.updateCounts.Set(badgestate.Bootc, 1)`,
			`uh.updateCounts.Set(badgestate.Bootc, 0)`,
			`uh.updateCounts.Set(badgestate.Flatpak, len(allUpdates))`,
			`uh.updateCounts.Set(badgestate.Homebrew, refresh.Count)`,
			`uh.updateCounts.Add(badgestate.Homebrew, -1)`,
		},
	}
	for path, required := range checks {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, fragment := range required {
			if !strings.Contains(string(source), fragment) {
				t.Errorf("%s does not contain badge-state wiring %q", path, fragment)
			}
		}
	}

	updatesSource, err := os.ReadFile(filepath.Join(viewsDir, "updates_page.go"))
	if err != nil {
		t.Fatalf("read updates_page.go: %v", err)
	}
	for _, removed := range []string{
		"bootcUpdateCount",
		"flatpakUpdateCount",
		"brewUpdateCount",
		"updateCountMu",
	} {
		if strings.Contains(string(updatesSource), removed) {
			t.Errorf("updates_page.go still contains retired badge field %q", removed)
		}
	}
}
