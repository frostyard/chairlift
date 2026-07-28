package navigation

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestResolveCoversEveryNavigationMutation(t *testing.T) {
	for index, item := range Items() {
		t.Run(item.Name, func(t *testing.T) {
			got, ok := Resolve(item.Name, func(name string) bool {
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
			if transition, ok := Resolve(tt.page, tt.available); ok {
				t.Fatalf("Resolve(%q) = %#v, true; want rejection", tt.page, transition)
			}
		})
	}
}

func TestShortcutsRegisterEveryAdvertisedAccelerator(t *testing.T) {
	advertised := make(map[string]Shortcut)
	for _, shortcut := range Shortcuts() {
		key := shortcut.Action + "\x00" + shortcut.Accelerator
		if _, exists := advertised[key]; exists {
			t.Fatalf("duplicate shortcut for action %q accelerator %q", shortcut.Action, shortcut.Accelerator)
		}
		advertised[key] = shortcut
	}

	registered := make(map[string]bool)
	for _, binding := range Bindings() {
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
	for _, item := range Items() {
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
	if Items()[0].Name == "changed" {
		t.Fatal("Items returned mutable canonical storage")
	}

	gotShortcuts := Shortcuts()
	gotShortcuts[0].Action = "changed"
	if Shortcuts()[0].Action == "changed" {
		t.Fatal("Shortcuts returned mutable canonical storage")
	}

	gotBindings := Bindings()
	gotBindings[0].Accelerators[0] = "changed"
	if Bindings()[0].Accelerators[0] == "changed" {
		t.Fatal("Bindings returned mutable canonical storage")
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
			`transition, ok := navigation.Resolve(pageName, func(name string) bool {`,
			`w.sidebarList.GetRowAtIndex(int32(transition.SelectedIndex))`,
			`w.contentStack.SetVisibleChildName(transition.VisibleChild)`,
			`w.contentPage.SetTitle(transition.Title)`,
			`w.splitView.SetShowContent(transition.ShowContent)`,
			`action := gio.NewSimpleAction("navigate-"+itemName, nil)`,
			`for _, shortcut := range navigation.Shortcuts()`,
		},
		filepath.Join(repoRoot, "internal", "app", "app.go"): {
			`for _, binding := range navigation.Bindings()`,
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
