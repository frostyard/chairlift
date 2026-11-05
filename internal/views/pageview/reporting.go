package pageview

import (
	"fmt"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
)

// Reporting group text of the Problems page (ADR-0015). Every function
// returns plain text: the view escapes it. Nothing here sends or writes
// anything, so it is tested on a host without GTK.

// MaxSentRows bounds the "Sent reports" list.
const MaxSentRows = 50

// Fixed texts of the reporting group.
const (
	// ReportingTitle is the group's title.
	ReportingTitle = "Reporting"
	// ReportingSwitchTitle is the consent switch.
	ReportingSwitchTitle = "Send problem reports"
	// ReportingOff is the subtitle while reporting is off.
	ReportingOff = "Off: nothing leaves this computer."
	// ReportingStale is the subtitle when the report format changed: the
	// user must agree again before anything is sent.
	ReportingStale = "The report format changed. Turn reporting on to agree again."
	// ReportingEndpointInvalid is the group's description when the endpoint
	// of an administrator-owned configuration file is not a valid URL: every
	// send path is off, because an invalid setting must never fall back to
	// the public service.
	ReportingEndpointInvalid = "Problem reporting is off: the report endpoint in the configuration is not valid."
	// ReportingModeTitle is the row that chooses between asking and sending
	// automatically.
	ReportingModeTitle = "When a problem is found"
	// ReportingModeAsk sends only from the Report dialog.
	ReportingModeAsk = "Ask each time"
	// ReportingModeAuto sends without asking.
	ReportingModeAuto = "Send automatically"
	// ReportingSentTitle is the expander that lists the sent reports.
	ReportingSentTitle = "Sent reports"
	// ReportingSentEmpty is the expander's description with no sent report.
	ReportingSentEmpty = "No reports have been sent."
	// ReportingWhatTitle is the row that links to the privacy page.
	ReportingWhatTitle = "What is sent"
	// ReportingCountTitle is the weekly count row in Ask mode.
	ReportingCountTitle = "Weekly count"
	// ReportingCountAction is the button that shows and sends the count.
	ReportingCountAction = "Review…"
	// ReportingCountSubtitle is the count row's subtitle when no count was
	// sent this week.
	ReportingCountSubtitle = "Sends the image age and the update channel once a week, with no detail of your problems."
)

// ReportingDescription returns the group's description: the switch's state.
func ReportingDescription(c reporting.Consent) string {
	if c.Stale() {
		return ReportingStale
	}
	if c.Allows() {
		return "On: " + reportingModeText(c.Mode) + "."
	}
	return ReportingOff
}

// ReportingMode returns the mode row's text: the consent's mode. The view
// makes the row sensitive only while reporting allows sending.
func ReportingMode(c reporting.Consent) Row {
	return Row{Title: ReportingModeTitle, Subtitle: reportingModeText(c.Mode)}
}

// reportingModeText returns the display text of a consent mode.
func reportingModeText(mode reporting.Mode) string {
	if mode == reporting.ModeAuto {
		return ReportingModeAuto
	}
	return ReportingModeAsk
}

// ReportingWhat returns the "What is sent" row: the privacy page.
func ReportingWhat() Row {
	return Row{Title: ReportingWhatTitle, Subtitle: reporting.PrivacyURL}
}

// SentRows returns up to 50 sent reports, newest first: the send date and
// the kind's title, with the report ID as the subtitle. loc is the display
// time zone.
func SentRows(state *reporting.State, loc *time.Location) []Row {
	if state == nil {
		return nil
	}
	if loc == nil {
		loc = time.UTC
	}
	entries := append([]reporting.Entry(nil), state.Sent...)
	// The sent log is oldest first; the page shows the newest first.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	if len(entries) > MaxSentRows {
		entries = entries[:MaxSentRows]
	}
	rows := make([]Row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, Row{
			Title:    e.SentAt.In(loc).Format("Jan 2, 2006") + " · " + SentKindTitle(e.Kind),
			Subtitle: e.ReportID,
		})
	}
	return rows
}

// SentKindTitle returns the readable title of a report kind.
func SentKindTitle(kind string) string {
	if kind == problems.KindCount {
		return "Weekly count"
	}
	return problems.Title(problems.Item{Kind: kind, KnownKind: true})
}

// ReportingCountRow returns the weekly count row: the title, the subtitle of
// this week's state, and whether a Review… button shows. The button shows in
// Ask mode only: in Auto mode the count goes out by itself.
func ReportingCountRow(c reporting.Consent, state *reporting.State, now time.Time) (Row, bool) {
	row := Row{Title: ReportingCountTitle, Subtitle: ReportingCountSubtitle}
	if !c.Allows() || c.Mode == reporting.ModeAuto {
		return row, false
	}
	if state != nil && !state.CountDue(now) {
		row.Subtitle = "Sent this week."
		return row, false
	}
	return row, true
}

// ReportOffered reports whether the page shows a Report… button for it:
// reporting is allowed, the item is reportable, its kind is not crash, and
// its week was not sent yet.
func ReportOffered(it problems.Item, allowed bool, state *reporting.State) bool {
	if !allowed || !it.Reportable || it.Kind == problems.KindCrash {
		return false
	}
	return !state.WasSent(it.ID, it.Week)
}

// ReportSummary returns the dialog's description: what the report is and
// what sending it means.
func ReportSummary(kind string) string {
	return fmt.Sprintf("This is the exact %s report that is sent. Nothing else leaves this computer.", SentKindTitle(kind))
}

// ReportedLabel is the row's state after a report was sent.
const ReportedLabel = "Sent"

// ReportButtonLabel is the button that opens the Report dialog.
const ReportButtonLabel = "Report…"
