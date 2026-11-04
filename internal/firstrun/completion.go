package firstrun

import (
	"fmt"
	"strings"

	"github.com/frostyard/chairlift/internal/i18n"
)

// dryRun is the first run's own dry-run flag (ADR-0009). app.New() sets it
// with the other wrappers' flags; it gates the color-scheme write and the
// completion markers. Flatpak actions use internal/flatpak's flag.
var dryRun = false

// SetDryRun sets the dry-run mode of the first run.
func SetDryRun(mode bool) {
	dryRun = mode
}

// IsDryRun reports whether the first run is in dry-run mode.
func IsDryRun() bool {
	return dryRun
}

// Completion is the single completion decision (ADR-0009): whether the
// markers are written, the log line, and the msgid of the Done step's
// description. The Done step and --mark-done branch only on it.
type Completion struct {
	// WriteMarkers is true when MarkDone runs, false in dry run.
	WriteMarkers bool
	// Log is the line to log: the "[DRY-RUN] would write first-run
	// markers: ..." preview in dry run, else the markers written.
	Log string
	// DoneDescription is the msgid the Done step shows: the dry-run preview
	// in dry run, so the window never claims a change that did not happen.
	DoneDescription string
}

// DecideCompletion builds the completion decision for the paths.
func DecideCompletion(dryRun bool, p Paths) Completion {
	markers := strings.Join(p.MarkerPaths(), ", ")
	if dryRun {
		return Completion{
			WriteMarkers:    false,
			Log:             "[DRY-RUN] would write first-run markers: " + markers,
			DoneDescription: i18n.MsgDryRunFinished,
		}
	}
	return Completion{
		WriteMarkers:    true,
		Log:             "first-run: markers written: " + markers,
		DoneDescription: i18n.MsgFinishedDesc,
	}
}

// Complete records a finished first run: it decides with DecideCompletion
// and calls MarkDone only when the decision says so. The first-run window
// calls it after the Progress actions end (also when some failed) and before
// the Done step shows; `chairlift-first-run --mark-done` calls it through
// Gate. The caller logs the returned Log line.
func Complete(dryRun bool, p Paths) (Completion, error) {
	decision := DecideCompletion(dryRun, p)
	if !decision.WriteMarkers {
		return decision, nil
	}
	if err := MarkDone(p); err != nil {
		decision.Log = fmt.Sprintf("first-run: could not record completion: %v", err)
		return decision, err
	}
	return decision, nil
}
