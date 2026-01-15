// Package config provides YAML configuration loading for ChairLift
package config

import (
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	SystemPage       PageConfig `yaml:"system_page"`
	UpdatesPage      PageConfig `yaml:"updates_page"`
	ApplicationsPage PageConfig `yaml:"applications_page"`
	MaintenancePage  PageConfig `yaml:"maintenance_page"`
	ExtensionsPage   PageConfig `yaml:"extensions_page"`
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

// configPaths are the locations to search for the config file
var configPaths = []string{
	"/etc/chairlift/config.yml",
	"/usr/share/chairlift/config.yml",
	"config.yml",
}

// Load loads the configuration from available config files
func Load() *Config {
	for _, path := range configPaths {
		cfg, err := loadFromPath(path)
		if err == nil {
			log.Printf("Loaded config from %s", path)
			return cfg
		}
	}

	// Return default config if no file found
	log.Println("No config file found, using defaults")
	return defaultConfig()
}

// loadFromPath attempts to load config from a specific path
func loadFromPath(path string) (*Config, error) {
	// Handle relative paths
	if !filepath.IsAbs(path) {
		// Try relative to executable
		execDir, err := os.Executable()
		if err == nil {
			execPath := filepath.Join(filepath.Dir(execDir), path)
			if _, err := os.Stat(execPath); err == nil {
				path = execPath
			}
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// defaultConfig returns the default configuration
func defaultConfig() *Config {
	return &Config{
		SystemPage: PageConfig{
			"system_info_group": GroupConfig{Enabled: true},
			"nbc_status_group":  GroupConfig{Enabled: true},
			"health_group": GroupConfig{
				Enabled: true,
				AppID:   "io.missioncenter.MissionCenter",
			},
		},
		UpdatesPage: PageConfig{
			"nbc_updates_group":      GroupConfig{Enabled: true},
			"flatpak_updates_group":  GroupConfig{Enabled: true},
			"brew_updates_group":     GroupConfig{Enabled: true},
			"updates_settings_group": GroupConfig{Enabled: true},
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
		ExtensionsPage: PageConfig{
			"installed_group": GroupConfig{Enabled: true},
			"discover_group":  GroupConfig{Enabled: true},
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
	case "extensions_page":
		page = c.ExtensionsPage
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
	case "extensions_page":
		page = c.ExtensionsPage
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
