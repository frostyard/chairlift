package reporting

import (
	"io/fs"
	"os"
	"testing"
	"time"
)

func TestAgeBucketsOfTheWeeklyCount(t *testing.T) {
	now := testStart
	tests := []struct {
		name string
		age  time.Duration
		want string
		ok   bool
	}{
		{"a minute", time.Minute, "<1w", true},
		{"just under a week", 7*24*time.Hour - time.Second, "<1w", true},
		{"exactly a week", 7 * 24 * time.Hour, "<1m", true},
		{"just under a month", 30*24*time.Hour - time.Second, "<1m", true},
		{"exactly a month", 30 * 24 * time.Hour, "<6m", true},
		{"just under half a year", 182*24*time.Hour - time.Second, "<6m", true},
		{"exactly half a year", 182 * 24 * time.Hour, ">=6m", true},
		{"years", 3 * 365 * 24 * time.Hour, ">=6m", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := InstallAge(now.Add(-tt.age), now)
			if ok != tt.ok || got != tt.want {
				t.Errorf("InstallAge(-%v) = (%q, %v), want (%q, %v)", tt.age, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestAgeRefusesAZeroOrFutureTime proves there is no count without a
// known install time.
func TestAgeRefusesAZeroOrFutureTime(t *testing.T) {
	if got, ok := InstallAge(time.Time{}, testStart); ok || got != "" {
		t.Errorf("InstallAge(zero) = (%q, %v), want (\"\", false)", got, ok)
	}
	if got, ok := InstallAge(testStart.Add(time.Hour), testStart); ok || got != "" {
		t.Errorf("InstallAge(future) = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestChannelCases(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"ghcr.io/frostyard/snow:latest", "latest"},
		{"ghcr.io/frostyard/snow", "other"},
		{"ghcr.io/frostyard/snow:stable", "other"},
		{"ghcr.io/frostyard/snow:latest@sha256:abc", "other"},
		{"ghcr.io/frostyard/snow@sha256:abc", "other"},
		{"localhost:5000/snow:latest", "latest"},
		{"localhost:5000/snow", "other"},
		{"", "other"},
	}
	for _, tt := range tests {
		if got := Channel(tt.ref); got != tt.want {
			t.Errorf("Channel(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

// TestMachineIDTimeReadsOnlyTheTime proves the count reads the modification
// time of the fixed path and nothing else.
func TestMachineIDTimeReadsOnlyTheTime(t *testing.T) {
	want := testStart.Add(-40 * 24 * time.Hour)
	var asked string
	got, ok := MachineIDTime(func(path string) (fs.FileInfo, error) {
		asked = path
		return fakeInfo{mod: want}, nil
	})
	if !ok || !got.Equal(want) {
		t.Fatalf("MachineIDTime() = (%v, %v), want (%v, true)", got, ok, want)
	}
	if asked != MachineIDPath {
		t.Errorf("MachineIDTime read %q, want %q", asked, MachineIDPath)
	}
	if _, ok := MachineIDTime(func(string) (fs.FileInfo, error) { return nil, os.ErrNotExist }); ok {
		t.Error("MachineIDTime() = true for a missing file")
	}
}

// fakeInfo is a file's metadata with only the time set.
type fakeInfo struct {
	mod time.Time
}

func (f fakeInfo) Name() string { return "machine-id" }
func (f fakeInfo) Size() int64  { return 32 }
func (f fakeInfo) Mode() fs.FileMode {
	return 0o444
}
func (f fakeInfo) ModTime() time.Time { return f.mod }
func (f fakeInfo) IsDir() bool        { return false }
func (f fakeInfo) Sys() any           { return nil }

// TestDefaultInstallInfoUsesTheCountsRules proves the production wiring: the
// install age comes from /etc/machine-id's time, and the channel is "other"
// on a host without bootc.
func TestDefaultInstallInfoUsesTheCountsRules(t *testing.T) {
	original := statFile
	t.Cleanup(func() { statFile = original })
	statFile = func(path string) (fs.FileInfo, error) {
		if path != MachineIDPath {
			return nil, os.ErrNotExist
		}
		return fakeInfo{mod: time.Now().Add(-40 * 24 * time.Hour)}, nil
	}
	age, channel, ok := DefaultInstallInfo(t.Context())
	if !ok {
		t.Fatal("DefaultInstallInfo() ok = false")
	}
	if age != "<6m" {
		t.Errorf("age = %q, want <6m", age)
	}
	if channel != "latest" && channel != "other" {
		t.Errorf("channel = %q, want a v1 channel", channel)
	}
	statFile = func(string) (fs.FileInfo, error) { return nil, os.ErrNotExist }
	if _, _, ok := DefaultInstallInfo(t.Context()); ok {
		t.Error("DefaultInstallInfo() ok = true without /etc/machine-id")
	}
}
