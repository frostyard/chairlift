package installcheck

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/chairlift/internal/updex"
)

// PolicyKit's polkitd is compiled with fixed actions/rules directories and
// does not consult PREFIX or XDG_DATA_DIRS (see the Makefile's PREFIX
// comment and yeti/OVERVIEW.md's "Privileged operations" section). ChairLift
// installs policies under actions and removes its obsolete passwordless rules
// from rules.d during source installs.
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

	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("GNU make not installed; skipping Makefile install-layout consistency check")
	}

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

func TestRunMakeInstallDryRunSkipsWithoutMake(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	returned := false
	t.Run("missing make", func(t *testing.T) {
		runMakeInstallDryRun(t, t.TempDir())
		returned = true
	})
	if returned {
		t.Fatal("runMakeInstallDryRun returned when make was unavailable")
	}
}

// assertInstallsPackageLayout checks that a `make -n install` dry-run's
// output places the updex helper binary at DESTDIR+updex.HelperPath (cross-
// referencing internal/updex.HelperPath as the single source of truth for
// both the helper's installed directory and file name, per c1) and both
// PolicyKit policies under DESTDIR + the fixed polkit-1 actions directory. It
// also requires removal of legacy passwordless rules, the package-owned
// maintainer config under /usr/share, and rejection of the administrator-owned
// /etc path.
func assertInstallsPackageLayout(t *testing.T, output, destDir string) {
	t.Helper()

	wantHelper := filepath.Join("build", filepath.Base(updex.HelperPath)) +
		" " + filepath.Join(destDir, updex.HelperPath)
	if !strings.Contains(output, wantHelper) {
		t.Errorf("make -n install output does not install the updex helper at DESTDIR+HelperPath\nwant substring: %q\noutput:\n%s", wantHelper, output)
	}

	for _, rel := range []string{
		filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.updex.policy"),
		filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.bootc.policy"),
		filepath.Join(polkitActionsDir, "org.frostyard.ChairLift.sysupdate.policy"),
	} {
		want := filepath.Join(destDir, rel)
		if !strings.Contains(output, want) {
			t.Errorf("make -n install output does not target required package path %q\noutput:\n%s", want, output)
		}
	}

	for _, name := range []string{
		"org.frostyard.ChairLift.updex.rules",
		"org.frostyard.ChairLift.bootc.rules",
	} {
		legacyRule := filepath.Join(destDir, polkitRulesDir, name)
		if want := "rm -f " + legacyRule; !strings.Contains(output, want) {
			t.Errorf("make install does not remove legacy passwordless rule %q\nwant substring: %q\noutput:\n%s", legacyRule, want, output)
		}
		if unwanted := "install -Dm644 data/" + name; strings.Contains(output, unwanted) {
			t.Errorf("make install still installs passwordless rule %q\nunwanted substring: %q\noutput:\n%s", name, unwanted, output)
		}
	}

	maintainerConfig := filepath.Join(destDir, "/usr/share/chairlift/config.yml")
	if want := "config.yml " + maintainerConfig; !strings.Contains(output, want) {
		t.Errorf("make -n install output does not install config.yml at %q\nwant substring: %q\noutput:\n%s", maintainerConfig, want, output)
	}

	adminConfig := filepath.Join(destDir, "/etc/chairlift/config.yml")
	if strings.Contains(output, adminConfig) {
		t.Errorf("make install must not overwrite administrator config %q\noutput:\n%s", adminConfig, output)
	}
}

// TestMakefileInstallUsesUsrPrefix is a real, gated consistency check
// between the Makefile's install recipe and internal/updex.HelperPath: it
// fails if either changes without the other, or if the Makefile's PREFIX
// default (or an explicit PREFIX=/usr) stops deriving the helper and
// policy destinations from /usr.
func TestMakefileInstallUsesUsrPrefix(t *testing.T) {
	t.Run("default PREFIX", func(t *testing.T) {
		destDir := t.TempDir()
		out := runMakeInstallDryRun(t, destDir)
		assertInstallsPackageLayout(t, out, destDir)
	})

	t.Run("explicit PREFIX=/usr", func(t *testing.T) {
		destDir := t.TempDir()
		out := runMakeInstallDryRun(t, destDir, "PREFIX=/usr")
		assertInstallsPackageLayout(t, out, destDir)
	})
}
