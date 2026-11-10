package views

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/activity"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/fwupd"
)

// The helpers of updates_test.go and firmware_test.go start with "upd" and
// "fw" so that they cannot clash with other test files of the package.

// updPost runs posted functions on the test goroutine, as the UI thread.
type updPost struct {
	mu  sync.Mutex
	fns []func()
}

func (q *updPost) post(fn func()) {
	q.mu.Lock()
	q.fns = append(q.fns, fn)
	q.mu.Unlock()
}

// flush runs what was posted, and what that posts, until nothing is left.
func (q *updPost) flush() {
	for {
		q.mu.Lock()
		fns := q.fns
		q.fns = nil
		q.mu.Unlock()
		if len(fns) == 0 {
			return
		}
		for _, f := range fns {
			f()
		}
	}
}

// until runs posted functions until cond holds.
func (q *updPost) until(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		q.flush()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(time.Millisecond)
	}
}

// updEnv returns an Env whose updates_page enables groups, with an
// in-memory activity log, and the queue its posts go to.
func updEnv(t *testing.T, dryRun bool, groups ...string) (*Env, *updPost) {
	t.Helper()
	pc := config.PageConfig{}
	for _, g := range groups {
		pc[g] = config.GroupConfig{Enabled: true}
	}
	q := &updPost{}
	env := NewEnv(&config.Config{UpdatesPage: pc}, dryRun, q.post)
	env.Activity, _ = activity.Open("", dryRun)
	return env, q
}

// updFake is an update source with fixed items. Its jobs end at once,
// unless hold has a channel for the item, and drop their item when they
// work, as the real sources do.
type updFake struct {
	env                *Env
	id                 SourceID
	title, icon, blurb string
	status             SourceStatus
	items              []UpdateItem
	hold               map[string]chan struct{}
	fail               map[string]error
	banner             string

	refreshed int
	asked     []string
}

func (s *updFake) ID() SourceID  { return s.id }
func (s *updFake) Title() string { return s.title }
func (s *updFake) Icon() string  { return s.icon }
func (s *updFake) Blurb() string { return s.blurb }
func (s *updFake) Status() SourceStatus {
	st := s.status
	st.Available = true
	return st
}
func (s *updFake) Items() []UpdateItem { return s.items }
func (s *updFake) Refresh()            { s.refreshed++ }

func (s *updFake) Jobs(keys ...string) []*frostui.Job {
	var out []*frostui.Job
	for _, it := range s.items {
		if len(keys) == 0 && (it.Held || it.Confirm) {
			continue
		}
		if len(keys) > 0 && !slices.Contains(keys, it.Key) {
			continue
		}
		out = append(out, s.job(it))
	}
	return out
}

func (s *updFake) job(it UpdateItem) *frostui.Job {
	key, block, err := it.Key, s.hold[it.Key], s.fail[it.Key]
	return &frostui.Job{
		Title: "Updating " + it.Title, Source: string(s.id), Key: key,
		Run: func(ctx context.Context, _ *frostui.Progress) error {
			if block != nil {
				select {
				case <-block:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return err
		},
		Done: func(err error) {
			if err == nil {
				s.items = slices.DeleteFunc(slices.Clone(s.items), func(it UpdateItem) bool { return it.Key == key })
			}
		},
	}
}

func (s *updFake) Update(key string) {
	s.asked = append(s.asked, key)
	s.env.Jobs.Enqueue(s.Jobs(key)...)
}

func (s *updFake) Extra(c *ui.Context) {
	if s.banner != "" {
		frostui.Banner(c, frostui.Warning, "changes-prevent", s.banner, "Trust tap", true)
	}
}

// updFakes returns the four sources other files own, with the mockup's
// items, checked five minutes ago.
func updFakes(env *Env) (image, features, flat, brew *updFake) {
	checked := time.Now().Add(-5 * time.Minute)
	image = &updFake{env: env, id: SourceImage, title: "System image", icon: "drive-harddisk-system",
		blurb:  "Downloaded and staged with bootc. It applies on restart; this version stays as the rollback.",
		status: SourceStatus{CheckedAt: checked},
		items:  []UpdateItem{{Key: "snow", Title: "Snow Linux", Detail: "ghcr.io/frostyard/snow:stable", From: "2026.11.05", To: "2026.11.08", Action: "Stage"}}}
	features = &updFake{env: env, id: SourceFeatures, title: "Features", icon: "application-x-addon",
		blurb:  "System extensions, updated with updex and merged without a restart.",
		status: SourceStatus{CheckedAt: checked},
		items: []UpdateItem{
			{Key: "docker", Title: "docker", Detail: "Docker Engine", From: "28.5.1", To: "28.5.2", Action: "Update"},
			{Key: "devtools", Title: "devtools", Detail: "Developer tools", From: "2026.10", To: "2026.11", Action: "Update"},
		}}
	flat = &updFake{env: env, id: SourceFlatpak, title: "Flatpak", icon: "package-x-generic",
		blurb:  "Apps and runtimes from Flathub, for you and for everyone on this computer.",
		status: SourceStatus{CheckedAt: checked},
		items: []UpdateItem{
			{Key: "firefox", Title: "Firefox", Detail: "org.mozilla.firefox", From: "144.0", To: "144.0.2", Action: "Update"},
			{Key: "bazaar", Title: "Bazaar", Detail: "io.github.kolunmi.Bazaar", From: "0.6.1", To: "0.6.3", Action: "Update"},
			{Key: "platform", Title: "GNOME Platform", Detail: "org.gnome.Platform//49", From: "49.1", To: "49.2", Action: "Update"},
		}}
	brew = &updFake{env: env, id: SourceBrew, title: "Homebrew", icon: "utilities-terminal",
		blurb:  "Formulae and casks in /home/linuxbrew. Pinned formulae are left alone.",
		status: SourceStatus{CheckedAt: checked},
		banner: "Homebrew skips bluefin-cli because ublue-os/tap is not trusted. Trust the tap to update its packages again.",
		items: []UpdateItem{
			{Key: "neovim", Title: "neovim", Detail: "homebrew/core", From: "0.11.4", To: "0.11.5", Action: "Update"},
			{Key: "gh", Title: "gh", Detail: "homebrew/core", From: "2.82.0", To: "2.83.1", Action: "Update"},
			{Key: "ripgrep", Title: "ripgrep", Detail: "homebrew/core", From: "14.1.1", To: "15.0.0", Action: "Update"},
			{Key: "starship", Title: "starship", Detail: "homebrew/core", From: "1.23.0", To: "1.24.0", Action: "Update"},
			{Key: "bluefin-cli", Title: "bluefin-cli", Detail: "ublue-os/tap", From: "0.4.1", To: "0.5.0", Action: "Update", Held: true, HeldWhy: "Untrusted tap"},
		}}
	return image, features, flat, brew
}

// updSetup returns a page over the four fakes and a real firmware source
// over fake fwupd functions, loaded.
func updSetup(t *testing.T) (env *Env, q *updPost, p *updatesPage, srcs []UpdateSource, fakes []*updFake, fw *Firmware, ff *fwFake) {
	t.Helper()
	env, q = updEnv(t, false, "firmware_updates_group")
	image, features, flat, brew := updFakes(env)
	fw, ff = fwLoaded(t, env, q, fwDbx, fwDock, fwEC)
	srcs = []UpdateSource{image, features, flat, brew, fw.Source()}
	p = newUpdatesPage(env, func() []UpdateSource { return srcs })
	return env, q, p, srcs, []*updFake{image, features, flat, brew}, fw, ff
}

// updView draws page as the shell does, with the dialogs over it.
func updView(env *Env, page Page) func(c *ui.Context) {
	return func(c *ui.Context) {
		t := frostui.Adapt(c)
		ui.Column(c).Fill().Background(t.Background).AlignItems(ui.Stretch).Children(func() {
			ui.Scroll(c).Grow(1).Fill().Padding(frostui.SpaceXL, 28).Children(func() {
				ui.Column(c).Gap(18).AlignItems(ui.Stretch).Children(func() { page.View(c) })
			})
		})
		env.Dialogs.View(c)
	}
}

func updIcons(t *testing.T) {
	t.Helper()
	t.Setenv("FROSTUI_ICON_THEME", "Adwaita")
	frostui.SetIcons(frostui.NewIcons(nil))
}

// updKeys lists jobs as "source/key".
func updKeys(jobs []*frostui.Job) []string {
	out := make([]string, len(jobs))
	for i, j := range jobs {
		out[i] = j.Source + "/" + j.Key
	}
	return out
}

func updSteps(p *updatesPage, srcs []UpdateSource) map[string]frostui.StepState {
	out := map[string]frostui.StepState{}
	for _, src := range srcs {
		st, _, _ := p.stage(src, time.Now())
		for _, s := range st.Steps {
			out[s.Title] = s.State
		}
	}
	return out
}

func TestUpdateEverythingQueuesDefaultJobsInSourceOrder(t *testing.T) {
	env, q, p, _, fakes, _, ff := updSetup(t)
	release := make(chan struct{})
	fakes[0].hold = map[string]chan struct{}{"snow": release}
	p.updateEverything()
	want := []string{"image/snow", "features/docker", "features/devtools", "flatpak/firefox", "flatpak/bazaar",
		"flatpak/platform", "brew/neovim", "brew/gh", "brew/ripgrep", "brew/starship"}
	if got := updKeys(updatesBatch(env.Jobs)); !slices.Equal(got, want) {
		t.Errorf("queued %v\nwant  %v", got, want)
	}
	// While it runs, a second click queues nothing.
	p.updateEverything()
	if _, total := env.Jobs.Batch(); total != len(want) {
		t.Errorf("a second Update everything queued more: %d jobs", total)
	}
	if toasts := env.TakeToasts(); len(toasts) != 1 || toasts[0].Title != "Wait for the running job to finish" {
		t.Errorf("toasts = %+v", toasts)
	}
	close(release)
	q.until(t, func() bool { return !env.Jobs.Busy() })
	if len(ff.installs()) != 0 {
		t.Errorf("Update everything installed firmware: %v", ff.installs())
	}
	if items := fakes[3].Items(); len(items) != 1 || items[0].Key != "bluefin-cli" {
		t.Errorf("Homebrew left %v, want only the held bluefin-cli", items)
	}
	// Nothing default is left: firmware and the held formula stay.
	p.updateEverything()
	if toasts := env.TakeToasts(); len(toasts) != 1 || toasts[0].Title != "Nothing to update" {
		t.Errorf("toasts = %+v", toasts)
	}
}

func TestUpdatesPipelineFollowsTheJobs(t *testing.T) {
	env, q, p, srcs, fakes, _, _ := updSetup(t)
	flat := fakes[2]
	release := make(chan struct{})
	flat.hold = map[string]chan struct{}{"firefox": release}
	p.track(srcs) // the view tracks every frame from the start
	p.updateEverything()
	q.until(t, func() bool { cur := env.Jobs.Current(); return cur != nil && cur.Key == "firefox" })
	p.track(srcs)
	want := map[string]frostui.StepState{
		"Snow Linux": frostui.StepDone, "docker": frostui.StepDone, "devtools": frostui.StepDone,
		"Firefox": frostui.StepRunning, "Bazaar": frostui.StepQueued, "neovim": frostui.StepQueued,
		"bluefin-cli": frostui.StepHeld, "UEFI dbx": frostui.StepHeld, "Thunderbolt 4 Dock": frostui.StepHeld,
		"Embedded Controller": frostui.StepHeld,
	}
	got := updSteps(p, srcs)
	for title, state := range want {
		if got[title] != state {
			t.Errorf("step %s = %v, want %v", title, got[title], state)
		}
	}
	close(release)
	q.until(t, func() bool { return !env.Jobs.Busy() })
	p.track(srcs)
	// The sources dropped what they updated; the run still shows it.
	got = updSteps(p, srcs)
	for _, title := range []string{"Snow Linux", "Firefox", "GNOME Platform", "starship"} {
		if got[title] != frostui.StepDone {
			t.Errorf("after the run, step %s = %v, want done", title, got[title])
		}
	}
	if got["bluefin-cli"] != frostui.StepHeld {
		t.Errorf("held step = %v", got["bluefin-cli"])
	}
	if s := updatesRunSummary(p.run); s != "Last run: 10 done" {
		t.Errorf("summary = %q", s)
	}
	// Check now forgets the run.
	p.checkNow()
	p.track(srcs)
	if _, ok := updSteps(p, srcs)["Firefox"]; ok {
		t.Error("Check now kept the finished run's steps")
	}
	for _, f := range fakes {
		if f.refreshed != 1 {
			t.Errorf("%s refreshed %d times", f.id, f.refreshed)
		}
	}
}

func TestUpdatesPipelineShowsAFailure(t *testing.T) {
	env, q, p, srcs, fakes, _, _ := updSetup(t)
	fakes[1].fail = map[string]error{"docker": errors.New("signature mismatch")}
	p.track(srcs)
	p.updateEverything()
	q.until(t, func() bool { return !env.Jobs.Busy() })
	p.track(srcs)
	steps := updSteps(p, srcs)
	if steps["docker"] != frostui.StepFailed || steps["devtools"] != frostui.StepDone {
		t.Errorf("docker = %v, devtools = %v", steps["docker"], steps["devtools"])
	}
	if s := updatesRunSummary(p.run); s != "Last run: 9 done · 1 failed" {
		t.Errorf("summary = %q", s)
	}
	// Another page's job is not part of the Updates page's run.
	release := make(chan struct{})
	env.Jobs.Enqueue(&frostui.Job{Title: "Cleaning up", Source: "maintenance", Key: "cleanup",
		Run: func(context.Context, *frostui.Progress) error { <-release; return nil }})
	p.track(srcs)
	if len(p.run) != 0 {
		t.Errorf("the run kept another page's job: %v", updKeys(p.run))
	}
	close(release)
	q.until(t, func() bool { return !env.Jobs.Busy() })
}

func TestUpdatesWords(t *testing.T) {
	for action, want := range map[string][2]string{
		"Update": {"Updating", "Updated"}, "Stage": {"Staging", "Staged"}, "Install…": {"Installing", "Installed"},
		"Restart required": {"Updating", "Restart required"},
	} {
		if got := [2]string{updatesDoing(action), updatesDone(action)}; got != want {
			t.Errorf("%q: %v, want %v", action, got, want)
		}
	}
	for d, want := range map[time.Duration]string{8 * time.Second: "8s", 63 * time.Second: "1m 03s", 3723 * time.Second: "1h 02m"} {
		if got := updatesDuration(d); got != want {
			t.Errorf("updatesDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestUpdatesMergeKeepsPlaces(t *testing.T) {
	a, b, c, d := UpdateItem{Key: "a"}, UpdateItem{Key: "b", Title: "old"}, UpdateItem{Key: "c"}, UpdateItem{Key: "d"}
	b2 := UpdateItem{Key: "b", Title: "new"}
	got := updatesMerge([]UpdateItem{a, b, c}, []UpdateItem{d, b2}, func(key string) bool { return key == "a" })
	keys := []string{}
	for _, it := range got {
		keys = append(keys, it.Key+it.Title)
	}
	// a stays (the run updated it), b is replaced in place, c left, d is new.
	if want := []string{"a", "bnew", "d"}; !slices.Equal(keys, want) {
		t.Errorf("merge = %v, want %v", keys, want)
	}
}

func TestUpdatesRowsAndButtons(t *testing.T) {
	updIcons(t)
	env, q, p, _, fakes, _, _ := updSetup(t)
	brew := fakes[3]
	release := make(chan struct{})
	brew.hold = map[string]chan struct{}{"gh": release}
	tt := ui.NewTester(updView(env, p), 1128, 3000)
	for _, want := range []string{"Updates", "Update run", "System image", "Snow Linux", "2026.11.05", "2026.11.08",
		"Firefox", "bluefin-cli", "Thunderbolt 4 Dock", "Needs attention: lid-is-closed", brew.banner} {
		if !tt.HasText(want) {
			t.Errorf("missing %q", want)
		}
	}
	if b, ok := tt.AccessByRole(ui.RoleButton, "Update bluefin-cli"); !ok || !b.Disabled {
		t.Errorf("held row button = %+v, %v; want disabled", b, ok)
	}
	if b, ok := tt.AccessByRole(ui.RoleButton, "Install Embedded Controller"); !ok || !b.Disabled {
		t.Errorf("blocked firmware button = %+v, %v; want disabled", b, ok)
	}
	if err := tt.Click("Update gh"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(brew.asked, []string{"gh"}) {
		t.Errorf("asked = %v", brew.asked)
	}
	q.flush()
	tt.Frame()
	if !tt.HasText("Updating") {
		t.Errorf("the running row shows no progress: %q", tt.Texts())
	}
	if b, ok := tt.AccessByRole(ui.RoleButton, "Update everything"); !ok || !b.Disabled {
		t.Errorf("Update everything while busy = %+v, %v; want disabled", b, ok)
	}
	close(release)
	q.until(t, func() bool { return !env.Jobs.Busy() })
	tt.Frame()
	if !tt.HasText("Updated") {
		t.Errorf("the updated row shows no Updated pill: %q", tt.Texts())
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "Update gh"); ok {
		t.Error("the updated row still offers Update")
	}
}

func TestUpdatesFirmwareRowAsksFirst(t *testing.T) {
	updIcons(t)
	env, q, p, _, _, _, ff := updSetup(t)
	ff.setReport(fwupd.InstallReport{Result: fwupd.InstallFlashed, Updates: []fwupd.Update{fwDbx, fwEC}})
	tt := ui.NewTester(updView(env, p), 1128, 3000)
	if err := tt.Click("Install Thunderbolt 4 Dock"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Install firmware for Thunderbolt 4 Dock?") || env.Jobs.Busy() {
		t.Fatalf("no confirmation before the install: busy %v, %q", env.Jobs.Busy(), tt.Texts())
	}
	if err := tt.Click("Install"); err != nil {
		t.Fatal(err)
	}
	q.until(t, func() bool { return !env.Jobs.Busy() })
	tt.Frame()
	if got := ff.installs(); !slices.Equal(got, []string{fwDock.DeviceID}) {
		t.Errorf("installs = %v", got)
	}
	if !tt.HasText("Installed") {
		t.Errorf("the installed row shows no Installed pill: %q", tt.Texts())
	}
}

func TestUpdatesCheckNow(t *testing.T) {
	updIcons(t)
	env, q, p, _, fakes, _, ff := updSetup(t)
	tt := ui.NewTester(updView(env, p), 1128, 3000)
	calls := ff.listCount()
	if err := tt.Click("Check now"); err != nil {
		t.Fatal(err)
	}
	for _, f := range fakes {
		if f.refreshed != 1 {
			t.Errorf("%s refreshed %d times", f.id, f.refreshed)
		}
	}
	q.until(t, func() bool { return ff.listCount() == calls+1 })
	q.until(t, func() bool { return !p.sources()[4].Status().Checking })
	// While a source checks, Check now is off.
	fakes[2].status.Checking = true
	tt.Frame()
	if b, ok := tt.AccessByRole(ui.RoleButton, "Check now"); !ok || !b.Disabled {
		t.Errorf("Check now while checking = %+v, %v; want disabled", b, ok)
	}
	if !tt.HasText("Checking for updates…") {
		t.Errorf("the checking source says nothing: %q", tt.Texts())
	}
}

func TestUpdatesBadge(t *testing.T) {
	env, _, p, _, fakes, _, _ := updSetup(t)
	// 1 + 2 + 3 + 5 + 3 firmware devices, held ones included.
	if text, tone := p.Badge(); text != "14" || tone != frostui.Accent {
		t.Errorf("Badge = %q %v", text, tone)
	}
	empty := newUpdatesPage(env, func() []UpdateSource { return []UpdateSource{fakes[0]} })
	fakes[0].items = nil
	if text, _ := empty.Badge(); text != "" {
		t.Errorf("Badge with nothing = %q", text)
	}
}

func TestUpdatesSubtitle(t *testing.T) {
	_, _, p, srcs, fakes, _, _ := updSetup(t)
	now := time.Date(2026, 11, 10, 15, 30, 0, 0, time.Local)
	p.env.Now = func() time.Time { return now }
	for _, f := range fakes {
		f.status.CheckedAt = now.Add(-10 * time.Minute)
	}
	want := "12 ready, 2 held · checked today at 15:20 · staged system updates apply on restart"
	if got := p.subtitle(srcs); got != want {
		t.Errorf("subtitle = %q\nwant       %q", got, want)
	}
	fakes[1].status.Checking = true
	if got := p.subtitle(srcs); !strings.Contains(got, "checking now") {
		t.Errorf("subtitle while checking = %q", got)
	}
	if got := p.subtitle(srcs[1:2]); got != "2 ready · checking now" {
		t.Errorf("subtitle of features alone = %q", got)
	}
}

func TestUpdatesEmptyStates(t *testing.T) {
	updIcons(t)
	env, _ := updEnv(t, false)
	none := newUpdatesPage(env, func() []UpdateSource { return nil })
	tt := ui.NewTester(updView(env, none), 1128, 800)
	if !tt.HasText("No update source applies here") {
		t.Errorf("no empty state: %q", tt.Texts())
	}
	if text, _ := none.Badge(); text != "" {
		t.Errorf("Badge = %q", text)
	}

	image, features, _, _ := updFakes(env)
	image.items, features.items = nil, nil
	current := newUpdatesPage(env, func() []UpdateSource { return []UpdateSource{image, features} })
	tt = ui.NewTester(updView(env, current), 1128, 800)
	if !tt.HasText("Up to date") || tt.HasText("Update run") {
		t.Errorf("up-to-date page: %q", tt.Texts())
	}
	if b, ok := tt.AccessByRole(ui.RoleButton, "Update everything"); !ok || !b.Disabled {
		t.Errorf("Update everything with nothing to do = %+v, %v; want disabled", b, ok)
	}
	if !strings.HasPrefix(current.subtitle(current.sources()), "Everything is up to date") {
		t.Errorf("subtitle = %q", current.subtitle(current.sources()))
	}
}

func TestUpdatesCommands(t *testing.T) {
	updIcons(t)
	env, q, p, _, fakes, _, _ := updSetup(t)
	byTitle := map[string]frostui.Command{}
	for _, c := range p.Commands() {
		if _, dup := byTitle[c.Title]; dup {
			t.Errorf("duplicate command %q", c.Title)
		}
		byTitle[c.Title] = c
		if len(c.Keys) != 0 {
			t.Errorf("%q binds keys %v", c.Title, c.Keys)
		}
	}
	for _, want := range []string{"Update everything", "Stage the system update", "Update features", "Update Flatpak apps",
		"Update Homebrew packages", "Install firmware for UEFI dbx", "Install firmware for Thunderbolt 4 Dock"} {
		if _, ok := byTitle[want]; !ok {
			t.Errorf("no command %q", want)
		}
	}
	if _, ok := byTitle["Install firmware for Embedded Controller"]; ok {
		t.Error("a blocked device has an install command")
	}
	// A firmware command asks first.
	byTitle["Install firmware for UEFI dbx"].Run()
	if !env.Dialogs.Showing() || env.Jobs.Busy() {
		t.Errorf("firmware command: dialog %v, busy %v", env.Dialogs.Showing(), env.Jobs.Busy())
	}
	// A source command queues that source's default jobs only.
	release := make(chan struct{})
	fakes[3].hold = map[string]chan struct{}{"neovim": release}
	byTitle["Update Homebrew packages"].Run()
	if got := updKeys(updatesBatch(env.Jobs)); !slices.Equal(got, []string{"brew/neovim", "brew/gh", "brew/ripgrep", "brew/starship"}) {
		t.Errorf("Homebrew command queued %v", got)
	}
	for _, c := range p.Commands() {
		if c.Title == "Update everything" && !c.Disabled {
			t.Error("Update everything is enabled while jobs run")
		}
		if c.Title == "Update Homebrew packages" && !c.Disabled {
			t.Error("the Homebrew command is enabled while its jobs run")
		}
	}
	close(release)
	q.until(t, func() bool { return !env.Jobs.Busy() })
}

// TestSnapshotUpdates draws the page idle, running and done, light and
// dark, into $CHAIRLIFT_SNAPSHOT_DIR.
func TestSnapshotUpdates(t *testing.T) {
	updIcons(t)
	for _, dark := range []bool{false, true} {
		theme := "light"
		if dark {
			theme = "dark"
		}
		env, q, p, _, fakes, _, _ := updSetup(t)
		release := make(chan struct{})
		fakes[2].hold = map[string]chan struct{}{"bazaar": release}
		tt := ui.NewTesterWith(updView(env, p), 1128, 2300, ui.TesterOptions{Dark: dark})
		snapshot(t, tt, "updates-idle-"+theme)

		if err := tt.Click("Install UEFI dbx"); err != nil {
			t.Fatal(err)
		}
		snapshot(t, tt, "updates-firmware-confirm-"+theme)
		if err := tt.Click("Cancel"); err != nil {
			t.Fatal(err)
		}

		if err := tt.Click("Update everything"); err != nil {
			t.Fatal(err)
		}
		q.until(t, func() bool { cur := env.Jobs.Current(); return cur != nil && cur.Key == "bazaar" })
		updAge(updatesBatch(env.Jobs))
		tt.Frame()
		if !tt.HasText("Job 5 of 10") {
			t.Errorf("no batch progress: %q", tt.Texts())
		}
		snapshot(t, tt, "updates-running-"+theme)

		close(release)
		q.until(t, func() bool { return !env.Jobs.Busy() })
		tt.Frame() // the page keeps the finished batch
		updAge(p.run)
		tt.Frame()
		if !tt.HasText("Last run: 10 done") {
			t.Errorf("no run summary: %q", tt.Texts())
		}
		snapshot(t, tt, "updates-done-"+theme)
	}
}

// updJobTime is a plausible time for a job, for the snapshots.
func updJobTime(key string) time.Duration {
	return map[string]time.Duration{
		"snow": 63 * time.Second, "docker": 23 * time.Second, "devtools": 27 * time.Second, "firefox": 25 * time.Second,
		"bazaar": 14 * time.Second, "platform": 29 * time.Second, "neovim": 16 * time.Second, "gh": 13 * time.Second,
		"ripgrep": 11 * time.Second, "starship": 11 * time.Second,
	}[key]
}

// updAge backdates jobs so the snapshot shows plausible times. It runs on
// the test goroutine, the UI thread, where the queue writes these fields.
func updAge(jobs []*frostui.Job) {
	for _, j := range jobs {
		switch {
		case j.State.Ended():
			j.Started = j.Ended.Add(-updJobTime(j.Key))
		case j.State == frostui.JobRunning:
			j.Started = time.Now().Add(-9 * time.Second)
		}
	}
}
