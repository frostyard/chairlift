package views

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/frostui"
)

// The e2e fixtures (test/e2e/testdata/problems): 16 system problems and 7
// app problems for the crash fixture's uid.
const (
	fixtureUID     = 60999
	fixtureSystem  = 16
	fixtureApps    = 7
	fixtureVersion = "0.12.0"
)

var problemsTestNow = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

// syncBuffer is a log destination the worker goroutines may write to while
// the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// fakeSender records every body it is given; it never touches a network.
type fakeSender struct {
	mu     sync.Mutex
	bodies [][]byte
	err    error
}

func (f *fakeSender) Send(_ context.Context, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies = append(f.bodies, append([]byte(nil), body...))
	return f.err
}

func (f *fakeSender) sent() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]byte(nil), f.bodies...)
}

// problemsHarness runs a Problems state against the fixtures, in a
// temporary home, with a fake sender, and runs what the workers post on
// the test goroutine (the UI thread of the test).
type problemsHarness struct {
	t        *testing.T
	env      *Env
	p        *Problems
	page     Page
	posts    chan func()
	home     string
	fixtures string
	sender   *fakeSender
	logs     *syncBuffer
	loads    int
	tt       *ui.Tester
}

var allProblemGroups = []string{"problems_system_group", "problems_apps_group", "problems_reporting_group"}

func newProblemsHarness(t *testing.T, dryRun bool, groups ...string) *problemsHarness {
	t.Helper()
	frostui.SetIcons(frostui.NewIcons(nil))
	logs := &syncBuffer{}
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	fixtures, err := filepath.Abs(filepath.Join("..", "..", "test", "e2e", "testdata", "problems"))
	if err != nil {
		t.Fatal(err)
	}
	// A group the configuration does not name is enabled: name all three.
	page := config.PageConfig{}
	for _, g := range allProblemGroups {
		page[g] = config.GroupConfig{Enabled: false}
	}
	for _, g := range groups {
		page[g] = config.GroupConfig{Enabled: true}
	}
	h := &problemsHarness{t: t, posts: make(chan func(), 64), home: t.TempDir(), fixtures: fixtures, sender: &fakeSender{}, logs: logs}
	h.env = NewEnv(&config.Config{ProblemsPage: page}, dryRun, func(fn func()) { h.posts <- fn })
	h.env.Now = func() time.Time { return problemsTestNow }

	p := NewProblems(h.env)
	p.getenv = func(key string) string {
		switch key {
		case "HOME":
			return h.home
		case "XDG_CONFIG_HOME":
			return filepath.Join(h.home, "config")
		case "XDG_STATE_HOME":
			return filepath.Join(h.home, "state")
		case problems.FixturesEnv:
			return h.fixtures
		}
		return ""
	}
	p.uid = fixtureUID
	p.loc = time.UTC
	p.version = fixtureVersion
	p.pace = 0
	p.newSender = func(string) reporting.Sender { return h.sender }
	p.installInfo = func(context.Context) (string, string, bool) { return "<6m", "latest", true }
	// A live run reads the fixtures too: no test runs coredumpctl or
	// journalctl. The dry-run path keeps problems.DecideFixtures' decision.
	p.loadSnap = func(ctx context.Context, opts problems.Options) problems.Snapshot {
		h.loads++
		if !dryRun {
			opts.Fixtures = problems.FixtureDecision{Use: true, Dir: fixtures}
		}
		return problems.Load(ctx, opts)
	}
	h.p = p
	h.page = &problemsView{p: p}
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(24).Gap(18).AlignItems(ui.Stretch).Children(func() { h.page.View(c) })
	}, 1360, 2600)
	return h
}

// waitFor runs the workers' posts until cond holds.
func (h *problemsHarness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case fn := <-h.posts:
			fn()
		case <-deadline:
			h.t.Fatalf("timed out waiting for %s", what)
		}
	}
	h.tt.Frame()
}

// start starts the load and waits for it.
func (h *problemsHarness) start() {
	h.t.Helper()
	h.p.Start()
	h.waitFor("the first load", func() bool {
		return (h.p.loaded || !h.p.dataOn()) && (h.p.reportingLoaded || !h.p.reportingOn) && !h.p.loading
	})
}

func (h *problemsHarness) click(label string) {
	h.t.Helper()
	if err := h.tt.Click(label); err != nil {
		h.t.Fatalf("click %q: %v\n%q", label, err, h.tt.Texts())
	}
}

// clickSwitch clicks the consent switch, which has no text of its own.
func (h *problemsHarness) clickSwitch() {
	h.t.Helper()
	sw, ok := h.tt.AccessByRole(ui.RoleSwitch, pageview.ReportingSwitchTitle)
	if !ok {
		h.t.Fatal("no consent switch")
	}
	h.tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
}

func (h *problemsHarness) toasts() []string {
	var out []string
	for _, t := range h.env.TakeToasts() {
		out = append(out, t.Title)
	}
	return out
}

// item returns the first shown item matching ok.
func (h *problemsHarness) item(ok func(problems.Item) bool) problems.Item {
	h.t.Helper()
	for _, it := range append(append([]problems.Item(nil), h.p.system...), h.p.apps...) {
		if ok(it) {
			return it
		}
	}
	h.t.Fatal("no item matches")
	return problems.Item{}
}

// openOnly expands it alone, so its actions are the only ones on screen.
func (h *problemsHarness) openOnly(it problems.Item) string {
	h.p.open = map[string]bool{it.ID: true}
	h.tt.Frame()
	return pageview.ProblemCardText(it, problemsTestNow, time.UTC).Title
}

func (h *problemsHarness) saveConsent(enabled bool, mode reporting.Mode) {
	h.t.Helper()
	path, err := reporting.ConsentPath(h.p.getenv)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := reporting.SaveConsent(path, reporting.Decide(enabled, mode, problemsTestNow)); err != nil {
		h.t.Fatal(err)
	}
}

func reportableNotCrash(it problems.Item) bool {
	return it.Reportable && it.Kind != problems.KindCrash
}

func TestProblemsLoadTheFixturesInADryRun(t *testing.T) {
	h := newProblemsHarness(t, true, allProblemGroups...)
	h.start()
	if len(h.p.system) != fixtureSystem || len(h.p.apps) != fixtureApps {
		t.Fatalf("system=%d apps=%d, want %d and %d", len(h.p.system), len(h.p.apps), fixtureSystem, fixtureApps)
	}
	marker := "problems: page built system=16 apps=7 source=ok"
	if !strings.Contains(h.logs.String(), marker+"\n") {
		t.Errorf("the log has no %q:\n%s", marker, h.logs.String())
	}
	if got := strings.Count(h.logs.String(), "problems: page built"); got != 1 {
		t.Errorf("the marker was logged %d times", got)
	}
	want := ProblemSummary{Known: true, Count: fixtureSystem + fixtureApps, Attention: fixtureSystem}
	if got := h.p.Summary(); got != want {
		t.Errorf("Summary = %+v, want %+v", got, want)
	}
	if badge, tone := h.page.Badge(); badge != "23" || tone != frostui.Warning {
		t.Errorf("Badge = %q %v", badge, tone)
	}
	for _, text := range []string{
		"Problems",
		"23 recorded on this computer, in plain words. Nothing leaves it unless you turn reports on.",
		pageview.ProblemsSystemSection, pageview.ProblemsSystemSectionNote,
		pageview.ProblemsAppsSection, pageview.ProblemsAppsSectionNote,
		"System files failed an integrity check",
		pageview.ReportingCardTitle, pageview.ReportingOff,
	} {
		if !h.tt.HasText(text) {
			t.Errorf("the page does not show %q", text)
		}
	}
	// The consent is off by default: no card offers Report….
	for _, it := range append(append([]problems.Item(nil), h.p.system...), h.p.apps...) {
		if h.p.reportOffered(it) {
			t.Errorf("%s offers a report while reporting is off", it.ID)
		}
	}
}

func TestProblemsFixturesNeedADryRun(t *testing.T) {
	h := newProblemsHarness(t, false, "problems_system_group")
	var got problems.Options
	h.p.loadSnap = func(_ context.Context, opts problems.Options) problems.Snapshot {
		got = opts
		return problems.Snapshot{}
	}
	h.start()
	if got.Fixtures.Use {
		t.Error("a live run used the fixtures")
	}
	if !strings.Contains(h.logs.String(), problems.FixturesEnv+" ignored: it needs --dry-run") {
		t.Errorf("the log does not say the fixtures were ignored:\n%s", h.logs.String())
	}
	if !strings.Contains(h.logs.String(), "problems: page built system=0 apps=0 source=missing") {
		t.Errorf("no marker for the missing source:\n%s", h.logs.String())
	}
	if !got.System || got.Apps || got.UID != fixtureUID || !got.Now.Equal(problemsTestNow) {
		t.Errorf("Load options = %+v", got)
	}
	if !h.tt.HasText(pageview.ProblemsNotAvailable) {
		t.Errorf("the system section does not say %q", pageview.ProblemsNotAvailable)
	}
}

func TestProblemsDisabledGroupsLoadNothing(t *testing.T) {
	h := newProblemsHarness(t, true)
	h.p.Start()
	if !strings.Contains(h.logs.String(), "problems: page built system=0 apps=0 source=disabled") {
		t.Errorf("no marker at once:\n%s", h.logs.String())
	}
	select {
	case <-h.posts:
		t.Error("a load ran with every group off")
	case <-time.After(50 * time.Millisecond):
	}
	if h.loads != 0 || h.p.Summary().Known {
		t.Errorf("loads=%d summary=%+v", h.loads, h.p.Summary())
	}
	if badge, _ := h.page.Badge(); badge != "" {
		t.Errorf("Badge = %q", badge)
	}
	if cmds := h.page.Commands(); len(cmds) != 0 {
		t.Errorf("Commands = %+v", cmds)
	}
}

func TestProblemsReportingAloneLoadsTheConsentOnly(t *testing.T) {
	h := newProblemsHarness(t, false, "problems_reporting_group")
	h.saveConsent(true, reporting.ModeAsk)
	h.start()
	if h.loads != 0 {
		t.Errorf("the problems were loaded %d times with both lists off", h.loads)
	}
	if !h.p.reportingLoaded || !h.p.consentCopy().Allows() {
		t.Errorf("the consent was not loaded: loaded=%t %+v", h.p.reportingLoaded, h.p.consentCopy())
	}
	if !strings.Contains(h.logs.String(), "problems: page built system=0 apps=0 source=disabled") {
		t.Errorf("no marker:\n%s", h.logs.String())
	}
	// The user can still withdraw the consent.
	h.clickSwitch()
	if h.p.consentCopy().Allows() {
		t.Error("the switch did not turn reporting off")
	}
}

func TestProblemsSystemGroupAloneSkipsTheApps(t *testing.T) {
	h := newProblemsHarness(t, true, "problems_system_group")
	h.start()
	// Without the apps group no system crash moves there (the merge rule):
	// the fixtures' merged crash stays a system problem.
	if len(h.p.system) != fixtureSystem+1 || len(h.p.apps) != 0 {
		t.Errorf("system=%d apps=%d", len(h.p.system), len(h.p.apps))
	}
	if !strings.Contains(h.logs.String(), "problems: page built system=17 apps=0 source=ok") {
		t.Errorf("marker:\n%s", h.logs.String())
	}
	if h.tt.HasText(pageview.ProblemsAppsSection) || h.tt.HasText(pageview.ReportingCardTitle) {
		t.Error("a disabled group is drawn")
	}
}

func TestProblemsDismissInADryRunChangesNothing(t *testing.T) {
	h := newProblemsHarness(t, true, allProblemGroups...)
	h.start()
	it := h.p.system[0]
	title := h.openOnly(it)
	h.click("Dismiss")
	toasts := h.toasts()
	if len(toasts) != 1 || toasts[0] != "[DRY-RUN] Preview: dismiss "+title+" — no changes made" {
		t.Errorf("toasts = %q", toasts)
	}
	if !strings.Contains(h.logs.String(), "[DRY-RUN] would dismiss problem "+it.ID) {
		t.Errorf("no dry-run log line:\n%s", h.logs.String())
	}
	if len(h.p.system) != fixtureSystem || h.p.systemHidden != 0 {
		t.Errorf("a dry run hid a problem: %d shown, %d hidden", len(h.p.system), h.p.systemHidden)
	}
	if _, err := os.Stat(filepath.Join(h.home, "state", "chairlift", "problems-seen.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a dry run wrote the dismiss store: %v", err)
	}
	if len(h.p.dismissed) != 0 {
		t.Errorf("a dry run remembered a dismissal: %v", h.p.dismissed)
	}
	// A reload still shows it.
	h.p.Reload()
	h.waitFor("the reload", func() bool { return !h.p.loading })
	if len(h.p.system) != fixtureSystem || h.p.systemHidden != 0 || h.p.system[0].ID != it.ID {
		t.Errorf("after a reload a dry-run dismissal hid a problem: %d shown, %d hidden", len(h.p.system), h.p.systemHidden)
	}
}

// TestProblemsLoadBegunBeforeADismissalKeepsItHidden: a reload that read
// the problems before a live dismissal publishes after it. The dismissed
// problem stays hidden, and the automatic send that the reload starts does
// not consider it. A reload begun after the dismissal read the dismiss
// store, which decides from then on.
func TestProblemsLoadBegunBeforeADismissalKeepsItHidden(t *testing.T) {
	h := newProblemsHarness(t, false, allProblemGroups...)
	h.saveConsent(true, reporting.ModeAuto)
	var mu sync.Mutex
	var runs []reporting.RunInput
	h.p.runReports = func(_ context.Context, in reporting.RunInput) reporting.RunResult {
		mu.Lock()
		defer mu.Unlock()
		runs = append(runs, in)
		return reporting.RunResult{}
	}
	runCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(runs)
	}
	h.start()
	h.waitFor("the automatic run after the first load", func() bool { return runCount() == 1 && !h.p.reportingRunning })

	var it problems.Item
	for _, s := range h.p.system {
		if reportableNotCrash(s) {
			it = s
			break
		}
	}
	if it.ID == "" {
		t.Fatal("the fixtures hold no reportable system problem")
	}

	// The reload reads the problems, the dismissed one among them, then
	// waits before it hands them back.
	read, release := make(chan struct{}, 1), make(chan struct{})
	load := h.p.loadSnap
	h.p.loadSnap = func(ctx context.Context, opts problems.Options) problems.Snapshot {
		snap := load(ctx, opts)
		read <- struct{}{}
		<-release
		return snap
	}
	h.p.Reload()
	select {
	case <-read:
	case <-time.After(10 * time.Second):
		t.Fatal("the reload did not read the problems")
	}

	h.openOnly(it)
	h.click("Dismiss")
	h.waitFor("the dismissal", func() bool { return !h.p.dismissing[it.ID] })
	if h.p.systemHidden != 1 {
		t.Fatalf("the dismissal did not hide the problem: %d hidden", h.p.systemHidden)
	}

	close(release)
	h.waitFor("the reload", func() bool { return !h.p.loading })
	h.waitFor("the automatic run after the reload", func() bool { return runCount() == 2 && !h.p.reportingRunning })
	for _, shown := range append(append([]problems.Item(nil), h.p.system...), h.p.apps...) {
		if shown.ID == it.ID {
			t.Fatal("a load begun before the dismissal showed the problem again")
		}
	}
	if h.p.systemHidden != 1 || !h.tt.HasText("1 dismissed problem is hidden") {
		t.Errorf("hidden = %d, want the 1 dismissal", h.p.systemHidden)
	}
	if got := h.p.Summary().Count; got != fixtureSystem+fixtureApps-1 {
		t.Errorf("Count = %d", got)
	}
	mu.Lock()
	auto := runs[1]
	mu.Unlock()
	if len(auto.Items) != fixtureSystem+fixtureApps-1 {
		t.Errorf("the automatic run considered %d problems", len(auto.Items))
	}
	for _, considered := range append(append([]problems.Item(nil), auto.Items...), h.p.reportingInput().Items...) {
		if considered.ID == it.ID {
			t.Fatal("an automatic send considers the dismissed problem")
		}
	}
	if _, ok := h.p.dismissed[it.ID]; !ok {
		t.Error("the dismissal was forgotten by a load that began before it")
	}

	// A reload begun after the dismissal read the dismiss store: it decides,
	// and the dismissal held in memory is forgotten.
	h.p.loadSnap = load
	h.p.Reload()
	h.waitFor("the later reload", func() bool { return !h.p.loading })
	for _, shown := range h.p.system {
		if shown.ID == it.ID {
			t.Fatal("the later reload showed the dismissed problem")
		}
	}
	if h.p.systemHidden != 1 || len(h.p.dismissed) != 0 {
		t.Errorf("after the later reload: %d hidden, remembered %v", h.p.systemHidden, h.p.dismissed)
	}
}

// TestProblemsLiveDismissalsBindOnlyLoadsBegunBefore: a remembered
// dismissal hides its problem from a load begun before it, never from one
// begun after it, which read the dismiss store (a problem that happened
// again shows again); that load's publication forgets it.
func TestProblemsLiveDismissalsBindOnlyLoadsBegunBefore(t *testing.T) {
	p := &Problems{dismissed: map[string]uint64{"0000000000000001": 2}}
	items := []problems.Item{{ID: "0000000000000001"}, {ID: "0000000000000002"}}
	if got, n := p.withoutLiveDismissals(items, 2); n != 1 || len(got) != 1 || got[0].ID != "0000000000000002" {
		t.Errorf("a load begun before the dismissal: %d dropped, %+v", n, got)
	}
	if got, n := p.withoutLiveDismissals(items, 3); n != 0 || len(got) != 2 {
		t.Errorf("a load begun after the dismissal: %d dropped, %+v", n, got)
	}
	p.forgetLiveDismissals(2)
	if len(p.dismissed) != 1 {
		t.Error("a load begun before the dismissal forgot it")
	}
	p.forgetLiveDismissals(3)
	if len(p.dismissed) != 0 {
		t.Error("a load begun after the dismissal did not forget it")
	}
}

func TestProblemsDismissHidesAfterTheWrite(t *testing.T) {
	h := newProblemsHarness(t, false, allProblemGroups...)
	h.start()
	it := h.p.system[0]
	title := h.openOnly(it)
	h.click("Dismiss")
	if !h.p.dismissing[it.ID] {
		t.Fatal("the dismissal did not start")
	}
	h.waitFor("the dismissal", func() bool { return !h.p.dismissing[it.ID] })
	if len(h.p.system) != fixtureSystem-1 || h.p.systemHidden != 1 {
		t.Errorf("%d shown, %d hidden", len(h.p.system), h.p.systemHidden)
	}
	for _, rest := range h.p.reportingItems {
		if rest.ID == it.ID {
			t.Error("a dismissed problem stays reportable")
		}
	}
	if toasts := h.toasts(); len(toasts) != 1 || toasts[0] != "Dismissed: "+title {
		t.Errorf("toasts = %q", toasts)
	}
	seen, err := problems.LoadSeen(filepath.Join(h.home, "state", "chairlift", "problems-seen.json"))
	if err != nil || !seen.Hidden(it) {
		t.Errorf("the dismiss store does not hide it: %v", err)
	}
	if !h.tt.HasText("1 dismissed problem is hidden") {
		t.Error("the section does not count the hidden problem")
	}
	if got := h.p.Summary().Count; got != fixtureSystem+fixtureApps-1 {
		t.Errorf("Count = %d", got)
	}
}

func TestProblemsCopyDetails(t *testing.T) {
	h := newProblemsHarness(t, true, allProblemGroups...)
	h.start()
	it := h.p.apps[0]
	title := h.openOnly(it)
	h.click("Copy details")
	if got := h.tt.Clipboard(); got != problems.DetailsText(it) {
		t.Errorf("clipboard = %q, want the details text", got)
	}
	if toasts := h.toasts(); len(toasts) != 1 || toasts[0] != "Copied the details of "+title {
		t.Errorf("toasts = %q", toasts)
	}
}

func TestProblemsCardShowsItsDetails(t *testing.T) {
	h := newProblemsHarness(t, true, allProblemGroups...)
	h.start()
	it := h.p.system[0]
	h.openOnly(it)
	details := pageview.ProblemDetails(it)
	for _, row := range details {
		if !h.tt.HasText(row.Subtitle) {
			t.Errorf("the open card does not show %q", row.Subtitle)
		}
	}
	text := pageview.ProblemCardText(it, problemsTestNow, time.UTC)
	for _, s := range []string{text.Subtitle, text.Count, text.FirstSeen} {
		if !h.tt.HasText(s) {
			t.Errorf("the card does not show %q", s)
		}
	}
	// A click on the header closes it.
	h.click(text.Title)
	if h.p.open[it.ID] {
		t.Error("the card did not close")
	}
}

func TestProblemsControlsWaitForTheFirstLoad(t *testing.T) {
	h := newProblemsHarness(t, false, allProblemGroups...)
	h.saveConsent(true, reporting.ModeAuto)
	h.p.runReports = func(context.Context, reporting.RunInput) reporting.RunResult { return reporting.RunResult{} }
	h.p.Start()
	h.tt.Frame()
	sw, ok := h.tt.AccessByRole(ui.RoleSwitch, pageview.ReportingSwitchTitle)
	if !ok || !sw.Disabled || sw.Checked {
		t.Errorf("before the load the switch is %+v, want off and disabled", sw)
	}
	// A change before the load is ignored and writes nothing.
	h.p.onReportingSwitched(true)
	h.p.onReportingModeSelected(reporting.ModeAsk)
	if h.p.consentCopy().Allows() {
		t.Error("a change before the load took effect")
	}
	h.waitFor("the first load", func() bool { return h.p.loaded && h.p.reportingLoaded })
	sw, _ = h.tt.AccessByRole(ui.RoleSwitch, pageview.ReportingSwitchTitle)
	if sw.Disabled || !sw.Checked {
		t.Errorf("after the load the switch is %+v, want on and enabled", sw)
	}
}

func TestProblemsConsentIsSavedAndModeChanges(t *testing.T) {
	h := newProblemsHarness(t, false, allProblemGroups...)
	var runs []reporting.RunInput
	h.p.runReports = func(_ context.Context, in reporting.RunInput) reporting.RunResult {
		runs = append(runs, in)
		return reporting.RunResult{}
	}
	h.start()
	path, _ := reporting.ConsentPath(h.p.getenv)

	h.clickSwitch()
	stored, err := reporting.LoadConsent(path)
	if err != nil || !stored.Allows() || stored.Mode != reporting.ModeAsk {
		t.Fatalf("after On the file holds %+v (%v)", stored, err)
	}
	if toasts := h.toasts(); len(toasts) != 1 || toasts[0] != "Problem reporting is on" {
		t.Errorf("toasts = %q", toasts)
	}
	h.tt.Frame()
	if !h.tt.HasText("On: Ask each time.") {
		t.Error("the card does not say reporting is on")
	}
	reportable := h.item(reportableNotCrash)
	if !h.p.reportOffered(reportable) {
		t.Error("a reportable problem offers no report while reporting is on")
	}
	if len(runs) != 0 {
		t.Error("Ask mode started an automatic run")
	}

	h.click(pageview.ReportingModeAuto)
	stored, _ = reporting.LoadConsent(path)
	if stored.Mode != reporting.ModeAuto {
		t.Errorf("after Auto the file holds %+v", stored)
	}
	h.waitFor("the automatic run", func() bool { return !h.p.reportingRunning })
	if len(runs) != 1 || !runs[0].GroupOn || runs[0].DryRun || runs[0].Sender == nil || runs[0].Pace != 0 {
		t.Fatalf("automatic runs = %+v", runs)
	}
	if len(runs[0].Items) != fixtureSystem+fixtureApps || !runs[0].Envelope.Valid {
		t.Errorf("the run got %d items, envelope %+v", len(runs[0].Items), runs[0].Envelope)
	}
	// The live gate reads the file first: a withdrawal in the file stops it.
	if err := reporting.SaveConsent(path, reporting.Decide(false, reporting.ModeAuto, problemsTestNow)); err != nil {
		t.Fatal(err)
	}
	if runs[0].Consent().Allows() {
		t.Error("the live consent gate ignores the file")
	}
	if err := reporting.SaveConsent(path, stored); err != nil {
		t.Fatal(err)
	}

	h.clickSwitch()
	stored, _ = reporting.LoadConsent(path)
	if stored.Allows() || h.p.consentCopy().Allows() {
		t.Errorf("after Off: file %+v, memory %+v", stored, h.p.consentCopy())
	}
}

func TestProblemsConsentInADryRunWritesNothing(t *testing.T) {
	h := newProblemsHarness(t, true, allProblemGroups...)
	var previews int
	h.p.runReports = func(_ context.Context, in reporting.RunInput) reporting.RunResult {
		if in.DryRun {
			previews++
		}
		return reporting.RunResult{}
	}
	h.start()
	h.clickSwitch()
	if toasts := h.toasts(); len(toasts) != 1 || toasts[0] != "[DRY-RUN] Preview: problem reporting on — no changes made" {
		t.Errorf("toasts = %q", toasts)
	}
	if !strings.Contains(h.logs.String(), "[DRY-RUN] would save problem reporting enabled=true mode=ask") {
		t.Errorf("no dry-run line:\n%s", h.logs.String())
	}
	path, _ := reporting.ConsentPath(h.p.getenv)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a dry run wrote the consent: %v", err)
	}
	if !h.p.consentCopy().Allows() {
		t.Error("a dry run does not keep the choice in memory")
	}
	// A new automatic choice previews its reports at once.
	h.tt.Frame()
	h.click(pageview.ReportingModeAuto)
	h.waitFor("the preview run", func() bool { return !h.p.reportingRunning })
	if previews != 1 {
		t.Errorf("previews = %d, want 1", previews)
	}
}

func TestProblemsReportDialogSendsTheBytesItShows(t *testing.T) {
	h := newProblemsHarness(t, false, allProblemGroups...)
	h.saveConsent(true, reporting.ModeAsk)
	h.start()
	it := h.item(reportableNotCrash)
	h.openOnly(it)
	h.click(pageview.ReportButtonLabel)
	h.waitFor("the report dialog", func() bool { return h.p.dialog.open })
	shown := h.p.dialog.report
	if !h.tt.HasText(shown.Text()) {
		t.Error("the dialog does not show the report's text")
	}
	if !h.tt.HasText(pageview.ReportSummary(shown.Kind)) {
		t.Error("the dialog does not say what the report is")
	}
	if len(h.sender.sent()) != 0 {
		t.Fatal("something was sent before Send")
	}
	h.click("Send")
	h.waitFor("the send", func() bool { return len(h.p.reportBusy) == 0 })
	bodies := h.sender.sent()
	if len(bodies) != 1 || !bytes.Equal(bodies[0], shown.Body) || string(bodies[0]) != shown.Text() {
		t.Fatalf("sent %d bodies; the first equals the shown bytes: %t", len(bodies), len(bodies) == 1 && bytes.Equal(bodies[0], shown.Body))
	}
	if !h.p.reported[it.ID] || h.p.dialog.open {
		t.Errorf("reported=%t dialog open=%t", h.p.reported[it.ID], h.p.dialog.open)
	}
	if !h.tt.HasText(pageview.ReportedLabel) {
		t.Error("the card does not say Sent")
	}
	state := h.p.stateCopy()
	if state == nil || len(state.Sent) != 1 || state.Sent[0].ProblemID != it.ID {
		t.Errorf("the sent list is %+v", state)
	}
	if toasts := h.toasts(); len(toasts) != 1 || toasts[0] != "Sent report "+shown.Kind {
		t.Errorf("toasts = %q", toasts)
	}
	if !h.tt.HasText("1 report sent") {
		t.Error("the card does not count the sent report")
	}
}

func TestProblemsReportDialogCancelSendsNothing(t *testing.T) {
	h := newProblemsHarness(t, false, allProblemGroups...)
	h.saveConsent(true, reporting.ModeAsk)
	h.start()
	h.openOnly(h.item(reportableNotCrash))
	h.click(pageview.ReportButtonLabel)
	h.waitFor("the report dialog", func() bool { return h.p.dialog.open })
	h.click("Cancel")
	if h.p.dialog.open || len(h.sender.sent()) != 0 {
		t.Errorf("open=%t sent=%d", h.p.dialog.open, len(h.sender.sent()))
	}
}

func TestProblemsReportDialogInADryRunSendsNothing(t *testing.T) {
	h := newProblemsHarness(t, true, allProblemGroups...)
	h.p.runReports = func(context.Context, reporting.RunInput) reporting.RunResult { return reporting.RunResult{} }
	h.start()
	h.clickSwitch() // in memory only
	h.toasts()
	it := h.item(reportableNotCrash)
	h.openOnly(it)
	h.click(pageview.ReportButtonLabel)
	h.waitFor("the report dialog", func() bool { return h.p.dialog.open })
	kind := h.p.dialog.report.Kind
	h.click("Send")
	if len(h.sender.sent()) != 0 || h.p.reported[it.ID] {
		t.Error("a dry run sent or marked a report")
	}
	if toasts := h.toasts(); len(toasts) != 1 || toasts[0] != "[DRY-RUN] Preview: report "+kind+" — no changes made" {
		t.Errorf("toasts = %q", toasts)
	}
	if !strings.Contains(h.logs.String(), "[DRY-RUN] would send report "+kind) {
		t.Errorf("no dry-run line:\n%s", h.logs.String())
	}
}

func TestProblemsCrashOffersNoReport(t *testing.T) {
	h := newProblemsHarness(t, false, allProblemGroups...)
	h.saveConsent(true, reporting.ModeAsk)
	h.start()
	crashes := 0
	for _, it := range h.p.apps {
		if it.Kind != problems.KindCrash {
			continue
		}
		crashes++
		if h.p.reportOffered(it) {
			t.Errorf("crash %s offers a report", it.ID)
		}
		h.openOnly(it)
		if h.tt.HasText(pageview.ReportButtonLabel) {
			t.Errorf("the card of crash %s shows %s", it.ID, pageview.ReportButtonLabel)
		}
	}
	if crashes == 0 {
		t.Fatal("the fixtures hold no crash")
	}
	// A reportable problem that is not a crash does show it.
	h.openOnly(h.item(reportableNotCrash))
	if !h.tt.HasText(pageview.ReportButtonLabel) {
		t.Errorf("a reportable problem shows no %s", pageview.ReportButtonLabel)
	}
}

func TestProblemsInvalidEndpointTurnsSendingOff(t *testing.T) {
	h := newProblemsHarness(t, false, allProblemGroups...)
	h.saveConsent(true, reporting.ModeAuto)
	ran := false
	h.p.runReports = func(context.Context, reporting.RunInput) reporting.RunResult {
		ran = true
		return reporting.RunResult{}
	}
	h.p.Start()
	h.p.reportingEndpointErr = errors.New("not a URL")
	h.p.reportingEndpoint = ""
	h.waitFor("the first load", func() bool { return h.p.loaded && h.p.reportingLoaded })
	if ran {
		t.Error("an automatic run started without a valid endpoint")
	}
	if h.p.reportOffered(h.item(reportableNotCrash)) {
		t.Error("a report is offered without a valid endpoint")
	}
	if in := h.p.reportingInput(); in.Sender != nil {
		t.Error("the send input has a sender without a valid endpoint")
	}
	if !h.tt.HasText(pageview.ReportingEndpointInvalid) {
		t.Error("the card does not say the endpoint is invalid")
	}
}

func TestProblemsSendPendingReports(t *testing.T) {
	h := newProblemsHarness(t, false, allProblemGroups...)
	h.saveConsent(true, reporting.ModeAuto)
	var runs int
	h.p.runReports = func(_ context.Context, in reporting.RunInput) reporting.RunResult {
		runs++
		return reporting.RunResult{Skipped: "paused until tomorrow"}
	}
	h.start()
	h.waitFor("the automatic run after the load", func() bool { return runs == 1 && !h.p.reportingRunning })
	h.toasts()
	cmd := findCommand(h.page.Commands(), "problems-send-pending")
	if cmd == nil || cmd.Disabled {
		t.Fatalf("send pending = %+v", cmd)
	}
	h.click(pageview.ReportingSendPending)
	h.waitFor("the manual run", func() bool { return runs == 2 && !h.p.reportingRunning })
	if toasts := h.toasts(); len(toasts) != 1 || toasts[0] != "No report was sent: paused until tomorrow" {
		t.Errorf("toasts = %q", toasts)
	}
	// Ask mode has no such command: only the dialog sends.
	h.click(pageview.ReportingModeAsk)
	if cmd := findCommand(h.page.Commands(), "problems-send-pending"); cmd != nil {
		t.Errorf("Ask mode offers %+v", cmd)
	}
	if h.tt.HasText(pageview.ReportingSendPending) {
		t.Error("Ask mode shows the send button")
	}
}

func TestProblemsCommands(t *testing.T) {
	h := newProblemsHarness(t, true, allProblemGroups...)
	h.p.runReports = func(context.Context, reporting.RunInput) reporting.RunResult { return reporting.RunResult{} }
	h.start()
	reload := findCommand(h.page.Commands(), "problems-reload")
	if reload == nil || reload.Disabled {
		t.Fatalf("reload = %+v", reload)
	}
	toggle := findCommand(h.page.Commands(), "problems-reporting")
	if toggle == nil || toggle.Title != "Turn problem reports on" || toggle.Disabled {
		t.Fatalf("toggle = %+v", toggle)
	}
	toggle.Run()
	if !h.p.consentCopy().Allows() {
		t.Error("the command did not turn reports on")
	}
	if toggle := findCommand(h.page.Commands(), "problems-reporting"); toggle.Title != "Turn problem reports off" {
		t.Errorf("toggle = %+v", toggle)
	}

	// A reload reads the problems again and keeps the consent in memory.
	reload.Run()
	if !h.p.loading {
		t.Fatal("the reload did not start")
	}
	if cmd := findCommand(h.page.Commands(), "problems-reload"); !cmd.Disabled {
		t.Error("reload is offered while a load runs")
	}
	h.waitFor("the reload", func() bool { return !h.p.loading })
	if !h.p.consentCopy().Allows() || len(h.p.system) != fixtureSystem {
		t.Errorf("after the reload: consent %+v, %d system problems", h.p.consentCopy(), len(h.p.system))
	}
	if got := strings.Count(h.logs.String(), "problems: page built system=16 apps=7 source=ok"); got != 2 {
		t.Errorf("the marker was logged %d times after a reload", got)
	}
}

func TestProblemsWeeklyCountReview(t *testing.T) {
	h := newProblemsHarness(t, false, allProblemGroups...)
	h.saveConsent(true, reporting.ModeAsk)
	h.start()
	if !h.tt.HasText(pageview.ReportingCountTitle) {
		t.Fatal("Ask mode shows no weekly count")
	}
	h.click(pageview.ReportingCountAction)
	h.waitFor("the count dialog", func() bool { return h.p.dialog.open })
	if h.p.dialog.report.Kind != problems.KindCount || h.p.dialog.title != pageview.ReportingCountTitle {
		t.Fatalf("dialog = %+v", h.p.dialog)
	}
	shown := h.p.dialog.report.Body
	h.click("Send")
	h.waitFor("the send", func() bool { return len(h.sender.sent()) == 1 })
	h.waitFor("the sent list", func() bool { s := h.p.stateCopy(); return s != nil && s.CountWeek != "" })
	if !bytes.Equal(h.sender.sent()[0], shown) {
		t.Error("the count sent is not the count shown")
	}
	if h.tt.HasText(pageview.ReportingCountAction) {
		t.Error("the count is offered again in the same week")
	}
}

func findCommand(cmds []frostui.Command, id string) *frostui.Command {
	for i := range cmds {
		if cmds[i].ID == id {
			return &cmds[i]
		}
	}
	return nil
}

// TestSnapshotProblems draws the page in the shell with the fixtures: one
// card open, reporting on in Ask mode with a sent report.
func TestSnapshotProblems(t *testing.T) {
	for _, dark := range []bool{false, true} {
		h := newProblemsHarness(t, false, allProblemGroups...)
		h.saveConsent(true, reporting.ModeAsk)
		state := &reporting.State{}
		state.Record(reporting.Entry{ReportID: "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", Kind: problems.KindUnitFailed,
			ProblemID: "27d5ac0722e592bf", Week: "2026-W45", SentAt: problemsTestNow.Add(-26 * time.Hour)})
		statePath, _ := reporting.StatePath(h.p.getenv)
		if err := state.Save(statePath); err != nil {
			t.Fatal(err)
		}
		h.start()
		h.p.open = map[string]bool{h.item(func(it problems.Item) bool { return it.Kind == problems.KindIntegrityError }).ID: true}
		h.p.sentOpen = true

		var sh frostui.Shell
		sh.Brand = frostui.Brand{Name: "ChairLift", Tagline: "Snow Linux command center"}
		sh.Selected = "problems"
		view := func(c *ui.Context) {
			badge, tone := h.page.Badge()
			sh.View(c, frostui.Frame{
				Nav: []frostui.NavSection{{Title: "System", Items: []frostui.NavItem{
					{ID: "overview", Title: "Overview", Icon: "view-grid"},
					{ID: "problems", Title: "Problems", Icon: "dialog-warning", Badge: badge, BadgeTone: tone},
				}}},
				Content:   func() { h.page.View(c) },
				Commands:  h.page.Commands(),
				StatusEnd: []frostui.StatusItem{{Icon: "dialog-warning", Text: "23 problems", Tone: frostui.Warning, Progress: -1}},
			})
		}
		tt := ui.NewTesterWith(view, 1440, 920, ui.TesterOptions{Dark: dark})
		name := "problems-light"
		if dark {
			name = "problems-dark"
		}
		snapshot(t, tt, name)
		if !dark {
			// The whole page, and a narrow window where the reporting card
			// wraps under the lists.
			tt.SetSize(1440, 2400)
			tt.Frame()
			snapshot(t, tt, name+"-full")
			tt.SetSize(980, 2600)
			tt.Frame()
			snapshot(t, tt, name+"-narrow")
			tt.SetSize(1440, 920)
		}

		h.p.sentOpen = false
		h.p.open = map[string]bool{}
		h.p.openReportDialog("A system service failed", mustReport(t, h.item(reportableNotCrash)), nil)
		tt.Frame()
		snapshot(t, tt, name+"-dialog")
	}
}

func mustReport(t *testing.T, it problems.Item) reporting.Report {
	t.Helper()
	r, err := reporting.ForItem(it, "chairlift/"+fixtureVersion)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
