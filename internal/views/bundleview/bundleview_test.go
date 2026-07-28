package bundleview

import (
	"sync"
	"testing"
)

func TestPresentEnumeratesLoadOutcomes(t *testing.T) {
	tests := []struct {
		name               string
		count              int
		warning            string
		homebrewAvailable  bool
		wantDescription    string
		wantPlaceholder    string
		wantPlaceholderSub string
	}{
		{
			name:               "empty",
			homebrewAvailable:  true,
			wantDescription:    "No Brew bundles found",
			wantPlaceholder:    "No bundles available",
			wantPlaceholderSub: "Check the configured bundles_paths directories",
		},
		{
			name:               "empty with errors",
			warning:            "permission denied",
			homebrewAvailable:  true,
			wantDescription:    "Brew bundles could not be loaded",
			wantPlaceholder:    "Bundles unavailable",
			wantPlaceholderSub: "permission denied",
		},
		{
			name:              "one",
			count:             1,
			homebrewAvailable: true,
			wantDescription:   "1 Brew bundle available",
		},
		{
			name:              "multiple",
			count:             2,
			homebrewAvailable: true,
			wantDescription:   "2 Brew bundles available",
		},
		{
			name:              "partial",
			count:             2,
			warning:           "permission denied",
			homebrewAvailable: true,
			wantDescription:   "2 Brew bundles available; some configured paths could not be read: permission denied",
		},
		{
			name:              "one partial",
			count:             1,
			warning:           "permission denied",
			homebrewAvailable: true,
			wantDescription:   "1 Brew bundle available; some configured paths could not be read: permission denied",
		},
		{
			name:               "empty without homebrew",
			wantDescription:    "No Brew bundles found. Homebrew is not installed; install actions are disabled.",
			wantPlaceholder:    "No bundles available",
			wantPlaceholderSub: "Check the configured bundles_paths directories",
		},
		{
			name:            "one without homebrew",
			count:           1,
			wantDescription: "1 Brew bundle available. Homebrew is not installed; install actions are disabled.",
		},
		{
			name:            "partial without homebrew",
			count:           2,
			warning:         "permission denied",
			wantDescription: "2 Brew bundles available; some configured paths could not be read: permission denied. Homebrew is not installed; install actions are disabled.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Present(tt.count, tt.warning, tt.homebrewAvailable)
			if got.Description != tt.wantDescription {
				t.Errorf("Present() description = %q, want %q", got.Description, tt.wantDescription)
			}
			if got.PlaceholderTitle != tt.wantPlaceholder {
				t.Errorf("Present() placeholder title = %q, want %q", got.PlaceholderTitle, tt.wantPlaceholder)
			}
			if got.PlaceholderSubtitle != tt.wantPlaceholderSub {
				t.Errorf("Present() placeholder subtitle = %q, want %q", got.PlaceholderSubtitle, tt.wantPlaceholderSub)
			}
		})
	}
}

func TestGateAllowsOnlyOneConcurrentAction(t *testing.T) {
	var gate InstallGate
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
	var gate InstallGate
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
