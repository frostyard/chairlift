package window

import (
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/firstrun"
	"github.com/frostyard/chairlift/internal/i18n"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/views/actionmsg"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// This file holds the optional problem-reporting step of the first run
// (ADR-0015). It is user-level: it runs no privileged command, and it never
// records the completion. The switch writes the same consent file the
// Problems page writes, through internal/reporting.

// buildReporting builds the optional Reporting step: a short text, the
// consent switch (off unless the consent file says on), and a button that
// opens the privacy page.
func (w *FirstRunWindow) buildReporting() *adw.StatusPage {
	page := adw.NewStatusPage()
	page.SetIconName("dialog-warning-symbolic")
	page.SetTitle(i18n.T(i18n.MsgReportingTitle))
	page.SetDescription(i18n.T(i18n.MsgReportingDesc))
	w.reportingPage = page

	w.reportingSwitch = adw.NewSwitchRow()
	w.reportingSwitch.SetTitle(glib.MarkupEscapeText(i18n.T(i18n.MsgReportingSwitch), -1))
	w.reportingSwitch.SetSubtitle(glib.MarkupEscapeText(i18n.T(i18n.MsgReportingOff), -1))

	// The switch row has no typed signal: its state is the "active"
	// property, so the step follows "notify" and keeps the active change.
	notifyCb := func(_ gobject.Object, pspec uintptr) {
		if gobject.ParamSpecNewFromInternalPtr(pspec).GetName() != "active" {
			return
		}
		w.onReportingSwitched(w.reportingSwitch.GetActive())
	}
	w.reportingSwitch.ConnectNotify(&notifyCb)
	if w.reportingRecord.Allows() {
		w.reportingSwitch.SetActive(true)
	}
	w.showReporting()

	group := adw.NewPreferencesGroup()
	group.Add(&w.reportingSwitch.Widget)

	what := gtk.NewButtonWithLabel(i18n.T(i18n.MsgReportingWhat))
	what.SetHalign(gtk.AlignCenterValue)
	what.AddCssClass("pill")
	whatCb := func(gtk.Button) { w.openPrivacyPage() }
	what.ConnectClicked(&whatCb)

	clamp := adw.NewClamp()
	clamp.SetMaximumSize(600)
	clamp.SetChild(&group.Widget)
	box := gtk.NewBox(gtk.OrientationVerticalValue, 24)
	box.Append(&clamp.Widget)
	box.Append(&what.Widget)
	page.SetChild(&box.Widget)
	return page
}

// onReportingSwitched takes the user's choice. The consent file is about 200
// bytes, so it is written here, on the GTK main thread, with
// reporting.SaveConsent: a small local file write, and because it is
// synchronous an exit cannot lose an Off. One value in memory
// (w.reportingRecord) is the only authority; a redraw sets
// w.reportingApplying, so it cannot start a second save.
//
// A withdrawal (Off) reaches the in-memory value BEFORE the save, so a
// running ChairLift window that sends stops at once; a grant reaches it only
// after a successful save. The first run sends nothing, so this only orders
// the text.
func (w *FirstRunWindow) onReportingSwitched(enabled bool) {
	if w.reportingApplying || !reporting.SwitchChanged(w.reportingRecord, enabled) {
		return
	}
	before := w.reportingRecord
	mode := before.Mode
	if mode != reporting.ModeAuto {
		mode = reporting.ModeAsk
	}
	decision := actionmsg.ReportingConsent(firstrun.IsDryRun(), enabled, string(mode))
	if decision.Log != "" {
		log.Println(decision.Log)
	}
	record := reporting.Decide(enabled, mode, time.Now())
	withdraw := before.Allows() && (!record.Allows() || record.Mode != reporting.ModeAuto)
	if !decision.Persist {
		w.reportingRecord = record
		w.showReporting()
		w.toasts.AddToast(plainToast(decision.Toast))
		return
	}
	if w.reportingPath == "" {
		if withdraw {
			w.reportingRecord = record
		}
		w.showReporting()
		w.toasts.AddToast(plainToast(i18n.T(i18n.MsgReportingSaveFailed)))
		return
	}
	if withdraw {
		w.reportingRecord = record
		w.showReporting()
	}
	if err := reporting.SaveConsent(w.reportingPath, record); err != nil {
		log.Printf("first-run: save problem reporting: %v", err)
		// A withdrawal stays in memory (fail safe); a grant never arrived, so
		// the switch goes back to the value in memory.
		if !withdraw {
			w.showReporting()
		}
		w.toasts.AddToast(plainToast(i18n.T(i18n.MsgReportingSaveFailed)))
		return
	}
	if !withdraw {
		w.reportingRecord = record
		w.showReporting()
	}
	w.toasts.AddToast(plainToast(decision.Toast))
}

// showReporting applies the stored consent to the step's switch and text.
// Main thread only.
func (w *FirstRunWindow) showReporting() {
	if w.reportingSwitch == nil {
		return
	}
	applying := w.reportingApplying
	w.reportingApplying = true
	w.reportingSwitch.SetActive(w.reportingRecord.Allows())
	w.reportingApplying = applying
	w.reportingSwitch.SetSubtitle(glib.MarkupEscapeText(i18n.T(firstrun.ReportingStatusMsgid(w.reportingRecord)), -1))
}

// openPrivacyPage opens the privacy page in the default browser with
// xdg-open. It is a launch, not a state change, so it also runs in a dry run.
func (w *FirstRunWindow) openPrivacyPage() {
	cmd := exec.Command("xdg-open", reporting.PrivacyURL)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		log.Printf("first-run: open %s: %v", reporting.PrivacyURL, err)
		return
	}
	go func() { _ = cmd.Wait() }()
}

// plainToast returns a toast with plain text, so an error cannot be read as
// markup.
func plainToast(text string) *adw.Toast {
	toast := adw.NewToast(text)
	toast.SetUseMarkup(false)
	return toast
}

// reportingStepAvailable reports whether the first run shows the optional
// reporting step, and reads the saved consent for its switch.
func reportingStepAvailable(cfg *config.Config) (bool, reporting.Consent, string) {
	groupOn := true
	if cfg != nil {
		groupOn = cfg.IsGroupEnabled("problems_page", "problems_reporting_group")
	}
	if !firstrun.ReportingAvailable(groupOn, os.Stat) {
		return false, reporting.Consent{}, ""
	}
	path, err := reporting.ConsentPath(os.Getenv)
	if err != nil {
		return false, reporting.Consent{}, ""
	}
	consent, err := reporting.LoadConsent(path)
	if err != nil {
		log.Printf("first-run: problem reporting is off: %v", err)
	}
	return true, consent, path
}
