package badgestate

import (
	"sync"
	"testing"
)

func TestCountsReplaceRepeatedRefreshState(t *testing.T) {
	var counts Counts
	if got := counts.Total(); got != 0 {
		t.Fatalf("zero-value Total() = %d, want 0", got)
	}

	if got := counts.Set(Bootc, 1); got != (Snapshot{Count: 1, Total: 1}) {
		t.Fatalf("Set(Bootc, 1) = %#v", got)
	}
	if got := counts.Set(Flatpak, 4); got != (Snapshot{Count: 4, Total: 5}) {
		t.Fatalf("Set(Flatpak, 4) = %#v", got)
	}
	if got := counts.Set(Homebrew, 3); got != (Snapshot{Count: 3, Total: 8}) {
		t.Fatalf("Set(Homebrew, 3) = %#v", got)
	}
	if got := counts.Set(Sysupdate, 1); got != (Snapshot{Count: 1, Total: 9}) {
		t.Fatalf("Set(Sysupdate, 1) = %#v", got)
	}
	if got := counts.Set(Firmware, 2); got != (Snapshot{Count: 2, Total: 11}) {
		t.Fatalf("Set(Firmware, 2) = %#v", got)
	}

	// A second provider refresh replaces its previous count; it must not
	// accumulate duplicate rows into the badge total.
	if got := counts.Set(Flatpak, 2); got != (Snapshot{Count: 2, Total: 9}) {
		t.Fatalf("second Set(Flatpak, 2) = %#v, want replacement total", got)
	}
	if got := counts.Get(Flatpak); got != 2 {
		t.Fatalf("Get(Flatpak) = %d, want 2", got)
	}
}

func TestCountsAddNeverProducesNegativeBadgeState(t *testing.T) {
	var counts Counts
	counts.Set(Homebrew, 2)

	if got := counts.Add(Homebrew, -1); got != (Snapshot{Count: 1, Total: 1}) {
		t.Fatalf("Add(Homebrew, -1) = %#v", got)
	}
	if got := counts.Add(Homebrew, -10); got != (Snapshot{Count: 0, Total: 0}) {
		t.Fatalf("Add(Homebrew, -10) = %#v, want clamped zero", got)
	}

	counts.Set(Firmware, 2)
	if got := counts.Add(Firmware, -1); got != (Snapshot{Count: 1, Total: 1}) {
		t.Fatalf("Add(Firmware, -1) = %#v", got)
	}
	counts.Set(Firmware, 0)
	if got := counts.Set(Flatpak, -4); got != (Snapshot{Count: 0, Total: 0}) {
		t.Fatalf("Set(Flatpak, -4) = %#v, want clamped zero", got)
	}
}

func TestCountsRejectUnknownSourceWithoutChangingTotal(t *testing.T) {
	var counts Counts
	counts.Set(Bootc, 1)
	unknown := Source(255)

	if got := counts.Set(unknown, 10); got != (Snapshot{Count: 0, Total: 1}) {
		t.Fatalf("Set(unknown, 10) = %#v", got)
	}
	if got := counts.Add(unknown, 10); got != (Snapshot{Count: 0, Total: 1}) {
		t.Fatalf("Add(unknown, 10) = %#v", got)
	}
	if got := counts.Get(unknown); got != 0 {
		t.Fatalf("Get(unknown) = %d, want 0", got)
	}
}

// TestCountsCoverEveryProvider proves the source order and count of providers:
// the five update providers, then the sourceCount sentinel.
func TestCountsCoverEveryProvider(t *testing.T) {
	providers := []Source{Bootc, Flatpak, Homebrew, Sysupdate, Firmware}
	if int(sourceCount) != len(providers) {
		t.Fatalf("sourceCount = %d, want %d providers", int(sourceCount), len(providers))
	}
	for index, source := range providers {
		if int(source) != index {
			t.Errorf("provider %d = %d, want %d", index, int(source), index)
		}
	}
}

func TestCountsConcurrentProviderUpdates(t *testing.T) {
	var counts Counts
	var wg sync.WaitGroup
	for _, source := range []Source{Bootc, Flatpak, Homebrew, Sysupdate, Firmware} {
		source := source
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				counts.Add(source, 1)
			}
		}()
	}
	wg.Wait()

	if got := counts.Total(); got != 500 {
		t.Fatalf("Total() after concurrent updates = %d, want 500", got)
	}
	for _, source := range []Source{Bootc, Flatpak, Homebrew, Sysupdate, Firmware} {
		if got := counts.Get(source); got != 100 {
			t.Errorf("Get(%d) = %d, want 100", source, got)
		}
	}
}
