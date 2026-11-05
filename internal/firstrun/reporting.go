package firstrun

import (
	"io/fs"
	"os"

	"github.com/frostyard/chairlift/internal/i18n"
	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
)

// ReportingAvailable reports whether the optional problem-reporting step
// (ADR-0015) takes part in the first run: the problems_reporting_group is
// enabled AND this host has the system problem file. On a host that is not
// a snosi host there is nothing to report, so the step does not show.
//
// stat is os.Stat in production; a test passes its own. Only the file's
// existence is read, never its content.
func ReportingAvailable(groupOn bool, stat func(string) (fs.FileInfo, error)) bool {
	if !groupOn {
		return false
	}
	if stat == nil {
		stat = os.Stat
	}
	info, err := stat(problems.ProblemsPath)
	return err == nil && info.Mode().IsRegular()
}

// ReportingStatusMsgid returns the msgid of the first run's reporting status
// text for a consent: off, on and asking before each report, or on and
// sending automatically. A stale consent reads as off, as its switch does.
func ReportingStatusMsgid(c reporting.Consent) string {
	if !c.Allows() {
		return i18n.MsgReportingOff
	}
	if c.Mode == reporting.ModeAuto {
		return i18n.MsgReportingOnAuto
	}
	return i18n.MsgReportingOn
}
