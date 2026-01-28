// Package system implements the System page showing system info, NBC status, and health links.
package system

import (
	"bufio"
	"context"
	"os"
	"strings"

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
