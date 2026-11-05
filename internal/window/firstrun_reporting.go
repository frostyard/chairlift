package window

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/firstrun"
	"github.com/frostyard/chairlift/internal/i18n"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/views/actionmsg"

	sgtk "github.com/frostyard/snowkit/gtk"

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

	// 1 serial writer for the consent: the latest choice wins, and a fast On
	// then Off cannot end On (ADR-0015).
	w.reportingWrite = reporting.NewConsentWriter(w.reportingRecord, func(c reporting.Consent) error {
		if w.reportingPath == "" {
			return errNoConsentPath
		}
		return reporting.SaveConsent(w.reportingPath, c)
	}, w.onReportingWritten)

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

// onReportingSwitched takes the user's choice. The wanted consent is stored
// and shown at once, so the switch reflects the choice whatever the write
// does; the serial writer saves it. A dry run writes nothing (ADR-0009).
func (w *FirstRunWindow) onReportingSwitched(enabled bool) {
	if !reporting.SwitchChanged(w.reportingRecord, enabled) {
		return
	}
	mode := w.reportingRecord.Mode
	if mode != reporting.ModeAuto {
		mode = reporting.ModeAsk
	}
	decision := actionmsg.ReportingConsent(firstrun.IsDryRun(), enabled, string(mode))
	if decision.Log != "" {
		log.Println(decision.Log)
	}
	record := reporting.Decide(enabled, mode, time.Now())
	w.reportingRecord = record
	w.showReporting()
	if !decision.Persist {
		w.toasts.AddToast(plainToast(decision.Toast))
		return
	}
	if w.reportingWrite == nil {
		w.reportingRecord = reporting.Consent{}
		w.showReporting()
		w.toasts.AddToast(plainToast(i18n.T(i18n.MsgReportingSaveFailed)))
		return
	}
	w.reportingWrite.Set(record)
}

// onReportingWritten handles 1 consent write on the GTK main thread. A result
// that a newer choice already replaced is ignored, and a failed latest write
// puts the value on disk back on the switch.
func (w *FirstRunWindow) onReportingWritten(res reporting.ConsentResult) {
	sgtk.RunOnMainThread(func() {
		if res.Err != nil {
			log.Printf("first-run: save problem reporting: %v", res.Err)
			if res.Restored {
				w.reportingRecord = res.Persisted
				w.showReporting()
				w.toasts.AddToast(plainToast(i18n.T(i18n.MsgReportingSaveFailed)))
			}
			return
		}
		if w.reportingWrite == nil || res.Gen != w.reportingWrite.Current() {
			return
		}
		w.toasts.AddToast(plainToast(actionmsg.ReportingConsent(false, res.Consent.Allows(), string(res.Consent.Mode)).Toast))
	})
}

// showReporting applies the stored consent to the step's switch and text.
// Main thread only.
func (w *FirstRunWindow) showReporting() {
	if w.reportingSwitch == nil {
		return
	}
	w.reportingSwitch.SetActive(w.reportingRecord.Allows())
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

// errNoConsentPath is the failure of a consent write when this user has no
// config home.
var errNoConsentPath = errors.New("first-run: the consent file has no path")

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
