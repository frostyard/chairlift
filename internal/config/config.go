// Package config provides YAML configuration loading for ChairLift
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	SystemPage       PageConfig `yaml:"system_page"`
	UpdatesPage      PageConfig `yaml:"updates_page"`
	ApplicationsPage PageConfig `yaml:"applications_page"`
	MaintenancePage  PageConfig `yaml:"maintenance_page"`
	FeaturesPage     PageConfig `yaml:"features_page"`
	// ProblemsPage configures the Problems page (ADR-0015): local problem
	// records of the system and of the user's apps.
	ProblemsPage PageConfig `yaml:"problems_page"`
	HelpPage     PageConfig `yaml:"help_page"`
	// FirstRunPage configures `chairlift --first-run`. It is not a
	// navigation page: internal/navigation never lists it.
	FirstRunPage PageConfig `yaml:"first_run_page"`

	// source is the exact file this configuration was read from. It is
	// unexported, so the reflected schema and the YAML overlay never see
	// it, and it decides whether a group's endpoint is administrator-owned
	// (Config.ReportEndpoint).
	source string
}

// PageConfig represents configuration for a single page
type PageConfig map[string]GroupConfig

// GroupConfig represents configuration for a preference group
type GroupConfig struct {
	Enabled      bool           `yaml:"enabled"`
	AppID        string         `yaml:"app_id,omitempty"`
	AppIDs       []string       `yaml:"-"`
	Actions      []ActionConfig `yaml:"actions,omitempty"`
	Website      string         `yaml:"website,omitempty"`
	Issues       string         `yaml:"issues,omitempty"`
	Chat         string         `yaml:"chat,omitempty"`
	BundlesPaths []string       `yaml:"bundles_paths,omitempty"`
	AppsPath     string         `yaml:"apps_path,omitempty"`
	// Endpoint overrides the problem-report endpoint. Only an
	// administrator-owned configuration file may set it (ADR-0015): the
	// development fallback beside the executable is user-writable, so
	// Config.ReportEndpoint ignores the value it sets.
	Endpoint string `yaml:"endpoint,omitempty"`
}

// ActionConfig represents a configurable action
type ActionConfig struct {
	Title  string `yaml:"title"`
	Script string `yaml:"script"`
	Sudo   bool   `yaml:"sudo"`
}

// rawConfig mirrors Config for YAML parsing, but every optional field is a
// pointer so yaml.v3 can distinguish "key omitted" (nil) from "key present,
// possibly with the zero value" (non-nil). It is never exposed outside this
// file; loadFromPath merges it onto defaultConfig() to produce the *Config
// callers see.
type rawConfig struct {
	SystemPage       rawPageConfig `yaml:"system_page"`
	UpdatesPage      rawPageConfig `yaml:"updates_page"`
	ApplicationsPage rawPageConfig `yaml:"applications_page"`
	MaintenancePage  rawPageConfig `yaml:"maintenance_page"`
	FeaturesPage     rawPageConfig `yaml:"features_page"`
	ProblemsPage     rawPageConfig `yaml:"problems_page"`
	HelpPage         rawPageConfig `yaml:"help_page"`
	FirstRunPage     rawPageConfig `yaml:"first_run_page"`
}

// rawPageConfig mirrors PageConfig for YAML parsing.
type rawPageConfig map[string]rawGroupConfig

// rawGroupConfig mirrors GroupConfig for YAML parsing. A nil field means the
// key was absent (or explicitly null) in the source file and the merge keeps
// defaultConfig()'s value; a non-nil field, including a pointer to an empty
// string/slice, means the file set that field explicitly and it replaces the
// default outright.
type rawGroupConfig struct {
	Enabled      *bool           `yaml:"enabled"`
	AppID        *appIDList      `yaml:"app_id"`
	Actions      *[]ActionConfig `yaml:"actions"`
	Website      *string         `yaml:"website"`
	Issues       *string         `yaml:"issues"`
	Chat         *string         `yaml:"chat"`
	BundlesPaths *[]string       `yaml:"bundles_paths"`
	AppsPath     *string         `yaml:"apps_path"`
	Endpoint     *string         `yaml:"endpoint"`
}

// appIDList accepts a desktop ID or a preference-ordered sequence of IDs.
type appIDList []string

func (ids *appIDList) UnmarshalYAML(node *yaml.Node) error {
	values := []*yaml.Node{node}
	if node.Kind == yaml.SequenceNode {
		values = node.Content
	}
	result := make(appIDList, 0, len(values))
	for _, value := range values {
		if value.Kind != yaml.ScalarNode || value.ShortTag() != "!!str" {
			return fmt.Errorf("app_id must contain only strings (line %d)", value.Line)
		}
		result = append(result, value.Value)
	}
	*ids = result
	return nil
}

// configPaths are the locations to search for the config file
var configPaths = []string{
	"/etc/chairlift/config.yml",
	"/usr/share/chairlift/config.yml",
	"config.yml",
}

// readFile is an injection seam for deterministic read-failure tests. Its
// production value is always os.ReadFile.
var readFile = os.ReadFile

// Load loads the first existing configuration candidate. A missing candidate
// continues the documented search order; any other failure in the first
// existing candidate is authoritative and returns a fail-closed configuration
// together with the actionable error.
func Load() (*Config, *LoadError) {
	for _, candidate := range configPaths {
		path := resolveCandidatePath(candidate)
		cfg, err := loadResolvedPath(path)
		if err == nil {
			log.Printf("Loaded config from %s", path)
			return cfg, nil
		}
		if err.Kind == KindRead && errors.Is(err, fs.ErrNotExist) && !danglingSymlink(path) {
			continue
		}

		log.Print(err.LogMessage())
		return disabledConfig(), err
	}

	// Preserve built-in defaults only when every candidate is genuinely absent.
	log.Println("No config file found, using defaults")
	return defaultConfig(), nil
}

// loadFromPath attempts to load config from a specific path
func loadFromPath(path string) (*Config, *LoadError) {
	return loadResolvedPath(resolveCandidatePath(path))
}

// loadResolvedPath reads and strictly validates one already-resolved
// candidate, then overlays it onto the built-in defaults.
func loadResolvedPath(path string) (*Config, *LoadError) {
	data, err := readFile(path)
	if err != nil {
		return nil, &LoadError{
			Path:   path,
			Kind:   KindRead,
			Detail: "reading configuration file",
			Err:    err,
		}
	}

	raw, loadErr := parseAndValidate(path, data)
	if loadErr != nil {
		return nil, loadErr
	}

	cfg := mergeConfig(defaultConfig(), raw)
	cfg.source = path
	return cfg, nil
}

// adminConfigPaths are the configuration files that only an administrator
// can write. Every other candidate, including the development fallback
// beside the executable and in the working directory, is user-writable.
var adminConfigPaths = []string{
	"/etc/chairlift/config.yml",
	"/usr/share/chairlift/config.yml",
}

// adminConfigPath reports whether path is one of adminConfigPaths.
func adminConfigPath(path string) bool {
	for _, candidate := range adminConfigPaths {
		if path == candidate {
			return true
		}
	}
	return false
}

// ReportEndpoint returns the endpoint that problems_reporting_group sets and
// whether the file that set it is administrator-owned. The string is the raw
// configured value, which may be empty: the caller parses it, so this
// package does not import the reporting package. A value from a
// user-writable file is ignored with 1 log line, because a user setting must
// not choose where a report goes.
func (c *Config) ReportEndpoint() (string, bool) {
	if c == nil {
		return "", false
	}
	group, ok := c.ProblemsPage["problems_reporting_group"]
	if !ok || group.Endpoint == "" {
		return "", false
	}
	if !adminConfigPath(c.source) {
		log.Printf("configuration: the endpoint of problems_reporting_group is ignored: %s is not administrator-owned", c.source)
		return "", false
	}
	return group.Endpoint, true
}

// danglingSymlink reports whether path is a symlink whose target is missing.
// os.ReadFile follows the link and fails with ENOENT for such a link exactly
// as it does for an absent path, so an Lstat on the candidate is the only way
// to tell a present-but-unusable authoritative symlink apart from a genuinely
// missing candidate. A missing directory entry returns false (Lstat also fails
// with ENOENT), so Load still advances past it. A dangling symlink's directory
// entry exists, so Load treats it as the authoritative file and fails closed.
func danglingSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	_, err = os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// disabledConfig retains the canonical pages, groups, and non-visibility
// defaults while forcing every known group off. It is the only configuration
// returned for an authoritative read, parse/type, or schema failure.
func disabledConfig() *Config {
	cfg := defaultConfig()
	pages := []PageConfig{
		cfg.SystemPage,
		cfg.UpdatesPage,
		cfg.ApplicationsPage,
		cfg.MaintenancePage,
		cfg.FeaturesPage,
		cfg.ProblemsPage,
		cfg.HelpPage,
		cfg.FirstRunPage,
	}
	for _, page := range pages {
		for name, group := range page {
			group.Enabled = false
			page[name] = group
		}
	}
	return cfg
}

// mergeConfig overlays raw (a parsed config file) onto def (defaultConfig())
// page by page, returning a new *Config. Every optional field on every group
// follows the same rule: omitted in raw -> keep def's value; present in raw
// (including an explicit empty string/slice) -> use raw's value, replacing
// def's outright.
func mergeConfig(def *Config, raw *rawConfig) *Config {
	return &Config{
		SystemPage:       mergePage(def.SystemPage, raw.SystemPage),
		UpdatesPage:      mergePage(def.UpdatesPage, raw.UpdatesPage),
		ApplicationsPage: mergePage(def.ApplicationsPage, raw.ApplicationsPage),
		MaintenancePage:  mergePage(def.MaintenancePage, raw.MaintenancePage),
		FeaturesPage:     mergePage(def.FeaturesPage, raw.FeaturesPage),
		ProblemsPage:     mergePage(def.ProblemsPage, raw.ProblemsPage),
		HelpPage:         mergePage(def.HelpPage, raw.HelpPage),
		FirstRunPage:     mergePage(def.FirstRunPage, raw.FirstRunPage),
	}
}

// mergePage overlays raw onto def for a single page. Runtime inputs have
// already passed strict schema validation, so the unknown-group branch is
// defensive support for trusted package-internal callers only. Groups present
// in both maps are merged field by field.
func mergePage(def PageConfig, raw rawPageConfig) PageConfig {
	result := make(PageConfig, len(def))
	for name, group := range def {
		result[name] = group
	}

	for name, rawGroup := range raw {
		base, ok := def[name]
		if !ok {
			// Preserve the historical package-internal merge behavior.
			base = GroupConfig{Enabled: true}
		}
		result[name] = mergeGroup(base, rawGroup)
	}

	return result
}

// mergeGroup overlays raw onto def for a single group, field by field. Each
// assignment is guarded by the corresponding raw pointer's nil-check: nil
// means the file omitted (or explicitly nulled) that key, so def's value is
// kept; non-nil means the file set the key, so raw's value replaces def's,
// including an explicit empty string or empty slice.
func mergeGroup(def GroupConfig, raw rawGroupConfig) GroupConfig {
	result := def

	if raw.Enabled != nil {
		result.Enabled = *raw.Enabled
	}
	if raw.AppID != nil {
		result.AppIDs = append([]string{}, (*raw.AppID)...)
		result.AppID = ""
		if len(result.AppIDs) > 0 {
			result.AppID = result.AppIDs[0]
		}
	}
	if raw.Actions != nil {
		result.Actions = *raw.Actions
	}
	if raw.Website != nil {
		result.Website = *raw.Website
	}
	if raw.Issues != nil {
		result.Issues = *raw.Issues
	}
	if raw.Chat != nil {
		result.Chat = *raw.Chat
	}
	if raw.BundlesPaths != nil {
		result.BundlesPaths = *raw.BundlesPaths
	}
	if raw.AppsPath != nil {
		result.AppsPath = *raw.AppsPath
	}
	if raw.Endpoint != nil {
		result.Endpoint = *raw.Endpoint
	}

	return result
}

// defaultConfig returns the default configuration
func defaultConfig() *Config {
	return &Config{
		SystemPage: PageConfig{
			"system_info_group":  GroupConfig{Enabled: true},
			"bootc_status_group": GroupConfig{Enabled: true},
			"health_group": GroupConfig{
				Enabled: true,
				AppID:   "org.gnome.SystemMonitor",
				AppIDs:  []string{"org.gnome.SystemMonitor", "org.kde.plasma-systemmonitor"},
			},
		},
		UpdatesPage: PageConfig{
			"bootc_updates_group":     GroupConfig{Enabled: true},
			"sysupdate_updates_group": GroupConfig{Enabled: true},
			"firmware_updates_group":  GroupConfig{Enabled: true},
			"flatpak_updates_group":   GroupConfig{Enabled: true},
			"brew_updates_group":      GroupConfig{Enabled: true},
			"brew_trust_group":        GroupConfig{Enabled: true},
		},
		ApplicationsPage: PageConfig{
			"applications_installed_group": GroupConfig{
				Enabled: true,
				AppID:   "io.github.kolunmi.Bazaar",
			},
			"flatpak_user_group":   GroupConfig{Enabled: true},
			"flatpak_system_group": GroupConfig{Enabled: true},
			"brew_group":           GroupConfig{Enabled: true},
			"brew_search_group":    GroupConfig{Enabled: true},
			"brew_bundles_group": GroupConfig{
				Enabled:      true,
				BundlesPaths: []string{"/usr/share/snow/bundles"},
			},
		},
		MaintenancePage: PageConfig{
			"maintenance_cleanup_group": GroupConfig{
				Enabled: false,
				Actions: []ActionConfig{
					{
						Title:  "Clean Up Boot Old Entries",
						Script: "/usr/libexec/bls-gc",
						Sudo:   true,
					},
				},
			},
			"maintenance_brew_group":    GroupConfig{Enabled: true},
			"maintenance_flatpak_group": GroupConfig{Enabled: true},
			// Deprecated: maintenance_optimization_group only ever showed a
			// "Coming soon" placeholder and now renders nothing. It stays
			// in the schema because an unknown group is a schema error that
			// disables every group, and existing files name it. It is in no
			// navigation.Item, so it cannot keep the Maintenance page visible.
			"maintenance_optimization_group": GroupConfig{Enabled: true},
		},
		FeaturesPage: PageConfig{
			"features_group": GroupConfig{Enabled: true},
		},
		ProblemsPage: PageConfig{
			"problems_system_group":    GroupConfig{Enabled: true},
			"problems_apps_group":      GroupConfig{Enabled: true},
			"problems_reporting_group": GroupConfig{Enabled: true},
		},
		HelpPage: PageConfig{
			"help_resources_group": GroupConfig{
				Enabled: true,
				Website: "https://github.com/frostyard/snosi",
				Issues:  "https://github.com/frostyard/snosi/issues",
				Chat:    "https://github.com/frostyard/snosi/discussions",
			},
		},
		FirstRunPage: PageConfig{
			"first_run_apps_group": GroupConfig{
				Enabled:  true,
				AppsPath: defaultFirstRunAppsPath,
			},
		},
	}
}

// defaultFirstRunAppsPath is the default apps_path of first_run_apps_group:
// the application list that the Snow images ship for the first run's Apps
// step (ADR-0014).
const defaultFirstRunAppsPath = "/usr/share/snow/first-run/apps.json"

// lookupPage returns the configuration of the page named pageName, and false
// when no page has that name.
func (c *Config) lookupPage(pageName string) (PageConfig, bool) {
	switch pageName {
	case "system_page":
		return c.SystemPage, true
	case "updates_page":
		return c.UpdatesPage, true
	case "applications_page":
		return c.ApplicationsPage, true
	case "maintenance_page":
		return c.MaintenancePage, true
	case "features_page":
		return c.FeaturesPage, true
	case "problems_page":
		return c.ProblemsPage, true
	case "help_page":
		return c.HelpPage, true
	case "first_run_page":
		return c.FirstRunPage, true
	default:
		return nil, false
	}
}

// IsGroupEnabled checks if a preference group is enabled
func (c *Config) IsGroupEnabled(pageName, groupName string) bool {
	page, ok := c.lookupPage(pageName)
	if !ok {
		return true
	}

	group, ok := page[groupName]
	if !ok {
		return true // Default to enabled if not specified
	}
	return group.Enabled
}

// GetGroupConfig returns the configuration for a specific group
func (c *Config) GetGroupConfig(pageName, groupName string) *GroupConfig {
	page, ok := c.lookupPage(pageName)
	if !ok {
		return nil
	}

	group, ok := page[groupName]
	if !ok {
		return nil
	}
	return &group
}
