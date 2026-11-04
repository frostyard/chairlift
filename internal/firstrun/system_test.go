package firstrun

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/i18n"
)

var (
	firefox = App{Name: "Firefox", ID: "org.mozilla.firefox", Active: true}
	boxes   = App{Name: "Boxes", ID: "org.gnome.Boxes", Active: true}
)

func TestPlanActions(t *testing.T) {
	if got := PlanActions(nil); got != nil {
		t.Fatalf("PlanActions(nil) = %v, want no action (no remote-add without an app)", got)
	}
	got := PlanActions([]App{firefox, boxes})
	want := []Action{{Kind: ActionAddFlathub}, {Kind: ActionInstall, App: firefox}, {Kind: ActionInstall, App: boxes}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanActions() = %+v, want %+v", got, want)
	}
}

func TestActionTitle(t *testing.T) {
	if got := (Action{Kind: ActionAddFlathub}).Title(); got != i18n.MsgSettingUpFlathub {
		t.Errorf("remote title = %q", got)
	}
	if got := (Action{Kind: ActionInstall, App: firefox}).Title(); got != "Installing Firefox" {
		t.Errorf("install title = %q", got)
	}
}

func TestExecuteRunsEveryActionAndCollectsErrors(t *testing.T) {
	actions := PlanActions([]App{firefox, boxes})
	failure := errors.New("remote failed")
	var ran []Action
	var events []Event
	report := Execute(actions, func(a Action) error {
		ran = append(ran, a)
		if a.Kind == ActionAddFlathub {
			return failure
		}
		return nil
	}, func(e Event) { events = append(events, e) })

	if !reflect.DeepEqual(ran, actions) {
		t.Fatalf("ran %v, want every action %v even after a failure", ran, actions)
	}
	wantEvents := []Event{
		{Index: 0, State: ActionRunning}, {Index: 0, State: ActionFailed, Err: failure},
		{Index: 1, State: ActionRunning}, {Index: 1, State: ActionSucceeded},
		{Index: 2, State: ActionRunning}, {Index: 2, State: ActionSucceeded},
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %+v, want %+v", events, wantEvents)
	}
	if !reflect.DeepEqual(report.Errors, []error{failure, nil, nil}) || !report.Failed() {
		t.Fatalf("report = %+v, want the remote error only", report)
	}

	entries := ErrorLog(actions, report)
	want := []ErrorEntry{{Title: "Setup failed: " + i18n.MsgSettingUpFlathub, Message: "remote failed"}}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("ErrorLog() = %+v, want %+v", entries, want)
	}
}

func TestExecuteWithoutFailures(t *testing.T) {
	report := Execute(nil, func(Action) error { t.Fatal("ran an action"); return nil }, func(Event) { t.Fatal("emitted an event") })
	if report.Failed() || len(ErrorLog(nil, report)) != 0 {
		t.Fatalf("empty report = %+v", report)
	}
	actions := PlanActions([]App{firefox})
	report = Execute(actions, func(Action) error { return nil }, func(Event) {})
	if report.Failed() || ErrorLog(actions, report) != nil {
		t.Fatalf("report = %+v, want no failure and no error log", report)
	}
}

// TestFlatpakRunnerDryRunRunsNothing proves the production runner reaches
// internal/flatpak's dry-run gate: with an empty PATH a real remote-add or
// install would fail, so success means nothing ran.
func TestFlatpakRunnerDryRunRunsNothing(t *testing.T) {
	flatpak.SetDryRun(true)
	t.Cleanup(func() { flatpak.SetDryRun(false) })
	t.Setenv("PATH", t.TempDir())
	for _, action := range PlanActions([]App{firefox}) {
		if err := FlatpakRunner(action); err != nil {
			t.Fatalf("FlatpakRunner(%+v) in dry run = %v, want nil", action, err)
		}
	}
}

func TestFlatpakRunnerArguments(t *testing.T) {
	flatpak.SetDryRun(false)
	dir := t.TempDir()
	capture := filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$@\" >> \"" + capture + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "flatpak"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	for _, action := range PlanActions([]App{firefox}) {
		if err := FlatpakRunner(action); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	want := "remote-add --user --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo\ninstall -y --user org.mozilla.firefox\n"
	if string(data) != want {
		t.Fatalf("flatpak calls = %q, want %q", data, want)
	}
}

func TestColorSchemeArgs(t *testing.T) {
	if got, want := ColorSchemeArgs(SchemeDark), []string{"set", "org.gnome.desktop.interface", "color-scheme", "prefer-dark"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ColorSchemeArgs(dark) = %v, want %v", got, want)
	}
	for _, scheme := range []ColorScheme{SchemeLight, SchemeDark} {
		for _, arg := range ColorSchemeArgs(scheme) {
			if arg == "gtk-theme" {
				t.Fatalf("ColorSchemeArgs(%s) writes gtk-theme", scheme)
			}
		}
	}
}

func withGsettingsRunner(t *testing.T, run func(args ...string) error) {
	t.Helper()
	saved := gsettingsRunner
	gsettingsRunner = run
	t.Cleanup(func() { gsettingsRunner = saved; SetDryRun(false) })
}

func TestApplyColorScheme(t *testing.T) {
	var calls [][]string
	withGsettingsRunner(t, func(args ...string) error {
		calls = append(calls, args)
		return nil
	})

	SetDryRun(false)
	if err := ApplyColorScheme(SchemeLight); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{ColorSchemeArgs(SchemeLight)}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}

	SetDryRun(true)
	if err := ApplyColorScheme(SchemeDark); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("dry run ran gsettings: %v", calls)
	}

	SetDryRun(false)
	if err := ApplyColorScheme("prefer-purple"); err == nil || len(calls) != 1 {
		t.Fatalf("ApplyColorScheme(unknown) = %v with calls %v, want an error and no call", err, calls)
	}
}

func TestGsettingsRunner(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$@\" > \"" + capture + "\"\n[ \"$4\" = prefer-dark ] || { echo bad value >&2; exit 3; }\n"
	if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := gsettingsRunner(ColorSchemeArgs(SchemeDark)...); err != nil {
		t.Fatalf("gsettingsRunner() = %v", err)
	}
	if data, _ := os.ReadFile(capture); string(data) != "set org.gnome.desktop.interface color-scheme prefer-dark\n" {
		t.Fatalf("gsettings args = %q", data)
	}
	err := gsettingsRunner(ColorSchemeArgs(SchemeLight)...)
	if err == nil || !strings.Contains(err.Error(), "bad value") {
		t.Fatalf("gsettingsRunner() failure = %v, want the output in the error", err)
	}
}

type fakeConn struct {
	net.Conn
	closed bool
}

func (c *fakeConn) Close() error { c.closed = true; return nil }

func TestCheckConnectivity(t *testing.T) {
	conn := &fakeConn{}
	var gotNetwork, gotAddress string
	var gotDeadline time.Time
	online := CheckConnectivity(context.Background(), func(ctx context.Context, network, address string) (net.Conn, error) {
		gotNetwork, gotAddress = network, address
		gotDeadline, _ = ctx.Deadline()
		return conn, nil
	})
	if online != ConnOnline || !conn.closed {
		t.Fatalf("CheckConnectivity() = %v, closed %v; want online and the connection closed", online, conn.closed)
	}
	if gotNetwork != "tcp" || gotAddress != "dl.flathub.org:443" {
		t.Fatalf("dialed %s %s, want tcp dl.flathub.org:443", gotNetwork, gotAddress)
	}
	if remaining := time.Until(gotDeadline); remaining <= 0 || remaining > 5*time.Second {
		t.Fatalf("dial deadline in %v, want at most 5s", remaining)
	}

	offline := CheckConnectivity(context.Background(), func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("no route")
	})
	if offline != ConnOffline {
		t.Fatalf("CheckConnectivity(failing dial) = %v, want offline", offline)
	}
	if DefaultDial == nil {
		t.Fatal("DefaultDial is nil")
	}
}

func TestWelcomeName(t *testing.T) {
	env := envOf(map[string]string{"USER": "envuser", "LOGNAME": "logname"})
	tests := []struct {
		name string
		u    *user.User
		err  error
		env  func(string) string
		want string
	}{
		{"GECOS first field", &user.User{Username: "ada", Name: "Ada Lovelace,Room 1,555"}, nil, env, "Ada Lovelace"},
		{"GECOS without commas", &user.User{Username: "ada", Name: " Ada "}, nil, env, "Ada"},
		{"empty GECOS uses login name", &user.User{Username: "ada", Name: ",,,"}, nil, env, "ada"},
		{"lookup error uses USER", nil, errors.New("unknown user"), env, "envuser"},
		{"LOGNAME after USER", nil, errors.New("unknown user"), envOf(map[string]string{"LOGNAME": "logname"}), "logname"},
		{"nothing", &user.User{}, nil, envOf(nil), ""},
	}
	for _, tt := range tests {
		if got := WelcomeName(tt.u, tt.err, tt.env); got != tt.want {
			t.Errorf("%s: WelcomeName() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestSettingsLaunchers(t *testing.T) {
	if got, want := SettingsArgv(PanelAccessibility), []string{"gnome-control-center", "universal-access"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SettingsArgv(accessibility) = %v, want %v", got, want)
	}
	if got, want := SettingsArgv(PanelWifi), []string{"gnome-control-center", "wifi"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SettingsArgv(wifi) = %v, want %v", got, want)
	}
	found := func(name string) (string, error) { return "/usr/bin/" + name, nil }
	missing := func(string) (string, error) { return "", exec.ErrNotFound }
	if !SettingsAvailable(found) || SettingsAvailable(missing) {
		t.Error("SettingsAvailable does not follow the PATH lookup")
	}

	saved := startProcess
	t.Cleanup(func() { startProcess = saved })
	var started [][]string
	startProcess = func(argv []string) error { started = append(started, argv); return nil }
	if err := LaunchSettings(PanelWifi); err != nil {
		t.Fatal(err)
	}
	if err := LaunchSettings("display"); err == nil {
		t.Error("LaunchSettings(unknown panel) = nil, want an error")
	}
	if want := [][]string{SettingsArgv(PanelWifi)}; !reflect.DeepEqual(started, want) {
		t.Fatalf("started %v, want %v", started, want)
	}
}

func TestStartProcess(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	script := filepath.Join(dir, "fake-settings")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch \""+marker+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := startProcess([]string{script, "wifi"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !exists(t, marker) {
		if time.Now().After(deadline) {
			t.Fatal("started process did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := startProcess([]string{filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("startProcess(missing) = nil, want an error")
	}
}
