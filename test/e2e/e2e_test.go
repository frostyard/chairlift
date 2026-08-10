package e2e

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const startupTimeout = "15s"

func TestApplicationHelp(t *testing.T) {
	app := filepath.Join(e2eBuildDir(t), "chairlift")
	requireExecutable(t, app)

	cmd := exec.Command(app, "--help")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s --help failed: %v\noutput:\n%s", app, err, output)
	}

	for _, want := range []string{
		"Usage:",
		"Application Options:",
		"--dry-run",
		"Don't make any changes to the system.",
	} {
		if !bytes.Contains(output, []byte(want)) {
			t.Errorf("%s --help output does not contain %q\noutput:\n%s", app, want, output)
		}
	}
}

func TestApplicationStartsInDryRun(t *testing.T) {
	app := filepath.Join(e2eBuildDir(t), "chairlift")
	requireExecutable(t, app)

	timeout := requireCommand(t, "timeout")
	dbusRunSession := requireCommand(t, "dbus-run-session")
	xvfbRun := requireCommand(t, "xvfb-run")

	cmd := exec.Command(
		timeout,
		"--signal=TERM",
		"--kill-after=5s",
		startupTimeout,
		dbusRunSession,
		"--",
		xvfbRun,
		"-a",
		app,
		"--dry-run",
	)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(
		os.Environ(),
		"LANG=C",
		"LC_ALL=C",
		"NO_AT_BRIDGE=1",
		"GTK_A11Y=none",
		"GSETTINGS_BACKEND=memory",
		"HOME="+t.TempDir(),
	)
	output, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 124 {
		t.Fatalf("ChairLift exited before the %s smoke window elapsed: %v\noutput:\n%s", startupTimeout, err, output)
	}

	for _, want := range []string{
		"Running in dry-run mode",
		"ChairLift activated",
		"app: window presented",
	} {
		if !bytes.Contains(output, []byte(want)) {
			t.Errorf("ChairLift dry-run startup output does not contain %q\noutput:\n%s", want, output)
		}
	}
}

func TestInstalledBundleAndHelperBoundary(t *testing.T) {
	root := repoRoot(t)
	buildDir := e2eBuildDir(t)
	stage := t.TempDir()

	cmd := exec.Command(
		"make",
		"install",
		"DESTDIR="+stage,
		"PREFIX=/usr",
		"BUILD_DIR="+buildDir,
	)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("staged make install failed: %v\noutput:\n%s", err, output)
	}

	executables := []string{
		"usr/bin/chairlift",
		"usr/bin/chairlift-wrapper",
		"usr/bin/chairlift-updex-helper",
	}
	for _, path := range executables {
		requireExecutable(t, filepath.Join(stage, path))
	}

	for _, path := range []string{
		"usr/share/chairlift/config.yml",
		"usr/share/applications/org.frostyard.ChairLift.desktop",
		"usr/share/icons/hicolor/scalable/apps/org.frostyard.ChairLift.svg",
		"usr/share/icons/hicolor/scalable/apps/org.frostyard.ChairLift-flower.svg",
		"usr/share/icons/hicolor/symbolic/apps/org.frostyard.ChairLift-symbolic.svg",
		"usr/share/polkit-1/actions/org.frostyard.ChairLift.bootc.policy",
		"usr/share/polkit-1/actions/org.frostyard.ChairLift.updex.policy",
	} {
		if info, err := os.Stat(filepath.Join(stage, path)); err != nil {
			t.Errorf("staged install is missing %s: %v", path, err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("staged install path %s has mode %s, want regular file", path, info.Mode())
		}
	}

	if _, err := os.Stat(filepath.Join(stage, "etc/chairlift/config.yml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("staged install created administrator-owned /etc/chairlift/config.yml: %v", err)
	}

	helper := filepath.Join(stage, "usr/bin/chairlift-updex-helper")
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "missing command", wantStderr: "usage: chairlift-updex-helper"},
		{name: "unknown command", args: []string{"unknown"}, wantStderr: "unknown command: unknown"},
		{name: "missing feature", args: []string{"enable-feature"}, wantStderr: "usage: chairlift-updex-helper enable-feature"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command(helper, test.args...)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("%s %s error = %v, want exit status 1", helper, strings.Join(test.args, " "), err)
			}
			if stdout.Len() != 0 {
				t.Errorf("%s %s wrote unexpected stdout %q", helper, strings.Join(test.args, " "), stdout.String())
			}
			if !strings.Contains(stderr.String(), test.wantStderr) {
				t.Errorf("%s %s stderr = %q, want substring %q", helper, strings.Join(test.args, " "), stderr.String(), test.wantStderr)
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate the E2E test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func e2eBuildDir(t *testing.T) string {
	t.Helper()

	dir := os.Getenv("CHAIRLIFT_E2E_BUILD_DIR")
	if dir == "" {
		t.Skip("E2E prerequisites are prepared by `make e2e`")
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve CHAIRLIFT_E2E_BUILD_DIR %q: %v", dir, err)
	}
	return absolute
}

func requireExecutable(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("required executable %s is unavailable: %v", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("required executable %s has mode %s", path, info.Mode())
	}
}

func requireCommand(t *testing.T, name string) string {
	t.Helper()

	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("required E2E command %q is unavailable: %v", name, err)
	}
	return path
}
