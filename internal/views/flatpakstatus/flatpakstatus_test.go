package flatpakstatus

import (
	"fmt"
	"strings"
	"testing"
)

// fixedCount is the single update count used for every n>0 case, so that the
// distinctness assertion below cannot be satisfied merely by the number
// differing between cases.
const fixedCount = 3

type caseSpec struct {
	name         string
	count        int
	userFailed   bool
	systemFailed bool
	want         string
}

// tableCases is one entry per row of the spec's five-case table, with the two
// "exactly one query failed" rows expanded into their userFailed-only and
// systemFailed-only variants.
func tableCases() []caseSpec {
	return []caseSpec{
		{
			name:  "both ok, no updates",
			count: 0,
			want:  "All applications are up to date",
		},
		{
			name:  "both ok, updates found",
			count: fixedCount,
			want:  fmt.Sprintf("%d updates available", fixedCount),
		},
		{
			name:       "user query failed, no updates",
			count:      0,
			userFailed: true,
			want:       "No updates found in the system installation; the user installation could not be checked",
		},
		{
			name:         "system query failed, no updates",
			count:        0,
			systemFailed: true,
			want:         "No updates found in the user installation; the system installation could not be checked",
		},
		{
			name:       "user query failed, updates found",
			count:      fixedCount,
			userFailed: true,
			want:       fmt.Sprintf("%d updates available; the user installation could not be checked", fixedCount),
		},
		{
			name:         "system query failed, updates found",
			count:        fixedCount,
			systemFailed: true,
			want:         fmt.Sprintf("%d updates available; the system installation could not be checked", fixedCount),
		},
		{
			name:         "both queries failed",
			count:        0,
			userFailed:   true,
			systemFailed: true,
			want:         "Could not check for updates",
		},
	}
}

func TestSubtitleTable(t *testing.T) {
	for _, tc := range tableCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := Subtitle(tc.count, tc.userFailed, tc.systemFailed)

			if got.Subtitle != tc.want {
				t.Errorf("Subtitle(%d, %t, %t).Subtitle = %q, want %q",
					tc.count, tc.userFailed, tc.systemFailed, got.Subtitle, tc.want)
			}

			wantExpandable := tc.count > 0
			if got.Expandable != wantExpandable {
				t.Errorf("Subtitle(%d, %t, %t).Expandable = %t, want %t",
					tc.count, tc.userFailed, tc.systemFailed, got.Expandable, wantExpandable)
			}
		})
	}
}

func TestSubtitlesArePairwiseDistinct(t *testing.T) {
	seen := make(map[string]string)
	for _, tc := range tableCases() {
		got := Subtitle(tc.count, tc.userFailed, tc.systemFailed).Subtitle
		if other, ok := seen[got]; ok {
			t.Errorf("case %q and case %q both produce %q; every case must be distinguishable",
				tc.name, other, got)
			continue
		}
		seen[got] = tc.name
	}
}

func TestUpToDateClaimOnlyWhenBothQueriesSucceededAndFoundNothing(t *testing.T) {
	const claim = "up to date"

	for _, tc := range tableCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := Subtitle(tc.count, tc.userFailed, tc.systemFailed).Subtitle
			bothOK := !tc.userFailed && !tc.systemFailed
			wantClaim := bothOK && tc.count == 0

			if hasClaim := strings.Contains(got, claim); hasClaim != wantClaim {
				t.Errorf("Subtitle(%d, %t, %t).Subtitle = %q; contains %q = %t, want %t",
					tc.count, tc.userFailed, tc.systemFailed, got, claim, hasClaim, wantClaim)
			}
		})
	}
}

func TestSubtitleSingularAndPlural(t *testing.T) {
	one := Subtitle(1, false, false).Subtitle
	if !strings.Contains(one, "1 update available") {
		t.Errorf("Subtitle(1, false, false).Subtitle = %q, want it to contain %q", one, "1 update available")
	}
	if strings.Contains(one, "1 updates") {
		t.Errorf("Subtitle(1, false, false).Subtitle = %q, must not contain %q", one, "1 updates")
	}

	two := Subtitle(2, false, false).Subtitle
	if !strings.Contains(two, "2 updates available") {
		t.Errorf("Subtitle(2, false, false).Subtitle = %q, want it to contain %q", two, "2 updates available")
	}

	onePartial := Subtitle(1, true, false).Subtitle
	if !strings.Contains(onePartial, "1 update available") || strings.Contains(onePartial, "1 updates") {
		t.Errorf("Subtitle(1, true, false).Subtitle = %q, want singular %q", onePartial, "1 update available")
	}
}

func TestPartialResultNamesTheInstallationThatCouldNotBeChecked(t *testing.T) {
	tests := []struct {
		name         string
		count        int
		userFailed   bool
		systemFailed bool
		wantNamed    string
	}{
		{name: "user query failed, no updates", count: 0, userFailed: true, wantNamed: "user"},
		{name: "system query failed, no updates", count: 0, systemFailed: true, wantNamed: "system"},
		{name: "user query failed, updates found", count: fixedCount, userFailed: true, wantNamed: "user"},
		{name: "system query failed, updates found", count: fixedCount, systemFailed: true, wantNamed: "system"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Subtitle(tc.count, tc.userFailed, tc.systemFailed).Subtitle

			wantPhrase := fmt.Sprintf("the %s installation could not be checked", tc.wantNamed)
			if !strings.Contains(got, wantPhrase) {
				t.Errorf("Subtitle(%d, %t, %t).Subtitle = %q, want it to contain %q",
					tc.count, tc.userFailed, tc.systemFailed, got, wantPhrase)
			}

			if tc.count > 0 && !strings.Contains(got, fmt.Sprintf("%d updates available", tc.count)) {
				t.Errorf("Subtitle(%d, %t, %t).Subtitle = %q, want it to still report the %d updates that were found",
					tc.count, tc.userFailed, tc.systemFailed, got, tc.count)
			}
		})
	}
}
