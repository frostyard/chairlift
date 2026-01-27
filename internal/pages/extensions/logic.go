// Package extensions provides the Extensions page logic layer.
// This package has no GTK dependencies, making it testable without a GTK runtime.
package extensions

import (
	"context"
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
