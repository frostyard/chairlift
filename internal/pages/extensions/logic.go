// Package extensions provides the Extensions page logic layer.
// This package has no GTK dependencies, making it testable without a GTK runtime.
package extensions

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/frostyard/pm/progress"
	"github.com/frostyard/updex/updex"
)

// ExtensionInfo represents an installed extension for UI display.
type ExtensionInfo struct {
	Component string
	Version   string
	Current   bool
}

// FeatureInfo represents a feature for UI display.
type FeatureInfo struct {
	Name        string
	Description string
	Enabled     bool
	Masked      bool
}

// DiscoveredExtension represents an available extension from a repository.
type DiscoveredExtension struct {
	Name     string
	Versions []string
}

// Client wraps updex.Client for the extensions page.
type Client struct {
	client *updex.Client
}

// NewClient creates a new extensions client without progress reporting.
func NewClient() *Client {
	return &Client{
		client: updex.NewClient(updex.ClientConfig{}),
	}
}

// NewClientWithProgress creates a client with progress reporting.
func NewClientWithProgress(reporter progress.ProgressReporter) *Client {
	return &Client{
		client: updex.NewClient(updex.ClientConfig{
			Progress: reporter,
		}),
	}
}

// IsAvailable checks if the extensions system is available.
// Returns true if systemd-sysext is installed and accessible.
func IsAvailable() bool {
	_, err := exec.LookPath("systemd-sysext")
	return err == nil
}

// ListInstalled returns installed extensions.
func (c *Client) ListInstalled(ctx context.Context) ([]ExtensionInfo, error) {
	versions, err := c.client.List(ctx, updex.ListOptions{})
	if err != nil {
		return nil, err
	}

	var extensions []ExtensionInfo
	for _, v := range versions {
		if v.Installed {
			extensions = append(extensions, ExtensionInfo{
				Component: v.Component,
				Version:   v.Version,
				Current:   v.Current,
			})
		}
	}
	return extensions, nil
}

// Discover finds available extensions from a repository URL.
func (c *Client) Discover(ctx context.Context, repoURL string) ([]DiscoveredExtension, error) {
	result, err := c.client.Discover(ctx, repoURL)
	if err != nil {
		return nil, err
	}

	var discovered []DiscoveredExtension
	for _, ext := range result.Extensions {
		discovered = append(discovered, DiscoveredExtension{
			Name:     ext.Name,
			Versions: ext.Versions,
		})
	}
	return discovered, nil
}

// Install installs an extension from a repository.
func (c *Client) Install(ctx context.Context, repoURL, component string) error {
	_, err := c.client.Install(ctx, repoURL, updex.InstallOptions{
		Component: component,
	})
	return err
}

// IsDiscoverAvailable returns true if discover functionality is available.
// This requires systemd-sysext to be installed.
func (c *Client) IsDiscoverAvailable() bool {
	return IsAvailable()
}

// Features returns all configured features with their status.
func (c *Client) Features(ctx context.Context) ([]FeatureInfo, error) {
	features, err := c.client.Features(ctx)
	if err != nil {
		return nil, err
	}

	var result []FeatureInfo
	for _, f := range features {
		result = append(result, FeatureInfo{
			Name:        f.Name,
			Description: f.Description,
			Enabled:     f.Enabled,
			Masked:      f.Masked,
		})
	}
	return result, nil
}

// EnableFeature enables a feature by name.
// Uses pkexec to run updex CLI since this requires root privileges.
func (c *Client) EnableFeature(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "pkexec", "updex", "features", "enable", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", string(output), err)
	}
	return nil
}

// DisableFeature disables a feature by name.
// Uses pkexec to run updex CLI since this requires root privileges.
// Uses --force to allow disabling merged extensions (requires reboot).
func (c *Client) DisableFeature(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "pkexec", "updex", "features", "disable", "--force", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", string(output), err)
	}
	return nil
}
