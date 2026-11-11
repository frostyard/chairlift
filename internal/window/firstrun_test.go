package window

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"log"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/firstrun"
	"github.com/frostyard/chairlift/internal/i18n"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/frostui"
)

// firstRunApps is a list in first-setup's shape: Chromium starts off,
// every other app on.
const firstRunApps = `{
	"browsers": [{"name": "Firefox", "id": "org.mozilla.firefox"}, {"name": "Chromium", "id": "org.chromium.Chromium", "active": false}],
	"utilities": [{"name": "Flatseal", "id": "com.github.tchx84.Flatseal"}, {"name": "Extension Manager", "id": "com.mattjakeman.ExtensionManager"}],
	"office": [{"name": "LibreOffice", "id": "org.libreoffice.LibreOffice"}]
}`

// frOptions are what a first-run fixture starts with.
type frOptions struct {
	online    bool
	apps      bool
	settings  bool
	reporting bool
	consent   reporting.Consent
	cfgErr    *config.LoadError
	pathsErr  error
	dark      bool
	// gated makes each action wait for a token on frFixture.gate.
	gated bool
	// fail maps an app ID ("" for the Flathub remote) to its error.
	fail map[string]error
	// saveErr makes the consent save fail.
	saveErr error
}

// frFixture drives a FirstRun with fakes for every side effect: the dial,
// the color-scheme writer, the window appearance, the settings launcher,
// the Flatpak runner, the consent save, the quit, and markers under
// t.TempDir(). post queues the UI-thread functions; the test goroutine
// runs them (settle), as the window's Update would.
type frFixture struct {
	t     *testing.T
	fr    *FirstRun
	tt    *ui.Tester
	queue chan func()
	paths firstrun.Paths

	online atomic.Bool
	gate   chan struct{}

	mu      sync.Mutex
	ran     []firstrun.Action
	written []firstrun.ColorScheme

	// UI thread only.
	appearance  []firstrun.ColorScheme
	launched    []firstrun.SettingsPanel
	consentPath string
	quits       int
	// checkedEarly is whether a connectivity check ran before the first
	// frame.
	checkedEarly bool
}

func newFRFixture(t *testing.T, o frOptions) *frFixture {
	t.Helper()
	frostui.SetIcons(frostui.NewIcons(map[string][]byte{"org.frostyard.ChairLift-symbolic": LogoSVG()}))
	dir := t.TempDir()
	f := &frFixture{t: t, queue: make(chan func(), 4096)}
	f.online.Store(o.online)
	if o.gated {
		f.gate = make(chan struct{})
	}
	paths, err := firstrun.ResolvePaths(func(key string) string {
		if key == "XDG_CONFIG_HOME" {
			return filepath.Join(dir, "config")
		}
		return ""
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	f.paths = paths
	var apps *firstrun.AppList
	if o.apps {
		apps, err = firstrun.ParseApps([]byte(firstRunApps))
		if err != nil {
			t.Fatal(err)
		}
	}
	f.consentPath = filepath.Join(dir, "state", "chairlift", "reporting.json")
	save := reporting.SaveConsent
	if o.saveErr != nil {
		save = func(string, reporting.Consent) error { return o.saveErr }
	}
	f.fr = newFirstRun(func(fn func()) { f.queue <- fn }, firstRunDeps{
		cfgErr:      o.cfgErr,
		apps:        apps,
		paths:       paths,
		pathsErr:    o.pathsErr,
		name:        "Ada Lovelace",
		settings:    o.settings,
		reportingOn: o.reporting,
		consent:     o.consent,
		consentPath: f.consentPath,
		scheme: firstrun.NewSchemeWriter(func(s firstrun.ColorScheme) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.written = append(f.written, s)
			return nil
		}),
		dial: func(context.Context, string, string) (net.Conn, error) {
			if !f.online.Load() {
				return nil, errors.New("network is unreachable")
			}
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		},
		run: func(a firstrun.Action) error {
			f.mu.Lock()
			f.ran = append(f.ran, a)
			f.mu.Unlock()
			if f.gate != nil {
				<-f.gate
			}
			if a.Kind == firstrun.ActionAddFlathub {
				return o.fail[""]
			}
			return o.fail[a.App.ID]
		},
		launch: func(p firstrun.SettingsPanel) error {
			f.launched = append(f.launched, p)
			return nil
		},
		setAppearance: func(s firstrun.ColorScheme) { f.appearance = append(f.appearance, s) },
		saveConsent:   save,
		now:           func() time.Time { return time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC) },
		quit:          func() { f.quits++ },
	})
	f.checkedEarly = f.fr.checking || f.fr.started
	f.tt = ui.NewTesterWith(f.fr.View, 760, 680, ui.TesterOptions{Dark: o.dark})
	return f
}

// settle runs the posted UI-thread functions until cond holds, then draws
// a frame.
func (f *frFixture) settle(what string, cond func() bool) {
	f.t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case fn := <-f.queue:
			fn()
		case <-deadline:
			f.t.Fatalf("timed out waiting for %s (step %s)", what, f.fr.flow.Current())
		}
	}
	f.tt.Frame()
}

func (f *frFixture) step() firstrun.Step { return f.fr.flow.Current() }

func (f *frFixture) click(label string) {
	f.t.Helper()
	if err := f.tt.Click(label); err != nil {
		f.t.Fatalf("click %q: %v; texts %q", label, err, f.tt.Texts())
	}
}

// clickRole clicks the element of role r named name at its center: a
// switch whose label is also a visible text.
func (f *frFixture) clickRole(r ui.Role, name string) {
	f.t.Helper()
	info, ok := f.tt.AccessByRole(r, name)
	if !ok {
		f.t.Fatalf("no %v named %q; texts %q", r, name, f.tt.Texts())
	}
	f.tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
}

func (f *frFixture) wantStep(want firstrun.Step) {
	f.t.Helper()
	if got := f.step(); got != want {
		f.t.Fatalf("step = %s, want %s", got, want)
	}
}

func (f *frFixture) wantText(texts ...string) {
	f.t.Helper()
	for _, text := range texts {
		if !f.tt.HasText(text) {
			f.t.Errorf("missing %q in %q", text, f.tt.Texts())
		}
	}
}

func (f *frFixture) nextDisabled() bool {
	f.t.Helper()
	info, ok := f.tt.AccessByRole(ui.RoleButton, i18n.T(i18n.MsgNext))
	if !ok {
		f.t.Fatalf("no Next button; texts %q", f.tt.Texts())
	}
	return info.Disabled
}

// toConnectivity starts and waits for the connectivity check's result.
func (f *frFixture) toConnectivity() {
	f.t.Helper()
	f.click(i18n.T(i18n.MsgStart))
	f.wantStep(firstrun.StepConnectivity)
	f.settle("the connectivity check", func() bool { return f.fr.flow.Connectivity() != firstrun.ConnChecking })
}

// toDone walks from the current step to Done and waits for the Progress
// worker.
func (f *frFixture) toDone() {
	f.t.Helper()
	for f.step() != firstrun.StepProgress && f.step() != firstrun.StepDone {
		f.click(i18n.T(i18n.MsgNext))
	}
	f.settle("the Done step", func() bool { return f.step() == firstrun.StepDone })
}

func (f *frFixture) ranActions() []firstrun.Action {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]firstrun.Action(nil), f.ran...)
}

func setFirstRunDryRun(t *testing.T) {
	t.Helper()
	firstrun.SetDryRun(true)
	t.Cleanup(func() { firstrun.SetDryRun(false) })
}

func TestFirstRunWalksTheStepsForwardAndBack(t *testing.T) {
	f := newFRFixture(t, frOptions{online: true, apps: true})
	f.wantStep(firstrun.StepWelcome)
	f.wantText(i18n.Format(i18n.T(i18n.MsgHello), "Ada Lovelace"), i18n.T(i18n.MsgWelcomeDesc), i18n.T(i18n.MsgStart))
	if f.tt.HasText(i18n.T(i18n.MsgNext)) || f.tt.HasText(i18n.T(i18n.MsgBack)) {
		t.Errorf("Welcome shows Back or Next: %q", f.tt.Texts())
	}

	f.click(i18n.T(i18n.MsgStart))
	f.wantStep(firstrun.StepConnectivity)
	f.wantText(i18n.T(i18n.MsgCheckingConnection))
	if !f.nextDisabled() {
		t.Error("Next is enabled while the connectivity check runs")
	}
	f.settle("online", func() bool { return f.fr.flow.Connectivity() == firstrun.ConnOnline })
	f.wantText(i18n.T(i18n.MsgConnectionAvailable), i18n.T(i18n.MsgBack))
	if f.nextDisabled() {
		t.Error("Next is disabled after the check")
	}

	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepStyle)
	f.click(i18n.T(i18n.MsgBack))
	f.wantStep(firstrun.StepConnectivity)
	f.click(i18n.T(i18n.MsgNext))
	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepApps)
	f.wantText(i18n.T(i18n.MsgApplications), i18n.T(i18n.MsgSelectApps), "Firefox", "org.mozilla.firefox")
	f.click(i18n.T(i18n.MsgBack))
	f.wantStep(firstrun.StepStyle)
	f.click(i18n.T(i18n.MsgBack))
	f.click(i18n.T(i18n.MsgBack))
	f.wantStep(firstrun.StepWelcome)
	if f.tt.HasText(i18n.T(i18n.MsgBack)) {
		t.Error("Welcome shows Back")
	}
}

// The app builds the state before the window exists and its post drops
// what comes earlier, so the check must start with the first frame.
func TestFirstRunStartsTheConnectivityCheckWithTheFirstFrame(t *testing.T) {
	f := newFRFixture(t, frOptions{online: false})
	if f.checkedEarly {
		t.Fatal("the connectivity check started before the first frame")
	}
	if !f.fr.started {
		t.Fatal("the first frame did not start the connectivity check")
	}
	f.settle("offline", func() bool { return f.fr.flow.Connectivity() == firstrun.ConnOffline })
	f.tt.Frame()
	if f.fr.checking {
		t.Error("a later frame started another check")
	}
}

func TestFirstRunWelcomeOpensAccessibilityOnlyWithSettings(t *testing.T) {
	f := newFRFixture(t, frOptions{online: true, settings: true})
	f.click(i18n.T(i18n.MsgAccessibility))
	if !slices.Equal(f.launched, []firstrun.SettingsPanel{firstrun.PanelAccessibility}) {
		t.Errorf("launched %v, want the accessibility panel", f.launched)
	}

	f = newFRFixture(t, frOptions{online: true})
	if f.tt.HasText(i18n.T(i18n.MsgAccessibility)) {
		t.Error("Accessibility shows without gnome-control-center")
	}
}

func TestFirstRunOfflineOffersRetryAndSettings(t *testing.T) {
	f := newFRFixture(t, frOptions{online: false, apps: true, settings: true})
	f.toConnectivity()
	f.wantText(i18n.T(i18n.MsgNoInternet), i18n.T(i18n.MsgOfflineDesc), i18n.T(i18n.MsgCheckAgain), i18n.T(i18n.MsgOpenSettings))
	if f.nextDisabled() {
		t.Error("offline users cannot go on")
	}
	f.click(i18n.T(i18n.MsgOpenSettings))
	if !slices.Equal(f.launched, []firstrun.SettingsPanel{firstrun.PanelWifi}) {
		t.Errorf("launched %v, want the wifi panel", f.launched)
	}

	f.online.Store(true)
	f.click(i18n.T(i18n.MsgCheckAgain))
	if f.fr.flow.Connectivity() != firstrun.ConnChecking || !f.nextDisabled() {
		t.Error("Check Again does not start a check that holds Next")
	}
	f.settle("online", func() bool { return f.fr.flow.Connectivity() == firstrun.ConnOnline })
	f.click(i18n.T(i18n.MsgNext))
	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepApps)
}

func TestFirstRunOfflineSkipsAppsAndRunsNoAction(t *testing.T) {
	f := newFRFixture(t, frOptions{online: false, apps: true})
	f.toConnectivity()
	if f.tt.HasText(i18n.T(i18n.MsgOpenSettings)) {
		t.Error("Open Settings shows without gnome-control-center")
	}
	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepStyle)
	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepProgress)
	f.settle("the Done step", func() bool { return f.step() == firstrun.StepDone })
	if ran := f.ranActions(); len(ran) != 0 {
		t.Errorf("offline run ran %v", ran)
	}
}

func TestFirstRunAppSelectionReachesPlanActions(t *testing.T) {
	f := newFRFixture(t, frOptions{online: true, apps: true})
	f.toConnectivity()
	f.click(i18n.T(i18n.MsgNext))
	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepApps)
	f.wantText("Flatseal", "Chromium")

	f.clickRole(ui.RoleSwitch, i18n.T(i18n.MsgUtilities))
	if f.fr.selection.CategoryOn("utilities") {
		t.Fatal("the utilities switch did not turn the category off")
	}
	if f.tt.HasText("Flatseal") {
		t.Error("an app of a category that is off still shows")
	}
	if info, _ := f.tt.AccessByRole(ui.RoleSwitch, "Chromium"); info.Checked {
		t.Error("Chromium starts on; its list entry says active false")
	}
	f.clickRole(ui.RoleSwitch, "Chromium")
	f.clickRole(ui.RoleSwitch, "Firefox")

	f.toDone()
	apps, _ := firstrun.ParseApps([]byte(firstRunApps))
	sel := firstrun.NewSelection(apps)
	sel.SetCategory("utilities", false)
	sel.SetApp("org.chromium.Chromium", true)
	sel.SetApp("org.mozilla.firefox", false)
	want := firstrun.PlanActions(sel.Selected())
	got := f.ranActions()
	if len(got) != 3 || !slices.EqualFunc(got, want, func(a, b firstrun.Action) bool { return a.Kind == b.Kind && a.App.ID == b.App.ID }) {
		t.Errorf("ran %+v, want %+v", got, want)
	}
}

func TestFirstRunCannotCloseWhileProgressRuns(t *testing.T) {
	f := newFRFixture(t, frOptions{online: true, apps: true, gated: true})
	f.toConnectivity()
	f.click(i18n.T(i18n.MsgNext))
	f.click(i18n.T(i18n.MsgNext))
	if !f.fr.CanClose() {
		t.Fatal("the window refuses to close before Progress")
	}
	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepProgress)
	if f.fr.CanClose() {
		t.Fatal("the window may close while the actions run")
	}
	f.settle("the first action", func() bool { return len(f.fr.states) > 0 && f.fr.states[0] == firstrun.ActionRunning })
	f.wantText(i18n.T(i18n.MsgSettingUpFlathub), i18n.T(i18n.MsgInstalling)+" Firefox")
	if f.tt.HasText(i18n.T(i18n.MsgBack)) || !f.nextDisabled() {
		t.Error("Progress offers Back or Next while it runs")
	}
	f.gate <- struct{}{}
	f.settle("the second action", func() bool { return f.fr.states[1] == firstrun.ActionRunning })
	if f.fr.states[0] != firstrun.ActionSucceeded || f.fr.CanClose() {
		t.Errorf("states %v, CanClose %v", f.fr.states, f.fr.CanClose())
	}
	close(f.gate)
	f.settle("the Done step", func() bool { return f.step() == firstrun.StepDone })
	if !f.fr.CanClose() {
		t.Error("the window still refuses to close after Progress")
	}
}

func TestFirstRunCompletionWritesMarkersAndExits(t *testing.T) {
	f := newFRFixture(t, frOptions{online: true, apps: true})
	f.toConnectivity()
	f.toDone()
	for _, marker := range f.paths.MarkerPaths() {
		if _, err := os.Stat(marker); err != nil {
			t.Errorf("marker %s: %v", marker, err)
		}
	}
	if !firstrun.IsDone(f.paths) {
		t.Error("the gate would run the first run again")
	}
	f.wantText(i18n.T(i18n.MsgFinished), i18n.T(i18n.MsgFinishedDesc), i18n.T(i18n.MsgExit))
	if f.tt.HasText(i18n.T(i18n.MsgErrorLog)) || f.tt.HasText(i18n.T(i18n.MsgNext)) {
		t.Errorf("Done shows an error log or Next: %q", f.tt.Texts())
	}
	f.click(i18n.T(i18n.MsgExit))
	if f.quits != 1 {
		t.Errorf("Exit quit %d times", f.quits)
	}
}

func TestFirstRunDryRunWritesNoMarkers(t *testing.T) {
	setFirstRunDryRun(t)
	f := newFRFixture(t, frOptions{online: true, apps: true})
	f.toConnectivity()
	f.toDone()
	if firstrun.IsDone(f.paths) {
		t.Error("a dry run wrote a marker")
	}
	if _, err := os.Stat(f.paths.ConfigHome); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a dry run created the config home: %v", err)
	}
	f.wantText(i18n.T(i18n.MsgDryRunFinished))
}

func TestFirstRunFailedActionShowsErrorLogAndStillCompletes(t *testing.T) {
	f := newFRFixture(t, frOptions{online: true, apps: true, fail: map[string]error{"org.mozilla.firefox": errors.New("exit status 1: no remote refs")}})
	f.toConnectivity()
	f.toDone()
	if !f.fr.done.Warning || len(f.fr.done.Entries) != 1 {
		t.Fatalf("done = %+v, want one warning entry", f.fr.done)
	}
	if !firstrun.IsDone(f.paths) {
		t.Error("the completion is not recorded after a failed action")
	}
	f.wantText(i18n.T(i18n.MsgErrorLog))
	entry := i18n.T(i18n.MsgSetupFailed) + i18n.T(i18n.MsgInstalling) + " Firefox"
	if f.tt.HasText(entry) {
		t.Error("the error log starts open")
	}
	f.click(i18n.T(i18n.MsgErrorLog))
	f.tt.Advance(time.Second)
	f.wantText(entry, "exit status 1: no remote refs")
}

func TestFirstRunCompletionFailureWithoutConfigHome(t *testing.T) {
	f := newFRFixture(t, frOptions{online: false, pathsErr: errors.New("neither XDG_CONFIG_HOME nor HOME is set")})
	f.toConnectivity()
	f.toDone()
	if f.fr.done.Description != i18n.MsgFinishedWithErrors || !f.fr.done.Warning {
		t.Errorf("done = %+v, want the finished-with-errors warning (GateUnknown)", f.fr.done)
	}
	f.wantText(i18n.T(i18n.MsgFinishedWithErrors), i18n.T(i18n.MsgErrorLog))
	f.click(i18n.T(i18n.MsgErrorLog))
	f.tt.Advance(time.Second)
	f.wantText(i18n.T(i18n.MsgSetupFailed) + i18n.T(i18n.MsgCompletionFailed))
}

func TestFirstRunStyleWritesSchemeSerially(t *testing.T) {
	f := newFRFixture(t, frOptions{online: true})
	f.toConnectivity()
	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepStyle)
	if info, ok := f.tt.AccessByRole(ui.RoleRadio, i18n.T(i18n.MsgLight)); !ok || !info.Checked {
		t.Fatalf("the light desktop's scheme is not the one chosen: %+v", info)
	}
	f.clickRole(ui.RoleRadio, i18n.T(i18n.MsgLight))
	if len(f.appearance) != 0 {
		t.Error("choosing the scheme that shows wrote it")
	}
	f.clickRole(ui.RoleRadio, i18n.T(i18n.MsgDark))
	f.clickRole(ui.RoleRadio, i18n.T(i18n.MsgDark))
	f.clickRole(ui.RoleRadio, i18n.T(i18n.MsgLight))
	if !slices.Equal(f.appearance, []firstrun.ColorScheme{firstrun.SchemeDark, firstrun.SchemeLight}) {
		t.Errorf("appearance %v, want dark then light", f.appearance)
	}
	if info, _ := f.tt.AccessByRole(ui.RoleRadio, i18n.T(i18n.MsgLight)); !info.Checked {
		t.Error("Light is not shown as chosen after the last choice")
	}
	f.fr.scheme.Wait()
	f.mu.Lock()
	written := append([]firstrun.ColorScheme(nil), f.written...)
	f.mu.Unlock()
	if len(written) == 0 || written[len(written)-1] != firstrun.SchemeLight {
		t.Errorf("written %v, want the last choice (light) last", written)
	}
}

func TestFirstRunReportingConsentIsOffAndSaves(t *testing.T) {
	f := newFRFixture(t, frOptions{online: true, apps: true, reporting: true})
	f.toConnectivity()
	f.click(i18n.T(i18n.MsgNext))
	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepReporting)
	f.wantText(i18n.T(i18n.MsgReportingTitle), i18n.T(i18n.MsgReportingOff), i18n.T(i18n.MsgReportingWhat))
	name := i18n.T(i18n.MsgReportingSwitch)
	if info, ok := f.tt.AccessByRole(ui.RoleSwitch, name); !ok || info.Checked {
		t.Fatalf("the consent switch is not off by default: %+v", info)
	}

	f.clickRole(ui.RoleSwitch, name)
	saved, err := reporting.LoadConsent(f.consentPath)
	if err != nil || !saved.Allows() || saved.Mode != reporting.ModeAsk {
		t.Fatalf("saved %+v, %v; want on in ask mode", saved, err)
	}
	f.tt.Frame()
	f.wantText(i18n.T(i18n.MsgReportingOn), actionmsg.ReportingConsent(false, true, string(reporting.ModeAsk)).Toast)

	f.clickRole(ui.RoleSwitch, name)
	saved, _ = reporting.LoadConsent(f.consentPath)
	if saved.Allows() {
		t.Error("turning the switch off did not save off")
	}

	f.click(i18n.T(i18n.MsgReportingWhat))
	if !slices.Contains(f.tt.OpenedURLs(), reporting.PrivacyURL) {
		t.Errorf("opened %v, want the privacy page", f.tt.OpenedURLs())
	}
	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepApps)
}

func TestFirstRunReportingDryRunSavesNothing(t *testing.T) {
	setFirstRunDryRun(t)
	f := newFRFixture(t, frOptions{online: true, reporting: true})
	f.toConnectivity()
	f.click(i18n.T(i18n.MsgNext))
	f.click(i18n.T(i18n.MsgNext))
	f.clickRole(ui.RoleSwitch, i18n.T(i18n.MsgReportingSwitch))
	if _, err := os.Stat(f.consentPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a dry run wrote the consent file: %v", err)
	}
	if !f.fr.reportingRecord.Allows() {
		t.Error("a dry run does not keep the choice for this run")
	}
	f.tt.Frame()
	f.wantText(actionmsg.ReportingConsent(true, true, string(reporting.ModeAsk)).Toast)
}

func TestFirstRunReportingSaveFailureKeepsTheSwitchOff(t *testing.T) {
	f := newFRFixture(t, frOptions{online: true, reporting: true, saveErr: errors.New("read-only file system")})
	f.toConnectivity()
	f.click(i18n.T(i18n.MsgNext))
	f.click(i18n.T(i18n.MsgNext))
	f.clickRole(ui.RoleSwitch, i18n.T(i18n.MsgReportingSwitch))
	f.tt.Frame()
	if info, _ := f.tt.AccessByRole(ui.RoleSwitch, i18n.T(i18n.MsgReportingSwitch)); info.Checked || f.fr.reportingRecord.Allows() {
		t.Error("a grant that was not saved shows on")
	}
	f.wantText(i18n.T(i18n.MsgReportingSaveFailed))
}

func TestFirstRunHidesTheReportingStepWhenUnavailable(t *testing.T) {
	f := newFRFixture(t, frOptions{online: true, apps: true})
	f.toConnectivity()
	f.click(i18n.T(i18n.MsgNext))
	f.click(i18n.T(i18n.MsgNext))
	f.wantStep(firstrun.StepApps)
}

func TestFirstRunConfigErrorToastStays(t *testing.T) {
	cfgErr := &config.LoadError{Path: "/etc/chairlift/config.yml", Kind: config.KindRead, Detail: "a <bad> & broken file"}
	f := newFRFixture(t, frOptions{online: true, cfgErr: cfgErr})
	f.tt.Frame()
	f.wantText(cfgErr.ToastMessage())
	f.tt.Advance(time.Minute)
	f.wantText(cfgErr.ToastMessage())
	// The toast covers neither Start nor Next: the user can still finish.
	f.toConnectivity()
	f.click(i18n.T(i18n.MsgNext))
	f.wantText(cfgErr.ToastMessage())
	// The user can close it, as the GTK toast's close button did.
	f.click(i18n.T(i18n.MsgClose))
	f.tt.Advance(time.Second)
	if f.tt.HasText(cfgErr.ToastMessage()) {
		t.Error("the close button does not close the configuration toast")
	}
	f.toDone()
}

func TestFirstRunPresentedLogsTheReadinessMarker(t *testing.T) {
	var buf bytes.Buffer
	out := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(out) })
	newFRFixture(t, frOptions{online: true}).fr.Presented()
	if !strings.Contains(buf.String(), "first-run: window presented\n") {
		t.Errorf("log = %q", buf.String())
	}
}

// firstRunSnapshot saves what tt shows as <name>.png in
// $CHAIRLIFT_SNAPSHOT_DIR, when it is set:
// CHAIRLIFT_SNAPSHOT_DIR=/tmp/shots go test ./internal/window -run SnapshotFirstRun
func firstRunSnapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("CHAIRLIFT_SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot: %s", f.Name())
}

func TestSnapshotFirstRunSteps(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := "light"
		if dark {
			theme = "dark"
		}
		shot := func(f *frFixture, name string) {
			f.tt.Frame()
			firstRunSnapshot(t, f.tt, "firstrun-"+name+"-"+theme)
		}

		f := newFRFixture(t, frOptions{online: true, apps: true, settings: true, reporting: true, gated: true, dark: dark,
			fail: map[string]error{"org.libreoffice.LibreOffice": errors.New("error: Unable to load summary from remote flathub: Could not resolve hostname")}})
		shot(f, "welcome")
		f.click(i18n.T(i18n.MsgStart))
		shot(f, "connectivity-checking")
		f.settle("online", func() bool { return f.fr.flow.Connectivity() == firstrun.ConnOnline })
		shot(f, "connectivity-online")
		f.click(i18n.T(i18n.MsgNext))
		shot(f, "style")
		f.click(i18n.T(i18n.MsgNext))
		shot(f, "reporting")
		f.click(i18n.T(i18n.MsgNext))
		f.clickRole(ui.RoleSwitch, "Chromium")
		shot(f, "apps")
		// The window's minimum size (internal/app: 640 x 560).
		f.tt.SetSize(640, 560)
		shot(f, "apps-small")
		f.tt.SetSize(760, 680)
		f.click(i18n.T(i18n.MsgNext))
		f.gate <- struct{}{}
		f.gate <- struct{}{}
		f.settle("the third action", func() bool { return f.fr.states[2] == firstrun.ActionRunning })
		shot(f, "progress")
		close(f.gate)
		f.settle("the Done step", func() bool { return f.step() == firstrun.StepDone })
		f.click(i18n.T(i18n.MsgErrorLog))
		f.tt.Advance(time.Second)
		shot(f, "done-errors")

		f = newFRFixture(t, frOptions{online: false, settings: true, dark: dark,
			cfgErr: &config.LoadError{Path: "/etc/chairlift/config.yml", Kind: config.KindRead, Detail: "permission denied"}})
		f.toConnectivity()
		shot(f, "connectivity-offline")
		f.toDone()
		shot(f, "done")
	}
}
