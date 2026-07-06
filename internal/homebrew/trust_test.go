package homebrew

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

const tapInfoJSON = `[
  {"name": "homebrew/core", "installed": true, "trusted": true},
  {"name": "multica-ai/tap", "installed": true, "trusted": false},
  {"name": "ublue-os/tap", "installed": true, "trusted": false},
  {"name": "charmbracelet/tap", "installed": true, "trusted": true}
]`

func TestParseUntrustedTapNames(t *testing.T) {
	names, err := parseUntrustedTapNames([]byte(tapInfoJSON))
	if err != nil {
		t.Fatalf("parseUntrustedTapNames: %v", err)
	}
	sort.Strings(names)
	want := []string{"multica-ai/tap", "ublue-os/tap"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestParseUntrustedTapNamesMalformed(t *testing.T) {
	if _, err := parseUntrustedTapNames([]byte("nope")); err == nil {
		t.Error("want error on malformed JSON")
	}
}

// writeFile creates a file with parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledFormulaeByTap(t *testing.T) {
	cellar := t.TempDir()
	writeFile(t, filepath.Join(cellar, "multica", "0.3.19", "INSTALL_RECEIPT.json"),
		`{"source": {"tap": "multica-ai/tap"}}`)
	writeFile(t, filepath.Join(cellar, "jq", "1.7", "INSTALL_RECEIPT.json"),
		`{"source": {"tap": "homebrew/core"}}`)
	// keg with no receipt is skipped silently
	if err := os.MkdirAll(filepath.Join(cellar, "broken", "1.0"), 0o755); err != nil {
		t.Fatal(err)
	}

	byTap := installedFormulaeByTap(cellar)
	if got := byTap["multica-ai/tap"]; !reflect.DeepEqual(got, []string{"multica-ai/tap/multica"}) {
		t.Errorf("multica-ai/tap = %v", got)
	}
	if got := byTap["homebrew/core"]; !reflect.DeepEqual(got, []string{"homebrew/core/jq"}) {
		t.Errorf("homebrew/core = %v", got)
	}
}

func TestInstalledCasksByTap(t *testing.T) {
	caskroom := t.TempDir()
	writeFile(t, filepath.Join(caskroom, "somecask", ".metadata", "1.0", "20260101", "Casks", "somecask.json"),
		`{"token": "somecask", "tap": "ublue-os/tap"}`)
	// API-installed cask without Casks/*.json metadata is skipped (trusted)
	writeFile(t, filepath.Join(caskroom, "codex", ".metadata", "INSTALL_RECEIPT.json"),
		`{"loaded_from_api": true}`)

	// Two metadata snapshots whose directory names sort in the opposite
	// order of their actual chronology ("9" sorts lexically after "10",
	// even though it is the older snapshot). Each snapshot claims a
	// different tap so we can tell which one installedCasksByTap picked.
	oldMetaPath := filepath.Join(caskroom, "multitap", ".metadata", "9", "20260101", "Casks", "multitap.json")
	newMetaPath := filepath.Join(caskroom, "multitap", ".metadata", "10", "20260201", "Casks", "multitap.json")
	writeFile(t, oldMetaPath, `{"token": "multitap", "tap": "stale-org/tap"}`)
	writeFile(t, newMetaPath, `{"token": "multitap", "tap": "fresh-org/tap"}`)

	oldTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldMetaPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newMetaPath, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	byTap := installedCasksByTap(caskroom)
	if got := byTap["ublue-os/tap"]; !reflect.DeepEqual(got, []string{"ublue-os/tap/somecask"}) {
		t.Errorf("ublue-os/tap = %v", got)
	}
	if _, ok := byTap["homebrew/cask"]; ok {
		t.Error("API cask should not be attributed to any tap")
	}
	if got := byTap["fresh-org/tap"]; !reflect.DeepEqual(got, []string{"fresh-org/tap/multitap"}) {
		t.Errorf("fresh-org/tap (chronologically newest, lexically first dir) = %v, want [fresh-org/tap/multitap]", got)
	}
	if _, ok := byTap["stale-org/tap"]; ok {
		t.Error("stale metadata (lexically last dir but chronologically older) should not win attribution")
	}
}
