package actionmsg

import "fmt"

// ProblemDismissDecision is the single decision of the Problems page's
// Dismiss button (ADR-0009): whether the dismissal is written to the
// dismiss store, whether the row is hidden after the write succeeds, the
// toast, and the line to log.
type ProblemDismissDecision struct {
	// Persist is true when the page writes the dismissal to the dismiss
	// store. It is exactly !dryRun.
	Persist bool
	// Hide is true when the page removes the row after the write succeeds.
	// A failed write keeps the row and shows ProblemDismissFailed instead.
	// Under dry-run nothing is written, so nothing is hidden.
	Hide bool
	// Toast is the message to show; for a live dismissal, after the write
	// succeeds.
	Toast string
	// Log is the line to log, or "".
	Log string
}

// ProblemDismiss decides the Dismiss button of the problem id with the row
// title title. Dry run: no write, no hide, a preview toast and a
// "[DRY-RUN] would dismiss problem <id>" log line. Live: write, then hide
// the row and show "Dismissed: <title>".
func ProblemDismiss(dryRun bool, id, title string) ProblemDismissDecision {
	if dryRun {
		return ProblemDismissDecision{
			Toast: fmt.Sprintf("[DRY-RUN] Preview: dismiss %s — no changes made", title),
			Log:   fmt.Sprintf("[DRY-RUN] would dismiss problem %s", id),
		}
	}
	return ProblemDismissDecision{
		Persist: true,
		Hide:    true,
		Toast:   fmt.Sprintf("Dismissed: %s", title),
	}
}

// ProblemDismissFailed is the error toast of a dismissal whose write
// failed; the row stays.
func ProblemDismissFailed(title string, err error) string {
	return fmt.Sprintf("Could not dismiss %s: %v", title, err)
}

// ProblemDetailsCopied is the toast after "Copy details".
func ProblemDetailsCopied(title string) string {
	return fmt.Sprintf("Copied the details of %s", title)
}
