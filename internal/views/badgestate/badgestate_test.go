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

	// A second provider refresh replaces its previous count; it must not
	// accumulate duplicate rows into the badge total.
	if got := counts.Set(Flatpak, 2); got != (Snapshot{Count: 2, Total: 7}) {
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

func TestCountsConcurrentProviderUpdates(t *testing.T) {
	var counts Counts
	var wg sync.WaitGroup
	for _, source := range []Source{Bootc, Flatpak, Homebrew, Sysupdate} {
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

	if got := counts.Total(); got != 400 {
		t.Fatalf("Total() after concurrent updates = %d, want 400", got)
	}
	for _, source := range []Source{Bootc, Flatpak, Homebrew, Sysupdate} {
		if got := counts.Get(source); got != 100 {
			t.Errorf("Get(%d) = %d, want 100", source, got)
		}
	}
}
