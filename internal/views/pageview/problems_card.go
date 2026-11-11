package pageview

import (
	"fmt"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
)

// Command-center text of the Problems page (ADR-0015): the page subtitle,
// the section headers, each problem card's lines and severity, and the
// reporting card's limits. Every function returns plain text; the views
// draw it with MyGo, which parses no markup.

// Section headers and card titles of the command-center Problems page.
const (
	ProblemsSystemSection     = "System"
	ProblemsSystemSectionNote = "from snosi-health checks"
	ProblemsAppsSection       = "Your apps"
	ProblemsAppsSectionNote   = "crashes and problems frostyard apps recorded in the last 90 days"
	// ReportingCardTitle heads the reporting card.
	ReportingCardTitle = "Problem reports"
	// ReportingSendPending is the button and command that run the
	// automatic send now, in Auto mode only.
	ReportingSendPending = "Send pending reports"
)

// Severity is how much a problem needs the user, for the tone of its count.
type Severity int

const (
	// SeverityInfo is a problem that resolved itself or only informs.
	SeverityInfo Severity = iota
	// SeverityWarning is a problem the user may want to look at.
	SeverityWarning
	// SeverityDanger is a problem with the system's integrity.
	SeverityDanger
)

// ProblemSeverity returns the severity of a problem from its kind and, for
// some kinds, a validated value:
//
//   - integrity_error is danger: files do not match the signed image.
//   - tpm_unlock that unlocked after a retry, and an app error (not a
//     panic), only inform; an unknown kind informs too.
//   - every other kind is a warning.
func ProblemSeverity(it problems.Item) Severity {
	if !it.KnownKind {
		return SeverityInfo
	}
	switch it.Kind {
	case problems.KindIntegrityError:
		return SeverityDanger
	case problems.KindTPMUnlock:
		if it.ValidString("outcome") == "ok_after_retry" {
			return SeverityInfo
		}
	case problems.KindAppProblem:
		if it.ValidString("class") == "error" {
			return SeverityInfo
		}
	case problems.KindCount:
		return SeverityInfo
	}
	return SeverityWarning
}

// ProblemCard is the text of 1 problem card.
type ProblemCard struct {
	// Title is the problem in plain words (Problem's title).
	Title string
	// Subtitle is "<subject> · last seen <when>", or "Last seen <when>"
	// when the problem names no subject.
	Subtitle string
	// Count is "once" or "<n> times".
	Count string
	// FirstSeen is "First seen <when>", "" when the time is unknown.
	FirstSeen string
	Severity  Severity
}

// ProblemCardText returns the text of the card of it, in loc.
func ProblemCardText(it problems.Item, now time.Time, loc *time.Location) ProblemCard {
	card := ProblemCard{
		Title:    Problem(it, now, loc).Title,
		Count:    ProblemCount(it.Count),
		Severity: ProblemSeverity(it),
	}
	when := "last seen " + whenText(it.LastSeen, now, loc)
	if subject := ProblemSubject(it); subject != "" {
		card.Subtitle = subject + " · " + when
	} else {
		card.Subtitle = capitalize(when)
	}
	if !it.FirstSeen.IsZero() {
		card.FirstSeen = "First seen " + whenText(it.FirstSeen, now, loc)
	}
	return card
}

// ProblemCount returns how often a problem happened: "once" or "<n> times".
// A count below 2 is once: every shown problem happened at least once.
func ProblemCount(n int) string {
	if n < 2 {
		return "once"
	}
	return fmt.Sprintf("%d times", n)
}

// ProblemSubject returns what a problem concerns, in a few words: Problem's
// context (a unit, feature, version, code, or an unknown kind's name), or
// for the kinds it leaves out, the crash's signal, the integrity check's
// subsystem or the security chip's code. It is "" when nothing valid names
// one.
func ProblemSubject(it problems.Item) string {
	if context := problemContext(it); context != "" {
		return context
	}
	switch it.Kind {
	case problems.KindCrash:
		return it.ValidString("signal")
	case problems.KindIntegrityError:
		return it.ValidString("subsystem")
	case problems.KindTPMUnlock:
		if rc := it.ValidString("tpm_rc"); rc != "" {
			return "TPM " + rc
		}
	case problems.KindUpdateStageFailed, problems.KindSysextUpdateFailed:
		return it.ValidString("error_class")
	}
	return ""
}

// ProblemsSubtitle returns the page's subtitle: how many problems it shows
// and whether anything leaves the computer. dataOn is false when both
// problem groups are off; reportingOn is the reporting group.
func ProblemsSubtitle(shown int, loaded, dataOn, reportingOn bool, c reporting.Consent) string {
	var first string
	switch {
	case !dataOn:
		first = "The problem lists are turned off on this computer."
	case !loaded:
		return ProblemsLoading
	case shown == 0:
		first = "No problems recorded on this computer."
	default:
		first = fmt.Sprintf("%d recorded on this computer, in plain words.", shown)
	}
	switch {
	case !reportingOn:
		return first + " Nothing leaves it."
	case c.Allows() && c.Mode == reporting.ModeAuto:
		return first + " Reports are on: they are sent automatically."
	case c.Allows():
		return first + " Reports are on: you see each one before it is sent."
	}
	return first + " Nothing leaves it unless you turn reports on."
}

// ProblemsSystemNotes returns the short status lines of the system
// section's header: "Last checked <when>" while problems are shown (with
// none, ProblemsSystemStatus's row says it), and how many dismissed
// problems are hidden.
func ProblemsSystemNotes(outcome problems.SourceOutcome, generated time.Time, shown, hidden int, now time.Time, loc *time.Location) []string {
	var notes []string
	if outcome == problems.SourceOK && shown > 0 {
		notes = append(notes, LastChecked(generated, now, loc))
	}
	if hidden > 0 {
		notes = append(notes, hiddenText(hidden))
	}
	return notes
}

// ProblemsAppsNotes returns the apps section's status lines: how many
// dismissed problems are hidden.
func ProblemsAppsNotes(hidden int) []string {
	if hidden > 0 {
		return []string{hiddenText(hidden)}
	}
	return nil
}

func hiddenText(hidden int) string {
	if hidden == 1 {
		return "1 dismissed problem is hidden"
	}
	return fmt.Sprintf("%d dismissed problems are hidden", hidden)
}

// ReportingLimits says the limits of sending: the daily budget, 1 report of
// a problem a week, and the weekly count.
func ReportingLimits() string {
	return fmt.Sprintf("At most %d reports a day and each problem once a week, plus one weekly count of the image age and update channel.", reporting.MaxPerDay)
}

// ReportingSentSummary returns the sent list's heading: how many reports
// the sent log holds, or ReportingSentEmpty.
func ReportingSentSummary(state *reporting.State) string {
	n := 0
	if state != nil {
		n = len(state.Sent)
	}
	switch n {
	case 0:
		return ReportingSentEmpty
	case 1:
		return "1 report sent"
	}
	return fmt.Sprintf("%d reports sent", n)
}

// ReportingRunToast returns the toast of a "Send pending reports" run that
// the user started: what it sent or previewed, or why it sent nothing. An
// error is not here: the caller shows its own error toast.
func ReportingRunToast(res reporting.RunResult, dryRun bool) string {
	switch {
	case dryRun && len(res.Previews) > 0:
		return fmt.Sprintf("[DRY-RUN] Preview: %s — no changes made", reportsText(len(res.Previews)))
	case len(res.Sent) > 0:
		return "Sent " + reportsText(len(res.Sent))
	case res.Skipped != "":
		return "No report was sent: " + res.Skipped
	}
	return "No report is pending"
}

func reportsText(n int) string {
	if n == 1 {
		return "1 report"
	}
	return fmt.Sprintf("%d reports", n)
}
