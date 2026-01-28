package updates

import "testing"

func TestUpdateCounts_Total(t *testing.T) {
	counts := UpdateCounts{NBC: 1, Flatpak: 2, Homebrew: 3}
	if counts.Total() != 6 {
		t.Errorf("Total() = %d, want 6", counts.Total())
	}
}

func TestUpdateCounts_TotalZeroValues(t *testing.T) {
	counts := UpdateCounts{NBC: 5, Flatpak: 0, Homebrew: 10}
	if counts.Total() != 15 {
		t.Errorf("Total() = %d, want 15", counts.Total())
	}
}

func TestUpdateCounts_TotalEmpty(t *testing.T) {
	counts := UpdateCounts{}
	if counts.Total() != 0 {
		t.Errorf("Total() = %d, want 0", counts.Total())
	}
}

func TestUpdateCounts_TotalOnlyNBC(t *testing.T) {
	counts := UpdateCounts{NBC: 42}
	if counts.Total() != 42 {
		t.Errorf("Total() = %d, want 42", counts.Total())
	}
}

func TestIsNBCAvailable(t *testing.T) {
	// Just verify it returns without panic - actual value depends on system
	result := IsNBCAvailable()

	// On most systems /run/nbc-booted won't exist, so expect false
	// But we don't assert this since the test should work on any system
	t.Logf("IsNBCAvailable() returned %v", result)
}

func TestNBCUpdateStatus_Fields(t *testing.T) {
	// Verify the struct fields are accessible and hold values correctly
	status := NBCUpdateStatus{
		UpdateNeeded:  true,
		NewDigest:     "sha256:abc123",
		CurrentDigest: "sha256:def456",
	}

	if !status.UpdateNeeded {
		t.Error("UpdateNeeded should be true")
	}
	if status.NewDigest != "sha256:abc123" {
		t.Errorf("NewDigest = %q, want %q", status.NewDigest, "sha256:abc123")
	}
	if status.CurrentDigest != "sha256:def456" {
		t.Errorf("CurrentDigest = %q, want %q", status.CurrentDigest, "sha256:def456")
	}
}
