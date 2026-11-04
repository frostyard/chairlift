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

// Completion is the single completion decision and outcome (ADR-0009):
// whether the markers are written, the log line, what the Done step shows,
// and the error when the completion could not be recorded. The Done step and
// --mark-done branch only on it.
type Completion struct {
	// WriteMarkers is true when MarkDone runs, false in dry run.
	WriteMarkers bool
	// Log is the line to log: the "[DRY-RUN] would write first-run
	// markers: ..." preview in dry run, the markers written, or the failure.
	Log string
	// DoneDescription is the msgid the Done step shows: the dry-run preview
	// in dry run, so the window never claims a change that did not happen.
	// After a failure it is the "starts again at your next login" text, but
	// DoneState replaces it when a marker exists after all (a partial
	// failure), because the gate then does not start the first run again.
	DoneDescription string
	// Err is set when the completion could not be recorded in full: the
	// config home is unknown, or a MarkDone step failed. MarkDone may have
	// written a marker before a later step failed, so Err alone does not
	// tell whether the next login runs the first run again; IsDone does.
	Err error
	// ShowWarning tells the Done step to show the warning icon instead of
	// the success icon. It is true exactly when Err is set.
	ShowWarning bool
}

// ErrorLog returns the Done step's error-log entry for a failed completion:
// "Setup failed: " and "Could not record that the first run is finished",
// translated, with the error text. It is empty when Err is nil.
func (c Completion) ErrorLog() []ErrorEntry {
	if c.Err == nil {
		return nil
	}
	return []ErrorEntry{{
		Title:   i18n.T(i18n.MsgSetupFailed) + i18n.T(i18n.MsgCompletionFailed),
		Message: c.Err.Error(),
	}}
}

// failed turns a decision into the failure outcome for err.
func (c Completion) failed(err error) Completion {
	c.Err = err
	c.ShowWarning = true
	c.Log = fmt.Sprintf("first-run: could not record completion: %v", err)
	c.DoneDescription = i18n.MsgCompletionFailedDesc
	return c
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
// and calls MarkDone only when the decision says so. A MarkDone error comes
// back both as the returned error and in the Completion (Err, ShowWarning,
// Log, DoneDescription). `chairlift-first-run --mark-done` calls it through
// Gate; the first-run window calls it through CompleteAt. The caller logs
// the returned Log line.
func Complete(dryRun bool, p Paths) (Completion, error) {
	decision := DecideCompletion(dryRun, p)
	if !decision.WriteMarkers {
		return decision, nil
	}
	if err := MarkDone(p); err != nil {
		decision = decision.failed(err)
		return decision, err
	}
	return decision, nil
}

// CompleteAt is the first-run window's completion: pathsErr is the error of
// ResolvePaths. When a live run (not dry run) has no config home, nothing
// can be recorded and the outcome is a failure; otherwise it is Complete.
// The window calls it after the Progress actions end (also when some failed)
// and before the Done step shows, and shows the outcome with DoneState.
func CompleteAt(dryRun bool, p Paths, pathsErr error) Completion {
	if pathsErr != nil && !dryRun {
		return DecideCompletion(dryRun, p).failed(fmt.Errorf("cannot find the config home: %w", pathsErr))
	}
	decision, _ := Complete(dryRun, p)
	return decision
}

// Done is what the Done step shows.
type Done struct {
	// Warning selects the warning icon instead of the success icon.
	Warning bool
	// Description is the msgid of the description.
	Description string
	// Entries is the error log; the log is hidden when it is empty.
	Entries []ErrorEntry
}

// DoneState composes the Done step from the failed actions' entries, the
// completion outcome, and done: the gate state after the completion
// (DoneAt), that is, whether the next login skips the first run. The action
// entries come first, then the completion entry; the warning shows when any
// entry exists. When the completion failed, the description is "The first
// run starts again at your next login" only when done is false; when a
// marker exists after all (a partial MarkDone failure), the gate skips the
// next run and the description is "Setup finished with errors".
func DoneState(actionEntries []ErrorEntry, c Completion, done bool) Done {
	entries := append(append([]ErrorEntry(nil), actionEntries...), c.ErrorLog()...)
	description := c.DoneDescription
	if c.Err != nil {
		description = i18n.MsgCompletionFailedDesc
		if done {
			description = i18n.MsgFinishedWithErrors
		}
	}
	return Done{
		Warning:     c.ShowWarning || len(entries) > 0,
		Description: description,
		Entries:     entries,
	}
}

// ProgressSteps are the parts of the Progress sequence. The window passes
// the production parts; tests inject their own.
type ProgressSteps struct {
	// Run runs one action (FlatpakRunner in production).
	Run Runner
	// Emit receives each action event (the window marshals it to the GTK
	// main thread).
	Emit func(Event)
	// Drain waits until no color-scheme write is pending
	// (SchemeWriter.Wait). It may be nil.
	Drain func()
	// Complete records the completion (CompleteAt in production).
	Complete func() Completion
	// IsDone reports the gate state after Complete (DoneAt in production).
	IsDone func() bool
}

// RunProgress is the Progress worker's whole sequence, in this order: run
// every action, wait for the color-scheme writer to drain, record the
// completion, then read the gate state and compose the Done step. Draining
// before Complete means the user's last style choice is written before the
// completion is recorded, the application hold is released, and Exit can
// end the process. Back is not possible from Progress, so no newer style
// choice can come after the drain. It runs off the GTK main thread.
func RunProgress(actions []Action, steps ProgressSteps) (Completion, Done) {
	report := Execute(actions, steps.Run, steps.Emit)
	if steps.Drain != nil {
		steps.Drain()
	}
	completion := steps.Complete()
	return completion, DoneState(ErrorLog(actions, report), completion, steps.IsDone())
}
