package actionstate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpdatesPageUsesGuardedRefreshDecisions(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	path := filepath.Join(repoRoot, "internal", "views", "updates_page.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(source)

	for _, required := range []string{
		`if !updateGate.TryStart()`,
		`btn.SetLabel("Updating...")`,
		`go uh.updateHomebrew(btn, updateGate)`,
		`if !upgradeGate.TryStart()`,
		`btn.SetLabel("Upgrading...")`,
		`dryRun := homebrew.IsDryRun()`,
		`actionstate.PackageUpgrade(err == nil, dryRun)`,
		`actionstate.OutdatedRefresh(err == nil, currentCount, len(packages))`,
		`actionstate.OutdatedPresentation(refresh.Count)`,
		`uh.outdatedRows.Remove(row`,
		`uh.updateCounts.Add(badgestate.Homebrew, -1)`,
		`actionstate.OutdatedPresentation(remaining)`,
		`uh.loadOutdatedPackages()`,
		`actionstate.MetadataUpdate(err == nil, dryRun)`,
		`uh.loadOutdatedPackagesWithDone(func(bool)`,
		`generation := uh.brewRefresh.Begin()`,
		`go uh.loadOutdatedPackagesGeneration(generation, done)`,
		`!uh.brewRefresh.IsCurrent(generation)`,
		`button.SetLabel("Update")`,
		`uh.outdatedRows.Clear(func(row *adw.ActionRow)`,
		`uh.outdatedRows.Add(row)`,
		`Error refreshing updates:`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("updates-page wiring does not contain %q", required)
		}
	}
}

// TestUpdatesPageGuardsFlatpakUpdateRefreshes pins the Flatpak update
// inventory reload to the same discipline as the Homebrew one: every reload
// takes a RefreshGate generation, only the current generation publishes, and a
// load in which both installation queries failed keeps the last known rows and
// count through OutdatedRefresh instead of publishing an empty inventory.
func TestUpdatesPageGuardsFlatpakUpdateRefreshes(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	viewsDir := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
	read := func(name string) string {
		t.Helper()
		source, err := os.ReadFile(filepath.Join(viewsDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(source)
	}
	updates := read("updates_page.go")

	for _, required := range []string{
		`generation := uh.flatpakUpdatesRefresh.Begin()`,
		`go uh.loadFlatpakUpdatesGeneration(generation)`,
		`!uh.flatpakUpdatesRefresh.IsCurrent(generation)`,
		`status.Authoritative,`,
		`uh.updateCounts.Get(badgestate.Flatpak),`,
		`uh.updateCounts.Set(badgestate.Flatpak, refresh.Count)`,
		`if !refresh.ReplaceRows {`,
	} {
		if !strings.Contains(updates, required) {
			t.Errorf("updates-page Flatpak refresh wiring does not contain %q", required)
		}
	}
	for _, forbidden := range []string{
		`go uh.loadFlatpakUpdates()`,
		`uh.updateCounts.Set(badgestate.Flatpak, len(allUpdates))`,
	} {
		if strings.Contains(updates, forbidden) {
			t.Errorf("updates_page.go still contains unguarded Flatpak refresh %q", forbidden)
		}
	}
	if got := strings.Count(updates, `!uh.flatpakUpdatesRefresh.IsCurrent(generation)`); got < 2 {
		t.Errorf("flatpakUpdatesRefresh.IsCurrent guards = %d, want the not-installed and the query publication paths both guarded", got)
	}
	if !strings.Contains(read("views.go"), `flatpakUpdatesRefresh actionstate.RefreshGate`) {
		t.Error("views.go does not declare flatpakUpdatesRefresh actionstate.RefreshGate")
	}
}
