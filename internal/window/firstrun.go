package window

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/user"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/firstrun"
	"github.com/frostyard/chairlift/internal/i18n"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/selfreport"
)

// FirstRun is the window of `chairlift --first-run` (ADR-0014): a calm
// centered wizard that walks the steps of internal/firstrun. Every decision
// (step order and visibility, the application list, the actions, the
// completion) comes from internal/firstrun; this file and its
// firstrun_*.go siblings only draw state and move the flow. All text goes
// through i18n.T.
//
// Its state belongs to the UI thread. Worker goroutines (the connectivity
// check and the Progress actions) copy their inputs first and hand their
// results back through post (the window's Update); the color-scheme writer
// is a firstrun.SchemeWriter with a worker of its own.
type FirstRun struct {
	// Quit ends the app: the Done step's Exit. NewFirstRun sets
	// mygo.App.Quit, whose close goes through the window's close guard.
	Quit func()

	post func(func())

	cfgErr    *config.LoadError
	toasts    []ui.Toast
	flow      *firstrun.Flow
	scheme    *firstrun.SchemeWriter
	paths     firstrun.Paths
	pathsErr  error
	apps      *firstrun.AppList
	selection *firstrun.Selection
	name      string
	settings  bool // gnome-control-center is on PATH
	logo      *ui.SVG

	dial          firstrun.DialFunc
	run           firstrun.Runner
	launch        func(firstrun.SettingsPanel) error
	setAppearance func(firstrun.ColorScheme)

	// started is true once the first frame started the connectivity
	// check; checking is true while a check runs.
	started  bool
	checking bool

	// chosen is the color scheme the Style step chose last, "" before the
	// first choice (the step then shows the desktop's).
	chosen firstrun.ColorScheme

	// The optional problem-reporting step (ADR-0015):
	// firstrun_reporting.go. reportingRecord is the one authority for the
	// switch.
	reportingRecord reporting.Consent
	reportingPath   string
	saveConsent     func(string, reporting.Consent) error
	now             func() time.Time

	progressStarted bool
	actions         []firstrun.Action
	states          []firstrun.ActionState

	done         firstrun.Done
	errorLogOpen bool
}

// firstRunDeps is everything the window reads from the machine or changes
// on it. NewFirstRun fills it with the production parts; tests inject
// fakes.
type firstRunDeps struct {
	cfgErr      *config.LoadError
	apps        *firstrun.AppList
	paths       firstrun.Paths
	pathsErr    error
	name        string
	settings    bool
	reportingOn bool
	consent     reporting.Consent
	consentPath string

	scheme        *firstrun.SchemeWriter
	dial          firstrun.DialFunc
	run           firstrun.Runner
	launch        func(firstrun.SettingsPanel) error
	setAppearance func(firstrun.ColorScheme)
	saveConsent   func(string, reporting.Consent) error
	now           func() time.Time
	quit          func()
}

// NewFirstRun builds the first-run window's state. post runs a function on
// the UI thread (the window's Update). It loads the configuration and the
// application list (a failure hides the Apps step and is logged) and
// resolves the marker paths; the first frame starts the connectivity check.
// A configuration error also stays on screen as a persistent toast
// (ADR-0003); the user can still finish.
func NewFirstRun(post func(func())) *FirstRun {
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		log.Printf("first-run: configuration error; the Apps step is hidden: %v", cfgErr)
		selfreport.Error(selfreport.CodeConfigLoadFailed, cfgErr)
	}
	apps, appsErr := firstrun.LoadConfiguredApps(cfg)
	if appsErr != nil {
		log.Printf("first-run: Apps step hidden: %v", appsErr)
	}
	home, _ := os.UserHomeDir()
	paths, pathsErr := firstrun.ResolvePaths(os.Getenv, home)
	if pathsErr != nil {
		log.Printf("first-run: cannot find the config home: %v", pathsErr)
	}
	// The optional problem-reporting step (ADR-0015) needs the reporting
	// group and the system problem file; its switch shows the saved consent.
	reportingOn, consent, consentPath := reportingStepAvailable(cfg)
	u, err := user.Current()

	return newFirstRun(post, firstRunDeps{
		cfgErr:      cfgErr,
		apps:        apps,
		paths:       paths,
		pathsErr:    pathsErr,
		name:        firstrun.WelcomeName(u, err, os.Getenv),
		settings:    firstrun.SettingsAvailable(exec.LookPath),
		reportingOn: reportingOn,
		consent:     consent,
		consentPath: consentPath,

		scheme:        firstrun.NewSchemeWriter(firstrun.ApplyColorScheme),
		dial:          firstrun.DefaultDial,
		run:           firstrun.FlatpakRunner,
		launch:        firstrun.LaunchSettings,
		setAppearance: forceAppearance,
		saveConsent:   reporting.SaveConsent,
		now:           time.Now,
		quit:          mygo.App.Quit,
	})
}

// newFirstRun builds the window's state from d.
func newFirstRun(post func(func()), d firstRunDeps) *FirstRun {
	w := &FirstRun{
		Quit:            d.quit,
		post:            post,
		cfgErr:          d.cfgErr,
		flow:            firstrun.NewFlowWithReporting(d.apps != nil, d.reportingOn),
		scheme:          d.scheme,
		paths:           d.paths,
		pathsErr:        d.pathsErr,
		apps:            d.apps,
		selection:       firstrun.NewSelection(d.apps),
		name:            d.name,
		settings:        d.settings,
		logo:            ui.MustParseSVG(logoSVG),
		dial:            d.dial,
		run:             d.run,
		launch:          d.launch,
		setAppearance:   d.setAppearance,
		reportingRecord: d.consent,
		reportingPath:   d.consentPath,
		saveConsent:     d.saveConsent,
		now:             d.now,
		done:            firstrun.Done{Description: i18n.MsgFinishedDesc},
	}
	if cfgErr := w.cfgErr; cfgErr != nil {
		// MyGo toasts are plain text, so a path with <, > or & shows as it
		// is; a negative timeout keeps the diagnostic until it is
		// dismissed, as the main window's (ADR-0003).
		w.toasts = append(w.toasts, ui.Toast{ID: "config-error", Title: cfgErr.ToastMessage(), Type: "error", Timeout: -1})
	}
	return w
}

// start runs once, in the first frame: it starts the connectivity check.
// The app builds this state before it creates the window. Its poster keeps
// what arrives before the window exists, but the check belongs to a shown
// window: a check that failed at once (offline: "network is unreachable")
// is answered on the step the user sees.
func (w *FirstRun) start() {
	if w.started {
		return
	}
	w.started = true
	w.startConnectivityCheck()
}

// CanClose reports whether closing the window now is safe: false while the
// first run's actions run (Flow.CanClose), since a close would end the app
// and lose the Flatpak installs and the completion markers. The app vetoes
// the window's close, and so mygo.App.Quit and SIGTERM too, while it is
// false; it is true again once finishProgress ran.
func (w *FirstRun) CanClose() bool {
	return w.flow.CanClose()
}

// Presented logs the readiness marker the end-to-end test and the lab read
// (ADR-0008, ADR-0014).
func (w *FirstRun) Presented() {
	log.Println("first-run: window presented")
}

// toast queues a plain-text toast for the next frame.
func (w *FirstRun) toast(text string) {
	w.toasts = append(w.toasts, ui.Toast{Title: text})
}

// forceAppearance shows the chosen scheme in this window at once, as the
// GTK window did through its style manager; the desktop follows when the
// SchemeWriter's gsettings write lands (not in dry run).
func forceAppearance(scheme firstrun.ColorScheme) {
	source := mygo.ThemeLight
	if scheme == firstrun.SchemeDark {
		source = mygo.ThemeDark
	}
	mygo.Theme.SetSource(source)
}

// startConnectivityCheck runs the TCP check on a worker goroutine. It is
// status only: offline, the user can go on and the Apps step is skipped.
func (w *FirstRun) startConnectivityCheck() {
	if w.checking {
		return
	}
	w.checking = true
	w.flow.SetConnectivity(firstrun.ConnChecking)
	dial, post := w.dial, w.post
	go func() {
		status := firstrun.CheckConnectivity(context.Background(), dial)
		post(func() {
			w.checking = false
			w.flow.SetConnectivity(status)
		})
	}()
}

// launchSettings opens a GNOME Settings panel. It is a launch, not a state
// change, so it also runs in dry run.
func (w *FirstRun) launchSettings(panel firstrun.SettingsPanel) {
	if err := w.launch(panel); err != nil {
		log.Printf("first-run: open settings %s: %v", panel, err)
	}
}

// applyColorScheme shows the scheme in this window and asks the serial
// writer to write it with gsettings (dry run: logged only). The writer runs
// one write at a time, and the latest choice wins.
func (w *FirstRun) applyColorScheme(scheme firstrun.ColorScheme) {
	w.chosen = scheme
	w.setAppearance(scheme)
	w.scheme.Request(scheme)
}

// goNext moves to the next visible step; entering Progress starts it.
func (w *FirstRun) goNext() {
	if !w.flow.Next() {
		return
	}
	if w.flow.Current() == firstrun.StepProgress {
		w.startProgress()
	}
}

// goBack moves to the previous visible step.
func (w *FirstRun) goBack() {
	w.flow.Back()
}

// startProgress plans the actions once and runs them, then the drain of the
// color-scheme writer, the completion and the gate read, on a worker
// goroutine through firstrun.RunProgress. The flow is already on Progress,
// so CanClose is false before the worker starts and until finishProgress.
func (w *FirstRun) startProgress() {
	if w.progressStarted {
		return
	}
	w.progressStarted = true

	var selected []firstrun.App
	if w.flow.Visible(firstrun.StepApps) {
		selected = w.selection.Selected()
	}
	actions := firstrun.PlanActions(selected)
	w.actions = actions
	w.states = make([]firstrun.ActionState, len(actions))

	dryRun := firstrun.IsDryRun()
	paths, pathsErr := w.paths, w.pathsErr
	post := w.post
	// RunProgress runs the actions, waits for the color-scheme writer to
	// drain, then records the completion (also when some actions failed)
	// before the Done step shows and the window may close again.
	steps := firstrun.ProgressSteps{
		Run: w.run,
		Emit: func(event firstrun.Event) {
			post(func() { w.showActionState(event) })
		},
		Drain: w.scheme.Wait,
		Complete: func() firstrun.Completion {
			return firstrun.CompleteAt(dryRun, paths, pathsErr)
		},
		Gate: func() firstrun.GateState { return firstrun.GateStateAt(paths, pathsErr) },
	}
	go func() {
		decision, done := firstrun.RunProgress(actions, steps)
		// CompleteAt logs nothing itself; its Log line carries the dry-run
		// preview, the markers written, or the failure, which the Done
		// step also shows.
		log.Println(decision.Log)
		if decision.Err != nil {
			selfreport.Error(selfreport.CodeFirstRunCompleteFailed, decision.Err)
		}
		post(func() { w.finishProgress(done) })
	}()
}

// showActionState records an action's new state.
func (w *FirstRun) showActionState(event firstrun.Event) {
	if event.Index < 0 || event.Index >= len(w.states) {
		return
	}
	w.states[event.Index] = event.State
}

// finishProgress ends the Progress step, so the window may close again,
// keeps what the Done step shows, and moves to Done once.
func (w *FirstRun) finishProgress(done firstrun.Done) {
	w.flow.FinishProgress()
	w.done = done
	for _, entry := range done.Entries {
		log.Printf("first-run: %s: %s", entry.Title, entry.Message)
	}
	if w.flow.Current() == firstrun.StepProgress {
		w.flow.Next()
	}
}

// exit ends the app from the Done step.
func (w *FirstRun) exit() {
	if w.Quit != nil {
		w.Quit()
	}
}
