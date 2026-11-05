package pageview

import (
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

// Problems page text (ADR-0015). Every function returns plain text: the
// view escapes it with glib.MarkupEscapeText, because titles and subtitles
// come from files and the journal.

// Fixed texts of the Problems page.
const (
	ProblemsSystemTitle       = "System"
	ProblemsSystemDescription = "Problems that the system checks (snosi-health) recorded"
	ProblemsAppsTitle         = "Apps"
	ProblemsAppsDescription   = "Crashes of your programs and problems that frostyard apps recorded in the last 90 days"
	ProblemsReportingTitle    = "Reporting"
	ProblemsLoading           = "Loading…"
	ProblemsNotAvailable      = "System checks are not available on this system"
	ProblemsUnreadable        = "System checks could not be read"
	ProblemsNoneFound         = "No problems found"
)

// ProblemsReporting is the only row of the reporting group in phase 1.
func ProblemsReporting() Row {
	return Row{
		Title:    "Problem reports stay on this computer",
		Subtitle: "ChairLift does not send anything.",
	}
}

// Problem returns the row text of 1 problem: the title in plain words, and
// a subtitle with when it happened and, for some kinds, what it concerns
// (the unit, feature, version, code, or the unknown kind's name).
func Problem(it problems.Item, now time.Time, loc *time.Location) Row {
	subtitle := ProblemWhen(it, now, loc)
	if context := problemContext(it); context != "" {
		subtitle += " · " + context
	}
	return Row{Title: problems.Title(it), Subtitle: subtitle}
}

// problemContext returns the short "what" of an item's subtitle.
func problemContext(it problems.Item) string {
	if !it.KnownKind {
		return "kind " + it.Kind
	}
	switch it.Kind {
	case problems.KindUnitFailed:
		if unit := it.ValidString("unit"); unit != "" {
			return unit
		}
		if len(it.Local.Units) > 0 {
			return it.Local.Units[0]
		}
	case problems.KindSysextUpdateFailed:
		if f, ok := it.Field("feature"); ok {
			return f.Display
		}
	case problems.KindUpdateStageFailed:
		if v := it.ValidString("target_version"); v != "" {
			return "version " + v
		}
	case problems.KindBootedOlderImage:
		if v := it.ValidString("newer_version"); v != "" {
			return "newer version " + v
		}
	case problems.KindAppProblem:
		if code := it.ValidString("code"); code != "" {
			return code
		}
	}
	return ""
}

// ProblemWhen returns when a problem happened: "today at 15:04",
// "yesterday at 15:04", "on Nov 5 at 15:04", or "on Nov 5, 2025 at 15:04"
// in another year, in loc; with "N times, last " before it when it
// happened more than once. A zero time is "time unknown".
func ProblemWhen(it problems.Item, now time.Time, loc *time.Location) string {
	when := whenText(it.LastSeen, now, loc)
	if it.Count > 1 {
		return fmt.Sprintf("%d times, last %s", it.Count, when)
	}
	return capitalize(when)
}

func whenText(t, now time.Time, loc *time.Location) string {
	if t.IsZero() {
		return "time unknown"
	}
	if loc == nil {
		loc = time.UTC
	}
	t, now = t.In(loc), now.In(loc)
	clock := t.Format("15:04")
	y, m, d := t.Date()
	ny, nm, nd := now.Date()
	if y == ny && m == nm && d == nd {
		return "today at " + clock
	}
	yesterday := now.AddDate(0, 0, -1)
	if yy, ym, yd := yesterday.Date(); y == yy && m == ym && d == yd {
		return "yesterday at " + clock
	}
	if y == ny {
		return "on " + t.Format("Jan 2") + " at " + clock
	}
	return "on " + t.Format("Jan 2, 2006") + " at " + clock
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// LastChecked returns "Last checked <when>" for problems.json's generated
// time, or "Last check time unknown".
func LastChecked(generated, now time.Time, loc *time.Location) string {
	if generated.IsZero() {
		return "Last check time unknown"
	}
	return "Last checked " + whenText(generated, now, loc)
}

// ProblemsStatus is the system or apps group's status: the group
// description, and the single row that replaces the item rows when there
// are none (nil when items are shown).
type ProblemsStatus struct {
	Description string
	Empty       *Row
}

// ProblemsSystemStatus returns the system group's status for each
// problems.json outcome: SourceMissing gives the row "System checks are
// not available on this system"; SourceUnreadable and SourceMalformed give
// "System checks could not be read"; SourceOK with no shown item gives "No
// problems found" with "Last checked <when>"; SourceOK with items gives no
// row and adds "Last checked <when>" to the description. hidden dismissed
// items are counted in the description.
func ProblemsSystemStatus(outcome problems.SourceOutcome, generated time.Time, shown, hidden int, now time.Time, loc *time.Location) ProblemsStatus {
	status := ProblemsStatus{Description: ProblemsSystemDescription}
	switch outcome {
	case problems.SourceMissing:
		status.Empty = &Row{Title: ProblemsNotAvailable}
		return status
	case problems.SourceUnreadable, problems.SourceMalformed:
		status.Empty = &Row{Title: ProblemsUnreadable, Subtitle: "See the log for details."}
		return status
	}
	checked := LastChecked(generated, now, loc)
	status.Description += ". " + checked + dismissedText(hidden)
	if shown == 0 {
		status.Empty = &Row{Title: ProblemsNoneFound, Subtitle: checked}
	}
	return status
}

// ProblemsAppsStatus returns the apps group's status: "No problems found"
// when no item is shown.
func ProblemsAppsStatus(shown, hidden int) ProblemsStatus {
	status := ProblemsStatus{Description: ProblemsAppsDescription + dismissedText(hidden)}
	if shown == 0 {
		status.Empty = &Row{Title: ProblemsNoneFound, Subtitle: "No crashes or app problems in the last 90 days"}
	}
	return status
}

func dismissedText(hidden int) string {
	switch hidden {
	case 0:
		return ""
	case 1:
		return ". 1 dismissed problem is hidden"
	default:
		return fmt.Sprintf(". %d dismissed problems are hidden", hidden)
	}
}

// ProblemDetails returns the rows inside a problem's expander: what it
// means (the hint), the recorded values, and the local details when there
// are any.
func ProblemDetails(it problems.Item) []Row {
	rows := []Row{{Title: "What it means", Subtitle: problems.Hint(it)}}
	var values []string
	for _, f := range it.Fields {
		if f.Valid {
			values = append(values, f.Name+": "+f.Display)
		} else {
			values = append(values, fmt.Sprintf("%s: %s (not valid: %s)", f.Name, f.Display, f.Reason))
		}
	}
	for _, name := range it.Missing {
		values = append(values, name+": (not recorded)")
	}
	if len(values) > 0 {
		rows = append(rows, Row{Title: "Recorded values", Subtitle: strings.Join(values, "\n")})
	}
	var local []string
	for _, f := range it.Local.Values {
		local = append(local, f.Name+": "+f.Display)
	}
	if len(it.Local.Units) > 0 {
		local = append(local, "Units: "+strings.Join(it.Local.Units, ", "))
	}
	if it.Local.ExePath != "" {
		local = append(local, "Program: "+it.Local.ExePath)
	}
	if it.Local.Comm != "" {
		local = append(local, "Process: "+it.Local.Comm)
	}
	if it.Local.Unit != "" {
		local = append(local, "Unit: "+it.Local.Unit)
	}
	if it.Local.Message != "" {
		local = append(local, "Message: "+it.Local.Message)
	}
	local = append(local, it.Local.Messages...)
	if len(local) > 0 {
		rows = append(rows, Row{Title: "On this computer", Subtitle: strings.Join(local, "\n")})
	}
	return rows
}
