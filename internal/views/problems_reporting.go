package views

import (
	"context"
	"log"

	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/pageview"
)

// Problem reporting (ADR-0015 decision 10, docs/specs/problem-reporting.md).
// internal/reporting is the only sender. Nothing is sent until the user
// turns reporting on: the consent is off by default and the controls stay
// disabled until the first load published the stored consent. In Ask mode
// only the Report dialog sends, and it posts the very Report.Body bytes it
// shows; in Auto mode reporting.Run sends within the budget, paced, under
// the one send lock. A kind crash report is never offered.

// reportDialog is the open Report dialog: the exact report Send posts.
type reportDialog struct {
	open   bool
	title  string
	report reporting.Report
	// onSent runs on the UI thread after a live send succeeded.
	onSent func()
}

// setUpReporting finds the consent file and the sent log of this user and
// checks the configured endpoint. An invalid administrator setting
// disables every send path: it never falls back to the public default.
func (p *Problems) setUpReporting() {
	if path, err := reporting.ConsentPath(p.getenv); err != nil {
		log.Printf("problems: the reporting consent has no path: %v", err)
	} else {
		p.reportingConsentAt = path
	}
	if path, err := reporting.StatePath(p.getenv); err != nil {
		log.Printf("problems: the sent report log has no path: %v", err)
	} else {
		p.reportingStateAt = path
	}
	endpoint, err := reporting.ParseEndpoint(p.reportedEndpoint())
	if err != nil {
		log.Printf("problems: problem reporting is off: %v", err)
		p.reportingEndpointErr = err
	} else {
		p.reportingEndpoint = endpoint
	}
}

// reportedEndpoint returns the endpoint of an administrator-owned
// configuration file, or "" for the default (config.Config.ReportEndpoint).
func (p *Problems) reportedEndpoint() string {
	if p.env.Config == nil {
		return ""
	}
	raw, adminOwned := p.env.Config.ReportEndpoint()
	if !adminOwned {
		return ""
	}
	return raw
}

// consentCopy returns a copy of the wanted consent, safe from any goroutine.
func (p *Problems) consentCopy() reporting.Consent {
	p.reportingMu.Lock()
	defer p.reportingMu.Unlock()
	return p.reportingConsent
}

// stateCopy returns the sent log the page knows. UI thread.
func (p *Problems) stateCopy() *reporting.State {
	p.reportingMu.Lock()
	defer p.reportingMu.Unlock()
	return p.reportingState
}

// publishConsent makes consent the wanted consent for the page and the send
// goroutine. The view draws it from there. UI thread.
func (p *Problems) publishConsent(consent reporting.Consent) {
	p.reportingMu.Lock()
	p.reportingConsent = consent
	p.reportingMu.Unlock()
}

// selectedMode returns the mode the mode control shows: the consent's.
func (p *Problems) selectedMode() reporting.Mode {
	if p.consentCopy().Mode == reporting.ModeAuto {
		return reporting.ModeAuto
	}
	return reporting.ModeAsk
}

// modeIndex returns the mode control's segment of a mode.
func modeIndex(mode reporting.Mode) int {
	if mode == reporting.ModeAuto {
		return 1
	}
	return 0
}

// modeOf returns the mode of a segment of the mode control.
func modeOf(index int) reporting.Mode {
	if index == 1 {
		return reporting.ModeAuto
	}
	return reporting.ModeAsk
}

// onReportingSwitched handles the consent switch (and the palette command
// that flips it). A change back to the state the switch shows does nothing;
// a stale consent shows off, so turning it on renews it
// (reporting.SwitchChanged).
func (p *Problems) onReportingSwitched(enabled bool) {
	if !p.reportingLoaded || p.reportingApplying || !reporting.SwitchChanged(p.consentCopy(), enabled) {
		return
	}
	p.applyReportingConsent(enabled, p.selectedMode())
}

// onReportingModeSelected handles the mode control, which works only while
// reporting is on.
func (p *Problems) onReportingModeSelected(mode reporting.Mode) {
	if !p.reportingLoaded || p.reportingApplying {
		return
	}
	current := p.consentCopy()
	if mode == current.Mode || !current.Allows() {
		return
	}
	p.applyReportingConsent(true, mode)
}

// applyReportingConsent takes the user's choice: actionmsg.ReportingConsent
// decides the toast and whether the consent file is written, exactly once
// (ADR-0009).
//
// The consent file is about 200 bytes, so it is written here, on the UI
// thread, with reporting.SaveConsent (ADR-0015): a small local file write,
// not an external tool call. Because the write is synchronous, an exit
// cannot lose an Off and no callback can restore an older value.
//
// A withdrawal (Off, or leaving automatic) reaches the in-memory value
// BEFORE the save, so a running send stops at once even while the save
// runs; a grant reaches it only after a successful save.
func (p *Problems) applyReportingConsent(enabled bool, mode reporting.Mode) {
	decision := actionmsg.ReportingConsent(p.env.DryRun, enabled, string(mode))
	if decision.Log != "" {
		log.Println(decision.Log)
	}
	before := p.consentCopy()
	record := reporting.Decide(enabled, mode, p.env.Now())
	withdraw := before.Allows() && (!record.Allows() || record.Mode != reporting.ModeAuto)

	if !decision.Persist {
		// A dry run keeps the choice in memory for this run only. A new
		// automatic choice previews its reports: Run sends nothing and
		// writes no file.
		p.withoutSaving(func() { p.publishConsent(record) })
		p.env.Toast(decision.Toast)
		if reporting.StartsAutoRun(before, record) {
			p.runAutomaticReports(false)
		}
		return
	}

	path := p.reportingConsentAt
	if path == "" {
		if withdraw {
			p.withoutSaving(func() { p.publishConsent(record) })
		}
		p.env.ToastError("Could not save the problem reporting setting: no consent file path")
		return
	}
	if withdraw {
		p.withoutSaving(func() { p.publishConsent(record) })
	}
	if err := p.saveConsent(path, record); err != nil {
		log.Printf("problems: save the reporting consent: %v", err)
		// A withdrawal stays in memory (fail safe); a grant never arrived,
		// so the controls keep showing the value in memory.
		p.env.ToastError("Could not save the problem reporting setting: " + err.Error())
		return
	}
	if !withdraw {
		p.withoutSaving(func() { p.publishConsent(record) })
	}
	p.env.ToastSuccess(decision.Toast)
	p.env.Record("problems", decision.Toast, pageview.ReportingDescription(record))
	if reporting.StartsAutoRun(before, record) {
		p.runAutomaticReports(false)
	}
}

// withoutSaving runs fn with the applying guard set, so nothing fn changes
// can start a second save. UI thread.
func (p *Problems) withoutSaving(fn func()) {
	previous := p.reportingApplying
	p.reportingApplying = true
	defer func() { p.reportingApplying = previous }()
	fn()
}

// reportingInput builds the input of a manual or automatic send from the
// page's state, on the UI thread; the send goroutine only reads the values
// it copies.
func (p *Problems) reportingInput() reporting.RunInput {
	p.reportingMu.Lock()
	statePath := p.reportingStateAt
	items := p.reportingItems
	envelope := p.reportingEnvelope
	p.reportingMu.Unlock()
	// A live send reads the consent file again before every request, so a
	// withdrawal from another ChairLift process (the first-run window can
	// run at the same time) stops it. reporting.LiveConsent reads the file
	// first and memory second, so a local Off stops it at once, even while
	// its save runs; a dry run uses the in-memory value alone.
	consent := p.consentCopy
	if !p.env.DryRun && p.reportingConsentAt != "" {
		consent = reporting.LiveConsent(p.consentCopy, reporting.FileConsent(p.reportingConsentAt))
	}
	// Only ParseEndpoint chooses the endpoint; without a valid one there is
	// no sender, and Run and SendReport send nothing.
	var sender reporting.Sender
	if p.reportingEndpointErr == nil && p.reportingEndpoint != "" {
		sender = p.newSender(p.reportingEndpoint)
	}
	return reporting.RunInput{
		Now:         p.env.Now,
		GroupOn:     p.env.Enabled(problemsPage, "problems_reporting_group"),
		Consent:     consent,
		StatePath:   statePath,
		Items:       items,
		Envelope:    envelope,
		Client:      p.reportingClient,
		InstallInfo: p.installInfo,
		Sender:      sender,
		DryRun:      p.env.DryRun,
		Pace:        p.pace,
		Log:         func(line string) { log.Println(line) },
	}
}

// sendPendingBlocked says why "Send pending reports" cannot run now, ""
// when it can: in Auto mode with a valid endpoint and a release build, and
// not while a run is going.
func (p *Problems) sendPendingBlocked() string {
	consent := p.consentCopy()
	switch {
	case !p.reportingLoaded:
		return "loading"
	case !consent.Allows() || consent.Mode != reporting.ModeAuto:
		return "reports are not sent automatically"
	case p.reportingEndpointErr != nil:
		return "the report endpoint is not valid"
	case !problems.ValidClient(p.reportingClient):
		return "not a release build"
	case p.reportingRunning:
		return "sending"
	}
	return ""
}

// autoRun is what 1 automatic run gives back to the UI thread.
type autoRun struct {
	res     reporting.RunResult
	state   *reporting.State
	stateOK bool
}

// runAutomaticReports sends what the consent and the budget allow, on a
// worker goroutine, then refreshes the sent list on the UI thread. It runs
// at most once at a time. manual is true when the user asked for it
// ("Send pending reports"): then a toast says what it did.
func (p *Problems) runAutomaticReports(manual bool) {
	if p.reportingRunning || !p.consentCopy().Allows() || p.reportingClient == "" || p.reportingEndpointErr != nil {
		return
	}
	p.reportingRunning = true
	in := p.reportingInput()
	run, loadState := p.runReports, p.loadState
	dryRun := p.env.DryRun
	Go(p.env, func() autoRun {
		res := run(context.Background(), in)
		for _, r := range res.Previews {
			log.Println(r.Text())
		}
		if res.Skipped != "" {
			log.Println("problems: no report was sent: " + res.Skipped)
		}
		if res.Err != nil {
			log.Printf("problems: the automatic report send stopped: %v", res.Err)
		}
		out := autoRun{res: res}
		if len(res.Sent) > 0 {
			if state, err := loadState(in.StatePath); err == nil {
				out.state, out.stateOK = state, true
			}
		}
		return out
	}, func(out autoRun) {
		p.reportingRunning = false
		if manual {
			if out.res.Err != nil {
				p.env.ToastError("Could not send the pending reports: " + out.res.Err.Error())
			} else {
				p.env.Toast(pageview.ReportingRunToast(out.res, dryRun))
			}
		}
		if len(out.res.Sent) == 0 {
			return
		}
		if out.stateOK {
			p.reportingMu.Lock()
			p.reportingState = out.state
			p.reportingMu.Unlock()
		}
		for _, entry := range out.res.Sent {
			if entry.ProblemID != "" {
				p.reported[entry.ProblemID] = true
			}
			p.env.Record("problems", "Sent a problem report", pageview.SentKindTitle(entry.Kind)+" · "+entry.ReportID)
		}
	})
}

// recordSent adds a report the dialog sent to the sent list. UI thread.
func (p *Problems) recordSent(entry reporting.Entry) {
	p.reportingMu.Lock()
	defer p.reportingMu.Unlock()
	state := p.reportingState
	if state == nil {
		state = &reporting.State{}
	}
	if entry.Kind == problems.KindCount {
		state.RecordCount(entry.Week, entry)
	} else {
		state.Record(entry)
	}
	p.reportingState = state
}

// markReportSent shows "Sent" on the card of a problem that was sent. UI
// thread.
func (p *Problems) markReportSent(report reporting.Report) {
	if report.ProblemID != "" {
		p.reported[report.ProblemID] = true
	}
}

// reportOffered reports whether a problem's card shows Report…: the
// reporting group is on and pageview.ReportOffered allows it (reporting
// allowed with a valid endpoint, a reportable item that is not a crash,
// not sent this week).
func (p *Problems) reportOffered(item problems.Item) bool {
	if !p.reportingOn {
		return false
	}
	p.reportingMu.Lock()
	allowed := p.reportingConsent.Allows() && p.reportingEndpointErr == nil
	state := p.reportingState
	p.reportingMu.Unlock()
	return pageview.ReportOffered(item, allowed, state)
}

// reportItem builds a problem's report on a worker goroutine and opens the
// Report dialog with it.
func (p *Problems) reportItem(item problems.Item, title string) {
	if p.reportBusy[item.ID] {
		return
	}
	p.reportBusy[item.ID] = true
	forItem, client := p.forItem, p.reportingClient
	type built struct {
		report reporting.Report
		err    error
	}
	Go(p.env, func() built {
		report, err := forItem(item, client)
		return built{report, err}
	}, func(b built) {
		delete(p.reportBusy, item.ID)
		if b.err != nil {
			log.Printf("problems: build the report of %s: %v", item.ID, b.err)
			p.env.ToastError("Could not build the report: " + b.err.Error())
			return
		}
		report := b.report
		p.openReportDialog(title, report, func() { p.markReportSent(report) })
	})
}

// reviewWeeklyCount builds this week's count report on a worker goroutine
// and opens the same dialog the problem cards use.
func (p *Problems) reviewWeeklyCount() {
	in := p.reportingInput()
	info := p.installInfo
	type count struct {
		age, channel string
		ok           bool
	}
	Go(p.env, func() count {
		age, channel, ok := info(context.Background())
		return count{age, channel, ok}
	}, func(n count) {
		if !n.ok || !in.Envelope.Valid {
			p.env.ToastError("The weekly count has no install age on this computer")
			return
		}
		report, err := reporting.NewCountReport(in.Envelope, in.Client, n.age, n.channel, p.env.Now())
		if err != nil {
			log.Printf("problems: build the weekly count: %v", err)
			p.env.ToastError("Could not build the weekly count: " + err.Error())
			return
		}
		p.openReportDialog(pageview.ReportingCountTitle, report, nil)
	})
}

// openReportDialog shows the exact report the Send button posts: the same
// bytes, monospaced, selectable and scrollable (ADR-0015).
func (p *Problems) openReportDialog(title string, report reporting.Report, onSent func()) {
	p.dialog = reportDialog{open: true, title: title, report: report, onSent: onSent}
}

// cancelReport closes the Report dialog; nothing is sent.
func (p *Problems) cancelReport() {
	p.dialog = reportDialog{}
}

// sendDialogReport sends the report the dialog shows. actionmsg.Report
// decides it: a dry run previews it and sends nothing; a live send posts
// the same Report value, its Body, which the dialog showed as Text().
func (p *Problems) sendDialogReport() {
	report, onSent := p.dialog.report, p.dialog.onSent
	p.dialog = reportDialog{}
	decision := actionmsg.Report(p.env.DryRun, report.Kind)
	if decision.Log != "" {
		log.Println(decision.Log)
	}
	if !decision.Send {
		p.env.Toast(decision.Toast)
		return
	}
	in := p.reportingInput()
	send := p.sendReport
	if report.ProblemID != "" {
		p.reportBusy[report.ProblemID] = true
	}
	type sent struct {
		entry reporting.Entry
		err   error
	}
	Go(p.env, func() sent {
		entry, err := send(context.Background(), in, report)
		return sent{entry, err}
	}, func(s sent) {
		delete(p.reportBusy, report.ProblemID)
		if s.err != nil {
			log.Printf("problems: send the %s report: %v", report.Kind, s.err)
			p.env.ToastError(actionmsg.ReportFailed(report.Kind, s.err))
			return
		}
		if decision.MarkSent {
			p.recordSent(s.entry)
			if onSent != nil {
				onSent()
			}
		}
		p.env.ToastSuccess(decision.Toast)
		p.env.Record("problems", "Sent a problem report", pageview.SentKindTitle(report.Kind)+" · "+s.entry.ReportID)
	})
}
