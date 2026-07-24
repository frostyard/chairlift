// Package installcheck contains headless, gate-enforced regression tests
// that verify the repository's two installation paths — a source `make
// install` and the packaged nFPM (deb/rpm/apk) layout produced from
// .goreleaser.yaml — stay in agreement with each other and with
// internal/updex.HelperPath, the fixed absolute path PolicyKit's
// org.freedesktop.policykit.exec.path annotation requires (see
// internal/updex/updex.go and data/org.frostyard.ChairLift.updex.policy).
//
// This package holds no logic reachable from cmd/ or any other internal/
// package — only shared test-time helpers used by makefile_test.go and
// goreleaser_test.go. It imports no puregotk, directly or transitively, so a
// _test.go living here never trips the gtk-headless-tests.md constraint
// (docs/agents/skills/gtk-headless-tests.md), and it lives under
// internal/... so it is actually exercised by gates_chunk, make ci, and CI's
// `go test ./internal/...` filter, per
// docs/agents/skills/gate-test-scope-is-internal-only.md.
package installcheck

import (
	"path/filepath"
	"runtime"
)

// RepoRoot returns the absolute path to the repository root. It is computed
// from this source file's own location (via runtime.Caller) rather than the
// test binary's working directory: `go test` runs each package's tests with
// the package directory as cwd, not the repo root, and internal/installcheck
// sits two directories below the repo root
// (<root>/internal/installcheck/installcheck.go).
func RepoRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// GoreleaserConfig is a minimal, forward-compatible subset of the fields in
// .goreleaser.yaml this package's tests care about. yaml.v3's default
// unmarshaling silently ignores every field not named here, so this struct
// does not need to (and deliberately does not) mirror the whole schema.
type GoreleaserConfig struct {
	Nfpms []NfpmConfig `yaml:"nfpms"`
}

// NfpmConfig is the subset of an nfpms[] entry relevant to install-location
// consistency: where packaged binaries land (Bindir) and where explicitly
// listed files (policy/rules, wrapper script, icons, ...) land (Contents).
type NfpmConfig struct {
	Bindir   string        `yaml:"bindir"`
	Contents []NfpmContent `yaml:"contents"`
}

// NfpmContent is one nfpm contents[] entry's source/destination pair.
type NfpmContent struct {
	Src string `yaml:"src"`
	Dst string `yaml:"dst"`
}
