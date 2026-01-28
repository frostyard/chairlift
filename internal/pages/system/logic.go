// Package system implements the System page showing system info, NBC status, and health links.
package system

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/frostyard/chairlift/internal/nbc"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// OSReleaseEntry represents a parsed line from /etc/os-release.
type OSReleaseEntry struct {
	Key        string // Raw key (e.g., "PRETTY_NAME")
	Value      string // Parsed value
	DisplayKey string // Human-readable key (e.g., "Pretty Name")
	IsURL      bool   // True if key ends with "URL"
}

// ParseOSRelease reads and parses /etc/os-release.
// This is pure Go logic, testable without GTK.
func ParseOSRelease() ([]OSReleaseEntry, error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var entries []OSReleaseEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		key := parts[0]
		value := strings.Trim(parts[1], "\"'")

		entries = append(entries, OSReleaseEntry{
			Key:        key,
			Value:      value,
			DisplayKey: formatKey(key),
			IsURL:      strings.HasSuffix(key, "URL"),
		})
	}

	return entries, scanner.Err()
}

// formatKey converts a key like "PRETTY_NAME" to "Pretty Name".
func formatKey(key string) string {
	readable := strings.ReplaceAll(key, "_", " ")
	return cases.Title(language.English).String(strings.ToLower(readable))
}

// IsNBCAvailable checks if the system is running on NBC.
func IsNBCAvailable() bool {
	_, err := os.Stat("/run/nbc-booted")
	return err == nil
}

// FetchNBCStatus fetches NBC status using the provided context.
// This wraps nbc.GetStatus for testability (can mock nbc package).
func FetchNBCStatus(ctx context.Context) (*nbc.StatusOutput, error) {
	return nbc.GetStatus(ctx)
}

// ManifestPackage represents a package entry in the manifest.
type ManifestPackage struct {
	Type         string `json:"type"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Architecture string `json:"architecture"`
}

// ManifestConfig represents the configuration section of the manifest.
type ManifestConfig struct {
	Name         string `json:"name"`
	Distribution string `json:"distribution"`
	Architecture string `json:"architecture"`
	Release      string `json:"release"`
	Version      string `json:"version"`
}

// Manifest represents the package manifest JSON structure.
type Manifest struct {
	ManifestVersion int               `json:"manifest_version"`
	Config          ManifestConfig    `json:"config"`
	Packages        []ManifestPackage `json:"packages"`
}

// GetOSReleaseValue extracts a specific value from os-release entries.
func GetOSReleaseValue(entries []OSReleaseEntry, key string) string {
	for _, entry := range entries {
		if entry.Key == key {
			return entry.Value
		}
	}
	return ""
}

// FetchManifest fetches the package manifest from the repository.
// Returns nil, nil if the manifest is not found (404 response).
func FetchManifest(ctx context.Context, imageID, imageVersion string) (*Manifest, error) {
	if imageID == "" || imageVersion == "" {
		return nil, nil
	}

	url := fmt.Sprintf("https://repository.frostyard.org/manifests/%s/%s.%s.manifest.json",
		imageID, imageID, imageVersion)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decoding manifest: %w", err)
	}

	return &manifest, nil
}
