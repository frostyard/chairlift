package views

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/activity"
	"github.com/frostyard/chairlift/internal/cmdlog"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/frostui/frostuitest"
	"github.com/frostyard/chairlift/internal/sysinfo"
	"github.com/frostyard/chairlift/internal/views/pageview"
)

// maintFake records what the page asked the backends to do.
type maintFake struct {
	mu       sync.Mutex
	commands []pageview.Command
	order    []string

	brewChecks, flatpakChecks atomic.Int32
	brewErr, flatpakErr       error
	scriptErr                 error
	brewOut                   string
	dryRun                    bool
}

func (f *maintFake) record(key string) {
	f.mu.Lock()
	f.order = append(f.order, key)
	f.mu.Unlock()
}

func (f *maintFake) ran() ([]pageview.Command, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]pageview.Command(nil), f.commands...), append([]string(nil), f.order...)
}

type maintFixture struct {
	t     *testing.T
	env   *Env
	loop  *frostuitest.Loop
	store *Store
	m     *Maintenance
	page  Page
	fake  *maintFake
	// brewInstalled and flatpakInstalled are what the install checks find.
	brewInstalled, flatpakInstalled bool
	disk                            sysinfo.Disk
	info                            SystemInfo
}

// maintConfig turns on every Maintenance group, with a privileged and an
// unprivileged configured action, and bootc's status.
func maintConfig() *config.Config {
	return &config.Config{
		MaintenancePage: config.PageConfig{
			"maintenance_cleanup_group": {Enabled: true, Actions: []config.ActionConfig{
				{Title: "Clean Up Boot Old Entries", Script: "/usr/libexec/bls-gc", Sudo: true},
				{Title: "Trim caches", Script: "/usr/libexec/snow-trim-caches"},
			}},
			"maintenance_brew_group":    {Enabled: true},
			"maintenance_flatpak_group": {Enabled: true},
		},
		SystemPage: config.PageConfig{
			"system_info_group":  {Enabled: true},
			"bootc_status_group": {Enabled: true},
			"health_group":       {Enabled: true},
		},
	}
}

func newMaintFixture(t *testing.T, cfg *config.Config, dryRun bool) *maintFixture {
	t.Helper()
	f := &maintFixture{t: t, loop: frostuitest.NewLoop(), fake: &maintFake{dryRun: dryRun}, brewInstalled: true, flatpakInstalled: true}
	f.env = NewEnv(cfg, dryRun, f.loop.Post)
	f.env.Now = func() time.Time { return time.Date(2026, 11, 10, 9, 41, 0, 0, time.UTC) }
	f.store = &Store{Env: f.env}
	f.m = NewMaintenance(f.env, f.store)
	f.store.Maint = f.m
	fake := f.fake
	f.m.brewInstalled = func() bool { fake.brewChecks.Add(1); return f.brewInstalled }
	f.m.flatpakInstalled = func() bool { fake.flatpakChecks.Add(1); return f.flatpakInstalled }
	f.m.brewCleanup = func() (string, error) {
		fake.record("brew")
		if fake.dryRun {
			return "[DRY-RUN] Would execute: brew cleanup", nil
		}
		return fake.brewOut, fake.brewErr
	}
	f.m.flatpakCleanup = func() (string, error) {
		fake.record("flatpak")
		return "", fake.flatpakErr
	}
	f.m.brewDryRun = func() bool { return fake.dryRun }
	f.m.flatpakDryRun = func() bool { return fake.dryRun }
	f.m.runCommand = func(_ context.Context, cmd pageview.Command) error {
		fake.mu.Lock()
		fake.commands = append(fake.commands, cmd)
		fake.order = append(fake.order, cmd.Name+" "+strings.Join(cmd.Args, " "))
		fake.mu.Unlock()
		return fake.scriptErr
	}
	f.m.disk = func() sysinfo.Disk { return f.disk }
	f.m.info = func() SystemInfo { return f.info }
	f.page = NewMaintenancePage(f.store)
	return f
}

// start runs Start and lets its install checks end.
func (f *maintFixture) start() {
	f.t.Helper()
	f.m.Start()
	f.loop.RunUntil(f.t, func() bool {
		return (!f.env.Enabled(maintPage, maintBrewGroup) || f.m.brew != maintToolUnknown) &&
			(!f.env.Enabled(maintPage, maintFlatpakGroup) || f.m.flatpak != maintToolUnknown)
	})
}

// settle lets every queued job end.
func (f *maintFixture) settle() {
	f.t.Helper()
	f.loop.RunUntil(f.t, func() bool { return !f.env.Jobs.Busy() })
}

// view draws the page as the shell's content area does, with the
// confirmations over it.
func (f *maintFixture) view(c *ui.Context) {
	ui.Scroll(c).Fill().Padding(frostui.SpaceXL, 28).Children(func() {
		ui.Column(c).Gap(18).AlignItems(ui.Stretch).Children(func() { f.page.View(c) })
	})
	f.env.Dialogs.View(c)
}

func (f *maintFixture) tester() *ui.Tester { return ui.NewTester(f.view, 1360, 900) }

func (f *maintFixture) click(tt *ui.Tester, label string) {
	f.t.Helper()
	tt.Frame()
	if err := tt.Click(label); err != nil {
		f.t.Fatalf("click %q: %v\ntexts: %q", label, err, tt.Texts())
	}
}

func maintKeys(cls []cleanup) []string {
	keys := []string{}
	for _, cl := range cls {
		keys = append(keys, cl.Key)
	}
	return keys
}

func maintToasts(env *Env) []ui.Toast { return env.TakeToasts() }

func TestMaintenanceCleanupsFollowGroupsAndTools(t *testing.T) {
	tests := []struct {
		name             string
		off              []string
		brew, flatpak    bool
		want             []string
		wantBrewChecks   int32
		wantFlatpakCheck int32
	}{
		{name: "everything on", brew: true, flatpak: true, want: []string{"script:0", "script:1", "brew", "flatpak"}, wantBrewChecks: 1, wantFlatpakCheck: 1},
		{name: "configured actions off", off: []string{"maintenance_cleanup_group"}, brew: true, flatpak: true, want: []string{"brew", "flatpak"}, wantBrewChecks: 1, wantFlatpakCheck: 1},
		{name: "homebrew group off", off: []string{"maintenance_brew_group"}, brew: true, flatpak: true, want: []string{"script:0", "script:1", "flatpak"}, wantFlatpakCheck: 1},
		{name: "flatpak group off", off: []string{"maintenance_flatpak_group"}, brew: true, flatpak: true, want: []string{"script:0", "script:1", "brew"}, wantBrewChecks: 1},
		{name: "homebrew missing", flatpak: true, want: []string{"script:0", "script:1", "flatpak"}, wantBrewChecks: 1, wantFlatpakCheck: 1},
		{name: "flatpak missing", brew: true, want: []string{"script:0", "script:1", "brew"}, wantBrewChecks: 1, wantFlatpakCheck: 1},
		{name: "every group off", off: []string{"maintenance_cleanup_group", "maintenance_brew_group", "maintenance_flatpak_group"}, brew: true, flatpak: true, want: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := maintConfig()
			for _, g := range tc.off {
				gc := cfg.MaintenancePage[g]
				gc.Enabled = false
				cfg.MaintenancePage[g] = gc
			}
			f := newMaintFixture(t, cfg, false)
			f.brewInstalled, f.flatpakInstalled = tc.brew, tc.flatpak
			f.start()
			if got := maintKeys(f.m.Cleanups()); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("cleanups = %v, want %v", got, tc.want)
			}
			if got := maintKeys(f.m.ready()); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ready cleanups = %v, want %v", got, tc.want)
			}
			if got := f.fake.brewChecks.Load(); got != tc.wantBrewChecks {
				t.Errorf("Homebrew looked for %d times, want %d", got, tc.wantBrewChecks)
			}
			if got := f.fake.flatpakChecks.Load(); got != tc.wantFlatpakCheck {
				t.Errorf("Flatpak looked for %d times, want %d", got, tc.wantFlatpakCheck)
			}
		})
	}
}

func TestMaintenanceShowsToolsAsCheckedUntilTheirCheckEnds(t *testing.T) {
	f := newMaintFixture(t, maintConfig(), false)
	release := make(chan struct{})
	f.m.brewInstalled = func() bool { <-release; return true }
	f.m.flatpakInstalled = func() bool { <-release; return false }
	f.m.Start()
	tt := f.tester()
	if !tt.HasText("Checking…") || !tt.HasText("Homebrew downloads and old versions") || !tt.HasText("Unused Flatpak runtimes") {
		t.Fatalf("the rows being checked are missing: %q", tt.Texts())
	}
	if got := maintKeys(f.m.ready()); !reflect.DeepEqual(got, []string{"script:0", "script:1"}) {
		t.Errorf("ready while checking = %v, want only the configured actions", got)
	}
	if _, ok := tt.Find("Clean: Homebrew downloads and old versions"); ok {
		t.Error("a cleanup being checked offers its button")
	}
	close(release)
	f.loop.RunUntil(t, func() bool { return f.m.brew != maintToolUnknown && f.m.flatpak != maintToolUnknown })
	tt.Frame()
	if tt.HasText("Checking…") || tt.HasText("Unused Flatpak runtimes") {
		t.Errorf("after the checks a missing tool's row stays: %q", tt.Texts())
	}
	if _, ok := tt.Find("Clean: Homebrew downloads and old versions"); !ok {
		t.Error("Homebrew's row has no button after it was found")
	}
}

func TestMaintenanceScriptsRunExactlyTheMaintenanceCommand(t *testing.T) {
	f := newMaintFixture(t, maintConfig(), false)
	f.start()
	tt := f.tester()
	for _, want := range []string{"pkexec /usr/libexec/bls-gc", "/usr/libexec/snow-trim-caches", "brew cleanup", "flatpak uninstall --unused -y", "admin"} {
		if !tt.HasText(want) {
			t.Errorf("the rows do not show %q: %q", want, tt.Texts())
		}
	}
	admins := 0
	for _, s := range tt.Texts() {
		if s == "admin" {
			admins++
		}
	}
	if admins != 1 {
		t.Errorf("%d admin pills, want 1 (only the sudo action)", admins)
	}

	f.click(tt, "Clean: Clean Up Boot Old Entries")
	if !tt.HasText("Run Clean Up Boot Old Entries?") || !tt.HasText("Needs administrator authentication.") {
		t.Fatalf("no confirmation naming administrator authentication: %q", tt.Texts())
	}
	f.click(tt, "Run cleanup")
	f.settle()
	f.click(tt, "Clean: Trim caches")
	if tt.HasText("Needs administrator authentication.") {
		t.Error("the unprivileged action claims administrator authentication")
	}
	f.click(tt, "Run cleanup")
	f.settle()

	got, _ := f.fake.ran()
	want := []pageview.Command{
		pageview.MaintenanceCommand("/usr/libexec/bls-gc", true),
		pageview.MaintenanceCommand("/usr/libexec/snow-trim-caches", false),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ran %#v, want %#v", got, want)
	}
	if got[0].Name != "pkexec" || !reflect.DeepEqual(got[0].Args, []string{"/usr/libexec/bls-gc"}) {
		t.Errorf("the sudo action ran %#v, want exactly pkexec <script>", got[0])
	}
	if got[1].Name != "/usr/libexec/snow-trim-caches" || len(got[1].Args) != 0 {
		t.Errorf("the plain action ran %#v, want the script alone", got[1])
	}
	toasts := maintToasts(f.env)
	if len(toasts) != 2 || toasts[0].Title != "Clean Up Boot Old Entries completed" || toasts[1].Title != "Trim caches completed" || toasts[0].Type != "success" {
		t.Errorf("toasts = %+v", toasts)
	}
	tt.Frame()
	if !tt.HasText("Cleaned") {
		t.Errorf("the rows do not show their result: %q", tt.Texts())
	}
}

func TestMaintenanceAsksBeforeRunning(t *testing.T) {
	f := newMaintFixture(t, maintConfig(), false)
	f.start()
	tt := f.tester()
	f.click(tt, "Clean: Homebrew downloads and old versions")
	if !tt.HasText("Clean up Homebrew?") || !tt.HasText("brew cleanup") {
		t.Fatalf("no confirmation: %q", tt.Texts())
	}
	if f.env.Jobs.Busy() {
		t.Fatal("a job was queued before the user confirmed")
	}
	f.click(tt, "Cancel")
	if f.env.Dialogs.Showing() || f.env.Jobs.Busy() {
		t.Fatal("cancel left the dialog or queued a job")
	}
	if _, order := f.fake.ran(); len(order) != 0 {
		t.Fatalf("ran %v after a cancel", order)
	}

	f.click(tt, "Clean: Homebrew downloads and old versions")
	f.click(tt, "Run cleanup")
	if !f.m.Pending("brew") {
		t.Fatal("the confirmed cleanup is not queued")
	}
	tt.Frame()
	if !tt.HasText("Cleaning…") && !tt.HasText("Waiting") {
		t.Errorf("the row shows no pending state: %q", tt.Texts())
	}
	f.settle()
	if _, order := f.fake.ran(); !reflect.DeepEqual(order, []string{"brew"}) {
		t.Fatalf("ran %v, want brew once", order)
	}
	toasts := maintToasts(f.env)
	if len(toasts) != 1 || toasts[0].Title != "Homebrew cleanup completed" {
		t.Errorf("toasts = %+v", toasts)
	}
}

func TestMaintenanceRecordsActivityOnlyForALiveSuccess(t *testing.T) {
	f := newMaintFixture(t, maintConfig(), false)
	log, err := activity.Open(filepath.Join(t.TempDir(), "activity.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Activity = log
	f.start()
	tt := f.tester()
	f.click(tt, "Clean: Unused Flatpak runtimes")
	if !tt.HasText("Remove unused Flatpak runtimes?") {
		t.Fatalf("no confirmation: %q", tt.Texts())
	}
	f.click(tt, "Run cleanup")
	f.settle()
	recent := log.Recent(5)
	if len(recent) != 1 || recent[0].Title != "Flatpak cleanup completed" || recent[0].Source != maintSource {
		t.Errorf("activity = %+v", recent)
	}
}

func TestMaintenanceDryRunRunsNothing(t *testing.T) {
	var mu sync.Mutex
	var events []cmdlog.Event
	cmdlog.SetSink(func(e cmdlog.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	t.Cleanup(func() { cmdlog.SetSink(nil) })

	f := newMaintFixture(t, maintConfig(), true)
	f.m.runCommand = func(context.Context, pageview.Command) error {
		t.Error("a dry run started a configured action")
		return nil
	}
	f.start()
	tt := f.tester()
	f.click(tt, "Clean: Clean Up Boot Old Entries")
	if !tt.HasText("Dry run") {
		t.Errorf("the confirmation does not say it is a dry run: %q", tt.Texts())
	}
	f.click(tt, "Run cleanup")
	f.settle()
	f.click(tt, "Clean: Homebrew downloads and old versions")
	f.click(tt, "Run cleanup")
	f.settle()

	toasts := maintToasts(f.env)
	want := []string{
		"[DRY-RUN] Preview: Clean Up Boot Old Entries would run — no changes made",
		"[DRY-RUN] Would execute: brew cleanup",
	}
	if len(toasts) != len(want) {
		t.Fatalf("toasts = %+v, want %q", toasts, want)
	}
	for i, w := range want {
		if toasts[i].Title != w || toasts[i].Type != "info" {
			t.Errorf("toast %d = %+v, want info %q", i, toasts[i], w)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 || events[0].Kind != cmdlog.KindDryRun || !reflect.DeepEqual(events[0].Argv, []string{"pkexec", "/usr/libexec/bls-gc"}) {
		t.Errorf("console events = %+v, want one dry-run preview of pkexec /usr/libexec/bls-gc", events)
	}
	tt.Frame()
	if !tt.HasText("Previewed") || tt.HasText("Cleaned") {
		t.Errorf("a dry run shows as cleaned: %q", tt.Texts())
	}
}

func TestMaintenanceFailuresKeepAnErrorToast(t *testing.T) {
	f := newMaintFixture(t, maintConfig(), false)
	f.fake.brewErr = errors.New("Brew command failed: boom")
	f.fake.scriptErr = errors.New("exit status 2")
	f.start()
	tt := f.tester()
	f.click(tt, "Clean: Homebrew downloads and old versions")
	f.click(tt, "Run cleanup")
	f.settle()
	f.click(tt, "Clean: Trim caches")
	f.click(tt, "Run cleanup")
	f.settle()
	toasts := maintToasts(f.env)
	want := []string{"Homebrew cleanup failed: Brew command failed: boom", "Trim caches failed: exit status 2"}
	if len(toasts) != 2 {
		t.Fatalf("toasts = %+v", toasts)
	}
	for i, w := range want {
		if toasts[i].Title != w || toasts[i].Type != "error" || toasts[i].Timeout != -1 {
			t.Errorf("toast %d = %+v, want a persistent error %q", i, toasts[i], w)
		}
	}
	tt.Frame()
	if !tt.HasText("Failed") {
		t.Errorf("the rows do not show the failure: %q", tt.Texts())
	}
	if r := f.m.Result("brew"); r.Outcome != cleanupFailed || r.Err == "" {
		t.Errorf("brew result = %+v", r)
	}
}

func TestMaintenanceCleanEverythingAsksOnceAndQueuesAll(t *testing.T) {
	f := newMaintFixture(t, maintConfig(), false)
	f.start()
	tt := f.tester()
	f.click(tt, "Clean everything")
	for _, want := range []string{"Clean everything?", "Runs 4 cleanups, one after the other.", "One of them needs administrator authentication.", "pkexec /usr/libexec/bls-gc", "Flatpak runtimes"} {
		if !tt.HasText(want) {
			t.Errorf("the confirmation lacks %q: %q", want, tt.Texts())
		}
	}
	if f.env.Jobs.Busy() {
		t.Fatal("jobs were queued before the confirmation")
	}
	f.click(tt, "Run all")
	f.settle()
	_, order := f.fake.ran()
	want := []string{"pkexec /usr/libexec/bls-gc", "/usr/libexec/snow-trim-caches ", "brew", "flatpak"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("ran %q, want %q", order, want)
	}
	if f.env.Dialogs.Showing() {
		t.Error("a second confirmation came up")
	}
}

func TestMaintenanceDeploymentsCardFollowsItsGroup(t *testing.T) {
	booted := SystemInfo{
		Loaded: true, Kind: "bootc",
		Booted:   Deployment{Present: true, Version: "2026.11.05", Digest: "sha256:4f1c2a9b7e3d5c8a1b2c3d4e5f60718293a4b5c6"},
		Rollback: Deployment{Present: true, Version: "2026.10.29", Digest: "sha256:aa11bb22cc33dd44ee55ff6677889900", Pinned: true},
	}
	tests := []struct {
		name string
		on   bool
		info SystemInfo
		want bool
	}{
		{name: "group off", on: false, info: booted, want: false},
		{name: "nothing booted", on: true, info: SystemInfo{Loaded: true}, want: false},
		{name: "booted", on: true, info: booted, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := maintConfig()
			cfg.SystemPage["bootc_status_group"] = config.GroupConfig{Enabled: tc.on}
			f := newMaintFixture(t, cfg, false)
			f.info = tc.info
			f.start()
			tt := f.tester()
			if got := tt.HasText("Deployments"); got != tc.want {
				t.Fatalf("Deployments shown = %v, want %v: %q", got, tc.want, tt.Texts())
			}
			if !tc.want {
				return
			}
			for _, want := range []string{"Booted · running now", "2026.11.05", "Staged · next boot", "nothing yet", "Rollback · in the boot menu", "2026.10.29", "pinned", pageview.ShortDigest(booted.Booted.Digest)} {
				if !tt.HasText(want) {
					t.Errorf("the deployments lack %q: %q", want, tt.Texts())
				}
			}
			for _, banned := range []string{"Roll back", "Hold to roll back", "Keep this deployment"} {
				if tt.HasText(banned) {
					t.Errorf("the deployments offer %q, which is not approved", banned)
				}
			}
		})
	}
}

func TestMaintenanceHeaderAndGaugeShowTheDisk(t *testing.T) {
	f := newMaintFixture(t, maintConfig(), false)
	f.start()
	tt := f.tester()
	if tt.HasText("Free space") {
		t.Errorf("a free-space card without a disk sample: %q", tt.Texts())
	}
	f.disk = sysinfo.Disk{Path: "/var/home", Free: 288_400_000_000, Total: 512_000_000_000}
	tt.Frame()
	for _, want := range []string{"288 GB free of 512 GB", "Free space", "/var/home", "288 GB", "free of 512 GB"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q: %q", want, tt.Texts())
		}
	}
}

func TestMaintenanceCommandsOfferEachCleanup(t *testing.T) {
	f := newMaintFixture(t, maintConfig(), false)
	if got := f.page.Commands(); len(got) != 3 {
		// Before the checks end only the configured actions can run.
		t.Errorf("commands before the checks = %+v", got)
	}
	f.start()
	cmds := f.page.Commands()
	var titles []string
	for _, c := range cmds {
		titles = append(titles, c.Title)
		if c.Disabled {
			t.Errorf("%q is disabled with nothing running", c.Title)
		}
	}
	want := []string{"Clean Up Boot Old Entries", "Trim caches", "Clean up Homebrew", "Clean up unused Flatpak runtimes", "Clean everything"}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("commands = %q, want %q", titles, want)
	}
	cmds[2].Run()
	if !f.env.Dialogs.Showing() || f.env.Jobs.Busy() {
		t.Fatal("the palette's cleanup did not ask first")
	}
	if badge, _ := f.page.Badge(); badge != "" {
		t.Errorf("badge = %q, want none", badge)
	}

	none := newMaintFixture(t, &config.Config{MaintenancePage: config.PageConfig{
		"maintenance_cleanup_group": {Enabled: false},
		"maintenance_brew_group":    {Enabled: false},
		"maintenance_flatpak_group": {Enabled: false},
	}}, false)
	none.start()
	if got := none.page.Commands(); got != nil {
		t.Errorf("commands with every group off = %+v", got)
	}
	tt := none.tester()
	if tt.HasText("Clean everything") || !tt.HasText("Nothing to clean up here") {
		t.Errorf("an empty page offers cleanups: %q", tt.Texts())
	}
}

func TestMaintenancePendingCleanupIsNotQueuedTwice(t *testing.T) {
	f := newMaintFixture(t, maintConfig(), false)
	block := make(chan struct{})
	f.m.brewCleanup = func() (string, error) { <-block; return "", nil }
	f.start()
	tt := f.tester()
	f.click(tt, "Clean: Homebrew downloads and old versions")
	f.click(tt, "Run cleanup")
	if !f.m.Pending("brew") {
		t.Fatal("not pending")
	}
	f.m.Ask("brew")
	if f.env.Dialogs.Showing() {
		t.Error("a pending cleanup asks again")
	}
	for _, c := range f.page.Commands() {
		if c.ID == "maintenance.clean.brew" && !c.Disabled {
			t.Error("the palette offers a pending cleanup")
		}
	}
	f.m.AskEverything()
	f.click(tt, "Run all")
	close(block)
	f.settle()
	if got := len(f.env.Jobs.History()); got != 4 {
		t.Errorf("%d jobs ran, want 4 (brew once)", got)
	}
}

func TestMaintenanceRunCommandStreamsOutputToTheConsole(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "cleanup.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho freed 12 files\necho a warning >&2\nexit \"${1:-0}\"\n"), 0o755); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	var mu sync.Mutex
	var events []cmdlog.Event
	cmdlog.SetSink(func(e cmdlog.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	t.Cleanup(func() { cmdlog.SetSink(nil) })

	if err := runMaintenanceCommand(context.Background(), pageview.MaintenanceCommand(script, false)); err != nil {
		t.Fatalf("run: %v", err)
	}
	mu.Lock()
	var lines []string
	for _, e := range events {
		if e.Kind == cmdlog.KindLine {
			lines = append(lines, e.Text)
		}
	}
	first, last := events[0], events[len(events)-1]
	mu.Unlock()
	if first.Kind != cmdlog.KindStart || !reflect.DeepEqual(first.Argv, []string{script}) {
		t.Errorf("start = %+v, want the script's argv", first)
	}
	if !reflect.DeepEqual(lines, []string{"freed 12 files", "a warning"}) && !reflect.DeepEqual(lines, []string{"a warning", "freed 12 files"}) {
		t.Errorf("lines = %q", lines)
	}
	if last.Kind != cmdlog.KindEnd || last.Err != nil {
		t.Errorf("end = %+v", last)
	}

	if err := runMaintenanceCommand(context.Background(), pageview.Command{Name: script, Args: []string{"3"}}); err == nil {
		t.Error("a failing script returned no error")
	}

	slow := filepath.Join(dir, "slow.sh")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	start := time.Now()
	if err := runMaintenanceCommand(ctx, pageview.Command{Name: slow}); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled run = %v, want context.Canceled", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("cancel did not stop the script")
	}
}

// TestMaintenanceStartsNoPrivilegedCommandOfItsOwn keeps the privilege
// boundary: the page's only command runner is runMaintenanceCommand, and
// pkexec comes only from pageview.MaintenanceCommand.
func TestMaintenanceStartsNoPrivilegedCommandOfItsOwn(t *testing.T) {
	files, err := filepath.Glob("maintenance*.go")
	if err != nil {
		t.Fatal(err)
	}
	execs := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path) //nolint:gosec // the package's own source
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		if strings.Contains(src, `"pkexec"`) || strings.Contains(src, `"sudo"`) {
			t.Errorf("%s names pkexec or sudo itself", path)
		}
		execs += strings.Count(src, "exec.Command")
		if strings.Contains(src, "exec.CommandContext(ctx, script)") || strings.Contains(src, `exec.CommandContext(ctx, "pkexec"`) {
			t.Errorf("%s builds a maintenance command by hand", path)
		}
	}
	if execs != 1 {
		t.Errorf("%d exec.Command calls in the Maintenance files, want 1 (runMaintenanceCommand)", execs)
	}
}

// TestMaintenanceBackendCommandsMatchTheRows keeps the commands the rows
// show in step with what homebrew.Cleanup and flatpak.UninstallUnused run.
func TestMaintenanceBackendCommandsMatchTheRows(t *testing.T) {
	check := func(file, fn, runner string, argv []string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", file)) //nolint:gosec // the repository's own source
		if err != nil {
			t.Fatal(err)
		}
		body := regexp.MustCompile(`(?s)func ` + fn + `\(\) \(string, error\) \{(.*?)\n\}`).FindStringSubmatch(string(data))
		if body == nil {
			t.Fatalf("%s: no func %s", file, fn)
		}
		quoted := make([]string, len(argv)-1)
		for i, a := range argv[1:] {
			quoted[i] = `"` + a + `"`
		}
		want := runner + "(" + strings.Join(quoted, ", ") + ")"
		if !strings.Contains(body[1], want) {
			t.Errorf("%s does not run %s: the row shows %q", fn, want, strings.Join(argv, " "))
		}
	}
	check("homebrew/homebrew.go", "Cleanup", "runBrewCommand", brewCleanupArgv)
	check("flatpak/flatpak.go", "UninstallUnused", "runFlatpakCommand", flatpakUnusedArgv)
}

func TestSnapshotMaintenance(t *testing.T) {
	t.Setenv("FROSTUI_ICON_THEME", "Adwaita")
	frostui.SetIcons(frostui.NewIcons(nil))
	t.Cleanup(func() { frostui.SetIcons(nil) })
	for _, dark := range []bool{false, true} {
		name := "maintenance-light"
		if dark {
			name = "maintenance-dark"
		}
		t.Run(name, func(t *testing.T) {
			f := newMaintFixture(t, maintConfig(), false)
			f.disk = sysinfo.Disk{Path: "/var/home", Free: 288_400_000_000, Total: 512_000_000_000}
			f.info = SystemInfo{
				Loaded: true, Kind: "bootc",
				Booted:   Deployment{Present: true, Version: "2026.11.05", Digest: "sha256:4f1c2a9b7e3d5c8a1b2c3d4e5f60718293a4b5c6"},
				Rollback: Deployment{Present: true, Version: "2026.10.29", Digest: "sha256:aa11bb22cc33dd44ee55ff6677889900"},
			}
			f.start()
			// One cleanup ran, one failed, one waits.
			f.m.setResult("brew", cleanupDone, "")
			f.m.setResult("script:1", cleanupFailed, "exit status 1")
			block := make(chan struct{})
			f.m.flatpakCleanup = func() (string, error) { <-block; return "", nil }
			f.env.Jobs.Enqueue(f.m.job(cleanup{Key: "flatpak", Kind: cleanupByFlatpak}))
			defer func() { close(block); f.settle() }()

			shell := &frostui.Shell{Brand: frostui.Brand{Name: "ChairLift", Tagline: "Snow Linux command center"}, Selected: "maintenance"}
			view := func(c *ui.Context) {
				shell.View(c, frostui.Frame{
					Nav: []frostui.NavSection{{Title: "System", Items: []frostui.NavItem{
						{ID: "overview", Title: "Overview", Icon: "view-grid"},
						{ID: "updates", Title: "Updates", Icon: "software-update-available"},
						{ID: "apps", Title: "Apps", Icon: "view-app-grid"},
						{ID: "features", Title: "Features", Icon: "application-x-addon"},
						{ID: "problems", Title: "Problems", Icon: "dialog-warning"},
						{ID: "maintenance", Title: "Maintenance", Icon: "preferences-other"},
					}}},
					Content:     func() { f.page.View(c) },
					Commands:    f.page.Commands(),
					StatusEnd:   []frostui.StatusItem{{Icon: "drive-harddisk", Text: "288 GB free", Progress: -1}},
					ConsoleData: f.env.Console,
				})
				f.env.Dialogs.View(c)
			}
			tt := ui.NewTesterWith(view, 1440, 920, ui.TesterOptions{Dark: dark})
			snapshot(t, tt, name)
			if err := tt.Click("Clean everything"); err != nil {
				t.Fatal(err)
			}
			snapshot(t, tt, name+"-confirm")
		})
	}
}

func TestMaintenanceCancelKillsTheScriptsChildren(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := filepath.Join(dir, "parent.sh")
	body := "#!/bin/sh\nsleep 30 &\necho $! > " + pidFile + "\nwait\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runMaintenanceCommand(ctx, pageview.Command{Name: script}) }()
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			_, _ = fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("the script's child never started")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("canceled run = %v, want context.Canceled", err)
	}
	gone := false
	for range 200 {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			gone = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !gone {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Error("the script's child outlived the cancel: the process group was not killed")
	}
}

func TestPrivilegedJobsCannotBeStopped(t *testing.T) {
	env, _ := sysEnv(t, sysConfig(sysAllGroups...), false)
	s := NewSystem(env)
	if j := s.stageJob(); !j.Unstoppable {
		t.Error("the stage job (pkexec stage script) is stoppable")
	}
	m := NewMaintenance(env, nil)
	for _, tc := range []struct {
		cl   cleanup
		want bool
	}{
		{cleanup{Key: "script:0", Kind: cleanupByScript, script: "/usr/libexec/bls-gc", sudo: true}, true},
		{cleanup{Key: "script:1", Kind: cleanupByScript, script: "/usr/bin/tidy"}, false},
		{cleanup{Key: "brew", Kind: cleanupByBrew}, false},
	} {
		if got := m.job(tc.cl).Unstoppable; got != tc.want {
			t.Errorf("%s: Unstoppable = %v, want %v", tc.cl.Key, got, tc.want)
		}
	}
}
