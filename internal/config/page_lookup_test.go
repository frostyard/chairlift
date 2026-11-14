package config

import "testing"

// pageLookupConfig gives each of the 8 pages 1 group whose name and AppID
// are unique to that page, so a lookup that maps a page name to the wrong
// field fails.
func pageLookupConfig() (*Config, []string) {
	pages := []string{
		"system_page",
		"updates_page",
		"applications_page",
		"maintenance_page",
		"features_page",
		"problems_page",
		"help_page",
		"first_run_page",
	}
	group := func(page string) PageConfig {
		return PageConfig{
			page + "_on":  {Enabled: true, AppID: page},
			page + "_off": {Enabled: false, AppID: page + ".off"},
		}
	}
	c := &Config{
		SystemPage:       group("system_page"),
		UpdatesPage:      group("updates_page"),
		ApplicationsPage: group("applications_page"),
		MaintenancePage:  group("maintenance_page"),
		FeaturesPage:     group("features_page"),
		ProblemsPage:     group("problems_page"),
		HelpPage:         group("help_page"),
		FirstRunPage:     group("first_run_page"),
	}
	return c, pages
}

func TestPageLookupIsGroupEnabled(t *testing.T) {
	c, pages := pageLookupConfig()
	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			if !c.IsGroupEnabled(page, page+"_on") {
				t.Errorf("IsGroupEnabled(%q, %q) = false, want true", page, page+"_on")
			}
			if c.IsGroupEnabled(page, page+"_off") {
				t.Errorf("IsGroupEnabled(%q, %q) = true, want false", page, page+"_off")
			}
			if !c.IsGroupEnabled(page, "no_such_group") {
				t.Errorf("IsGroupEnabled(%q, missing group) = false, want true", page)
			}
		})
	}
	if !c.IsGroupEnabled("no_such_page", "system_page_off") {
		t.Error("IsGroupEnabled(unknown page) = false, want true")
	}
}

func TestPageLookupGetGroupConfig(t *testing.T) {
	c, pages := pageLookupConfig()
	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			got := c.GetGroupConfig(page, page+"_off")
			if got == nil {
				t.Fatalf("GetGroupConfig(%q, %q) = nil", page, page+"_off")
			}
			if got.Enabled || got.AppID != page+".off" {
				t.Errorf("GetGroupConfig(%q, %q) = %+v, want that page's group", page, page+"_off", *got)
			}
			// The result is a copy: a change to it must not reach the map.
			got.AppID = "changed"
			if again := c.GetGroupConfig(page, page+"_off"); again.AppID != page+".off" {
				t.Errorf("GetGroupConfig(%q) returned map storage, not a copy", page)
			}
			if g := c.GetGroupConfig(page, "no_such_group"); g != nil {
				t.Errorf("GetGroupConfig(%q, missing group) = %+v, want nil", page, *g)
			}
		})
	}
	if g := c.GetGroupConfig("no_such_page", "system_page_off"); g != nil {
		t.Errorf("GetGroupConfig(unknown page) = %+v, want nil", *g)
	}
}
