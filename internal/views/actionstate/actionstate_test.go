package actionstate

import (
	"reflect"
	"sync"
	"testing"
)

func TestPackageUpgradeEnumeratesEveryOutcome(t *testing.T) {
	tests := []struct {
		name      string
		succeeded bool
		dryRun    bool
		want      Decision
	}{
		{
			name: "failure restores the action without changing rows",
			want: Decision{RestoreControl: true},
		},
		{
			name:      "dry-run success restores the action without refreshing",
			succeeded: true,
			dryRun:    true,
			want:      Decision{RestoreControl: true},
		},
		{
			name:      "live success removes the row and refreshes metadata",
			succeeded: true,
			want:      Decision{Refresh: true, RemoveRow: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PackageUpgrade(tt.succeeded, tt.dryRun)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("PackageUpgrade(%v, %v) = %#v, want %#v", tt.succeeded, tt.dryRun, got, tt.want)
			}
		})
	}
}

func TestPackageInstallEnumeratesEveryOutcome(t *testing.T) {
	tests := []struct {
		name      string
		succeeded bool
		dryRun    bool
		want      Decision
	}{
		{
			name: "failure restores the install control",
			want: Decision{RestoreControl: true},
		},
		{
			name:      "dry-run success restores the install control",
			succeeded: true,
			dryRun:    true,
			want:      Decision{RestoreControl: true},
		},
		{
			name:      "live success completes the control and refreshes installed packages",
			succeeded: true,
			want:      Decision{Refresh: true, CompleteControl: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PackageInstall(tt.succeeded, tt.dryRun)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("PackageInstall(%v, %v) = %#v, want %#v", tt.succeeded, tt.dryRun, got, tt.want)
			}
		})
	}
}

func TestMetadataUpdateEnumeratesEveryOutcome(t *testing.T) {
	tests := []struct {
		name      string
		succeeded bool
		dryRun    bool
		want      Decision
	}{
		{
			name: "failure restores the action without refreshing",
			want: Decision{RestoreControl: true},
		},
		{
			name:      "dry-run success restores the action without refreshing",
			succeeded: true,
			dryRun:    true,
			want:      Decision{RestoreControl: true},
		},
		{
			name:      "live success refreshes before restoring the action",
			succeeded: true,
			want:      Decision{Refresh: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MetadataUpdate(tt.succeeded, tt.dryRun)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("MetadataUpdate(%v, %v) = %#v, want %#v", tt.succeeded, tt.dryRun, got, tt.want)
			}
		})
	}
}

func TestOutdatedRefreshPreservesLastKnownStateOnFailure(t *testing.T) {
	tests := []struct {
		name            string
		succeeded       bool
		currentCount    int
		discoveredCount int
		want            RefreshDecision
	}{
		{
			name:            "failed query preserves current count and rows",
			currentCount:    4,
			discoveredCount: 0,
			want:            RefreshDecision{Count: 4},
		},
		{
			name:            "successful query replaces count and rows",
			succeeded:       true,
			currentCount:    4,
			discoveredCount: 2,
			want:            RefreshDecision{ReplaceRows: true, Count: 2},
		},
		{
			name:         "successful empty query clears count and rows",
			succeeded:    true,
			currentCount: 4,
			want:         RefreshDecision{ReplaceRows: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OutdatedRefresh(tt.succeeded, tt.currentCount, tt.discoveredCount)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf(
					"OutdatedRefresh(%v, %d, %d) = %#v, want %#v",
					tt.succeeded,
					tt.currentCount,
					tt.discoveredCount,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestOutdatedPresentationCoversCountStates(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  Presentation
	}{
		{
			name: "zero",
			want: Presentation{Subtitle: "0 packages available"},
		},
		{
			name:  "singular",
			count: 1,
			want:  Presentation{Subtitle: "1 package available", Expandable: true},
		},
		{
			name:  "plural",
			count: 2,
			want:  Presentation{Subtitle: "2 packages available", Expandable: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OutdatedPresentation(tt.count)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("OutdatedPresentation(%d) = %#v, want %#v", tt.count, got, tt.want)
			}
		})
	}
}

func TestGateRejectsRepeatedConcurrentStarts(t *testing.T) {
	var gate Gate
	const callers = 64

	start := make(chan struct{})
	results := make(chan bool, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- gate.TryStart()
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	acquired := 0
	for result := range results {
		if result {
			acquired++
		}
	}
	if acquired != 1 {
		t.Fatalf("concurrent TryStart acquisitions = %d, want exactly 1", acquired)
	}
}

func TestGateResetAndCompletion(t *testing.T) {
	var gate Gate
	if !gate.TryStart() {
		t.Fatal("zero-value gate did not start")
	}
	gate.Reset()
	if !gate.TryStart() {
		t.Fatal("reset gate did not restart")
	}
	gate.Complete()
	if gate.TryStart() {
		t.Fatal("completed gate restarted")
	}
	gate.Reset()
	if gate.TryStart() {
		t.Fatal("reset reopened a completed gate")
	}
}

func TestRefreshGateAllowsOnlyTheNewestGeneration(t *testing.T) {
	var gate RefreshGate
	if gate.IsCurrent(0) {
		t.Fatal("zero generation is current before any refresh")
	}

	first := gate.Begin()
	if !gate.IsCurrent(first) {
		t.Fatalf("first generation %d is not current", first)
	}

	second := gate.Begin()
	if second <= first {
		t.Fatalf("second generation %d is not newer than first %d", second, first)
	}
	if gate.IsCurrent(first) {
		t.Fatalf("superseded generation %d remains current", first)
	}
	if !gate.IsCurrent(second) {
		t.Fatalf("newest generation %d is not current", second)
	}
}

func TestRefreshGateConcurrentRequestsHaveOneCurrentGeneration(t *testing.T) {
	var gate RefreshGate
	const callers = 64

	generations := make(chan uint64, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			generations <- gate.Begin()
		}()
	}
	wg.Wait()
	close(generations)

	seen := make(map[uint64]bool, callers)
	current := 0
	for generation := range generations {
		if seen[generation] {
			t.Fatalf("generation %d was assigned more than once", generation)
		}
		seen[generation] = true
		if gate.IsCurrent(generation) {
			current++
		}
	}
	if len(seen) != callers {
		t.Fatalf("unique generation count = %d, want %d", len(seen), callers)
	}
	if current != 1 {
		t.Fatalf("current generation count = %d, want exactly 1", current)
	}
}
