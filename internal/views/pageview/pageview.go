// Package pageview derives widget-independent page content for the views package.
package pageview

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Row is the text displayed by a view row.
type Row struct {
	Title    string
	Subtitle string
}

// HelpResource is one configured link on the Help page.
type HelpResource struct {
	Title string
	URL   string
}

// Command is the executable and arguments for a configured maintenance action.
type Command struct {
	Name string
	Args []string
}

// OSReleaseEntry is one parsed field from an os-release file.
type OSReleaseEntry struct {
	Title string
	Value string
	IsURL bool
}

// FlatpakApplication returns the row text for an installed Flatpak application.
func FlatpakApplication(name, applicationID, version string) Row {
	subtitle := applicationID
	if version != "" {
		subtitle = fmt.Sprintf("%s (%s)", applicationID, version)
	}
	return Row{Title: name, Subtitle: subtitle}
}

// HomebrewPackage returns the row text for an installed Homebrew package.
func HomebrewPackage(name, version string, pinned bool) Row {
	subtitle := version
	if pinned {
		subtitle += " • Pinned"
	}
	return Row{Title: name, Subtitle: subtitle}
}

// BrewBundle returns the row text for a configured Homebrew bundle.
func BrewBundle(name, description, path string) Row {
	subtitle := path
	if description != "" {
		subtitle = fmt.Sprintf("%s — %s", description, path)
	}
	return Row{Title: name, Subtitle: subtitle}
}

// SearchResult returns the row text for a Homebrew search result.
func SearchResult(name, kind string) Row {
	return Row{Title: name, Subtitle: kind}
}

// UntrustedTap returns the row text for an untrusted Homebrew tap.
func UntrustedTap(name string, formulae, casks []string) Row {
	packages := make([]string, 0, len(formulae)+len(casks))
	for _, names := range [][]string{formulae, casks} {
		for _, packageName := range names {
			if i := strings.LastIndex(packageName, "/"); i >= 0 {
				packageName = packageName[i+1:]
			}
			packages = append(packages, packageName)
		}
	}
	return Row{
		Title:    name,
		Subtitle: fmt.Sprintf("%d installed: %s", len(packages), strings.Join(packages, ", ")),
	}
}

// FlatpakUpdate returns the row text for an available Flatpak update.
func FlatpakUpdate(name, applicationID, newVersion, installation string) Row {
	subtitle := applicationID
	if newVersion != "" {
		subtitle = fmt.Sprintf("%s → %s", applicationID, newVersion)
	}
	if installation == "user" {
		subtitle += " (user)"
	}
	return Row{Title: name, Subtitle: subtitle}
}

// BootcUpdateSubtitle returns the system-update expander subtitle.
func BootcUpdateSubtitle(staged bool, version string) string {
	if !staged {
		return "Check for and download the latest system image"
	}
	if version == "" {
		return "Update staged — restart to apply"
	}
	return fmt.Sprintf("Update %s staged — restart to apply", version)
}

// BootcStageResultSubtitle returns the subtitle after a staging action completes.
func BootcStageResultSubtitle(staged bool, version, lastMessage string) string {
	if staged {
		return BootcUpdateSubtitle(true, version)
	}
	if lastMessage != "" {
		return lastMessage
	}
	return "System is up to date"
}

// Feature returns the initial row text for an updex feature.
func Feature(name, description string) Row {
	return Row{Title: description, Subtitle: name}
}

// FeatureGroupDescription returns the description after features are loaded.
func FeatureGroupDescription(count int) string {
	return fmt.Sprintf("%d features available", count)
}

// HelpResources returns configured Help links in their display order.
func HelpResources(website, issues, chat string) []HelpResource {
	candidates := []HelpResource{
		{Title: "Website", URL: website},
		{Title: "Report Issues", URL: issues},
		{Title: "Community Discussions", URL: chat},
	}
	resources := make([]HelpResource, 0, len(candidates))
	for _, resource := range candidates {
		if resource.URL != "" {
			resources = append(resources, resource)
		}
	}
	return resources
}

// MaintenanceCommand returns the invocation for a configured maintenance action.
func MaintenanceCommand(script string, sudo bool) Command {
	if sudo {
		return Command{Name: "pkexec", Args: []string{script}}
	}
	return Command{Name: script}
}

// ParseOSRelease parses displayable fields from an os-release stream.
func ParseOSRelease(reader io.Reader) ([]OSReleaseEntry, error) {
	var entries []OSReleaseEntry
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		key := parts[0]
		value := strings.Trim(parts[1], "\"'")
		readableKey := strings.ReplaceAll(key, "_", " ")
		readableKey = cases.Title(language.English).String(strings.ToLower(readableKey))

		entries = append(entries, OSReleaseEntry{
			Title: readableKey,
			Value: value,
			IsURL: strings.HasSuffix(key, "URL"),
		})
	}
	return entries, scanner.Err()
}

// ShortDigest returns a compact bootc digest for display.
func ShortDigest(digest string) string {
	if len(digest) > 19 {
		return digest[:19] + "..."
	}
	return digest
}
