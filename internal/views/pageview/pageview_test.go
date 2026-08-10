package pageview

import (
	"bufio"
	"reflect"
	"strings"
	"testing"
)

func TestApplicationRowsCoverEveryPresentation(t *testing.T) {
	tests := []struct {
		name string
		got  Row
		want Row
	}{
		{
			name: "Flatpak without version",
			got:  FlatpakApplication("Firefox", "org.mozilla.firefox", ""),
			want: Row{Title: "Firefox", Subtitle: "org.mozilla.firefox"},
		},
		{
			name: "Flatpak with version",
			got:  FlatpakApplication("Firefox", "org.mozilla.firefox", "128.0"),
			want: Row{Title: "Firefox", Subtitle: "org.mozilla.firefox (128.0)"},
		},
		{
			name: "unpinned Homebrew package",
			got:  HomebrewPackage("ripgrep", "14.1.1", false),
			want: Row{Title: "ripgrep", Subtitle: "14.1.1"},
		},
		{
			name: "pinned Homebrew package",
			got:  HomebrewPackage("ripgrep", "14.1.1", true),
			want: Row{Title: "ripgrep", Subtitle: "14.1.1 • Pinned"},
		},
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
