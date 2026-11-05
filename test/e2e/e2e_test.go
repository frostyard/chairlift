package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	startupTimeout  = 30 * time.Second
	stabilityWindow = time.Second
	shutdownTimeout = 5 * time.Second
)

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
		"--first-run",
		"--send-pending-reports",
	} {
		if !bytes.Contains(output, []byte(want)) {
			t.Errorf("%s --help output does not contain %q\noutput:\n%s", app, want, output)
		}
	}
}

// TestSendPendingReportsDryRunNeedsNoWindow proves the windowless mode of
// ADR-0015: `chairlift --send-pending-reports --dry-run` exits 0 with no
// window and no GTK on a machine with no consent, says that reporting is off,
// and writes no sent log.
func TestSendPendingReportsDryRunNeedsNoWindow(t *testing.T) {
	app := filepath.Join(e2eBuildDir(t), "chairlift")
	requireExecutable(t, app)

	home := t.TempDir()
	configHome := filepath.Join(home, "config")
	stateHome := filepath.Join(home, "state")
	cmd := exec.Command(app, "--send-pending-reports", "--dry-run")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(),
		"LANG=C",
		"LC_ALL=C",
		"HOME="+home,
		"XDG_CONFIG_HOME="+configHome,
		"XDG_STATE_HOME="+stateHome,
		// Keep the GTK libraries out of the way: the mode needs no display.
		"DISPLAY=",
		"WAYLAND_DISPLAY=",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s --send-pending-reports --dry-run failed: %v\noutput:\n%s", app, err, output)
	}
	if !bytes.Contains(output, []byte("problem reporting is off")) {
		t.Errorf("output does not say that reporting is off:\n%s", output)
	}
	for _, path := range []string{
		filepath.Join(stateHome, "chairlift", "reports.json"),
		filepath.Join(home, ".local", "state", "chairlift", "reports.json"),
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("the dry run wrote the sent log %s", path)
		}
	}
}

func TestApplicationStartsInDryRun(t *testing.T) {
	app := filepath.Join(e2eBuildDir(t), "chairlift")
	requireExecutable(t, app)

	dbusRunSession := requireCommand(t, "dbus-run-session")
	xvfbRun := requireCommand(t, "xvfb-run")

	cmd := exec.Command(
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
		// The Problems page reads its sources from fixtures under
		// --dry-run (ADR-0015): no snosi-health file, coredumpctl or
		// journalctl of the host.
		"CHAIRLIFT_PROBLEMS_FIXTURES="+problemsFixtures(t),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output := &lockedBuffer{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ChairLift dry-run smoke process: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	want := []string{
		"Running in dry-run mode",
		"ChairLift activated",
		"app: window presented",
		// 17 system problems, 1 merged into the apps group; 4 app record
		// groups and 3 crash groups of this user.
		"problems: page built system=16 apps=7 source=ok",
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(startupTimeout)
	defer timer.Stop()
	var readyAt time.Time

	for {
		select {
		case err := <-done:
			t.Fatalf("ChairLift exited before startup readiness: %v\noutput:\n%s", err, output.String())
		case <-ticker.C:
			if missingOutput(output.String(), want) != "" {
				continue
			}
			if readyAt.IsZero() {
				readyAt = time.Now()
				continue
			}
			if time.Since(readyAt) >= stabilityWindow {
				stopProcessGroup(t, cmd, done)
				logLinesWith(t, output.String(), "problems: ")
				return
			}
		case <-timer.C:
			missing := missingOutput(output.String(), want)
			stopProcessGroup(t, cmd, done)
			t.Fatalf("ChairLift did not become ready within %s; missing %s\noutput:\n%s",
				startupTimeout, missing, output.String())
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
		"usr/bin/chairlift-first-run",
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
		"usr/share/polkit-1/actions/org.frostyard.ChairLift.sysupdate.policy",
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

	// ChairLift starts nothing on its own: the image that wants a first run
	// ships the autostart entry (ADR-0014).
	if _, err := os.Stat(filepath.Join(stage, "etc/xdg/autostart")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("staged install created etc/xdg/autostart: %v", err)
	}
	if err := filepath.WalkDir(stage, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(entry.Name()), "autostart") {
			t.Errorf("staged install contains autostart path %s", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk staged install: %v", err)
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

func TestFirstRunStartsInDryRun(t *testing.T) {
	app := filepath.Join(e2eBuildDir(t), "chairlift")
	requireExecutable(t, app)

	dbusRunSession := requireCommand(t, "dbus-run-session")
	xvfbRun := requireCommand(t, "xvfb-run")

	home := t.TempDir()
	configHome := filepath.Join(home, "config")
	cmd := exec.Command(
		dbusRunSession,
		"--",
		xvfbRun,
		"-a",
		app,
		"--first-run",
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
		"HOME="+home,
		"XDG_CONFIG_HOME="+configHome,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output := &lockedBuffer{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ChairLift first-run dry-run smoke process: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	want := []string{
		"Running in dry-run mode",
		"Running in first-run mode",
		"first-run: window presented",
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(startupTimeout)
	defer timer.Stop()
	var readyAt time.Time

	for ready := false; !ready; {
		select {
		case err := <-done:
			t.Fatalf("ChairLift first run exited before startup readiness: %v\noutput:\n%s", err, output.String())
		case <-ticker.C:
			if missingOutput(output.String(), want) != "" {
				continue
			}
			if readyAt.IsZero() {
				readyAt = time.Now()
				continue
			}
			if time.Since(readyAt) >= stabilityWindow {
				stopProcessGroup(t, cmd, done)
				ready = true
			}
		case <-timer.C:
			missing := missingOutput(output.String(), want)
			stopProcessGroup(t, cmd, done)
			t.Fatalf("ChairLift first run did not become ready within %s; missing %s\noutput:\n%s",
				startupTimeout, missing, output.String())
		}
	}

	for _, marker := range []string{
		filepath.Join(configHome, "chairlift", "first-run.done"),
		filepath.Join(configHome, "snow-first-setup.done"),
		filepath.Join(home, ".config", "chairlift", "first-run.done"),
		filepath.Join(home, ".config", "snow-first-setup.done"),
	} {
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("dry-run first run left marker %s: %v", marker, err)
		}
	}
}

func TestFirstRunGateHelp(t *testing.T) {
	gate := filepath.Join(e2eBuildDir(t), "chairlift-first-run")
	requireExecutable(t, gate)

	cmd := exec.Command(gate, "--help")
	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s --help failed: %v\nstderr:\n%s", gate, err, stderr.String())
	}
	for _, want := range []string{"Usage: chairlift-first-run", "--mark-done", "--dry-run", "--help"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("%s --help output does not contain %q\noutput:\n%s", gate, want, stdout.String())
		}
	}

	cmd = exec.Command(gate, "--bogus")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Errorf("%s --bogus error = %v, want exit status 2", gate, err)
	}
}

// firstRunHome is a home and config home for one gate run. When the test
// runs as root (a container), the gate runs as nobody, because the gate
// skips root on purpose; the directories are then world-writable.
type firstRunHome struct {
	home       string
	configHome string
	credential *syscall.Credential
}

func newFirstRunHome(t *testing.T) firstRunHome {
	t.Helper()
	dir, err := os.MkdirTemp("", "chairlift-first-run-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	h := firstRunHome{home: dir, configHome: filepath.Join(dir, "config")}
	if os.Geteuid() == 0 {
		if err := os.Chmod(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		h.credential = &syscall.Credential{Uid: 65534, Gid: 65534}
	}
	return h
}

func (h firstRunHome) markers() []string {
	return []string{
		filepath.Join(h.configHome, "chairlift", "first-run.done"),
		filepath.Join(h.configHome, "snow-first-setup.done"),
	}
}

// run runs the gate binary and returns its exit code and output. The
// timeout proves that a skip exits at once.
func (h firstRunHome) run(t *testing.T, gate string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(gate, args...)
	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C", "HOME="+h.home, "XDG_CONFIG_HOME="+h.configHome)
	if h.credential != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: h.credential}
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", gate, err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case err := <-waited:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), output.String()
		}
		if err != nil {
			t.Fatalf("%s %v: %v", gate, args, err)
		}
		return 0, output.String()
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		if err := <-waited; err != nil {
			t.Logf("%s %v killed: %v", gate, args, err)
		}
		t.Fatalf("%s %v did not exit within 10s\noutput:\n%s", gate, args, output.String())
	}
	return -1, ""
}

func TestFirstRunGateMarkersSkipTheFirstRun(t *testing.T) {
	gate := filepath.Join(e2eBuildDir(t), "chairlift-first-run")
	requireExecutable(t, gate)

	t.Run("mark-done then gate", func(t *testing.T) {
		h := newFirstRunHome(t)
		code, output := h.run(t, gate, "--mark-done")
		if code != 0 {
			t.Fatalf("--mark-done exit = %d\noutput:\n%s", code, output)
		}
		for _, marker := range h.markers() {
			if _, err := os.Stat(marker); err != nil {
				t.Errorf("--mark-done did not write %s: %v", marker, err)
			}
		}
		code, output = h.run(t, gate)
		if code != 0 || !strings.Contains(output, "first-run: already done") {
			t.Fatalf("gate after --mark-done = exit %d, output %q; want exit 0 and no exec", code, output)
		}
	})

	t.Run("legacy marker only", func(t *testing.T) {
		h := newFirstRunHome(t)
		legacy := h.markers()[1]
		if err := os.MkdirAll(filepath.Dir(legacy), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacy, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		code, output := h.run(t, gate)
		if code != 0 || !strings.Contains(output, "first-run: already done") {
			t.Fatalf("gate with only snow-first-setup.done = exit %d, output %q; want exit 0 and no exec", code, output)
		}
		if _, err := os.Stat(h.markers()[0]); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("gate wrote %s: %v", h.markers()[0], err)
		}
	})

	t.Run("mark-done dry run writes nothing", func(t *testing.T) {
		h := newFirstRunHome(t)
		code, output := h.run(t, gate, "--mark-done", "--dry-run")
		if code != 0 || !strings.Contains(output, "[DRY-RUN] would write first-run markers:") {
			t.Fatalf("--mark-done --dry-run = exit %d, output %q", code, output)
		}
		for _, marker := range h.markers() {
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("--mark-done --dry-run wrote %s: %v", marker, err)
			}
		}
	})
}

type lockedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (buffer *lockedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.Write(data)
}

func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.String()
}

// logLinesWith logs every output line that contains marker, so a passing
// run shows the Problems page's marker line.
func logLinesWith(t *testing.T, output, marker string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, marker) {
			t.Log(line)
		}
	}
}

func missingOutput(output string, markers []string) string {
	missing := make([]string, 0, len(markers))
	for _, marker := range markers {
		if !strings.Contains(output, marker) {
			missing = append(missing, fmt.Sprintf("%q", marker))
		}
	}
	return strings.Join(missing, ", ")
}

func stopProcessGroup(t *testing.T, cmd *exec.Cmd, done <-chan error) {
	t.Helper()
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Errorf("terminate ChairLift smoke process group: %v", err)
	}
	// A deliberate shutdown makes Wait report the signal, so the exit status
	// is logged rather than treated as a test failure.
	select {
	case waitErr := <-done:
		if waitErr != nil {
			t.Logf("ChairLift smoke process stopped: %v", waitErr)
		}
		return
	case <-time.After(shutdownTimeout):
	}

	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Errorf("kill ChairLift smoke process group: %v", err)
	}
	if waitErr := <-done; waitErr != nil {
		t.Logf("ChairLift smoke process killed: %v", waitErr)
	}
}

// fixtureUIDPlaceholder is the uid of the crashes in
// testdata/problems/coredumps.json that belong to the user who runs the
// test; problemsFixtures replaces it with that user's uid.
const fixtureUIDPlaceholder = `"uid": 60999`

// problemsFixtures copies testdata/problems into a temporary folder for the
// CHAIRLIFT_PROBLEMS_FIXTURES hook and returns its absolute path.
func problemsFixtures(t *testing.T) string {
	t.Helper()
	src := filepath.Join(repoRoot(t), "test", "e2e", "testdata", "problems")
	dst := t.TempDir()
	for _, name := range []string{"problems.json", "details.json", "coredumps.json", "journal.json"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("read problems fixture: %v", err)
		}
		if name == "coredumps.json" {
			if !bytes.Contains(data, []byte(fixtureUIDPlaceholder)) {
				t.Fatalf("%s has no %s placeholder", name, fixtureUIDPlaceholder)
			}
			data = bytes.ReplaceAll(data, []byte(fixtureUIDPlaceholder), []byte(fmt.Sprintf(`"uid": %d`, os.Getuid())))
		}
		if err := os.WriteFile(filepath.Join(dst, name), data, 0o644); err != nil {
			t.Fatalf("write problems fixture: %v", err)
		}
	}
	return dst
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
