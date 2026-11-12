package views

import (
	"fmt"
	"log"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/bundleview"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/frostui"
)

// brewBundles is the Bundles tab (brew_bundles_group): the Brewfiles of the
// configured bundles_paths. It needs nothing of brew_group.
type brewBundles struct {
	loading bool
	loaded  bool
	items   []*brewBundle
	// presentation is bundleview's text for what discovery found; plain
	// is its text without a warning, which the tab does not repeat.
	presentation bundleview.Presentation
	plain        string
	// dirs are the configured bundles_paths.
	dirs []string
	// homebrew is whether Homebrew is installed: without it the bundles
	// show, with their installs off.
	homebrew bool
	// columns is the grid's column count, from its width last frame.
	columns int
}

// brewBundle is one bundle card.
type brewBundle struct {
	homebrew.Bundle
	// packages are what its Brewfile installs, nil when it could not be
	// read.
	packages []pageview.BrewfileEntry
	gate     bundleview.InstallGate
	running  bool
	// installed is set by a live install; nothing tells it before.
	installed bool
}

// BundlesAvailable reports whether the Apps page shows the Bundles tab
// (brew_bundles_group).
func (b *Brew) BundlesAvailable() bool {
	return b.enabled(brewAppsPage, groupBrewBundles)
}

// loadBundles discovers the configured Brewfiles and reads their packages
// on a worker.
func (b *Brew) loadBundles() {
	if !b.BundlesAvailable() || b.bundles.loading {
		return
	}
	var paths []string
	if g := b.env.Group(brewAppsPage, groupBrewBundles); g != nil {
		paths = append(paths, g.BundlesPaths...)
	}
	b.bundles.loading = true
	be := b.be
	type result struct {
		bundles   []homebrew.Bundle
		packages  [][]pageview.BrewfileEntry
		warning   string
		available bool
	}
	Go(b.env, func() result {
		bundles, err := be.bundles(paths)
		r := result{bundles: bundles, available: be.installed()}
		if err != nil {
			log.Printf("Error discovering Brew bundles: %v", err)
			r.warning = err.Error()
		}
		for _, bundle := range bundles {
			var entries []pageview.BrewfileEntry
			if text, err := be.readBrewfile(bundle.Path); err == nil {
				entries = pageview.BrewfileEntries(text)
			}
			r.packages = append(r.packages, entries)
		}
		return r
	}, func(r result) {
		bd := &b.bundles
		bd.loading, bd.loaded = false, true
		bd.homebrew = r.available
		bd.dirs = paths
		bd.presentation = bundleview.Present(len(r.bundles), r.warning, r.available)
		bd.plain = bundleview.Present(len(r.bundles), "", true).Description
		bd.items = nil
		for i, bundle := range r.bundles {
			bd.items = append(bd.items, &brewBundle{Bundle: bundle, packages: r.packages[i]})
		}
	})
}

func (b *Brew) findBundle(path string) *brewBundle {
	for _, bd := range b.bundles.items {
		if bd.Path == path {
			return bd
		}
	}
	return nil
}

// AskBundle asks before it installs the bundle at path with `brew bundle`.
func (b *Brew) AskBundle(path string) {
	bd := b.findBundle(path)
	if bd == nil || !b.bundles.homebrew || !bd.gate.TryStart() {
		return
	}
	details := []frostui.KeyValue{{Key: "Brewfile", Value: bd.Path, Mono: true}}
	if len(bd.packages) > 0 {
		details = append(details, frostui.KeyValue{Key: "Installs", Value: pageview.BrewCount(len(bd.packages), "package", "packages")})
	}
	b.env.Dialogs.Ask(frostui.Confirm{
		Title:     fmt.Sprintf("Install the %s bundle?", bd.Name),
		Body:      "Homebrew installs every package of this Brewfile for your user with brew bundle. Nothing needs administrator rights.",
		Details:   details,
		Action:    "Install",
		OnCancel:  bd.gate.Reset,
		OnConfirm: func() { b.runBundle(bd) },
	})
}

func (b *Brew) runBundle(bd *brewBundle) {
	bd.running = true
	install := b.be.bundleInstall
	name, path := bd.Name, bd.Path
	b.enqueue("bundle:"+path, "Installing the "+name+" bundle", func() error { return install(path) }, func(err error) {
		bd.running = false
		if brewCanceled(err) {
			bd.gate.Reset()
			return
		}
		if err != nil {
			bd.gate.Reset()
			b.env.ToastError(fmt.Sprintf("Could not install Brew bundle %s: %v", name, err))
			return
		}
		dryRun := b.dryRun()
		decision := actionmsg.BundleInstall(dryRun, name)
		if decision.Complete {
			bd.gate.Complete()
			bd.installed = true
			b.env.ToastSuccess(decision.Toast)
			b.env.Record(brewJobSource, "Installed the "+name+" bundle", bd.Description)
			// Nothing when brew_group is off: bundles never assume it.
			b.RefreshApps()
			return
		}
		bd.gate.Reset()
		b.env.Toast(decision.Toast)
	})
}

// brewBundleMinWidth is the narrowest a bundle card gets before the grid
// drops a column.
const brewBundleMinWidth = 320

// DrawBundles draws the Bundles tab of the Apps page: a card per configured
// Brewfile, with what it installs and its Install button.
func (b *Brew) DrawBundles(c *ui.Context) {
	if !b.BundlesAvailable() {
		return
	}
	bd := &b.bundles
	ui.Column(c).Gap(frostui.SpaceL).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(frostui.SpaceXS).Children(func() {
				from := ""
				if len(bd.dirs) > 0 {
					from = " from " + strings.Join(bd.dirs, ", ")
				}
				frostui.Muted(c, "Curated Brewfiles"+from+". Installing one runs brew bundle for your user; nothing needs administrator rights.").Wrap()
				if bd.loaded && bd.presentation.Description != bd.plain && len(bd.items) > 0 {
					// Discovery warnings, and that Homebrew is missing.
					frostui.Toned(c, bd.presentation.Description, frostui.Warning).Wrap()
				}
			})
			if b.ExportAvailable() {
				label := "Export to ~/Brewfile"
				if b.export.running != "" {
					label = b.export.running
				}
				if frostui.LabeledButton(c, "document-save", label, false).Disabled(b.export.inUse()).
					Tooltip("Write every installed formula, cask and tap to ~/Brewfile").Clicked() {
					b.ExportBrewfile()
				}
			}
		})
		switch {
		case !bd.loaded:
			frostui.Loading(c, "Looking for Brew bundles…")
		case len(bd.items) == 0:
			frostui.Card(c).Children(func() {
				frostui.Empty(c, "system-software-install", bd.presentation.PlaceholderTitle, bd.presentation.PlaceholderSubtitle)
				if bd.presentation.Description != bd.plain {
					frostui.Muted(c, bd.presentation.Description).Wrap().TextAlign(ui.Center)
				}
			})
		default:
			b.drawBundleGrid(c)
		}
	})
}

func (b *Brew) drawBundleGrid(c *ui.Context) {
	bd := &b.bundles
	cols := max(1, bd.columns)
	grid := ui.Grid(c).Columns(cols).Gap(frostui.SpaceL)
	if w := grid.LayoutBounds().W; w > 0 {
		fit := max(1, min(3, int((w+frostui.SpaceL)/(brewBundleMinWidth+frostui.SpaceL))))
		if fit != bd.columns {
			bd.columns = fit
			c.Invalidate()
		}
	}
	grid.Children(func() {
		for _, item := range bd.items {
			b.drawBundle(c, item)
		}
	})
}

// brewBundleChips is how many package names a card shows.
const brewBundleChips = 8

// sameName reports whether another bundle has item's name, from another
// directory: its card then shows its path.
func (bd *brewBundles) sameName(item *brewBundle) bool {
	for _, other := range bd.items {
		if other != item && other.Name == item.Name {
			return true
		}
	}
	return false
}

func (b *Brew) drawBundle(c *ui.Context, item *brewBundle) {
	presentation := pageview.BrewBundle(item.Name, item.Description, item.Path)
	frostui.Card(c).Key(item.Path).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Tooltip(presentation.Subtitle).Children(func() {
			frostui.IconTile(c, "system-software-install", frostui.Accent, 36)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				frostui.Strong(c, presentation.Title).SingleLine()
				what := item.Description
				if what == "" {
					what = "Brewfile"
				}
				frostui.Muted(c, what).MaxLines(2)
			})
		})
		if n := len(item.packages); n > 0 {
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for i, p := range item.packages[:min(n, brewBundleChips)] {
					brewChip(c.Key(i), p.Name)
				}
				if n > brewBundleChips {
					brewChip(c.Key("more"), fmt.Sprintf("+%d more", n-brewBundleChips))
				}
			})
		}
		if b.bundles.sameName(item) {
			frostui.Caption(c, item.Path).Font("monospace").SingleLine()
		}
		// The footer sits at the bottom, level with its neighbours'.
		ui.Spacer(c)
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
			count := "Brewfile"
			if n := len(item.packages); n > 0 {
				count = pageview.BrewCount(n, "package", "packages")
			}
			frostui.Muted(c, count).Grow(1)
			switch {
			case item.installed:
				frostui.IconPill(c, "object-select", "Installed", frostui.Success)
			case item.running || b.pending("bundle:"+item.Path):
				ui.Button(c, "Installing…").Disabled(true)
			default:
				btn := ui.PrimaryButton(c, "Install").Label("Install the " + item.Name + " bundle").Disabled(!b.bundles.homebrew)
				if !b.bundles.homebrew {
					btn.Tooltip("Homebrew is not installed")
				}
				if btn.Clicked() {
					b.AskBundle(item.Path)
				}
			}
		})
	})
}

// brewChip is a package name on a bundle card: the brand's quiet tag, the
// tag text inside a neutral hairline. Unlike a kit pill it keeps the name
// as written, since package names are lowercase.
func brewChip(c *ui.Context, text string) ui.Element {
	t := c.Theme()
	return ui.Row(c).Padding(2, 7).Radius(frostui.RadiusChip).Border(1, frostui.ToneLine(t, frostui.Neutral)).Children(func() {
		frostui.Caption(c, text).TextColor(frostui.ColorsOf(t).TagText).SingleLine()
	})
}
