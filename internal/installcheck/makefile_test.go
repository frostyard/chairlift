package installcheck

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/chairlift/internal/updex"
)

// PolicyKit's polkitd is compiled with a fixed actions/rules directory and
// does not consult PREFIX or XDG_DATA_DIRS (see the Makefile's PREFIX
// comment and yeti/OVERVIEW.md's "Privileged operations" section), so these
// two locations must never move regardless of what PREFIX resolves to.
const (
	polkitActionsDir = "/usr/share/polkit-1/actions"
	polkitRulesDir   = "/usr/share/polkit-1/rules.d"
)

// runMakeInstallDryRun runs `make -n install DESTDIR=<destDir> [extraArgs...]`
// from the repo root and returns its combined output. `-n` is a dry run:
// make prints the recipe's shell command lines without executing them, so
// this never compiles anything, writes outside destDir, or requires root
// (confirmed by hand during planning and again here: the real `install`
// target's first two recipe lines are `go build` invocations that a dry run
// only prints).
func runMakeInstallDryRun(t *testing.T, destDir string, extraArgs ...string) string {
	t.Helper()

	args := append([]string{"-n", "install", "DESTDIR=" + destDir}, extraArgs...)
	cmd := exec.Command("make", args...)
	cmd.Dir = RepoRoot()

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("make %s failed: %v\noutput:\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

// assertInstallsHelperAndPolicies checks that a `make -n install` dry-run's
// output places the updex helper binary at DESTDIR+updex.HelperPath (cross-
// referencing internal/updex.HelperPath as the single source of truth for
// both the helper's installed directory and file name, per c1) and both
// PolicyKit policy/rules pairs under DESTDIR + the fixed polkit-1
// directories.
func assertInstallsHelperAndPolicies(t *testing.T, output, destDir string) {
	t.Helper()

	wantHelper := filepath.Join("build", filepath.Base(updex.HelperPath)) +
		" " + filepath.Join(destDir, updex.HelperPath)
	if !strings.Contains(output, wantHelper) {
		t.Errorf("make -n install output does not install the updex helper at DESTDIR+HelperPath\nwant substring: %q\noutput:\n%s", wantHelper, output)
	}

	for _, rel := range []string{
		filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.updex.policy"),
		filepath.Join(polkitRulesDir, "org.frostyard.ChairLift.updex.rules"),
		filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.bootc.policy"),
		filepath.Join(polkitRulesDir, "org.frostyard.ChairLift.bootc.rules"),
	} {
		want := filepath.Join(destDir, rel)
		if !strings.Contains(output, want) {
			t.Errorf("make -n install output does not target %q (fixed PolicyKit read location)\noutput:\n%s", want, output)
		}
	}
}

// TestMakefileInstallUsesUsrPrefix is a real, gated consistency check
// between the Makefile's install recipe and internal/updex.HelperPath: it
// fails if either changes without the other, or if the Makefile's PREFIX
// default (or an explicit PREFIX=/usr) stops deriving the helper and
// policy/rules destinations from /usr.
func TestMakefileInstallUsesUsrPrefix(t *testing.T) {
	t.Run("default PREFIX", func(t *testing.T) {
		destDir := t.TempDir()
		out := runMakeInstallDryRun(t, destDir)
		assertInstallsHelperAndPolicies(t, out, destDir)
	})

	t.Run("explicit PREFIX=/usr", func(t *testing.T) {
		destDir := t.TempDir()
		out := runMakeInstallDryRun(t, destDir, "PREFIX=/usr")
		assertInstallsHelperAndPolicies(t, out, destDir)
	})
}
