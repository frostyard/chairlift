package maintenance

import (
	"testing"

	"github.com/frostyard/chairlift/internal/config"
)

func TestParseActions_NilConfig(t *testing.T) {
	actions := ParseActions(nil)
	if actions != nil {
		t.Errorf("expected nil, got %v", actions)
	}
}

func TestParseActions_EmptyActions(t *testing.T) {
	cfg := &config.GroupConfig{
		Enabled: true,
		Actions: []config.ActionConfig{},
	}

	actions := ParseActions(cfg)
	if len(actions) != 0 {
		t.Errorf("expected empty slice, got %v", actions)
	}
}

func TestParseActions_WithActions(t *testing.T) {
	cfg := &config.GroupConfig{
		Enabled: true,
		Actions: []config.ActionConfig{
			{
				Title:  "Clean Boot Entries",
				Script: "/usr/libexec/bls-gc",
				Sudo:   true,
			},
			{
				Title:  "Clear Cache",
				Script: "rm -rf ~/.cache/tmp",
				Sudo:   false,
			},
		},
	}

	actions := ParseActions(cfg)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}

	// Verify first action
	if actions[0].Title != "Clean Boot Entries" {
		t.Errorf("action[0].Title = %q, want %q", actions[0].Title, "Clean Boot Entries")
	}
	if actions[0].Script != "/usr/libexec/bls-gc" {
		t.Errorf("action[0].Script = %q, want %q", actions[0].Script, "/usr/libexec/bls-gc")
	}
	if !actions[0].Sudo {
		t.Error("action[0].Sudo should be true")
	}

	// Verify second action
	if actions[1].Title != "Clear Cache" {
		t.Errorf("action[1].Title = %q, want %q", actions[1].Title, "Clear Cache")
	}
	if actions[1].Script != "rm -rf ~/.cache/tmp" {
		t.Errorf("action[1].Script = %q, want %q", actions[1].Script, "rm -rf ~/.cache/tmp")
	}
	if actions[1].Sudo {
		t.Error("action[1].Sudo should be false")
	}
}

func TestParseActions_DescriptionMatchesScript(t *testing.T) {
	cfg := &config.GroupConfig{
		Enabled: true,
		Actions: []config.ActionConfig{
			{
				Title:  "Test Action",
				Script: "/path/to/script.sh",
				Sudo:   false,
			},
		},
	}

	actions := ParseActions(cfg)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}

	// Description should match Script for now
	if actions[0].Description != actions[0].Script {
		t.Errorf("Description = %q, Script = %q, they should match", actions[0].Description, actions[0].Script)
	}
}
