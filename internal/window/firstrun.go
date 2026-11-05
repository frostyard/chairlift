package window

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/user"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/firstrun"
	"github.com/frostyard/chairlift/internal/i18n"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/selfreport"

	sgtk "github.com/frostyard/snowkit/gtk"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// FirstRunWindow is the window of `chairlift --first-run` (ADR-0014). It
// shows the steps of internal/firstrun in a stack, with Back and Next in the
// header bar. Every decision (step order and visibility, the application
// list, the actions, the completion) comes from internal/firstrun; this file
// only builds widgets and moves the stack. All text goes through i18n.T.
//
// Every method runs on the GTK main thread. Worker goroutines (the
// connectivity check, the Progress actions, the color-scheme write) return
// to it with sgtk.RunOnMainThread.
type FirstRunWindow struct {
	app       adw.Application
	win       *adw.ApplicationWindow
	toasts    *adw.ToastOverlay
	scheme    *firstrun.SchemeWriter
	holding   bool // the application is held while Progress runs
	flow      *firstrun.Flow
	paths     firstrun.Paths
	pathsErr  error
	apps      *firstrun.AppList
	selection *firstrun.Selection
	settings  bool // gnome-control-center is on PATH

	stack   *gtk.Stack
	backBtn *gtk.Button
	nextBtn *gtk.Button

	connPage        *adw.StatusPage
	connSettingsBtn *gtk.Button
	connRetryBtn    *gtk.Button
	checking        bool

	categoryRows map[string]*adw.ExpanderRow
	appRows      map[string]*adw.SwitchRow

	// The optional problem-reporting step (ADR-0015): firstrun_reporting.go.
	reportingPage   *adw.StatusPage
	reportingSwitch *adw.SwitchRow
	reportingRecord reporting.Consent
	reportingPath   string

	progressGroup   *adw.PreferencesGroup
	progressStarted bool
	actionRows      []*adw.ActionRow
	actionSuffixes  []*gtk.Widget

	donePage   *adw.StatusPage
	doneErrors *adw.PreferencesGroup
	doneLog    *adw.ExpanderRow
	exitBtn    *gtk.Button
}

// NewFirstRun builds the first-run window for app. It loads the
// configuration and the application list (a failure hides the Apps step and
// is logged), resolves the marker paths, and starts the connectivity check.
// A configuration error also stays on screen as a persistent toast (ADR-0003);
// the user can still finish.
func NewFirstRun(app adw.Application) *FirstRunWindow {
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

	w := &FirstRunWindow{
		app:             app,
		win:             adw.NewApplicationWindow(&app.Application),
		scheme:          firstrun.NewSchemeWriter(firstrun.ApplyColorScheme),
		flow:            firstrun.NewFlowWithReporting(apps != nil, reportingOn),
		reportingRecord: consent,
		reportingPath:   consentPath,
		paths:           paths,
		pathsErr:        pathsErr,
		apps:            apps,
		selection:       firstrun.NewSelection(apps),
		settings:        firstrun.SettingsAvailable(exec.LookPath),
		categoryRows:    make(map[string]*adw.ExpanderRow),
		appRows:         make(map[string]*adw.SwitchRow),
	}
	w.win.SetTitle("ChairLift")
	w.win.SetDefaultSize(720, 640)
	w.buildUI()
	if cfgErr != nil {
		// buildUI runs on the GTK main thread and made the overlay; timeout
		// 0 keeps the diagnostic until it is dismissed.
		toast := adw.NewToast(cfgErr.ToastMessage())
		toast.SetUseMarkup(false)
		toast.SetTimeout(0)
		w.toasts.AddToast(toast)
	}
	// While the Progress actions run (Flow.CanClose is false), a close would
	// end the application and lose the completion markers: keep the window.
	closeCb := func(gtk.Window) bool {
		return !w.flow.CanClose()
	}
	w.win.ConnectCloseRequest(&closeCb)
	w.startConnectivityCheck()
	w.update()
	return w
}

// Present shows the window and logs the readiness marker that the E2E test
// and the lab read (ADR-0008 style).
func (w *FirstRunWindow) Present() {
	w.win.Present()
	log.Println("first-run: window presented")
}

// buildUI builds the header bar and one stack page per step.
func (w *FirstRunWindow) buildUI() {
	header := adw.NewHeaderBar()
	w.backBtn = gtk.NewButtonWithLabel(i18n.T(i18n.MsgBack))
	backCb := func(gtk.Button) { w.goBack() }
	w.backBtn.ConnectClicked(&backCb)
	header.PackStart(&w.backBtn.Widget)

	w.nextBtn = gtk.NewButtonWithLabel(i18n.T(i18n.MsgNext))
	w.nextBtn.AddCssClass("suggested-action")
	nextCb := func(gtk.Button) { w.goNext() }
	w.nextBtn.ConnectClicked(&nextCb)
	header.PackEnd(&w.nextBtn.Widget)

	w.stack = gtk.NewStack()
	w.stack.SetTransitionType(gtk.StackTransitionTypeSlideLeftRightValue)
	w.stack.SetVexpand(true)
	w.stack.AddNamed(&w.buildWelcome().Widget, firstrun.StepWelcome.String())
	w.stack.AddNamed(&w.buildConnectivity().Widget, firstrun.StepConnectivity.String())
	w.stack.AddNamed(&w.buildStyle().Widget, firstrun.StepStyle.String())
	if w.flow.Visible(firstrun.StepReporting) {
		w.stack.AddNamed(&w.buildReporting().Widget, firstrun.StepReporting.String())
	}
	w.stack.AddNamed(&w.buildApps().Widget, firstrun.StepApps.String())
	w.stack.AddNamed(&w.buildProgress().Widget, firstrun.StepProgress.String())
	w.stack.AddNamed(&w.buildDone().Widget, firstrun.StepDone.String())

	toolbar := adw.NewToolbarView()
	toolbar.AddTopBar(&header.Widget)
	toolbar.SetContent(&w.stack.Widget)
	w.toasts = adw.NewToastOverlay()
	w.toasts.SetChild(&toolbar.Widget)
	w.win.SetContent(&w.toasts.Widget)
}

// pillButton makes a centered pill button.
func pillButton(label string, suggested bool) *gtk.Button {
	btn := gtk.NewButtonWithLabel(label)
	btn.AddCssClass("pill")
	if suggested {
		btn.AddCssClass("suggested-action")
	}
	btn.SetHalign(gtk.AlignCenterValue)
	return btn
}

// buttonColumn stacks buttons vertically.
func buttonColumn(buttons ...*gtk.Button) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVerticalValue, 12)
	box.SetHalign(gtk.AlignCenterValue)
	for _, btn := range buttons {
		box.Append(&btn.Widget)
	}
	return box
}

// buildWelcome builds the Welcome step: the greeting, Start, and
// Accessibility when GNOME Settings is installed.
func (w *FirstRunWindow) buildWelcome() *adw.StatusPage {
	page := adw.NewStatusPage()
	page.SetIconName("org.frostyard.ChairLift")
	u, err := user.Current()
	page.SetTitle(i18n.Format(i18n.T(i18n.MsgHello), firstrun.WelcomeName(u, err, os.Getenv)))
	page.SetDescription(i18n.T(i18n.MsgWelcomeDesc))

	start := pillButton(i18n.T(i18n.MsgStart), true)
	startCb := func(gtk.Button) { w.goNext() }
	start.ConnectClicked(&startCb)
	buttons := []*gtk.Button{start}

	if w.settings {
		access := pillButton(i18n.T(i18n.MsgAccessibility), false)
		accessCb := func(gtk.Button) { w.launchSettings(firstrun.PanelAccessibility) }
		access.ConnectClicked(&accessCb)
		buttons = append(buttons, access)
	}
	page.SetChild(&buttonColumn(buttons...).Widget)
	return page
}

// buildConnectivity builds the connectivity status step. It is status only:
// offline, the user can go on and the Apps step is skipped.
func (w *FirstRunWindow) buildConnectivity() *adw.StatusPage {
	w.connPage = adw.NewStatusPage()

	w.connRetryBtn = pillButton(i18n.T(i18n.MsgCheckAgain), false)
	retryCb := func(gtk.Button) { w.startConnectivityCheck() }
	w.connRetryBtn.ConnectClicked(&retryCb)

	w.connSettingsBtn = pillButton(i18n.T(i18n.MsgOpenSettings), false)
	settingsCb := func(gtk.Button) { w.launchSettings(firstrun.PanelWifi) }
	w.connSettingsBtn.ConnectClicked(&settingsCb)

	w.connPage.SetChild(&buttonColumn(w.connRetryBtn, w.connSettingsBtn).Widget)
	w.showConnectivity()
	return w.connPage
}

// showConnectivity shows the flow's connectivity status.
func (w *FirstRunWindow) showConnectivity() {
	offline := false
	switch w.flow.Connectivity() {
	case firstrun.ConnChecking:
		w.connPage.SetIconName("network-transmit-receive-symbolic")
		w.connPage.SetTitle(i18n.T(i18n.MsgCheckingConnection))
		w.connPage.SetDescription(i18n.T(i18n.MsgCheckingDesc))
	case firstrun.ConnOnline:
		w.connPage.SetIconName("object-select-symbolic")
		w.connPage.SetTitle(i18n.T(i18n.MsgConnectionAvailable))
		w.connPage.SetDescription(i18n.T(i18n.MsgConnectionDesc))
	case firstrun.ConnOffline:
		offline = true
		w.connPage.SetIconName("network-offline-symbolic")
		w.connPage.SetTitle(i18n.T(i18n.MsgNoInternet))
		w.connPage.SetDescription(i18n.T(i18n.MsgOfflineDesc))
	}
	w.connRetryBtn.SetVisible(offline)
	w.connSettingsBtn.SetVisible(offline && w.settings)
}

// startConnectivityCheck runs the TCP check on a worker goroutine.
func (w *FirstRunWindow) startConnectivityCheck() {
	if w.checking {
		return
	}
	w.checking = true
	w.flow.SetConnectivity(firstrun.ConnChecking)
	w.showConnectivity()
	w.update()
	go func() {
		status := firstrun.CheckConnectivity(context.Background(), firstrun.DefaultDial)
		sgtk.RunOnMainThread(func() {
			w.checking = false
			w.flow.SetConnectivity(status)
			w.showConnectivity()
			w.update()
		})
	}()
}

// buildStyle builds the color-scheme step. A toggle applies at once, as in
// first-setup; the window itself follows the choice through the style
// manager.
func (w *FirstRunWindow) buildStyle() *adw.StatusPage {
	page := adw.NewStatusPage()
	page.SetIconName("applications-graphics-symbolic")
	page.SetTitle(i18n.T(i18n.MsgColorScheme))
	page.SetDescription(i18n.T(i18n.MsgColorSchemeDesc))

	light := gtk.NewToggleButtonWithLabel(i18n.T(i18n.MsgLight))
	light.SetTooltipText(i18n.T(i18n.MsgLightMode))
	dark := gtk.NewToggleButtonWithLabel(i18n.T(i18n.MsgDark))
	dark.SetTooltipText(i18n.T(i18n.MsgDarkMode))
	dark.SetGroup(light)
	for _, btn := range []*gtk.ToggleButton{light, dark} {
		btn.AddCssClass("pill")
		btn.SetSizeRequest(140, -1)
	}

	styleManager := adw.StyleManagerGetDefault()
	if styleManager.GetDark() {
		dark.SetActive(true)
	} else {
		light.SetActive(true)
	}

	lightCb := func(btn gtk.ToggleButton) {
		if btn.GetActive() {
			w.applyColorScheme(firstrun.SchemeLight)
		}
	}
	light.ConnectToggled(&lightCb)
	darkCb := func(btn gtk.ToggleButton) {
		if btn.GetActive() {
			w.applyColorScheme(firstrun.SchemeDark)
		}
	}
	dark.ConnectToggled(&darkCb)

	box := gtk.NewBox(gtk.OrientationHorizontalValue, 12)
	box.SetHalign(gtk.AlignCenterValue)
	box.Append(&light.Widget)
	box.Append(&dark.Widget)
	page.SetChild(&box.Widget)
	return page
}

// applyColorScheme shows the scheme in this window and asks the serial
// writer to write it with gsettings (dry run: logged only). The writer runs
// one write at a time, and the latest choice wins.
func (w *FirstRunWindow) applyColorScheme(scheme firstrun.ColorScheme) {
	styleManager := adw.StyleManagerGetDefault()
	if scheme == firstrun.SchemeDark {
		styleManager.SetColorScheme(adw.ColorSchemeForceDarkValue)
	} else {
		styleManager.SetColorScheme(adw.ColorSchemeForceLightValue)
	}
	w.scheme.Request(scheme)
}

// buildApps builds the application picker: one expander row with an enable
// switch per category and one switch row per application. It has no icons
// and fetches nothing from the network.
func (w *FirstRunWindow) buildApps() *adw.StatusPage {
	page := adw.NewStatusPage()
	page.SetIconName("applications-utilities-symbolic")
	page.SetTitle(i18n.T(i18n.MsgApplications))
	page.SetDescription(i18n.T(i18n.MsgApplicationsDesc))
	if w.apps == nil {
		return page
	}

	group := adw.NewPreferencesGroup()
	group.SetDescription(i18n.T(i18n.MsgSelectApps))
	for _, category := range w.apps.Categories {
		expander := adw.NewExpanderRow()
		// Rows parse Pango markup; list data is escaped.
		expander.SetTitle(glib.MarkupEscapeText(i18n.T(category.Title), -1))
		if category.Subtitle != "" {
			expander.SetSubtitle(glib.MarkupEscapeText(i18n.T(category.Subtitle), -1))
		}
		expander.SetShowEnableSwitch(true)
		expander.SetEnableExpansion(w.selection.CategoryOn(category.Key))
		expander.SetExpanded(false)
		w.categoryRows[category.Key] = expander
		for _, app := range category.Apps {
			row := adw.NewSwitchRow()
			row.SetTitle(glib.MarkupEscapeText(app.Name, -1))
			row.SetSubtitle(glib.MarkupEscapeText(app.ID, -1))
			row.SetActive(w.selection.AppOn(app.ID))
			w.appRows[app.ID] = row
			expander.AddRow(&row.Widget)
		}
		group.Add(&expander.Widget)
	}
	clamp := adw.NewClamp()
	clamp.SetMaximumSize(600)
	clamp.SetChild(&group.Widget)
	page.SetChild(&clamp.Widget)
	return page
}

// readSelection copies the switches into the selection model and returns
// the apps to install.
func (w *FirstRunWindow) readSelection() []firstrun.App {
	for key, expander := range w.categoryRows {
		w.selection.SetCategory(key, expander.GetEnableExpansion())
	}
	for id, row := range w.appRows {
		w.selection.SetApp(id, row.GetActive())
	}
	return w.selection.Selected()
}

// buildProgress builds the Progress step; startProgress fills it.
func (w *FirstRunWindow) buildProgress() *adw.StatusPage {
	page := adw.NewStatusPage()
	page.SetTitle(i18n.T(i18n.MsgProgress))
	page.SetDescription(i18n.T(i18n.MsgProgressDesc))
	w.progressGroup = adw.NewPreferencesGroup()
	w.progressGroup.SetVisible(false)
	clamp := adw.NewClamp()
	clamp.SetMaximumSize(600)
	clamp.SetChild(&w.progressGroup.Widget)
	page.SetChild(&clamp.Widget)
	return page
}

// startProgress plans the actions once, runs them on a worker goroutine,
// then records the completion before the Done step shows.
func (w *FirstRunWindow) startProgress() {
	if w.progressStarted {
		return
	}
	w.progressStarted = true

	var selected []firstrun.App
	if w.flow.Visible(firstrun.StepApps) {
		selected = w.readSelection()
	}
	actions := firstrun.PlanActions(selected)
	for _, action := range actions {
		row := adw.NewActionRow()
		row.SetTitle(glib.MarkupEscapeText(action.Title(), -1))
		w.progressGroup.Add(&row.Widget)
		w.actionRows = append(w.actionRows, row)
		w.actionSuffixes = append(w.actionSuffixes, nil)
	}
	w.progressGroup.SetVisible(len(actions) > 0)

	dryRun := firstrun.IsDryRun()
	paths, pathsErr := w.paths, w.pathsErr
	// Hold the application until the worker ends: the actions and the
	// completion markers must finish even if the window could go away.
	w.app.Hold()
	w.holding = true
	go func() {
		// RunProgress runs the actions, waits for the color-scheme writer
		// to drain, then records the completion (also when some actions
		// failed) before the Done step shows and the hold is released.
		decision, done := firstrun.RunProgress(actions, firstrun.ProgressSteps{
			Run: firstrun.FlatpakRunner,
			Emit: func(event firstrun.Event) {
				sgtk.RunOnMainThread(func() { w.showActionState(event) })
			},
			Drain: w.scheme.Wait,
			Complete: func() firstrun.Completion {
				return firstrun.CompleteAt(dryRun, paths, pathsErr)
			},
			Gate: func() firstrun.GateState { return firstrun.GateStateAt(paths, pathsErr) },
		})
		// CompleteAt logs nothing itself; its Log line carries the dry-run
		// preview, the markers written, or the failure, which the Done
		// step also shows.
		log.Println(decision.Log)
		if decision.Err != nil {
			selfreport.Error(selfreport.CodeFirstRunCompleteFailed, decision.Err)
		}
		sgtk.RunOnMainThread(func() { w.finishProgress(done) })
	}()
}

// showActionState replaces an action row's status widget.
func (w *FirstRunWindow) showActionState(event firstrun.Event) {
	if event.Index < 0 || event.Index >= len(w.actionRows) {
		return
	}
	if old := w.actionSuffixes[event.Index]; old != nil {
		old.SetVisible(false)
	}
	var suffix *gtk.Widget
	switch event.State {
	case firstrun.ActionRunning:
		spinner := gtk.NewSpinner()
		spinner.Start()
		suffix = &spinner.Widget
	case firstrun.ActionSucceeded:
		icon := gtk.NewImageFromIconName("object-select-symbolic")
		icon.AddCssClass("success")
		suffix = &icon.Widget
	case firstrun.ActionFailed:
		icon := gtk.NewImageFromIconName("dialog-warning-symbolic")
		icon.AddCssClass("warning")
		suffix = &icon.Widget
	default:
		return
	}
	w.actionRows[event.Index].AddSuffix(suffix)
	w.actionSuffixes[event.Index] = suffix
}

// finishProgress ends the Progress step, releases the application hold, and
// moves to Done once.
func (w *FirstRunWindow) finishProgress(done firstrun.Done) {
	w.flow.FinishProgress()
	if w.holding {
		w.holding = false
		w.app.Release()
	}
	w.donePage.SetDescription(i18n.T(done.Description))
	if done.Warning {
		w.donePage.SetIconName("dialog-warning-symbolic")
	}
	for _, entry := range done.Entries {
		log.Printf("first-run: %s: %s", entry.Title, entry.Message)
		label := gtk.NewLabel(entry.Title + "\n" + entry.Message)
		label.SetWrap(true)
		label.SetSelectable(true)
		label.SetXalign(0)
		label.SetMarginTop(6)
		label.SetMarginBottom(6)
		label.SetMarginStart(12)
		label.SetMarginEnd(12)
		w.doneLog.AddRow(&label.Widget)
	}
	w.doneErrors.SetVisible(len(done.Entries) > 0)
	if w.flow.Current() == firstrun.StepProgress && w.flow.Next() {
		w.exitBtn.GrabFocus()
	}
	w.update()
}

// buildDone builds the Done step: "Finished!", Exit, and the error log,
// which shows only when an action failed.
func (w *FirstRunWindow) buildDone() *adw.StatusPage {
	w.donePage = adw.NewStatusPage()
	w.donePage.SetIconName("object-select-symbolic")
	w.donePage.SetTitle(i18n.T(i18n.MsgFinished))
	w.donePage.SetDescription(i18n.T(i18n.MsgFinishedDesc))

	w.exitBtn = pillButton(i18n.T(i18n.MsgExit), true)
	exitCb := func(gtk.Button) { w.win.Close() }
	w.exitBtn.ConnectClicked(&exitCb)

	w.doneLog = adw.NewExpanderRow()
	w.doneLog.SetTitle(glib.MarkupEscapeText(i18n.T(i18n.MsgErrorLog), -1))
	w.doneErrors = adw.NewPreferencesGroup()
	w.doneErrors.Add(&w.doneLog.Widget)
	w.doneErrors.SetVisible(false)

	box := gtk.NewBox(gtk.OrientationVerticalValue, 24)
	box.Append(&w.exitBtn.Widget)
	clamp := adw.NewClamp()
	clamp.SetMaximumSize(600)
	clamp.SetChild(&w.doneErrors.Widget)
	box.Append(&clamp.Widget)
	w.donePage.SetChild(&box.Widget)
	return w.donePage
}

// launchSettings opens a GNOME Settings panel. It is a launch, not a state
// change, so it also runs in dry run.
func (w *FirstRunWindow) launchSettings(panel firstrun.SettingsPanel) {
	if err := firstrun.LaunchSettings(panel); err != nil {
		log.Printf("first-run: open settings %s: %v", panel, err)
	}
}

// goNext moves to the next visible step.
func (w *FirstRunWindow) goNext() {
	if !w.flow.Next() {
		return
	}
	if w.flow.Current() == firstrun.StepProgress {
		w.startProgress()
	}
	w.update()
}

// goBack moves to the previous visible step.
func (w *FirstRunWindow) goBack() {
	if w.flow.Back() {
		w.update()
	}
}

// update shows the current step and the header buttons the flow allows.
func (w *FirstRunWindow) update() {
	current := w.flow.Current()
	w.stack.SetVisibleChildName(current.String())
	w.backBtn.SetVisible(w.flow.CanBack())
	w.nextBtn.SetVisible(current != firstrun.StepWelcome && current != firstrun.StepDone)
	w.nextBtn.SetSensitive(w.flow.CanNext())
}
