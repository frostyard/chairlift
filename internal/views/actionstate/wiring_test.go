package actionstate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestViewsUseTheActionStateDecisions checks, by source, that the Apps and
// Updates domains route every package action through its decision here and
// guard every inventory reload with a RefreshGate generation (AGENTS.md:
// "Homebrew update actions preserve known state", "Flatpak update reloads
// are generation-guarded and preserve known state", "Homebrew application
// actions are typed and refresh-safe"). The outcomes themselves are covered
// by behavior tests in internal/views (flatpak_test.go, brew_test.go).
func TestViewsUseTheActionStateDecisions(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	viewsDir := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(viewsDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(data)
	}

	for file, required := range map[string][]string{
		"flatpak.go": {
			"generation := f.appsRefresh.Begin()",
			"if !f.appsRefresh.IsCurrent(generation)",
			"generation := f.flatpakUpdatesRefresh.Begin()",
			"if !f.flatpakUpdatesRefresh.IsCurrent(generation)",
			"actionstate.OutdatedRefresh(r.status.Authoritative,",
			"actionstate.PackageUpgrade(",
			"actionstate.PackageUninstall(",
		},
		"brew_apps.go": {
			"generation := b.inv.refresh.Begin()",
			"if !b.inv.refresh.IsCurrent(generation)",
			"actionstate.PackagePin(",
			"actionstate.PackageUninstall(",
		},
		"brew_source.go": {
			"generation := b.updates.refresh.Begin()",
			"if !b.updates.refresh.IsCurrent(generation)",
			"generation := b.updates.tapsRefresh.Begin()",
			"if !b.updates.tapsRefresh.IsCurrent(generation)",
			"actionstate.OutdatedRefresh(",
			"actionstate.PackageUpgrade(",
			"actionstate.MetadataUpdate(",
		},
		"brew_search.go": {
			"generation := s.refresh.Begin()",
			"if !s.refresh.IsCurrent(generation)",
			"actionstate.PackageInstall(",
		},
		"brew.go": {
			"gate actionstate.Gate",
			"if !a.gate.TryStart()",
		},
	} {
		source := read(file)
		for _, fragment := range required {
			if !strings.Contains(source, fragment) {
				t.Errorf("%s does not contain %q", file, fragment)
			}
		}
	}

	// Search must carry the result's kind into `brew install [--cask]`.
	search := read("brew_search.go")
	if !strings.Contains(search, "homebrew.Cask") {
		t.Error("brew_search.go does not carry the result kind (homebrew.Cask) into the install")
	}
}
