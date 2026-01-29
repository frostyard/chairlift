package extensions

import (
	"testing"
)

func TestExtensionInfo(t *testing.T) {
	ext := ExtensionInfo{
		Component: "mesa",
		Version:   "24.1.0",
		Current:   true,
	}

	if ext.Component != "mesa" {
		t.Errorf("expected component 'mesa', got %s", ext.Component)
	}
	if ext.Version != "24.1.0" {
		t.Errorf("expected version '24.1.0', got %s", ext.Version)
	}
	if !ext.Current {
		t.Error("expected Current to be true")
	}
}

func TestDiscoveredExtension(t *testing.T) {
	ext := DiscoveredExtension{
		Name:     "steam",
		Versions: []string{"1.0.0", "0.9.0"},
	}

	if ext.Name != "steam" {
		t.Errorf("expected name 'steam', got %s", ext.Name)
	}
	if len(ext.Versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(ext.Versions))
	}
}

func TestNewClient(t *testing.T) {
	// Verify client creation doesn't panic
	client := NewClient()
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.client == nil {
		t.Error("expected non-nil inner client")
	}
}

func TestIsAvailableReturns(t *testing.T) {
	// Just verify it runs without panic - result depends on system
	_ = IsAvailable()
}
