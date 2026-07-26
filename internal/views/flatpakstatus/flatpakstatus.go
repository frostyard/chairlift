// Package flatpakstatus turns the outcome of the two Flatpak update queries —
// how many updates are known, and which of the user/system installations could
// not be checked — into the text of the Flatpak updates expander's subtitle and
// whether that expander should be expandable.
//
// It is deliberately free of any GTK binding import — in fact it imports
// nothing outside the standard library — so its logic can be unit-tested on a
// headless host. internal/views cannot host this logic: the Go GTK bindings it
// imports dlopen the GTK, Libadwaita, GLib and graphene shared libraries at
// package-init time, so a test binary for any package that imports them panics
// before a single test function runs. Decidable logic must therefore live in a
// binding-free package to be testable, which is the same reason
// internal/views/actionmsg, internal/views/trustmsg and internal/views/rowset
// are separate. See docs/agents/skills/gtk-headless-tests.md.
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
}

// Subtitle derives the expander state from the number of updates that were
// actually found and which of the two installation queries failed.
//
// Expandable is count > 0 in every case: a failed query never invents updates,
// so there is nothing to expand beyond what the count already reflects, while
// the rows that were found in a partially failed load are real and stay
// reachable. The claim that everything is up to date is made only when both
// queries succeeded and found nothing.
func Subtitle(count int, userFailed, systemFailed bool) Result {
	return Result{
		Subtitle:   subtitleText(count, userFailed, systemFailed),
		Expandable: count > 0,
	}
}

func subtitleText(count int, userFailed, systemFailed bool) string {
	if userFailed && systemFailed {
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
