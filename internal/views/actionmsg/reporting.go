package actionmsg

import "fmt"

// ReportDecision is the single decision of the Problems page's Report
// dialog Send button (ADR-0009): whether the page calls
// reporting.SendReport, whether the problem is marked sent afterwards, the
// toast, and the line to log.
type ReportDecision struct {
	// Send is true when the page sends the report. It is exactly !dryRun:
	// a dry run previews the report and sends nothing.
	Send bool
	// MarkSent is true when the page marks the row "Sent" and adds the
	// entry to "Sent reports" after the send succeeds. A dry run marks
	// nothing.
	MarkSent bool
	// Toast is the message to show; for a live send, after it succeeds.
	Toast string
	// Log is the line to log, or "".
	Log string
}

// Report decides the Report dialog's Send button for a report of kind.
// Dry run: no send, no mark, the toast
// "[DRY-RUN] Preview: report <kind> — no changes made" and the log line
// "[DRY-RUN] would send report <kind>". Live: send, then mark the row and
// show "Sent report <kind>".
func Report(dryRun bool, kind string) ReportDecision {
	if dryRun {
		return ReportDecision{
			Toast: fmt.Sprintf("[DRY-RUN] Preview: report %s — no changes made", kind),
			Log:   fmt.Sprintf("[DRY-RUN] would send report %s", kind),
		}
	}
	return ReportDecision{
		Send:     true,
		MarkSent: true,
		Toast:    fmt.Sprintf("Sent report %s", kind),
	}
}

// ReportFailed is the error toast of a report whose send failed. The row is
// not marked sent, so the report can be sent again.
func ReportFailed(kind string, err error) string {
	return fmt.Sprintf("Could not send the %s report: %v", kind, err)
}

// ConsentDecision is the single decision of the reporting group's switch and
// mode row (ADR-0009): whether the page writes the consent to the consent
// file, the toast, and the line to log.
type ConsentDecision struct {
	// Persist is true when the page writes the consent. It is exactly
	// !dryRun; a dry run keeps the choice in memory for this run only.
	Persist bool
	// Toast is the message to show; for a live decision, after the write
	// succeeds.
	Toast string
	// Log is the line to log, or "".
	Log string
}

// ReportingConsent decides the reporting switch for an enabled state and a
// consent mode ("ask" or "auto"). Dry run: nothing is written, the toast
// "[DRY-RUN] Preview: problem reporting <on|off> — no changes made" and the
// log line "[DRY-RUN] would save problem reporting enabled=<b> mode=<m>".
// Live: write, and show "Problem reporting is on" or "Problem reporting is
// off".
func ReportingConsent(dryRun, enabled bool, mode string) ConsentDecision {
	if mode != "auto" {
		mode = "ask"
	}
	if dryRun {
		return ConsentDecision{
			Toast: fmt.Sprintf("[DRY-RUN] Preview: problem reporting %s — no changes made", onOff(enabled)),
			Log:   fmt.Sprintf("[DRY-RUN] would save problem reporting enabled=%t mode=%s", enabled, mode),
		}
	}
	return ConsentDecision{
		Persist: true,
		Toast:   "Problem reporting is " + onOff(enabled),
	}
}

// onOff returns "on" or "off".
func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}
