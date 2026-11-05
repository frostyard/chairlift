package views

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/selfreport"
	"github.com/frostyard/chairlift/internal/version"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/pageview"

	sgtk "github.com/frostyard/snowkit/gtk"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// sendPace is the wait between 2 report requests: at most 10 requests in a
// minute, which is the intake API's rate limit.
const sendPace = 7 * time.Second

// reportRow is 1 problem row's Report… button, so a completed send can mark
// the row. Main thread only.
type reportRow struct {
	button *gtk.Button
	item   problems.Item
}

// buildProblemsPage builds the Problems page (ADR-0015): the system
// problems of snosi-health, the user's crashes and app problem records, and
// the reporting group. Nothing is sent until the user turns reporting on;
// internal/reporting is the only sender.
func (uh *UserHome) buildProblemsPage() {
	page := uh.problemsPrefsPage
	if page == nil {
		return
	}

	systemOn := uh.config.IsGroupEnabled("problems_page", "problems_system_group")
	appsOn := uh.config.IsGroupEnabled("problems_page", "problems_apps_group")
	reportingOn := uh.config.IsGroupEnabled("problems_page", "problems_reporting_group")

	// Every build makes the row map: addProblemRow writes to it whether or
	// not the reporting group exists.
	uh.reportingRows = make(map[string]*reportRow)
	uh.reportingClient, _ = problems.Client(version.Version)

	if systemOn {
		group := adw.NewPreferencesGroup()
		group.SetTitle(pageview.ProblemsSystemTitle)
		group.SetDescription(pageview.ProblemsLoading)
		page.Add(group)
		uh.problemsSystemGroup = group
	}

	if appsOn {
		group := adw.NewPreferencesGroup()
		group.SetTitle(pageview.ProblemsAppsTitle)
		group.SetDescription(pageview.ProblemsLoading)
		page.Add(group)
		uh.problemsAppsGroup = group
	}

	if reportingOn {
		uh.buildReportingGroup(page)
	}

	if !systemOn && !appsOn {
		// The marker is a contract (ADR-0008) and comes at once. The reporting
		// group still needs its consent and sent log, or a user could not
		// withdraw a consent while both data groups are off; there are no
		// items to send and no envelope, so no count either.
		log.Println(problems.BuiltMarker(0, 0, "disabled"))
		if reportingOn {
			go uh.loadProblems(systemOn, appsOn, reportingOn)
		}
		return
	}
	go uh.loadProblems(systemOn, appsOn, reportingOn)
}

// buildReportingGroup builds the reporting group: the consent switch, the
// mode row, the sent reports, the privacy link, and the weekly count row.
// It shows reporting off until the consent file was read off the main
// thread, so a failure can never look like consent.
func (uh *UserHome) buildReportingGroup(page *adw.PreferencesPage) {
	group := adw.NewPreferencesGroup()
	group.SetTitle(pageview.ReportingTitle)
	group.SetDescription(pageview.ReportingOff)

	if path, err := reporting.ConsentPath(os.Getenv); err != nil {
		log.Printf("problems: the reporting consent has no path: %v", err)
	} else {
		uh.reportingConsentAt = path
	}
	if path, err := reporting.StatePath(os.Getenv); err != nil {
		log.Printf("problems: the sent report log has no path: %v", err)
	} else {
		uh.reportingStateAt = path
	}
	endpoint, err := reporting.ParseEndpoint(uh.reportedEndpoint())
	if err != nil {
		// An invalid administrator setting disables every send path: it must
		// never fall back to the public default.
		log.Printf("problems: problem reporting is off: %v", err)
		uh.reportingEndpointErr = err
	} else {
		uh.reportingEndpoint = endpoint
	}

	uh.reportingSwitch = adw.NewSwitchRow()
	uh.reportingSwitch.SetTitle(pageview.ReportingSwitchTitle)
	uh.reportingSwitch.SetSubtitle(pageview.ReportingOff)
	uh.reportingSwitch.SetActive(false)
	// Insensitive until the first consent load published its value, so the
	// load can never overwrite a choice the user made while it ran.
	uh.reportingSwitch.SetSensitive(false)
	notifyCb := func(_ gobject.Object, pspec uintptr) {
		if gobject.ParamSpecNewFromInternalPtr(pspec).GetName() != "active" {
			return
		}
		uh.onReportingSwitched(uh.reportingSwitch.GetActive())
	}
	uh.reportingSwitch.ConnectNotify(&notifyCb)
	group.Add(&uh.reportingSwitch.Widget)

	uh.reportingMode = adw.NewComboRow()
	uh.reportingMode.SetTitle(pageview.ReportingModeTitle)
	model := gtk.NewStringList([]string{pageview.ReportingModeAsk, pageview.ReportingModeAuto})
	uh.reportingMode.SetModel(&gio.ListModelBase{Ptr: model.GoPointer()})
	uh.reportingMode.SetSelected(0)
	uh.reportingMode.SetSensitive(false)
	modeCb := func(_ gobject.Object, pspec uintptr) {
		if gobject.ParamSpecNewFromInternalPtr(pspec).GetName() != "selected" {
			return
		}
		uh.onReportingModeSelected()
	}
	uh.reportingMode.ConnectNotify(&modeCb)
	group.Add(&uh.reportingMode.Widget)

	uh.reportingSent = adw.NewExpanderRow()
	uh.reportingSent.SetTitle(pageview.ReportingSentTitle)
	uh.reportingSent.SetSubtitle(pageview.ReportingSentEmpty)
	uh.reportingSent.SetExpanded(false)
	group.Add(&uh.reportingSent.Widget)

	what := adw.NewActionRow()
	what.SetTitle(pageview.ReportingWhat().Title)
	what.SetSubtitle(pageview.ReportingWhat().Subtitle)
	what.SetActivatable(true)
	icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
	what.AddSuffix(&icon.Widget)
	whatCb := func(adw.ActionRow) {
		uh.openURL(reporting.PrivacyURL)
	}
	what.ConnectActivated(&whatCb)
	group.Add(&what.Widget)

	uh.reportingCountRow = adw.NewActionRow()
	uh.reportingCountRow.SetTitle(pageview.ReportingCountTitle)
	uh.reportingCountRow.SetSubtitle(pageview.ReportingCountSubtitle)
	review := gtk.NewButtonWithLabel(pageview.ReportingCountAction)
	review.SetValign(gtk.AlignCenterValue)
	reviewCb := func(gtk.Button) { uh.reviewWeeklyCount() }
	review.ConnectClicked(&reviewCb)
	uh.reportingCountRow.AddSuffix(&review.Widget)
	uh.reportingCountRow.SetVisible(false)
	group.Add(&uh.reportingCountRow.Widget)

	page.Add(group)
	uh.reportingGroup = group
}

// reportedEndpoint returns the endpoint of an administrator-owned
// configuration file, or "" for the default.
func (uh *UserHome) reportedEndpoint() string {
	raw, adminOwned := uh.config.ReportEndpoint()
	if !adminOwned {
		return ""
	}
	return raw
}

// onReportingSwitched handles a change of the consent switch. Main thread
// only. A change back to the state the switch shows (the page restoring the
// switch) does nothing; a stale consent shows off, so turning it on renews
// it (reporting.SwitchChanged).
func (uh *UserHome) onReportingSwitched(enabled bool) {
	if !uh.reportingLoaded || uh.reportingApplying || !reporting.SwitchChanged(uh.consentCopy(), enabled) {
		return
	}
	uh.applyReportingConsent(enabled, uh.selectedMode())
}

// onReportingModeSelected handles a change of the mode row. Main thread
// only.
func (uh *UserHome) onReportingModeSelected() {
	if !uh.reportingLoaded || uh.reportingApplying {
		return
	}
	current := uh.consentCopy()
	mode := uh.selectedMode()
	if mode == current.Mode || !current.Allows() {
		return
	}
	uh.applyReportingConsent(true, mode)
}

// applyReportingConsent takes the user's choice: the decision struct from
// internal/views/actionmsg decides the toast and whether the consent file is
// written, exactly once (ADR-0009).
//
// The consent file is about 200 bytes, so it is written here, on the GTK main
// thread, with reporting.SaveConsent. That is a small local file write, not an
// external tool call, so the main-thread rule allows it; and because the write
// is synchronous, an exit cannot lose an Off and no callback can restore an
// older value.
//
// A withdrawal (Off, or leaving automatic) reaches the in-memory value BEFORE
// the save, so a running send stops at once even while the save runs; a grant
// reaches it only after a successful save.
func (uh *UserHome) applyReportingConsent(enabled bool, mode reporting.Mode) {
	decision := actionmsg.ReportingConsent(IsDryRun(), enabled, string(mode))
	if decision.Log != "" {
		log.Println(decision.Log)
	}
	before := uh.consentCopy()
	record := reporting.Decide(enabled, mode, time.Now())
	withdraw := before.Allows() && (!record.Allows() || record.Mode != reporting.ModeAuto)

	if !decision.Persist {
		// A dry run keeps the choice in memory for this run only. A new
		// automatic choice previews its reports: Run sends nothing and
		// writes no file.
		uh.withoutSaving(func() { uh.publishConsent(record) })
		uh.toastAdder.ShowToast(decision.Toast)
		if reporting.StartsAutoRun(before, record) {
			uh.runAutomaticReports()
		}
		return
	}

	path := uh.reportingConsentAt
	if path == "" {
		if withdraw {
			uh.withoutSaving(func() { uh.publishConsent(record) })
		} else {
			uh.withoutSaving(func() { uh.showReportingConsent(uh.consentCopy()) })
		}
		uh.toastAdder.ShowErrorToast("Could not save the problem reporting setting: no consent file path")
		return
	}
	if withdraw {
		uh.withoutSaving(func() { uh.publishConsent(record) })
	}
	if err := reporting.SaveConsent(path, record); err != nil {
		log.Printf("problems: save the reporting consent: %v", err)
		// A withdrawal stays in memory (fail safe); a grant never arrived,
		// so the widgets go back to the value in memory.
		if !withdraw {
			uh.withoutSaving(func() { uh.showReportingConsent(uh.consentCopy()) })
		}
		uh.toastAdder.ShowErrorToast("Could not save the problem reporting setting: " + err.Error())
		return
	}
	if !withdraw {
		uh.withoutSaving(func() { uh.publishConsent(record) })
	}
	uh.toastAdder.ShowToast(decision.Toast)
	if reporting.StartsAutoRun(before, record) {
		uh.runAutomaticReports()
	}
}

// withoutSaving runs fn with the applying guard set, so the widget updates fn
// makes cannot start a second save. Main thread only.
func (uh *UserHome) withoutSaving(fn func()) {
	previous := uh.reportingApplying
	uh.reportingApplying = true
	defer func() { uh.reportingApplying = previous }()
	fn()
}

// publishConsent makes consent the wanted consent for the page and the send
// goroutine, and shows it. Main thread only.
func (uh *UserHome) publishConsent(consent reporting.Consent) {
	uh.reportingMu.Lock()
	uh.reportingConsent = consent
	uh.reportingMu.Unlock()
	uh.showReportingConsent(consent)
}

// consentCopy returns a copy of the wanted consent, safe from any goroutine.
func (uh *UserHome) consentCopy() reporting.Consent {
	uh.reportingMu.Lock()
	defer uh.reportingMu.Unlock()
	return uh.reportingConsent
}

// showReportingConsent applies a consent to the widgets. Main thread only.
func (uh *UserHome) showReportingConsent(consent reporting.Consent) {
	if uh.reportingSwitch == nil {
		return
	}
	uh.reportingSwitch.SetActive(consent.Allows())
	uh.reportingSwitch.SetSensitive(uh.reportingLoaded)
	if uh.reportingGroup != nil {
		description := pageview.ReportingDescription(consent)
		if uh.reportingEndpointErr != nil {
			description = pageview.ReportingEndpointInvalid
		}
		uh.reportingGroup.SetDescription(glib.MarkupEscapeText(description, -1))
	}
	row := pageview.ReportingMode(consent)
	if uh.reportingMode != nil {
		uh.reportingMode.SetSelected(modeIndex(consent.Mode))
		uh.reportingMode.SetSensitive(uh.reportingLoaded && consent.Allows())
		uh.reportingMode.SetSubtitle(glib.MarkupEscapeText(row.Subtitle, -1))
	}
	if uh.reportingSent != nil {
		uh.reportingSent.SetSensitive(consent.Allows())
	}
	uh.refreshReportButtons()
	uh.refreshCountRow()
}

// selectedMode returns the mode of the mode row. Main thread only.
func (uh *UserHome) selectedMode() reporting.Mode {
	if uh.reportingMode != nil && uh.reportingMode.GetSelected() == 1 {
		return reporting.ModeAuto
	}
	return reporting.ModeAsk
}

// modeIndex returns the mode row's position of a mode.
func modeIndex(mode reporting.Mode) uint32 {
	if mode == reporting.ModeAuto {
		return 1
	}
	return 0
}

// loadProblems reads every source off the GTK thread, then fills the
// groups on the main thread and logs the page's built marker. It runs with
// reportingOn and both data groups off too: the reporting group needs its
// consent and sent log, or the user could not withdraw a consent.
func (uh *UserHome) loadProblems(systemOn, appsOn, reportingOn bool) {
	dataOn := systemOn || appsOn
	dryRun := IsDryRun()

	now := time.Now()
	var snap problems.Snapshot
	seen := problems.NewSeen("")
	if dataOn {
		fixtures := problems.DecideFixtures(os.Getenv(problems.FixturesEnv), dryRun)
		if fixtures.Log != "" {
			log.Println(fixtures.Log)
		}

		if path, err := problems.SeenPath(os.Getenv); err != nil {
			log.Printf("problems: dismissals are not kept: %v", err)
		} else if loaded, err := problems.LoadSeen(path); err != nil {
			log.Printf("problems: dismiss store not read: %v", err)
			seen = loaded
		} else {
			seen = loaded
		}

		snap = problems.Load(context.Background(), problems.Options{
			Now:      now,
			UID:      os.Getuid(),
			System:   systemOn,
			Apps:     appsOn,
			Fixtures: fixtures,
			Seen:     seen,
		})
		for _, line := range snap.LogLines() {
			log.Println(line)
		}
		if err := snap.SourceInvalid(); err != nil {
			selfreport.Error(selfreport.CodeProblemsSourceInvalid, err)
		}
	}

	var consent reporting.Consent
	if reportingOn {
		if uh.reportingConsentAt == "" {
			log.Println("problems: problem reporting is off: the consent file has no path")
		} else if loaded, err := reporting.LoadConsent(uh.reportingConsentAt); err != nil {
			log.Printf("problems: problem reporting is off: %v", err)
		} else {
			consent = loaded
		}
	}
	var sent *reporting.State
	if uh.reportingStateAt != "" {
		loaded, err := reporting.LoadState(uh.reportingStateAt)
		if err != nil {
			log.Printf("problems: %v", err)
		}
		sent = loaded
	}

	items := append(append([]problems.Item(nil), snap.SystemItems...), snap.AppItems...)
	envelope := snap.System.Envelope
	endpointErr := uh.reportingEndpointErr

	sgtk.RunOnMainThread(func() {
		uh.reportingMu.Lock()
		uh.reportingConsent = consent
		uh.reportingState = sent
		uh.reportingItems = items
		uh.reportingEnvelope = envelope
		uh.reportingMu.Unlock()
		if reportingOn {
			uh.reportingLoaded = true
			uh.showReportingConsent(consent)
			uh.showSentReports(sent)
		}

		systemCount, appsCount, source := 0, 0, "disabled"
		if group := uh.problemsSystemGroup; group != nil {
			status := pageview.ProblemsSystemStatus(snap.System.Outcome, snap.System.Generated,
				len(snap.SystemItems), snap.SystemHidden, now, time.Local)
			uh.showProblems(group, status, snap.SystemItems, seen, now)
			systemCount, source = len(snap.SystemItems), snap.System.Outcome.String()
		}
		if group := uh.problemsAppsGroup; group != nil {
			status := pageview.ProblemsAppsStatus(len(snap.AppItems), snap.AppsHidden)
			uh.showProblems(group, status, snap.AppItems, seen, now)
			appsCount = len(snap.AppItems)
		}
		if dataOn {
			// With both data groups off the builder already logged the marker
			// at once, and there is nothing to send or count.
			log.Println(problems.BuiltMarker(systemCount, appsCount, source))
			return
		}
	})

	if dataOn && reportingOn && consent.Allows() && consent.Mode == reporting.ModeAuto && endpointErr == nil {
		// On the main thread: the run guard and the row map are main-thread
		// state, and the send itself runs in its own goroutine.
		sgtk.RunOnMainThread(func() { uh.runAutomaticReports() })
	}
}

// showProblems sets a group's description and adds its rows. Main thread
// only.
func (uh *UserHome) showProblems(group *adw.PreferencesGroup, status pageview.ProblemsStatus, items []problems.Item, seen *problems.Seen, now time.Time) {
	group.SetDescription(glib.MarkupEscapeText(status.Description, -1))
	if status.Empty != nil {
		group.Add(problemsTextRow(*status.Empty))
	}
	for _, item := range items {
		uh.addProblemRow(group, item, seen, now)
	}
}

// addProblemRow adds 1 expander row for item, with its hint, its values,
// its local details, and the Copy details, Dismiss and Report… buttons.
func (uh *UserHome) addProblemRow(group *adw.PreferencesGroup, item problems.Item, seen *problems.Seen, now time.Time) {
	text := pageview.Problem(item, now, time.Local)
	expander := adw.NewExpanderRow()
	expander.SetTitle(glib.MarkupEscapeText(text.Title, -1))
	expander.SetSubtitle(glib.MarkupEscapeText(text.Subtitle, -1))

	for _, detail := range pageview.ProblemDetails(item) {
		expander.AddRow(problemsTextRow(detail))
	}

	actions := adw.NewActionRow()
	copyBtn := gtk.NewButtonWithLabel("Copy details")
	copyBtn.SetValign(gtk.AlignCenterValue)
	copyClickedCb := func(btn gtk.Button) {
		btn.GetClipboard().SetText(problems.DetailsText(item))
		uh.toastAdder.ShowToast(actionmsg.ProblemDetailsCopied(text.Title))
	}
	copyBtn.ConnectClicked(&copyClickedCb)
	actions.AddSuffix(&copyBtn.Widget)

	dismissBtn := gtk.NewButtonWithLabel("Dismiss")
	dismissBtn.SetValign(gtk.AlignCenterValue)
	dismissClickedCb := func(btn gtk.Button) {
		decision := actionmsg.ProblemDismiss(IsDryRun(), item.ID, text.Title)
		if decision.Log != "" {
			log.Println(decision.Log)
		}
		if !decision.Persist {
			uh.toastAdder.ShowToast(decision.Toast)
			return
		}
		dismissBtn.SetSensitive(false)
		at := problems.DismissAt(item, time.Now())
		go func() {
			err := seen.Dismiss(item.ID, at)
			sgtk.RunOnMainThread(func() {
				if err != nil {
					log.Printf("problems: dismiss %s: %v", item.ID, err)
					dismissBtn.SetSensitive(true)
					uh.toastAdder.ShowErrorToast(actionmsg.ProblemDismissFailed(text.Title, err))
					return
				}
				if decision.Hide {
					group.Remove(&expander.Widget)
				}
				uh.toastAdder.ShowToast(decision.Toast)
			})
		}()
	}
	dismissBtn.ConnectClicked(&dismissClickedCb)
	actions.AddSuffix(&dismissBtn.Widget)

	// No Report… button without the reporting group: the button needs the
	// consent the group holds, and a hidden button must never become visible.
	reportingOn := uh.config.IsGroupEnabled("problems_page", "problems_reporting_group")
	reportBtn := gtk.NewButtonWithLabel(pageview.ReportButtonLabel)
	reportBtn.SetValign(gtk.AlignCenterValue)
	reportBtn.SetVisible(false)
	reportClickedCb := func(gtk.Button) {
		go func() {
			report, err := reporting.ForItem(item, uh.reportingClient)
			sgtk.RunOnMainThread(func() {
				if err != nil {
					log.Printf("problems: build the report of %s: %v", item.ID, err)
					uh.toastAdder.ShowErrorToast("Could not build the report: " + err.Error())
					return
				}
				uh.openReportDialog(text.Title, report, func() {
					uh.markReportSent(report)
				})
			})
		}()
	}
	reportBtn.ConnectClicked(&reportClickedCb)
	if reportingOn {
		actions.AddSuffix(&reportBtn.Widget)
	}
	expander.AddRow(&actions.Widget)

	group.Add(&expander.Widget)
	if reportingOn {
		uh.reportingRows[item.ID] = &reportRow{button: reportBtn, item: item}
		uh.refreshReportButtons()
	}
}

// openReportDialog shows the exact report the Send button posts: the same
// bytes, in a monospace, selectable, scrollable label (ADR-0015). Main
// thread only.
func (uh *UserHome) openReportDialog(title string, report reporting.Report, onSent func()) {
	dialog := adw.NewAlertDialog(title, pageview.ReportSummary(report.Kind))
	label := gtk.NewLabel(report.Text())
	label.SetSelectable(true)
	label.SetWrap(false)
	label.SetXalign(0)
	label.AddCssClass("monospace")
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyAutomaticValue, gtk.PolicyAutomaticValue)
	scrolled.SetMinContentHeight(240)
	scrolled.SetChild(&label.Widget)
	dialog.SetExtraChild(&scrolled.Widget)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("send", "Send")
	dialog.SetResponseAppearance("send", adw.ResponseSuggestedValue)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	responseCb := func(_ adw.AlertDialog, response string) {
		if response != "send" {
			return
		}
		decision := actionmsg.Report(IsDryRun(), report.Kind)
		if decision.Log != "" {
			log.Println(decision.Log)
		}
		if !decision.Send {
			uh.toastAdder.ShowToast(decision.Toast)
			return
		}
		// The same Report value the dialog shows is the body that is sent.
		in := uh.reportingInput()
		go func() {
			entry, err := reporting.SendReport(context.Background(), in, report)
			sgtk.RunOnMainThread(func() {
				if err != nil {
					log.Printf("problems: send the %s report: %v", report.Kind, err)
					uh.toastAdder.ShowErrorToast(actionmsg.ReportFailed(report.Kind, err))
					return
				}
				if decision.MarkSent {
					uh.recordSent(entry)
					if onSent != nil {
						onSent()
					}
				}
				uh.toastAdder.ShowToast(decision.Toast)
			})
		}()
	}
	dialog.ConnectResponse(&responseCb)
	dialog.Present(&uh.problemsPrefsPage.Widget)
}

// reportingInput builds the input of a manual or automatic send from the
// page's state. It is safe to call from the main thread; the send goroutine
// only reads the values it copies.
func (uh *UserHome) reportingInput() reporting.RunInput {
	uh.reportingMu.Lock()
	statePath := uh.reportingStateAt
	items := uh.reportingItems
	envelope := uh.reportingEnvelope
	uh.reportingMu.Unlock()
	// A live send reads the consent file again before every request, so a
	// withdrawal from another ChairLift process (the first-run window can run
	// at the same time) stops it. It also combines that value with the
	// in-memory one, so a local Off stops it at once, even while its save
	// runs; a dry run uses the in-memory value alone.
	consent := uh.consentCopy
	if !IsDryRun() && uh.reportingConsentAt != "" {
		fromFile := reporting.FileConsent(uh.reportingConsentAt)
		consent = func() reporting.Consent {
			return reporting.Stricter(uh.consentCopy(), fromFile())
		}
	}
	return reporting.RunInput{
		Now:       time.Now,
		GroupOn:   uh.config.IsGroupEnabled("problems_page", "problems_reporting_group"),
		Consent:   consent,
		StatePath: statePath,
		Items:     items,
		Envelope:  envelope,
		Client:    uh.reportingClient,
		Sender:    reporting.NewHTTPSender(uh.reportingEndpoint),
		DryRun:    IsDryRun(),
		Pace:      sendPace,
		Log:       func(line string) { log.Println(line) },
	}
}

// runAutomaticReports sends what the consent and the budget allow, off the
// GTK thread, then refreshes the sent list on it. It runs at most once at a
// time.
func (uh *UserHome) runAutomaticReports() {
	if uh.reportingRunning || !uh.consentCopy().Allows() || uh.reportingClient == "" || uh.reportingEndpointErr != nil {
		return
	}
	uh.reportingRunning = true
	in := uh.reportingInput()
	go func() {
		res := reporting.Run(context.Background(), in)
		for _, r := range res.Previews {
			log.Println(r.Text())
		}
		if res.Skipped != "" {
			log.Println("problems: no report was sent: " + res.Skipped)
		}
		if res.Err != nil {
			log.Printf("problems: the automatic report send stopped: %v", res.Err)
		}
		sgtk.RunOnMainThread(func() {
			uh.reportingRunning = false
			if len(res.Sent) == 0 {
				return
			}
			if state, err := reporting.LoadState(in.StatePath); err == nil {
				uh.reportingMu.Lock()
				uh.reportingState = state
				uh.reportingMu.Unlock()
				uh.showSentReports(state)
			}
			for _, entry := range res.Sent {
				if row, ok := uh.reportingRows[entry.ProblemID]; ok {
					row.button.SetSensitive(false)
				}
			}
			uh.refreshCountRow()
		})
	}()
}

// recordSent adds a report the dialog sent to the sent list. Main thread
// only.
func (uh *UserHome) recordSent(entry reporting.Entry) {
	uh.reportingMu.Lock()
	state := uh.reportingState
	if state == nil {
		state = &reporting.State{}
	}
	if entry.Kind == problems.KindCount {
		state.RecordCount(entry.Week, entry)
	} else {
		state.Record(entry)
	}
	uh.reportingState = state
	uh.reportingMu.Unlock()
	uh.showSentReports(state)
	uh.refreshCountRow()
}

// markReportSent disables the Report… button of a problem that was sent and
// labels it "Sent". Main thread only.
func (uh *UserHome) markReportSent(report reporting.Report) {
	if row, ok := uh.reportingRows[report.ProblemID]; ok && row.button != nil {
		row.button.SetLabel(pageview.ReportedLabel)
		row.button.SetSensitive(false)
	}
	uh.refreshReportButtons()
}

// showSentReports fills the "Sent reports" expander. Main thread only.
func (uh *UserHome) showSentReports(state *reporting.State) {
	if uh.reportingSent == nil {
		return
	}
	for _, widget := range uh.reportingSentRows {
		uh.reportingSent.Remove(widget)
	}
	uh.reportingSentRows = nil
	rows := pageview.SentRows(state, time.Local)
	if len(rows) == 0 {
		uh.reportingSent.SetSubtitle(pageview.ReportingSentEmpty)
		return
	}
	uh.reportingSent.SetSubtitle("")
	for _, row := range rows {
		child := problemsTextRow(row)
		uh.reportingSent.AddRow(child)
		uh.reportingSentRows = append(uh.reportingSentRows, child)
	}
}

// refreshReportButtons shows a Report… button only for a problem the consent
// allows, that is reportable, whose kind is not crash, and whose week was not
// sent. Main thread only.
func (uh *UserHome) refreshReportButtons() {
	if uh.reportingSwitch == nil || uh.reportingRows == nil {
		return
	}
	uh.reportingMu.Lock()
	allowed := uh.reportingConsent.Allows() && uh.reportingEndpointErr == nil
	state := uh.reportingState
	uh.reportingMu.Unlock()
	for _, row := range uh.reportingRows {
		if row.button.GetLabel() == pageview.ReportedLabel {
			// Sent in this run: the row keeps the "Sent" button on screen.
			continue
		}
		row.button.SetVisible(pageview.ReportOffered(row.item, allowed, state))
	}
}

// refreshCountRow shows the weekly count row in Ask mode while this week's
// count was not sent. Main thread only.
func (uh *UserHome) refreshCountRow() {
	if uh.reportingCountRow == nil {
		return
	}
	uh.reportingMu.Lock()
	consent := uh.reportingConsent
	state := uh.reportingState
	uh.reportingMu.Unlock()
	row, offered := pageview.ReportingCountRow(consent, state, time.Now())
	uh.reportingCountRow.SetSubtitle(glib.MarkupEscapeText(row.Subtitle, -1))
	uh.reportingCountRow.SetVisible(offered)
}

// reviewWeeklyCount builds this week's count report off the GTK thread and
// opens the same dialog the problem rows use.
func (uh *UserHome) reviewWeeklyCount() {
	in := uh.reportingInput()
	go func() {
		age, channel, ok := reporting.DefaultInstallInfo(context.Background())
		sgtk.RunOnMainThread(func() {
			if !ok || !in.Envelope.Valid {
				uh.toastAdder.ShowErrorToast("The weekly count has no install age on this computer")
				return
			}
			report, err := reporting.NewCountReport(in.Envelope, in.Client, age, channel, time.Now())
			if err != nil {
				log.Printf("problems: build the weekly count: %v", err)
				uh.toastAdder.ShowErrorToast("Could not build the weekly count: " + err.Error())
				return
			}
			uh.openReportDialog(pageview.ReportingCountTitle, report, nil)
		})
	}()
}

// problemsTextRow returns a plain row; its text is escaped.
func problemsTextRow(text pageview.Row) *gtk.Widget {
	row := adw.NewActionRow()
	row.SetTitle(glib.MarkupEscapeText(text.Title, -1))
	if text.Subtitle != "" {
		row.SetSubtitle(glib.MarkupEscapeText(text.Subtitle, -1))
	}
	return &row.Widget
}
