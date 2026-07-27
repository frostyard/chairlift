// Package config provides YAML configuration loading for ChairLift
package config

import (
	"errors"
	"io/fs"
	"log"
	"os"
)

// Config represents the application configuration
type Config struct {
	SystemPage       PageConfig `yaml:"system_page"`
	UpdatesPage      PageConfig `yaml:"updates_page"`
	ApplicationsPage PageConfig `yaml:"applications_page"`
	MaintenancePage  PageConfig `yaml:"maintenance_page"`
	FeaturesPage     PageConfig `yaml:"features_page"`
	HelpPage         PageConfig `yaml:"help_page"`
}

// PageConfig represents configuration for a single page
type PageConfig map[string]GroupConfig

// GroupConfig represents configuration for a preference group
type GroupConfig struct {
	Enabled      bool           `yaml:"enabled"`
	AppID        string         `yaml:"app_id,omitempty"`
	Actions      []ActionConfig `yaml:"actions,omitempty"`
	Website      string         `yaml:"website,omitempty"`
	Issues       string         `yaml:"issues,omitempty"`
	Chat         string         `yaml:"chat,omitempty"`
	BundlesPaths []string       `yaml:"bundles_paths,omitempty"`
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
	HelpPage         rawPageConfig `yaml:"help_page"`
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
	AppID        *string         `yaml:"app_id"`
	Actions      *[]ActionConfig `yaml:"actions"`
	Website      *string         `yaml:"website"`
	Issues       *string         `yaml:"issues"`
	Chat         *string         `yaml:"chat"`
	BundlesPaths *[]string       `yaml:"bundles_paths"`
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
		if err.Kind == KindRead && errors.Is(err, fs.ErrNotExist) {
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

	return mergeConfig(defaultConfig(), raw), nil
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
		cfg.HelpPage,
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
		HelpPage:         mergePage(def.HelpPage, raw.HelpPage),
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
		result.AppID = *raw.AppID
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
				AppID:   "io.missioncenter.MissionCenter",
			},
		},
		UpdatesPage: PageConfig{
			"bootc_updates_group":   GroupConfig{Enabled: true},
			"flatpak_updates_group": GroupConfig{Enabled: true},
			"brew_updates_group":    GroupConfig{Enabled: true},
			"brew_trust_group":      GroupConfig{Enabled: true},
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
			"maintenance_brew_group":         GroupConfig{Enabled: true},
			"maintenance_flatpak_group":      GroupConfig{Enabled: true},
			"maintenance_optimization_group": GroupConfig{Enabled: true},
		},
		FeaturesPage: PageConfig{
			"features_group": GroupConfig{Enabled: true},
		},
		HelpPage: PageConfig{
			"help_resources_group": GroupConfig{
				Enabled: true,
				Website: "https://github.com/frostyard/snow",
				Issues:  "https://github.com/frostyard/snow/issues",
				Chat:    "https://github.com/frostyard/snow/discussions",
			},
		},
	}
}

// IsGroupEnabled checks if a preference group is enabled
func (c *Config) IsGroupEnabled(pageName, groupName string) bool {
	var page PageConfig
	switch pageName {
	case "system_page":
		page = c.SystemPage
	case "updates_page":
		page = c.UpdatesPage
	case "applications_page":
		page = c.ApplicationsPage
	case "maintenance_page":
		page = c.MaintenancePage
	case "features_page":
		page = c.FeaturesPage
	case "help_page":
		page = c.HelpPage
	default:
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
	var page PageConfig
	switch pageName {
	case "system_page":
		page = c.SystemPage
	case "updates_page":
		page = c.UpdatesPage
	case "applications_page":
		page = c.ApplicationsPage
	case "maintenance_page":
		page = c.MaintenancePage
	case "features_page":
		page = c.FeaturesPage
	case "help_page":
		page = c.HelpPage
	default:
		return nil
	}

	group, ok := page[groupName]
	if !ok {
		return nil
	}
	return &group
}
