package actionstate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestApplicationsPageWiresTypedSearchInstallState(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate applications_wiring_test.go")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	path := filepath.Join(repoRoot, "internal", "views", "applications_page.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(source)

	for _, required := range []string{
		`Search for and install Homebrew formulae and casks`,
		`generation := uh.brewPackagesRefresh.Begin()`,
		`if !uh.brewPackagesRefresh.IsCurrent(generation)`,
		`generation := uh.searchRefresh.Begin()`,
		`if !uh.searchRefresh.IsCurrent(generation)`,
		`uh.searchResultRows.Clear(func(row *adw.ActionRow)`,
		`row.SetSubtitle(result.Kind.DisplayName())`,
		`if !gate.TryStart()`,
		`uh.confirmHomebrewInstall(result, button, gate)`,
		`dialog.AddResponse("install", "Install")`,
		`button.SetLabel("Installing...")`,
		`homebrew.Install(result.Name, result.Kind == homebrew.Cask)`,
		`actionstate.PackageInstall(err == nil, dryRun)`,
		`if decision.RestoreControl`,
		`if decision.CompleteControl`,
		`go uh.loadHomebrewPackages()`,
		`uh.formulaeRows.Clear(func(row *adw.ActionRow)`,
		`uh.caskRows.Clear(func(row *adw.ActionRow)`,
		`uh.confirmHomebrewPin(pkg.Name, !pkg.Pinned, pinBtn, controls, gate)`,
		`uh.confirmHomebrewUninstall(pkg.Name, homebrew.Formula, uninstallBtn, controls, gate)`,
		`uh.confirmHomebrewUninstall(pkg.Name, homebrew.Cask, uninstallBtn, controls, gate)`,
		`dialog.SetResponseAppearance("uninstall", adw.ResponseDestructiveValue)`,
		`primary.SetLabel("Uninstalling...")`,
		`homebrew.Uninstall(name, kind == homebrew.Cask)`,
		`actionstate.PackageUninstall(err == nil, dryRun)`,
		`homebrew.Pin(name)`,
		`homebrew.Unpin(name)`,
		`actionstate.PackagePin(err == nil, dryRun)`,
		`setHomebrewControlsSensitive(controls, false)`,
		`uh.finishHomebrewPackageMutation(`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("applications-page wiring does not contain %q", required)
		}
	}
}
