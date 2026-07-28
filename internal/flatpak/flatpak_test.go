package flatpak

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func installCapturingFlatpak(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	capture := filepath.Join(dir, "args")
	script := filepath.Join(dir, "flatpak")
	source := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CHAIRLIFT_FLATPAK_ARGS\"\n" + body + "\n"
	if err := os.WriteFile(script, []byte(source), 0o755); err != nil {
		t.Fatalf("write fake flatpak: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("CHAIRLIFT_FLATPAK_ARGS", capture)
	return capture
}

func capturedFlatpakArgs(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read captured flatpak arguments: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestParseApplicationList(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		installFlag string
		want        []Application
	}{
		{
			name:        "tab-separated system applications",
			output:      "Firefox\torg.mozilla.firefox\t120.0\tstable\tflathub\tapp/org.mozilla.firefox/x86_64/stable\n",
			installFlag: "--system",
			want: []Application{{
				Name: "Firefox", ApplicationID: "org.mozilla.firefox", Version: "120.0",
				Branch: "stable", Origin: "flathub", Installation: "system",
				Ref: "app/org.mozilla.firefox/x86_64/stable",
			}},
		},
		{
			name:        "space-separated user application",
			output:      "GIMP org.gimp.GIMP 2.10 stable flathub app/org.gimp.GIMP/x86_64/stable",
			installFlag: "--user",
			want: []Application{{
				Name: "GIMP", ApplicationID: "org.gimp.GIMP", Version: "2.10",
				Branch: "stable", Origin: "flathub", Installation: "user",
				Ref: "app/org.gimp.GIMP/x86_64/stable",
			}},
		},
		{
			name:        "malformed and blank rows are skipped",
			output:      "\nnot-enough-fields\n",
			installFlag: "--user",
			want:        nil,
		},
		{name: "empty output", output: "", installFlag: "--system", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseApplicationList(tt.output, tt.installFlag)
			if err != nil {
				t.Fatalf("parseApplicationList() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseApplicationList() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCommandWrappersUseExpectedArguments(t *testing.T) {
	SetDryRun(false)
	t.Cleanup(func() { SetDryRun(false) })

	body := `case "$1" in
list) printf 'Firefox\torg.mozilla.firefox\t120.0\tstable\tflathub\tapp/org.mozilla.firefox/x86_64/stable\n' ;;
remote-ls) printf 'Firefox\torg.mozilla.firefox\t121.0\tstable\tflathub\n' ;;
remotes) printf 'flathub\nvendor\n' ;;
info) printf 'name=Firefox\nversion=120.0\nbranch=stable\norigin=flathub\nruntime=org.freedesktop.Platform\n' ;;
esac`
	capture := installCapturingFlatpak(t, body)

	tests := []struct {
		name string
		run  func() error
		want []string
	}{
		{
			name: "list user applications",
			run: func() error {
				apps, err := ListUserApplications()
				if err == nil && (len(apps) != 1 || apps[0].Installation != "user") {
					return errors.New("user application result was not parsed")
				}
				return err
			},
			want: []string{"list", "--user", "--app", "--columns=name,application,version,branch,origin,ref"},
		},
		{
			name: "list system applications",
			run: func() error {
				apps, err := ListSystemApplications()
				if err == nil && (len(apps) != 1 || apps[0].Installation != "system") {
					return errors.New("system application result was not parsed")
				}
				return err
			},
			want: []string{"list", "--system", "--app", "--columns=name,application,version,branch,origin,ref"},
		},
		{name: "install user", run: func() error { return Install("org.example.App", true) }, want: []string{"install", "-y", "--user", "org.example.App"}},
		{name: "install system", run: func() error { return Install("org.example.App", false) }, want: []string{"install", "-y", "--system", "org.example.App"}},
		{name: "uninstall user", run: func() error { return Uninstall("org.example.App", true) }, want: []string{"uninstall", "-y", "--user", "org.example.App"}},
		{name: "uninstall system", run: func() error { return Uninstall("org.example.App", false) }, want: []string{"uninstall", "-y", "--system", "org.example.App"}},
		{name: "update one user app", run: func() error { return Update("org.example.App", true) }, want: []string{"update", "-y", "--user", "org.example.App"}},
		{name: "update all system apps", run: func() error { return Update("", false) }, want: []string{"update", "-y", "--system"}},
		{
			name: "list user app updates",
			run: func() error {
				updates, err := ListUpdates(true)
				if err == nil && (len(updates) != 1 || updates[0].Installation != "user") {
					return errors.New("user update result was not parsed")
				}
				return err
			},
			want: []string{"remote-ls", "--updates", "--app", "--columns=name,application,version,branch,origin", "--user"},
		},
		{
			name: "list remotes",
			run: func() error {
				remotes, err := GetRemotes(false)
				if err == nil && !reflect.DeepEqual(remotes, []string{"flathub", "vendor"}) {
					return errors.New("remote result was not parsed")
				}
				return err
			},
			want: []string{"remotes", "--columns=name", "--system"},
		},
		{
			name: "application info",
			run: func() error {
				info, err := Info("org.mozilla.firefox", true)
				if err == nil && (info.Name != "Firefox" || info.Installation != "user" ||
					info.Runtime != "org.freedesktop.Platform") {
					return errors.New("application info was not parsed")
				}
				return err
			},
			want: []string{"info", "--show-metadata", "--user", "org.mozilla.firefox"},
		},
		{name: "remove unused", run: func() error { _, err := UninstallUnused(); return err }, want: []string{"uninstall", "--unused", "-y"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err != nil {
				t.Fatalf("wrapper returned error: %v", err)
			}
			if got := capturedFlatpakArgs(t, capture); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("flatpak arguments = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQueryFailuresPropagate(t *testing.T) {
	SetDryRun(false)
	t.Cleanup(func() { SetDryRun(false) })
	installCapturingFlatpak(t, `echo 'query failed' >&2; exit 7`)

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "applications", run: func() error { _, err := ListUserApplications(); return err }},
		{name: "updates", run: func() error { _, err := ListUpdates(false); return err }},
		{name: "remotes", run: func() error { _, err := GetRemotes(true); return err }},
		{name: "info", run: func() error { _, err := Info("org.example.App", false); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), "query failed") {
				t.Fatalf("query error = %v, want propagated stderr", err)
			}
		})
	}
}

func TestDryRunSkipsEveryStateChangingCommand(t *testing.T) {
	SetDryRun(true)
	t.Cleanup(func() { SetDryRun(false) })
	t.Setenv("PATH", t.TempDir())

	for command := range stateChangingCommands {
		t.Run(command, func(t *testing.T) {
			output, err := runFlatpakCommand(command, "--example")
			if err != nil {
				t.Fatalf("runFlatpakCommand dry-run error = %v", err)
			}
			want := "[DRY-RUN] Would execute: flatpak " + command + " --example"
			if output != want {
				t.Fatalf("dry-run output = %q, want %q", output, want)
			}
		})
	}
	if !IsDryRun() {
		t.Fatal("IsDryRun() = false after SetDryRun(true)")
	}
}

func TestUpdateListArgs(t *testing.T) {
	tests := []struct {
		name        string
		user        bool
		wantFlag    string
		notWantFlag string
	}{
		{name: "user installation", user: true, wantFlag: "--user", notWantFlag: "--system"},
		{name: "system installation", user: false, wantFlag: "--system", notWantFlag: "--user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := updateListArgs(tt.user)

			for _, want := range []string{"remote-ls", "--updates", "--app", tt.wantFlag} {
				if !slices.Contains(args, want) {
					t.Errorf("updateListArgs(%v) = %v, missing %q", tt.user, args, want)
				}
			}
			if slices.Contains(args, tt.notWantFlag) {
				t.Errorf("updateListArgs(%v) = %v, must not contain %q", tt.user, args, tt.notWantFlag)
			}
		})
	}
}

func TestParseUpdateList(t *testing.T) {
	tests := []struct {
		name   string
		output string
		user   bool
		want   []UpdateInfo
	}{
		{
			name:   "tab separated rows",
			output: "Firefox\torg.mozilla.firefox\t120.0\tstable\tflathub\nGIMP\torg.gimp.GIMP\t2.10.36\tstable\tflathub\n",
			user:   false,
			want: []UpdateInfo{
				{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "120.0", Branch: "stable", Origin: "flathub", Installation: "system"},
				{Name: "GIMP", ApplicationID: "org.gimp.GIMP", NewVersion: "2.10.36", Branch: "stable", Origin: "flathub", Installation: "system"},
			},
		},
		{
			name:   "whitespace separated fallback",
			output: "Firefox   org.mozilla.firefox   120.0   stable   flathub",
			user:   false,
			want: []UpdateInfo{
				{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "120.0", Branch: "stable", Origin: "flathub", Installation: "system"},
			},
		},
		{
			name:   "short row is partially parsed",
			output: "Firefox org.mozilla.firefox 120.0",
			user:   false,
			want: []UpdateInfo{
				{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "120.0", Installation: "system"},
			},
		},
		{
			name:   "row with fewer than two fields is skipped",
			output: "Firefox\nGIMP\torg.gimp.GIMP\t2.10.36\tstable\tflathub",
			user:   false,
			want: []UpdateInfo{
				{Name: "GIMP", ApplicationID: "org.gimp.GIMP", NewVersion: "2.10.36", Branch: "stable", Origin: "flathub", Installation: "system"},
			},
		},
		{
			name:   "blank and whitespace-only lines are skipped",
			output: "\n   \nFirefox\torg.mozilla.firefox\t120.0\tstable\tflathub\n\t\n",
			user:   false,
			want: []UpdateInfo{
				{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "120.0", Branch: "stable", Origin: "flathub", Installation: "system"},
			},
		},
		{
			name:   "empty output yields no updates",
			output: "",
			user:   false,
			want:   nil,
		},
		{
			name:   "user installation label",
			output: "Firefox\torg.mozilla.firefox\t120.0\tstable\tflathub",
			user:   true,
			want: []UpdateInfo{
				{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "120.0", Branch: "stable", Origin: "flathub", Installation: "user"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUpdateList(tt.output, tt.user)
			if err != nil {
				t.Fatalf("parseUpdateList() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseUpdateList() = %+v, want %+v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("parseUpdateList()[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCommandTimeout(t *testing.T) {
	if len(stateChangingCommands) != 4 {
		t.Fatalf("stateChangingCommands has %d entries, want 4: update this test when the map changes", len(stateChangingCommands))
	}

	for cmd := range stateChangingCommands {
		t.Run("state-changing/"+cmd, func(t *testing.T) {
			if got := commandTimeout([]string{cmd, "-y", "org.example.App"}); got != mutationTimeout {
				t.Errorf("commandTimeout(%q) = %v, want %v", cmd, got, mutationTimeout)
			}
		})
	}

	readCases := []struct {
		name string
		args []string
	}{
		{name: "read/list", args: []string{"list", "--user", "--app"}},
		{name: "read/remote-ls", args: []string{"remote-ls", "--updates", "--app"}},
		{name: "read/info", args: []string{"info", "--show-metadata", "org.example.App"}},
		{name: "empty args", args: nil},
	}

	for _, tc := range readCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandTimeout(tc.args); got != readTimeout {
				t.Errorf("commandTimeout(%v) = %v, want %v", tc.args, got, readTimeout)
			}
		})
	}
}

func TestTimeoutConstants(t *testing.T) {
	if readTimeout != 30*time.Second {
		t.Errorf("readTimeout = %v, want 30s", readTimeout)
	}
	if mutationTimeout != 30*time.Minute {
		t.Errorf("mutationTimeout = %v, want 30m", mutationTimeout)
	}
}
