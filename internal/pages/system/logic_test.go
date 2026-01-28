package system

import (
	"context"
	"strings"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	// ParseOSRelease reads /etc/os-release which should exist on Linux systems
	entries, err := ParseOSRelease()
	if err != nil {
		t.Fatalf("ParseOSRelease failed: %v", err)
	}

	if len(entries) == 0 {
		t.Error("expected at least one entry from /etc/os-release")
	}

	// Verify each entry has expected fields populated
	for i, entry := range entries {
		if entry.Key == "" {
			t.Errorf("entry %d: Key should not be empty", i)
		}
		if entry.Value == "" {
			t.Errorf("entry %d: Value should not be empty (key=%s)", i, entry.Key)
		}
		if entry.DisplayKey == "" {
			t.Errorf("entry %d: DisplayKey should not be empty (key=%s)", i, entry.Key)
		}

		// Verify IsURL is set correctly for URL keys
		if strings.HasSuffix(entry.Key, "URL") && !entry.IsURL {
			t.Errorf("entry %d: expected IsURL=true for key %q", i, entry.Key)
		}
		if !strings.HasSuffix(entry.Key, "URL") && entry.IsURL {
			t.Errorf("entry %d: expected IsURL=false for key %q", i, entry.Key)
		}
	}
}

func TestParseOSRelease_DisplayKeyFormat(t *testing.T) {
	// ParseOSRelease returns entries with formatted DisplayKey
	entries, err := ParseOSRelease()
	if err != nil {
		t.Fatalf("ParseOSRelease failed: %v", err)
	}

	// Find some common keys and verify their DisplayKey format
	keyTests := map[string]string{
		"PRETTY_NAME": "Pretty Name",
		"VERSION_ID":  "Version Id",
		"HOME_URL":    "Home Url",
		"ID":          "Id",
		"NAME":        "Name",
	}

	for _, entry := range entries {
		if expected, ok := keyTests[entry.Key]; ok {
			if entry.DisplayKey != expected {
				t.Errorf("key %q: expected DisplayKey %q, got %q", entry.Key, expected, entry.DisplayKey)
			}
		}
	}
}

func TestIsNBCAvailable(t *testing.T) {
	// Just verify it returns without panic - actual value depends on system
	result := IsNBCAvailable()

	// Type check - should be a bool
	_ = result // Use the result to avoid unused variable warning

	// On most systems /run/nbc-booted won't exist, so expect false
	// But we don't assert this since the test should work on any system
	t.Logf("IsNBCAvailable() returned %v", result)
}

func TestGetOSReleaseValue(t *testing.T) {
	entries := []OSReleaseEntry{
		{Key: "ID", Value: "debian"},
		{Key: "IMAGE_ID", Value: "snowloaded"},
		{Key: "IMAGE_VERSION", Value: "20260128190159"},
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"ID", "debian"},
		{"IMAGE_ID", "snowloaded"},
		{"IMAGE_VERSION", "20260128190159"},
		{"NONEXISTENT", ""},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			result := GetOSReleaseValue(entries, tc.key)
			if result != tc.expected {
				t.Errorf("GetOSReleaseValue(entries, %q) = %q, want %q", tc.key, result, tc.expected)
			}
		})
	}
}

func TestGetOSReleaseValue_EmptyEntries(t *testing.T) {
	result := GetOSReleaseValue(nil, "ID")
	if result != "" {
		t.Errorf("GetOSReleaseValue(nil, \"ID\") = %q, want empty string", result)
	}

	result = GetOSReleaseValue([]OSReleaseEntry{}, "ID")
	if result != "" {
		t.Errorf("GetOSReleaseValue([], \"ID\") = %q, want empty string", result)
	}
}

func TestFetchManifest_EmptyParams(t *testing.T) {
	tests := []struct {
		name         string
		imageID      string
		imageVersion string
	}{
		{"empty image ID", "", "20260128190159"},
		{"empty image version", "snowloaded", ""},
		{"both empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest, err := FetchManifest(context.Background(), tc.imageID, tc.imageVersion)
			if err != nil {
				t.Errorf("FetchManifest() returned error: %v", err)
			}
			if manifest != nil {
				t.Error("FetchManifest() returned non-nil manifest for empty params")
			}
		})
	}
}

func TestFetchManifest_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := FetchManifest(ctx, "snowloaded", "20260128190159")
	if err == nil {
		t.Error("FetchManifest() should return error for cancelled context")
	}
}
