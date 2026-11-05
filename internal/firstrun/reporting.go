package firstrun

import (
	"io/fs"
	"os"

	"github.com/frostyard/chairlift/internal/problems"
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
