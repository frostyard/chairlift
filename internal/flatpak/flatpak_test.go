package flatpak

import (
	"context"
	"errors"
	"os"
	"os/exec"
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
			// ListUpdates queries each remote; the capture keeps the last
			// invocation, the query of the second listed remote.
			name: "list user app updates",
			run: func() error {
				updates, err := ListUpdates(true)
				if err == nil && (len(updates) != 2 || updates[0].Installation != "user") {
					return errors.New("user update result was not parsed")
				}
				return err
			},
			want: []string{"remote-ls", "--updates", "--app", "--columns=name,application,version,branch,origin", "--user", "vendor"},
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

// installAppendingFlatpak installs a fake flatpak that appends one line per
// invocation (its space-joined arguments) to the capture file, so tests can
// assert on every call of a wrapper that runs flatpak more than once.
func installAppendingFlatpak(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	capture := filepath.Join(dir, "args")
	script := filepath.Join(dir, "flatpak")
	source := "#!/bin/sh\necho \"$@\" >> \"$CHAIRLIFT_FLATPAK_ARGS\"\n" + body + "\n"
	if err := os.WriteFile(script, []byte(source), 0o755); err != nil {
		t.Fatalf("write fake flatpak: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("CHAIRLIFT_FLATPAK_ARGS", capture)
	return capture
}

func TestUninstallUnusedRunsBothScopes(t *testing.T) {
	SetDryRun(false)
	t.Cleanup(func() { SetDryRun(false) })
	capture := installAppendingFlatpak(t, `case "$*" in *--user*) echo user-out;; *--system*) echo system-out;; esac`)

	output, err := UninstallUnused()
	if err != nil {
		t.Fatalf("UninstallUnused() error = %v", err)
	}
	want := []string{"uninstall --unused -y --user", "uninstall --unused -y --system"}
	if got := capturedFlatpakArgs(t, capture); !reflect.DeepEqual(got, want) {
		t.Fatalf("flatpak invocations = %v, want %v", got, want)
	}
	if output != "user-out\n\nsystem-out\n" {
		t.Fatalf("combined output = %q, want both scopes' output joined", output)
	}
}

func TestUninstallUnusedReportsEveryScopeError(t *testing.T) {
	SetDryRun(false)
	t.Cleanup(func() { SetDryRun(false) })

	t.Run("system fails, user still runs", func(t *testing.T) {
		capture := installAppendingFlatpak(t, `case "$*" in *--system*) echo 'system failed' >&2; exit 1;; *) echo user-out;; esac`)
		output, err := UninstallUnused()
		if err == nil || !strings.Contains(err.Error(), "--system scope") || !strings.Contains(err.Error(), "system failed") {
			t.Fatalf("error = %v, want the system scope and its stderr", err)
		}
		if strings.Contains(err.Error(), "--user scope") {
			t.Fatalf("error = %v, want no user-scope failure", err)
		}
		if !strings.Contains(output, "user-out") {
			t.Fatalf("output = %q, want the successful user scope's output", output)
		}
		if got := capturedFlatpakArgs(t, capture); len(got) != 2 {
			t.Fatalf("flatpak invocations = %v, want both scopes attempted", got)
		}
	})

	t.Run("both fail", func(t *testing.T) {
		installAppendingFlatpak(t, `echo "fail $*" >&2; exit 1`)
		_, err := UninstallUnused()
		if err == nil || !strings.Contains(err.Error(), "--user scope") || !strings.Contains(err.Error(), "--system scope") {
			t.Fatalf("error = %v, want both scope failures joined", err)
		}
		var fe *Error
		if !errors.As(err, &fe) {
			t.Fatalf("errors.As(*Error) = false for %v", err)
		}
	})
}

func TestUninstallUnusedDryRunPreviewsBothScopes(t *testing.T) {
	SetDryRun(true)
	t.Cleanup(func() { SetDryRun(false) })
	t.Setenv("PATH", t.TempDir())

	output, err := UninstallUnused()
	if err != nil {
		t.Fatalf("UninstallUnused() dry-run error = %v", err)
	}
	want := "[DRY-RUN] Would execute: flatpak uninstall --unused -y --user\n[DRY-RUN] Would execute: flatpak uninstall --unused -y --system"
	if output != want {
		t.Fatalf("dry-run output = %q, want %q", output, want)
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
			args := updateListArgs(tt.user, "flathub")
			want := []string{"remote-ls", "--updates", "--app", "--columns=name,application,version,branch,origin", tt.wantFlag, "flathub"}
			if !reflect.DeepEqual(args, want) {
				t.Errorf("updateListArgs(%v, flathub) = %v, want %v", tt.user, args, want)
			}
			if slices.Contains(args, tt.notWantFlag) {
				t.Errorf("updateListArgs(%v) = %v, must not contain %q", tt.user, args, tt.notWantFlag)
			}
		})
	}
}

func TestPerInstallationQueryArgs(t *testing.T) {
	for _, user := range []bool{true, false} {
		flag, name := "--system", "system"
		if user {
			flag, name = "--user", "user"
		}
		if got := installationFlag(user); got != flag {
			t.Errorf("installationFlag(%v) = %q, want %q", user, got, flag)
		}
		if got := installationName(user); got != name {
			t.Errorf("installationName(%v) = %q, want %q", user, got, name)
		}
		if got, want := updateRemoteListArgs(user), []string{"remotes", flag, "--columns=name,options"}; !reflect.DeepEqual(got, want) {
			t.Errorf("updateRemoteListArgs(%v) = %v, want %v", user, got, want)
		}
		if got, want := originListArgs(user), []string{"list", flag, "--app", "--columns=origin"}; !reflect.DeepEqual(got, want) {
			t.Errorf("originListArgs(%v) = %v, want %v", user, got, want)
		}
	}
}

func TestParseUpdateRemotesSkipsOnlyDisabledRemotes(t *testing.T) {
	output := "flathub\n" +
		"testhub\toci,no-gpg-verify\n" +
		"goose-origin\tdisabled,no-enumerate,no-gpg-verify\n" +
		"flathub-beta\tno-enumerate\n" +
		"\n"
	want := []string{"flathub", "testhub", "flathub-beta"}
	if got := parseUpdateRemotes(output); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseUpdateRemotes = %q, want %q", got, want)
	}
	if got := parseUpdateRemotes(""); len(got) != 0 {
		t.Fatalf("parseUpdateRemotes(\"\") = %q, want none", got)
	}
}

func TestParseOrigins(t *testing.T) {
	got := parseOrigins("flathub\n\n fedora \nflathub\n")
	want := map[string]bool{"flathub": true, "fedora": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseOrigins = %v, want %v", got, want)
	}
}

func TestAbortsUpdateQuery(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "remote failure", ctx: context.Background(), err: &Error{Message: "Flatpak command failed: unreachable"}, want: false},
		{name: "per-query timeout", ctx: context.Background(), err: &Error{Message: "timed out", Err: context.DeadlineExceeded}, want: false},
		{name: "missing executable", ctx: context.Background(), err: &NotFoundError{Message: "Flatpak not found"}, want: true},
		{name: "canceled error", ctx: context.Background(), err: &Error{Message: "canceled", Err: context.Canceled}, want: true},
		{name: "canceled caller context", ctx: cancelled, err: &Error{Message: "Flatpak command failed"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := abortsUpdateQuery(tt.ctx, tt.err); got != tt.want {
				t.Fatalf("abortsUpdateQuery = %v, want %v", got, tt.want)
			}
		})
	}
}

// fakeRemotesFlatpak installs a flatpak stand-in for the per-remote update
// query. It lists the remotes flathub, test-center and fedora; flathub offers
// a Firefox update, fedora a GIMP update, and test-center fails like a remote
// whose host no longer resolves. appOrigins are the origin remotes of the
// installed applications that `list --app --columns=origin` reports. Every
// invocation is appended to the returned log.
func fakeRemotesFlatpak(t *testing.T, appOrigins string) string {
	t.Helper()
	return fakeFlatpakScript(t, `case "$1" in
remotes) printf 'flathub\ntest-center\tno-gpg-verify\nfedora\nold\tdisabled\n' ;;
list) printf '`+appOrigins+`' ;;
remote-ls)
	case "$6" in
	flathub) printf 'Firefox\torg.mozilla.firefox\t131.0\tstable\tflathub\n' ;;
	fedora) printf 'GIMP\torg.gimp.GIMP\t3.0\tstable\tfedora\n' ;;
	*) echo "error: Unable to load summary from remote $6: Could not resolve hostname" >&2; exit 1 ;;
	esac ;;
*) exit 1 ;;
esac`)
}

// fakeFlatpakScript installs a fake flatpak on PATH that appends each
// invocation's arguments to a log before running body.
func fakeFlatpakScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "log")
	source := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CHAIRLIFT_FLATPAK_LOG\"\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "flatpak"), []byte(source), 0o755); err != nil {
		t.Fatalf("write fake flatpak: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("CHAIRLIFT_FLATPAK_LOG", logPath)
	return logPath
}

func loggedCalls(t *testing.T, logPath string) []string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read flatpak log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

var healthyRemoteUpdates = []UpdateInfo{
	{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "131.0", Branch: "stable", Origin: "flathub", Installation: "user"},
	{Name: "GIMP", ApplicationID: "org.gimp.GIMP", NewVersion: "3.0", Branch: "stable", Origin: "fedora", Installation: "user"},
}

// A remote left behind after its application was uninstalled made a
// remote-less `remote-ls --updates` fail for the whole installation. A broken
// remote no installed application comes from is ignored, the disabled remote
// is never queried, and every healthy remote's updates are still listed.
func TestListUpdatesIgnoresABrokenUnusedRemote(t *testing.T) {
	SetDryRun(false)
	logPath := fakeRemotesFlatpak(t, `flathub\nflathub\nfedora\n`)

	updates, err := ListUpdates(true)
	if err != nil {
		t.Fatalf("ListUpdates error = %v, want the unused broken remote ignored", err)
	}
	if !reflect.DeepEqual(updates, healthyRemoteUpdates) {
		t.Fatalf("ListUpdates = %#v, want %#v", updates, healthyRemoteUpdates)
	}
	want := []string{
		"remotes --user --columns=name,options",
		"remote-ls --updates --app --columns=name,application,version,branch,origin --user flathub",
		"remote-ls --updates --app --columns=name,application,version,branch,origin --user test-center",
		"remote-ls --updates --app --columns=name,application,version,branch,origin --user fedora",
		"list --user --app --columns=origin",
	}
	if got := loggedCalls(t, logPath); !reflect.DeepEqual(got, want) {
		t.Fatalf("ListUpdates ran %q, want %q", got, want)
	}
}

// A broken remote an installed application still comes from hides that
// application's updates, so it is an error naming that remote only, returned
// alongside the updates the healthy remotes reported.
func TestListUpdatesReportsABrokenRemoteInUse(t *testing.T) {
	SetDryRun(false)
	fakeRemotesFlatpak(t, `flathub\ntest-center\nfedora\n`)

	updates, err := ListUpdates(true)
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("ListUpdates error = %v, want a *RemoteError", err)
	}
	if remoteErr.Remote != "test-center" || remoteErr.Installation != "user" {
		t.Fatalf("RemoteError = %+v, want test-center in the user installation", remoteErr)
	}
	var cause *Error
	if !errors.As(remoteErr, &cause) {
		t.Fatalf("RemoteError does not unwrap to the query's *Error: %v", remoteErr)
	}
	for _, want := range []string{`"test-center"`, "user installation", "Could not resolve hostname"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	for _, healthy := range []string{"flathub", "fedora"} {
		if strings.Contains(err.Error(), `"`+healthy+`"`) {
			t.Errorf("error %q names healthy remote %q", err, healthy)
		}
	}
	if !reflect.DeepEqual(updates, healthyRemoteUpdates) {
		t.Fatalf("ListUpdates = %#v, want the healthy remotes' updates %#v", updates, healthyRemoteUpdates)
	}
}

// When the origins cannot be read, no failed remote can be shown unused, so
// it is still reported together with the origin listing failure.
func TestListUpdatesReportsABrokenRemoteWhenOriginsAreUnreadable(t *testing.T) {
	SetDryRun(false)
	fakeFlatpakScript(t, `case "$1" in
remotes) printf 'test-center\n' ;;
*) echo "$1 failed" >&2; exit 1 ;;
esac`)

	_, err := ListUpdates(false)
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) || remoteErr.Remote != "test-center" || remoteErr.Installation != "system" {
		t.Fatalf("ListUpdates error = %v, want test-center reported in the system installation", err)
	}
	if !strings.Contains(err.Error(), "list failed") {
		t.Fatalf("ListUpdates error = %v, want the origin listing failure too", err)
	}
}

func TestListUpdatesWithNoRemotesQueriesNothing(t *testing.T) {
	SetDryRun(false)
	logPath := fakeFlatpakScript(t, "")

	updates, err := ListUpdates(true)
	if err != nil || len(updates) != 0 {
		t.Fatalf("ListUpdates = %v, %v; want no updates and no error", updates, err)
	}
	if got := loggedCalls(t, logPath); !reflect.DeepEqual(got, []string{"remotes --user --columns=name,options"}) {
		t.Fatalf("ListUpdates ran %q, want only the remote listing", got)
	}
}

// A missing executable is not one remote's failure: it aborts the whole query.
func TestListUpdatesMissingExecutableAborts(t *testing.T) {
	SetDryRun(false)
	t.Setenv("PATH", t.TempDir())

	updates, err := ListUpdates(true)
	var notFound *NotFoundError
	if !errors.As(err, &notFound) || updates != nil {
		t.Fatalf("ListUpdates = %v, %v; want nil updates and a *NotFoundError", updates, err)
	}
}

// A cancelled caller context aborts the whole query instead of being judged as
// one remote's failure.
func TestListUpdatesCancellationAborts(t *testing.T) {
	SetDryRun(false)

	t.Run("before the first query", func(t *testing.T) {
		fakeRemotesFlatpak(t, `test-center\n`)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		updates, err := listUpdates(ctx, true)
		if !errors.Is(err, context.Canceled) || updates != nil {
			t.Fatalf("listUpdates(cancelled) = %v, %v; want nil updates and context.Canceled", updates, err)
		}
	})

	t.Run("during a remote query", func(t *testing.T) {
		dir := t.TempDir()
		started := filepath.Join(dir, "started")
		// The fake replaces PATH, so resolve the tools it needs first.
		touch, err := exec.LookPath("touch")
		if err != nil {
			t.Skip("touch not available")
		}
		sleep, err := exec.LookPath("sleep")
		if err != nil {
			t.Skip("sleep not available")
		}
		fakeFlatpakScript(t, `case "$1" in
remotes) printf 'flathub\nslow\n' ;;
remote-ls)
	case "$6" in
	flathub) printf 'Firefox\torg.mozilla.firefox\t131.0\tstable\tflathub\n' ;;
	*) "`+touch+`" "`+started+`"; exec "`+sleep+`" 30 ;;
	esac ;;
esac`)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(started); err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			cancel()
		}()
		updates, err := listUpdates(ctx, true)
		if !errors.Is(err, context.Canceled) || updates != nil {
			t.Fatalf("listUpdates = %v, %v; want nil updates and context.Canceled", updates, err)
		}
		var remoteErr *RemoteError
		if errors.As(err, &remoteErr) {
			t.Fatalf("cancellation was reported as a remote failure: %v", err)
		}
	})
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
