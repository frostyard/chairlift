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
