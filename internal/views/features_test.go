package views

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/updex"
	updexapi "github.com/frostyard/updex/v2/updex"
)

// featuresUIThread stands in for the UI thread: Env.Post queues functions, and
// the test runs them with settle.
type featuresUIThread struct{ queue chan func() }

func newFeaturesUIThread() *featuresUIThread { return &featuresUIThread{queue: make(chan func(), 256)} }

func (u *featuresUIThread) post(fn func()) { u.queue <- fn }

// settle runs posted functions until none arrives for a while: the
// backends' goroutines have ended and their results are applied.
func (u *featuresUIThread) settle(t *testing.T) {
	t.Helper()
	for {
		select {
		case fn := <-u.queue:
			fn()
		case <-time.After(60 * time.Millisecond):
			return
		}
	}
}

// fakeUpdex records what the Features domain asks of updex. It never runs
// pkexec, the helper or updex itself.
type fakeUpdex struct {
	mu        sync.Mutex
	calls     []string
	missing   bool
	features  []updex.Feature
	listErr   error
	checks    []updex.FeatureCheck
	checkErr  error
	toggleErr error
	results   []updex.FeatureUpdate
	updateErr error
	// hold, when set, keeps enable, disable and update running until it
	// is closed.
	hold chan struct{}
}

func (u *fakeUpdex) record(call string) {
	u.mu.Lock()
	u.calls = append(u.calls, call)
	u.mu.Unlock()
}

// Calls returns the calls so far, as "list", "check", "enable docker",
// "disable dev" and "update".
func (u *fakeUpdex) Calls() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.calls...)
}

func (u *fakeUpdex) count(call string) int {
	n := 0
	for _, c := range u.Calls() {
		if c == call {
			n++
		}
	}
	return n
}

// wait holds a change until hold closes or the job is canceled, as the
// helper's process is killed with its context.
func (u *fakeUpdex) wait(ctx context.Context) error {
	if u.hold == nil {
		return nil
	}
	select {
	case <-u.hold:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (u *fakeUpdex) install(f *Features) {
	f.installed = func() bool { return !u.missing }
	f.list = func(context.Context) ([]updex.Feature, error) {
		u.record("list")
		u.mu.Lock()
		defer u.mu.Unlock()
		return append([]updex.Feature(nil), u.features...), u.listErr
	}
	f.check = func(context.Context) ([]updex.FeatureCheck, error) {
		u.record("check")
		u.mu.Lock()
		defer u.mu.Unlock()
		return append([]updex.FeatureCheck(nil), u.checks...), u.checkErr
	}
	f.enable = func(ctx context.Context, name string) error {
		u.record("enable " + name)
		if err := u.wait(ctx); err != nil {
			return err
		}
		return u.toggleErr
	}
	f.disable = func(ctx context.Context, name string) error {
		u.record("disable " + name)
		if err := u.wait(ctx); err != nil {
			return err
		}
		return u.toggleErr
	}
	f.update = func(ctx context.Context) ([]updex.FeatureUpdate, error) {
		u.record("update")
		if err := u.wait(ctx); err != nil {
			return nil, err
		}
		return u.results, u.updateErr
	}
}

// set changes what the fake answers, as the system would after a change.
func (u *fakeUpdex) set(fn func(u *fakeUpdex)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	fn(u)
}

func fakeFeature(name, description string, on bool, version string) updex.Feature {
	f := updex.Feature{Name: name, Description: description, Enabled: on}
	if version != "" {
		f.Transfers = []string{name}
		f.TransferStatus = []updexapi.TransferStatus{{Component: name, RunningVersion: version, LinkedVersion: version}}
	}
	return f
}

// snowFeatures are features as Snow Linux defines them, two of them on.
func snowFeatures() []updex.Feature {
	return []updex.Feature{
		fakeFeature("docker", "Docker Containers", true, "28.5.1"),
		fakeFeature("dev", "Development Tools", true, "2026.10"),
		fakeFeature("incus", "Incus", false, ""),
		fakeFeature("tailscale", "Tailscale VPN", false, ""),
		fakeFeature("podman", "Podman Containers", false, ""),
		fakeFeature("nix", "Nix Package Manager", false, ""),
	}
}

// snowChecks: docker has an update, dev is current.
func snowChecks() []updex.FeatureCheck {
	return []updex.FeatureCheck{
		{Feature: "docker", Results: []updex.CheckResult{{Component: "docker", CurrentVersion: "28.5.1", NewestVersion: "28.5.2", UpdateAvailable: true}}},
		{Feature: "dev", Results: []updex.CheckResult{{Component: "dev", CurrentVersion: "2026.10", NewestVersion: "2026.10"}}},
	}
}

type featuresHarness struct {
	env  *Env
	ui   *featuresUIThread
	fake *fakeUpdex
	f    *Features
	page Page
}

func newFeaturesHarness(t *testing.T, cfg *config.Config, dryRun bool, fake *fakeUpdex) *featuresHarness {
	t.Helper()
	frostui.SetIcons(frostui.NewIcons(nil))
	u := newFeaturesUIThread()
	env := NewEnv(cfg, dryRun, u.post)
	env.Now = func() time.Time { return time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC) }
	f := NewFeatures(env)
	fake.install(f)
	return &featuresHarness{env: env, ui: u, fake: fake, f: f, page: NewFeaturesPage(&Store{Env: env, Features: f})}
}

func (h *featuresHarness) start(t *testing.T) {
	t.Helper()
	h.f.Start()
	h.ui.settle(t)
}

// view draws the page and the confirmations, as the window does.
func (h *featuresHarness) view(c *ui.Context) {
	frostui.Adapt(c)
	ui.Column(c).Fill().Padding(24, 28).Gap(18).AlignItems(ui.Stretch).Background(c.Theme().Background).Children(func() {
		h.page.View(c)
	})
	h.env.Dialogs.View(c)
}

func (h *featuresHarness) toasts() []string {
	var out []string
	for _, t := range h.env.TakeToasts() {
		out = append(out, t.Type+": "+t.Title)
	}
	return out
}

func enabledFeatures() *config.Config {
	return &config.Config{FeaturesPage: config.PageConfig{"features_group": {Enabled: true}}}
}

func TestFeaturesBackendsAreTheFixedHelperCalls(t *testing.T) {
	f := NewFeatures(NewEnv(enabledFeatures(), false, func(fn func()) { fn() }))
	same := func(a, b any) bool { return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer() }
	for name, pair := range map[string][2]any{
		"installed": {f.installed, updex.IsInstalledCached},
		"list":      {f.list, updex.ListFeatures},
		"check":     {f.check, updex.CheckFeatures},
		"enable":    {f.enable, updex.EnableFeature},
		"disable":   {f.disable, updex.DisableFeature},
		"update":    {f.update, updex.UpdateFeatures},
	} {
		if !same(pair[0], pair[1]) {
			t.Errorf("%s is not internal/updex's own function", name)
		}
	}
}

func TestFeaturesListThenCheck(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: snowChecks()})
	h.start(t)

	if got := h.fake.Calls(); strings.Join(got, ",") != "list,check" {
		t.Fatalf("calls = %v, want list then check", got)
	}
	if got := h.f.Summary(); got != (FeatureSummary{Known: true, On: 2, Total: 6}) {
		t.Errorf("Summary = %+v", got)
	}
	if got := h.f.groupDescription(); got != "6 features available (1 update)" {
		t.Errorf("description = %q", got)
	}
	if got := h.f.subtitle("docker"); got != "docker — update available for docker (v28.5.1 → v28.5.2)" {
		t.Errorf("docker subtitle = %q", got)
	}
	if got := h.f.subtitle("incus"); got != "Off" {
		t.Errorf("incus subtitle = %q, want its base subtitle", got)
	}
	if badge, tone := h.page.Badge(); badge != "1" || tone != frostui.Accent {
		t.Errorf("Badge = %q %v", badge, tone)
	}

	tt := ui.NewTester(h.view, 1360, 2000)
	for _, want := range []string{"Features", "2 of 6 on · system extensions that add to /usr without changing the image", "Docker Containers", "docker", "On · 28.5.1 · update ready", "Update available for docker (v28.5.1 → v28.5.2)", "Update features", "Check for updates", "6 features available (1 update)"} {
		if !tt.HasText(want) {
			t.Errorf("page lacks %q; it shows %q", want, tt.Texts())
		}
	}
	if sw, ok := tt.AccessByRole(ui.RoleSwitch, "Docker Containers (docker)"); !ok || !sw.Checked {
		t.Errorf("docker switch = %+v, %v", sw, ok)
	}
	if sw, ok := tt.AccessByRole(ui.RoleSwitch, "Incus (incus)"); !ok || sw.Checked {
		t.Errorf("incus switch = %+v, %v", sw, ok)
	}
}

func TestFeaturesToggleAsksBeforeItRuns(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: snowChecks()})
	h.start(t)
	tt := ui.NewTester(h.view, 1360, 2000)

	if err := tt.Click("Incus"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Turn on Incus?") || !tt.HasText("Needs administrator authentication.") {
		t.Fatalf("no confirmation: %q", tt.Texts())
	}
	if h.fake.count("enable incus") != 0 {
		t.Fatal("enable ran before the user agreed")
	}
	if row, _ := h.f.row("incus"); row.On {
		t.Fatal("incus flipped before the job ran")
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	h.ui.settle(t)
	if h.fake.count("enable incus") != 0 {
		t.Fatal("enable ran after Cancel")
	}

	if err := tt.Click("Incus"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Turn on"); err != nil {
		t.Fatal(err)
	}
	h.ui.settle(t)
	tt.Frame()
	if h.fake.count("enable incus") != 1 {
		t.Fatalf("calls = %v, want one enable incus", h.fake.Calls())
	}
	if got := h.toasts(); len(got) != 1 || got[0] != "info: incus enabled. Update to download, reboot to apply." {
		t.Errorf("toasts = %q", got)
	}
	// After the change the page lists and checks again.
	if h.fake.count("list") != 2 || h.fake.count("check") != 2 {
		t.Errorf("calls = %v, want a second list and check", h.fake.Calls())
	}
}

func TestFeaturesTurnOffRunsDisable(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: snowChecks()})
	h.start(t)
	tt := ui.NewTester(h.view, 1360, 2000)

	// The list's switch asks as the tile does.
	if err := tt.Click("Development Tools (dev)"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Turn off Development Tools?") {
		t.Fatalf("no confirmation: %q", tt.Texts())
	}
	if err := tt.Click("Turn off"); err != nil {
		t.Fatal(err)
	}
	h.ui.settle(t)
	if got := h.fake.Calls(); !slices.Contains(got, "disable dev") || slices.Contains(got, "enable dev") {
		t.Errorf("calls = %v, want disable dev", got)
	}
	if got := h.toasts(); len(got) != 1 || got[0] != "info: dev disabled. Update to apply, reboot to complete." {
		t.Errorf("toasts = %q", got)
	}
}

func TestFeaturesToggleShowsBusyWhileItRuns(t *testing.T) {
	hold := make(chan struct{})
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: snowChecks(), hold: hold})
	h.start(t)
	tt := ui.NewTester(h.view, 1360, 2000)

	h.f.AskToggle("incus", true)
	tt.Frame()
	if err := tt.Click("Turn on"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !h.f.busy("incus") {
		t.Fatal("no job pending for incus")
	}
	if !tt.HasText("Working…") || !tt.HasText("Working on Incus") {
		t.Errorf("no busy state: %q", tt.Texts())
	}
	// A second ask while the job runs asks nothing.
	h.f.AskToggle("incus", true)
	if h.env.Dialogs.Showing() {
		t.Error("asked again while the job runs")
	}
	close(hold)
	h.ui.settle(t)
	if h.f.busy("incus") {
		t.Error("incus is still busy")
	}
}

func TestFeaturesCanceledJobs(t *testing.T) {
	hold := make(chan struct{})
	defer close(hold)
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: snowChecks(), hold: hold})
	h.start(t)

	h.f.toggle("incus", "Incus", true)
	h.f.UpdateAll()
	if !h.f.busy("incus") || !h.f.updating() {
		t.Fatal("the jobs are not pending")
	}
	h.env.Jobs.CancelAll()
	h.ui.settle(t)
	if h.fake.count("update") != 0 {
		t.Error("the dropped update ran")
	}
	got := h.toasts()
	if !slices.Contains(got, "info: Canceled: Updating features") || !slices.Contains(got, "info: Canceled: Turning on Incus") {
		t.Errorf("toasts = %q", got)
	}
	// The canceled toggle reads back what the system has.
	if h.fake.count("list") != 2 {
		t.Errorf("calls = %v, want a second listing", h.fake.Calls())
	}
}

func TestFeaturesToggleDryRunChangesNothing(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), true, &fakeUpdex{features: snowFeatures(), checks: snowChecks()})
	h.start(t)

	h.f.AskToggle("incus", true)
	tt := ui.NewTester(h.view, 1360, 2000)
	if !tt.HasText("Dry run: ChairLift only logs the command it would run.") {
		t.Errorf("the dry-run confirmation says %q", tt.Texts())
	}
	if err := tt.Click("Turn on"); err != nil {
		t.Fatal(err)
	}
	h.ui.settle(t)
	if row, _ := h.f.row("incus"); row.On {
		t.Error("a dry run turned incus on")
	}
	if got := h.toasts(); len(got) != 1 || got[0] != "info: [DRY-RUN] Preview: incus would be enabled — no changes made" {
		t.Errorf("toasts = %q", got)
	}
	if h.fake.count("list") != 1 {
		t.Errorf("a dry run listed again: %v", h.fake.Calls())
	}
}

func TestFeaturesToggleFailureKeepsTheState(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: snowChecks(), toggleErr: errors.New("not authorized")})
	h.start(t)

	h.f.AskToggle("docker", false)
	tt := ui.NewTester(h.view, 1360, 2000)
	if err := tt.Click("Turn off"); err != nil {
		t.Fatal(err)
	}
	h.ui.settle(t)
	tt.Frame()
	if row, _ := h.f.row("docker"); !row.On {
		t.Error("a failed disable turned docker off")
	}
	if sw, ok := tt.AccessByRole(ui.RoleSwitch, "Docker Containers (docker)"); !ok || !sw.Checked {
		t.Errorf("docker switch = %+v, %v; want it on again", sw, ok)
	}
	if got := h.toasts(); len(got) != 1 || got[0] != "error: Failed to update docker: not authorized" {
		t.Errorf("toasts = %q", got)
	}
	if h.fake.count("list") != 1 {
		t.Errorf("a failure listed again: %v", h.fake.Calls())
	}
}

func TestFeaturesMaskedCannotBeToggled(t *testing.T) {
	features := snowFeatures()
	features[2].Masked = true
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: features, checks: snowChecks()})
	h.start(t)
	h.f.AskToggle("incus", true)
	if h.env.Dialogs.Showing() {
		t.Error("asked to turn on a masked feature")
	}
	for _, cmd := range h.page.Commands() {
		if strings.Contains(cmd.ID, "incus") {
			t.Errorf("command %q for a masked feature", cmd.ID)
		}
	}
}

func TestFeaturesUpdateRunsTheHelperOnceAndRechecks(t *testing.T) {
	results := []updex.FeatureUpdate{{Feature: "docker", Results: []updex.TransferUpdate{{Component: "docker", Version: "28.5.2", Downloaded: true, Linked: true}}}}
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: snowChecks(), results: results})
	h.start(t)
	tt := ui.NewTester(h.view, 1360, 2000)

	if err := tt.Click("Update features"); err != nil {
		t.Fatal(err)
	}
	// The row's button queues nothing more while the update waits.
	h.f.UpdateAll()
	h.ui.settle(t)
	if h.fake.count("update") != 1 {
		t.Fatalf("calls = %v, want one update", h.fake.Calls())
	}
	if got := h.toasts(); len(got) != 1 || got[0] != "info: Features updated. Changes apply after reboot." {
		t.Errorf("toasts = %q", got)
	}
	if h.fake.count("check") != 2 {
		t.Errorf("calls = %v, want a check after the update", h.fake.Calls())
	}
}

func TestFeaturesUpdateActivationIncomplete(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{
		features: snowFeatures(), checks: snowChecks(),
		updateErr: &updex.ActivationIncompleteError{Message: "command failed (exit 3)"},
	})
	h.start(t)
	h.f.UpdateAll()
	h.ui.settle(t)
	if got := h.toasts(); len(got) != 1 || got[0] != "error: Features merged, but activation did not finish. Reboot to finish." {
		t.Errorf("toasts = %q", got)
	}
	if h.fake.count("check") != 2 {
		t.Errorf("calls = %v, want a check after the update", h.fake.Calls())
	}
}

func TestFeaturesUpdateDryRunDoesNotRecheck(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), true, &fakeUpdex{features: snowFeatures(), checks: snowChecks()})
	h.start(t)
	h.f.UpdateAll()
	h.ui.settle(t)
	if got := h.toasts(); len(got) != 1 || got[0] != "info: [DRY-RUN] Preview: features would be updated — no changes made" {
		t.Errorf("toasts = %q", got)
	}
	if h.fake.count("check") != 1 {
		t.Errorf("a dry run checked again: %v", h.fake.Calls())
	}
}

func TestFeaturesCheckGenerationGate(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: snowChecks()})
	h.start(t)

	older := h.f.featureCheck.Begin()
	newer := h.f.featureCheck.Begin()
	// The newer check says docker is current now.
	current := []updex.FeatureCheck{{Feature: "docker", Results: []updex.CheckResult{{Component: "docker", CurrentVersion: "28.5.2", NewestVersion: "28.5.2"}}}}
	h.f.checkFeatureUpdates(newer, current, nil)
	if got := h.f.subtitle("docker"); got != "docker — v28.5.2" {
		t.Fatalf("docker subtitle = %q", got)
	}
	// The older check ends late with its update and publishes nothing.
	h.f.checkFeatureUpdates(older, snowChecks(), nil)
	if got := h.f.subtitle("docker"); got != "docker — v28.5.2" {
		t.Errorf("a stale check replaced the rows: %q", got)
	}
	if n := len(h.f.updatable()); n != 0 {
		t.Errorf("updatable = %d after the stale check", n)
	}
	// A failed check keeps the rows and says it failed.
	failed := h.f.featureCheck.Begin()
	h.f.checkFeatureUpdates(failed, nil, errors.New("offline"))
	if got := h.f.groupDescription(); got != "6 features available — update check failed" {
		t.Errorf("description = %q", got)
	}
	if got := h.f.subtitle("docker"); got != "docker — v28.5.2" {
		t.Errorf("a failed check replaced the rows: %q", got)
	}
}

func TestFeaturesRecheckRestoresBaseSubtitles(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: snowChecks()})
	h.start(t)
	if !strings.Contains(h.f.subtitle("docker"), "update available") {
		t.Fatalf("docker subtitle = %q", h.f.subtitle("docker"))
	}
	// The next check does not answer for docker: its row reads its base
	// subtitle again, not the earlier "update available".
	h.fake.set(func(u *fakeUpdex) { u.checks = snowChecks()[1:] })
	h.f.Refresh()
	h.ui.settle(t)
	if got := h.f.subtitle("docker"); got != "On · 28.5.1" {
		t.Errorf("docker subtitle = %q, want its base subtitle", got)
	}
	if got := h.f.groupDescription(); got != "6 features available — all up to date" {
		t.Errorf("description = %q", got)
	}
}

func TestFeaturesSourceItemsAndJobs(t *testing.T) {
	checks := append(snowChecks(), updex.FeatureCheck{Feature: "dev", Results: []updex.CheckResult{
		{Component: "dev-a", CurrentVersion: "1", NewestVersion: "2", UpdateAvailable: true},
		{Component: "dev-b", CurrentVersion: "1", NewestVersion: "3", UpdateAvailable: true},
	}})
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: checks})
	h.start(t)
	src := h.f.Source()

	st := src.Status()
	if !st.Available || st.Checking || st.Err != "" || st.CheckedAt.IsZero() {
		t.Errorf("Status = %+v", st)
	}
	if st.Note != "2 of 6 on · one update covers them all" {
		t.Errorf("Note = %q", st.Note)
	}
	items := src.Items()
	if len(items) != 2 {
		t.Fatalf("Items = %+v", items)
	}
	if got := items[0]; got.Key != "docker" || got.Title != "Docker Containers" || got.From != "28.5.1" || got.To != "28.5.2" || got.Action != "Update" {
		t.Errorf("docker item = %+v", got)
	}
	if got := items[1]; got.Key != "dev" || got.From != "" || got.To != "" {
		t.Errorf("dev item = %+v; two components have no single version", got)
	}

	all := src.Jobs()
	some := src.Jobs("dev")
	if len(all) != 1 || len(some) != 1 || all[0].Key != featuresUpdateKey || all[0].Source != string(SourceFeatures) {
		t.Fatalf("Jobs() = %v, Jobs(dev) = %v; want the one update job", all, some)
	}
	h.env.Jobs.Enqueue(all[0])
	if again := src.Jobs(); len(again) != 0 {
		t.Errorf("Jobs while the update waits = %v", again)
	}
	h.ui.settle(t)
	if h.fake.count("update") != 1 {
		t.Errorf("calls = %v", h.fake.Calls())
	}
}

func TestFeaturesCommands(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: snowFeatures(), checks: snowChecks()})
	h.start(t)
	byTitle := map[string]frostui.Command{}
	for _, cmd := range h.page.Commands() {
		byTitle[cmd.Title] = cmd
	}
	for _, want := range []string{"Turn off Docker Containers", "Turn on Incus", "Turn on Tailscale VPN", "Check features for updates", "Update features"} {
		if _, ok := byTitle[want]; !ok {
			t.Errorf("no command %q in %v", want, byTitle)
		}
	}
	if byTitle["Update features"].Disabled {
		t.Error("Update features is off although docker has an update")
	}
	byTitle["Turn on Incus"].Run()
	if !h.env.Dialogs.Showing() || h.fake.count("enable incus") != 0 {
		t.Error("the command did not ask first")
	}
}

func TestFeaturesGroupDisabledLoadsNothing(t *testing.T) {
	cfg := &config.Config{FeaturesPage: config.PageConfig{"features_group": {Enabled: false}}}
	h := newFeaturesHarness(t, cfg, false, &fakeUpdex{features: snowFeatures(), checks: snowChecks()})
	h.start(t)
	h.f.Refresh()
	h.ui.settle(t)
	if calls := h.fake.Calls(); len(calls) != 0 {
		t.Errorf("a disabled group called %v", calls)
	}
	if h.f.Source().Status().Available || h.f.Summary().Known || len(h.page.Commands()) != 0 {
		t.Error("a disabled group still offers its source, summary or commands")
	}
	if badge, _ := h.page.Badge(); badge != "" {
		t.Errorf("badge = %q", badge)
	}
}

func TestFeaturesUnavailableWithoutUpdex(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{missing: true})
	h.start(t)
	if h.fake.count("list") != 0 {
		t.Error("listed features without updex")
	}
	if h.f.Source().Status().Available {
		t.Error("the source applies without updex")
	}
	tt := ui.NewTester(h.view, 1360, 880)
	if !tt.HasText("Feature Manager Not Available") {
		t.Errorf("page shows %q", tt.Texts())
	}
}

func TestFeaturesListFailure(t *testing.T) {
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{listErr: errors.New("failed to list features: boom")})
	h.start(t)
	tt := ui.NewTester(h.view, 1360, 880)
	if !tt.HasText("Error: failed to list features: boom") {
		t.Errorf("page shows %q", tt.Texts())
	}
	if h.fake.count("check") != 0 {
		t.Error("checked without a listing")
	}
	if st := h.f.Source().Status(); st.Err == "" {
		t.Errorf("Status = %+v", st)
	}
	// Trying again lists again.
	h.fake.set(func(u *fakeUpdex) { u.listErr, u.features, u.checks = nil, snowFeatures(), snowChecks() })
	if err := tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	h.ui.settle(t)
	if !h.f.Summary().Known {
		t.Error("the second listing did not load")
	}
}

func TestFeatureLine(t *testing.T) {
	for _, tc := range []struct{ subtitle, line, version string }{
		{"docker — v28.5.1", "Up to date", "28.5.1"},
		{"docker — up to date", "Up to date", ""},
		{"docker — update available for docker (v1 → v2)", "Update available for docker (v1 → v2)", ""},
		{"docker — waiting for a newer OS image (docker: needs image >= 2026.11)", "Waiting for a newer OS image (docker: needs image >= 2026.11)", ""},
		{"Off", "Off", ""},
		{"", "", ""},
	} {
		line, version := featureLine("docker", tc.subtitle)
		if line != tc.line || version != tc.version {
			t.Errorf("featureLine(%q) = %q, %q; want %q, %q", tc.subtitle, line, version, tc.line, tc.version)
		}
	}
}

func TestFeatureVersion(t *testing.T) {
	status := func(running, linked string) updexapi.TransferStatus {
		return updexapi.TransferStatus{RunningVersion: running, LinkedVersion: linked}
	}
	for _, tc := range []struct {
		name     string
		statuses []updexapi.TransferStatus
		want     string
	}{
		{"none", nil, ""},
		{"running", []updexapi.TransferStatus{status("1.2", "1.3"), status("1.2", "1.2")}, "1.2"},
		{"unknown running falls back to linked", []updexapi.TransferStatus{status("unknown", "1.3")}, "1.3"},
		{"not merged falls back to linked", []updexapi.TransferStatus{status("", "1.3"), status("", "1.3")}, "1.3"},
		{"disagreeing", []updexapi.TransferStatus{status("1", "1"), status("2", "2")}, ""},
	} {
		if got := featureVersion(updex.Feature{Name: "x", TransferStatus: tc.statuses}); got != tc.want {
			t.Errorf("%s: featureVersion = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSnapshotFeatures(t *testing.T) {
	checks := append(snowChecks(), updex.FeatureCheck{Feature: "dev", Results: []updex.CheckResult{
		{Component: "dev", CurrentVersion: "2026.10", NewestVersion: "2026.11", UpdateAvailable: true},
	}})
	features := snowFeatures()
	features = append(features, fakeFeature("bitwarden", "Bitwarden password manager desktop application", false, ""))
	for _, dark := range []bool{false, true} {
		h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: features, checks: checks})
		h.start(t)
		name := "features-light"
		if dark {
			name = "features-dark"
		}
		tt := ui.NewTesterWith(h.view, 1128, 1240, ui.TesterOptions{Dark: dark})
		snapshot(t, tt, name)
		h.f.AskToggle("incus", true)
		tt.Frame()
		snapshot(t, tt, name+"-dialog")
	}

	// The other states: a feature waiting for a newer OS image, one whose
	// change applies after a reboot, a masked one and one being turned on.
	hold := make(chan struct{})
	defer close(hold)
	states := snowFeatures()
	states[5].Masked = true
	h := newFeaturesHarness(t, enabledFeatures(), false, &fakeUpdex{features: states, hold: hold, checks: []updex.FeatureCheck{
		{Feature: "docker", Results: []updex.CheckResult{{Component: "docker", CurrentVersion: "28.5.1", NewestVersion: "28.5.2", NeedsImage: "needs image >= 2026.11.20"}}},
		{Feature: "dev", Results: []updex.CheckResult{{Component: "dev", CurrentVersion: "2026.11", NewestVersion: "2026.11", PendingReboot: true}}},
	}})
	h.start(t)
	h.f.toggle("podman", "Podman Containers", true)
	tt := ui.NewTesterWith(h.view, 1128, 1100, ui.TesterOptions{Dark: true})
	if !tt.HasText("Waiting for a newer OS image (docker: needs image >= 2026.11.20)") || !tt.HasText("Changes apply after reboot") {
		t.Errorf("states page shows %q", tt.Texts())
	}
	snapshot(t, tt, "features-states")
}
