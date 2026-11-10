package views

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
	"github.com/frostyard/chairlift/internal/views/pageview"
)

// brewSearch is the Find more tab (brew_search_group): a Homebrew search of
// both namespaces and installs from its results.
type brewSearch struct {
	// query is the field's text; searched is the query the results are for.
	query, searched string
	// refresh orders the searches: only the newest one shows. It is not
	// the installed inventory's: a search and a reload never retire each
	// other.
	refresh actionstate.RefreshGate
	loading uint64 // the generation in flight, 0 when none
	ran     bool
	results []homebrew.SearchResult
	err     string
	// rows are the results' install controls by key ("cask:firefox"); a
	// new search starts them afresh.
	rows map[string]*brewAction
	list ui.ListState
	// focus asks the next draw to focus the field.
	focus bool
}

// SearchAvailable reports whether the Apps page shows the Find more tab
// (brew_search_group, unless Homebrew is known to be missing).
func (b *Brew) SearchAvailable() bool {
	return b.enabled(brewAppsPage, groupBrewSearch) && !b.missing()
}

// Search searches Homebrew's formulae and casks for query. An empty query
// does nothing.
func (b *Brew) Search(query string) {
	b.search.query = query
	b.runSearch()
}

func (b *Brew) runSearch() {
	s := &b.search
	query := strings.TrimSpace(s.query)
	if query == "" || !b.SearchAvailable() {
		return
	}
	generation := s.refresh.Begin()
	s.loading = generation
	s.searched = query
	search := b.be.search
	type result struct {
		results []homebrew.SearchResult
		err     error
	}
	Go(b.env, func() result {
		r, err := search(query)
		return result{r, err}
	}, func(r result) {
		if !s.refresh.IsCurrent(generation) {
			return
		}
		s.loading = 0
		s.ran = true
		if r.err != nil {
			s.err = fmt.Sprintf("Error: %v", r.err)
			s.results = nil
			return
		}
		s.err = ""
		s.results = r.results
		s.rows = brewKeepRunning(s.rows)
	})
}

// askInstall asks before it installs a search result, of its own kind:
// `brew install` for a formula, `brew install --cask` for a cask.
func (b *Brew) askInstall(result homebrew.SearchResult) {
	key := brewKey(result.Kind, result.Name)
	a := brewActionFor(b.search.rows, key)
	if !a.take() {
		return
	}
	kind := strings.ToLower(result.Kind.DisplayName())
	b.env.Dialogs.Ask(frostui.Confirm{
		Title:     fmt.Sprintf("Install %s?", result.Name),
		Body:      fmt.Sprintf("Homebrew will install the %s %s and run its package-defined installation steps.", kind, result.Name),
		Details:   []frostui.KeyValue{{Key: "Command", Value: pageview.BrewInstallCommand(result.Name, result.Kind == homebrew.Cask), Mono: true}},
		Action:    "Install",
		OnCancel:  a.restore,
		OnConfirm: func() { b.runInstall(result, a) },
	})
}

func (b *Brew) runInstall(result homebrew.SearchResult, a *brewAction) {
	a.running = "Installing…"
	install := b.be.install
	name, cask := result.Name, result.Kind == homebrew.Cask
	b.enqueue(brewKey(result.Kind, name), "Installing "+name, func() error { return install(name, cask) }, func(err error) {
		if brewCanceled(err) {
			a.restore()
			return
		}
		dryRun := b.dryRun()
		decision := actionstate.PackageInstall(err == nil, dryRun)
		if decision.RestoreControl {
			a.restore()
		}
		if err != nil {
			b.env.ToastError(fmt.Sprintf("Install failed: %v", err))
			return
		}
		if decision.CompleteControl {
			a.complete("Installed")
			b.env.Record(brewJobSource, "Installed "+name, strings.ToLower(result.Kind.DisplayName()))
		}
		b.notify(dryRun, actionmsg.Install(dryRun, name))
		if decision.Refresh {
			// Nothing when brew_group is off: there is no inventory.
			b.RefreshApps()
		}
	})
}

// DrawSearch draws the Find more tab of the Apps page: Homebrew search and
// install. It searches when the user presses Enter or the button, not on
// every key.
func (b *Brew) DrawSearch(c *ui.Context) {
	if !b.SearchAvailable() {
		return
	}
	s := &b.search
	ui.Column(c).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
			field := ui.SearchField(c, &s.query).Label("Search Homebrew").
				Placeholder("Search Homebrew formulae and casks").MaxWidth(560).Grow(1)
			if s.focus {
				s.focus = false
				field.Focus()
			}
			if field.Submitted() {
				b.runSearch()
			}
			empty := strings.TrimSpace(s.query) == ""
			search := ui.PrimaryButton(c, "").Label("Search").Disabled(empty || s.loading != 0).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					frostui.Icon(c, "edit-find")
					ui.Text(c, "Search").SingleLine()
				})
			})
			if search.Clicked() {
				b.runSearch()
			}
		})
		frostui.Muted(c, pageview.BrewSearchStatus(s.loading != 0, s.searched, s.ran, len(s.results), s.err != ""))
		b.drawResults(c)
	})
}

// brewResultHeight is a result row's height, which sizes the list.
const brewResultHeight = 58

func (b *Brew) drawResults(c *ui.Context) {
	s := &b.search
	t := c.Theme()
	card := frostui.Card(c).Padding(6).Gap(0)
	card.Children(func() {
		switch {
		case s.loading != 0:
			ui.Row(c).Padding(frostui.SpaceS, frostui.SpaceM).Children(func() {
				frostui.Loading(c, "Searching Homebrew…")
			})
		case s.err != "":
			ui.Row(c).Padding(frostui.SpaceM).Children(func() { frostui.Problem(c, s.err) })
		case !s.ran:
			frostui.Empty(c, "edit-find", "Search Homebrew", "Find formulae and casks by name, then install them for your user. Nothing needs administrator rights.")
		case len(s.results) == 0:
			frostui.Empty(c, "edit-find", "Nothing found", "brew search found no formula or cask by that name.")
		default:
			results := s.results
			s.list.Key = func(i int) any { return brewKey(results[i].Kind, results[i].Name) }
			height := float32(min(len(results), 10)*brewResultHeight + 2)
			ui.List(c, &s.list, len(results), func(i int) {
				b.drawResult(c, results[i])
			}).Height(height).Dividers(1, t.Border)
		}
	})
}

func (b *Brew) drawResult(c *ui.Context, r homebrew.SearchResult) {
	t := c.Theme()
	key := brewKey(r.Kind, r.Name)
	cask := r.Kind == homebrew.Cask
	presentation := pageview.SearchResult(r.Name, r.Kind.DisplayName())
	ui.Row(c).Height(brewResultHeight).Gap(frostui.SpaceM).Padding(0, frostui.SpaceM).AlignItems(ui.Center).Children(func() {
		ui.Avatar(c, r.Name, nil).Size(30, 30)
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Strong(c, presentation.Title).SingleLine()
				tone := frostui.Warning
				if cask {
					tone = frostui.Accent
				}
				frostui.Pill(c, presentation.Subtitle, tone)
			})
			frostui.Mono(c, pageview.BrewInstallCommand(r.Name, cask)).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
		})
		ui.Row(c).Width(130).Justify(ui.End).Children(func() {
			a := b.search.rows[key]
			switch {
			case a != nil && a.completed != "":
				frostui.IconPill(c, "object-select", a.completed, frostui.Success)
			case b.installedKey(r.Kind, r.Name) && (a == nil || a.running == ""):
				frostui.IconPill(c, "object-select", "Installed", frostui.Success)
			case (a != nil && a.running != "") || b.pending(key):
				label := "Installing…"
				if a != nil && a.running != "" {
					label = a.running
				}
				ui.Button(c, label).Disabled(true)
			default:
				if ui.Button(c, "Install").Label("Install " + r.Name + " (" + strings.ToLower(r.Kind.DisplayName()) + ")").Clicked() {
					b.askInstall(r)
				}
			}
		})
	})
}
