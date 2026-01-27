package help

import (
	"testing"

	"github.com/frostyard/chairlift/internal/config"
)

func TestBuildResourceLinks_NilConfig(t *testing.T) {
	links := BuildResourceLinks(nil)
	if links != nil {
		t.Errorf("expected nil for nil config, got %v", links)
	}
}

func TestBuildResourceLinks_EmptyConfig(t *testing.T) {
	cfg := &config.GroupConfig{}
	links := BuildResourceLinks(cfg)
	if len(links) != 0 {
		t.Errorf("expected empty slice for empty config, got %d links", len(links))
	}
}

func TestBuildResourceLinks_WebsiteOnly(t *testing.T) {
	cfg := &config.GroupConfig{
		Website: "https://example.com",
	}
	links := BuildResourceLinks(cfg)

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	if links[0].Title != "Website" {
		t.Errorf("expected Title 'Website', got %q", links[0].Title)
	}
	if links[0].Subtitle != "https://example.com" {
		t.Errorf("expected Subtitle 'https://example.com', got %q", links[0].Subtitle)
	}
	if links[0].URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %q", links[0].URL)
	}
}

func TestBuildResourceLinks_AllFields(t *testing.T) {
	cfg := &config.GroupConfig{
		Website: "https://example.com",
		Issues:  "https://github.com/example/issues",
		Chat:    "https://discord.gg/example",
	}
	links := BuildResourceLinks(cfg)

	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d", len(links))
	}

	// Verify order: Website, Issues, Chat
	expected := []struct {
		title    string
		subtitle string
	}{
		{"Website", "https://example.com"},
		{"Report Issues", "https://github.com/example/issues"},
		{"Community Discussions", "https://discord.gg/example"},
	}

	for i, exp := range expected {
		if links[i].Title != exp.title {
			t.Errorf("link %d: expected Title %q, got %q", i, exp.title, links[i].Title)
		}
		if links[i].Subtitle != exp.subtitle {
			t.Errorf("link %d: expected Subtitle %q, got %q", i, exp.subtitle, links[i].Subtitle)
		}
		if links[i].URL != exp.subtitle {
			t.Errorf("link %d: expected URL %q, got %q", i, exp.subtitle, links[i].URL)
		}
	}
}

func TestBuildResourceLinks_PartialConfig_IssuesOnly(t *testing.T) {
	cfg := &config.GroupConfig{
		Issues: "https://github.com/example/issues",
	}
	links := BuildResourceLinks(cfg)

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	if links[0].Title != "Report Issues" {
		t.Errorf("expected Title 'Report Issues', got %q", links[0].Title)
	}
}

func TestBuildResourceLinks_PartialConfig_WebsiteAndChat(t *testing.T) {
	cfg := &config.GroupConfig{
		Website: "https://example.com",
		Chat:    "https://discord.gg/example",
	}
	links := BuildResourceLinks(cfg)

	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}

	// First should be Website, second should be Chat (no Issues in between)
	if links[0].Title != "Website" {
		t.Errorf("expected first link Title 'Website', got %q", links[0].Title)
	}
	if links[1].Title != "Community Discussions" {
		t.Errorf("expected second link Title 'Community Discussions', got %q", links[1].Title)
	}
}
