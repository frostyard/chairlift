package navigation

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/frostyard/chairlift/internal/config"
)

func TestCanonicalPageOrderIsTheCommandCenter(t *testing.T) {
	var got []string
	for _, item := range Items() {
		got = append(got, item.Name)
	}
	want := []string{Overview, Updates, Apps, Features, Problems, Maintenance}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("page order = %v, want %v", got, want)
	}
}

func TestResolveFindsOnlyVisiblePages(t *testing.T) {
	visible := VisibleItems(func(page, group string) bool { return page == "features_page" })
	if item, ok := Resolve(Features, visible); !ok || item.Title != "Features" {
		t.Fatalf("Resolve(features) = %+v, %v", item, ok)
	}
	for _, page := range []string{Updates, "help", "not-a-page", ""} {
		if item, ok := Resolve(page, visible); ok {
			t.Errorf("Resolve(%q) = %+v, want rejection", page, item)
		}
	}
}

func TestOverviewAlwaysShows(t *testing.T) {
	got := VisibleItems(func(page, group string) bool { return false })
	if len(got) != 1 || got[0].Name != Overview {
		t.Fatalf("VisibleItems(all disabled) = %#v, want the Overview only", got)
	}
	if got[0].Display() != "Alt+1" {
		t.Errorf("Overview keys = %q, want Alt+1", got[0].Display())
	}
	if got := VisibleItems(nil); len(got) != 1 {
		t.Errorf("VisibleItems(nil) = %v", got)
	}
}

func TestVisiblePagesKeepEachGroup(t *testing.T) {
	for _, page := range Items() {
		for _, enabledGroup := range page.Groups {
			got := VisibleItems(func(configPage, group string) bool {
				return configPage == page.ConfigPage && group == enabledGroup
			})
			found := false
			for _, item := range got {
				found = found || item.Name == page.Name
			}
			if !found {
				t.Errorf("VisibleItems omitted %q when %q was enabled", page.Name, enabledGroup)
			}
		}
	}
}

func TestVisiblePagesCompactShortcuts(t *testing.T) {
	got := VisibleItems(func(page, group string) bool {
		return page == "problems_page" || page == "maintenance_page"
	})
	want := map[string]string{Overview: "Alt+1", Problems: "Alt+2", Maintenance: "Alt+3"}
	if len(got) != len(want) {
		t.Fatalf("visible = %v", got)
	}
	for _, item := range got {
		if item.Display() != want[item.Name] {
			t.Errorf("%s keys = %q, want %q", item.Name, item.Display(), want[item.Name])
		}
	}
}

func TestShortcutsAreUniqueAndCoverEveryPage(t *testing.T) {
	items := Items()
	shortcuts := Shortcuts(items)
	seenKeys := map[string]string{}
	seenActions := map[string]bool{}
	for _, s := range shortcuts {
		if len(s.Keys) == 0 || s.Title == "" || s.Group == "" {
			t.Errorf("incomplete shortcut %+v", s)
		}
		if other, ok := seenKeys[s.Display()]; ok {
			t.Errorf("%s is bound to %s and %s", s.Display(), other, s.Action)
		}
		seenKeys[s.Display()] = s.Action
		seenActions[s.Action] = true
	}
	for _, item := range items {
		if !seenActions[item.Name] {
			t.Errorf("page %s has no shortcut", item.Name)
		}
	}
	for _, action := range []string{ActionPalette, ActionConsole, ActionCheck, ActionShortcuts, ActionHelp, ActionQuit} {
		if !seenActions[action] {
			t.Errorf("general action %s has no shortcut", action)
		}
	}
}

func TestReturnedInventoriesCannotMutateCanonicalState(t *testing.T) {
	first := Items()
	first[0].Groups[0] = "mutated"
	first[0].Keys[0] = "mutated"
	shortcuts := Shortcuts(first)
	shortcuts[len(shortcuts)-1].Keys[0] = "mutated"
	again := Items()
	if again[0].Groups[0] == "mutated" || again[0].Keys[0] == "mutated" {
		t.Fatal("Items returned shared slices")
	}
	if s := Shortcuts(again); s[len(s)-1].Keys[0] == "mutated" {
		t.Fatal("Shortcuts returned shared key slices")
	}
}

// TestEveryConfiguredGroupHasOneHome checks that each group of the
// configuration's pages, other than first-run and help, belongs to exactly
// one navigation page, so no group is silently ignored.
func TestEveryConfiguredGroupHasOneHome(t *testing.T) {
	pages, err := config.SchemaPages()
	if err != nil {
		t.Fatal(err)
	}
	homes := map[string]int{}
	for _, item := range Items() {
		for _, g := range item.Groups {
			homes[item.ConfigPage+"/"+g]++
		}
	}
	for _, page := range pages {
		groups, err := config.SchemaGroups(page)
		if err != nil {
			t.Fatal(err)
		}
		for _, group := range groups {
			switch page {
			case HelpConfigPage, "first_run_page":
				if homes[page+"/"+group] != 0 {
					t.Errorf("group %s/%s is not a page's but a page claims it", page, group)
				}
				continue
			}
			if homes[page+"/"+group] != 1 {
				t.Errorf("group %s/%s belongs to %d pages, want 1", page, group, homes[page+"/"+group])
			}
		}
	}
	helpGroups, _ := config.SchemaGroups(HelpConfigPage)
	found := false
	for _, g := range helpGroups {
		found = found || g == HelpGroup
	}
	if !found {
		t.Errorf("help group %s missing from the configuration schema", HelpGroup)
	}
}

// TestViewsGateEveryGroup checks that the views name every group of their
// pages, so each group's content is behind its own switch.
func TestViewsGateEveryGroup(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Join(filepath.Dir(filename), "..", "views")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var source strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		source.Write(data)
	}
	for _, item := range Items() {
		for _, g := range item.Groups {
			if !strings.Contains(source.String(), `"`+g+`"`) {
				t.Errorf("no view names group %q of page %s", g, item.Name)
			}
		}
	}
}
