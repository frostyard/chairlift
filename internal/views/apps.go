package views

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/navigation"
)

// The Apps page's tabs.
const (
	appsTabInstalled = "installed"
	appsTabBundles   = "bundles"
	appsTabSearch    = "search"
)

// The kind filter's choices, in its order.
const (
	appsShowAll      = "All"
	appsShowFlatpak  = "Flatpak"
	appsShowHomebrew = "Homebrew"
	appsShowCasks    = "Casks"
)

// The table's column IDs, which ListState.Sort names.
const (
	appsColApp     = "app"
	appsColSource  = "source"
	appsColVersion = "version"
	appsColStatus  = "status"
)

// appsDefaultCenter is the software center the header opens when the
// configuration names none.
const appsDefaultCenter = "io.github.kolunmi.Bazaar"

// appsPage is the Apps page: every installed Flatpak app and Homebrew
// package in one sortable table with an inspector for the chosen row, and
// Homebrew's bundles and search in tabs of their own.
type appsPage struct {
	s *Store

	// The data and the actions, replaceable in tests. The page reads each
	// kind's entries only while its status says it is available.
	flatpakApps, brewApps     func() []AppEntry
	flatpakStatus, brewStatus func() AppsStatus
	actFlatpak, actBrew       func(key string, action AppAction)
	bundles, search           func() bool
	drawBundles, drawSearch   func(c *ui.Context)

	tab    string
	filter string
	query  string
	sort   ui.SortOrder
	table  ui.ListState
	row    int
	// chosen is the Key of the entry the inspector shows, "" for none.
	chosen string
	cols   []ui.TableColumn
}

// NewAppsPage returns the Apps page.
func NewAppsPage(s *Store) Page { return newAppsPage(s) }

func newAppsPage(s *Store) *appsPage {
	return &appsPage{
		s:             s,
		flatpakApps:   func() []AppEntry { return s.Flatpak.Apps() },
		brewApps:      func() []AppEntry { return s.Brew.Apps() },
		flatpakStatus: func() AppsStatus { return s.Flatpak.AppsStatus() },
		brewStatus:    func() AppsStatus { return s.Brew.AppsStatus() },
		actFlatpak:    func(key string, a AppAction) { s.Flatpak.Act(key, a) },
		actBrew:       func(key string, a AppAction) { s.Brew.Act(key, a) },
		bundles:       func() bool { return s.Brew.BundlesAvailable() },
		search:        func() bool { return s.Brew.SearchAvailable() },
		drawBundles:   func(c *ui.Context) { s.Brew.DrawBundles(c) },
		drawSearch:    func(c *ui.Context) { s.Brew.DrawSearch(c) },
		tab:           appsTabInstalled,
		filter:        appsShowAll,
		sort:          ui.SortOrder{Column: appsColApp},
		row:           -1,
		cols: []ui.TableColumn{
			{Title: "App", ID: appsColApp, Sortable: true, MinWidth: 180},
			{Title: "Source", ID: appsColSource, Width: 130, Sortable: true},
			{Title: "Version", ID: appsColVersion, Width: 130, Sortable: true},
			{Title: "", ID: appsColStatus, Width: 112},
		},
	}
}

// appsTab is one tab of the page.
type appsTab struct{ id, title, icon string }

// tabs are the tabs that apply: Installed while Flatpak or Homebrew lists
// anything, Bundles and Find more as Homebrew offers them.
func (p *appsPage) tabs(fp, br AppsStatus) []appsTab {
	var tabs []appsTab
	if fp.Available || br.Available {
		tabs = append(tabs, appsTab{appsTabInstalled, "Installed", "package-x-generic"})
	}
	if p.bundles() {
		tabs = append(tabs, appsTab{appsTabBundles, "Bundles", "view-list-bullet"})
	}
	if p.search() {
		tabs = append(tabs, appsTab{appsTabSearch, "Find more", "edit-find"})
	}
	return tabs
}

// center is the configured software center (applications_installed_group),
// ok false when the group is off.
func (p *appsPage) center() (appID, name string, ok bool) {
	env := p.s.Env
	if !env.Enabled("applications_page", "applications_installed_group") {
		return "", "", false
	}
	appID = appsDefaultCenter
	if g := env.Group("applications_page", "applications_installed_group"); g != nil && g.AppID != "" {
		appID = g.AppID
	}
	return appID, appsCenterName(appID), true
}

// entries are the rows of every available kind.
func (p *appsPage) entries(fp, br AppsStatus) []AppEntry {
	var all []AppEntry
	if fp.Available {
		all = append(all, p.flatpakApps()...)
	}
	if br.Available {
		all = append(all, p.brewApps()...)
	}
	return all
}

// View draws the page.
func (p *appsPage) View(c *ui.Context) {
	fp, br := p.flatpakStatus(), p.brewStatus()
	entries := p.entries(fp, br)
	tabs := p.tabs(fp, br)
	if len(tabs) > 0 && !slices.ContainsFunc(tabs, func(t appsTab) bool { return t.id == p.tab }) {
		p.tab = tabs[0].id
	}
	frostui.PageHeader(c, "Apps", appsSubtitle(entries, fp, br), func() {
		if appID, name, ok := p.center(); ok {
			if frostui.LabeledButton(c, "system-software-install", "Open "+name, false).
				Tooltip("Open the application manager to install and manage applications").Clicked() {
				p.s.Flatpak.LaunchApp(appID)
			}
		}
		p.tabBar(c, tabs)
	})
	if len(tabs) == 0 {
		p.nothing(c)
		return
	}
	switch p.tab {
	case appsTabBundles:
		p.drawBundles(c)
	case appsTabSearch:
		p.drawSearch(c)
	default:
		p.installed(c, entries, fp, br)
	}
}

// nothing is the page with only the software center: no group lists apps.
func (p *appsPage) nothing(c *ui.Context) {
	_, name, ok := p.center()
	body := ""
	if ok {
		body = "Find and install apps with " + name + "."
	}
	frostui.Card(c).Children(func() {
		frostui.Empty(c, "package-x-generic", "ChairLift lists no apps here", body)
	})
}

// tabBar draws the tabs as pills, when there is more than one.
func (p *appsPage) tabBar(c *ui.Context, tabs []appsTab) {
	if len(tabs) < 2 {
		return
	}
	idx := slices.IndexFunc(tabs, func(t appsTab) bool { return t.id == p.tab })
	idx = max(idx, 0)
	pills := make([]frostui.Tab, len(tabs))
	for i, tab := range tabs {
		pills[i] = frostui.Tab{Title: tab.title, Icon: tab.icon}
	}
	frostui.PillTabs(c, &idx, pills)
	p.tab = tabs[idx].id
}

// installed draws the Installed tab: the kind filter and the filter field,
// then the table and the inspector side by side.
func (p *appsPage) installed(c *ui.Context, entries []AppEntry, fp, br AppsStatus) {
	filters := appsFilters(fp, br)
	if !slices.Contains(filters, p.filter) {
		p.filter = appsShowAll
	}
	ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
		if len(filters) > 2 {
			idx := slices.Index(filters, p.filter)
			ui.Segmented(c, &idx, filters...).Label("Show")
			p.filter = filters[max(idx, 0)]
		}
		ui.Spacer(c)
		refresh := frostui.IconButton(c, "view-refresh", "Refresh installed apps").Disabled(appsLoading(fp, br))
		if refresh.Clicked() {
			p.refresh()
		}
		ui.SearchField(c, &p.query).Label("Filter apps").Placeholder("Filter apps").Width(260)
	})
	rows := appsSort(appsFilter(entries, p.filter, p.query), p.sort)
	p.row = slices.IndexFunc(rows, func(e AppEntry) bool { return e.Key == p.chosen })
	ui.Row(c).Gap(frostui.SpaceL).AlignItems(ui.Start).Children(func() {
		frostui.Card(c).Padding(6).Gap(0).Grow(1).Basis(0).MinWidth(0).Children(func() {
			p.loadState(c, entries, fp, br)
			switch {
			case len(entries) == 0 && appsLoading(fp, br):
				// The loading lines say it all.
			case len(entries) == 0:
				body := ""
				if _, name, ok := p.center(); ok {
					body = "Find apps to install with " + name + "."
				}
				frostui.Empty(c, "package-x-generic", "No apps installed", body)
			case len(rows) == 0:
				frostui.Empty(c, "edit-find", "No apps match", "Try another name or source.")
			default:
				p.drawTable(c, rows)
			}
		})
		var chosen *AppEntry
		if p.row >= 0 && p.row < len(rows) {
			chosen = &rows[p.row]
		}
		p.inspector(c, chosen)
	})
}

// loadState draws a line for each kind that loads or failed.
func (p *appsPage) loadState(c *ui.Context, entries []AppEntry, fp, br AppsStatus) {
	type line struct {
		key, loading, problem string
	}
	var lines []line
	has := func(flatpak bool) bool {
		return slices.ContainsFunc(entries, func(e AppEntry) bool { return appsIsFlatpak(e.Kind) == flatpak })
	}
	if fp.Available {
		l := line{key: "flatpak"}
		if fp.Loading && !has(true) {
			l.loading = "Loading Flatpak apps…"
		}
		if fp.Err != "" {
			l.problem = "Flatpak: " + fp.Err
		}
		lines = append(lines, l)
	}
	if br.Available {
		l := line{key: "brew"}
		if br.Loading && !has(false) {
			l.loading = "Loading Homebrew packages…"
		}
		if br.Err != "" {
			l.problem = "Homebrew: " + br.Err
		}
		lines = append(lines, l)
	}
	for _, l := range lines {
		if l.loading == "" && l.problem == "" {
			continue
		}
		ui.Column(c.Key(l.key)).Gap(frostui.SpaceXS).Padding(6, 12).AlignItems(ui.Stretch).Children(func() {
			if l.loading != "" {
				frostui.Loading(c, l.loading)
			}
			if l.problem != "" {
				frostui.Problem(c, l.problem)
			}
		})
	}
}

// appsRowHeight and appsHeaderHeight are the table's metrics, to size it
// to its rows: the page scrolls, so the table is only as tall as needed,
// up to the room the window leaves it.
const (
	appsRowHeight      = 46
	appsHeaderHeight   = 38
	appsChromeHeight   = 270
	appsMinTableHeight = 280
)

func (p *appsPage) drawTable(c *ui.Context, rows []AppEntry) {
	t := c.Theme()
	_, winH := c.Size()
	room := max(winH-appsChromeHeight, appsMinTableHeight)
	height := min(float32(appsHeaderHeight+appsRowHeight*len(rows)), room)
	p.table.Selected = &p.row
	p.table.Sort = &p.sort
	p.table.Key = func(i int) any { return rows[i].Key }
	// The table chooses rows in the accent; under this theme the chosen
	// row is a soft tint of it instead, so the pills on it still show.
	// The cells take their colors from t, the page's own theme.
	ui.Themed(c, frostui.SoftSelection(t)).Background(ui.Transparent).Height(height).Children(func() {
		ui.Table(c, &p.table, p.cols, len(rows), func(row, col int) {
			e := rows[row]
			switch col {
			case 0:
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					frostui.SquareAvatar(c, e.Name, 28)
					ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
						ui.Text(c, e.Name).FontSize(13.5).FontWeight(600).TextColor(t.Text).SingleLine()
						ui.Text(c, e.ID).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
					})
				})
			case 1:
				ui.Row(c).Children(func() { frostui.ColorPill(c, appsKindLabel(e.Kind), appsKindColor(t, e.Kind)) })
			case 2:
				if e.Version == "" {
					ui.Text(c, "—").TextColor(t.TextMuted)
				} else {
					frostui.Mono(c, e.Version).TextColor(t.Text).SingleLine()
				}
			case 3:
				ui.Row(c).Children(func() { appsStatusPill(c, t, e) })
			}
		}).Grow(1)
	})
	if p.row >= 0 && p.row < len(rows) {
		p.chosen = rows[p.row].Key
	}
}

// inspector is the card at the right: the chosen entry's facts and its
// actions.
func (p *appsPage) inspector(c *ui.Context, e *AppEntry) {
	t := c.Theme()
	frostui.Card(c).Width(300).Gap(14).Shrink(0).Children(func() {
		if e == nil {
			frostui.Empty(c, "package-x-generic", "Pick an app", "Its details and actions show here.")
			return
		}
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			frostui.SquareAvatar(c, e.Name, 52)
			ui.Column(c).Gap(5).Grow(1).MinWidth(0).AlignItems(ui.Start).Children(func() {
				ui.Text(c, e.Name).FontSize(17).Bold().MaxLines(2)
				frostui.ColorPill(c, appsKindLabel(e.Kind), appsKindColor(t, e.Kind))
			})
		})
		if e.Summary != "" {
			frostui.Muted(c, e.Summary).Wrap()
		}
		frostui.RowDivider(c)
		facts := []frostui.KeyValue{{Key: "ID", Value: e.ID, Mono: true}}
		if e.Version != "" {
			facts = append(facts, frostui.KeyValue{Key: "Version", Value: e.Version})
		}
		if e.Origin != "" {
			facts = append(facts, frostui.KeyValue{Key: "Origin", Value: e.Origin})
		}
		if e.Update != "" {
			facts = append(facts, frostui.KeyValue{Key: "Update to", Value: e.Update})
		}
		if e.Pinned {
			facts = append(facts, frostui.KeyValue{Key: "Pinned", Value: "Upgrades leave it at this version"})
		}
		frostui.KeyValues(c, facts, frostui.KeyValuesStyle{KeyWidth: 72, Gap: 10, FontSize: 12.5})
		if len(e.Actions) == 0 {
			return
		}
		frostui.RowDivider(c)
		if e.Busy {
			frostui.Loading(c, "Working…")
		}
		for _, a := range e.Actions {
			p.actionButton(c.Key(int(a)), t, *e, a)
		}
	})
}

// actionButton draws one inspector action and dispatches it to the
// entry's domain.
func (p *appsPage) actionButton(c *ui.Context, t *ui.Theme, e AppEntry, a AppAction) {
	var b ui.Element
	switch a {
	case ActUpdate:
		b = frostui.LabeledButton(c, "software-update-available", "Update to "+e.Update, true)
	case ActOpen:
		b = frostui.LabeledButton(c, "window-new", "Open", false)
	case ActPin:
		b = frostui.LabeledButton(c, "changes-prevent", "Pin this version", false)
	case ActUnpin:
		b = frostui.LabeledButton(c, "changes-allow", "Unpin version", false)
	case ActUninstall:
		b = frostui.LabeledButton(c, "user-trash", "Uninstall", false).TextColor(t.Danger).AlignSelf(ui.Start)
	default:
		return
	}
	// Open starts the app and changes nothing, so it stays while a job runs.
	if b.Disabled(e.Busy && a != ActOpen).Clicked() {
		p.act(e, a)
	}
}

// act sends action to the domain of e's kind.
func (p *appsPage) act(e AppEntry, a AppAction) {
	if appsIsFlatpak(e.Kind) {
		p.actFlatpak(e.Key, a)
	} else {
		p.actBrew(e.Key, a)
	}
}

func (p *appsPage) refresh() {
	if p.flatpakStatus().Available {
		p.s.Flatpak.RefreshApps()
	}
	if p.brewStatus().Available {
		p.s.Brew.RefreshApps()
	}
}

// show opens the Installed tab with a kind filter.
func (p *appsPage) show(filter string) {
	p.tab, p.filter, p.query = appsTabInstalled, filter, ""
	p.s.Env.Navigate(navigation.Apps)
}

// Commands are the page's palette entries: refresh the inventory, open the
// software center, show one kind, and Homebrew's own.
func (p *appsPage) Commands() []frostui.Command {
	fp, br := p.flatpakStatus(), p.brewStatus()
	var cmds []frostui.Command
	if fp.Available || br.Available {
		cmds = append(cmds, frostui.Command{
			ID: "apps-refresh", Title: "Refresh installed apps", Group: "Apps", Icon: "view-refresh",
			Hint:     "reload installed flatpak homebrew packages",
			Disabled: appsLoading(fp, br), Reason: "loading", Run: p.refresh,
		})
	}
	if appID, name, ok := p.center(); ok {
		cmds = append(cmds, frostui.Command{
			ID: "apps-open-center", Title: "Open " + name, Group: "Apps", Icon: "system-software-install",
			Hint: "software center store install find apps",
			Run:  func() { p.s.Flatpak.LaunchApp(appID) },
		})
	}
	if fp.Available {
		cmds = append(cmds, frostui.Command{
			ID: "apps-show-flatpak", Title: "Show Flatpak apps", Group: "Apps", Icon: "package-x-generic",
			Run: func() { p.show(appsShowFlatpak) },
		})
	}
	if br.Available {
		cmds = append(cmds, frostui.Command{
			ID: "apps-show-homebrew", Title: "Show Homebrew packages", Group: "Apps", Icon: "package-x-generic",
			Hint: "formulae casks brew", Run: func() { p.show(appsShowHomebrew) },
		})
	}
	return append(cmds, p.s.Brew.Commands()...)
}

// Badge is none: the Updates page counts the updates.
func (p *appsPage) Badge() (string, frostui.Tone) { return "", frostui.Neutral }

// --- Pure helpers -----------------------------------------------------------

func appsIsFlatpak(k AppKind) bool { return k == AppFlatpakSystem || k == AppFlatpakUser }

func appsLoading(fp, br AppsStatus) bool {
	return (fp.Available && fp.Loading) || (br.Available && br.Loading)
}

// appsFilters are the kind filter's choices: All, then each kind whose
// inventory is available.
func appsFilters(fp, br AppsStatus) []string {
	out := []string{appsShowAll}
	if fp.Available {
		out = append(out, appsShowFlatpak)
	}
	if br.Available {
		out = append(out, appsShowHomebrew, appsShowCasks)
	}
	return out
}

// appsFilter returns the entries of the kinds filter shows whose name or
// ID holds query, ignoring case. Homebrew shows formulae and casks; Casks
// only casks.
func appsFilter(rows []AppEntry, filter, query string) []AppEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []AppEntry
	for _, e := range rows {
		switch filter {
		case appsShowFlatpak:
			if !appsIsFlatpak(e.Kind) {
				continue
			}
		case appsShowHomebrew:
			if appsIsFlatpak(e.Kind) {
				continue
			}
		case appsShowCasks:
			if e.Kind != AppCask {
				continue
			}
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Name), q) && !strings.Contains(strings.ToLower(e.ID), q) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// appsSort sorts a copy of rows by order's column: the name (the default),
// the source (the kinds' order) or the version (numbers as numbers), then
// by name and key so equal rows keep a stable order.
func appsSort(rows []AppEntry, order ui.SortOrder) []AppEntry {
	out := slices.Clone(rows)
	slices.SortStableFunc(out, func(a, b AppEntry) int {
		r := 0
		switch order.Column {
		case appsColSource:
			r = cmp.Compare(a.Kind, b.Kind)
		case appsColVersion:
			r = appsNaturalCompare(a.Version, b.Version)
		}
		if r == 0 {
			r = appsNaturalCompare(a.Name, b.Name)
		}
		if r == 0 {
			r = strings.Compare(a.Key, b.Key)
		}
		if order.Descending {
			r = -r
		}
		return r
	})
	return out
}

// appsNaturalCompare compares a and b ignoring case, with runs of digits
// compared as numbers: "1.9" before "1.10", "gh" before "GIMP".
func appsNaturalCompare(a, b string) int {
	for a != "" && b != "" {
		if appsIsDigit(a[0]) && appsIsDigit(b[0]) {
			na, ra := appsDigits(a)
			nb, rb := appsDigits(b)
			if r := cmp.Compare(len(na), len(nb)); r != 0 {
				return r
			}
			if r := strings.Compare(na, nb); r != 0 {
				return r
			}
			a, b = ra, rb
			continue
		}
		ca, sa := utf8.DecodeRuneInString(a)
		cb, sb := utf8.DecodeRuneInString(b)
		if r := cmp.Compare(unicode.ToLower(ca), unicode.ToLower(cb)); r != 0 {
			return r
		}
		a, b = a[sa:], b[sb:]
	}
	return cmp.Compare(len(a), len(b))
}

func appsIsDigit(c byte) bool { return c >= '0' && c <= '9' }

// appsDigits splits the leading digits off s, without leading zeros.
func appsDigits(s string) (digits, rest string) {
	i := 0
	for i < len(s) && appsIsDigit(s[i]) {
		i++
	}
	digits = strings.TrimLeft(s[:i], "0")
	return digits, s[i:]
}

// appsSubtitle counts the installed entries by kind: "20 installed: 10
// Flatpak apps and 10 Homebrew packages".
func appsSubtitle(entries []AppEntry, fp, br AppsStatus) string {
	flatpaks, brews := 0, 0
	for _, e := range entries {
		if appsIsFlatpak(e.Kind) {
			flatpaks++
		} else {
			brews++
		}
	}
	if len(entries) == 0 && appsLoading(fp, br) {
		return "Looking for installed apps…"
	}
	fps := appsCount(flatpaks, "Flatpak app", "Flatpak apps")
	bws := appsCount(brews, "Homebrew package", "Homebrew packages")
	switch {
	case fp.Available && br.Available:
		return fmt.Sprintf("%d installed: %s and %s", flatpaks+brews, fps, bws)
	case fp.Available:
		return fps + " installed"
	case br.Available:
		return bws + " installed"
	}
	return "Find and install apps"
}

func appsCount(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// appsCenterName names a software center by its application ID: the last
// part of the ID, with the usual centers spelled as their desktops do.
func appsCenterName(appID string) string {
	switch appID {
	case "org.gnome.Software":
		return "Software"
	case "org.kde.discover":
		return "Discover"
	}
	name := appID[strings.LastIndex(appID, ".")+1:]
	if name == "" {
		return appID
	}
	return name
}

// appsKindLabel is the Source pill's text.
func appsKindLabel(k AppKind) string {
	switch k {
	case AppFlatpakSystem:
		return "Flatpak"
	case AppFlatpakUser:
		return "Flatpak (you)"
	case AppFormula:
		return "Formula"
	case AppCask:
		return "Cask"
	}
	return ""
}

// appsKindColor is the Source pill's color: Flatpak in the accent, the
// user's Flatpak apps in green, formulae in amber, casks in violet.
func appsKindColor(t *ui.Theme, k AppKind) ui.Color {
	switch k {
	case AppFlatpakSystem:
		return t.Accent
	case AppFlatpakUser:
		return t.Success
	case AppFormula:
		return t.Warning
	}
	if t.Dark {
		return ui.Oklch(0.78, 0.13, 300)
	}
	return ui.Oklch(0.5, 0.2, 300)
}

// appsStatusPill is the last column: a spinner while a job works on the
// entry, else Update or Pinned. The table draws it under SoftSelection, so
// the Update pill takes the page theme's accent from t.
func appsStatusPill(c *ui.Context, t *ui.Theme, e AppEntry) {
	switch {
	case e.Busy:
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(14, 14).Label("Working")
			ui.Text(c, "Working").FontSize(11.5).TextColor(t.TextMuted)
		})
	case e.Update != "":
		frostui.DotPill(c, "Update", t.Accent).Tooltip("Update to " + e.Update)
	case e.Pinned:
		frostui.IconPill(c, "changes-prevent", "Pinned", frostui.Neutral)
	}
}
