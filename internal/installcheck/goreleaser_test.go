package installcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/chairlift/internal/updex"
	"gopkg.in/yaml.v3"
)

// loadGoreleaserConfig parses the real, repo-root .goreleaser.yaml — not a
// fixture or copy that could drift from the file goreleaser actually reads
// — using the yaml.v3 dependency already vendored for internal/config.
func loadGoreleaserConfig(t *testing.T) GoreleaserConfig {
	t.Helper()

	path := filepath.Join(RepoRoot(), ".goreleaser.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var cfg GoreleaserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return cfg
}

// nfpmDst finds the contents[] entry whose src ends in srcSuffix and returns
// its dst, failing the test if no such entry exists.
func nfpmDst(t *testing.T, nfpm NfpmConfig, srcSuffix string) string {
	t.Helper()

	for _, c := range nfpm.Contents {
		if strings.HasSuffix(c.Src, srcSuffix) {
			return c.Dst
		}
	}
	t.Fatalf("no nfpm contents entry with src ending in %q", srcSuffix)
	return ""
}

// TestGoreleaserNfpmLayoutMatchesUsrPrefix parses the real .goreleaser.yaml
// and asserts its nFPM package layout agrees with internal/updex.HelperPath
// and the fixed PolicyKit read locations, so a future edit to any one of
// .goreleaser.yaml, internal/updex.HelperPath, or the polkit directories
// fails this test instead of silently drifting.
func TestGoreleaserNfpmLayoutMatchesUsrPrefix(t *testing.T) {
	cfg := loadGoreleaserConfig(t)
	if len(cfg.Nfpms) == 0 {
		t.Fatal(".goreleaser.yaml has no nfpms entries")
	}
	nfpm := cfg.Nfpms[0]

	// nFPM auto-packages both build ids' binaries (chairlift,
	// chairlift-updex-helper) into bindir, so bindir alone determines where
	// the packaged updex helper lands. It must match the directory of
	// internal/updex.HelperPath — the fixed absolute path pkexec matches
	// against the PolicyKit policy's exec.path annotation — not just a
	// hardcoded "/usr/bin" literal, so this fails if either side changes
	// without the other.
	if wantBindir := filepath.Dir(updex.HelperPath); nfpm.Bindir != wantBindir {
		t.Errorf("nfpms[0].bindir = %q, want %q (must match the directory of internal/updex.HelperPath)", nfpm.Bindir, wantBindir)
	}

	tests := []struct {
		name      string
		srcSuffix string
		want      string
	}{
		{"updex policy", "org.frostyard.ChairLift.updex.policy", filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.updex.policy")},
		{"updex rules", "org.frostyard.ChairLift.updex.rules", filepath.Join(polkitRulesDir, "org.frostyard.ChairLift.updex.rules")},
		{"bootc policy", "org.frostyard.ChairLift.bootc.policy", filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.bootc.policy")},
		{"bootc rules", "org.frostyard.ChairLift.bootc.rules", filepath.Join(polkitRulesDir, "org.frostyard.ChairLift.bootc.rules")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nfpmDst(t, nfpm, tt.srcSuffix)
			if got != tt.want {
				t.Errorf("nfpm contents dst for %s = %q, want %q (fixed PolicyKit read location)", tt.srcSuffix, got, tt.want)
			}
		})
	}
}
