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

// wantSPDXLicense is the SPDX identifier the project's actual license (GPLv3
// text in LICENSE, gtk.LicenseGpl30Value — the GTK bindings' "GPL 3.0 or later"
// enum value — in internal/window/window.go's about dialog) resolves to.
// .goreleaser.yaml's top-level metadata.license and every nfpms[] entry's
// license must agree with this constant; see
// yeti/package-managers.md's "Install-path consistency" section for the
// MIT/GPL drift bug this guards against.
const wantSPDXLicense = "GPL-3.0-or-later"

// wantHomepage is the repository URL .goreleaser.yaml's metadata.homepage must
// declare. It is the single source of truth for the repository URL in that
// file: release.footer's "Full Changelog" link derives from
// {{ .Metadata.Homepage }} rather than repeating an owner literal, so a silent
// edit here would silently redirect every generated release note.
const wantHomepage = "https://github.com/frostyard/chairlift"

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
		{"maintainer config", "config.yml", "/usr/share/chairlift/config.yml"},
		{"updex policy", "org.frostyard.ChairLift.updex.policy", filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.updex.policy")},
		{"bootc policy", "org.frostyard.ChairLift.bootc.policy", filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.bootc.policy")},
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
						t.Errorf("nfpms[%d] contents dst for %s = %q, want required package path %q", i, cc.srcSuffix, got, cc.want)
					}
				})
			}

			for _, content := range nfpm.Contents {
				if content.Dst == "/etc/chairlift/config.yml" {
					t.Errorf("nfpms[%d] packages administrator-owned /etc/chairlift/config.yml", i)
				}
				if strings.HasSuffix(content.Src, ".rules") || strings.HasSuffix(content.Dst, ".rules") {
					t.Errorf("nfpms[%d] still packages passwordless PolicyKit rule: src=%q dst=%q", i, content.Src, content.Dst)
				}
			}
		})
	}
}

// TestGoreleaserLicenseIsGPL parses the real .goreleaser.yaml and asserts
// both the top-level metadata.license and every nfpms[] entry's license
// equal wantSPDXLicense, so a future edit reverting either field back to
// MIT (or any other value) — as happened before this test was added — fails
// this test instead of silently shipping mislabeled deb/rpm/apk packages.
//
// It iterates every nfpms[] entry rather than only nfpms[0]: per
// docs/agents/skills/regression-tests-must-cover-every-collection-entry.md,
// a consistency check that only special-cases the first entry stops
// protecting anything the moment a second nfpms[] entry is added or
// reordered with the wrong license.
func TestGoreleaserLicenseIsGPL(t *testing.T) {
	cfg := loadGoreleaserConfig(t)

	if cfg.Metadata.License != wantSPDXLicense {
		t.Errorf("metadata.license = %q, want %q", cfg.Metadata.License, wantSPDXLicense)
	}

	if len(cfg.Nfpms) == 0 {
		t.Fatal(".goreleaser.yaml has no nfpms entries")
	}

	for i, nfpm := range cfg.Nfpms {
		t.Run(fmt.Sprintf("nfpms[%d]", i), func(t *testing.T) {
			if nfpm.License != wantSPDXLicense {
				t.Errorf("nfpms[%d].license = %q, want %q", i, nfpm.License, wantSPDXLicense)
			}
		})
	}
}

// TestGoreleaserMetadataHomepageIsFrostyardRepo parses the real
// .goreleaser.yaml and asserts metadata.homepage still names this
// repository. The value became load-bearing when release.footer started
// deriving its Full Changelog URL from {{ .Metadata.Homepage }}: a drifting
// homepage now silently points every release note's changelog link at the
// wrong repository, which this test catches instead.
func TestGoreleaserMetadataHomepageIsFrostyardRepo(t *testing.T) {
	cfg := loadGoreleaserConfig(t)

	if cfg.Metadata.Homepage != wantHomepage {
		t.Errorf("metadata.homepage = %q, want %q (single source of truth for the repository URL, consumed by release.footer)", cfg.Metadata.Homepage, wantHomepage)
	}
}

// TestGoreleaserReleaseFooterUsesMetadataHomepage parses the real
// .goreleaser.yaml and asserts release.footer's "Full Changelog" line builds
// its URL from the {{ .Metadata.Homepage }} template variable rather than
// from a hardcoded repository owner literal.
//
// The assertions are on the raw *template text* read from the YAML: the
// footer is expanded by goreleaser at release time, and goreleaser (Pro) is
// not installed on the gate host or in make ci, so nothing here renders the
// template or shells out.
//
// It is deliberately non-vacuous: an absent or empty footer, or zero or more
// than one "Full Changelog" line, is a t.Fatal rather than a silent pass.
func TestGoreleaserReleaseFooterUsesMetadataHomepage(t *testing.T) {
	cfg := loadGoreleaserConfig(t)

	footer := cfg.Release.Footer
	if strings.TrimSpace(footer) == "" {
		t.Fatal("release.footer is absent or empty in .goreleaser.yaml")
	}

	var matches []string
	for _, line := range strings.Split(footer, "\n") {
		if strings.Contains(line, "Full Changelog") {
			matches = append(matches, line)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("release.footer has %d lines containing %q, want exactly 1; footer:\n%s", len(matches), "Full Changelog", footer)
	}
	line := matches[0]

	if !strings.Contains(line, "{{ .Metadata.Homepage }}") {
		t.Errorf("Full Changelog line = %q, want it to derive its repository URL from the %q template variable", line, "{{ .Metadata.Homepage }}")
	}
	if !strings.Contains(line, "/compare/{{ .PreviousTag }}...{{ .Tag }}") {
		t.Errorf("Full Changelog line = %q, want it to keep the %q suffix", line, "/compare/{{ .PreviousTag }}...{{ .Tag }}")
	}
	if strings.Contains(line, ".ProjectName") {
		t.Errorf("Full Changelog line = %q, want no .ProjectName: metadata.homepage already ends in the repository name, so the two must not be concatenated", line)
	}

	// The footer must contain no repository URL literal at all. Asserting the
	// absence of "github.com/" (rather than of the previous owner's name)
	// also catches the URL being rewritten to a hardcoded current-owner
	// literal, which would reintroduce the duplication this guards against —
	// and it keeps the removed literal out of the tree, per
	// docs/agents/skills/grep-removal-criteria-must-exclude-mill-and-tests.md.
	// The surviving https://goreleaser.com/pro link is unaffected.
	if strings.Contains(footer, "github.com/") {
		t.Errorf("release.footer contains a hardcoded repository URL literal; it must reference {{ .Metadata.Homepage }} instead. footer:\n%s", footer)
	}
}
