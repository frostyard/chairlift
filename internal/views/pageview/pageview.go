// Package pageview derives widget-independent page content for the views package.
package pageview

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

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

// SysupdateUpdateSubtitle returns the native A/B system-update expander
// subtitle from the /run/snosi state-file presentation (the outcome grammar
// is internal/sysupdate.Status.Presentation's): "staged" shows the pending
// version, "current" shows the last check time, "failed" prompts a retry,
// and anything else — including the fresh-boot no-files state — is the
// neutral idle prompt.
func SysupdateUpdateSubtitle(outcome, version, checkedAt string) string {
	switch outcome {
	case "staged":
		if version == "" {
			return "Update staged — restart to apply"
		}
		return fmt.Sprintf("Update %s staged — restart to apply", version)
	case "current":
		if formatted := formatCheckedAt(checkedAt); formatted != "" {
			return fmt.Sprintf("System is up to date (checked %s)", formatted)
		}
		return "System is up to date"
	case "failed":
		return "Last update check failed — use Check for Updates to retry"
	default:
		return "Check for and download the latest system image"
	}
}

// formatCheckedAt renders a stager ISO-8601 timestamp as a local wall-clock
// time, or "" when unparseable.
func formatCheckedAt(checkedAt string) string {
	parsed, err := time.Parse(time.RFC3339, checkedAt)
	if err != nil {
		return ""
	}
	return parsed.Local().Format("15:04")
}

// SysupdateStageResultSubtitle returns the subtitle after a native A/B
// staging action completes.
func SysupdateStageResultSubtitle(staged bool, version, lastMessage string) string {
	if staged {
		return SysupdateUpdateSubtitle("staged", version, "")
	}
	if lastMessage != "" {
		return lastMessage
	}
	return "System is up to date"
}

// SysupdateRollbackSubtitle returns the read-only rollback row subtitle.
// version is the inactive slot's version only when it is older than the
// running one (internal/sysupdate.RollbackCandidate); a staged-but-newer
// slot or an empty slot both present as no rollback.
func SysupdateRollbackSubtitle(version string) string {
	if version == "" {
		return "No previous version on disk"
	}
	return fmt.Sprintf("Version %s is on the inactive slot — choose it in the boot menu at restart to roll back", version)
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

// Status is display text plus whether its container may expand.
type Status struct {
	Subtitle   string
	Expandable bool
}

// FirmwareUpdate returns the row text for an available firmware update: the
// device name (or "Unknown device"), and a subtitle built from the vendor and
// the version transition. Empty parts are omitted rather than left as gaps.
func FirmwareUpdate(name, vendor, currentVersion, newVersion string) Row {
	title := name
	if title == "" {
		title = "Unknown device"
	}

	parts := make([]string, 0, 2)
	if vendor != "" {
		parts = append(parts, vendor)
	}
	if versions := versionTransition(currentVersion, newVersion); versions != "" {
		parts = append(parts, versions)
	}

	return Row{Title: title, Subtitle: strings.Join(parts, " · ")}
}

// FirmwareBlockedUpdate returns the row text for a device whose update cannot
// be installed now: the same text FirmwareUpdate gives, plus the reason fwupd
// reports. The row has no install control, so the reason must be visible.
func FirmwareBlockedUpdate(name, vendor, currentVersion, newVersion, reason string) Row {
	row := FirmwareUpdate(name, vendor, currentVersion, newVersion)

	parts := make([]string, 0, 2)
	if row.Subtitle != "" {
		parts = append(parts, row.Subtitle)
	}
	if reason != "" {
		parts = append(parts, reason)
	}

	return Row{Title: row.Title, Subtitle: strings.Join(parts, " · ")}
}

// versionTransition renders the version change, keeping only the parts that
// are known.
func versionTransition(currentVersion, newVersion string) string {
	switch {
	case currentVersion != "" && newVersion != "":
		return fmt.Sprintf("%s → %s", currentVersion, newVersion)
	case newVersion != "":
		return newVersion
	default:
		return currentVersion
	}
}

// FirmwareUpdatesSubtitle returns the firmware expander's subtitle and whether
// it can expand. A failed query never claims the firmware is up to date; when
// the view still holds rows from an earlier load (retainedRows), those rows
// stay reachable, so a failed refresh does not hide devices the user can act
// on.
func FirmwareUpdatesSubtitle(count int, err error, retainedRows int) Status {
	if err != nil {
		return Status{
			Subtitle:   fmt.Sprintf("Error checking firmware updates: %v", err),
			Expandable: retainedRows > 0,
		}
	}
	return Status{
		Subtitle:   firmwareAvailableText(count),
		Expandable: count > 0,
	}
}

// firmwareAvailableText formats the firmware update count.
func firmwareAvailableText(count int) string {
	switch count {
	case 0:
		return "Firmware is up to date"
	case 1:
		return "1 firmware update available"
	default:
		return fmt.Sprintf("%d firmware updates available", count)
	}
}

// FirmwareInstallConfirmation returns the heading and body of the dialog shown
// before one device's firmware update is installed. The body always carries
// the power warning: an interrupted firmware write can leave a device
// unusable. The completion sentence says what the device needs afterwards, and
// the last sentence states that ChairLift itself never restarts the computer.
func FirmwareInstallConfirmation(name, newVersion string, needsReboot, needsShutdown bool) (string, string) {
	device := name
	if device == "" {
		device = "Unknown device"
	}

	sentences := make([]string, 0, 4)
	if newVersion != "" {
		sentences = append(sentences, fmt.Sprintf("Version %s will be installed.", newVersion))
	}
	sentences = append(sentences, "Do not turn off or unplug the computer during the update. Keep the power connected.")
	switch {
	case needsReboot:
		sentences = append(sentences, "The computer must restart to finish the update.")
	case needsShutdown:
		sentences = append(sentences, "The computer must shut down to finish the update.")
	default:
		sentences = append(sentences, "Some devices restart or disconnect while they update.")
	}
	sentences = append(sentences, "ChairLift will not restart the computer for you.")

	return fmt.Sprintf("Install firmware for %s?", device), strings.Join(sentences, " ")
}
