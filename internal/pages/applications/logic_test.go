package applications

import "testing"

func TestPMCategory_Constants(t *testing.T) {
	// Verify constants have expected values
	if CategoryAll != "all" {
		t.Errorf("CategoryAll = %q, want %q", CategoryAll, "all")
	}
	if CategoryFlatpak != "flatpak" {
		t.Errorf("CategoryFlatpak = %q, want %q", CategoryFlatpak, "flatpak")
	}
	if CategoryHomebrew != "homebrew" {
		t.Errorf("CategoryHomebrew = %q, want %q", CategoryHomebrew, "homebrew")
	}
	if CategorySnap != "snap" {
		t.Errorf("CategorySnap = %q, want %q", CategorySnap, "snap")
	}
}

func TestPMCategory_TypeIsString(t *testing.T) {
	// Verify PMCategory can be used as string
	var cat PMCategory = "custom"
	if string(cat) != "custom" {
		t.Errorf("PMCategory string conversion failed: got %q", string(cat))
	}
}

func TestSidebarItem_Fields(t *testing.T) {
	// Verify SidebarItem struct fields are accessible
	item := SidebarItem{
		Category:    CategoryFlatpak,
		Label:       "Test Label",
		IconName:    "test-icon-symbolic",
		IsInstalled: true,
	}

	if item.Category != CategoryFlatpak {
		t.Errorf("Category = %q, want %q", item.Category, CategoryFlatpak)
	}
	if item.Label != "Test Label" {
		t.Errorf("Label = %q, want %q", item.Label, "Test Label")
	}
	if item.IconName != "test-icon-symbolic" {
		t.Errorf("IconName = %q, want %q", item.IconName, "test-icon-symbolic")
	}
	if !item.IsInstalled {
		t.Error("IsInstalled should be true")
	}
}

func TestSearchResult_Fields(t *testing.T) {
	// Verify SearchResult struct fields are accessible
	result := SearchResult{
		Name:        "test-package",
		Description: "A test package",
		PM:          CategoryHomebrew,
		IsInstalled: false,
	}

	if result.Name != "test-package" {
		t.Errorf("Name = %q, want %q", result.Name, "test-package")
	}
	if result.Description != "A test package" {
		t.Errorf("Description = %q, want %q", result.Description, "A test package")
	}
	if result.PM != CategoryHomebrew {
		t.Errorf("PM = %q, want %q", result.PM, CategoryHomebrew)
	}
	if result.IsInstalled {
		t.Error("IsInstalled should be false")
	}
}

func TestGetSidebarItems_ReturnsItems(t *testing.T) {
	items := GetSidebarItems()
	if len(items) == 0 {
		t.Error("GetSidebarItems() returned empty slice")
	}
}

func TestGetSidebarItems_FirstItemIsAll(t *testing.T) {
	items := GetSidebarItems()
	if len(items) == 0 {
		t.Fatal("GetSidebarItems() returned empty slice")
	}

	// First item should always be "All Applications"
	if items[0].Category != CategoryAll {
		t.Errorf("first item Category = %q, want %q", items[0].Category, CategoryAll)
	}
	if items[0].Label != "All Applications" {
		t.Errorf("first item Label = %q, want %q", items[0].Label, "All Applications")
	}
	// All Applications should always be "installed" (available)
	if !items[0].IsInstalled {
		t.Error("first item (All Applications) should have IsInstalled=true")
	}
}

func TestGetSidebarItems_ContainsExpectedCategories(t *testing.T) {
	items := GetSidebarItems()

	// Check that expected categories exist
	categories := make(map[PMCategory]bool)
	for _, item := range items {
		categories[item.Category] = true
	}

	expectedCategories := []PMCategory{CategoryAll, CategoryFlatpak, CategoryHomebrew, CategorySnap}
	for _, expected := range expectedCategories {
		if !categories[expected] {
			t.Errorf("expected category %q not found in sidebar items", expected)
		}
	}
}

func TestHasSearchCapability_ReturnsWithoutPanic(t *testing.T) {
	// Just verify it returns without panic - actual value depends on installed PMs
	result := HasSearchCapability()
	t.Logf("HasSearchCapability() returned %v", result)
}
