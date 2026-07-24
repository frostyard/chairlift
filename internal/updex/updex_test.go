package updex

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeFakePkexec writes an executable shell script standing in for pkexec:
// it records its own argv (one element per line) to capturedArgsFile and
// exits 0. It never execs the real pkexec or requires root.
func writeFakePkexec(t *testing.T, capturedArgsFile string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-pkexec")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + capturedArgsFile + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake pkexec: %v", err)
	}
	return path
}

func TestRunHelperNonDryRunInvokesPkexecWithAbsoluteHelperPath(t *testing.T) {
	SetDryRun(false)

	capturedArgsFile := filepath.Join(t.TempDir(), "captured-args")
	fakePkexec := writeFakePkexec(t, capturedArgsFile)

	ctx := context.Background()
	if _, _, err := runHelper(ctx, fakePkexec, "enable-feature", "demo"); err != nil {
		t.Fatalf("runHelper: %v", err)
	}

	data, err := os.ReadFile(capturedArgsFile)
	if err != nil {
		t.Fatalf("reading captured pkexec argv: %v", err)
	}
	got := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	want := []string{HelperPath, "enable-feature", "demo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pkexec argv = %v, want %v", got, want)
	}
	if got[0] != "/usr/bin/chairlift-updex-helper" {
		t.Fatalf("helper path passed to pkexec = %q, want the fixed absolute path matching data/org.frostyard.ChairLift.updex.policy's exec.path annotation", got[0])
	}
}

func TestRunHelperDryRunNeverInvokesPkexec(t *testing.T) {
	SetDryRun(true)
	defer SetDryRun(false)

	// A path that does not exist: if runHelper failed to short-circuit and
	// tried to actually run it, cmd.Run() would return an error and this
	// test would fail loudly instead of silently passing.
	nonexistentPkexec := filepath.Join(t.TempDir(), "pkexec-should-never-run")

	ctx := context.Background()
	stdout, stderr, err := runHelper(ctx, nonexistentPkexec, "enable-feature", "demo")
	if err != nil {
		t.Fatalf("runHelper dry-run returned error, want short-circuit with nil error: %v", err)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("runHelper dry-run returned stdout=%q stderr=%q, want both empty", stdout, stderr)
	}
}

func TestEnableDisableUpdateFeaturesDryRunNeverInvokePkexec(t *testing.T) {
	SetDryRun(true)
	defer SetDryRun(false)

	ctx := context.Background()
	if err := EnableFeature(ctx, "demo"); err != nil {
		t.Errorf("EnableFeature dry-run: %v", err)
	}
	if err := DisableFeature(ctx, "demo"); err != nil {
		t.Errorf("DisableFeature dry-run: %v", err)
	}
	if err := UpdateFeatures(ctx); err != nil {
		t.Errorf("UpdateFeatures dry-run: %v", err)
	}
}
