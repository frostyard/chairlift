// Package flatpakstatus turns the outcome of the two Flatpak update queries —
// how many updates are known, and which of the user/system installations could
// not be checked — into the Flatpak update source's status line and whether
// the load is authoritative (usable to replace the known rows).
//
// It imports nothing outside the standard library, so the decision is a
// plain, table-tested function, like internal/views/actionmsg and
// internal/views/trustmsg. See
// docs/design/engineering-lessons.md#no-tests-in-puregotk-packages.
//
// The package is pure and holds no state: it takes booleans rather than error
// values (which would drag in internal/flatpak) and returns a value, so it is
// safe to call from a worker goroutine or from inside an
// sgtk.RunOnMainThread closure.
package flatpakstatus

import "fmt"

// Result is the expander state derived from an update load. Both halves come
// from one call so the subtitle and the expansion decision cannot drift apart.
type Result struct {
	// Subtitle is the text to show under the expander's title.
	Subtitle string
	// Expandable reports whether the expander should allow expansion.
	Expandable bool
	// Authoritative reports whether this load learned anything about the
	// update inventory. It is true when at least one installation returned a
	// usable result: either its query succeeded, or it returned updates with
	// a RemoteError. It is false only when both queries failed and found no
	// updates; the caller then knows nothing and must keep the previous rows
	// and badge count rather than publishing an empty inventory over a known one.
	Authoritative bool
}

// Subtitle derives the expander state from the number of updates that were
// actually found and which of the two installation queries failed.
//
// Expandable is count > 0 in every case: a failed query never invents updates,
// so there is nothing to expand beyond what the count already reflects, while
// the rows that were found in a partially failed load are real and stay
// reachable. The claim that everything is up to date is made only when both
// queries succeeded and found nothing.
//
// Authoritative is true when at least one installation returned a usable
// result. That means its query succeeded or it returned one or more updates
// with an error. In the boolean API, it is false only when both queries failed
// and count is zero.
func Subtitle(count int, userFailed, systemFailed bool) Result {
	return Result{
		Subtitle:      subtitleText(count, userFailed, systemFailed),
		Expandable:    count > 0,
		Authoritative: !userFailed || !systemFailed || count > 0,
	}
}

func subtitleText(count int, userFailed, systemFailed bool) string {
	if userFailed && systemFailed {
		if count > 0 {
			return fmt.Sprintf("%s; the user and system installations could not be fully checked", availableText(count))
		}
		return "Could not check for updates"
	}

	if !userFailed && !systemFailed {
		if count == 0 {
			return "All applications are up to date"
		}
		return availableText(count)
	}

	failed, ok := "system", "user"
	if userFailed {
		failed, ok = "user", "system"
	}

	if count == 0 {
		return fmt.Sprintf(
			"No updates found in the %s installation; the %s installation could not be checked",
			ok, failed,
		)
	}
	return fmt.Sprintf("%s; the %s installation could not be checked", availableText(count), failed)
}

func availableText(count int) string {
	if count == 1 {
		return "1 update available"
	}
	return fmt.Sprintf("%d updates available", count)
}
