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
// The update count in the group description is a count of FEATURES, not of
// components: a feature with three outdated components counts once. The
// sentence's first number is a feature count, so its second number must be one
// too or the sentence is incoherent.
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

	"github.com/frostyard/chairlift/internal/updex"
)

// Status is the row state derived from one feature's check results. Both
// halves come from one call so the subtitle text and the update decision
// cannot drift apart.
type Status struct {
	// Subtitle is the text to show under the feature row's title.
	Subtitle string
	// HasUpdate reports whether any component of the feature has an update.
	HasUpdate bool
}

// Feature derives the row state for the feature called name from the check
// results of all of its components.
//
// The second return value is false when results is empty: the check reported
// nothing about this feature, so the caller must leave the row's existing
// subtitle untouched and must not count the feature. The returned Status is
// then meaningless and is not part of the caller contract.
func Feature(name string, results []updex.CheckResult) (Status, bool) {
	if len(results) == 0 {
		return Status{}, false
	}

	updates := make([]updex.CheckResult, 0, len(results))
	for _, res := range results {
		if res.UpdateAvailable {
			updates = append(updates, res)
		}
	}

	return Status{
		Subtitle:  subtitleText(name, results, updates),
		HasUpdate: len(updates) > 0,
	}, true
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

// GroupDescription is the features group's description after a check that
// completed. featuresWithUpdates is a count of features, not of components.
func GroupDescription(totalFeatures, featuresWithUpdates int) string {
	switch featuresWithUpdates {
	case 0:
		return fmt.Sprintf("%s — all up to date", available(totalFeatures))
	case 1:
		return fmt.Sprintf("%s (1 update)", available(totalFeatures))
	default:
		return fmt.Sprintf("%s (%d updates)", available(totalFeatures), featuresWithUpdates)
	}
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
