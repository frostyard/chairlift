package views

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/navigation"
	"github.com/frostyard/chairlift/internal/views/trustmsg"
)

// fakeBrew stands in for Homebrew: no test runs brew. Its functions run on
// the workers, so it locks.
type fakeBrew struct {
	mu        sync.Mutex
	installed bool
	dry       bool

	formulae, casks, outdated          []homebrew.Package
	formulaeErr, casksErr, outdatedErr error
	taps                               []homebrew.UntrustedTap
	tapsErr                            error
	search                             map[string][]homebrew.SearchResult

	// hooks replace a list call when set, to block it or count it.
	outdatedHook func() ([]homebrew.Package, error)
	formulaeHook func() ([]homebrew.Package, error)
	searchHook   func(query string) ([]homebrew.SearchResult, error)

	// errs fails a mutation by its call text, as "upgrade gh".
	errs  map[string]error
	calls []string
}

func newFakeBrew() *fakeBrew {
	return &fakeBrew{
		installed: true,
		formulae: []homebrew.Package{
			{Name: "bbrew", Version: "0.4.1"},
			{Name: "gh", Version: "2.82.0"},
			{Name: "go", Version: "1.25.3", Pinned: true},
		},
		casks: []homebrew.Package{{Name: "font-inter", Version: "4.1"}},
		outdated: []homebrew.Package{
			{Name: "gh", Version: "2.82.0", Outdated: true},
			{Name: "go", Version: "1.25.3", Outdated: true, Pinned: true},
			{Name: "bbrew", Version: "0.4.1", Outdated: true},
		},
		taps:   []homebrew.UntrustedTap{{Name: "valkyrie00/bbrew", Formulae: []string{"valkyrie00/bbrew/bbrew"}}},
		search: map[string][]homebrew.SearchResult{},
		errs:   map[string]error{},
	}
}

func (f *fakeBrew) call(c string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	return f.errs[c]
}

func (f *fakeBrew) count(c string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, got := range f.calls {
		if got == c {
			n++
		}
	}
	return n
}

func (f *fakeBrew) set(fn func(f *fakeBrew)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeBrew) backend() brewBackend {
	list := func(name string, pkgs *[]homebrew.Package, err *error, hook *func() ([]homebrew.Package, error)) func() ([]homebrew.Package, error) {
		return func() ([]homebrew.Package, error) {
			f.mu.Lock()
			h := *hook
			f.mu.Unlock()
			_ = f.call(name)
			if h != nil {
				return h()
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			return slices.Clone(*pkgs), *err
		}
	}
	var noHook func() ([]homebrew.Package, error)
	return brewBackend{
		installed: func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.installed },
		dryRun:    func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.dry },
		formulae:  list("formulae", &f.formulae, &f.formulaeErr, &f.formulaeHook),
		casks:     list("casks", &f.casks, &f.casksErr, &noHook),
		outdated:  list("outdated", &f.outdated, &f.outdatedErr, &f.outdatedHook),
		untrusted: func() ([]homebrew.UntrustedTap, error) {
			_ = f.call("taps")
			f.mu.Lock()
			defer f.mu.Unlock()
			return slices.Clone(f.taps), f.tapsErr
		},
		search: func(q string) ([]homebrew.SearchResult, error) {
			_ = f.call("search " + q)
			f.mu.Lock()
			h := f.searchHook
			r := f.search[q]
			f.mu.Unlock()
			if h != nil {
				return h(q)
			}
			return r, nil
		},
		install: func(name string, cask bool) error {
			if cask {
				return f.call("install --cask " + name)
			}
			return f.call("install " + name)
		},
		uninstall: func(name string, cask bool) error {
			if cask {
				return f.call("uninstall --cask " + name)
			}
			return f.call("uninstall " + name)
		},
		upgrade: func(name string) error { return f.call("upgrade " + name) },
		update:  func() error { return f.call("update") },
		pin:     func(name string) error { return f.call("pin " + name) },
		unpin:   func(name string) error { return f.call("unpin " + name) },
		trust: func(tap homebrew.UntrustedTap) error {
			return f.call("trust " + tap.Name + " " + strings.Join(append(tap.Formulae, tap.Casks...), " "))
		},
		bundles:       homebrew.AvailableBundles, // reads the test's own directory
		bundleInstall: func(path string) error { return f.call("bundle install " + path) },
		bundleDump:    func(path string, force bool) error { return f.call("bundle dump " + path) },
		readBrewfile:  readBrewfile,
		home:          func() (string, error) { return "/home/frosty", nil },
	}
}

// uiLoop is the test's UI thread: Env.Post queues, and the test runs what
// was posted.
type uiLoop struct{ ch chan func() }

func newUILoop() *uiLoop { return &uiLoop{ch: make(chan func(), 4096)} }

func (l *uiLoop) post(fn func()) { l.ch <- fn }

// until runs posted functions until cond holds.
func (l *uiLoop) until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for !cond() {
		select {
		case fn := <-l.ch:
			fn()
		case <-time.After(2 * time.Millisecond):
			// cond may wait on a worker that posts nothing yet.
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// settle runs posted functions until none comes for a while.
func (l *uiLoop) settle() {
	for {
		select {
		case fn := <-l.ch:
			fn()
		case <-time.After(60 * time.Millisecond):
			return
		}
	}
}

var brewGroups = map[string]string{
	"brew_group":         "applications_page",
	"brew_search_group":  "applications_page",
	"brew_bundles_group": "applications_page",
	"brew_updates_group": "updates_page",
	"brew_trust_group":   "updates_page",
}

// allBrewGroups enables every Homebrew group.
var allBrewGroups = []string{"brew_group", "brew_search_group", "brew_bundles_group", "brew_updates_group", "brew_trust_group"}

// brewConfig enables the given Homebrew groups and disables the others;
// bundles reads bundleDir.
func brewConfig(bundleDir string, groups ...string) *config.Config {
	cfg := &config.Config{ApplicationsPage: config.PageConfig{}, UpdatesPage: config.PageConfig{}}
	for group, page := range brewGroups {
		g := config.GroupConfig{Enabled: slices.Contains(groups, group)}
		if group == "brew_bundles_group" && bundleDir != "" {
			g.BundlesPaths = []string{bundleDir}
		}
		if page == "applications_page" {
			cfg.ApplicationsPage[group] = g
		} else {
			cfg.UpdatesPage[group] = g
		}
	}
	return cfg
}

// writeBundles writes the two Brewfiles Snow ships into a directory.
func writeBundles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"cli.Brewfile": "# Recommended CLI Tools\ntap \"valkyrie00/bbrew\"\nbrew \"atuin\"\nbrew \"bat\"\nbrew \"bash-preexec\"\n" +
			"brew \"valkyrie00/bbrew/bbrew\"\nbrew \"chezmoi\"\nbrew \"direnv\"\nbrew \"dysk\"\nbrew \"eza\"\nbrew \"fd\"\nbrew \"gh\"\n" +
			"brew \"glab\"\nbrew \"rg\"\nbrew \"starship\"\nbrew \"shellcheck\"\nbrew \"stress-ng\"\nbrew \"tealdeer\"\n" +
			"brew \"trash-cli\"\nbrew \"television\"\nbrew \"ugrep\"\nbrew \"yq\"\nbrew \"zoxide\"",
		"fonts.Brewfile": "# Recommended monospaced fonts\ncask \"font-caskaydia-mono-nerd-font\"\ncask \"font-comic-shanns-mono-nerd-font\"\n" +
			"cask \"font-droid-sans-mono-nerd-font\"\ncask \"font-go-mono-nerd-font\"\ncask \"font-blex-mono-nerd-font\"\n" +
			"cask \"font-sauce-code-pro-nerd-font\"\ncask \"font-source-code-pro\"\ncask \"font-ubuntu-nerd-font\"\n" +
			"cask \"font-fira-code-nerd-font\"\ncask \"font-0xproto-nerd-font\"",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type brewHarness struct {
	t    *testing.T
	f    *fakeBrew
	env  *Env
	loop *uiLoop
	b    *Brew
	dir  string
}

// startBrew starts a Brew on the fake with groups enabled and waits for
// its first loads.
func startBrew(t *testing.T, f *fakeBrew, groups ...string) *brewHarness {
	t.Helper()
	h := &brewHarness{t: t, f: f, loop: newUILoop(), dir: writeBundles(t)}
	h.env = NewEnv(brewConfig(h.dir, groups...), false, h.loop.post)
	h.b = newBrew(h.env, f.backend())
	h.b.Start()
	h.idle()
	return h
}

// idle waits until no Homebrew load is in flight.
func (h *brewHarness) idle() {
	h.t.Helper()
	b := h.b
	bundles := b.BundlesAvailable()
	h.loop.until(h.t, "the loads", func() bool {
		return b.have != brewUnknown && b.inv.loading == 0 && b.updates.loading == 0 &&
			b.updates.tapsLoading == 0 && b.search.loading == 0 && (!bundles || b.bundles.loaded)
	})
}

// jobsDone waits until the job queue is idle.
func (h *brewHarness) jobsDone() {
	h.t.Helper()
	h.loop.until(h.t, "the jobs", func() bool { return !h.env.Jobs.Busy() })
}

// confirm answers the open confirmation with the button labelled answer.
func (h *brewHarness) confirm(answer string) {
	h.t.Helper()
	if !h.env.Dialogs.Showing() {
		h.t.Fatalf("no confirmation to answer %q", answer)
	}
	dt := ui.NewTester(h.env.Dialogs.View, 900, 700)
	if err := dt.Click(answer); err != nil {
		h.t.Fatalf("click %q: %v in %q", answer, err, dt.Texts())
	}
}

// dialogTexts are what the open confirmation says.
func (h *brewHarness) dialogTexts() []string {
	if !h.env.Dialogs.Showing() {
		return nil
	}
	return ui.NewTester(h.env.Dialogs.View, 900, 700).Texts()
}

// toasts returns the toasts queued since the last call, as "type: title".
func (h *brewHarness) toasts() []string {
	var out []string
	for _, t := range h.env.TakeToasts() {
		out = append(out, t.Type+": "+t.Title)
	}
	return out
}

func (h *brewHarness) entry(key string) AppEntry {
	h.t.Helper()
	for _, e := range h.b.Apps() {
		if e.Key == key {
			return e
		}
	}
	h.t.Fatalf("no Apps entry %q", key)
	return AppEntry{}
}

func itemKeys(items []UpdateItem) []string {
	var keys []string
	for _, it := range items {
		keys = append(keys, it.Key)
	}
	return keys
}

func TestBrewLoadsPackagesUpdatesAndTaps(t *testing.T) {
	h := startBrew(t, newFakeBrew(), allBrewGroups...)
	b := h.b

	apps := b.Apps()
	var keys []string
	for _, e := range apps {
		keys = append(keys, e.Key)
	}
	if want := []string{"formula:bbrew", "formula:gh", "formula:go", "cask:font-inter"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("Apps keys = %v, want %v", keys, want)
	}
	gh := h.entry("formula:gh")
	if gh.Kind != AppFormula || gh.Version != "2.82.0" || gh.Update == "" || gh.Busy ||
		!reflect.DeepEqual(gh.Actions, []AppAction{ActUpdate, ActPin, ActUninstall}) {
		t.Errorf("gh = %+v", gh)
	}
	goEntry := h.entry("formula:go")
	if !goEntry.Pinned || !reflect.DeepEqual(goEntry.Actions, []AppAction{ActUnpin, ActUninstall}) {
		t.Errorf("a pinned formula offers no update: %+v", goEntry)
	}
	bbrew := h.entry("formula:bbrew")
	if bbrew.Origin != "valkyrie00/bbrew" || !reflect.DeepEqual(bbrew.Actions, []AppAction{ActPin, ActUninstall}) {
		t.Errorf("a package of an untrusted tap is held: %+v", bbrew)
	}
	inter := h.entry("cask:font-inter")
	if inter.Kind != AppCask || !reflect.DeepEqual(inter.Actions, []AppAction{ActUninstall}) {
		t.Errorf("a cask offers no pin: %+v", inter)
	}
	if st := b.AppsStatus(); !st.Available || st.Loading || st.Err != "" {
		t.Errorf("AppsStatus = %+v", st)
	}

	src := b.Source()
	items := src.Items()
	if got := itemKeys(items); !reflect.DeepEqual(got, []string{"gh", "go", "bbrew"}) {
		t.Fatalf("items = %v", got)
	}
	if items[0].Held || items[0].From != "2.82.0" || items[0].Detail != "formula" {
		t.Errorf("gh item = %+v", items[0])
	}
	if !items[1].Held || items[1].HeldWhy != "pinned" {
		t.Errorf("go item = %+v", items[1])
	}
	if !items[2].Held || items[2].HeldWhy != "tap not trusted" || items[2].Detail != "formula · valkyrie00/bbrew" {
		t.Errorf("bbrew item = %+v", items[2])
	}
	st := src.Status()
	if !st.Available || st.Checking || st.Err != "" || st.Note != "3 outdated · 2 held · 1 untrusted tap" {
		t.Errorf("Status = %+v", st)
	}
	if got := itemKeys(b.src.upgradable(nil)); !reflect.DeepEqual(got, []string{"gh"}) {
		t.Errorf("Update everything would upgrade %v, want only gh", got)
	}
}

func TestBrewSourceAndTabsFollowTheirGroups(t *testing.T) {
	h := startBrew(t, newFakeBrew(), "brew_bundles_group")
	if h.b.Source() != nil {
		t.Error("Source is not nil with brew_updates_group and brew_trust_group off")
	}
	if h.b.Apps() != nil || h.b.AppsStatus().Available || h.b.SearchAvailable() {
		t.Error("installed packages or search show with their groups off")
	}
	if !h.b.BundlesAvailable() {
		t.Error("bundles are off with brew_bundles_group on")
	}
	if n := h.f.count("formulae") + h.f.count("outdated") + h.f.count("taps"); n != 0 {
		t.Errorf("disabled groups loaded %d lists: %v", n, h.f.calls)
	}

	f := newFakeBrew()
	f.installed = false
	h = startBrew(t, f, allBrewGroups...)
	if st := h.b.Source().Status(); st.Available {
		t.Errorf("the source is available without Homebrew: %+v", st)
	}
	if h.b.AppsStatus().Available || h.b.SearchAvailable() || len(h.b.Commands()) != 0 {
		t.Error("Apps, search or commands are offered without Homebrew")
	}
	if !h.b.BundlesAvailable() || !strings.Contains(h.b.bundles.presentation.Description, "Homebrew is not installed") {
		t.Errorf("bundles say %q, want them shown with installs off", h.b.bundles.presentation.Description)
	}
	h.b.AskBundle(h.b.bundles.items[0].Path)
	if h.env.Dialogs.Showing() {
		t.Error("a bundle install asked without Homebrew")
	}
}

func TestBrewUpgradeOutcomes(t *testing.T) {
	trustErr := &homebrew.UntrustedTapError{Message: "Error: gh comes from an untrusted tap"}
	tests := []struct {
		name         string
		dry          bool
		err          error
		wantRemoved  bool
		wantRefresh  bool
		wantToast    string
		wantReopened bool
	}{
		{name: "live success removes the row and reloads", wantRemoved: true, wantRefresh: true, wantToast: "success: gh upgraded"},
		{name: "failure keeps the row and reopens", err: errors.New("boom"), wantToast: "error: Upgrade failed: boom", wantReopened: true},
		{name: "dry run keeps the row and reopens", dry: true, wantToast: "info: [DRY-RUN] Preview: gh would be upgraded — no changes made", wantReopened: true},
		{name: "untrusted tap says what to trust", err: trustErr, wantToast: "error: " + trustmsg.UpgradeMessage("gh", true), wantReopened: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeBrew()
			f.dry = tt.dry
			if tt.err != nil {
				f.errs["upgrade gh"] = tt.err
			}
			h := startBrew(t, f, allBrewGroups...)
			_ = h.toasts()
			if tt.wantRemoved {
				// brew outdated no longer lists what was upgraded.
				f.set(func(f *fakeBrew) { f.outdated = f.outdated[1:] })
			}
			before := f.count("outdated")
			h.b.Source().Update("gh")
			if !h.entry("formula:gh").Busy || h.b.StateLabel("gh") != "Upgrading…" {
				t.Errorf("gh is not busy while it upgrades: %+v", h.entry("formula:gh"))
			}
			h.b.Source().Update("gh")
			h.loop.until(t, "the upgrade", func() bool { return !h.env.Jobs.Busy() })
			if f.count("upgrade gh") != 1 {
				t.Fatalf("upgrade calls = %v, want one: a second click while it runs does nothing", f.calls)
			}
			if got := h.toasts(); !slices.Contains(got, tt.wantToast) {
				t.Errorf("toasts = %q, want %q", got, tt.wantToast)
			}
			removed := !slices.Contains(itemKeys(h.b.Source().Items()), "gh")
			if removed != tt.wantRemoved {
				t.Errorf("gh removed = %v, want %v", removed, tt.wantRemoved)
			}
			h.idle()
			if refreshed := f.count("outdated") > before; refreshed != tt.wantRefresh {
				t.Errorf("reloaded = %v, want %v", refreshed, tt.wantRefresh)
			}
			reopened := slices.Contains(itemKeys(h.b.src.upgradable(nil)), "gh")
			if reopened != tt.wantReopened {
				t.Errorf("upgrade control open again = %v, want %v", reopened, tt.wantReopened)
			}
		})
	}
}

func TestBrewUpgradeAllSkipsHeldPackages(t *testing.T) {
	f := newFakeBrew()
	f.outdated = append(f.outdated, homebrew.Package{Name: "font-inter", Version: "4.1"})
	h := startBrew(t, f, allBrewGroups...)
	h.b.UpgradeAll()
	h.jobsDone()
	if f.count("upgrade gh") != 1 || f.count("upgrade font-inter") != 1 || f.count("upgrade go") != 0 || f.count("upgrade bbrew") != 0 {
		t.Errorf("calls = %v, want gh and font-inter upgraded, go (pinned) and bbrew (untrusted tap) held", f.calls)
	}
}

func TestBrewFailedRefreshKeepsLastKnownRows(t *testing.T) {
	f := newFakeBrew()
	h := startBrew(t, f, allBrewGroups...)
	f.set(func(f *fakeBrew) {
		f.outdatedErr = errors.New("network down")
		f.outdated = nil
		f.formulaeErr = errors.New("brew info broke")
		f.formulae = nil
	})
	h.b.Source().Refresh()
	h.b.RefreshApps()
	h.idle()
	if got := itemKeys(h.b.Source().Items()); len(got) != 3 {
		t.Errorf("a failed check replaced the rows: %v", got)
	}
	if st := h.b.Source().Status(); st.Err != "Error refreshing updates: network down" || !strings.HasPrefix(st.Note, "3 outdated") {
		t.Errorf("Status = %+v, want the error and the last known count", st)
	}
	if n := len(h.b.Apps()); n != 4 {
		t.Errorf("a failed inventory list dropped rows: %d entries", n)
	}
	if st := h.b.AppsStatus(); st.Err != "Could not list formulae: brew info broke" {
		t.Errorf("AppsStatus.Err = %q", st.Err)
	}
	f.set(func(f *fakeBrew) { f.outdatedErr = nil })
	h.b.Source().Refresh()
	h.idle()
	if got := h.b.Source().Items(); len(got) != 0 || h.b.Source().Status().Err != "" {
		t.Errorf("a check that worked did not replace the rows: %v", got)
	}
}

// TestBrewFailedRefreshKeepsCompletedControls: after a live uninstall or
// pin, a reload whose list of that kind fails keeps the stale row, and
// keeps its control completed, so it does not offer the action again. A
// list that loads resets the controls of its kind, and only of its kind.
func TestBrewFailedRefreshKeepsCompletedControls(t *testing.T) {
	for _, tc := range []struct {
		name, key, verb, label string
		act                    AppAction
		apply                  func(f *fakeBrew) // what the action changed in Homebrew
	}{
		{"uninstall", "formula:gh", "Uninstall", "Uninstalled", ActUninstall, func(f *fakeBrew) {
			f.formulae = slices.DeleteFunc(f.formulae, func(p homebrew.Package) bool { return p.Name == "gh" })
		}},
		{"pin", "formula:gh", "Pin", "Pinned", ActPin, func(f *fakeBrew) {
			for i := range f.formulae {
				if f.formulae[i].Name == "gh" {
					f.formulae[i].Pinned = true
				}
			}
		}},
		{"cask uninstall", "cask:font-inter", "Uninstall", "Uninstalled", ActUninstall, func(f *fakeBrew) {
			f.casks = nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeBrew()
			h := startBrew(t, f, allBrewGroups...)
			b := h.b
			cask := strings.HasPrefix(tc.key, "cask:")
			fail := func(f *fakeBrew) {
				if cask {
					f.casksErr = errors.New("brew info broke")
				} else {
					f.formulaeErr = errors.New("brew info broke")
				}
			}

			// The action succeeds, but the reload after it cannot list
			// its kind.
			b.Act(tc.key, tc.act)
			h.confirm(tc.verb)
			f.set(func(f *fakeBrew) { tc.apply(f); fail(f) })
			h.jobsDone()
			h.idle()
			if st := b.AppsStatus(); !strings.Contains(st.Err, "brew info broke") {
				t.Fatalf("AppsStatus.Err = %q, want the failed list", st.Err)
			}
			e := h.entry(tc.key)
			if len(e.Actions) != 0 || !e.Busy || b.StateLabel(tc.key) != tc.label {
				t.Errorf("after a failed reload the stale row reopened: %+v, %q", e, b.StateLabel(tc.key))
			}
			for _, act := range []AppAction{ActPin, ActUnpin, ActUninstall, ActUpdate} {
				b.Act(tc.key, act)
				if h.env.Dialogs.Showing() || h.env.Jobs.Busy() {
					t.Fatalf("the completed row took %v again: %q", act, h.dialogTexts())
				}
			}

			// A reload whose list of this kind fails again keeps the row
			// closed, though its other list loads.
			b.RefreshApps()
			h.idle()
			if e := h.entry(tc.key); len(e.Actions) != 0 {
				t.Errorf("a reload that failed again reopened the row: %+v", e)
			}

			// A list of its kind that loads replaces the row and its
			// control.
			f.set(func(f *fakeBrew) { f.formulaeErr, f.casksErr = nil, nil })
			b.RefreshApps()
			h.idle()
			if b.inv.rows[tc.key] != nil {
				t.Errorf("a list that loaded kept the completed control: %+v", b.inv.rows[tc.key])
			}
			var listed *AppEntry
			for _, e := range b.Apps() {
				if e.Key == tc.key {
					listed = &e
				}
			}
			switch tc.act {
			case ActUninstall:
				if listed != nil {
					t.Errorf("the uninstalled package is still listed: %+v", *listed)
				}
			case ActPin:
				if listed == nil || !listed.Pinned || !slices.Contains(listed.Actions, ActUnpin) {
					t.Errorf("the pinned formula = %+v, want it pinned and offering Unpin", listed)
				}
			}
		})
	}

	// A formula's completed control is reset by a formulae list that
	// loads, even while the casks list fails.
	f := newFakeBrew()
	h := startBrew(t, f, allBrewGroups...)
	h.b.Act("formula:gh", ActUninstall)
	h.confirm("Uninstall")
	f.set(func(f *fakeBrew) {
		f.formulae = f.formulae[:1]
		f.casksErr = errors.New("cask list broke")
	})
	h.jobsDone()
	h.idle()
	if h.b.inv.rows["formula:gh"] != nil {
		t.Error("a formulae list that loaded kept the completed control because the casks failed")
	}
	for _, e := range h.b.Apps() {
		if e.Key == "formula:gh" {
			t.Errorf("gh is still listed: %+v", e)
		}
	}
}

func TestBrewStaleGenerationsAreIgnored(t *testing.T) {
	f := newFakeBrew()
	h := startBrew(t, f, allBrewGroups...)

	// Outdated: a slow check that a newer one overtook publishes nothing.
	release := make(chan []homebrew.Package)
	f.set(func(f *fakeBrew) {
		first := true
		f.outdatedHook = func() ([]homebrew.Package, error) {
			f.mu.Lock()
			slow := first
			first = false
			f.mu.Unlock()
			if slow {
				return <-release, nil
			}
			return []homebrew.Package{{Name: "fresh", Version: "1"}}, nil
		}
	})
	h.b.loadOutdated(nil)
	h.loop.until(t, "the slow check to start", func() bool { return f.count("outdated") >= 2 })
	h.b.loadOutdated(nil)
	h.loop.until(t, "the newer check", func() bool { return h.b.updates.loading == 0 })
	release <- []homebrew.Package{{Name: "stale", Version: "0"}}
	h.loop.settle()
	if got := itemKeys(h.b.Source().Items()); !reflect.DeepEqual(got, []string{"fresh"}) {
		t.Errorf("items = %v, want the newer check's", got)
	}

	// Installed: the same for the inventory.
	inv := make(chan []homebrew.Package)
	f.set(func(f *fakeBrew) {
		first := true
		f.formulaeHook = func() ([]homebrew.Package, error) {
			f.mu.Lock()
			slow := first
			first = false
			f.mu.Unlock()
			if slow {
				return <-inv, nil
			}
			return []homebrew.Package{{Name: "new", Version: "1"}}, nil
		}
	})
	n := f.count("formulae")
	h.b.RefreshApps()
	h.loop.until(t, "the slow list to start", func() bool { return f.count("formulae") > n })
	h.b.RefreshApps()
	h.loop.until(t, "the newer list", func() bool { return h.b.inv.loading == 0 })
	inv <- []homebrew.Package{{Name: "old", Version: "0"}}
	h.loop.settle()
	if _, ok := h.b.inv.find(homebrew.Formula, "new"); !ok || len(h.b.inv.formulae) != 1 {
		t.Errorf("formulae = %+v, want the newer list's", h.b.inv.formulae)
	}

	// Search: its own generations, which an inventory reload never retires.
	wait := make(chan struct{})
	f.set(func(f *fakeBrew) {
		f.searchHook = func(q string) ([]homebrew.SearchResult, error) {
			if q == "old" {
				<-wait
				return []homebrew.SearchResult{{Name: "old", Kind: homebrew.Formula}}, nil
			}
			return []homebrew.SearchResult{{Name: q, Kind: homebrew.Cask}}, nil
		}
	})
	h.b.Search("old")
	h.loop.until(t, "the slow search to start", func() bool { return f.count("search old") == 1 })
	h.b.Search("new")
	h.b.RefreshApps()
	h.loop.until(t, "the newer search", func() bool { return h.b.search.loading == 0 })
	close(wait)
	h.idle()
	h.loop.settle()
	if r := h.b.search.results; len(r) != 1 || r[0].Name != "new" || h.b.search.searched != "new" {
		t.Errorf("results = %+v, want the newer search's", r)
	}
}

func TestBrewSearchShowsBothKindsAndInstallsTyped(t *testing.T) {
	f := newFakeBrew()
	f.search["docker"] = []homebrew.SearchResult{{Name: "docker", Kind: homebrew.Formula}, {Name: "docker", Kind: homebrew.Cask}}
	h := startBrew(t, f, allBrewGroups...)
	tt := ui.NewTester(h.b.DrawSearch, 1000, 700)
	if err := tt.Click("Search Homebrew"); err != nil {
		t.Fatal(err)
	}
	tt.Type("docker")
	h.loop.settle()
	if f.count("search docker") != 0 {
		t.Fatal("the search ran while typing; it runs on Enter")
	}
	tt.Key(0, ui.KeyEnter)
	h.idle()
	tt.Frame()
	for _, want := range []string{"2 results for “docker”", "Formula", "Cask", "brew install docker", "brew install --cask docker"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	_ = h.toasts()
	formulae := f.count("formulae")
	if err := tt.Click("Install docker (cask)"); err != nil {
		t.Fatal(err)
	}
	if texts := h.dialogTexts(); !slices.Contains(texts, "Install docker?") || !slices.Contains(texts, "brew install --cask docker") {
		t.Errorf("confirmation = %q", texts)
	}
	h.confirm("Install")
	h.jobsDone()
	if f.count("install --cask docker") != 1 || f.count("install docker") != 0 {
		t.Errorf("calls = %v, want the cask installed with --cask", f.calls)
	}
	if got := h.toasts(); !slices.Contains(got, "success: docker installed") {
		t.Errorf("toasts = %q", got)
	}
	h.idle()
	if f.count("formulae") != formulae+1 {
		t.Error("a live install did not reload the installed packages")
	}
	tt.Frame()
	if !tt.HasText("Installed") {
		t.Errorf("the installed result does not say so: %q", tt.Texts())
	}
}

func TestBrewSearchInstallDryRunAndFailureReopen(t *testing.T) {
	for _, tc := range []struct {
		name  string
		dry   bool
		err   error
		toast string
	}{
		{"dry run", true, nil, "info: [DRY-RUN] Preview: lazygit would be installed — no changes made"},
		{"failure", false, errors.New("no bottle"), "error: Install failed: no bottle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeBrew()
			f.dry = tc.dry
			f.errs["install lazygit"] = tc.err
			f.search["lazygit"] = []homebrew.SearchResult{{Name: "lazygit", Kind: homebrew.Formula}}
			h := startBrew(t, f, allBrewGroups...)
			h.b.Search("lazygit")
			h.idle()
			formulae := f.count("formulae")
			h.b.askInstall(h.b.search.results[0])
			h.confirm("Install")
			h.jobsDone()
			if got := h.toasts(); !slices.Contains(got, tc.toast) {
				t.Errorf("toasts = %q, want %q", got, tc.toast)
			}
			if a := h.b.search.rows["formula:lazygit"]; a.inUse() {
				t.Errorf("the install control stays closed: %+v", a)
			}
			h.idle()
			if f.count("formulae") != formulae {
				t.Error("an install that changed nothing reloaded the inventory")
			}
		})
	}
}

func TestBrewRowActionsConfirmAndShareOneGate(t *testing.T) {
	f := newFakeBrew()
	h := startBrew(t, f, allBrewGroups...)
	b := h.b

	b.Act("formula:gh", ActPin)
	if texts := h.dialogTexts(); !slices.Contains(texts, "Pin gh?") {
		t.Fatalf("pin asks %q", texts)
	}
	b.Act("formula:gh", ActUninstall) // the row's gate is taken by the pin
	b.Act("formula:gh", ActUpdate)
	h.confirm("Cancel")
	if h.env.Dialogs.Showing() {
		t.Fatalf("a second action of the row asked while the first did: %q", h.dialogTexts())
	}
	if h.env.Jobs.Busy() {
		t.Fatal("an update started while the row's pin was asked")
	}

	b.Act("formula:gh", ActUninstall)
	if texts := h.dialogTexts(); !slices.Contains(texts, "Uninstall gh?") ||
		!slices.Contains(texts, "Homebrew will remove the formula gh and its package-managed files.") {
		t.Fatalf("uninstall asks %q", texts)
	}
	h.confirm("Uninstall")
	if e := h.entry("formula:gh"); !e.Busy || b.StateLabel("formula:gh") != "Uninstalling…" {
		t.Errorf("gh is not busy while it uninstalls: %+v", e)
	}
	b.Act("formula:gh", ActPin)
	b.Source().Update("gh")
	if h.env.Dialogs.Showing() {
		t.Error("the row asked again while its uninstall runs")
	}
	f.set(func(f *fakeBrew) { f.formulae = f.formulae[:1] })
	inv := f.count("formulae")
	h.loop.until(t, "the uninstall", func() bool { return h.b.inv.rows["formula:gh"].completed != "" })
	if e := h.entry("formula:gh"); len(e.Actions) != 0 || b.StateLabel("formula:gh") != "Uninstalled" {
		t.Errorf("after a live uninstall the row keeps controls: %+v", e)
	}
	h.jobsDone()
	h.idle()
	if f.count("uninstall gh") != 1 || f.count("upgrade gh") != 0 {
		t.Errorf("calls = %v", f.calls)
	}
	if f.count("formulae") <= inv {
		t.Error("a live uninstall did not reload the inventory")
	}
	for _, e := range b.Apps() {
		if e.Key == "formula:gh" {
			t.Errorf("gh is still listed after the reload: %+v", e)
		}
	}

	// An upgrade in flight closes the row's other controls too.
	f.set(func(f *fakeBrew) { f.formulae = []homebrew.Package{{Name: "gh", Version: "2.82.0"}} })
	f.set(func(f *fakeBrew) { f.outdated = []homebrew.Package{{Name: "gh", Version: "2.82.0"}} })
	b.RefreshApps()
	b.Source().Refresh()
	h.idle()
	b.Source().Update("gh")
	b.Act("formula:gh", ActUninstall)
	if h.env.Dialogs.Showing() {
		t.Error("uninstall asked while gh upgrades")
	}
	h.jobsDone()
}

func TestBrewRowActionDryRunAndFailureReopen(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dry    bool
		err    error
		toasts []string
	}{
		{"dry run", true, nil, []string{"info: [DRY-RUN] Preview: gh would be pinned — no changes made"}},
		{"failure", false, errors.New("locked"), []string{"error: Pin failed: locked"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeBrew()
			f.dry = tc.dry
			f.errs["pin gh"] = tc.err
			h := startBrew(t, f, allBrewGroups...)
			_ = h.toasts()
			inv := f.count("formulae")
			h.b.Act("formula:gh", ActPin)
			h.confirm("Pin")
			h.jobsDone()
			if got := h.toasts(); !reflect.DeepEqual(got, tc.toasts) {
				t.Errorf("toasts = %q, want %q", got, tc.toasts)
			}
			if e := h.entry("formula:gh"); e.Busy || !slices.Contains(e.Actions, ActPin) {
				t.Errorf("the row's controls did not reopen: %+v", e)
			}
			h.idle()
			if f.count("formulae") != inv {
				t.Error("an action that changed nothing reloaded the inventory")
			}
		})
	}

	// A live unpin completes the row until the reload.
	f := newFakeBrew()
	h := startBrew(t, f, allBrewGroups...)
	h.b.Act("formula:go", ActUnpin)
	if texts := h.dialogTexts(); !slices.Contains(texts, "Unpin go?") {
		t.Fatalf("unpin asks %q", texts)
	}
	h.confirm("Unpin")
	h.loop.until(t, "the unpin", func() bool { return h.b.StateLabel("formula:go") == "Unpinned" })
	h.jobsDone()
	h.idle()
	if f.count("unpin go") != 1 {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestBrewBundlesWorkWithoutBrewGroup(t *testing.T) {
	f := newFakeBrew()
	h := startBrew(t, f, "brew_bundles_group")
	b := h.b
	if len(b.bundles.items) != 2 {
		t.Fatalf("bundles = %+v", b.bundles.items)
	}
	cli := b.bundles.items[0]
	if cli.Name != "cli" || cli.Description != "Recommended CLI Tools" || len(cli.packages) != 21 {
		t.Errorf("cli bundle = %s %q %d packages", cli.Name, cli.Description, len(cli.packages))
	}
	if b.bundles.presentation.Description != "2 Brew bundles available" {
		t.Errorf("presentation = %+v", b.bundles.presentation)
	}

	f.dry = true
	b.AskBundle(cli.Path)
	if texts := h.dialogTexts(); !slices.Contains(texts, "Install the cli bundle?") {
		t.Fatalf("bundle asks %q", texts)
	}
	h.confirm("Install")
	h.jobsDone()
	if cli.installed || !cli.gate.TryStart() {
		t.Error("a dry-run bundle install closed the card")
	}
	cli.gate.Reset()
	if got := h.toasts(); !slices.Contains(got, "info: [DRY-RUN] Preview: Brew bundle cli would be installed — no changes made") {
		t.Errorf("toasts = %q", got)
	}

	f.set(func(f *fakeBrew) { f.dry = false })
	b.AskBundle(cli.Path)
	h.confirm("Install")
	h.jobsDone()
	h.loop.settle()
	if !cli.installed {
		t.Error("a live bundle install did not mark the card installed")
	}
	if got := h.toasts(); !slices.Contains(got, "success: Brew bundle cli installed") {
		t.Errorf("toasts = %q", got)
	}
	if f.count("bundle install "+cli.Path) != 2 {
		t.Errorf("calls = %v", f.calls)
	}
	if f.count("formulae") != 0 {
		t.Error("a bundle install reloaded the inventory with brew_group off")
	}
	b.AskBundle(cli.Path)
	if h.env.Dialogs.Showing() {
		t.Error("an installed bundle asked again")
	}

	f2 := newFakeBrew()
	f2.errs = map[string]error{}
	h2 := startBrew(t, f2, "brew_bundles_group", "brew_group")
	path := h2.b.bundles.items[1].Path
	f2.errs["bundle install "+path] = errors.New("cask failed")
	h2.b.AskBundle(path)
	h2.confirm("Install")
	h2.jobsDone()
	if got := h2.toasts(); !slices.Contains(got, "error: Could not install Brew bundle fonts: cask failed") {
		t.Errorf("toasts = %q", got)
	}
	if h2.b.bundles.items[1].installed || !h2.b.bundles.items[1].gate.TryStart() {
		t.Error("a failed bundle install did not reopen its card")
	}
}

func TestBrewTrustBannerFlow(t *testing.T) {
	banner := "Homebrew skips bbrew because valkyrie00/bbrew is not trusted. Trust the tap to update its packages again."
	for _, tc := range []struct {
		name      string
		dry       bool
		err       error
		wantGone  bool
		wantToast string
	}{
		{"live", false, nil, true, "success: Trusted valkyrie00/bbrew. Its packages can update again."},
		{"dry run", true, nil, false, "info: [DRY-RUN] Preview: valkyrie00/bbrew would be trusted — no changes made"},
		{"failure", false, errors.New("no"), false, "error: Failed to trust valkyrie00/bbrew: no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeBrew()
			f.dry = tc.dry
			f.errs["trust valkyrie00/bbrew valkyrie00/bbrew/bbrew"] = tc.err
			h := startBrew(t, f, allBrewGroups...)
			_ = h.toasts()
			tt := ui.NewTester(h.b.Source().Extra, 1000, 400)
			if !tt.HasText(banner) {
				t.Fatalf("no banner in %q", tt.Texts())
			}
			if err := tt.Click("Trust tap"); err != nil {
				t.Fatal(err)
			}
			if texts := h.dialogTexts(); !slices.Contains(texts, "Trust packages from valkyrie00/bbrew?") || !slices.Contains(texts, "1 installed: bbrew") {
				t.Fatalf("trust asks %q", texts)
			}
			outdated := f.count("outdated")
			h.confirm("Trust")
			h.jobsDone()
			h.idle()
			if got := h.toasts(); !slices.Contains(got, tc.wantToast) {
				t.Errorf("toasts = %q, want %q", got, tc.wantToast)
			}
			tt.Frame()
			if gone := !tt.HasText(banner); gone != tc.wantGone {
				t.Errorf("banner gone = %v, want %v", gone, tc.wantGone)
			}
			if reloaded := f.count("outdated") > outdated; reloaded != tc.wantGone {
				t.Errorf("outdated reloaded = %v, want %v", reloaded, tc.wantGone)
			}
			if !tc.wantGone {
				if a := h.b.updates.trusts["valkyrie00/bbrew"]; a.inUse() {
					t.Error("the trust control stays closed")
				}
			}
		})
	}

	// Trusting needs only brew_trust_group: with the updates group off it
	// reloads nothing it does not have.
	f := newFakeBrew()
	h := startBrew(t, f, "brew_trust_group")
	h.b.AskTrust("valkyrie00/bbrew")
	h.confirm("Trust")
	h.jobsDone()
	h.idle()
	if f.count("outdated") != 0 || len(h.b.updates.taps) != 0 {
		t.Errorf("calls = %v taps = %v", f.calls, h.b.updates.taps)
	}
}

func TestBrewTrustBannerCloses(t *testing.T) {
	h := startBrew(t, newFakeBrew(), allBrewGroups...)
	tt := ui.NewTester(h.b.Source().Extra, 1000, 400)
	if err := tt.Click("Dismiss"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Trust tap") {
		t.Error("the closed banner still shows")
	}
	if !strings.Contains(h.b.Source().Status().Note, "outdated") {
		t.Error("closing the banner changed the note")
	}
	h.b.Source().Refresh()
	h.idle()
	tt.Frame()
	if !tt.HasText("Trust tap") {
		t.Error("a new check does not show the banner again")
	}
}

func TestBrewMetadataUpdate(t *testing.T) {
	for _, tc := range []struct {
		name        string
		dry         bool
		err         error
		wantToast   string
		wantRefresh bool
	}{
		{"live", false, nil, "success: Homebrew updated successfully", true},
		{"dry run", true, nil, "info: [DRY-RUN] Preview: Homebrew would be updated — no changes made", false},
		{"failure", false, errors.New("git"), "error: Update failed: git", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeBrew()
			f.dry = tc.dry
			f.errs["update"] = tc.err
			h := startBrew(t, f, allBrewGroups...)
			_ = h.toasts()
			tt := ui.NewTester(h.b.Source().Extra, 1000, 400)
			outdated := f.count("outdated")
			if err := tt.Click("Update Homebrew metadata"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if !tt.HasText("Updating…") {
				t.Errorf("the control does not say it runs: %q", tt.Texts())
			}
			h.b.UpdateMetadata() // a second click does nothing
			h.jobsDone()
			if f.count("update") != 1 {
				t.Errorf("calls = %v", f.calls)
			}
			if tc.wantRefresh && !h.b.updates.meta.inUse() {
				t.Error("a live update reopened its control before the reload")
			}
			h.idle()
			if h.b.updates.meta.inUse() {
				t.Error("the control stays closed")
			}
			if got := h.toasts(); !slices.Contains(got, tc.wantToast) {
				t.Errorf("toasts = %q, want %q", got, tc.wantToast)
			}
			if refreshed := f.count("outdated") > outdated; refreshed != tc.wantRefresh {
				t.Errorf("reloaded = %v, want %v", refreshed, tc.wantRefresh)
			}
		})
	}
}

func TestBrewExportAsksAndDumps(t *testing.T) {
	f := newFakeBrew()
	h := startBrew(t, f, allBrewGroups...)
	h.b.ExportBrewfile()
	if texts := h.dialogTexts(); !slices.Contains(texts, "Export installed packages?") {
		t.Fatalf("export asks %q", texts)
	}
	h.confirm("Export")
	h.jobsDone()
	if f.count("bundle dump /home/frosty/Brewfile") != 1 {
		t.Errorf("calls = %v", f.calls)
	}
	if got := h.toasts(); !slices.Contains(got, "success: Brewfile saved to /home/frosty/Brewfile") {
		t.Errorf("toasts = %q", got)
	}
}

func TestBrewCommands(t *testing.T) {
	h := startBrew(t, newFakeBrew(), allBrewGroups...)
	var navigated string
	h.env.SetNavigate(func(page string) { navigated = page })
	cmds := h.b.Commands()
	byID := map[string]int{}
	for i, c := range cmds {
		byID[c.ID] = i
	}
	for _, id := range []string{"brew.metadata", "brew.upgrade-all", "brew.trust.valkyrie00/bbrew", "brew.search", "brew.export"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("no command %q in %+v", id, cmds)
		}
	}
	if c := cmds[byID["brew.trust.valkyrie00/bbrew"]]; c.Title != "Trust valkyrie00/bbrew" {
		t.Errorf("trust command = %+v", c)
	}
	bundles := 0
	for _, c := range cmds {
		if strings.HasPrefix(c.ID, "brew.bundle.") {
			bundles++
		}
	}
	if bundles != 2 {
		t.Errorf("bundle commands = %d, want 2", bundles)
	}
	cmds[byID["brew.search"]].Run()
	if navigated != navigation.Apps || !h.b.TakeSearchRequest() || h.b.TakeSearchRequest() {
		t.Errorf("search navigated to %q and asked for the tab once", navigated)
	}
	cmds[byID["brew.upgrade-all"]].Run()
	h.jobsDone()
	h.idle()
	if h.f.count("upgrade gh") != 1 {
		t.Errorf("upgrade all ran %v", h.f.calls)
	}
}
