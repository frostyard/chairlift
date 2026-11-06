package installcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCorrectionsBaseRejectsUnresolvableBase guards the corrections-base rule.
// `make ci BASE=<ref>` compares .memory/corrections.jsonl with BASE. The
// comparison may be skipped only when BASE resolves and predates the gate
// (it has no scripts/check-corrections.mjs). A BASE that does not resolve at
// all — a typo, or a ref that was never fetched — used to take the same skip
// branch, so `make ci` passed without any comparison. This test runs the real
// Makefile against a throwaway repository and calls out that bug.
func TestCorrectionsBaseRejectsUnresolvableBase(t *testing.T) {
	for _, tool := range []string{"git", "make"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not on PATH; skipping the corrections-base guard", tool)
		}
	}

	dir := t.TempDir()
	correctionsBaseGit(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write the temp repository's file: %v", err)
	}
	correctionsBaseGit(t, dir, "add", "README.md")
	// Identity and signing live on this command line only: the host's git
	// configuration is never read for them and never modified.
	correctionsBaseGit(t, dir,
		"-c", "user.name=corrections-base-test",
		"-c", "user.email=corrections-base-test@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "-q", "-m", "base commit",
	)

	t.Run("unresolvable base fails", func(t *testing.T) {
		output, err := correctionsBaseMake(t, dir, "does-not-exist")
		if err == nil {
			t.Errorf("corrections-base BASE=does-not-exist exited 0, want non-zero: an unresolvable BASE must not skip the comparison\noutput:\n%s", output)
		}
		if !strings.Contains(output, "does not resolve to a commit") {
			t.Errorf("corrections-base BASE=does-not-exist output does not name the unresolvable BASE\nwant substring: %q\noutput:\n%s", "does not resolve to a commit", output)
		}
	})

	t.Run("base without the script is skipped", func(t *testing.T) {
		output, err := correctionsBaseMake(t, dir, "HEAD")
		if err != nil {
			t.Errorf("corrections-base BASE=HEAD failed, want exit 0: the temp commit has no scripts/check-corrections.mjs, so the comparison is skipped\noutput:\n%s", output)
		}
		if !strings.Contains(output, "predates the corrections gate") {
			t.Errorf("corrections-base BASE=HEAD output does not print the skip notice\nwant substring: %q\noutput:\n%s", "predates the corrections gate", output)
		}
	})

	t.Run("empty base does nothing", func(t *testing.T) {
		if output, err := correctionsBaseMake(t, dir, ""); err != nil {
			t.Errorf("corrections-base BASE= failed, want exit 0 (no BASE means no comparison)\noutput:\n%s", output)
		}
	})
}

// correctionsBaseGit runs one git command in the temp repository and fails
// the test when it does not succeed.
func correctionsBaseGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\noutput:\n%s", strings.Join(args, " "), err, output)
	}
}

// correctionsBaseMake runs the repository Makefile's corrections-base rule in
// dir, with BASE set to base, and returns the combined output. The Makefile is
// named by absolute path because -C changes the working directory first.
func correctionsBaseMake(t *testing.T, dir, base string) (string, error) {
	t.Helper()
	cmd := exec.Command("make",
		"-f", filepath.Join(RepoRoot(), "Makefile"),
		"-C", dir,
		"--no-print-directory",
		"corrections-base",
		"BASE="+base,
	)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	return string(output), err
}
