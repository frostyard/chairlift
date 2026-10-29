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

// branch identifies which of the six subtitle branches a case exercises. The
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

	return append(cases, waitingCases()...)
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

	if len(seen) != 6 {
		t.Fatalf("covered %d subtitle branches, want all 6", len(seen))
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
