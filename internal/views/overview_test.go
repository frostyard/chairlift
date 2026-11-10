package views

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/activity"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/navigation"
	"github.com/frostyard/chairlift/internal/sysinfo"
)

// ovSource is an update source with the items a test gives it.
type ovSource struct {
	id        SourceID
	title     string
	items     []UpdateItem
	st        SourceStatus
	refreshed int
}

func (s *ovSource) ID() SourceID                       { return s.id }
func (s *ovSource) Title() string                      { return s.title }
func (s *ovSource) Icon() string                       { return "package-x-generic" }
func (s *ovSource) Blurb() string                      { return "" }
func (s *ovSource) Status() SourceStatus               { return s.st }
func (s *ovSource) Items() []UpdateItem                { return s.items }
func (s *ovSource) Refresh()                           { s.refreshed++ }
func (s *ovSource) Jobs(keys ...string) []*frostui.Job { return nil }
func (s *ovSource) Update(key string)                  {}
func (s *ovSource) Extra(c *ui.Context)                {}

func ovFake(id SourceID, checked time.Time, items ...UpdateItem) *ovSource {
	return &ovSource{id: id, title: string(id), items: items, st: SourceStatus{Available: true, CheckedAt: checked}}
}

func ovItems(prefix string, n int, mod func(*UpdateItem)) []UpdateItem {
	var out []UpdateItem
	for i := 0; i < n; i++ {
		it := UpdateItem{Key: prefix + string(rune('a'+i)), Title: prefix + " " + string(rune('A'+i)), Action: "Update"}
		if mod != nil {
			mod(&it)
		}
		out = append(out, it)
	}
	return out
}

// ovIcons gives the kit Adwaita's icons and ChairLift's logo, without
// asking the desktop which theme it uses.
func ovIcons(t *testing.T) {
	t.Helper()
	t.Setenv("FROSTUI_ICON_THEME", "Adwaita")
	embedded := map[string][]byte{}
	if data, err := os.ReadFile(filepath.Join("..", "..", "data", "icons", "hicolor", "symbolic", "apps", "org.frostyard.ChairLift-symbolic.svg")); err == nil {
		embedded["org.frostyard.ChairLift-symbolic"] = data
	}
	frostui.SetIcons(frostui.NewIcons(embedded))
}

// ovStore is a Store whose System reads the fake, loaded.
func ovStore(t *testing.T, cfg *config.Config, dryRun bool, f *sysFake) (*Store, *overviewPage, *sysPosts) {
	t.Helper()
	ovIcons(t)
	env, q := sysEnv(t, cfg, dryRun)
	s := NewStore(env)
	f.install(s.System)
	s.System.Start()
	q.until(t, "the system load", func() bool { return s.System.loaded && !s.System.checking })
	p := s.Page(navigation.Overview).(*overviewPage)
	p.launch = func(string) error { return errors.New("no launch in tests") }
	return s, p, q
}

// ovView draws a page as the shell's content area does.
func ovView(p Page) func(c *ui.Context) {
	return func(c *ui.Context) {
		t := frostui.Adapt(c)
		ui.Scroll(c).Fill().Padding(frostui.SpaceXL, 28).Background(t.Background).Children(func() {
			ui.Column(c).Gap(18).AlignItems(ui.Stretch).Children(func() { p.View(c) })
		})
	}
}

func ovBootc(staged string) *sysFake {
	f := newSysFake()
	f.bootcHost, f.bootcScript = true, true
	f.status = sysBootcStatus("2026.11.05", staged, "2026.10.29")
	return f
}

func ovFillLive(l *Live) {
	l.history = true
	for i := 0; i < liveHistory; i++ {
		cpu := 18 + 6*math.Sin(float64(i)/4) + 3*math.Sin(float64(i)*1.7)
		if i > 50 {
			cpu += float64(i-50) * 3
		}
		used := uint64(8_200_000_000 + 150_000_000*math.Sin(float64(i)/9))
		l.apply(sysinfo.Sample{
			CPU:    cpu,
			Memory: sysinfo.Memory{Total: 16_000_000_000, Available: 16_000_000_000 - used},
			Disk:   sysinfo.Disk{Path: "/var/home/frosty", Free: 288_400_000_000, Total: 511_900_000_000},
		})
	}
}

func ovActivity(t *testing.T, env *Env) {
	t.Helper()
	entries := []activity.Entry{
		{At: sysFixedNow.AddDate(0, 0, -7), Source: "features", Title: "Turned on Developer tools", Detail: "devtools"},
		{At: sysFixedNow.AddDate(0, 0, -1).Add(-2 * time.Hour), Source: "image", Title: "Staged Snow Linux 2026.11.05", Detail: "ghcr.io/frostyard/snow:stable"},
		{At: sysFixedNow.AddDate(0, 0, -1), Source: "firmware", Title: "Updated System Firmware", Detail: "UEFI 1.38 on the system board"},
		{At: sysFixedNow.Add(-3 * time.Hour), Source: "brew", Title: "Upgraded 6 Homebrew packages", Detail: "gh, starship, fzf and 3 more"},
	}
	path := filepath.Join(t.TempDir(), "activity.jsonl")
	var b strings.Builder
	for _, e := range entries {
		line, _ := json.Marshal(e)
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	log, err := activity.Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	env.Activity = log
}

func ovHas(t *testing.T, tt *ui.Tester, want ...string) {
	t.Helper()
	for _, w := range want {
		if !tt.HasText(w) {
			t.Errorf("missing %q in %q", w, tt.Texts())
		}
	}
}

func ovHasNot(t *testing.T, tt *ui.Tester, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if tt.HasText(w) {
			t.Errorf("shows %q", w)
		}
	}
}

func TestHeroStateOrder(t *testing.T) {
	cases := []struct {
		name                  string
		updates, busy, staged bool
		tally                 updateTally
		want                  heroState
	}{
		{"jobs run", true, true, true, updateTally{runnable: 3}, heroWorking},
		{"jobs run without update sources", false, true, false, updateTally{}, heroWorking},
		{"updates ready", true, false, false, updateTally{runnable: 2, confirm: 1}, heroUpdate},
		{"updates beat a staged image", true, false, true, updateTally{runnable: 1}, heroUpdate},
		{"staged", true, false, true, updateTally{confirm: 2}, heroRestart},
		{"staged without update sources", false, false, true, updateTally{}, heroRestart},
		{"firmware only", true, false, false, updateTally{confirm: 2}, heroReview},
		{"held only", true, false, false, updateTally{held: 1}, heroReview},
		{"nothing", true, false, false, updateTally{}, heroUpToDate},
		{"no update sources", false, false, false, updateTally{runnable: 4}, heroNone},
	}
	for _, c := range cases {
		if got := heroStateOf(c.updates, c.busy, c.staged, c.tally); got != c.want {
			t.Errorf("%s: heroStateOf = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHeroTextSaysWhatWaits(t *testing.T) {
	now := sysFixedNow
	checked := now.Add(-12 * time.Minute)
	info := SystemInfo{OSName: "Snow Linux", Staged: Deployment{Present: true, Version: "2026.11.08"}}
	cases := []struct {
		state       heroState
		tally       updateTally
		title, note string
	}{
		{heroUpdate, updateTally{runnable: 12, checkedAt: checked}, "12 updates ready", "Checked 12 min ago"},
		{heroUpdate, updateTally{runnable: 1, confirm: 2, checkedAt: checked}, "1 update ready", "Checked 12 min ago · 2 firmware updates wait for you"},
		{heroUpdate, updateTally{runnable: 3, held: 1, checking: true}, "3 updates ready", "Checking for updates… · 1 update waits for you"},
		{heroRestart, updateTally{confirm: 2}, "Restart to finish", "Snow Linux 2026.11.08 is staged and applies when the computer restarts. 2 firmware updates wait for you."},
		{heroRestart, updateTally{}, "Restart to finish", "Snow Linux 2026.11.08 is staged and applies when the computer restarts."},
		{heroReview, updateTally{confirm: 1, checkedAt: checked}, "1 firmware update", "Each device updates after you confirm it · Checked 12 min ago"},
		{heroReview, updateTally{confirm: 1, held: 2, checkedAt: checked}, "3 updates wait", "Held packages and firmware wait for you · Checked 12 min ago"},
		{heroUpToDate, updateTally{checkedAt: now.AddDate(0, 0, -1)}, "Up to date", "Checked yesterday"},
		{heroUpToDate, updateTally{checkedAt: now.AddDate(0, 0, -6)}, "Up to date", "Checked on " + now.AddDate(0, 0, -6).Format("Mon")},
		{heroUpToDate, updateTally{}, "Not checked yet", "Check now asks every source what is new"},
		{heroUpToDate, updateTally{checking: true}, "Checking for updates", ""},
	}
	for _, c := range cases {
		title, note := heroText(c.state, info, c.tally, now)
		if title != c.title || note != c.note {
			t.Errorf("heroText(%v, %+v) = %q, %q; want %q, %q", c.state, c.tally, title, note, c.title, c.note)
		}
	}
}

func TestTallyCountsHeldAndConfirmApart(t *testing.T) {
	old, recent := sysFixedNow.Add(-time.Hour), sysFixedNow.Add(-time.Minute)
	busy := ovFake(SourceFlatpak, time.Time{}, ovItems("app", 3, nil)...)
	busy.st.Checking = true
	got := tallyUpdates([]UpdateSource{
		busy,
		ovFake(SourceBrew, recent, ovItems("pkg", 2, func(it *UpdateItem) { it.Held = true })...),
		ovFake(SourceFirmware, old, ovItems("dev", 1, func(it *UpdateItem) { it.Confirm = true })...),
		ovFake(SourceFeatures, recent),
	})
	want := updateTally{items: 6, sources: 3, runnable: 3, confirm: 1, held: 2, checking: true, checkedAt: old}
	if got != want {
		t.Errorf("tally = %+v, want %+v", got, want)
	}
}

func TestOverviewHeroRunsUpdateEverything(t *testing.T) {
	f := ovBootc("")
	f.stageLines = []string{"Pulling the image"}
	f.after = sysBootcStatus("2026.11.05", "2026.11.08", "2026.10.29")
	s, p, q := ovStore(t, sysConfig(sysAllGroups...), false, f)
	tt := ui.NewTester(ovView(p), 1360, 1500)

	ovHas(t, tt, "Overview", "frosty-x1 · Snow Linux, stable channel", "Snow Linux", "stable", "Secure Boot",
		"ghcr.io/frostyard/snow:stable · bootc · kernel 6.17.7-amd64", "2026.10.29", "2026.11.05", "nothing yet",
		"1 update ready", "Update everything", "Check now")
	if _, ok := tt.Access("Rollback: 2026.10.29, pinned"); !ok {
		t.Error("the pinned rollback is not named so")
	}

	if err := tt.Click("Update everything"); err != nil {
		t.Fatal(err)
	}
	if !s.Env.Jobs.Busy() {
		t.Fatal("Update everything queued nothing")
	}
	tt.Frame()
	ovHas(t, tt, "Working", "Staging the Snow Linux image")
	ovHasNot(t, tt, "Update everything")

	q.until(t, "the stage", func() bool { return !s.Env.Jobs.Busy() })
	tt.Frame()
	ovHas(t, tt, "Restart to finish", "2026.11.08",
		"Snow Linux 2026.11.08 is staged and applies when the computer restarts.")
	// ChairLift never restarts the computer.
	if _, ok := tt.AccessByRole(ui.RoleButton, "Restart…"); ok {
		t.Error("the hero offers a restart")
	}
	ovHasNot(t, tt, "Restart…", "Restart now", "nothing yet")
}

func TestOverviewCheckNowRefreshes(t *testing.T) {
	s, p, _ := ovStore(t, sysConfig(sysAllGroups...), false, ovBootc(""))
	tt := ui.NewTester(ovView(p), 1360, 1500)
	if err := tt.Click("Check now"); err != nil {
		t.Fatal(err)
	}
	if !s.System.checking {
		t.Error("Check now did not refresh the image source")
	}
	tt.Frame()
	if info, ok := tt.AccessByRole(ui.RoleButton, "Check now"); !ok || !info.Disabled {
		t.Errorf("Check now is not off while checking: %+v", info)
	}
}

func TestOverviewReviewFirmware(t *testing.T) {
	_, p, _ := ovStore(t, sysConfig(sysAllGroups...), false, newSysFake())
	fw := ovFake(SourceFirmware, sysFixedNow.Add(-12*time.Minute), ovItems("dev", 1, func(it *UpdateItem) { it.Confirm = true })...)
	p.sources = func() []UpdateSource { return []UpdateSource{fw} }
	var went string
	p.s.Env.SetNavigate(func(page string) { went = page })
	tt := ui.NewTester(ovView(p), 1360, 1500)
	ovHas(t, tt, "1 firmware update", "Review firmware")
	ovHasNot(t, tt, "Update everything")
	if err := tt.Click("Review firmware"); err != nil {
		t.Fatal(err)
	}
	if went != navigation.Updates {
		t.Errorf("Review firmware went to %q", went)
	}
}

func TestOverviewRestartWithFirmwareWaiting(t *testing.T) {
	_, p, _ := ovStore(t, sysConfig(sysAllGroups...), false, ovBootc("2026.11.08"))
	fw := ovFake(SourceFirmware, sysFixedNow, ovItems("dev", 2, func(it *UpdateItem) { it.Confirm = true })...)
	p.sources = func() []UpdateSource { return []UpdateSource{p.s.System.Source(), fw} }
	tt := ui.NewTester(ovView(p), 1360, 1500)
	ovHas(t, tt, "Restart to finish", "Review firmware",
		"Snow Linux 2026.11.08 is staged and applies when the computer restarts. 2 firmware updates wait for you.")
}

func TestOverviewUpToDate(t *testing.T) {
	_, p, _ := ovStore(t, sysConfig(sysAllGroups...), false, newSysFake())
	p.sources = func() []UpdateSource { return []UpdateSource{ovFake(SourceFlatpak, sysFixedNow.Add(-12*time.Minute))} }
	tt := ui.NewTester(ovView(p), 1360, 1500)
	ovHas(t, tt, "Up to date", "Checked 12 min ago", "Check now", "0", "all current")
	ovHasNot(t, tt, "Update everything", "Review firmware")
}

func TestOverviewFiguresAndCards(t *testing.T) {
	s, p, _ := ovStore(t, sysConfig(sysAllGroups...), false, ovBootc(""))
	ovFillLive(s.Live)
	ovActivity(t, s.Env)
	checked := sysFixedNow.Add(-12 * time.Minute)
	p.sources = func() []UpdateSource {
		return []UpdateSource{
			s.System.Source(),
			ovFake(SourceFeatures, checked, ovItems("feature", 2, nil)...),
			ovFake(SourceFlatpak, checked, ovItems("app", 5, nil)...),
			ovFake(SourceBrew, checked, ovItems("pkg", 2, func(it *UpdateItem) { it.Held = it.Key == "pkga" })...),
			ovFake(SourceFirmware, checked, ovItems("dev", 2, func(it *UpdateItem) { it.Confirm = true })...),
		}
	}
	tt := ui.NewTester(ovView(p), 1360, 1600)
	ovHas(t, tt,
		"Updates", "12", "across 5 sources", "Free space", "288 GB", "of 512 GB on /var/home/frosty",
		"9 updates ready",
		"Right now", "Processor and memory, last 60 seconds", "live", "Open Mission Center",
		"Disk", "224 GB used of 512 GB", "288 GB free",
		"System details", "Host name", "frosty-x1", "ThinkPad X1 Carbon Gen 12", "Home Url", "https://frostyard.org/",
		"Built", overviewBuilt("2026-11-05T10:19:52Z"), "Digest", "sha256:111111111111...",
		"Recent activity", "Upgraded 6 Homebrew packages", "3 h ago", "Yesterday",
	)
	// Pages whose summaries are not known show no figure.
	if _, ok := tt.Access("Problems: 0"); ok {
		t.Error("a Problems figure shows before the page loaded")
	}
	var went string
	s.Env.SetNavigate(func(page string) { went = page })
	if err := tt.Click("Updates: 12"); err != nil {
		t.Fatal(err)
	}
	if went != navigation.Updates {
		t.Errorf("the Updates figure went to %q", went)
	}
}

func TestOverviewGatesItsGroups(t *testing.T) {
	t.Run("nothing enabled", func(t *testing.T) {
		s, p, _ := ovStore(t, sysConfig(), false, ovBootc(""))
		tt := ui.NewTester(ovView(p), 1360, 1200)
		ovHas(t, tt, "Overview", "Recent activity", "Nothing yet")
		ovHasNot(t, tt, "Right now", "System details", "Update everything", "Check now", "Rollback", "Not checked yet")
		if cmds := p.Commands(); len(cmds) != 0 {
			t.Errorf("commands without groups: %+v", cmds)
		}
		if s.System.depsLoaded {
			t.Error("the deployments were read with every group off")
		}
	})
	t.Run("updates without the hero", func(t *testing.T) {
		_, p, _ := ovStore(t, sysConfig("updates_page/bootc_updates_group"), false, ovBootc(""))
		tt := ui.NewTester(ovView(p), 1360, 1200)
		// The hero's actions move to the page header.
		ovHas(t, tt, "Update everything", "Check now")
		ovHasNot(t, tt, "Rollback", "Right now", "System details", "1 update ready")
	})
	t.Run("hero and details without the health group", func(t *testing.T) {
		_, p, _ := ovStore(t, sysConfig("system_page/bootc_status_group", "system_page/system_info_group"), false, ovBootc(""))
		tt := ui.NewTester(ovView(p), 1360, 1400)
		ovHas(t, tt, "Snow Linux", "2026.11.05", "System details")
		ovHasNot(t, tt, "Right now", "Open Mission Center", "Update everything", "Check now")
		for _, cmd := range p.Commands() {
			if cmd.ID == "open-health-app" || cmd.ID == "update-everything" || cmd.ID == "stage-system-image" {
				t.Errorf("command %q without its group", cmd.ID)
			}
		}
	})
}

func TestOverviewOpensTheHealthApp(t *testing.T) {
	cfg := sysConfig(sysAllGroups...)
	cfg.SystemPage["health_group"] = config.GroupConfig{Enabled: true, AppID: "org.gnome.SystemMonitor"}
	s, p, _ := ovStore(t, cfg, false, newSysFake())
	var launched []string
	p.launch = func(id string) error {
		launched = append(launched, id)
		if len(launched) > 1 {
			return errors.New("gtk-launch: no such app")
		}
		return nil
	}
	tt := ui.NewTester(ovView(p), 1360, 1500)
	if err := tt.Click("Open System Monitor"); err != nil {
		t.Fatal(err)
	}
	if len(launched) != 1 || launched[0] != "org.gnome.SystemMonitor" {
		t.Errorf("launched %v", launched)
	}
	if toasts := s.Env.TakeToasts(); len(toasts) != 0 {
		t.Errorf("toasts after a launch: %+v", toasts)
	}
	var open *frostui.Command
	for _, cmd := range p.Commands() {
		if cmd.ID == "open-health-app" {
			open = &cmd
		}
	}
	if open == nil || open.Title != "Open System Monitor" {
		t.Fatalf("no palette command opens the monitor: %+v", open)
	}
	open.Run()
	if toasts := s.Env.TakeToasts(); len(toasts) != 1 || toasts[0].Title != "Failed to launch org.gnome.SystemMonitor" || toasts[0].Type != "error" {
		t.Errorf("toasts after a failed launch = %+v", toasts)
	}
}

func TestOverviewCommands(t *testing.T) {
	s, p, q := ovStore(t, sysConfig(sysAllGroups...), false, ovBootc(""))
	find := func(id string) *frostui.Command {
		for _, cmd := range p.Commands() {
			if cmd.ID == id {
				return &cmd
			}
		}
		return nil
	}
	if cmd := find("update-everything"); cmd == nil || cmd.Disabled {
		t.Errorf("update-everything = %+v", cmd)
	}
	stage := find("stage-system-image")
	if stage == nil || stage.Disabled {
		t.Fatalf("stage-system-image = %+v", stage)
	}
	stage.Run()
	if cur := s.Env.Jobs.Current(); cur == nil || cur.Key != imageItemKey {
		t.Fatalf("the stage command queued %+v", cur)
	}
	if cmd := find("update-everything"); cmd == nil || !cmd.Disabled || cmd.Reason != "a job is running" {
		t.Errorf("update-everything while busy = %+v", cmd)
	}
	q.until(t, "the stage", func() bool { return !s.Env.Jobs.Busy() })
	// The stage found nothing newer: nothing is left to update or stage.
	if cmd := find("update-everything"); cmd == nil || !cmd.Disabled || cmd.Reason != "nothing to update" {
		t.Errorf("update-everything with nothing to do = %+v", cmd)
	}
	if cmd := find("stage-system-image"); cmd != nil {
		t.Errorf("stage-system-image without an item = %+v", cmd)
	}
	if label, tone := p.Badge(); label != "" || tone != frostui.Neutral {
		t.Errorf("Badge() = %q, %v", label, tone)
	}
}

func TestOverviewDrawsBeforeAnythingLoads(t *testing.T) {
	ovIcons(t)
	env := NewEnv(&config.Config{}, true, func(fn func()) { fn() })
	s := NewStore(env)
	tt := ui.NewTester(ovView(s.Page(navigation.Overview)), 1360, 1200)
	ovHas(t, tt, "Overview", "This computer", "Recent activity")
	// With every group on, Homebrew's source counts finding brew as a
	// check, so the hero says it checks; it never claims "Up to date".
	if !tt.HasText("Not checked yet") && !tt.HasText("Checking for updates") {
		t.Errorf("the hero claims a state before anything loaded: %q", tt.Texts())
	}
	ovHasNot(t, tt, "Up to date")
}

func TestOverviewShowsOSReleaseFailure(t *testing.T) {
	f := newSysFake()
	delete(f.files, sysOSReleasePath)
	_, p, _ := ovStore(t, sysConfig("system_page/system_info_group"), false, f)
	tt := ui.NewTester(ovView(p), 1360, 1200)
	ovHas(t, tt, "System details", "Not available", "frosty-x1")
}

// ovShell draws the page in the frostui shell as the window does, for the
// snapshots.
func ovShell(s *Store, page Page) func(c *ui.Context) {
	var sh frostui.Shell
	sh.Brand = frostui.Brand{Name: "ChairLift", Tagline: "Snow Linux command center", Logo: frostui.IconSVG("org.frostyard.ChairLift-symbolic")}
	sh.Selected = navigation.Overview
	return func(c *ui.Context) {
		env := s.Env
		info := s.System.Info()
		var nav frostui.NavSection
		nav.Title = "System"
		for _, item := range navigation.VisibleItems(env.Enabled) {
			ni := frostui.NavItem{ID: item.Name, Title: item.Title, Icon: item.Icon}
			if item.Name == navigation.Updates {
				if n, _ := s.UpdateCount(); n > 0 {
					ni.Badge = "12"
				}
			}
			nav.Items = append(nav.Items, ni)
		}
		status := []frostui.StatusItem{
			{Icon: "security-high", Text: info.OSName + " " + info.Booted.Version, Progress: -1},
			{Icon: "media-playlist-shuffle", Text: info.Channel, Progress: -1},
			{Icon: "preferences-system-time", Text: info.Uptime, Progress: -1},
		}
		if info.Staged.Present {
			status = append(status, frostui.StatusItem{Icon: "system-reboot", Text: info.Staged.Version + " staged", Tone: frostui.Accent, Progress: -1})
		}
		var end []frostui.StatusItem
		if cur := env.Jobs.Current(); cur != nil {
			end = append(end, frostui.StatusItem{Text: cur.Title, Progress: max(env.Jobs.BatchFraction(), 0.01)})
		}
		end = append(end, frostui.StatusItem{Icon: "drive-harddisk", Text: sysinfo.FormatBytes(s.Live.Latest().Disk.Free) + " free", Progress: -1})
		sh.View(c, frostui.Frame{
			Nav:     []frostui.NavSection{nav},
			Content: func() { page.View(c) },
			RailFooter: func() {
				frostui.JobCard(c, env.Jobs)
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Avatar(c, info.Hostname, nil).Size(32, 32)
					ui.Column(c).Children(func() {
						ui.Text(c, info.Hostname).Bold().FontSize(13)
						frostui.Caption(c, info.Model)
					})
				})
			},
			Status: status, StatusEnd: end, ConsoleData: env.Console,
		})
	}
}

func TestSnapshotOverview(t *testing.T) {
	if os.Getenv("CHAIRLIFT_SNAPSHOT_DIR") == "" {
		t.Skip("CHAIRLIFT_SNAPSHOT_DIR is not set")
	}
	build := func(t *testing.T, staged string) (*Store, *overviewPage, *sysPosts) {
		f := ovBootc(staged)
		s, p, q := ovStore(t, sysConfig(sysAllGroups...), false, f)
		ovFillLive(s.Live)
		ovActivity(t, s.Env)
		checked := sysFixedNow.Add(-12 * time.Minute)
		features := ovFake(SourceFeatures, checked, ovItems("feature", 2, nil)...)
		flatpak := ovFake(SourceFlatpak, checked, ovItems("app", 5, nil)...)
		brew := ovFake(SourceBrew, checked, ovItems("pkg", 2, func(it *UpdateItem) { it.Held = it.Key == "pkga" })...)
		firmware := ovFake(SourceFirmware, checked, ovItems("dev", 2, func(it *UpdateItem) { it.Confirm = true })...)
		p.sources = func() []UpdateSource {
			if staged != "" {
				return []UpdateSource{s.System.Source(), firmware}
			}
			return []UpdateSource{s.System.Source(), features, flatpak, brew, firmware}
		}
		return s, p, q
	}
	for _, dark := range []bool{true, false} {
		suffix := "-light"
		if dark {
			suffix = "-dark"
		}
		t.Run("full"+suffix, func(t *testing.T) {
			s, p, _ := build(t, "")
			tt := ui.NewTesterWith(ovShell(s, p), 1440, 1500, ui.TesterOptions{Dark: dark})
			snapshot(t, tt, "overview-full"+suffix)
		})
		t.Run("running"+suffix, func(t *testing.T) {
			s, p, q := build(t, "")
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			quick := func(title string) *frostui.Job {
				return &frostui.Job{Source: "flatpak", Key: title, Title: title, Run: func(context.Context, *frostui.Progress) error { return nil }}
			}
			s.Env.Jobs.Enqueue(quick("Updating Firefox"), quick("Updating Builder"))
			s.Env.Jobs.Enqueue(&frostui.Job{Source: "features", Key: "devtools", Title: "Updating devtools",
				Run: func(ctx context.Context, _ *frostui.Progress) error {
					select {
					case <-release:
					case <-ctx.Done():
					}
					return nil
				}})
			for i := 0; i < 7; i++ {
				s.Env.Jobs.Enqueue(quick("queued " + string(rune('a'+i))))
			}
			q.until(t, "the third job", func() bool { cur := s.Env.Jobs.Current(); return cur != nil && cur.Key == "devtools" })
			s.Env.Jobs.Current().Fraction = 0.7
			s.Env.Jobs.Current().Status = "Downloading devtools_2026.11_x86-64.raw (118 MB)"
			tt := ui.NewTesterWith(ovShell(s, p), 1440, 1000, ui.TesterOptions{Dark: dark})
			snapshot(t, tt, "overview-running"+suffix)
		})
		t.Run("done"+suffix, func(t *testing.T) {
			s, p, _ := build(t, "2026.11.08")
			tt := ui.NewTesterWith(ovShell(s, p), 1440, 1000, ui.TesterOptions{Dark: dark})
			snapshot(t, tt, "overview-done"+suffix)
		})
	}
	t.Run("narrow", func(t *testing.T) {
		s, p, _ := build(t, "")
		tt := ui.NewTesterWith(ovShell(s, p), 1024, 1300, ui.TesterOptions{Dark: true})
		snapshot(t, tt, "overview-narrow-dark")
	})
}
