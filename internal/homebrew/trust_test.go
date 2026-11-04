package homebrew

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
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

// TestParseUntrustedTapNamesMissingField covers Homebrew versions older than
// 6, whose `tap-info --installed --json` output has no "trusted" key at all
// (rather than an explicit trusted: true/false). A missing key must not be
// treated as untrusted, since brew trust doesn't even exist on those
// versions and every tap would otherwise be misreported as untrusted.
func TestParseUntrustedTapNamesMissingField(t *testing.T) {
	const legacyJSON = `[{"name":"legacy/tap","installed":true}]`
	names, err := parseUntrustedTapNames([]byte(legacyJSON))
	if err != nil {
		t.Fatalf("parseUntrustedTapNames: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("got %v, want empty result for tap-info without a trusted field", names)
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

func TestFormulaeGroupedByTap(t *testing.T) {
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

func TestCasksGroupedByTap(t *testing.T) {
	caskroom := t.TempDir()
	// Homebrew's Cask::Tab.create writes the receipt to
	// <token>/.metadata/INSTALL_RECEIPT.json and records the origin tap in
	// source.tap.
	writeFile(t, filepath.Join(caskroom, "somecask", ".metadata", "INSTALL_RECEIPT.json"),
		`{"loaded_from_api": false, "source": {"tap": "ublue-os/tap", "path": "/x/Casks/somecask.rb"}}`)
	// API-installed cask: the receipt still records homebrew/cask, which
	// ListUntrustedTaps never looks up because tap-info reports it trusted.
	writeFile(t, filepath.Join(caskroom, "codex", ".metadata", "INSTALL_RECEIPT.json"),
		`{"loaded_from_api": true, "source": {"tap": "homebrew/cask"}}`)
	writeFile(t, filepath.Join(caskroom, "multitap", ".metadata", "INSTALL_RECEIPT.json"),
		`{"source": {"tap": "fresh-org/tap"}}`)
	// The legacy versioned metadata JSON is ignored even when it names a tap.
	writeFile(t, filepath.Join(caskroom, "multitap", ".metadata", "1.0", "20260101", "Casks", "multitap.json"),
		`{"token": "multitap", "tap": "stale-org/tap"}`)
	writeFile(t, filepath.Join(caskroom, "legacyonly", ".metadata", "1.0", "20260101", "Casks", "legacyonly.json"),
		`{"token": "legacyonly", "tap": "stale-org/tap"}`)
	// A receipt one level too deep (the wrong path) is not read.
	writeFile(t, filepath.Join(caskroom, "deep", ".metadata", "1.0", "INSTALL_RECEIPT.json"),
		`{"source": {"tap": "wrong-org/tap"}}`)
	// Receipts without a tap, or malformed JSON, are skipped.
	writeFile(t, filepath.Join(caskroom, "notap", ".metadata", "INSTALL_RECEIPT.json"), `{"source": {}}`)
	writeFile(t, filepath.Join(caskroom, "garbled", ".metadata", "INSTALL_RECEIPT.json"), `{not json`)
	// A cask directory with no receipt and a hidden directory are skipped.
	if err := os.MkdirAll(filepath.Join(caskroom, "broke"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(caskroom, ".hidden", ".metadata", "INSTALL_RECEIPT.json"),
		`{"source": {"tap": "hidden-org/tap"}}`)

	want := map[string][]string{
		"ublue-os/tap":  {"ublue-os/tap/somecask"},
		"homebrew/cask": {"homebrew/cask/codex"},
		"fresh-org/tap": {"fresh-org/tap/multitap"},
	}
	if got := installedCasksByTap(caskroom); !reflect.DeepEqual(got, want) {
		t.Errorf("installedCasksByTap() = %v, want %v", got, want)
	}
}

func TestCasksGroupedByTapMissingCaskroom(t *testing.T) {
	got := installedCasksByTap(filepath.Join(t.TempDir(), "absent"))
	if len(got) != 0 {
		t.Errorf("installedCasksByTap(missing dir) = %v, want empty", got)
	}
}

func TestUntrustedTapMessageDetection(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Error: Refusing to load formula opencode from untrusted tap anomalyco/tap.", true},
		{"Warning: The following taps are not trusted:\n  multica-ai/tap", true},
		{"Error: No such formula", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isUntrustedTapMessage(c.in); got != c.want {
			t.Errorf("isUntrustedTapMessage(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestTrustPackagesDryRun(t *testing.T) {
	SetDryRun(true)
	defer SetDryRun(false)
	err := TrustPackages(UntrustedTap{
		Name:     "multica-ai/tap",
		Formulae: []string{"multica-ai/tap/multica"},
	})
	if err != nil {
		t.Fatalf("dry-run TrustPackages: %v", err)
	}
}
