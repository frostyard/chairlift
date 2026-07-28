package navigation

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestResolveCoversEveryNavigationMutation(t *testing.T) {
	items := Items()
	for index, item := range items {
		t.Run(item.Name, func(t *testing.T) {
			got, ok := Resolve(item.Name, items, func(name string) bool {
				return name == item.Name
			})
			if !ok {
				t.Fatalf("Resolve(%q) rejected an available canonical page", item.Name)
			}
			want := Transition{
				SelectedIndex: index,
				VisibleChild:  item.Name,
				Title:         item.Title,
				ShowContent:   true,
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Resolve(%q) = %#v, want %#v", item.Name, got, want)
			}
		})
	}
}

func TestResolveRejectsUnavailableAndUnknownPages(t *testing.T) {
	tests := []struct {
		name      string
		page      string
		available func(string) bool
	}{
		{name: "unavailable", page: "help", available: func(string) bool { return false }},
		{name: "unknown", page: "not-a-page", available: func(string) bool { return true }},
		{name: "nil availability predicate", page: "help"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if transition, ok := Resolve(tt.page, Items(), tt.available); ok {
				t.Fatalf("Resolve(%q) = %#v, true; want rejection", tt.page, transition)
			}
		})
	}
}

func TestShortcutsRegisterEveryAdvertisedAccelerator(t *testing.T) {
	items := Items()
	advertised := make(map[string]Shortcut)
	for _, shortcut := range Shortcuts(items) {
		key := shortcut.Action + "\x00" + shortcut.Accelerator
		if _, exists := advertised[key]; exists {
			t.Fatalf("duplicate shortcut for action %q accelerator %q", shortcut.Action, shortcut.Accelerator)
		}
		advertised[key] = shortcut
	}

	registered := make(map[string]bool)
	for _, binding := range Bindings(items) {
		for _, accelerator := range binding.Accelerators {
			registered[binding.Action+"\x00"+accelerator] = true
		}
	}
	if len(registered) != len(advertised) {
		t.Fatalf("registered shortcut count = %d, advertised count = %d", len(registered), len(advertised))
	}
	for key, shortcut := range advertised {
		if !registered[key] {
			t.Errorf("advertised shortcut %s (%s) is not registered", shortcut.Display, shortcut.Title)
		}
	}
	for _, item := range items {
		key := "win.navigate-" + item.Name + "\x00" + item.Accelerator
		shortcut, ok := advertised[key]
		if !ok {
			t.Errorf("page %q has no advertised navigation shortcut", item.Name)
			continue
		}
		if shortcut.Display != item.Display ||
			shortcut.Title != "Go to "+item.Title ||
			shortcut.Group != GroupNavigation {
			t.Errorf("page %q shortcut = %#v, want canonical item metadata", item.Name, shortcut)
		}
	}

	help, ok := advertised["win.navigate-help\x00F1"]
	if !ok {
		t.Fatal("F1 is not mapped to the Help navigation action")
	}
	if help.Display != "F1" || help.Title != "Help" || help.Group != GroupGeneral {
		t.Fatalf("F1 shortcut = %#v, want advertised General/Help entry", help)
	}
}

func TestReturnedInventoriesCannotMutateCanonicalState(t *testing.T) {
	gotItems := Items()
	gotItems[0].Name = "changed"
	gotItems[0].Groups[0] = "changed"
	if Items()[0].Name == "changed" {
		t.Fatal("Items returned mutable canonical storage")
	}
	if Items()[0].Groups[0] == "changed" {
		t.Fatal("Items returned mutable canonical group storage")
	}

	gotShortcuts := Shortcuts(Items())
	gotShortcuts[0].Action = "changed"
	if Shortcuts(Items())[0].Action == "changed" {
		t.Fatal("Shortcuts returned mutable canonical storage")
	}

	gotBindings := Bindings(Items())
	gotBindings[0].Accelerators[0] = "changed"
	if Bindings(Items())[0].Accelerators[0] == "changed" {
		t.Fatal("Bindings returned mutable canonical storage")
	}
}

func TestVisiblePagesOmitEveryFullyDisabledFunctionalPage(t *testing.T) {
	for _, disabled := range Items() {
		if disabled.AlwaysShow {
			continue
		}
		t.Run(disabled.Name, func(t *testing.T) {
			got := VisibleItems(func(page, group string) bool {
				return page != disabled.ConfigPage
			})
			if containsPage(got, disabled.Name) {
				t.Fatalf("VisibleItems retained %q when all of its groups were disabled", disabled.Name)
			}
			if !containsPage(got, "help") {
				t.Fatal("VisibleItems omitted Help")
			}
		})
	}
}

func TestPageMetadataCoversEveryBuilderBackedGroup(t *testing.T) {
	want := map[string][]string{
		"applications": {
			"applications_installed_group",
			"flatpak_user_group",
			"flatpak_system_group",
			"brew_group",
			"brew_search_group",
			"brew_bundles_group",
		},
		"maintenance": {
			"maintenance_cleanup_group",
			"maintenance_brew_group",
			"maintenance_flatpak_group",
			"maintenance_optimization_group",
		},
		"updates": {
			"bootc_updates_group",
			"flatpak_updates_group",
			"brew_updates_group",
			"brew_trust_group",
		},
		"system": {
			"system_info_group",
			"bootc_status_group",
			"health_group",
		},
		"features": {"features_group"},
		"help":     {"help_resources_group"},
	}

	items := Items()
	if len(items) != len(want) {
		t.Fatalf("Items() has %d pages, want %d", len(items), len(want))
	}
	for _, item := range items {
		wantGroups, ok := want[item.Name]
		if !ok {
			t.Errorf("unexpected page metadata %q", item.Name)
			continue
		}
		if !reflect.DeepEqual(item.Groups, wantGroups) {
			t.Errorf("%s groups = %v, want %v", item.Name, item.Groups, wantGroups)
		}
	}
}

func TestVisiblePagesKeepEachBuilderBackedGroup(t *testing.T) {
	for _, page := range Items() {
		if page.AlwaysShow {
			continue
		}
		for _, enabledGroup := range page.Groups {
			t.Run(page.Name+"/"+enabledGroup, func(t *testing.T) {
				got := VisibleItems(func(configPage, group string) bool {
					return configPage == page.ConfigPage && group == enabledGroup
				})
				if !containsPage(got, page.Name) {
					t.Fatalf("VisibleItems omitted %q when %q was enabled", page.Name, enabledGroup)
				}
			})
		}
	}
}

func TestVisiblePagesAlwaysKeepHelp(t *testing.T) {
	got := VisibleItems(func(page, group string) bool { return false })
	if len(got) != 1 || got[0].Name != "help" {
		t.Fatalf("VisibleItems(all disabled) = %#v, want Help only", got)
	}
	if got[0].Accelerator != "<Alt>1" || got[0].Display != "Alt+1" {
		t.Fatalf("Help shortcut = %q/%q, want compacted Alt+1", got[0].Accelerator, got[0].Display)
	}
}

func TestVisiblePagesCompactShortcutsAndTransitions(t *testing.T) {
	visible := VisibleItems(func(page, group string) bool {
		return page == "updates_page" || page == "features_page"
	})
	wantNames := []string{"updates", "features", "help"}
	if len(visible) != len(wantNames) {
		t.Fatalf("VisibleItems selected %d pages, want %d: %#v", len(visible), len(wantNames), visible)
	}
	for index, wantName := range wantNames {
		if visible[index].Name != wantName {
			t.Fatalf("visible[%d].Name = %q, want %q", index, visible[index].Name, wantName)
		}
		wantAccelerator := "<Alt>" + strconv.Itoa(index+1)
		if visible[index].Accelerator != wantAccelerator {
			t.Errorf("visible[%d].Accelerator = %q, want %q", index, visible[index].Accelerator, wantAccelerator)
		}
		transition, ok := Resolve(wantName, visible, func(string) bool { return true })
		if !ok {
			t.Fatalf("Resolve(%q) rejected a visible page", wantName)
		}
		if transition.SelectedIndex != index {
			t.Errorf("Resolve(%q).SelectedIndex = %d, want %d", wantName, transition.SelectedIndex, index)
		}
	}

	shortcuts := Shortcuts(visible)
	for index, item := range visible {
		shortcut := shortcuts[index]
		if shortcut.Action != "win.navigate-"+item.Name ||
			shortcut.Accelerator != item.Accelerator ||
			shortcut.Display != item.Display {
			t.Errorf("shortcut[%d] = %#v, want compacted metadata for %#v", index, shortcut, item)
		}
	}

	if transition, ok := Resolve("maintenance", visible, func(string) bool { return true }); ok {
		t.Fatalf("Resolve accepted omitted Maintenance page: %#v", transition)
	}

	altBindings := make(map[string]string)
	for _, binding := range Bindings(visible) {
		if strings.HasPrefix(binding.Action, "win.navigate-") {
			for _, accelerator := range binding.Accelerators {
				if strings.HasPrefix(accelerator, "<Alt>") {
					altBindings[accelerator] = binding.Action
				}
			}
		}
	}
	if len(altBindings) != len(visible) {
		t.Fatalf("Alt binding count = %d, want %d: %v", len(altBindings), len(visible), altBindings)
	}
	for _, item := range visible {
		if got := altBindings[item.Accelerator]; got != "win.navigate-"+item.Name {
			t.Errorf("%s is bound to %q, want navigate action for %q", item.Accelerator, got, item.Name)
		}
	}
}

func TestWindowAndAppUseCanonicalNavigation(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate navigation_test.go")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	checks := map[string][]string{
		filepath.Join(repoRoot, "internal", "window", "window.go"): {
			`w.navigateToPage(name)`,
			`w.navItems = navigation.VisibleItems(w.config.IsGroupEnabled)`,
			`transition, ok := navigation.Resolve(pageName, w.navItems, func(name string) bool {`,
			`w.sidebarList.GetRowAtIndex(int32(transition.SelectedIndex))`,
			`w.contentStack.SetVisibleChildName(transition.VisibleChild)`,
			`w.contentPage.SetTitle(transition.Title)`,
			`w.splitView.SetShowContent(transition.ShowContent)`,
			`action := gio.NewSimpleAction("navigate-"+itemName, nil)`,
			`for _, shortcut := range navigation.Shortcuts(w.navItems)`,
		},
		filepath.Join(repoRoot, "internal", "app", "app.go"): {
			`a.setupKeyboardShortcuts(win.NavigationItems())`,
			`for _, binding := range navigation.Bindings(items)`,
			`a.SetAccelsForAction(binding.Action, binding.Accelerators)`,
		},
	}

	for path, required := range checks {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, fragment := range required {
			if !strings.Contains(string(source), fragment) {
				t.Errorf("%s does not contain canonical navigation wiring %q", path, fragment)
			}
		}
	}
}

func containsPage(items []Item, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
