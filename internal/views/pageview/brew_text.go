package pageview

import (
	"fmt"
	"strings"
)

// BrewfileEntry is one package a Brewfile installs.
type BrewfileEntry struct {
	// Kind is the Brewfile directive: "brew", "cask", "mas", "vscode",
	// "flatpak", "whalebrew", "go" or "cargo".
	Kind string
	// Name is the quoted argument, without a formula's tap prefix:
	// `brew "valkyrie00/bbrew/bbrew"` is "bbrew".
	Name string
}

// brewfileKinds are the Brewfile directives that install a package. A
// `tap` line adds a source and installs nothing.
var brewfileKinds = map[string]bool{
	"brew": true, "cask": true, "mas": true, "vscode": true,
	"flatpak": true, "whalebrew": true, "go": true, "cargo": true,
}

// BrewfileEntries returns the packages a Brewfile lists, in its order:
// every brew, cask, mas, vscode, flatpak, whalebrew, go and cargo line.
// Comments, blank lines, tap lines and lines it cannot read are left out.
func BrewfileEntries(text string) []BrewfileEntry {
	var out []BrewfileEntry
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kind, rest, ok := strings.Cut(line, " ")
		if !ok || !brewfileKinds[kind] {
			continue
		}
		rest = strings.TrimSpace(rest)
		quote := rest[:min(1, len(rest))]
		if quote != `"` && quote != `'` {
			continue
		}
		name, _, closed := strings.Cut(rest[1:], quote)
		if !closed || strings.TrimSpace(name) == "" {
			continue
		}
		if kind == "brew" || kind == "cask" {
			name = BrewShortName(name)
		}
		out = append(out, BrewfileEntry{Kind: kind, Name: name})
	}
	return out
}

// BrewShortName drops a tap prefix from a qualified Homebrew package
// name: "ublue-os/tap/bluefin-cli" is "bluefin-cli".
func BrewShortName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// BrewCount says how many of a thing there are: "1 package", "3 packages".
func BrewCount(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// brewJoinNames joins names as a sentence does: "a", "a and b", "a, b and c".
func brewJoinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// BrewTrustBanner is the Updates page's warning about an untrusted tap
// with installed packages: Homebrew leaves them out of upgrades until the
// user trusts the tap. formulae and casks are the tap's installed
// packages, qualified or not; more than four are summed up.
func BrewTrustBanner(tap string, formulae, casks []string) string {
	var names []string
	for _, list := range [][]string{formulae, casks} {
		for _, n := range list {
			names = append(names, BrewShortName(n))
		}
	}
	what := "its packages"
	switch {
	case len(names) > 4:
		what = fmt.Sprintf("%s and %d more", strings.Join(names[:3], ", "), len(names)-3)
	case len(names) > 0:
		what = brewJoinNames(names)
	}
	return fmt.Sprintf("Homebrew skips %s because %s is not trusted. Trust the tap to update its packages again.", what, tap)
}

// BrewSourceNote is the line under the Homebrew source's title on the
// Updates page: "3 outdated · 1 held · 1 untrusted tap", or "Up to date".
// checked is false before the first check ended, when it says nothing.
func BrewSourceNote(checked bool, outdated, held, untrustedTaps int) string {
	var parts []string
	if checked {
		if outdated == 0 {
			parts = append(parts, "Up to date")
		} else {
			parts = append(parts, fmt.Sprintf("%d outdated", outdated))
			if held > 0 {
				parts = append(parts, fmt.Sprintf("%d held", held))
			}
		}
	}
	if untrustedTaps > 0 {
		parts = append(parts, BrewCount(untrustedTaps, "untrusted tap", "untrusted taps"))
	}
	return strings.Join(parts, " · ")
}

// BrewSearchStatus is the line above the Homebrew search results: what the
// field searched for and what it found.
func BrewSearchStatus(searching bool, query string, ran bool, results int, failed bool) string {
	switch {
	case searching:
		return fmt.Sprintf("Searching formulae and casks for “%s”…", query)
	case !ran:
		return "Type a name and press Enter to search Homebrew's formulae and casks."
	case failed:
		return fmt.Sprintf("The search for “%s” failed.", query)
	case results == 0:
		return fmt.Sprintf("No formula or cask matches “%s”.", query)
	}
	return fmt.Sprintf("%s for “%s”", BrewCount(results, "result", "results"), query)
}

// BrewInstallCommand is the command an install from a search result runs,
// shown under the result so the user sees which namespace it installs from.
func BrewInstallCommand(name string, cask bool) string {
	if cask {
		return "brew install --cask " + name
	}
	return "brew install " + name
}
