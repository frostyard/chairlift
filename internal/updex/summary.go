package updex

import "strings"

// UpdateClass is the outcome class of one feature in an update run. A feature
// has exactly one class, chosen by SummarizeUpdate's precedence.
type UpdateClass int

const (
	// ClassCurrent means no transfer changed anything.
	ClassCurrent UpdateClass = iota
	// ClassUpdated means a transfer downloaded or linked a new version.
	ClassUpdated
	// ClassHeld means a rollback record holds the feature at an older version.
	ClassHeld
	// ClassWaitingForImage means the next-boot image gate keeps the feature
	// unlinked until a newer OS image boots (updex ADR-0016).
	ClassWaitingForImage
	// ClassFailed means a transfer reported an error.
	ClassFailed
)

// String returns the class name used in logs.
func (c UpdateClass) String() string {
	switch c {
	case ClassCurrent:
		return "current"
	case ClassUpdated:
		return "updated"
	case ClassHeld:
		return "held"
	case ClassWaitingForImage:
		return "waiting-for-image"
	case ClassFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// FeatureOutcome is the classified outcome of one feature. Reason is set for
// ClassFailed, ClassWaitingForImage and ClassHeld, and empty otherwise.
type FeatureOutcome struct {
	Feature string
	Class   UpdateClass
	Reason  string
}

// UpdateSummary is the classified result of an update run, in the helper's
// feature order.
type UpdateSummary struct {
	Features []FeatureOutcome
	// RefreshSkipped says why the systemd-sysext refresh did not run. It is
	// host-wide: updex puts the same text on every feature result, so the
	// summary keeps the first non-empty one.
	RefreshSkipped string
}

// Of returns the outcomes of one class, in order.
func (s UpdateSummary) Of(class UpdateClass) []FeatureOutcome {
	var out []FeatureOutcome
	for _, f := range s.Features {
		if f.Class == class {
			out = append(out, f)
		}
	}
	return out
}

// notLinkedPrefix starts the NextActionMessage of a transfer that the image
// gate left unlinked.
const notLinkedPrefix = "not linked: "

// SummarizeUpdate classifies every feature of an update run. updex returns a
// nil error when the next-boot image gate or a rollback record holds a
// feature, so only the results say what happened. A feature's class is the
// first that applies of: Failed (a transfer has Error), WaitingForImage (a
// transfer has NeedsImage or NotLinked), Held (a transfer has Held), Updated
// (a transfer has Downloaded or Linked), Current.
func SummarizeUpdate(results []FeatureUpdate) UpdateSummary {
	summary := UpdateSummary{Features: make([]FeatureOutcome, 0, len(results))}
	for _, feature := range results {
		if summary.RefreshSkipped == "" {
			summary.RefreshSkipped = feature.RefreshSkipped
		}
		summary.Features = append(summary.Features, classifyFeature(feature))
	}
	return summary
}

func classifyFeature(feature FeatureUpdate) FeatureOutcome {
	outcome := FeatureOutcome{Feature: feature.Feature, Class: ClassCurrent}

	var failed, held, notLinked, changed bool
	var failReason, heldReason, notLinkedReason string
	var needsImage []string
	for _, r := range feature.Results {
		if r.Error != "" {
			if !failed {
				failReason = r.Error
			}
			failed = true
		}
		if r.NeedsImage != "" {
			needsImage = append(needsImage, r.Component+": "+r.NeedsImage)
		} else if r.NotLinked {
			if !notLinked {
				notLinkedReason = strings.TrimPrefix(r.NextActionMessage, notLinkedPrefix)
			}
			notLinked = true
		}
		if r.Held {
			if !held {
				heldReason = r.NextActionMessage
			}
			held = true
		}
		if r.Downloaded || r.Linked {
			changed = true
		}
	}

	switch {
	case failed:
		outcome.Class, outcome.Reason = ClassFailed, failReason
	case len(needsImage) > 0:
		outcome.Class, outcome.Reason = ClassWaitingForImage, strings.Join(needsImage, ", ")
	case notLinked:
		outcome.Class, outcome.Reason = ClassWaitingForImage, notLinkedReason
	case held:
		outcome.Class, outcome.Reason = ClassHeld, heldReason
	case changed:
		outcome.Class = ClassUpdated
	}
	return outcome
}
