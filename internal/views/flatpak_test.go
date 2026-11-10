package views

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/frostui/frostuitest"
	"github.com/frostyard/chairlift/internal/views/flatpakstatus"
)

// fpConfig enables exactly the Flatpak groups asked for, and turns the
// Homebrew groups off.
func fpConfig(user, system, updates bool) *config.Config {
	return &config.Config{
		ApplicationsPage: config.PageConfig{
			"flatpak_user_group":           {Enabled: user},
			"flatpak_system_group":         {Enabled: system},
			"applications_installed_group": {Enabled: false},
			"brew_group":                   {Enabled: false},
			"brew_search_group":            {Enabled: false},
			"brew_bundles_group":           {Enabled: false},
		},
		UpdatesPage: config.PageConfig{
			"flatpak_updates_group": {Enabled: updates},
			"brew_updates_group":    {Enabled: false},
		},
	}
}

// fpFake is a Flatpak backend in memory. Every call is counted; the lists
// may block on a gate to order overlapping loads.
type fpFake struct {
	mu         sync.Mutex
	present    bool
	user       []flatpak.Application
	system     []flatpak.Application
	userUpd    []flatpak.UpdateInfo
	systemUpd  []flatpak.UpdateInfo
	userErr    error
	systemErr  error
	updUserErr error
	updSysErr  error
	updateErr  error
	removeErr  error
	launchErr  error

	appLists, updateLists int
	updated, removed      []string
	launched              []string
	gate                  chan struct{} // when set, an update check waits on it
	waiting               chan struct{} // the check says here that it waits
}

func (k *fpFake) wire(f *Flatpak, dryRun bool) {
	f.present = func() bool { k.mu.Lock(); defer k.mu.Unlock(); return k.present }
	f.listApps = func(user bool) ([]flatpak.Application, error) {
		k.mu.Lock()
		defer k.mu.Unlock()
		k.appLists++
		if user {
			return slices.Clone(k.user), k.userErr
		}
		return slices.Clone(k.system), k.systemErr
	}
	f.listUpdates = func(user bool) ([]flatpak.UpdateInfo, error) {
		// An update check lists the user installation first: only that
		// list waits, so each check waits once, at the gate set when it
		// began, and two checks can wait at different gates.
		if user {
			k.mu.Lock()
			gate, waiting := k.gate, k.waiting
			k.mu.Unlock()
			if gate != nil {
				if waiting != nil {
					waiting <- struct{}{}
				}
				<-gate
			}
		}
		k.mu.Lock()
		defer k.mu.Unlock()
		if user {
			k.updateLists++
			return slices.Clone(k.userUpd), k.updUserErr
		}
		return slices.Clone(k.systemUpd), k.updSysErr
	}
	f.update = func(appID string, user bool) error {
		k.mu.Lock()
		defer k.mu.Unlock()
		k.updated = append(k.updated, fpWhere(user)+":"+appID)
		return k.updateErr
	}
	f.uninstall = func(appID string, user bool) error {
		k.mu.Lock()
		defer k.mu.Unlock()
		k.removed = append(k.removed, fpWhere(user)+":"+appID)
		return k.removeErr
	}
	f.launch = func(appID string) error {
		k.launched = append(k.launched, appID)
		return k.launchErr
	}
	f.dryRun = func() bool { return dryRun }
}

func (k *fpFake) set(fn func(k *fpFake)) {
	k.mu.Lock()
	defer k.mu.Unlock()
	fn(k)
}

func (k *fpFake) counts() (apps, updates int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.appLists, k.updateLists
}

func fpWhere(user bool) string {
	if user {
		return "user"
	}
	return "system"
}

func fpApp(name, id, version, origin string, user bool) flatpak.Application {
	return flatpak.Application{Name: name, ApplicationID: id, Version: version, Branch: "stable", Origin: origin, Installation: fpWhere(user)}
}

func fpUpd(name, id, version string, user bool) flatpak.UpdateInfo {
	return flatpak.UpdateInfo{Name: name, ApplicationID: id, NewVersion: version, Branch: "stable", Origin: "flathub", Installation: fpWhere(user)}
}

// fpSample is a machine with three system apps, one of the user's, and two
// updates.
func fpSample() *fpFake {
	return &fpFake{
		present: true,
		system: []flatpak.Application{
			fpApp("Firefox", "org.mozilla.firefox", "144.0", "flathub", false),
			fpApp("Bazaar", "io.github.kolunmi.Bazaar", "0.6.1", "flathub", false),
			fpApp("Flatseal", "com.github.tchx84.Flatseal", "2.3.1", "flathub", false),
		},
		user: []flatpak.Application{
			fpApp("Blender", "org.blender.Blender", "4.5.3", "flathub", true),
		},
		systemUpd: []flatpak.UpdateInfo{
			fpUpd("Firefox", "org.mozilla.firefox", "144.0.2", false),
			fpUpd("Bazaar", "io.github.kolunmi.Bazaar", "0.6.2", false),
		},
	}
}

// fpStarted returns a Flatpak on fake whose first loads have published.
func fpStarted(t *testing.T, fake *fpFake, cfg *config.Config, dryRun bool) (*Flatpak, *Env, *frostuitest.Loop) {
	t.Helper()
	p := frostuitest.NewLoop()
	env := NewEnv(cfg, dryRun, p.Post)
	f := NewFlatpak(env)
	fake.wire(f, dryRun)
	f.Start()
	p.RunUntil(t, func() bool { return !f.appsLoading && !f.updatesChecking })
	return f, env, p
}

func fpToasts(env *Env) []string {
	var out []string
	for _, t := range env.TakeToasts() {
		out = append(out, t.Title)
	}
	return out
}

func fpItemKeys(items []UpdateItem) []string {
	var keys []string
	for _, it := range items {
		keys = append(keys, it.Key)
	}
	return keys
}

func TestFlatpakLoadsAppsAndUpdates(t *testing.T) {
	f, _, _ := fpStarted(t, fpSample(), fpConfig(true, true, true), false)

	apps := f.Apps()
	var keys []string
	for _, e := range apps {
		keys = append(keys, e.Key)
	}
	want := []string{
		"flatpak:system:org.mozilla.firefox", "flatpak:system:io.github.kolunmi.Bazaar",
		"flatpak:system:com.github.tchx84.Flatseal", "flatpak:user:org.blender.Blender",
	}
	if !slices.Equal(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	firefox := apps[0]
	if firefox.Kind != AppFlatpakSystem || firefox.Name != "Firefox" || firefox.Version != "144.0" ||
		firefox.Origin != "flathub" || firefox.Update != "144.0.2" {
		t.Errorf("firefox = %+v", firefox)
	}
	if !slices.Equal(firefox.Actions, []AppAction{ActUpdate, ActOpen, ActUninstall}) {
		t.Errorf("firefox actions = %v", firefox.Actions)
	}
	if blender := apps[3]; blender.Kind != AppFlatpakUser || blender.Update != "" ||
		!slices.Equal(blender.Actions, []AppAction{ActOpen, ActUninstall}) {
		t.Errorf("blender = %+v", blender)
	}

	st := f.AppsStatus()
	if !st.Available || st.Loading || st.Err != "" {
		t.Errorf("apps status = %+v", st)
	}

	src := f.Source()
	if src == nil || src.ID() != SourceFlatpak {
		t.Fatalf("source = %v", src)
	}
	ss := src.Status()
	if !ss.Available || ss.Checking || ss.Err != "" || ss.Note != "2 updates available" || ss.CheckedAt.IsZero() {
		t.Errorf("source status = %+v", ss)
	}
	items := src.Items()
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if it := items[0]; it.Key != "flatpak:system:org.mozilla.firefox" || it.Title != "Firefox" ||
		it.Detail != "org.mozilla.firefox" || it.From != "144.0" || it.To != "144.0.2" || it.Action != "Update" || it.Held || it.Confirm {
		t.Errorf("item = %+v", it)
	}
	if jobs := src.Jobs(); len(jobs) != 2 || jobs[0].Source != "flatpak" || jobs[0].Title != "Updating Firefox" {
		t.Errorf("everything jobs = %+v", jobs)
	}
	if jobs := src.Jobs("flatpak:system:io.github.kolunmi.Bazaar"); len(jobs) != 1 || jobs[0].Key != "flatpak:system:io.github.kolunmi.Bazaar" {
		t.Errorf("one job = %+v", jobs)
	}
}

func TestFlatpakUserUpdateDetailAndBranchKeys(t *testing.T) {
	fake := fpSample()
	fake.userUpd = []flatpak.UpdateInfo{fpUpd("Blender", "org.blender.Blender", "", true)}
	fake.user = append(fake.user, flatpak.Application{Name: "Blender", ApplicationID: "org.blender.Blender", Branch: "beta", Installation: "user"})
	f, _, _ := fpStarted(t, fake, fpConfig(true, true, true), false)

	items := f.Source().Items()
	if items[0].Key != "flatpak:user:org.blender.Blender" || items[0].Detail != "org.blender.Blender (user)" || items[0].To != "" {
		t.Errorf("user item = %+v", items[0])
	}
	var blender []AppEntry
	for _, e := range f.Apps() {
		if e.ID == "org.blender.Blender" {
			blender = append(blender, e)
		}
	}
	if len(blender) != 2 || blender[0].Key != "flatpak:user:org.blender.Blender" || blender[1].Key != "flatpak:user:org.blender.Blender//beta" {
		t.Fatalf("blender entries = %+v", blender)
	}
	// An update without a version still offers the update.
	if blender[0].Update != "latest" || blender[1].Update != "" {
		t.Errorf("updates = %q %q", blender[0].Update, blender[1].Update)
	}
}

func TestFlatpakPartialFailureKeepsHealthyRows(t *testing.T) {
	fake := fpSample()
	fake.updUserErr = &flatpak.RemoteError{Remote: "gnome-nightly", Installation: "user", Err: errors.New("unreachable")}
	f, _, _ := fpStarted(t, fake, fpConfig(true, true, true), false)

	ss := f.Source().Status()
	if !strings.Contains(ss.Err, "the user installation could not be checked") || ss.Note != "" {
		t.Errorf("status = %+v", ss)
	}
	// The system installation answered: its rows are authoritative.
	if got := len(f.Source().Items()); got != 2 {
		t.Errorf("items = %d", got)
	}
}

func TestFlatpakNonAuthoritativeReloadKeepsKnownRows(t *testing.T) {
	fake := fpSample()
	f, _, p := fpStarted(t, fake, fpConfig(true, true, true), false)
	before := fpItemKeys(f.Source().Items())

	fake.set(func(k *fpFake) {
		k.userUpd, k.systemUpd = nil, nil
		k.updUserErr, k.updSysErr = errors.New("user down"), errors.New("system down")
	})
	f.Source().Refresh()
	if !f.Source().Status().Checking {
		t.Fatal("Refresh did not start a check")
	}
	p.RunUntil(t, func() bool { return !f.updatesChecking })

	if got := fpItemKeys(f.Source().Items()); !slices.Equal(got, before) {
		t.Errorf("a load that learned nothing replaced the rows: %v, want %v", got, before)
	}
	if ss := f.Source().Status(); ss.Err != "Could not check for updates" {
		t.Errorf("status = %+v", ss)
	}
	// The Apps table still offers the known updates.
	if f.Apps()[0].Update != "144.0.2" {
		t.Error("the known update left the Apps table")
	}

	// A check that works again replaces them.
	fake.set(func(k *fpFake) { k.updUserErr, k.updSysErr = nil, nil })
	f.Source().Refresh()
	p.RunUntil(t, func() bool { return !f.updatesChecking })
	if n := len(f.Source().Items()); n != 0 {
		t.Errorf("authoritative empty load kept %d rows", n)
	}
	if ss := f.Source().Status(); ss.Note != "All applications are up to date" || ss.Err != "" {
		t.Errorf("status = %+v", ss)
	}
}

func TestFlatpakLiveUpdateRemovesRowBeforeTheReload(t *testing.T) {
	fake := fpSample()
	f, env, p := fpStarted(t, fake, fpConfig(true, true, true), false)
	apps0, upd0 := fake.counts()

	// The reload after the update waits, so the row's state is seen
	// before it publishes.
	gate := make(chan struct{})
	fake.set(func(k *fpFake) {
		k.gate = gate
		k.systemUpd = k.systemUpd[1:] // the server no longer offers Firefox
		k.system[0].Version = "144.0.2"
	})
	const key = "flatpak:system:org.mozilla.firefox"
	f.Source().Update(key)
	if !env.Jobs.Pending("flatpak", key) {
		t.Fatal("Update queued no job")
	}
	if e := f.Apps()[0]; !e.Busy {
		t.Error("the Apps row is not busy while its job runs")
	}
	f.Source().Update(key) // a second click while it runs queues nothing
	if q := env.Jobs.Queued(); len(q) != 0 {
		t.Errorf("a second update was queued: %v", q)
	}
	p.RunUntil(t, func() bool { return !env.Jobs.Busy() })

	if got := fpItemKeys(f.Source().Items()); !slices.Equal(got, []string{"flatpak:system:io.github.kolunmi.Bazaar"}) {
		t.Errorf("rows after the update = %v", got)
	}
	if !f.updatesChecking {
		t.Error("a live success did not reload")
	}
	if fake.updated[0] != "system:org.mozilla.firefox" {
		t.Errorf("updated = %v", fake.updated)
	}
	if toasts := fpToasts(env); !slices.Equal(toasts, []string{"org.mozilla.firefox updated"}) {
		t.Errorf("toasts = %v", toasts)
	}

	// The reload fails everywhere: it learned nothing, so the rows stay as
	// the update left them, without Firefox.
	fake.set(func(k *fpFake) { k.updUserErr, k.updSysErr, k.systemUpd = errors.New("x"), errors.New("y"), nil })
	close(gate)
	p.RunUntil(t, func() bool { return !f.updatesChecking && !f.appsLoading })
	if got := fpItemKeys(f.Source().Items()); !slices.Equal(got, []string{"flatpak:system:io.github.kolunmi.Bazaar"}) {
		t.Errorf("rows after a failed reload = %v", got)
	}
	apps1, upd1 := fake.counts()
	if upd1 != upd0+1 || apps1 != apps0+2 {
		t.Errorf("reloads: updates %d→%d, apps %d→%d", upd0, upd1, apps0, apps1)
	}
	if v := f.Apps()[0].Version; v != "144.0.2" {
		t.Errorf("the Apps table shows version %q after the update", v)
	}
}

func TestFlatpakFailedOrDryRunUpdateRestores(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dryRun bool
		err    error
		toast  string
	}{
		{"failure", false, errors.New("boom"), "Update failed: boom"},
		{"dry run", true, nil, "[DRY-RUN] Preview: org.mozilla.firefox would be updated — no changes made"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := fpSample()
			fake.updateErr = tc.err
			f, env, p := fpStarted(t, fake, fpConfig(true, true, true), tc.dryRun)
			before := fpItemKeys(f.Source().Items())
			_, upd0 := fake.counts()

			const key = "flatpak:system:org.mozilla.firefox"
			f.Act(key, ActUpdate)
			p.RunUntil(t, func() bool { return !env.Jobs.Busy() })

			if got := fpItemKeys(f.Source().Items()); !slices.Equal(got, before) {
				t.Errorf("rows = %v, want %v", got, before)
			}
			if f.updatesChecking || f.appsLoading {
				t.Error("a failure or a dry run reloaded")
			}
			if _, upd1 := fake.counts(); upd1 != upd0 {
				t.Error("the update list was queried again")
			}
			if env.Jobs.Pending("flatpak", key) || f.Apps()[0].Busy {
				t.Error("the row's control did not come back")
			}
			if jobs := f.Source().Jobs(key); len(jobs) != 1 {
				t.Error("the row cannot be updated again")
			}
			if toasts := fpToasts(env); !slices.Equal(toasts, []string{tc.toast}) {
				t.Errorf("toasts = %q", toasts)
			}
		})
	}
}

func TestFlatpakStaleGenerationsArePassedOver(t *testing.T) {
	f, _, _ := fpStarted(t, fpSample(), fpConfig(true, true, true), false)

	older := f.flatpakUpdatesRefresh.Begin()
	newer := f.flatpakUpdatesRefresh.Begin()
	f.applyUpdates(newer, fpUpdatesResult{present: true, updates: nil, status: fpStatus(0, false, false)})
	f.applyUpdates(older, fpUpdatesResult{present: true,
		updates: []flatpak.UpdateInfo{fpUpd("Firefox", "org.mozilla.firefox", "145", false)}, status: fpStatus(1, false, false)})
	if n := len(f.Source().Items()); n != 0 {
		t.Errorf("a stale update load published %d rows", n)
	}

	olderApps := f.appsRefresh.Begin()
	newerApps := f.appsRefresh.Begin()
	f.applyApps(newerApps, fpAppsResult{present: true, system: []flatpak.Application{fpApp("Firefox", "org.mozilla.firefox", "145", "flathub", false)}})
	f.applyApps(olderApps, fpAppsResult{present: true})
	if apps := f.Apps(); len(apps) != 1 || apps[0].Version != "145" {
		t.Errorf("a stale apps load published: %+v", apps)
	}
}

func fpStatus(n int, userFailed, systemFailed bool) flatpakstatus.Result {
	return flatpakstatus.Subtitle(n, userFailed, systemFailed)
}

func TestFlatpakOverlappingReloadsKeepTheNewest(t *testing.T) {
	fake := fpSample()
	f, _, p := fpStarted(t, fake, fpConfig(false, false, true), false)

	gate, waiting := make(chan struct{}), make(chan struct{}, 1)
	fake.set(func(k *fpFake) { k.gate, k.waiting = gate, waiting })
	f.loadUpdates() // the older load waits at the gate with the old list
	<-waiting
	fake.set(func(k *fpFake) { k.gate, k.waiting = nil, nil; k.systemUpd = nil })
	f.loadUpdates() // the newer one finds nothing to update
	p.RunUntil(t, func() bool { return !f.updatesChecking })
	if n := len(f.Source().Items()); n != 0 {
		t.Fatalf("newest load left %d rows", n)
	}
	ran := p.Ran()
	fake.set(func(k *fpFake) {
		k.systemUpd = []flatpak.UpdateInfo{fpUpd("Firefox", "org.mozilla.firefox", "999", false)}
	})
	close(gate)
	p.RunUntil(t, func() bool { return p.Ran() > ran })
	if n := len(f.Source().Items()); n != 0 {
		t.Errorf("the overtaken load brought back %d rows", n)
	}
}

func TestFlatpakGroupsGateEveryLoad(t *testing.T) {
	fake := fpSample()
	f, _, _ := fpStarted(t, fake, fpConfig(false, false, false), false)
	if apps, upd := fake.counts(); apps != 0 || upd != 0 {
		t.Errorf("disabled groups loaded: apps %d, updates %d", apps, upd)
	}
	if f.Source() != nil || f.AppsStatus().Available || len(f.Apps()) != 0 {
		t.Error("disabled groups show")
	}
	f.RefreshApps()
	if f.appsLoading {
		t.Error("RefreshApps loaded with its groups off")
	}

	// Only the user installation: the system one is never listed.
	fake = fpSample()
	f, _, _ = fpStarted(t, fake, fpConfig(true, false, false), false)
	if apps := f.Apps(); len(apps) != 1 || apps[0].Kind != AppFlatpakUser {
		t.Errorf("user-only apps = %+v", apps)
	}
	if apps, _ := fake.counts(); apps != 1 {
		t.Errorf("listed %d installations", apps)
	}
}

func TestFlatpakMissingToolIsUnavailable(t *testing.T) {
	fake := fpSample()
	fake.present = false
	f, _, _ := fpStarted(t, fake, fpConfig(true, true, true), false)
	if apps, upd := fake.counts(); apps != 0 || upd != 0 {
		t.Errorf("listed without flatpak: %d %d", apps, upd)
	}
	if f.AppsStatus().Available || len(f.Apps()) != 0 {
		t.Error("apps are available without flatpak")
	}
	if ss := f.Source().Status(); ss.Available || len(f.Source().Items()) != 0 {
		t.Errorf("source = %+v", ss)
	}
}

func TestFlatpakListFailureKeepsItsInstallationRows(t *testing.T) {
	fake := fpSample()
	f, _, p := fpStarted(t, fake, fpConfig(true, true, false), false)
	fake.set(func(k *fpFake) { k.systemErr = errors.New("system list broke"); k.system = nil })
	f.RefreshApps()
	p.RunUntil(t, func() bool { return !f.appsLoading })
	if n := len(f.Apps()); n != 4 {
		t.Errorf("a failed list dropped rows: %d left", n)
	}
	if st := f.AppsStatus(); st.Err != "system installation: system list broke" {
		t.Errorf("err = %q", st.Err)
	}
}

// fpConfirm answers the confirmation on screen by clicking label.
func fpConfirm(t *testing.T, env *Env, label string) {
	t.Helper()
	if !env.Dialogs.Showing() {
		t.Fatal("no confirmation is up")
	}
	tt := ui.NewTester(func(c *ui.Context) { env.Dialogs.View(c) }, 900, 700)
	if err := tt.Click(label); err != nil {
		t.Fatalf("click %q: %v in %q", label, err, tt.Texts())
	}
	tt.Frame()
}

func TestFlatpakUninstallAsksThenRemoves(t *testing.T) {
	fake := fpSample()
	f, env, p := fpStarted(t, fake, fpConfig(true, true, true), false)
	apps0, _ := fake.counts()
	const key = "flatpak:system:org.mozilla.firefox"

	f.Act(key, ActUninstall)
	if len(fake.removed) != 0 || env.Jobs.Busy() {
		t.Fatal("uninstall ran before the confirmation")
	}
	fpConfirm(t, env, "Cancel")
	if env.Dialogs.Showing() || env.Jobs.Busy() {
		t.Fatal("Cancel still uninstalled")
	}

	f.Act(key, ActUninstall)
	fpConfirm(t, env, "Uninstall")
	p.RunUntil(t, func() bool { return !env.Jobs.Busy() && !f.appsLoading })
	if !slices.Equal(fake.removed, []string{"system:org.mozilla.firefox"}) {
		t.Errorf("removed = %v", fake.removed)
	}
	if toasts := fpToasts(env); !slices.Equal(toasts, []string{"org.mozilla.firefox uninstalled"}) {
		t.Errorf("toasts = %v", toasts)
	}
	if apps1, _ := fake.counts(); apps1 != apps0+2 {
		t.Error("a live uninstall did not reload the apps")
	}
	if got := fpItemKeys(f.Source().Items()); slices.Contains(got, key) {
		t.Error("the uninstalled app still offers an update")
	}
}

// TestFlatpakUninstallRetiresAnUpdateCheckInFlight: an update check that
// began before a live uninstall still lists the app; it must not bring the
// removed row back. The check the uninstall starts decides.
func TestFlatpakUninstallRetiresAnUpdateCheckInFlight(t *testing.T) {
	fake := fpSample()
	f, env, p := fpStarted(t, fake, fpConfig(true, true, true), false)
	const key = "flatpak:system:org.mozilla.firefox"

	// The older check waits at its gate; it reads the lists, Firefox's
	// update included, once the gate opens.
	older, olderWaiting := make(chan struct{}), make(chan struct{}, 1)
	fake.set(func(k *fpFake) { k.gate, k.waiting = older, olderWaiting })
	f.Source().Refresh()
	<-olderWaiting
	// Any later check waits at its own gate.
	newer, newerWaiting := make(chan struct{}), make(chan struct{}, 1)
	fake.set(func(k *fpFake) { k.gate, k.waiting = newer, newerWaiting })

	f.Act(key, ActUninstall)
	fpConfirm(t, env, "Uninstall")
	p.RunUntil(t, func() bool { return !env.Jobs.Busy() })
	if got := fpItemKeys(f.Source().Items()); slices.Contains(got, key) {
		t.Fatalf("the uninstalled app still offers an update: %v", got)
	}
	p.RunUntil(t, func() bool { return !f.appsLoading })

	// The older check answers with Firefox's update. It is the only worker
	// left that can post (a newer check waits at its own gate), so the next
	// post is its result: it publishes nothing.
	ran := p.Ran()
	close(older)
	p.RunUntil(t, func() bool { return p.Ran() > ran })
	if got := fpItemKeys(f.Source().Items()); slices.Contains(got, key) {
		t.Fatalf("the check that began before the uninstall brought its row back: %v", got)
	}
	select {
	case <-newerWaiting:
	case <-time.After(frostuitest.Timeout):
		t.Fatal("the live uninstall did not start a newer update check")
	}
	if !f.updatesChecking {
		t.Error("the overtaken check ended the newer one's check")
	}

	// The newer check began after the uninstall: it decides.
	fake.set(func(k *fpFake) { k.systemUpd = k.systemUpd[1:] })
	close(newer)
	p.RunUntil(t, func() bool { return !f.updatesChecking })
	if got := fpItemKeys(f.Source().Items()); !slices.Equal(got, []string{"flatpak:system:io.github.kolunmi.Bazaar"}) {
		t.Errorf("rows after the newer check = %v", got)
	}
	if ss := f.Source().Status(); ss.Note != "1 update available" {
		t.Errorf("status = %+v", ss)
	}
}

// TestFlatpakUninstallWithoutACheckInFlightDoesNotCheck: with no update
// check running, a live uninstall only drops the row; it starts no check.
func TestFlatpakUninstallWithoutACheckInFlightDoesNotCheck(t *testing.T) {
	fake := fpSample()
	f, env, p := fpStarted(t, fake, fpConfig(true, true, true), false)
	_, upd0 := fake.counts()
	f.Act("flatpak:system:org.mozilla.firefox", ActUninstall)
	fpConfirm(t, env, "Uninstall")
	p.RunUntil(t, func() bool { return !env.Jobs.Busy() && !f.appsLoading })
	if _, upd1 := fake.counts(); upd1 != upd0 || f.updatesChecking {
		t.Errorf("a live uninstall checked for updates: %d→%d, checking %t", upd0, upd1, f.updatesChecking)
	}
}

func TestFlatpakUninstallDryRunAndFailureChangeNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dryRun bool
		err    error
		toast  string
	}{
		{"dry run", true, nil, "[DRY-RUN] Preview: org.blender.Blender would be uninstalled — no changes made"},
		{"failure", false, errors.New("in use"), "Uninstall failed: in use"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := fpSample()
			fake.removeErr = tc.err
			f, env, p := fpStarted(t, fake, fpConfig(true, true, true), tc.dryRun)
			apps0, upd0 := fake.counts()
			f.Act("flatpak:user:org.blender.Blender", ActUninstall)
			fpConfirm(t, env, "Uninstall")
			p.RunUntil(t, func() bool { return !env.Jobs.Busy() })
			if !slices.Equal(fake.removed, []string{"user:org.blender.Blender"}) {
				t.Errorf("removed = %v", fake.removed)
			}
			if apps1, upd1 := fake.counts(); apps1 != apps0 || upd1 != upd0 || f.appsLoading {
				t.Error("reloaded after a dry run or a failure")
			}
			if n := len(f.Apps()); n != 4 {
				t.Errorf("rows = %d", n)
			}
			if toasts := fpToasts(env); !slices.Equal(toasts, []string{tc.toast}) {
				t.Errorf("toasts = %q", toasts)
			}
		})
	}
}

func TestFlatpakOpenLaunchesTheApp(t *testing.T) {
	fake := fpSample()
	f, env, _ := fpStarted(t, fake, fpConfig(true, true, false), true)
	f.Act("flatpak:user:org.blender.Blender", ActOpen)
	if !slices.Equal(fake.launched, []string{"org.blender.Blender"}) {
		t.Errorf("launched = %v", fake.launched)
	}
	if env.Jobs.Busy() {
		t.Error("Open queued a job")
	}
	fake.launchErr = errors.New("no gtk-launch")
	f.Act("flatpak:user:org.blender.Blender", ActOpen)
	if toasts := fpToasts(env); !slices.Equal(toasts, []string{"Failed to launch org.blender.Blender"}) {
		t.Errorf("toasts = %v", toasts)
	}
	// An unknown key does nothing.
	f.Act("flatpak:user:nope", ActOpen)
	if len(fake.launched) != 2 {
		t.Errorf("launched = %v", fake.launched)
	}
}

func TestFlatpakKeys(t *testing.T) {
	for _, tc := range []struct {
		user           bool
		id, branch, to string
	}{
		{false, "org.a.B", "stable", "flatpak:system:org.a.B"},
		{true, "org.a.B", "", "flatpak:user:org.a.B"},
		{true, "org.a.B", "beta", "flatpak:user:org.a.B//beta"},
	} {
		if got := fpKey(tc.user, tc.id, tc.branch); got != tc.to {
			t.Errorf("fpKey(%v, %q, %q) = %q, want %q", tc.user, tc.id, tc.branch, got, tc.to)
		}
	}
	if fpName("", "org.a.B") != "org.a.B" || fpName("B", "org.a.B") != "B" {
		t.Error("fpName")
	}
}
