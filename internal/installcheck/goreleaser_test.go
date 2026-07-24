package installcheck

import (
	"fmt"
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
//
// It iterates every nfpms[] entry rather than only nfpms[0]: the acceptance
// criterion is a consistency invariant across the whole nFPM block, so adding
// or reordering a second package with the wrong bindir or polkit destinations
// must fail here too (per
// docs/agents/skills/regression-tests-must-cover-every-collection-entry.md).
func TestGoreleaserNfpmLayoutMatchesUsrPrefix(t *testing.T) {
	cfg := loadGoreleaserConfig(t)
	if len(cfg.Nfpms) == 0 {
		t.Fatal(".goreleaser.yaml has no nfpms entries")
	}

	// nFPM auto-packages both build ids' binaries (chairlift,
	// chairlift-updex-helper) into bindir, so bindir alone determines where
	// the packaged updex helper lands. It must match the directory of
	// internal/updex.HelperPath — the fixed absolute path pkexec matches
	// against the PolicyKit policy's exec.path annotation — not just a
	// hardcoded "/usr/bin" literal, so this fails if either side changes
	// without the other.
	wantBindir := filepath.Dir(updex.HelperPath)

	contentChecks := []struct {
		name      string
		srcSuffix string
		want      string
	}{
		{"updex policy", "org.frostyard.ChairLift.updex.policy", filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.updex.policy")},
		{"updex rules", "org.frostyard.ChairLift.updex.rules", filepath.Join(polkitRulesDir, "org.frostyard.ChairLift.updex.rules")},
		{"bootc policy", "org.frostyard.ChairLift.bootc.policy", filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.bootc.policy")},
		{"bootc rules", "org.frostyard.ChairLift.bootc.rules", filepath.Join(polkitRulesDir, "org.frostyard.ChairLift.bootc.rules")},
	}

	for i, nfpm := range cfg.Nfpms {
		t.Run(fmt.Sprintf("nfpms[%d]", i), func(t *testing.T) {
			if nfpm.Bindir != wantBindir {
				t.Errorf("nfpms[%d].bindir = %q, want %q (must match the directory of internal/updex.HelperPath)", i, nfpm.Bindir, wantBindir)
			}

			for _, cc := range contentChecks {
				t.Run(cc.name, func(t *testing.T) {
					got := nfpmDst(t, nfpm, cc.srcSuffix)
					if got != cc.want {
						t.Errorf("nfpms[%d] contents dst for %s = %q, want %q (fixed PolicyKit read location)", i, cc.srcSuffix, got, cc.want)
					}
				})
			}
		})
	}
}
