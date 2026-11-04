package actionstate

import (
	"os"
	"path/filepath"
	"regexp"
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

// TestUpdatesPageRetiresCompletedFlatpakUpdateRows pins the per-app Flatpak
// update button to the Homebrew upgrade discipline. The button must not wait
// for the reload to replace it: a reload in which both installation queries
// fail keeps the old rows, so a button left at "Updating..." would stay there.
// A live success removes the row and decrements the Flatpak count at once; a
// failure or a dry-run preview restores the button; only a live success
// reloads.
func TestUpdatesPageRetiresCompletedFlatpakUpdateRows(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	viewsDir := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
	source, err := os.ReadFile(filepath.Join(viewsDir, "updates_page.go"))
	if err != nil {
		t.Fatalf("read updates_page.go: %v", err)
	}
	updates := string(source)

	start := strings.Index(updates, `isUser := update.Installation == "user"`)
	end := strings.Index(updates, `updateBtn.ConnectClicked(&clickedCb)`)
	if start < 0 || end < start {
		t.Fatal("updates_page.go has no per-app Flatpak update button handler")
	}
	handler := updates[start:end]

	for _, required := range []string{
		`if !flatpakUpdateGate.TryStart()`,
		`btn.SetLabel("Updating...")`,
		`dryRun := flatpak.IsDryRun()`,
		`decision := actionstate.PackageUpgrade(err == nil, dryRun)`,
		`actionmsg.Update(dryRun, appID)`,
		`flatpakUpdateGate.Complete()`,
		`uh.flatpakUpdateRows.Remove(row, func(row *adw.ActionRow)`,
		`uh.updateCounts.Add(badgestate.Flatpak, -1)`,
		`uh.updateBadgeCount()`,
		`flatpakUpdateGate.Reset()`,
		`btn.SetLabel("Update")`,
	} {
		if !strings.Contains(handler, required) {
			t.Errorf("Flatpak update button handler does not contain %q", required)
		}
	}
	if got := strings.Count(handler, `if decision.RestoreControl {`); got != 2 {
		t.Errorf("RestoreControl branches = %d, want 2 (the failure path and the success/dry-run path)", got)
	}
	if !regexp.MustCompile(`if decision\.Refresh \{\s*uh\.loadFlatpakUpdates\(\)`).MatchString(handler) {
		t.Error("the Flatpak update button must reload only under decision.Refresh")
	}
	if strings.Count(handler, `uh.loadFlatpakUpdates()`) != 1 {
		t.Error("the Flatpak update button must reload exactly once, under decision.Refresh")
	}
	if !strings.Contains(updates, `uh.flatpakUpdateRows.Add(row)`) {
		t.Error("Flatpak update rows are not tracked with rowset, so a completed row cannot be removed")
	}
}
