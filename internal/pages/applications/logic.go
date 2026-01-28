// Package applications provides the Applications page for managing installed packages
// across multiple package managers (Flatpak, Homebrew, Snap).
package applications

import "github.com/frostyard/chairlift/internal/pm"

// PMCategory represents a package manager category for sidebar navigation.
type PMCategory string

const (
	CategoryAll      PMCategory = "all"
	CategoryFlatpak  PMCategory = "flatpak"
	CategoryHomebrew PMCategory = "homebrew"
	CategorySnap     PMCategory = "snap"
)

// SidebarItem represents an item in the applications page sidebar.
type SidebarItem struct {
	Category    PMCategory
	Label       string
	IconName    string
	IsInstalled bool
}

// GetSidebarItems returns the list of sidebar items with their availability status.
func GetSidebarItems() []SidebarItem {
	return []SidebarItem{
		{CategoryAll, "All Applications", "package-x-generic-symbolic", true},
		{CategoryFlatpak, "Flatpak", "application-x-flatpak-symbolic", pm.FlatpakIsInstalled()},
		{CategoryHomebrew, "Homebrew", "package-x-generic-symbolic", pm.HomebrewIsInstalled()},
		{CategorySnap, "Snap", "package-x-generic-symbolic", pm.SnapIsInstalled()},
	}
}

// SearchResult represents a search result from any package manager.
type SearchResult struct {
	Name        string
	Description string
	PM          PMCategory
	IsInstalled bool
}

// SearchHomebrew searches Homebrew for packages matching the query.
func SearchHomebrew(query string) ([]SearchResult, error) {
	results, err := pm.HomebrewSearch(query)
	if err != nil {
		return nil, err
	}
	var searchResults []SearchResult
	for _, r := range results {
		searchResults = append(searchResults, SearchResult{
			Name:        r.Name,
			Description: r.Description,
			PM:          CategoryHomebrew,
		})
	}
	return searchResults, nil
}

// HasSearchCapability returns true if any package manager with search is installed.
func HasSearchCapability() bool {
	return pm.HomebrewIsInstalled()
}
