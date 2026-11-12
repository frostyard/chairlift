package views

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/navigation"
	"github.com/frostyard/frostui"
	"github.com/frostyard/frostui/frostuitest"
)

// appsBrew is a Homebrew inventory as Brew.Apps would give it.
func appsBrew() []AppEntry {
	formula := func(name, version, update string, pinned bool) AppEntry {
		e := AppEntry{Key: "formula:" + name, Kind: AppFormula, Name: name, ID: name, Version: version, Origin: "homebrew/core", Update: update, Pinned: pinned}
		if update != "" {
			e.Actions = append(e.Actions, ActUpdate)
		}
		if pinned {
			e.Actions = append(e.Actions, ActUnpin)
		} else {
			e.Actions = append(e.Actions, ActPin)
		}
		e.Actions = append(e.Actions, ActUninstall)
		return e
	}
	cask := func(name, id, version string) AppEntry {
		return AppEntry{Key: "cask:" + id, Kind: AppCask, Name: name, ID: id, Version: version, Origin: "homebrew/cask", Actions: []AppAction{ActUninstall}}
	}
	return []AppEntry{
		formula("eza", "0.23.4", "", false),
		formula("fzf", "0.66.0", "", false),
		formula("gh", "2.82.0", "2.83.0", false),
		formula("go", "1.25.3", "", true),
		formula("neovim", "0.11.4", "0.11.5", false),
		cask("Inter", "font-inter", "4.1"),
		cask("JetBrains Mono Nerd Font", "font-jetbrains-mono-nerd-font", "3.4.0"),
	}
}

// appsFixture is an Apps page over a Flatpak domain on fake backends and a
// fake Homebrew inventory; it records the actions the inspector sends.
type appsFixture struct {
	page   *appsPage
	env    *Env
	loop   *frostuitest.Loop
	fake   *fpFake
	brew   []AppEntry
	brewSt AppsStatus
	acts   []string
}

func newAppsFixture(t *testing.T, cfg *config.Config, withBrew bool) *appsFixture {
	t.Helper()
	fake := fpSample()
	fake.system = append(fake.system,
		fpApp("Extension Manager", "com.mattjakeman.ExtensionManager", "0.6.3", "flathub", false),
		fpApp("Mission Center", "io.missioncenter.MissionCenter", "1.1.0", "flathub", false),
	)
	p := frostuitest.NewLoop()
	env := NewEnv(cfg, false, p.Post)
	s := NewStore(env)
	fake.wire(s.Flatpak, false)
	s.Flatpak.Start()
	p.RunUntil(t, func() bool { return !s.Flatpak.appsLoading && !s.Flatpak.updatesChecking })
	fx := &appsFixture{env: env, loop: p, fake: fake}
	if withBrew {
		fx.brew, fx.brewSt = appsBrew(), AppsStatus{Available: true}
	}
	page := newAppsPage(s)
	page.brewApps = func() []AppEntry { return fx.brew }
	page.brewStatus = func() AppsStatus { return fx.brewSt }
	page.actFlatpak = func(key string, a AppAction) { fx.acts = append(fx.acts, "flatpak "+key+" "+appsActionName(a)) }
	page.actBrew = func(key string, a AppAction) { fx.acts = append(fx.acts, "brew "+key+" "+appsActionName(a)) }
	page.bundles = func() bool { return false }
	page.search = func() bool { return false }
	fx.page = page
	return fx
}

func appsActionName(a AppAction) string {
	return [...]string{"update", "open", "pin", "unpin", "uninstall"}[a]
}

// appsConfig enables the Flatpak groups and the software center.
func appsConfig(center bool, appID string) *config.Config {
	cfg := fpConfig(true, true, true)
	cfg.ApplicationsPage["applications_installed_group"] = config.GroupConfig{Enabled: center, AppID: appID}
	return cfg
}

func appsTester(p Page, dark bool) *ui.Tester {
	return ui.NewTesterWith(func(c *ui.Context) {
		frostui.Adapt(c)
		ui.Column(c).Fill().Padding(24, 28).Gap(18).AlignItems(ui.Stretch).Background(c.Theme().Background).Children(func() { p.View(c) })
	}, 1360, 880, ui.TesterOptions{Dark: dark})
}

// appsShows reports whether tt shows exactly s (HasText matches a part).
func appsShows(tt *ui.Tester, s string) bool { return slices.Contains(tt.Texts(), s) }

func TestAppsFilterAndSort(t *testing.T) {
	rows := append(appsBrew(), AppEntry{Key: "flatpak:system:org.mozilla.firefox", Kind: AppFlatpakSystem, Name: "Firefox", ID: "org.mozilla.firefox", Version: "144.0"},
		AppEntry{Key: "flatpak:user:org.blender.Blender", Kind: AppFlatpakUser, Name: "Blender", ID: "org.blender.Blender", Version: "4.5.3"})
	names := func(es []AppEntry) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.Name)
		}
		return out
	}
	for _, tc := range []struct {
		filter, query string
		want          []string
	}{
		{appsShowAll, "", []string{"Blender", "eza", "Firefox", "fzf", "gh", "go", "Inter", "JetBrains Mono Nerd Font", "neovim"}},
		{appsShowFlatpak, "", []string{"Blender", "Firefox"}},
		{appsShowHomebrew, "", []string{"eza", "fzf", "gh", "go", "Inter", "JetBrains Mono Nerd Font", "neovim"}},
		{appsShowCasks, "", []string{"Inter", "JetBrains Mono Nerd Font"}},
		{appsShowAll, "MOZILLA", []string{"Firefox"}},
		{appsShowAll, " font-", []string{"Inter", "JetBrains Mono Nerd Font"}},
		{appsShowCasks, "firefox", nil},
	} {
		got := names(appsSort(appsFilter(rows, tc.filter, tc.query), ui.SortOrder{Column: appsColApp}))
		if !slices.Equal(got, tc.want) {
			t.Errorf("filter %q query %q = %v, want %v", tc.filter, tc.query, got, tc.want)
		}
	}

	bySource := names(appsSort(rows, ui.SortOrder{Column: appsColSource}))
	if bySource[0] != "Firefox" || bySource[1] != "Blender" || bySource[len(bySource)-1] != "JetBrains Mono Nerd Font" {
		t.Errorf("by source = %v", bySource)
	}
	byVersionDesc := names(appsSort(rows, ui.SortOrder{Column: appsColVersion, Descending: true}))
	if byVersionDesc[0] != "Firefox" || byVersionDesc[len(byVersionDesc)-1] != "neovim" {
		t.Errorf("by version, descending = %v", byVersionDesc)
	}
	if got := names(appsSort(rows, ui.SortOrder{Column: appsColApp, Descending: true})); got[0] != "neovim" {
		t.Errorf("by name, descending = %v", got)
	}
}

func TestAppsNaturalCompare(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"1.9", "1.10", -1},
		{"2.82.0", "2.82.0", 0},
		{"0.66.0", "0.6.1", 1},
		{"gh", "GIMP", -1},
		{"v1.02", "v1.2", 0},
		{"", "1", -1},
		{"1.0", "1.0-beta", -1},
	} {
		if got := appsNaturalCompare(tc.a, tc.b); got != tc.want {
			t.Errorf("appsNaturalCompare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestAppsHelpers(t *testing.T) {
	for in, want := range map[string]string{"io.github.kolunmi.Bazaar": "Bazaar", "org.gnome.Software": "Software", "org.kde.discover": "Discover", "plain": "plain"} {
		if got := appsCenterName(in); got != want {
			t.Errorf("appsCenterName(%q) = %q, want %q", in, got, want)
		}
	}
	both := AppsStatus{Available: true}
	entries := append(appsBrew(), AppEntry{Kind: AppFlatpakUser})
	if got := appsSubtitle(entries, both, both); got != "8 installed: 1 Flatpak app and 7 Homebrew packages" {
		t.Errorf("subtitle = %q", got)
	}
	if got := appsSubtitle(entries[7:], both, AppsStatus{}); got != "1 Flatpak app installed" {
		t.Errorf("subtitle = %q", got)
	}
	if got := appsSubtitle(nil, AppsStatus{Available: true, Loading: true}, AppsStatus{}); got != "Looking for installed apps…" {
		t.Errorf("subtitle = %q", got)
	}
}

func TestAppsPageShowsEveryKindAndTheChosenApp(t *testing.T) {
	fx := newAppsFixture(t, appsConfig(true, ""), true)
	tt := appsTester(fx.page, false)
	for _, want := range []string{
		"Apps", "13 installed: 6 Flatpak apps and 7 Homebrew packages", "All", "Flatpak", "Homebrew", "Casks",
		"App", "Source", "Version", "Bazaar", "io.github.kolunmi.Bazaar", "Formula", "Cask", "Flatpak (you)",
		"Update", "Pinned", "Pick an app", "Open Bazaar",
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q", want)
		}
	}
	if appsShows(tt, "Bundles") || appsShows(tt, "Find more") || appsShows(tt, "Installed") {
		t.Error("tabs show while Homebrew offers neither bundles nor search")
	}

	// Choosing a Flatpak row fills the inspector; its buttons go to Flatpak.
	if err := tt.Click("org.mozilla.firefox"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Update to 144.0.2", "Open", "Uninstall", "flathub", "Update to"} {
		if !tt.HasText(want) {
			t.Errorf("inspector misses %q: %q", want, tt.Texts())
		}
	}
	for _, label := range []string{"Update to 144.0.2", "Open", "Uninstall"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	// A formula goes to Homebrew.
	if err := tt.Click("go"); err != nil {
		t.Fatal(err)
	}
	if !appsShows(tt, "Unpin version") || appsShows(tt, "Open") {
		t.Errorf("formula inspector = %q", tt.Texts())
	}
	if err := tt.Click("Unpin version"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"flatpak flatpak:system:org.mozilla.firefox update",
		"flatpak flatpak:system:org.mozilla.firefox open",
		"flatpak flatpak:system:org.mozilla.firefox uninstall",
		"brew formula:go unpin",
	}
	if !slices.Equal(fx.acts, want) {
		t.Errorf("acts = %q, want %q", fx.acts, want)
	}
}

func TestAppsPageFiltersAndSorts(t *testing.T) {
	fx := newAppsFixture(t, appsConfig(false, ""), true)
	tt := appsTester(fx.page, false)
	if err := tt.Click("Casks"); err != nil {
		t.Fatal(err)
	}
	if !appsShows(tt, "Inter") || appsShows(tt, "Firefox") || appsShows(tt, "eza") {
		t.Errorf("Casks shows %q", tt.Texts())
	}
	if err := tt.Click("All"); err != nil {
		t.Fatal(err)
	}
	fx.page.query = "blend"
	tt.Frame()
	if !appsShows(tt, "Blender") || appsShows(tt, "Firefox") {
		t.Errorf("filter shows %q", tt.Texts())
	}
	fx.page.query = "zzz"
	tt.Frame()
	if !tt.HasText("No apps match") {
		t.Error("no empty state for a filter that matches nothing")
	}
	fx.page.query = ""
	tt.Frame()

	// The header sorts.
	if err := tt.Click("Version"); err != nil {
		t.Fatal(err)
	}
	if fx.page.sort.Column != appsColVersion || fx.page.sort.Descending {
		t.Errorf("sort = %+v", fx.page.sort)
	}
	if err := tt.Click("Version"); err != nil {
		t.Fatal(err)
	}
	if !fx.page.sort.Descending {
		t.Errorf("a second click did not reverse: %+v", fx.page.sort)
	}
}

func TestAppsPageWithFlatpakOnly(t *testing.T) {
	fx := newAppsFixture(t, appsConfig(false, ""), false)
	tt := appsTester(fx.page, false)
	if !tt.HasText("6 Flatpak apps installed") {
		t.Errorf("subtitle missing: %q", tt.Texts())
	}
	// One kind needs no kind filter, and the center's group is off.
	if appsShows(tt, "Casks") || appsShows(tt, "Homebrew") || appsShows(tt, "All") || appsShows(tt, "Open Bazaar") {
		t.Errorf("unexpected controls: %q", tt.Texts())
	}
	cmds := fx.page.Commands()
	var ids []string
	for _, c := range cmds {
		ids = append(ids, c.ID)
	}
	if !slices.Equal(ids, []string{"apps-refresh", "apps-show-flatpak"}) {
		t.Errorf("commands = %v", ids)
	}
}

func TestAppsPageLoadingErrorsAndEmpty(t *testing.T) {
	fx := newAppsFixture(t, appsConfig(true, ""), true)
	fx.page.flatpakStatus = func() AppsStatus { return AppsStatus{Available: true, Loading: true} }
	fx.page.flatpakApps = func() []AppEntry { return nil }
	fx.brewSt = AppsStatus{Available: true, Err: "brew list failed"}
	fx.brew = nil
	tt := appsTester(fx.page, false)
	for _, want := range []string{"Loading Flatpak apps…", "Homebrew: brew list failed", "Looking for installed apps…"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if appsShows(tt, "No apps installed") {
		t.Error("empty state while Flatpak loads")
	}
	fx.page.flatpakStatus = func() AppsStatus { return AppsStatus{Available: true} }
	fx.brewSt = AppsStatus{Available: true}
	tt.Frame()
	if !tt.HasText("No apps installed") || !tt.HasText("Find apps to install with Bazaar.") {
		t.Errorf("no empty state: %q", tt.Texts())
	}
}

func TestAppsPageTabs(t *testing.T) {
	fx := newAppsFixture(t, appsConfig(false, ""), true)
	drawn := ""
	fx.page.bundles = func() bool { return true }
	fx.page.search = func() bool { return true }
	fx.page.drawBundles = func(c *ui.Context) { drawn = "bundles"; ui.Text(c, "the bundles") }
	fx.page.drawSearch = func(c *ui.Context) { drawn = "search"; ui.Text(c, "the search") }
	tt := appsTester(fx.page, false)
	for _, want := range []string{"Installed", "Bundles", "Find more"} {
		if !tt.HasText(want) {
			t.Errorf("missing tab %q", want)
		}
	}
	if err := tt.Click("Bundles"); err != nil {
		t.Fatal(err)
	}
	if drawn != "bundles" || !appsShows(tt, "the bundles") || appsShows(tt, "Firefox") {
		t.Errorf("Bundles tab = %q", tt.Texts())
	}
	if err := tt.Click("Find more"); err != nil {
		t.Fatal(err)
	}
	if drawn != "search" || !tt.HasText("the search") {
		t.Errorf("Find more tab = %q", tt.Texts())
	}

	// Without Flatpak or Homebrew lists, Installed goes and the first
	// remaining tab shows alone, without a tab bar.
	fx.page.flatpakStatus = func() AppsStatus { return AppsStatus{} }
	fx.brewSt = AppsStatus{}
	fx.page.search = func() bool { return false }
	fx.page.tab = appsTabInstalled
	tt.Frame()
	if appsShows(tt, "Installed") || appsShows(tt, "Find more") || !appsShows(tt, "the bundles") {
		t.Errorf("bundles alone = %q", tt.Texts())
	}
}

func TestAppsPageSoftwareCenter(t *testing.T) {
	fx := newAppsFixture(t, appsConfig(true, "org.gnome.Software"), false)
	tt := appsTester(fx.page, false)
	if err := tt.Click("Open Software"); err != nil {
		t.Fatal(err)
	}
	// The launch runs on a goroutine.
	fx.loop.RunUntil(t, func() bool { return len(fx.fake.launches()) == 1 })
	if got := fx.fake.launches(); !slices.Equal(got, []string{"org.gnome.Software"}) {
		t.Errorf("launched = %v", got)
	}
	var cmd *frostui.Command
	for _, c := range fx.page.Commands() {
		if c.ID == "apps-open-center" {
			cmd = &c
		}
	}
	if cmd == nil || cmd.Title != "Open Software" {
		t.Fatalf("commands = %+v", fx.page.Commands())
	}
	cmd.Run()
	fx.loop.RunUntil(t, func() bool { return len(fx.fake.launches()) == 2 })
	fx.loop.Settle(10 * time.Millisecond)
	if toasts := fx.env.TakeToasts(); len(toasts) != 0 {
		t.Errorf("toasts after two launches: %+v", toasts)
	}

	// The center alone: no group lists apps.
	cfg := fpConfig(false, false, false)
	cfg.ApplicationsPage["applications_installed_group"] = config.GroupConfig{Enabled: true}
	alone := newAppsFixture(t, cfg, false)
	tt = appsTester(alone.page, false)
	if !appsShows(tt, "Open Bazaar") || !appsShows(tt, "Find and install apps with Bazaar.") || appsShows(tt, "Installed") {
		t.Errorf("center alone = %q", tt.Texts())
	}
}

func TestAppsShowCommandsFilterAndNavigate(t *testing.T) {
	fx := newAppsFixture(t, appsConfig(false, ""), true)
	var went string
	fx.env.SetNavigate(func(page string) { went = page })
	fx.page.filter, fx.page.query, fx.page.tab = appsShowCasks, "x", appsTabSearch
	for _, c := range fx.page.Commands() {
		if c.ID == "apps-show-homebrew" {
			c.Run()
		}
	}
	if went != navigation.Apps || fx.page.filter != appsShowHomebrew || fx.page.query != "" || fx.page.tab != appsTabInstalled {
		t.Errorf("went %q, filter %q, query %q, tab %q", went, fx.page.filter, fx.page.query, fx.page.tab)
	}
	if label, _ := fx.page.Badge(); label != "" {
		t.Errorf("badge = %q", label)
	}
}

func TestAppsPageBusyRowDisablesChanges(t *testing.T) {
	fx := newAppsFixture(t, appsConfig(false, ""), false)
	fx.page.flatpakApps = func() []AppEntry {
		return []AppEntry{{Key: "flatpak:system:org.a.B", Kind: AppFlatpakSystem, Name: "B", ID: "org.a.B", Busy: true, Actions: []AppAction{ActOpen, ActUninstall}}}
	}
	tt := appsTester(fx.page, false)
	if err := tt.Click("org.a.B"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Working…") {
		t.Errorf("busy row shows no work: %q", tt.Texts())
	}
	_ = tt.Click("Uninstall")
	_ = tt.Click("Open")
	if !slices.Equal(fx.acts, []string{"flatpak flatpak:system:org.a.B open"}) {
		t.Errorf("acts = %q", fx.acts)
	}
}

// TestSnapshotApps draws the page as the mockup shows it, in light and
// dark, for a person to compare; it also checks that the page fits.
func TestSnapshotApps(t *testing.T) {
	t.Setenv("FROSTUI_ICON_THEME", "Adwaita")
	frostui.SetIcons(frostui.NewIcons(nil))
	defer frostui.SetIcons(nil)
	for _, dark := range []bool{false, true} {
		fx := newAppsFixture(t, appsConfig(true, ""), true)
		fx.page.bundles = func() bool { return true }
		fx.page.search = func() bool { return true }
		fx.page.chosen = "flatpak:system:org.mozilla.firefox"
		var shell frostui.Shell
		shell.Brand = frostui.Brand{Name: "ChairLift", Tagline: "Snow Linux command center"}
		shell.Selected = navigation.Apps
		nav := []frostui.NavSection{{Title: "System", Items: []frostui.NavItem{
			{ID: navigation.Overview, Title: "Overview", Icon: "view-grid"},
			{ID: navigation.Updates, Title: "Updates", Icon: "software-update-available", Badge: "4"},
			{ID: navigation.Apps, Title: "Apps", Icon: "package-x-generic"},
		}}}
		tt := ui.NewTesterWith(func(c *ui.Context) {
			shell.View(c, frostui.Frame{Nav: nav, Content: func() { fx.page.View(c) }})
		}, 1440, 920, ui.TesterOptions{Dark: dark})
		if !tt.HasText("Update to 144.0.2") {
			t.Errorf("the chosen app is not in the inspector: %q", tt.Texts())
		}
		// Thirteen rows fit the window: the last one is built.
		if !appsShows(tt, "neovim") {
			t.Error("the table is too short for its rows")
		}
		name := "apps-light"
		if dark {
			name = "apps-dark"
		}
		snapshot(t, tt, name)

		// A job on Firefox, Homebrew failing, Flatpak loading again.
		apps := fx.page.flatpakApps
		fx.page.flatpakApps = func() []AppEntry {
			es := apps()
			for i := range es {
				es[i].Busy = es[i].ID == "org.mozilla.firefox"
			}
			return es
		}
		fx.brewSt = AppsStatus{Available: true, Err: "brew list --versions: exit status 1"}
		fx.brew = nil
		tt.Frame()
		if !tt.HasText("Working…") || !tt.HasText("Homebrew: brew list") {
			t.Errorf("states = %q", tt.Texts())
		}
		snapshot(t, tt, name+"-states")
	}
}

func TestAppsSearchCommandOpensFindMore(t *testing.T) {
	fx := newAppsFixture(t, appsConfig(false, ""), true)
	fx.page.search = func() bool { return true }
	fx.page.drawSearch = func(c *ui.Context) { ui.Text(c, "the search") }
	requested := false
	fx.page.searchRequest = func() bool { r := requested; requested = false; return r }
	tt := appsTester(fx.page, false)
	if appsShows(tt, "the search") {
		t.Fatal("Find more shows before the command ran")
	}
	requested = true
	tt.Frame()
	if !appsShows(tt, "the search") {
		t.Errorf("the search command did not open Find more: %q", tt.Texts())
	}
}
