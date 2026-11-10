package pageview

import (
	"bufio"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestApplicationRowsCoverEveryPresentation(t *testing.T) {
	tests := []struct {
		name string
		got  Row
		want Row
	}{
		{
			name: "bundle without description",
			got:  BrewBundle("Workstation", "", "/bundles/Workstation.Brewfile"),
			want: Row{Title: "Workstation", Subtitle: "/bundles/Workstation.Brewfile"},
		},
		{
			name: "bundle with description",
			got:  BrewBundle("Workstation", "Developer tools", "/bundles/Workstation.Brewfile"),
			want: Row{Title: "Workstation", Subtitle: "Developer tools — /bundles/Workstation.Brewfile"},
		},
		{
			name: "typed search result",
			got:  SearchResult("firefox", "Cask"),
			want: Row{Title: "firefox", Subtitle: "Cask"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("presentation = %#v, want %#v", tt.got, tt.want)
			}
		})
	}
}

func TestUpdateRowsCoverEveryPresentation(t *testing.T) {
	tapTests := []struct {
		name     string
		formulae []string
		casks    []string
		want     Row
	}{
		{
			name:     "qualified and unqualified packages",
			formulae: []string{"vendor/tools/demo", "plain"},
			casks:    []string{"vendor/apps/gui"},
			want: Row{
				Title:    "vendor/tap",
				Subtitle: "3 installed: demo, plain, gui",
			},
		},
		{
			name: "no packages",
			want: Row{Title: "vendor/tap", Subtitle: "0 installed: "},
		},
	}
	for _, tt := range tapTests {
		t.Run("untrusted tap/"+tt.name, func(t *testing.T) {
			got := UntrustedTap("vendor/tap", tt.formulae, tt.casks)
			if got != tt.want {
				t.Fatalf("UntrustedTap() = %#v, want %#v", got, tt.want)
			}
		})
	}

	updateTests := []struct {
		name         string
		version      string
		installation string
		want         Row
	}{
		{
			name: "system update without version",
			want: Row{Title: "Firefox", Subtitle: "org.mozilla.firefox"},
		},
		{
			name:    "system update with version",
			version: "129.0",
			want:    Row{Title: "Firefox", Subtitle: "org.mozilla.firefox → 129.0"},
		},
		{
			name:         "user update without version",
			installation: "user",
			want:         Row{Title: "Firefox", Subtitle: "org.mozilla.firefox (user)"},
		},
		{
			name:         "user update with version",
			version:      "129.0",
			installation: "user",
			want:         Row{Title: "Firefox", Subtitle: "org.mozilla.firefox → 129.0 (user)"},
		},
	}
	for _, tt := range updateTests {
		t.Run("Flatpak/"+tt.name, func(t *testing.T) {
			got := FlatpakUpdate("Firefox", "org.mozilla.firefox", tt.version, tt.installation)
			if got != tt.want {
				t.Fatalf("FlatpakUpdate() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestBootcUpdateSubtitlesCoverEveryState(t *testing.T) {
	tests := []struct {
		name    string
		staged  bool
		version string
		want    string
	}{
		{
			name: "not staged",
			want: "Check for and download the latest system image",
		},
		{
			name:   "staged without version",
			staged: true,
			want:   "Update staged — restart to apply",
		},
		{
			name:    "staged with version",
			staged:  true,
			version: "42.1",
			want:    "Update 42.1 staged — restart to apply",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BootcUpdateSubtitle(tt.staged, tt.version); got != tt.want {
				t.Fatalf("BootcUpdateSubtitle(%v, %q) = %q, want %q", tt.staged, tt.version, got, tt.want)
			}
		})
	}

	resultTests := []struct {
		name        string
		staged      bool
		version     string
		lastMessage string
		want        string
	}{
		{
			name:   "staged with version",
			staged: true, version: "42.1", lastMessage: "ignored",
			want: "Update 42.1 staged — restart to apply",
		},
		{
			name:   "staged without version",
			staged: true, lastMessage: "ignored",
			want: "Update staged — restart to apply",
		},
		{
			name:        "current with script message",
			lastMessage: "No update available",
			want:        "No update available",
		},
		{
			name: "current without script message",
			want: "System is up to date",
		},
	}
	for _, tt := range resultTests {
		t.Run("stage result/"+tt.name, func(t *testing.T) {
			got := BootcStageResultSubtitle(tt.staged, tt.version, tt.lastMessage)
			if got != tt.want {
				t.Fatalf(
					"BootcStageResultSubtitle(%v, %q, %q) = %q, want %q",
					tt.staged,
					tt.version,
					tt.lastMessage,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestSysupdateSubtitlesCoverEveryOutcome(t *testing.T) {
	// The expected checked-time rendering is derived through the same
	// time.Parse/Local path the formatter uses, so the assertion is
	// timezone-independent without weakening to a substring match.
	checkedAt := "2026-08-10T20:08:01-06:00"
	parsed, err := time.Parse(time.RFC3339, checkedAt)
	if err != nil {
		t.Fatal(err)
	}
	checkedClock := parsed.Local().Format("15:04")

	tests := []struct {
		name                        string
		outcome, version, checkedAt string
		want                        string
	}{
		{
			name:    "staged with version",
			outcome: "staged", version: "20260810200801",
			want: "Update 20260810200801 staged — restart to apply",
		},
		{
			name:    "staged without version",
			outcome: "staged",
			want:    "Update staged — restart to apply",
		},
		{
			name:    "current with check time",
			outcome: "current", checkedAt: checkedAt,
			want: "System is up to date (checked " + checkedClock + ")",
		},
		{
			name:    "current with unparseable check time",
			outcome: "current", checkedAt: "not-a-time",
			want: "System is up to date",
		},
		{
			name:    "failed",
			outcome: "failed", checkedAt: checkedAt,
			want: "Last update check failed — use Check for Updates to retry",
		},
		{
			name: "idle fresh boot",
			want: "Check for and download the latest system image",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SysupdateUpdateSubtitle(tt.outcome, tt.version, tt.checkedAt)
			if got != tt.want {
				t.Fatalf("SysupdateUpdateSubtitle(%q, %q, %q) = %q, want %q",
					tt.outcome, tt.version, tt.checkedAt, got, tt.want)
			}
		})
	}

	resultTests := []struct {
		name        string
		staged      bool
		version     string
		lastMessage string
		want        string
	}{
		{
			name:   "staged with version",
			staged: true, version: "20260810200801", lastMessage: "ignored",
			want: "Update 20260810200801 staged — restart to apply",
		},
		{
			name:        "current with script message",
			lastMessage: "No newer version available.",
			want:        "No newer version available.",
		},
		{
			name: "current without script message",
			want: "System is up to date",
		},
	}
	for _, tt := range resultTests {
		t.Run("stage result/"+tt.name, func(t *testing.T) {
			got := SysupdateStageResultSubtitle(tt.staged, tt.version, tt.lastMessage)
			if got != tt.want {
				t.Fatalf("SysupdateStageResultSubtitle(%v, %q, %q) = %q, want %q",
					tt.staged, tt.version, tt.lastMessage, got, tt.want)
			}
		})
	}

	rollbackTests := []struct {
		name    string
		version string
		want    string
	}{
		{
			name:    "older slot version available",
			version: "20260801000000",
			want:    "Version 20260801000000 is on the inactive slot — choose it in the boot menu at restart to roll back",
		},
		{
			name: "no rollback candidate",
			want: "No previous version on disk",
		},
	}
	for _, tt := range rollbackTests {
		t.Run("rollback/"+tt.name, func(t *testing.T) {
			if got := SysupdateRollbackSubtitle(tt.version); got != tt.want {
				t.Fatalf("SysupdateRollbackSubtitle(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestFeatureRowsAndDescriptions(t *testing.T) {
	if got, want := Feature("gaming", "Gaming support"), (Row{Title: "Gaming support", Subtitle: "gaming"}); got != want {
		t.Fatalf("Feature() = %#v, want %#v", got, want)
	}
	for _, tt := range []struct {
		count int
		want  string
	}{
		{count: 0, want: "0 features available"},
		{count: 1, want: "1 features available"},
		{count: 3, want: "3 features available"},
	} {
		if got := FeatureGroupDescription(tt.count); got != tt.want {
			t.Errorf("FeatureGroupDescription(%d) = %q, want %q", tt.count, got, tt.want)
		}
	}
}

func TestHelpResourcesPreserveConfiguredOrder(t *testing.T) {
	got := HelpResources("https://example.test", "", "https://chat.example.test")
	want := []HelpResource{
		{Title: "Website", URL: "https://example.test"},
		{Title: "Community Discussions", URL: "https://chat.example.test"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("HelpResources() = %#v, want %#v", got, want)
	}

	all := HelpResources("website", "issues", "chat")
	if len(all) != 3 || all[1] != (HelpResource{Title: "Report Issues", URL: "issues"}) {
		t.Fatalf("HelpResources(all configured) = %#v, want all three resources in display order", all)
	}
	if none := HelpResources("", "", ""); len(none) != 0 {
		t.Fatalf("HelpResources(empty) = %#v, want no resources", none)
	}
}

func TestMaintenanceCommandsPreservePrivilegeBoundary(t *testing.T) {
	tests := []struct {
		name string
		sudo bool
		want Command
	}{
		{
			name: "unprivileged",
			want: Command{Name: "/usr/bin/cleanup"},
		},
		{
			name: "privileged",
			sudo: true,
			want: Command{Name: "pkexec", Args: []string{"/usr/bin/cleanup"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaintenanceCommand("/usr/bin/cleanup", tt.sudo)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("MaintenanceCommand() = %#v, want %#v", got, tt.want)
			}
			if got.Privileged() != tt.sudo {
				t.Errorf("Privileged() = %v, want %v", got.Privileged(), tt.sudo)
			}
		})
	}
}

func TestSystemOSReleaseParsing(t *testing.T) {
	input := strings.NewReader(`
# comment
NAME="Snow Linux"
VERSION_ID='42'
HOME_URL=https://snow.example.test
BROKEN

`)
	got, err := ParseOSRelease(input)
	if err != nil {
		t.Fatalf("ParseOSRelease() error = %v", err)
	}
	want := []OSReleaseEntry{
		{Title: "Name", Value: "Snow Linux"},
		{Title: "Version Id", Value: "42"},
		{Title: "Home Url", Value: "https://snow.example.test", IsURL: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseOSRelease() = %#v, want %#v", got, want)
	}

	tooLong := strings.NewReader(strings.Repeat("x", bufio.MaxScanTokenSize+1))
	if _, err := ParseOSRelease(tooLong); err == nil {
		t.Fatal("ParseOSRelease() error = nil for an overlong input line")
	}
}

func TestSystemDigestShortening(t *testing.T) {
	tests := []struct {
		name   string
		digest string
		want   string
	}{
		{name: "empty"},
		{name: "short", digest: "sha256:1234", want: "sha256:1234"},
		{name: "exact boundary", digest: "1234567890123456789", want: "1234567890123456789"},
		{name: "truncated", digest: "12345678901234567890", want: "1234567890123456789..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShortDigest(tt.digest); got != tt.want {
				t.Fatalf("ShortDigest(%q) = %q, want %q", tt.digest, got, tt.want)
			}
		})
	}
}

func TestFirmwareUpdateRows(t *testing.T) {
	tests := []struct {
		name                       string
		deviceName, vendor         string
		currentVersion, newVersion string
		want                       Row
	}{
		{
			name:           "vendor and both versions",
			deviceName:     "ThinkPad Embedded Controller",
			vendor:         "Lenovo",
			currentVersion: "1.2",
			newVersion:     "1.3",
			want:           Row{Title: "ThinkPad Embedded Controller", Subtitle: "Lenovo · 1.2 → 1.3"},
		},
		{
			name:           "no vendor keeps the transition",
			deviceName:     "Thunderbolt Dock",
			currentVersion: "1.0",
			newVersion:     "2.0",
			want:           Row{Title: "Thunderbolt Dock", Subtitle: "1.0 → 2.0"},
		},
		{
			name:       "no current version keeps the new one",
			deviceName: "Thunderbolt Dock",
			vendor:     "Frostyard",
			newVersion: "2.0",
			want:       Row{Title: "Thunderbolt Dock", Subtitle: "Frostyard · 2.0"},
		},
		{
			name:           "no new version keeps the current one",
			deviceName:     "Thunderbolt Dock",
			vendor:         "Frostyard",
			currentVersion: "1.0",
			want:           Row{Title: "Thunderbolt Dock", Subtitle: "Frostyard · 1.0"},
		},
		{
			name: "empty device name falls back to a generic title",
			want: Row{Title: "Unknown device"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FirmwareUpdate(tt.deviceName, tt.vendor, tt.currentVersion, tt.newVersion)
			if got != tt.want {
				t.Fatalf("FirmwareUpdate() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFirmwareBlockedUpdateRows(t *testing.T) {
	tests := []struct {
		name                       string
		deviceName, vendor         string
		currentVersion, newVersion string
		reason                     string
		want                       Row
	}{
		{
			name:           "blocked device keeps its version text and adds the reason",
			deviceName:     "ThinkPad Embedded Controller",
			vendor:         "Lenovo",
			currentVersion: "1.2",
			newVersion:     "1.3",
			reason:         "Needs attention: Not enough battery power",
			want: Row{
				Title:    "ThinkPad Embedded Controller",
				Subtitle: "Lenovo · 1.2 → 1.3 · Needs attention: Not enough battery power",
			},
		},
		{
			name:       "blocked device without vendor or versions",
			deviceName: "Keyboard",
			reason:     "This device cannot be updated now",
			want:       Row{Title: "Keyboard", Subtitle: "This device cannot be updated now"},
		},
		{
			name:       "empty reason keeps the normal text",
			deviceName: "Keyboard",
			vendor:     "Frostyard",
			newVersion: "1.1",
			want:       Row{Title: "Keyboard", Subtitle: "Frostyard · 1.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FirmwareBlockedUpdate(tt.deviceName, tt.vendor, tt.currentVersion, tt.newVersion, tt.reason)
			if got != tt.want {
				t.Fatalf("FirmwareBlockedUpdate() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFirmwareUpdateSubtitleStates(t *testing.T) {
	tests := []struct {
		name           string
		count          int
		err            error
		retainedRows   int
		wantSubtitle   string
		wantExpandable bool
	}{
		{name: "none", count: 0, wantSubtitle: "Firmware is up to date"},
		{name: "one", count: 1, wantSubtitle: "1 firmware update available", wantExpandable: true},
		{name: "several", count: 3, wantSubtitle: "3 firmware updates available", wantExpandable: true},
		{
			name:         "error with no retained rows cannot expand",
			count:        0,
			err:          errors.New("could not reach the daemon"),
			wantSubtitle: "Error checking firmware updates: could not reach the daemon",
		},
		{
			name:           "error with retained rows keeps them reachable",
			count:          0,
			err:            errors.New("could not reach the daemon"),
			retainedRows:   2,
			wantSubtitle:   "Error checking firmware updates: could not reach the daemon",
			wantExpandable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FirmwareUpdatesSubtitle(tt.count, tt.err, tt.retainedRows)
			want := Status{Subtitle: tt.wantSubtitle, Expandable: tt.wantExpandable}
			if got != want {
				t.Fatalf("FirmwareUpdatesSubtitle() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestFirmwareInstallConfirmation(t *testing.T) {
	const powerWarning = "Do not turn off or unplug the computer during the update. Keep the power connected."
	const noRestart = "ChairLift will not restart the computer for you."

	tests := []struct {
		name                       string
		deviceName, newVersion     string
		needsReboot, needsShutdown bool
		wantHeading                string
		wantBody                   []string
		absentBody                 []string
	}{
		{
			name:        "restart needed",
			deviceName:  "ThinkPad Embedded Controller",
			newVersion:  "1.3",
			needsReboot: true,
			wantHeading: "Install firmware for ThinkPad Embedded Controller?",
			wantBody: []string{
				"Version 1.3 will be installed.",
				powerWarning,
				"The computer must restart to finish the update.",
				noRestart,
			},
			absentBody: []string{"must shut down", "disconnect while they update"},
		},
		{
			name:          "shutdown needed",
			deviceName:    "Thunderbolt Dock",
			newVersion:    "2.0",
			needsShutdown: true,
			wantHeading:   "Install firmware for Thunderbolt Dock?",
			wantBody: []string{
				powerWarning,
				"The computer must shut down to finish the update.",
				noRestart,
			},
			absentBody: []string{"must restart", "disconnect while they update"},
		},
		{
			name:        "no restart or shutdown",
			deviceName:  "Keyboard",
			newVersion:  "1.1",
			wantHeading: "Install firmware for Keyboard?",
			wantBody: []string{
				powerWarning,
				"Some devices restart or disconnect while they update.",
				noRestart,
			},
			absentBody: []string{"must restart", "must shut down"},
		},
		{
			name:        "unknown device without a version",
			wantHeading: "Install firmware for Unknown device?",
			wantBody:    []string{powerWarning, noRestart},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			heading, body := FirmwareInstallConfirmation(tt.deviceName, tt.newVersion, tt.needsReboot, tt.needsShutdown)
			if heading != tt.wantHeading {
				t.Errorf("heading = %q, want %q", heading, tt.wantHeading)
			}
			for _, want := range tt.wantBody {
				if !strings.Contains(body, want) {
					t.Errorf("body = %q, want it to contain %q", body, want)
				}
			}
			for _, absent := range tt.absentBody {
				if strings.Contains(body, absent) {
					t.Errorf("body = %q, must not contain %q", body, absent)
				}
			}
		})
	}
}
