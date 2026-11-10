package views

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/frostui"
)

// brewPage draws what draw draws as a page's content, on the window's
// background with the shell's padding.
func brewPage(draw func(c *ui.Context)) func(c *ui.Context) {
	return func(c *ui.Context) {
		t := frostui.Adapt(c)
		ui.Column(c).Fill().Background(t.Background).Padding(frostui.SpaceXL, 28).Gap(18).AlignItems(ui.Stretch).
			Children(func() { draw(c) })
	}
}

// TestSnapshotBrew fills the Find more and Bundles tabs and the Homebrew
// card's extra on the Updates page with realistic data and checks what
// they say; with CHAIRLIFT_SNAPSHOT_DIR set it saves them, light and dark.
func TestSnapshotBrew(t *testing.T) {
	t.Setenv("FROSTUI_ICON_THEME", "Adwaita")
	frostui.SetIcons(frostui.NewIcons(nil))
	f := newFakeBrew()
	f.formulae = append(f.formulae, homebrew.Package{Name: "git", Version: "2.51.2"})
	f.search["git"] = []homebrew.SearchResult{
		{Name: "git", Kind: homebrew.Formula},
		{Name: "git-delta", Kind: homebrew.Formula},
		{Name: "git-lfs", Kind: homebrew.Formula},
		{Name: "gitui", Kind: homebrew.Formula},
		{Name: "lazygit", Kind: homebrew.Formula},
		{Name: "github", Kind: homebrew.Cask},
		{Name: "gitkraken", Kind: homebrew.Cask},
	}
	h := startBrew(t, f, allBrewGroups...)
	b := h.b
	b.Search("git")
	h.idle()
	// lazygit installs, the cli bundle was installed.
	a := brewActionFor(b.search.rows, "formula:lazygit")
	a.take()
	a.running = "Installing…"
	b.bundles.items[0].installed = true

	views := []struct {
		name  string
		w, h  int
		draw  func(c *ui.Context)
		texts []string
	}{
		{"brew-search", 1150, 720, b.DrawSearch, []string{"7 results for “git”", "brew install --cask github", "Installing…", "Installed"}},
		{"brew-bundles", 1150, 560, b.DrawBundles, []string{"cli", "Recommended CLI Tools", "21 packages", "10 packages", "+13 more", "Export to ~/Brewfile"}},
		{"brew-extra", 1150, 300, func(c *ui.Context) {
			src := b.Source()
			frostui.Card(c).Children(func() {
				ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
					frostui.IconTile(c, src.Icon(), frostui.Accent, 32)
					ui.Column(c).Grow(1).Children(func() {
						frostui.CardHeader(c, src.Title(), src.Blurb()+" "+src.Status().Note, nil)
					})
				})
				src.Extra(c)
			})
		}, []string{"Homebrew skips bbrew because valkyrie00/bbrew is not trusted. Trust the tap to update its packages again.", "Trust tap", "Update metadata"}},
	}
	for _, v := range views {
		for _, dark := range []bool{false, true} {
			tt := ui.NewTesterWith(brewPage(v.draw), v.w, v.h, ui.TesterOptions{Dark: dark})
			tt.Frame()
			for _, want := range v.texts {
				if !tt.HasText(want) {
					t.Errorf("%s: missing %q in %q", v.name, want, tt.Texts())
				}
			}
			name := v.name
			if dark {
				name += "-dark"
			}
			snapshot(t, tt, name)
		}
	}
}
