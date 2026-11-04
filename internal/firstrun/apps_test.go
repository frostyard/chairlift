package firstrun

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/i18n"
)

// firstSetupApps is first-setup's apps.json at b5f7de3, the list snosi ships
// as /usr/share/snow/first-run/apps.json.
const firstSetupApps = "testdata/first-setup-apps.json"

func TestParseAppsAcceptsFirstSetupList(t *testing.T) {
	list, err := LoadApps(firstSetupApps)
	if err != nil {
		t.Fatalf("LoadApps(%s) error = %v", firstSetupApps, err)
	}
	var keys []string
	counts := map[string]int{}
	for _, c := range list.Categories {
		keys = append(keys, c.Key)
		counts[c.Key] = len(c.Apps)
	}
	if want := []string{"browsers", "utilities", "office"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("category order = %v, want %v", keys, want)
	}
	if want := map[string]int{"browsers": 6, "utilities": 10, "office": 2}; !reflect.DeepEqual(counts, want) {
		t.Fatalf("category sizes = %v, want %v", counts, want)
	}
	browsers := list.Categories[0]
	if browsers.Title != i18n.MsgBrowsers || browsers.Subtitle != i18n.MsgBrowsersDesc {
		t.Errorf("browsers texts = %q/%q", browsers.Title, browsers.Subtitle)
	}
	if got := browsers.Apps[0]; got != (App{Name: "Firefox", ID: "org.mozilla.firefox", Active: true}) {
		t.Errorf("first browser = %+v, want Firefox active (absent active means true)", got)
	}
	if got := browsers.Apps[1]; got.ID != "com.google.Chrome" || got.Active {
		t.Errorf("second browser = %+v, want Chrome inactive", got)
	}
	if got := list.Categories[1].Apps[1].Name; got != "Déjà Dup Backups" {
		t.Errorf("JSON unicode escape decoded to %q", got)
	}
}

func TestParseAppsCategoryOrderAndTitles(t *testing.T) {
	data := `{"zeta": [{"name": "Z", "id": "org.example.Zeta"}],
	"office": [{"name": "O", "id": "org.example.Office"}],
	"alpha": [{"name": "A", "id": "org.example.Alpha", "active": false}],
	"browsers": [{"name": "B", "id": "org.example.Browser"}]}`
	list, err := ParseApps([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	want := []Category{
		{Key: "browsers", Title: i18n.MsgBrowsers, Subtitle: i18n.MsgBrowsersDesc, Apps: []App{{"B", "org.example.Browser", true}}},
		{Key: "office", Title: i18n.MsgOffice, Subtitle: i18n.MsgOfficeDesc, Apps: []App{{"O", "org.example.Office", true}}},
		{Key: "alpha", Title: "alpha", Apps: []App{{"A", "org.example.Alpha", false}}},
		{Key: "zeta", Title: "zeta", Apps: []App{{"Z", "org.example.Zeta", true}}},
	}
	if !reflect.DeepEqual(list.Categories, want) {
		t.Fatalf("categories = %+v, want %+v", list.Categories, want)
	}
}

func TestParseAppsRejects(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"invalid JSON", `{"browsers": [`, "invalid JSON"},
		{"not an object", `[]`, "invalid JSON"},
		{"trailing data", `{"a": [{"name": "A", "id": "org.example.A"}]} {}`, "data after"},
		{"null", `null`, "no categories"},
		{"empty object", `{}`, "no categories"},
		{"empty category", `{"browsers": []}`, `category "browsers" has no applications`},
		{"null category", `{"browsers": null}`, `category "browsers" has no applications`},
		{"unknown field", `{"browsers": [{"name": "A", "id": "org.example.A", "icon": "x"}]}`, "unknown field"},
		{"empty name", `{"browsers": [{"name": " ", "id": "org.example.A"}]}`, "has no name"},
		{"null entry", `{"browsers": [null]}`, "has no name"},
		{"missing id", `{"browsers": [{"name": "A"}]}`, "invalid Flatpak application ID"},
		{"two-element id", `{"browsers": [{"name": "A", "id": "org.example"}]}`, "invalid Flatpak application ID"},
		{"shell in id", `{"browsers": [{"name": "A", "id": "org.example.A;rm"}]}`, "invalid Flatpak application ID"},
		{"option as id", `{"browsers": [{"name": "A", "id": "--system"}]}`, "invalid Flatpak application ID"},
		{"wrong active type", `{"browsers": [{"name": "A", "id": "org.example.A", "active": "no"}]}`, "invalid JSON"},
		{"duplicate id", `{"browsers": [{"name": "A", "id": "org.example.A"}], "office": [{"name": "B", "id": "org.example.A"}]}`, `appears in "browsers" and "office"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := ParseApps([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) || list != nil {
				t.Fatalf("ParseApps() = %v, %v; want nil and an error containing %q", list, err, tt.want)
			}
		})
	}
}

func TestLoadAppsRejectsUnreadableAndLargeFiles(t *testing.T) {
	dir := t.TempDir()
	large := filepath.Join(dir, "large.json")
	// A valid list padded with whitespace past the limit.
	body := `{"a": [{"name": "A", "id": "org.example.A"}]}` + strings.Repeat(" ", MaxAppsFileSize)
	if err := os.WriteFile(large, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	atLimit := filepath.Join(dir, "limit.json")
	exact := `{"a": [{"name": "A", "id": "org.example.A"}]}`
	if err := os.WriteFile(atLimit, []byte(exact+strings.Repeat(" ", MaxAppsFileSize-len(exact))), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty path", "", "no apps_path"},
		{"missing file", filepath.Join(dir, "missing.json"), "read application list"},
		{"directory", dir, "read application list"},
		{"larger than 1 MiB", large, "larger than 1048576 bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := LoadApps(tt.path); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadApps(%q) error = %v, want %q", tt.path, err, tt.want)
			}
		})
	}
	if _, err := LoadApps(atLimit); err != nil {
		t.Fatalf("LoadApps(file of exactly 1 MiB) error = %v", err)
	}
}

func firstRunConfig(enabled bool, path string) *config.Config {
	return &config.Config{FirstRunPage: config.PageConfig{
		"first_run_apps_group": config.GroupConfig{Enabled: enabled, AppsPath: path},
	}}
}

func TestLoadConfiguredApps(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		wantErr  error
		wantList bool
	}{
		{"enabled with a valid list", firstRunConfig(true, firstSetupApps), nil, true},
		{"disabled group (also a config error, ADR-0003)", firstRunConfig(false, firstSetupApps), ErrAppsDisabled, false},
		{"missing list", firstRunConfig(true, filepath.Join(t.TempDir(), "none.json")), nil, false},
		{"empty apps_path", firstRunConfig(true, ""), nil, false},
		{"nil config", nil, ErrAppsDisabled, false},
		{"no first-run group", &config.Config{FirstRunPage: config.PageConfig{}}, ErrAppsDisabled, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := LoadConfiguredApps(tt.cfg)
			if (list != nil) != tt.wantList {
				t.Fatalf("LoadConfiguredApps() list = %v, want list %v (err %v)", list, tt.wantList, err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("LoadConfiguredApps() error = %v, want %v", err, tt.wantErr)
			}
			if !tt.wantList && err == nil {
				t.Fatal("LoadConfiguredApps() error = nil without a list")
			}
			// The flow hides Apps whenever the list did not load, and the
			// user can still finish.
			f := NewFlow(list != nil)
			f.SetConnectivity(ConnOnline)
			if f.Visible(StepApps) != tt.wantList {
				t.Fatalf("Apps visible = %v, want %v", f.Visible(StepApps), tt.wantList)
			}
			for f.Current() != StepProgress {
				if !f.Next() {
					t.Fatalf("stuck at %v", f.Current())
				}
			}
			f.FinishProgress()
			if !f.Next() || f.Current() != StepDone {
				t.Fatal("cannot finish")
			}
		})
	}
}

func TestValidFlatpakID(t *testing.T) {
	valid := []string{
		"org.mozilla.firefox", "io.gitlab.librewolf-community", "page.tesk.Refine",
		"com.github.tchx84.Flatseal", "org._private.App",
		"a.b.c", "org.example.App" + strings.Repeat("x", 255-len("org.example.App")),
	}
	invalid := []string{
		"", "org.example", "org", ".org.example.App", "org.example.App.", "org..example.App",
		"org.my-example.App", "1org.example.App", "org.1example.App", "org.example.1App",
		"org.example.App!", "org.exämple.App", "org example.App", "--user",
		"org.example.App" + strings.Repeat("x", 256-len("org.example.App")),
	}
	for _, id := range valid {
		if !ValidFlatpakID(id) {
			t.Errorf("ValidFlatpakID(%q) = false, want true", id)
		}
	}
	for _, id := range invalid {
		if ValidFlatpakID(id) {
			t.Errorf("ValidFlatpakID(%q) = true, want false", id)
		}
	}
}

func TestCategoryHelpers(t *testing.T) {
	raw := map[string][]rawApp{"office": nil, "x": nil, "browsers": nil, "b": nil}
	if got, want := categoryOrder(raw), []string{"browsers", "office", "b", "x"}; !reflect.DeepEqual(got, want) {
		t.Errorf("categoryOrder() = %v, want %v", got, want)
	}
	if got := categoryFor("utilities"); got.Title != i18n.MsgUtilities || got.Subtitle != i18n.MsgUtilitiesDesc {
		t.Errorf("categoryFor(utilities) = %+v", got)
	}
	if got := categoryFor("games"); got.Title != "games" || got.Subtitle != "" {
		t.Errorf("categoryFor(games) = %+v", got)
	}
	// An unknown key's title has no translation, so T returns it as is.
	if got := i18n.T(categoryFor("games").Title); got != "games" {
		t.Errorf("T(unknown key) = %q", got)
	}
}

func TestSelection(t *testing.T) {
	list, err := ParseApps([]byte(`{
		"browsers": [{"name": "Firefox", "id": "org.mozilla.firefox"}, {"name": "Chrome", "id": "com.google.Chrome", "active": false}],
		"office": [{"name": "OnlyOffice", "id": "org.onlyoffice.desktopeditors", "active": false}],
		"utilities": [{"name": "Boxes", "id": "org.gnome.Boxes"}, {"name": "Flatseal", "id": "com.github.tchx84.Flatseal"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	ids := func(apps []App) []string {
		var out []string
		for _, app := range apps {
			out = append(out, app.ID)
		}
		return out
	}

	s := NewSelection(list)
	if !s.CategoryOn("browsers") || !s.CategoryOn("utilities") || s.CategoryOn("office") {
		t.Fatal("category defaults: want browsers and utilities on (an active app), office off (no active app)")
	}
	if !s.AppOn("org.mozilla.firefox") || s.AppOn("com.google.Chrome") {
		t.Fatal("app defaults do not follow active")
	}
	if got, want := ids(s.Selected()), []string{"org.mozilla.firefox", "org.gnome.Boxes", "com.github.tchx84.Flatseal"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default Selected() = %v, want %v", got, want)
	}

	s.SetApp("com.google.Chrome", true)
	s.SetCategory("utilities", false)
	s.SetApp("org.onlyoffice.desktopeditors", true) // category still off
	if got, want := ids(s.Selected()), []string{"org.mozilla.firefox", "com.google.Chrome"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Selected() = %v, want %v", got, want)
	}
	s.SetCategory("office", true)
	if got, want := ids(s.Selected()), []string{"org.mozilla.firefox", "com.google.Chrome", "org.onlyoffice.desktopeditors"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Selected() = %v, want %v (list order)", got, want)
	}

	if got := NewSelection(nil).Selected(); got != nil {
		t.Fatalf("NewSelection(nil).Selected() = %v, want nil", got)
	}
}
