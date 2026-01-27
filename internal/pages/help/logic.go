// Package help provides the Help page implementation.
package help

import "github.com/frostyard/chairlift/internal/config"

// ResourceLink represents a help resource link to display in the UI.
type ResourceLink struct {
	// Title is the display title (e.g., "Website", "Report Issues").
	Title string

	// Subtitle is the display subtitle (usually the URL itself).
	Subtitle string

	// URL is the actual URL to open when the link is activated.
	URL string
}

// BuildResourceLinks creates the list of resource links from group config.
// This is pure Go logic with no GTK dependencies, making it testable.
//
// Returns nil if cfg is nil.
func BuildResourceLinks(cfg *config.GroupConfig) []ResourceLink {
	if cfg == nil {
		return nil
	}

	var links []ResourceLink

	if cfg.Website != "" {
		links = append(links, ResourceLink{
			Title:    "Website",
			Subtitle: cfg.Website,
			URL:      cfg.Website,
		})
	}

	if cfg.Issues != "" {
		links = append(links, ResourceLink{
			Title:    "Report Issues",
			Subtitle: cfg.Issues,
			URL:      cfg.Issues,
		})
	}

	if cfg.Chat != "" {
		links = append(links, ResourceLink{
			Title:    "Community Discussions",
			Subtitle: cfg.Chat,
			URL:      cfg.Chat,
		})
	}

	return links
}
