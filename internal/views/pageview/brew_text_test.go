package pageview

import (
	"reflect"
	"testing"
)

func TestBrewfileEntriesListsPackagesNotTaps(t *testing.T) {
	text := `# Recommended CLI Tools
tap "valkyrie00/bbrew"
brew "atuin"
brew "valkyrie00/bbrew/bbrew"

  # a comment
cask "font-fira-code-nerd-font"
brew 'bat', args: ["HEAD"]
mas "Xcode", id: 497799835
flatpak "org.gnome.Boxes"
brewx "nope"
brew unquoted
brew ""
vscode "golang.go"`
	got := BrewfileEntries(text)
	want := []BrewfileEntry{
		{"brew", "atuin"},
		{"brew", "bbrew"},
		{"cask", "font-fira-code-nerd-font"},
		{"brew", "bat"},
		{"mas", "Xcode"},
		{"flatpak", "org.gnome.Boxes"},
		{"vscode", "golang.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BrewfileEntries =\n%v\nwant\n%v", got, want)
	}
	if got := BrewfileEntries(""); got != nil {
		t.Errorf("empty Brewfile = %v, want nil", got)
	}
}

func TestBrewTrustBannerNamesThePackages(t *testing.T) {
	tests := []struct {
		name            string
		formulae, casks []string
		want            string
	}{
		{"one", []string{"ublue-os/tap/bluefin-cli"}, nil,
			"Homebrew skips bluefin-cli because ublue-os/tap is not trusted. Trust the tap to update its packages again."},
		{"two kinds", []string{"ublue-os/tap/bluefin-cli"}, []string{"ublue-os/tap/jetbrains-toolbox"},
			"Homebrew skips bluefin-cli and jetbrains-toolbox because ublue-os/tap is not trusted. Trust the tap to update its packages again."},
		{"many", []string{"t/a/a", "t/a/b", "t/a/c", "t/a/d", "t/a/e"}, nil,
			"Homebrew skips a, b, c and 2 more because ublue-os/tap is not trusted. Trust the tap to update its packages again."},
		{"none", nil, nil,
			"Homebrew skips its packages because ublue-os/tap is not trusted. Trust the tap to update its packages again."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BrewTrustBanner("ublue-os/tap", tt.formulae, tt.casks); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestBrewSourceNoteSaysCountsAndHeld(t *testing.T) {
	tests := []struct {
		checked                bool
		outdated, held, untrus int
		want                   string
	}{
		{false, 0, 0, 0, ""},
		{true, 0, 0, 0, "Up to date"},
		{true, 3, 0, 0, "3 outdated"},
		{true, 3, 1, 1, "3 outdated · 1 held · 1 untrusted tap"},
		{false, 0, 0, 2, "2 untrusted taps"},
	}
	for _, tt := range tests {
		if got := BrewSourceNote(tt.checked, tt.outdated, tt.held, tt.untrus); got != tt.want {
			t.Errorf("BrewSourceNote(%v, %d, %d, %d) = %q, want %q", tt.checked, tt.outdated, tt.held, tt.untrus, got, tt.want)
		}
	}
}

func TestBrewSearchStatusFollowsTheSearch(t *testing.T) {
	tests := []struct {
		searching, ran, failed bool
		results                int
		want                   string
	}{
		{false, false, false, 0, "Type a name and press Enter to search Homebrew's formulae and casks."},
		{true, true, false, 3, "Searching formulae and casks for “git”…"},
		{false, true, true, 0, "The search for “git” failed."},
		{false, true, false, 0, "No formula or cask matches “git”."},
		{false, true, false, 1, "1 result for “git”"},
		{false, true, false, 12, "12 results for “git”"},
	}
	for _, tt := range tests {
		if got := BrewSearchStatus(tt.searching, "git", tt.ran, tt.results, tt.failed); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestBrewInstallCommandCarriesTheKind(t *testing.T) {
	if got := BrewInstallCommand("docker", true); got != "brew install --cask docker" {
		t.Errorf("cask: %q", got)
	}
	if got := BrewInstallCommand("docker", false); got != "brew install docker" {
		t.Errorf("formula: %q", got)
	}
	if got := BrewShortName("a/b/c"); got != "c" {
		t.Errorf("BrewShortName = %q", got)
	}
	if got := BrewCount(1, "package", "packages"); got != "1 package" {
		t.Errorf("BrewCount(1) = %q", got)
	}
}
