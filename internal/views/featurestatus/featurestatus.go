// Package featurestatus turns the outcome of an updex feature update check —
// the per-component [updex.CheckResult] slice for one feature, and the totals
// across all features — into the text the Features page shows: a feature row's
// subtitle plus whether that feature has an update, and the features group's
// description.
//
// A feature has an update when ANY of its components reports one — not the
// first component, not all of them. Status.HasUpdate is therefore an OR across
// every element of the results slice, and both halves of the answer come from
// the single Feature call so the subtitle and the update decision cannot drift
// apart (the same reason flatpakstatus.Subtitle returns a Result struct).
//
// A feature can instead WAIT for a newer OS image. When a component's check
// result carries NeedsImage ("needs image >= X"), its newest staged image is
// not linked and the next `features update` will not link it, because its
// UPDEX_MIN_IMAGE_VERSION is above the image that boots next (updex ADR-0016).
// That is not an error and it is not an update: nothing of that feature is
// linked until a newer OS image boots. Because updex links nothing of a feature
// when one of its transfers needs an image, ONE waiting component makes the
// whole feature wait.
//
// The waiting branch therefore has precedence over every update branch: a
// feature with a waiting component returns Status.WaitingForImage true and
// Status.HasUpdate false even when another component reports UpdateAvailable,
// and its subtitle names every waiting component and its image. A waiting
// feature is not counted as an update and is counted separately, in the group
// description's featuresWaiting argument.
//
// Below both of them sits a feature whose new version is linked and takes
// effect at the next boot: a component's check result carries PendingReboot
// when the systemd-sysext link names a version other than the merged one. That
// is neither an update nor a wait, and it is not nothing: the row says
// "changes apply after reboot" with the same words as the update action's own
// toast. Status.PendingReboot is true then, and the feature is counted as
// neither an update nor a wait.
//
// The update count in the group description is a count of FEATURES, not of
// components: a feature with three outdated components counts once. The
// sentence's first number is a feature count, so its second number must be one
// too or the sentence is incoherent. featuresWaiting is a feature count for the
// same reason.
//
// Summarize applies Feature to a whole check at once: it keeps the rows whose
// feature the page has, ignores a check entry for any other feature, and counts
// the resulting rows rather than the check entries. A name it does not answer
// for is deliberately absent from its result, so the caller can give that row
// its base subtitle back instead of leaving an earlier check's "update
// available" in place after an update changed the state.
//
// The package is separate from internal/views because internal/views can never
// host a _test.go: the Go GTK bindings it imports (puregotk) dlopen the GTK,
// Libadwaita, GLib and graphene shared libraries at package-init time, so a
// test binary for any package that imports them panics before a single test
// function runs. Decidable logic must live in a binding-free package to be
// testable at all, which is the same reason internal/views/actionmsg,
// internal/views/trustmsg, internal/views/rowset and
// internal/views/flatpakstatus are separate. See
// docs/agents/skills/gtk-headless-tests.md. Importing internal/updex for the
// CheckResult type is safe here because internal/updex is itself puregotk-free
// (`go list -deps ./internal/updex | grep -c puregotk` prints 0).
//
// The package is pure and holds no state, so it is safe to call from a worker
// goroutine or from inside an sgtk.RunOnMainThread closure.
package featurestatus

import (
	"fmt"
	"strings"

	"github.com/frostyard/chairlift/internal/updex"
)

// Status is the row state derived from one feature's check results. Both
// halves come from one call so the subtitle text and the update decision
// cannot drift apart.
type Status struct {
	// Subtitle is the text to show under the feature row's title.
	Subtitle string
	// HasUpdate reports whether any component of the feature has an update.
	// It is false when WaitingForImage is true: a feature that waits for a
	// newer OS image is not updatable now, so it is not an update.
	HasUpdate bool
	// WaitingForImage reports whether a component of the feature needs a newer
	// OS image. The callers use it to count the feature separately from the
	// updatable ones, never as an update.
	WaitingForImage bool
	// PendingReboot reports whether the feature has no update and no wait but a
	// component whose new version is linked and becomes active at the next
	// boot. The callers count such a feature as neither an update nor a wait.
	PendingReboot bool
}

// Feature derives the row state for the feature called name from the check
// results of all of its components.
//
// Its branches, in precedence order: a component needs a newer OS image (the
// feature waits), a component has an update, a component is linked and pending
// a reboot, and finally the agreed-version or up-to-date subtitle.
//
// The second return value is false when results is empty: the check reported
// nothing about this feature, so the caller must not derive a status from it
// and must not count the feature (Summarize leaves it out of Rows, and the
// Features page gives the row its base subtitle back). The returned Status is
// then meaningless and is not part of the caller contract.
func Feature(name string, results []updex.CheckResult) (Status, bool) {
	if len(results) == 0 {
		return Status{}, false
	}

	// The waiting branch comes first and wins over every update branch: one
	// component that needs a newer OS image keeps the whole feature unlinked.
	waits := make([]string, 0, len(results))
	for _, res := range results {
		if res.NeedsImage != "" {
			waits = append(waits, res.Component+": "+res.NeedsImage)
		}
	}
	if len(waits) > 0 {
		return Status{
			Subtitle: fmt.Sprintf("%s — waiting for a newer OS image (%s)",
				name, strings.Join(waits, ", ")),
			HasUpdate:       false,
			WaitingForImage: true,
		}, true
	}

	updates := make([]updex.CheckResult, 0, len(results))
	for _, res := range results {
		if res.UpdateAvailable {
			updates = append(updates, res)
		}
	}
	if len(updates) > 0 {
		return Status{
			Subtitle:  subtitleText(name, results, updates),
			HasUpdate: true,
		}, true
	}

	// A linked version that the next boot merges is not an update and not a
	// wait, so it is reported with the same words as the update action's
	// "Changes apply after reboot." toast.
	for _, res := range results {
		if res.PendingReboot {
			return Status{
				Subtitle:      fmt.Sprintf("%s — changes apply after reboot", name),
				PendingReboot: true,
			}, true
		}
	}

	return Status{Subtitle: subtitleText(name, results, updates)}, true
}

func subtitleText(name string, results, updates []updex.CheckResult) string {
	switch len(updates) {
	case 0:
		if version, ok := commonVersion(results); ok {
			return fmt.Sprintf("%s — v%s", name, version)
		}
		return fmt.Sprintf("%s — up to date", name)
	case 1:
		up := updates[0]
		if up.CurrentVersion == "" {
			return fmt.Sprintf("%s — update available for %s (→ v%s)", name, up.Component, up.NewestVersion)
		}
		return fmt.Sprintf("%s — update available for %s (v%s → v%s)",
			name, up.Component, up.CurrentVersion, up.NewestVersion)
	default:
		return fmt.Sprintf("%s — updates available for %d components", name, len(updates))
	}
}

// commonVersion reports the version every component agrees on, and whether
// there is one. It is not ok when any component's CurrentVersion is empty
// (which would render a bare `v`) or when the components disagree — presenting
// one component's version as the feature's version would be a lie in both
// cases.
func commonVersion(results []updex.CheckResult) (string, bool) {
	version := results[0].CurrentVersion
	if version == "" {
		return "", false
	}
	for _, res := range results[1:] {
		if res.CurrentVersion != version {
			return "", false
		}
	}
	return version, true
}

// Summary is the whole check turned into rows: the derived status of every
// feature the check answered for, plus the two feature counts the group
// description needs.
type Summary struct {
	// Rows holds one Status per feature of the check, keyed by feature name.
	// A feature with no result from the check is absent, and so is a check
	// entry for a feature that is not one of the rows' names.
	Rows map[string]Status
	// Updates is the number of features in Rows with HasUpdate, not the number
	// of updatable components.
	Updates int
	// Waiting is the number of features in Rows with WaitingForImage, again a
	// feature count. A feature that waits is never also counted in Updates,
	// and a feature that only pends a reboot is counted in neither.
	Waiting int
}

// Summarize derives the rows of names from the check results, and counts them.
//
// names are the features the page has rows for; a check entry for any other
// feature is ignored and is neither in Rows nor counted. A feature whose check
// reported no components is absent from Rows, so the caller can tell it apart
// from a feature whose check said something.
//
// The caller must give such a name its base subtitle back: a name that is not
// in Rows keeps whatever the row already shows otherwise, and an earlier
// check's "update available" would stay after the update that removed it.
//
// Both counts come from Rows, so a feature is counted once however many check
// entries name it (the last entry wins, as the row's subtitle does).
func Summarize(names []string, checks []updex.FeatureCheck) Summary {
	known := make(map[string]struct{}, len(names))
	for _, name := range names {
		known[name] = struct{}{}
	}

	summary := Summary{Rows: make(map[string]Status, len(names))}
	for _, check := range checks {
		if _, ok := known[check.Feature]; !ok {
			continue
		}
		status, ok := Feature(check.Feature, check.Results)
		if !ok {
			continue
		}
		summary.Rows[check.Feature] = status
	}

	for _, status := range summary.Rows {
		if status.HasUpdate {
			summary.Updates++
		}
		if status.WaitingForImage {
			summary.Waiting++
		}
	}

	return summary
}

// GroupDescription is the features group's description after a check that
// completed. featuresWithUpdates and featuresWaiting are counts of features,
// not of components, and they are disjoint: a waiting feature is never also
// counted as an update.
func GroupDescription(totalFeatures, featuresWithUpdates, featuresWaiting int) string {
	switch {
	case featuresWithUpdates == 0 && featuresWaiting == 0:
		return fmt.Sprintf("%s — all up to date", available(totalFeatures))
	case featuresWaiting == 0:
		return fmt.Sprintf("%s (%s)", available(totalFeatures), updateTail(featuresWithUpdates))
	case featuresWithUpdates == 0:
		return fmt.Sprintf("%s (%s)", available(totalFeatures), waitingTail(featuresWaiting))
	default:
		return fmt.Sprintf("%s (%s, %s)", available(totalFeatures),
			updateTail(featuresWithUpdates), waitingTail(featuresWaiting))
	}
}

// updateTail is the update fragment of the group description, singular for one
// feature. GroupDescription never calls it with 0, which has its own
// "all up to date" sentence.
func updateTail(featuresWithUpdates int) string {
	if featuresWithUpdates == 1 {
		return "1 update"
	}
	return fmt.Sprintf("%d updates", featuresWithUpdates)
}

// waitingTail is the waiting fragment of the group description. Its wording
// matches the update action's own phrase, so a row and a toast describe the
// same wait with the same words; only the number changes between one and many.
func waitingTail(featuresWaiting int) string {
	return fmt.Sprintf("%d waiting for a newer OS image", featuresWaiting)
}

// GroupDescriptionCheckFailed is the features group's description when the
// update check itself failed. It makes no claim about update state: a failed
// check neither found updates nor established that there are none.
func GroupDescriptionCheckFailed(totalFeatures int) string {
	return fmt.Sprintf("%s — update check failed", available(totalFeatures))
}

// available reproduces loadFeatures' own pre-check fragment verbatim, including
// its non-pluralized "features", so the description only ever changes in its
// update-status tail.
func available(totalFeatures int) string {
	return fmt.Sprintf("%d features available", totalFeatures)
}
