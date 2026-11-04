package firstrun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/i18n"
)

const (
	// AppsConfigPage and AppsConfigGroup name the config group of the Apps
	// step. first_run_page is not a navigation page.
	AppsConfigPage  = "first_run_page"
	AppsConfigGroup = "first_run_apps_group"

	// MaxAppsFileSize bounds the application list: a larger file is rejected.
	MaxAppsFileSize = 1 << 20
	// maxFlatpakIDLength is flatpak's limit for an application ID.
	maxFlatpakIDLength = 255
)

// ErrAppsDisabled is returned when first_run_apps_group is disabled,
// including by a configuration error (ADR-0003 fail-closed).
var ErrAppsDisabled = errors.New("first_run_apps_group is disabled")

// App is one application of the list.
type App struct {
	Name string
	ID   string
	// Active is the default of the app's switch ("active" in the file;
	// absent means true).
	Active bool
}

// Category is one category of the list, in file order of its apps.
type Category struct {
	// Key is the JSON key, such as "browsers".
	Key string
	// Title and Subtitle are msgids for a known key (translate them with
	// i18n.T); an unknown key has the key as title and no subtitle.
	Title    string
	Subtitle string
	Apps     []App
}

// AppList is a validated application list, with categories in display
// order: browsers, utilities, office, then the other keys sorted.
type AppList struct {
	Categories []Category
}

// knownCategories are first-setup's categories and their translated texts.
var knownCategories = []struct {
	key, title, subtitle string
}{
	{"browsers", i18n.MsgBrowsers, i18n.MsgBrowsersDesc},
	{"utilities", i18n.MsgUtilities, i18n.MsgUtilitiesDesc},
	{"office", i18n.MsgOffice, i18n.MsgOfficeDesc},
}

// rawApp is one entry of the file, in first-setup's shape.
type rawApp struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	Active *bool  `json:"active"`
}

// LoadConfiguredApps loads the application list that the configuration
// names: ErrAppsDisabled when first_run_apps_group is disabled, else
// LoadApps of its apps_path. Any error hides the Apps step; the user can
// still finish.
func LoadConfiguredApps(cfg *config.Config) (*AppList, error) {
	if cfg == nil || !cfg.IsGroupEnabled(AppsConfigPage, AppsConfigGroup) {
		return nil, ErrAppsDisabled
	}
	group := cfg.GetGroupConfig(AppsConfigPage, AppsConfigGroup)
	if group == nil {
		return nil, ErrAppsDisabled
	}
	return LoadApps(group.AppsPath)
}

// LoadApps reads and validates the application list at path. It rejects an
// empty path, a read error and a file larger than MaxAppsFileSize, then
// applies ParseApps.
func LoadApps(path string) (*AppList, error) {
	if path == "" {
		return nil, errors.New("no apps_path is configured")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read application list: %w", err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxAppsFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read application list %s: %w", path, err)
	}
	if len(data) > MaxAppsFileSize {
		return nil, fmt.Errorf("application list %s is larger than %d bytes", path, MaxAppsFileSize)
	}
	list, err := ParseApps(data)
	if err != nil {
		return nil, fmt.Errorf("application list %s: %w", path, err)
	}
	return list, nil
}

// ParseApps validates first-setup's JSON shape,
// {"<category>": [{"name": "...", "id": "...", "active": false}]}, where an
// absent "active" means true. It rejects invalid JSON, trailing data,
// unknown fields, an empty object, an empty category, an empty name, an id
// that is not a valid Flatpak application ID, and an id that appears twice.
func ParseApps(data []byte) (*AppList, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var raw map[string][]rawApp
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid JSON: data after the top-level object")
	}
	if len(raw) == 0 {
		return nil, errors.New("no categories")
	}

	seen := make(map[string]string)
	list := &AppList{}
	for _, key := range categoryOrder(raw) {
		entries := raw[key]
		if len(entries) == 0 {
			return nil, fmt.Errorf("category %q has no applications", key)
		}
		category := categoryFor(key)
		for i, entry := range entries {
			name := strings.TrimSpace(entry.Name)
			if name == "" {
				return nil, fmt.Errorf("category %q entry %d has no name", key, i)
			}
			if !ValidFlatpakID(entry.ID) {
				return nil, fmt.Errorf("category %q entry %q has an invalid Flatpak application ID %q", key, name, entry.ID)
			}
			if other, dup := seen[entry.ID]; dup {
				return nil, fmt.Errorf("application ID %q appears in %q and %q", entry.ID, other, key)
			}
			seen[entry.ID] = key
			active := true
			if entry.Active != nil {
				active = *entry.Active
			}
			category.Apps = append(category.Apps, App{Name: name, ID: entry.ID, Active: active})
		}
		list.Categories = append(list.Categories, category)
	}
	return list, nil
}

// categoryOrder returns the keys of raw in display order: the known
// categories first, in first-setup's order, then the other keys sorted.
func categoryOrder(raw map[string][]rawApp) []string {
	var keys, others []string
	known := make(map[string]bool)
	for _, c := range knownCategories {
		known[c.key] = true
		if _, ok := raw[c.key]; ok {
			keys = append(keys, c.key)
		}
	}
	for key := range raw {
		if !known[key] {
			others = append(others, key)
		}
	}
	sort.Strings(others)
	return append(keys, others...)
}

// categoryFor returns the empty category for key with its title and
// subtitle msgids.
func categoryFor(key string) Category {
	for _, c := range knownCategories {
		if c.key == key {
			return Category{Key: key, Title: c.title, Subtitle: c.subtitle}
		}
	}
	return Category{Key: key, Title: key}
}

// ValidFlatpakID reports whether id is a valid Flatpak application ID, with
// flatpak's rules (flatpak_is_valid_name): at most 255 bytes, at least 3
// elements separated by ".", no empty element, each element made of ASCII
// letters, digits, "_" and "-", no element that starts with a digit, and
// "-" only in the last element.
func ValidFlatpakID(id string) bool {
	if id == "" || len(id) > maxFlatpakIDLength {
		return false
	}
	elements := strings.Split(id, ".")
	if len(elements) < 3 {
		return false
	}
	for i, element := range elements {
		if element == "" {
			return false
		}
		last := i == len(elements)-1
		for j, c := range element {
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
			case c >= '0' && c <= '9':
				if j == 0 {
					return false
				}
			case c == '-':
				if !last {
					return false
				}
			default:
				return false
			}
		}
	}
	return true
}

// Selection is the state of the Apps step's switches: one per category
// (default on when at least 1 of its apps is active) and one per app
// (default from Active). It is not safe for concurrent use.
type Selection struct {
	list       *AppList
	categoryOn map[string]bool
	appOn      map[string]bool
}

// NewSelection starts a selection with the list's defaults. A nil list
// selects nothing.
func NewSelection(list *AppList) *Selection {
	s := &Selection{list: list, categoryOn: make(map[string]bool), appOn: make(map[string]bool)}
	if list == nil {
		return s
	}
	for _, category := range list.Categories {
		on := false
		for _, app := range category.Apps {
			s.appOn[app.ID] = app.Active
			on = on || app.Active
		}
		s.categoryOn[category.Key] = on
	}
	return s
}

// CategoryOn reports the switch of a category.
func (s *Selection) CategoryOn(key string) bool { return s.categoryOn[key] }

// SetCategory sets the switch of a category.
func (s *Selection) SetCategory(key string, on bool) { s.categoryOn[key] = on }

// AppOn reports the switch of an app.
func (s *Selection) AppOn(id string) bool { return s.appOn[id] }

// SetApp sets the switch of an app.
func (s *Selection) SetApp(id string, on bool) { s.appOn[id] = on }

// Selected returns the apps to install, in list order: an app is selected
// when its category is on and the app is on.
func (s *Selection) Selected() []App {
	if s.list == nil {
		return nil
	}
	var apps []App
	for _, category := range s.list.Categories {
		if !s.categoryOn[category.Key] {
			continue
		}
		for _, app := range category.Apps {
			if s.appOn[app.ID] {
				apps = append(apps, app)
			}
		}
	}
	return apps
}
