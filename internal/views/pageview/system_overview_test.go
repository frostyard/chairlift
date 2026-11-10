package pageview

import "testing"

func TestChannelFromImageReference(t *testing.T) {
	cases := map[string]string{
		"ghcr.io/frostyard/snow:stable":           "stable",
		"ghcr.io/frostyard/snow:testing":          "testing",
		"ghcr.io/frostyard/snow":                  "",
		"localhost:5000/snow":                     "",
		"localhost:5000/snow:beta":                "beta",
		"ghcr.io/frostyard/snow@sha256:abc":       "",
		"ghcr.io/frostyard/snow:stable@sha256:ab": "stable",
		"":                                  "",
		"  ghcr.io/frostyard/snow:stable  ": "stable",
	}
	for ref, want := range cases {
		if got := ImageChannel(ref); got != want {
			t.Errorf("ImageChannel(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestStagedNoteNamesTheVersion(t *testing.T) {
	if got := StagedNote("2026.11.08"); got != "Staged 2026.11.08 · applies on restart" {
		t.Errorf("StagedNote = %q", got)
	}
	if got := StagedNote(""); got != "An update is staged · applies on restart" {
		t.Errorf("StagedNote(\"\") = %q", got)
	}
}

func TestAppDisplayNameSplitsWords(t *testing.T) {
	cases := map[string]string{
		"io.missioncenter.MissionCenter": "Mission Center",
		"org.gnome.SystemMonitor":        "System Monitor",
		"org.kde.plasma-systemmonitor":   "plasma-systemmonitor",
		"htop":                           "htop",
		"net.nokyan.Resources":           "Resources",
		"org.example.GPUViewer":          "GPU Viewer",
		"":                               "",
	}
	for id, want := range cases {
		if got := AppDisplayName(id); got != want {
			t.Errorf("AppDisplayName(%q) = %q, want %q", id, got, want)
		}
	}
}
