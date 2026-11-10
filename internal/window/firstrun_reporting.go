package window

import (
	"log"
	"os"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/firstrun"
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/i18n"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
)

// This file holds the optional problem-reporting step of the first run
// (ADR-0015). It is user-level: it runs no privileged command, and it never
// records the completion. The switch writes the same consent file the
// Problems page writes, through internal/reporting.

// reportingStep draws the Reporting step's content: the consent switch
// (off unless the consent file says on) with its status, and a button that
// opens the privacy page.
func (w *FirstRun) reportingStep(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(frostui.SpaceM).Padding(14, 16).Radius(frostui.RadiusCard).Background(t.Background).Border(1, t.Border).
		AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			frostui.Strong(c, i18n.T(i18n.MsgReportingSwitch))
			frostui.Muted(c, i18n.T(firstrun.ReportingStatusMsgid(w.reportingRecord)))
		})
		// The switch shows the record in memory; it changes only through
		// onReportingSwitched, so a redraw cannot start a second save.
		on := w.reportingRecord.Allows()
		if ui.Switch(c, &on).Label(i18n.T(i18n.MsgReportingSwitch)).Changed() {
			w.onReportingSwitched(on)
		}
	})
	ui.Row(c).Justify(ui.Center).Children(func() {
		if frostui.LabeledButton(c, "web-browser", i18n.T(i18n.MsgReportingWhat), false).Clicked() {
			w.openPrivacyPage(c)
		}
	})
}

// onReportingSwitched takes the user's choice. The consent file is about 200
// bytes, so it is written here, on the UI thread, with
// reporting.SaveConsent: a small local file write, and because it is
// synchronous an exit cannot lose an Off. One value in memory
// (w.reportingRecord) is the only authority.
//
// A withdrawal (Off) reaches the in-memory value BEFORE the save, so a
// running ChairLift window that sends stops at once; a grant reaches it only
// after a successful save. The first run sends nothing, so this only orders
// the text.
func (w *FirstRun) onReportingSwitched(enabled bool) {
	if !reporting.SwitchChanged(w.reportingRecord, enabled) {
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
	record := reporting.Decide(enabled, mode, w.now())
	withdraw := before.Allows() && (!record.Allows() || record.Mode != reporting.ModeAuto)
	if !decision.Persist {
		w.reportingRecord = record
		w.toast(decision.Toast)
		return
	}
	if w.reportingPath == "" {
		if withdraw {
			w.reportingRecord = record
		}
		w.toast(i18n.T(i18n.MsgReportingSaveFailed))
		return
	}
	if withdraw {
		w.reportingRecord = record
	}
	if err := w.saveConsent(w.reportingPath, record); err != nil {
		log.Printf("first-run: save problem reporting: %v", err)
		// A withdrawal stays in memory (fail safe); a grant never arrived, so
		// the switch keeps the value in memory.
		w.toast(i18n.T(i18n.MsgReportingSaveFailed))
		return
	}
	if !withdraw {
		w.reportingRecord = record
	}
	w.toast(decision.Toast)
}

// openPrivacyPage opens the privacy page in the default browser. It is a
// launch, not a state change, so it also runs in a dry run.
func (w *FirstRun) openPrivacyPage(c *ui.Context) {
	c.OpenURLThen(reporting.PrivacyURL, func(err error) {
		if err != nil {
			log.Printf("first-run: open %s: %v", reporting.PrivacyURL, err)
		}
	})
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
