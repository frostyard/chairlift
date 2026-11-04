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
			name:         "both queries failed, no updates",
			count:        0,
			userFailed:   true,
			systemFailed: true,
			want:         "Could not check for updates",
		},
		{
			name:         "both queries failed, one update found",
			count:        1,
			userFailed:   true,
			systemFailed: true,
			want:         "1 update available; the user and system installations could not be fully checked",
		},
		{
			name:         "both queries failed, updates found",
			count:        fixedCount,
			userFailed:   true,
			systemFailed: true,
			want:         fmt.Sprintf("%d updates available; the user and system installations could not be fully checked", fixedCount),
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

			wantAuthoritative := !tc.userFailed || !tc.systemFailed || tc.count > 0
			if got.Authoritative != wantAuthoritative {
				t.Errorf("Subtitle(%d, %t, %t).Authoritative = %t, want %t",
					tc.count, tc.userFailed, tc.systemFailed, got.Authoritative, wantAuthoritative)
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

// TestSubtitleAuthoritative pins the contract the Updates page relies on to
// decide whether a completed load may replace the rows and badge count: a load
// is authoritative when at least one installation returned a usable result.
func TestSubtitleAuthoritative(t *testing.T) {
	tests := []struct {
		name         string
		count        int
		userFailed   bool
		systemFailed bool
		want         bool
	}{
		{name: "both queries succeeded with no updates", want: true},
		{name: "both queries succeeded with updates", count: 3, want: true},
		{name: "only the user query failed, no updates", userFailed: true, want: true},
		{name: "only the user query failed, updates", count: 2, userFailed: true, want: true},
		{name: "only the system query failed, no updates", systemFailed: true, want: true},
		{name: "only the system query failed, updates", count: 2, systemFailed: true, want: true},
		{name: "both queries failed, no updates", userFailed: true, systemFailed: true, want: false},
		{name: "both queries failed, one update", count: 1, userFailed: true, systemFailed: true, want: true},
		{name: "both queries failed, updates", count: 3, userFailed: true, systemFailed: true, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Subtitle(tc.count, tc.userFailed, tc.systemFailed).Authoritative; got != tc.want {
				t.Errorf("Subtitle(%d, %t, %t).Authoritative = %t, want %t",
					tc.count, tc.userFailed, tc.systemFailed, got, tc.want)
			}
		})
	}
}

// TestBothInstallationsPartialPublishesHealthyUpdates guards issue #263: two
// RemoteErrors can still carry updates, and those partial results are usable.
func TestBothInstallationsPartialPublishesHealthyUpdates(t *testing.T) {
	tests := []struct {
		count        int
		wantSubtitle string
	}{
		{count: 1, wantSubtitle: "1 update available; the user and system installations could not be fully checked"},
		{count: 3, wantSubtitle: "3 updates available; the user and system installations could not be fully checked"},
	}

	for _, tc := range tests {
		result := Subtitle(tc.count, true, true)
		if !result.Authoritative {
			t.Errorf("Subtitle(%d, true, true).Authoritative = false, want true", tc.count)
		}
		if !result.Expandable {
			t.Errorf("Subtitle(%d, true, true).Expandable = false, want true", tc.count)
		}
		if result.Subtitle != tc.wantSubtitle {
			t.Errorf("Subtitle(%d, true, true).Subtitle = %q, want %q", tc.count, result.Subtitle, tc.wantSubtitle)
		}
		if !strings.Contains(result.Subtitle, "user") || !strings.Contains(result.Subtitle, "system") {
			t.Errorf("Subtitle(%d, true, true).Subtitle = %q, want both installations named", tc.count, result.Subtitle)
		}
	}
}

func TestBothQueriesFailedWithoutUpdatesKeepsKnownState(t *testing.T) {
	result := Subtitle(0, true, true)
	if result.Authoritative {
		t.Error("Subtitle(0, true, true).Authoritative = true, want false")
	}
	if result.Expandable {
		t.Error("Subtitle(0, true, true).Expandable = true, want false")
	}
	if result.Subtitle != "Could not check for updates" {
		t.Errorf("Subtitle(0, true, true).Subtitle = %q, want %q", result.Subtitle, "Could not check for updates")
	}
}
