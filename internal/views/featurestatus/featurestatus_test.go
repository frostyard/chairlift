package featurestatus

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/frostyard/chairlift/internal/updex"
)

// featureName is the single feature name used by every case, so the
// distinctness assertion below cannot be satisfied merely by the name
// differing between cases.
const featureName = "f"

// branch identifies which of the seven subtitle branches a case exercises. The
// distinctness assertion compares one representative case per branch.
type branch string

const (
	branchNone         branch = ""
	branchOneVersioned branch = "one update, known current version"
	branchOneUnknown   branch = "one update, empty current version"
	branchMany         branch = "several updates"
	branchCommon       branch = "no updates, agreed version"
	branchMixed        branch = "no updates, no agreed version"
	branchWaiting      branch = "at least one component needs a newer OS image"
	branchPending      branch = "no update, no wait, a component pends a reboot"
)

// needsImageNew and needsImageOld are updex's own NeedsImage phrases: the
// component's staged image requires an IMAGE_VERSION at or above that value.
const (
	needsImageNew = "needs image >= 20270101000000"
	needsImageOld = "needs image >= 20261201000000"
)

type caseSpec struct {
	name    string
	results []updex.CheckResult
	want    string
	wantOK  bool
	branch  branch
	// wantWaiting is the expected Status.WaitingForImage. Only the waiting
	// cases set it; every other case leaves it false, so the table asserts that
	// no ordinary outcome is ever reported as waiting.
	wantWaiting bool
	// wantPending is the expected Status.PendingReboot. Only the pending-reboot
	// cases set it, so the table asserts that no other outcome is ever reported
	// as pending a reboot.
	wantPending bool
}

func res(component, current, newest string, update bool) updex.CheckResult {
	return updex.CheckResult{
		Component:       component,
		CurrentVersion:  current,
		NewestVersion:   newest,
		UpdateAvailable: update,
	}
}

// resWaiting builds a check result whose component needs a newer OS image. The
// update flag is independent of NeedsImage on purpose: updex can report both at
// once, and the feature still waits.
func resWaiting(component, current, newest string, update bool, needsImage string) updex.CheckResult {
	result := res(component, current, newest, update)
	result.NeedsImage = needsImage
	return result
}

// resPending builds a check result whose new version is linked and becomes
// active at the next boot. The update flag is independent of PendingReboot on
// purpose: updex can report both at once, and the update still wins.
func resPending(component, current, newest string, update bool) updex.CheckResult {
	result := res(component, current, newest, update)
	result.RunningVersion = "1.0.0"
	result.LinkedVersion = "2.0.0"
	result.PendingReboot = true
	return result
}

// tableCases is one entry per distinguishable input shape the Features page's
// update check can hand to Feature. The waiting cases come last so the older
// cases keep their positions.
func tableCases() []caseSpec {
	cases := []caseSpec{
		{
			name:    "zero components",
			results: nil,
			wantOK:  false,
			branch:  branchNone,
		},
		{
			name:    "zero components, empty slice",
			results: []updex.CheckResult{},
			wantOK:  false,
			branch:  branchNone,
		},
		{
			name: "update in the first component only",
			results: []updex.CheckResult{
				res("comp", "1.0", "2.0", true),
				res("other", "1.0", "1.0", false),
				res("third", "1.0", "1.0", false),
			},
			want:   "f — update available for comp (v1.0 → v2.0)",
			wantOK: true,
			branch: branchOneVersioned,
		},
		{
			name: "update in a later component only",
			results: []updex.CheckResult{
				res("other", "1.0", "1.0", false),
				res("third", "1.0", "1.0", false),
				res("comp", "1.0", "2.0", true),
			},
			want:   "f — update available for comp (v1.0 → v2.0)",
			wantOK: true,
			branch: branchOneVersioned,
		},
		{
			name: "update in a middle component only",
			results: []updex.CheckResult{
				res("other", "1.0", "1.0", false),
				res("comp", "1.0", "2.0", true),
				res("third", "1.0", "1.0", false),
			},
			want:   "f — update available for comp (v1.0 → v2.0)",
			wantOK: true,
			branch: branchOneVersioned,
		},
		{
			name: "updates in several components",
			results: []updex.CheckResult{
				res("comp", "1.0", "2.0", true),
				res("other", "1.0", "1.0", false),
				res("third", "3.0", "4.0", true),
			},
			want:   "f — updates available for 2 components",
			wantOK: true,
			branch: branchMany,
		},
		{
			name: "updates in every component",
			results: []updex.CheckResult{
				res("comp", "1.0", "2.0", true),
				res("other", "1.0", "2.0", true),
				res("third", "3.0", "4.0", true),
			},
			want:   "f — updates available for 3 components",
			wantOK: true,
			branch: branchMany,
		},
		{
			name: "no updates at all",
			results: []updex.CheckResult{
				res("comp", "1.0", "1.0", false),
				res("other", "1.0", "1.0", false),
			},
			want:   "f — v1.0",
			wantOK: true,
			branch: branchCommon,
		},
		{
			name: "a component whose CurrentVersion is empty",
			results: []updex.CheckResult{
				res("comp", "", "2.0", true),
			},
			want:   "f — update available for comp (→ v2.0)",
			wantOK: true,
			branch: branchOneUnknown,
		},
		{
			name: "no updates and a component whose CurrentVersion is empty",
			results: []updex.CheckResult{
				res("comp", "", "1.0", false),
				res("other", "1.0", "1.0", false),
			},
			want:   "f — up to date",
			wantOK: true,
			branch: branchMixed,
		},
		{
			name: "components reporting different CurrentVersion values with no update",
			results: []updex.CheckResult{
				res("comp", "1.0", "1.0", false),
				res("other", "2.5", "2.5", false),
			},
			want:   "f — up to date",
			wantOK: true,
			branch: branchMixed,
		},
	}

	return append(append(cases, waitingCases()...), pendingCases()...)
}

// waitingCases is one entry per shape of the waiting branch: a waiting
// component with and without an update flag of its own, a waiting component
// beside a normally updatable one, the only waiting component last or in the
// middle after an updatable one (detection must not look at index 0 only), and
// two waiting components in input order.
func waitingCases() []caseSpec {
	return []caseSpec{
		{
			name: "one component waits with an update available too",
			results: []updex.CheckResult{
				resWaiting("comp", "1.0", "2.0", true, needsImageNew),
			},
			want:        "f — waiting for a newer OS image (comp: needs image >= 20270101000000)",
			wantOK:      true,
			branch:      branchWaiting,
			wantWaiting: true,
		},
		{
			name: "one component waits with no update available",
			results: []updex.CheckResult{
				resWaiting("comp", "1.0", "1.0", false, needsImageNew),
			},
			want:        "f — waiting for a newer OS image (comp: needs image >= 20270101000000)",
			wantOK:      true,
			branch:      branchWaiting,
			wantWaiting: true,
		},
		{
			name: "one component waits while another has an update",
			results: []updex.CheckResult{
				resWaiting("comp", "1.0", "2.0", true, needsImageNew),
				res("other", "1.0", "2.0", true),
				res("third", "1.0", "1.0", false),
			},
			want:        "f — waiting for a newer OS image (comp: needs image >= 20270101000000)",
			wantOK:      true,
			branch:      branchWaiting,
			wantWaiting: true,
		},
		{
			name: "the only waiting component is last, after an update",
			results: []updex.CheckResult{
				res("other", "1.0", "2.0", true),
				res("third", "1.0", "1.0", false),
				resWaiting("comp", "1.0", "1.0", false, needsImageNew),
			},
			want:        "f — waiting for a newer OS image (comp: needs image >= 20270101000000)",
			wantOK:      true,
			branch:      branchWaiting,
			wantWaiting: true,
		},
		{
			name: "the only waiting component is in the middle, after an update",
			results: []updex.CheckResult{
				res("other", "1.0", "2.0", true),
				resWaiting("comp", "1.0", "2.0", true, needsImageNew),
				res("third", "1.0", "1.0", false),
			},
			want:        "f — waiting for a newer OS image (comp: needs image >= 20270101000000)",
			wantOK:      true,
			branch:      branchWaiting,
			wantWaiting: true,
		},
		{
			name: "two components wait, listed in input order",
			results: []updex.CheckResult{
				resWaiting("comp", "1.0", "2.0", true, needsImageNew),
				res("other", "1.0", "1.0", false),
				resWaiting("third", "3.0", "4.0", true, needsImageOld),
			},
			want: "f — waiting for a newer OS image (comp: needs image >= 20270101000000, " +
				"third: needs image >= 20261201000000)",
			wantOK:      true,
			branch:      branchWaiting,
			wantWaiting: true,
		},
	}
}

// pendingCases is one entry per shape of the pending-reboot branch: one
// pending component, several, a pending component beside an update (the update
// wins), a pending component beside a waiting one (the wait wins), and the only
// pending component in the middle or last after ordinary components (detection
// must not look at index 0 only).
func pendingCases() []caseSpec {
	return []caseSpec{
		{
			name: "one component pends a reboot",
			results: []updex.CheckResult{
				resPending("comp", "1.0", "2.0", false),
			},
			want:        "f — changes apply after reboot",
			wantOK:      true,
			branch:      branchPending,
			wantPending: true,
		},
		{
			name: "several components pend a reboot",
			results: []updex.CheckResult{
				resPending("comp", "1.0", "2.0", false),
				res("other", "1.0", "1.0", false),
				resPending("third", "3.0", "4.0", false),
			},
			want:        "f — changes apply after reboot",
			wantOK:      true,
			branch:      branchPending,
			wantPending: true,
		},
		{
			name: "a pending component beside an update still reads as the update",
			results: []updex.CheckResult{
				resPending("comp", "1.0", "2.0", true),
				res("other", "1.0", "1.0", false),
			},
			want:   "f — update available for comp (v1.0 → v2.0)",
			wantOK: true,
			branch: branchOneVersioned,
		},
		{
			name: "a pending component beside a waiting one reads as the wait",
			results: []updex.CheckResult{
				resPending("comp", "1.0", "2.0", false),
				resWaiting("other", "1.0", "1.0", false, needsImageNew),
			},
			want:        "f — waiting for a newer OS image (other: needs image >= 20270101000000)",
			wantOK:      true,
			branch:      branchWaiting,
			wantWaiting: true,
		},
		{
			name: "the only pending component is in the middle",
			results: []updex.CheckResult{
				res("other", "1.0", "1.0", false),
				resPending("comp", "1.0", "2.0", false),
				res("third", "1.0", "1.0", false),
			},
			want:        "f — changes apply after reboot",
			wantOK:      true,
			branch:      branchPending,
			wantPending: true,
		},
		{
			name: "the only pending component is last",
			results: []updex.CheckResult{
				res("other", "1.0", "1.0", false),
				res("third", "2.0", "2.0", false),
				resPending("comp", "1.0", "2.0", false),
			},
			want:        "f — changes apply after reboot",
			wantOK:      true,
			branch:      branchPending,
			wantPending: true,
		},
	}
}

func TestFeatureTable(t *testing.T) {
	for _, tc := range tableCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Feature(featureName, tc.results)

			if ok != tc.wantOK {
				t.Fatalf("Feature(%q, %#v) ok = %t, want %t", featureName, tc.results, ok, tc.wantOK)
			}
			if !ok {
				return
			}

			if got.Subtitle != tc.want {
				t.Errorf("Feature(%q, %#v).Subtitle = %q, want %q",
					featureName, tc.results, got.Subtitle, tc.want)
			}

			if got.WaitingForImage != tc.wantWaiting {
				t.Errorf("Feature(%q, %#v).WaitingForImage = %t, want %t",
					featureName, tc.results, got.WaitingForImage, tc.wantWaiting)
			}

			if got.PendingReboot != tc.wantPending {
				t.Errorf("Feature(%q, %#v).PendingReboot = %t, want %t",
					featureName, tc.results, got.PendingReboot, tc.wantPending)
			}

			// HasUpdate is an OR across EVERY element, not a look at index 0,
			// minus the waiting branch: one component that needs a newer OS image
			// keeps the whole feature unlinked, so it has no update.
			wantUpdate := false
			for _, r := range tc.results {
				if r.NeedsImage != "" {
					wantUpdate = false
					break
				}
				if r.UpdateAvailable {
					wantUpdate = true
				}
			}
			if got.HasUpdate != wantUpdate {
				t.Errorf("Feature(%q, %#v).HasUpdate = %t, want %t",
					featureName, tc.results, got.HasUpdate, wantUpdate)
			}
		})
	}
}

func TestFeatureHasUpdateWhenAnyComponentReportsOne(t *testing.T) {
	for _, tc := range tableCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Feature(featureName, tc.results)
			if !ok {
				return
			}

			// The update rule is the OR over every element of the slice, and the
			// waiting branch overrides it: a component that needs a newer OS image
			// keeps the whole feature unlinked, so the feature has no update.
			any, waits := false, false
			for _, r := range tc.results {
				if r.UpdateAvailable {
					any = true
				}
				if r.NeedsImage != "" {
					waits = true
				}
			}
			want := any && !waits
			if got.HasUpdate != want {
				t.Errorf("HasUpdate = %t, want %t (any component updates: %t, any waits for an image: %t)",
					got.HasUpdate, want, any, waits)
			}
		})
	}
}

// TestFeatureWaitingForImageTable asserts the whole waiting Status: the
// subtitle, HasUpdate false, and WaitingForImage true. A waiting feature is not
// an update even when a component's own UpdateAvailable is true.
func TestFeatureWaitingForImageTable(t *testing.T) {
	for _, tc := range waitingCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Feature(featureName, tc.results)
			if !ok {
				t.Fatalf("Feature(%q, %#v) ok = false, want true", featureName, tc.results)
			}
			if got.Subtitle != tc.want {
				t.Errorf("Feature(%q, %#v).Subtitle = %q, want %q",
					featureName, tc.results, got.Subtitle, tc.want)
			}
			if got.HasUpdate {
				t.Errorf("Feature(%q, %#v).HasUpdate = true, want false: a waiting feature is not an update",
					featureName, tc.results)
			}
			if !got.WaitingForImage {
				t.Errorf("Feature(%q, %#v).WaitingForImage = false, want true",
					featureName, tc.results)
			}
			if !strings.Contains(got.Subtitle, "waiting for a newer OS image") {
				t.Errorf("subtitle %q must say the feature waits for a newer OS image", got.Subtitle)
			}
		})
	}
}

// TestFeaturePendingRebootIsNeitherAnUpdateNorAWait asserts the whole pending
// Status, and that its words are the update toast's own words.
func TestFeaturePendingRebootIsNeitherAnUpdateNorAWait(t *testing.T) {
	results := []updex.CheckResult{
		res("other", "1.0", "1.0", false),
		resPending("comp", "1.0", "2.0", false),
	}

	got, ok := Feature(featureName, results)
	if !ok {
		t.Fatalf("Feature(%q, %#v) ok = false, want true", featureName, results)
	}
	if !got.PendingReboot {
		t.Errorf("Feature(%q, %#v).PendingReboot = false, want true", featureName, results)
	}
	if got.HasUpdate {
		t.Errorf("Feature(%q, %#v).HasUpdate = true, want false: a pending reboot is not an update",
			featureName, results)
	}
	if got.WaitingForImage {
		t.Errorf("Feature(%q, %#v).WaitingForImage = true, want false: a pending reboot is not a wait",
			featureName, results)
	}
	const want = "f — changes apply after reboot"
	if got.Subtitle != want {
		t.Errorf("Feature(%q, %#v).Subtitle = %q, want %q", featureName, results, got.Subtitle, want)
	}
}

func TestFeatureWithZeroComponentsIsSkipped(t *testing.T) {
	for _, results := range [][]updex.CheckResult{nil, {}} {
		// The caller contract is "leave the row alone"; the Status is not
		// relied upon, so only ok is asserted.
		if _, ok := Feature(featureName, results); ok {
			t.Errorf("Feature(%q, %#v) ok = true, want false so the caller leaves the existing subtitle untouched",
				featureName, results)
		}
	}
}

func TestFeatureNeverRendersABareV(t *testing.T) {
	bareV := regexp.MustCompile(`v(\s|$|—|→)`)

	for _, tc := range tableCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Feature(featureName, tc.results)
			if !ok {
				return
			}
			if loc := bareV.FindString(got.Subtitle); loc != "" {
				t.Errorf("Feature(%q, %#v).Subtitle = %q renders a bare %q",
					featureName, tc.results, got.Subtitle, loc)
			}
		})
	}
}

func TestFeatureEmptyCurrentVersionOmitsTheVersionArrowSource(t *testing.T) {
	results := []updex.CheckResult{res("comp", "", "2.0", true)}

	got, ok := Feature(featureName, results)
	if !ok {
		t.Fatalf("Feature(%q, %#v) ok = false, want true", featureName, results)
	}

	const want = "f — update available for comp (→ v2.0)"
	if got.Subtitle != want {
		t.Errorf("Feature(%q, %#v).Subtitle = %q, want %q", featureName, results, got.Subtitle, want)
	}
	if strings.Contains(got.Subtitle, "v ") {
		t.Errorf("subtitle %q contains a bare %q", got.Subtitle, "v ")
	}
	if strings.HasSuffix(got.Subtitle, "v") {
		t.Errorf("subtitle %q ends in a bare %q", got.Subtitle, "v")
	}
}

func TestSubtitleBranchesArePairwiseDistinct(t *testing.T) {
	seen := make(map[branch]string)
	for _, tc := range tableCases() {
		if tc.branch == branchNone {
			continue
		}
		if _, ok := seen[tc.branch]; ok {
			continue
		}
		got, ok := Feature(featureName, tc.results)
		if !ok {
			t.Fatalf("case %q: Feature ok = false, want true", tc.name)
		}
		seen[tc.branch] = got.Subtitle
	}

	if len(seen) != 7 {
		t.Fatalf("covered %d subtitle branches, want all 7", len(seen))
	}

	for a, subA := range seen {
		for b, subB := range seen {
			if a != b && subA == subB {
				t.Errorf("branch %q and branch %q both produce %q; every branch must be distinguishable",
					a, b, subA)
			}
		}
	}
}

func TestSubtitleSingularOneUpdate(t *testing.T) {
	results := []updex.CheckResult{
		res("comp", "1.0", "2.0", true),
		res("other", "1.0", "1.0", false),
	}

	got, _ := Feature(featureName, results)
	const want = "f — update available for comp (v1.0 → v2.0)"
	if got.Subtitle != want {
		t.Errorf("Subtitle = %q, want %q", got.Subtitle, want)
	}
	if strings.Contains(got.Subtitle, "updates available") {
		t.Errorf("Subtitle = %q, want the singular %q", got.Subtitle, "update available")
	}
}

func TestSubtitlePluralMultipleUpdates(t *testing.T) {
	results := []updex.CheckResult{
		res("comp", "1.0", "2.0", true),
		res("other", "1.0", "2.0", true),
	}

	got, _ := Feature(featureName, results)
	const want = "f — updates available for 2 components"
	if got.Subtitle != want {
		t.Errorf("Subtitle = %q, want %q", got.Subtitle, want)
	}
	if !strings.Contains(got.Subtitle, "updates available") {
		t.Errorf("Subtitle = %q, want the plural %q", got.Subtitle, "updates available")
	}
}

// preCheckDescription is the description loadFeatures sets before the update
// check runs. Every completed-check description must differ from it.
const preCheckDescription = "9 features available"

func TestGroupDescriptionSingularOneUpdate(t *testing.T) {
	got := GroupDescription(9, 1, 0)
	const want = "9 features available (1 update)"
	if got != want {
		t.Errorf("GroupDescription(9, 1, 0) = %q, want %q", got, want)
	}
	if strings.Contains(got, "1 updates") {
		t.Errorf("GroupDescription(9, 1, 0) = %q, must not contain %q", got, "1 updates")
	}
}

func TestGroupDescriptionPluralManyUpdates(t *testing.T) {
	got := GroupDescription(9, 3, 0)
	const want = "9 features available (3 updates)"
	if got != want {
		t.Errorf("GroupDescription(9, 3, 0) = %q, want %q", got, want)
	}
}

func TestGroupDescriptionZeroUpdates(t *testing.T) {
	got := GroupDescription(9, 0, 0)
	const want = "9 features available — all up to date"
	if got != want {
		t.Errorf("GroupDescription(9, 0, 0) = %q, want %q", got, want)
	}
	if got == preCheckDescription {
		t.Errorf("GroupDescription(9, 0, 0) = %q, want it to differ from the pre-check string %q",
			got, preCheckDescription)
	}
	if strings.Contains(got, "(0 updates)") {
		t.Errorf("GroupDescription(9, 0, 0) = %q, must not contain %q", got, "(0 updates)")
	}
}

// TestGroupDescriptionWaitingOnly says the group waits without claiming any
// update: the same sentence for one feature and for many, only the number
// changes.
func TestGroupDescriptionWaitingOnly(t *testing.T) {
	cases := []struct {
		name     string
		waiting  int
		want     string
		contains string
	}{
		{
			name:     "one feature waits",
			waiting:  1,
			want:     "9 features available (1 waiting for a newer OS image)",
			contains: "(1 waiting for a newer OS image)",
		},
		{
			name:     "several features wait",
			waiting:  2,
			want:     "9 features available (2 waiting for a newer OS image)",
			contains: "(2 waiting for a newer OS image)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GroupDescription(9, 0, tc.waiting)
			if got != tc.want {
				t.Errorf("GroupDescription(9, 0, %d) = %q, want %q", tc.waiting, got, tc.want)
			}
			if !strings.Contains(got, tc.contains) {
				t.Errorf("GroupDescription(9, 0, %d) = %q, want it to contain %q",
					tc.waiting, got, tc.contains)
			}
			if strings.Contains(got, "update") {
				t.Errorf("GroupDescription(9, 0, %d) = %q, must not claim an update", tc.waiting, got)
			}
		})
	}
}

// TestGroupDescriptionUpdatesAndWaiting reports both counts, with the update
// fragment keeping its singular and plural forms and the waiting fragment
// keeping one wording.
func TestGroupDescriptionUpdatesAndWaiting(t *testing.T) {
	cases := []struct {
		name    string
		updates int
		waiting int
		want    string
	}{
		{
			name:    "one update and two waiting",
			updates: 1,
			waiting: 2,
			want:    "9 features available (1 update, 2 waiting for a newer OS image)",
		},
		{
			name:    "several updates and one waiting",
			updates: 3,
			waiting: 1,
			want:    "9 features available (3 updates, 1 waiting for a newer OS image)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GroupDescription(9, tc.updates, tc.waiting)
			if got != tc.want {
				t.Errorf("GroupDescription(9, %d, %d) = %q, want %q",
					tc.updates, tc.waiting, got, tc.want)
			}
		})
	}
}
func TestGroupDescriptionCheckFailedSaysSo(t *testing.T) {
	got := GroupDescriptionCheckFailed(9)

	if !strings.Contains(got, "update check failed") {
		t.Errorf("GroupDescriptionCheckFailed(9) = %q, want it to contain %q", got, "update check failed")
	}
	for _, forbidden := range []string{"(0 updates)", "all up to date"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("GroupDescriptionCheckFailed(9) = %q, must not contain %q", got, forbidden)
		}
	}
	if got == preCheckDescription {
		t.Errorf("GroupDescriptionCheckFailed(9) = %q, want it to differ from the pre-check string %q",
			got, preCheckDescription)
	}
}

func TestGroupDescriptionsAreDistinct(t *testing.T) {
	descriptions := map[string]string{
		"failed":           GroupDescriptionCheckFailed(9),
		"zero updates":     GroupDescription(9, 0, 0),
		"one update":       GroupDescription(9, 1, 0),
		"many updates":     GroupDescription(9, 3, 0),
		"one waiting":      GroupDescription(9, 0, 1),
		"many waiting":     GroupDescription(9, 0, 3),
		"updates or waits": GroupDescription(9, 1, 2),
		"pre-check":        preCheckDescription,
	}

	seen := make(map[string]string)
	for name, desc := range descriptions {
		if other, ok := seen[desc]; ok {
			t.Errorf("case %q and case %q both produce %q", name, other, desc)
			continue
		}
		seen[desc] = name
	}

	for name, desc := range descriptions {
		if name == "pre-check" {
			continue
		}
		if !strings.HasPrefix(desc, preCheckDescription) {
			t.Errorf("%s description = %q, want it to keep the %q fragment verbatim",
				name, desc, preCheckDescription)
		}
	}
}

func TestGroupDescriptionCountsFeaturesNotComponents(t *testing.T) {
	// One feature with three outdated components counts once.
	results := []updex.CheckResult{
		res("comp", "1.0", "2.0", true),
		res("other", "1.0", "2.0", true),
		res("third", "1.0", "2.0", true),
	}

	status, ok := Feature(featureName, results)
	if !ok || !status.HasUpdate {
		t.Fatalf("Feature(%q, %#v) = (%+v, %t), want a single feature with an update",
			featureName, results, status, ok)
	}

	featuresWithUpdates := 0
	if status.HasUpdate {
		featuresWithUpdates++
	}

	got := GroupDescription(9, featuresWithUpdates, 0)
	want := fmt.Sprintf("%s (1 update)", preCheckDescription)
	if got != want {
		t.Errorf("GroupDescription(9, %d, 0) = %q, want %q", featuresWithUpdates, got, want)
	}
}

// TestGroupDescriptionCountsWaitingFeaturesSeparately follows the caller's two
// counters: one waiting feature plus one normally updatable feature gives one
// update and one waiting feature, never two updates.
func TestGroupDescriptionCountsWaitingFeaturesSeparately(t *testing.T) {
	waiting, ok := Feature("a", []updex.CheckResult{
		resWaiting("comp", "1.0", "2.0", true, needsImageNew),
		res("other", "1.0", "2.0", true),
	})
	if !ok {
		t.Fatal("Feature of the waiting feature ok = false, want true")
	}
	updating, ok := Feature("b", []updex.CheckResult{res("comp", "1.0", "2.0", true)})
	if !ok {
		t.Fatal("Feature of the updatable feature ok = false, want true")
	}

	featuresWithUpdates, featuresWaiting := 0, 0
	for _, status := range []Status{waiting, updating} {
		if status.HasUpdate {
			featuresWithUpdates++
		}
		if status.WaitingForImage {
			featuresWaiting++
		}
	}

	got := GroupDescription(9, featuresWithUpdates, featuresWaiting)
	want := fmt.Sprintf("%s (1 update, 1 waiting for a newer OS image)", preCheckDescription)
	if got != want {
		t.Errorf("GroupDescription(9, %d, %d) = %q, want %q",
			featuresWithUpdates, featuresWaiting, got, want)
	}
}

func TestUpdateTail(t *testing.T) {
	for n, want := range map[int]string{1: "1 update", 2: "2 updates", 7: "7 updates"} {
		if got := updateTail(n); got != want {
			t.Errorf("updateTail(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestWaitingTail(t *testing.T) {
	for n, want := range map[int]string{
		1: "1 waiting for a newer OS image",
		2: "2 waiting for a newer OS image",
	} {
		if got := waitingTail(n); got != want {
			t.Errorf("waitingTail(%d) = %q, want %q", n, got, want)
		}
	}
}

// check builds one check entry: a feature name and its components' results.
func check(name string, results ...updex.CheckResult) updex.FeatureCheck {
	return updex.FeatureCheck{Feature: name, Results: results}
}

// summarizeCases is one entry per shape Summarize must decide: which names it
// answers for, which check entries it ignores, and how it counts. The entries
// that must be counted are placed first, in the middle and last, and one case
// has the only countable entries after an unknown and an empty one, so a
// first-element-only implementation fails.
func summarizeCases() []struct {
	name        string
	names       []string
	checks      []updex.FeatureCheck
	wantUpdates int
	wantWaiting int
	// wantRows maps every expected row name to whether that row must report a
	// pending reboot. len(wantRows) is the expected number of rows.
	wantRows map[string]bool
} {
	return []struct {
		name        string
		names       []string
		checks      []updex.FeatureCheck
		wantUpdates int
		wantWaiting int
		wantRows    map[string]bool
	}{
		{
			name:  "a check entry for a name the page has no row for is ignored",
			names: []string{"a"},
			checks: []updex.FeatureCheck{
				check("a", res("comp", "1.0", "2.0", true)),
				check("b", res("comp", "1.0", "2.0", true)),
			},
			wantUpdates: 1,
			wantRows:    map[string]bool{"a": false},
		},
		{
			name:  "a feature whose check reported no components is not a row",
			names: []string{"a", "b"},
			checks: []updex.FeatureCheck{
				check("a", res("comp", "1.0", "2.0", true)),
				check("b"),
			},
			wantUpdates: 1,
			wantRows:    map[string]bool{"a": false},
		},
		{
			name:  "a feature with no check entry at all is not a row",
			names: []string{"a", "b", "c"},
			checks: []updex.FeatureCheck{
				check("a", res("comp", "1.0", "1.0", false)),
			},
			wantRows: map[string]bool{"a": false},
		},
		{
			name:  "updates, waits and pending reboots are counted apart",
			names: []string{"a", "b", "c"},
			checks: []updex.FeatureCheck{
				check("a", res("comp", "1.0", "2.0", true)),
				check("b", resWaiting("comp", "1.0", "2.0", true, needsImageNew)),
				check("c", resPending("comp", "1.0", "2.0", false)),
			},
			wantUpdates: 1,
			wantWaiting: 1,
			wantRows:    map[string]bool{"a": false, "b": false, "c": true},
		},
		{
			name:  "a duplicate entry for one feature is counted once, last entry wins",
			names: []string{"a"},
			checks: []updex.FeatureCheck{
				check("a", res("comp", "1.0", "2.0", true)),
				check("a", res("comp", "2.0", "2.0", false)),
			},
			wantRows: map[string]bool{"a": false},
		},
		{
			name:  "a duplicate entry for one waiting feature is counted once",
			names: []string{"a"},
			checks: []updex.FeatureCheck{
				check("a", resWaiting("comp", "1.0", "2.0", true, needsImageNew)),
				check("a", resWaiting("comp", "1.0", "2.0", true, needsImageNew)),
			},
			wantWaiting: 1,
			wantRows:    map[string]bool{"a": false},
		},
		{
			name:  "the countable entries are not the first ones",
			names: []string{"a", "b", "c", "d"},
			checks: []updex.FeatureCheck{
				check("zzz", res("comp", "1.0", "2.0", true)),
				check("a"),
				check("b", res("comp", "1.0", "1.0", false)),
				check("c", res("comp", "1.0", "2.0", true)),
				check("d", resWaiting("comp", "1.0", "2.0", true, needsImageNew)),
			},
			wantUpdates: 1,
			wantWaiting: 1,
			wantRows:    map[string]bool{"b": false, "c": false, "d": false},
		},
		{
			name:     "no check entries at all",
			names:    []string{"a"},
			checks:   nil,
			wantRows: map[string]bool{},
		},
	}
}

func TestSummarize(t *testing.T) {
	for _, tc := range summarizeCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := Summarize(tc.names, tc.checks)

			if len(got.Rows) != len(tc.wantRows) {
				t.Fatalf("Summarize rows = %v, want %d rows (%v)", got.Rows, len(tc.wantRows), tc.wantRows)
			}
			for name, wantPending := range tc.wantRows {
				status, ok := got.Rows[name]
				if !ok {
					t.Errorf("Summarize rows have no %q, want a row for every feature the check answered for", name)
					continue
				}
				if status.PendingReboot != wantPending {
					t.Errorf("row %q PendingReboot = %t, want %t", name, status.PendingReboot, wantPending)
				}
				if status.Subtitle == "" {
					t.Errorf("row %q has an empty subtitle", name)
				}
			}
			for _, name := range tc.names {
				if _, want := tc.wantRows[name]; want {
					continue
				}
				if _, ok := got.Rows[name]; ok {
					t.Errorf("rows contain %q, want no row for a feature the check did not answer for", name)
				}
			}

			if got.Updates != tc.wantUpdates {
				t.Errorf("Summarize Updates = %d, want %d", got.Updates, tc.wantUpdates)
			}
			if got.Waiting != tc.wantWaiting {
				t.Errorf("Summarize Waiting = %d, want %d", got.Waiting, tc.wantWaiting)
			}
		})
	}
}

// TestSummarizeCountsEveryRowNotOnlyTheFirst asserts the counts iterate all of
// Rows: a single first-element implementation would report one update and one
// waiting feature here instead of two of each.
func TestSummarizeCountsEveryRowNotOnlyTheFirst(t *testing.T) {
	got := Summarize(
		[]string{"a", "b", "c", "d"},
		[]updex.FeatureCheck{
			check("a", res("comp", "1.0", "2.0", true)),
			check("b", res("comp", "1.0", "2.0", true)),
			check("c", resWaiting("comp", "1.0", "2.0", true, needsImageNew)),
			check("d", resWaiting("comp", "1.0", "2.0", true, needsImageNew)),
		},
	)

	if got.Updates != 2 {
		t.Errorf("Summarize Updates = %d, want 2", got.Updates)
	}
	if got.Waiting != 2 {
		t.Errorf("Summarize Waiting = %d, want 2", got.Waiting)
	}
	if len(got.Rows) != 4 {
		t.Errorf("Summarize rows = %d, want 4", len(got.Rows))
	}
}

// TestSummarizePendingRebootIsCountedInNeitherCount follows the group
// description: a pending reboot is a real state, but it is not an update and
// not a wait, so it changes no number.
func TestSummarizePendingRebootIsCountedInNeitherCount(t *testing.T) {
	got := Summarize(
		[]string{"a"},
		[]updex.FeatureCheck{check("a", resPending("comp", "1.0", "2.0", false))},
	)

	if !got.Rows["a"].PendingReboot {
		t.Fatalf("Summarize row a PendingReboot = false, want true")
	}
	if got.Updates != 0 {
		t.Errorf("Summarize Updates = %d, want 0: a pending reboot is not an update", got.Updates)
	}
	if got.Waiting != 0 {
		t.Errorf("Summarize Waiting = %d, want 0: a pending reboot is not a wait", got.Waiting)
	}
}
