package views

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/activity"
	"github.com/frostyard/chairlift/internal/bootc"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/navigation"
	"github.com/frostyard/chairlift/internal/sysinfo"
	"github.com/frostyard/chairlift/internal/sysupdate"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/frostui"
)

// sysPosts stands in for the UI thread: Env.Post queues functions and the
// test runs them on its own goroutine, so worker goroutines never touch
// state (the race detector checks it).
type sysPosts struct {
	mu  sync.Mutex
	fns []func()
}

func (q *sysPosts) post(fn func()) {
	q.mu.Lock()
	q.fns = append(q.fns, fn)
	q.mu.Unlock()
}

func (q *sysPosts) drain() int {
	q.mu.Lock()
	fns := q.fns
	q.fns = nil
	q.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
	return len(fns)
}

// until runs posted functions until cond holds.
func (q *sysPosts) until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if q.drain() == 0 {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			time.Sleep(time.Millisecond)
		}
	}
}

// sysConfig is a configuration with every navigation group disabled but
// the "page/group" names given.
func sysConfig(enabled ...string) *config.Config {
	cfg := &config.Config{
		SystemPage: config.PageConfig{}, UpdatesPage: config.PageConfig{}, ApplicationsPage: config.PageConfig{},
		MaintenancePage: config.PageConfig{}, FeaturesPage: config.PageConfig{}, ProblemsPage: config.PageConfig{},
		HelpPage: config.PageConfig{},
	}
	pages := map[string]config.PageConfig{
		"system_page": cfg.SystemPage, "updates_page": cfg.UpdatesPage, "applications_page": cfg.ApplicationsPage,
		"maintenance_page": cfg.MaintenancePage, "features_page": cfg.FeaturesPage, "problems_page": cfg.ProblemsPage,
	}
	for _, item := range navigation.Items() {
		for _, g := range item.Groups {
			pages[item.ConfigPage][g] = config.GroupConfig{Enabled: false}
		}
	}
	for _, pg := range enabled {
		page, group, _ := strings.Cut(pg, "/")
		gc := pages[page][group]
		gc.Enabled = true
		pages[page][group] = gc
	}
	return cfg
}

// sysAllGroups are the groups the System domain and the Overview read.
var sysAllGroups = []string{
	"system_page/system_info_group", "system_page/bootc_status_group", "system_page/health_group",
	"updates_page/bootc_updates_group", "updates_page/sysupdate_updates_group",
}

var sysFixedNow = time.Date(2026, 11, 10, 14, 30, 0, 0, time.Local)

func sysEnv(t *testing.T, cfg *config.Config, dryRun bool) (*Env, *sysPosts) {
	t.Helper()
	q := &sysPosts{}
	env := NewEnv(cfg, dryRun, q.post)
	env.Now = func() time.Time { return sysFixedNow }
	log, err := activity.Open("", dryRun)
	if err != nil {
		t.Fatal(err)
	}
	env.Activity = log
	return env, q
}

const sysOSRelease = `PRETTY_NAME="Snow Linux"
NAME="Snow Linux"
VERSION_ID="13"
VERSION="13 (trixie)"
ID="snow"
HOME_URL="https://frostyard.org/"
BUG_REPORT_URL="https://github.com/frostyard/snosi/issues"
IMAGE_VERSION="20261105101952"
`

// sysFake is every backend System uses, with the state a test sets. Its
// methods run on worker goroutines, so they lock.
type sysFake struct {
	mu sync.Mutex

	files map[string]string

	bootcHost, bootcScript bool
	status                 *bootc.Status
	statusErr              error
	statusReads            int

	nativeHost, nativeScript bool
	suStatus                 sysupdate.Status
	rollback                 string

	stageLines []string
	stageErr   error
	// after is what a stage run leaves: the status read after it.
	after   *bootc.Status
	suAfter *sysupdate.Status
	stages  int
	dryRun  bool
}

func newSysFake() *sysFake {
	return &sysFake{files: map[string]string{
		sysOSReleasePath:              sysOSRelease,
		sysUptimePath:                 "274320.52 1000000.00\n",
		sysDMIProductName:             "21KCCTO1WW\n",
		sysDMIProductVer:              "ThinkPad X1 Carbon Gen 12\n",
		sysinfo.SecureBootVariable:    string([]byte{6, 0, 0, 0, 1}),
		"/sys/class/dmi/id/sys_other": "",
	}}
}

func (f *sysFake) install(s *System) {
	s.readFile = func(path string) ([]byte, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		v, ok := f.files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(v), nil
	}
	s.hostname = func() (string, error) { return "frosty-x1", nil }
	s.kernel = func() string { return "6.17.7-amd64" }
	s.bootcBooted = func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.bootcHost }
	s.bootcStageAvailable = func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.bootcScript }
	s.bootcStatus = func(context.Context) (*bootc.Status, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.statusReads++
		return f.status, f.statusErr
	}
	s.bootcDryRun = func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.dryRun }
	s.bootcStage = func(_ context.Context, ch chan<- bootc.ProgressEvent) error { return f.stage(ch) }
	s.nativeAB = func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.nativeHost }
	s.sysupdateStageAvailable = func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.nativeScript }
	s.sysupdateStatus = func() sysupdate.Status { f.mu.Lock(); defer f.mu.Unlock(); return f.suStatus }
	s.sysupdateRollback = func(context.Context) (string, bool) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.rollback, f.rollback != ""
	}
	s.sysupdateDryRun = s.bootcDryRun
	s.sysupdateStage = func(_ context.Context, ch chan<- sysupdate.ProgressEvent) error { return f.stage(ch) }
}

// stage acts as the stage script: it prints its lines, and a live run that
// works leaves the "after" state.
func (f *sysFake) stage(ch chan<- bootc.ProgressEvent) error {
	defer close(ch)
	f.mu.Lock()
	f.stages++
	lines, err, dry := f.stageLines, f.stageErr, f.dryRun
	if dry {
		lines = []string{"[DRY-RUN] would run /usr/libexec/bootc-update-stage"}
	}
	if err == nil && !dry {
		if f.after != nil {
			f.status = f.after
		}
		if f.suAfter != nil {
			f.suStatus = *f.suAfter
		}
	}
	f.mu.Unlock()
	for _, l := range lines {
		ch <- bootc.ProgressEvent{Type: bootc.EventMessage, Message: l}
	}
	if err != nil {
		return err
	}
	ch <- bootc.ProgressEvent{Type: bootc.EventComplete, Message: "Staging complete"}
	return nil
}

func (f *sysFake) counts() (reads, stages int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statusReads, f.stages
}

const (
	sysDigestBooted   = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	sysDigestStaged   = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	sysDigestRollback = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
)

func sysDeployment(version, digest string, pinned bool) *bootc.Deployment {
	if version == "" {
		return nil
	}
	return &bootc.Deployment{Pinned: pinned, Image: &bootc.ImageStatus{
		Image:   bootc.ImageReference{Image: "ghcr.io/frostyard/snow:stable", Transport: "registry"},
		Version: version, ImageDigest: digest, Timestamp: "2026-11-05T10:19:52Z",
	}}
}

func sysBootcStatus(booted, staged, rollback string) *bootc.Status {
	return &bootc.Status{Status: bootc.StatusInfo{
		Booted:   sysDeployment(booted, sysDigestBooted, false),
		Staged:   sysDeployment(staged, sysDigestStaged, false),
		Rollback: sysDeployment(rollback, sysDigestRollback, true),
	}}
}

// sysStart builds a System with the fake and runs its first load.
func sysStart(t *testing.T, env *Env, q *sysPosts, f *sysFake) *System {
	t.Helper()
	s := NewSystem(env)
	f.install(s)
	s.Start()
	q.until(t, "the first load", func() bool { return s.loaded && !s.checking })
	return s
}

// sysRunJobs queues jobs and runs them to their end.
func sysRunJobs(t *testing.T, env *Env, q *sysPosts, jobs []*frostui.Job) {
	t.Helper()
	if len(jobs) == 0 {
		t.Fatal("no job to run")
	}
	env.Jobs.Enqueue(jobs...)
	q.until(t, "the jobs to end", func() bool { return !env.Jobs.Busy() })
}

func TestSystemFillsInfoFromBootcStatus(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	f := newSysFake()
	f.bootcHost, f.bootcScript = true, true
	f.status = sysBootcStatus("2026.11.05", "", "2026.10.29")
	s := sysStart(t, env, q, f)

	info := s.Info()
	want := SystemInfo{
		Loaded: true, Kind: KindBootc, OSName: "Snow Linux", OSVersion: "20261105101952", Channel: "stable",
		ImageRef: "ghcr.io/frostyard/snow:stable",
		Booted:   Deployment{Present: true, Version: "2026.11.05", Digest: pageview.ShortDigest(sysDigestBooted)},
		Rollback: Deployment{Present: true, Version: "2026.10.29", Digest: pageview.ShortDigest(sysDigestRollback), Pinned: true},
		Hostname: "frosty-x1", Kernel: "6.17.7-amd64", Model: "ThinkPad X1 Carbon Gen 12", SecureBoot: "on",
		Uptime: "up 3d 4h 12m",
	}
	if info != want {
		t.Errorf("Info() =\n %+v\nwant\n %+v", info, want)
	}
	entries, ok := s.OSRelease()
	if !ok || len(entries) != 8 {
		t.Errorf("OSRelease() = %d entries, ok %v", len(entries), ok)
	}
	if built := s.ImageBuilt(); built != "2026-11-05T10:19:52Z" {
		t.Errorf("ImageBuilt() = %q", built)
	}

	src := s.Source()
	if src == nil || !src.Status().Available {
		t.Fatalf("the bootc source is not available: %+v", src)
	}
	items := src.Items()
	if len(items) != 1 || items[0].Key != imageItemKey || items[0].From != "2026.11.05" || items[0].Action != "Stage" ||
		items[0].Title != "Snow Linux" || items[0].Detail != "ghcr.io/frostyard/snow:stable" {
		t.Errorf("Items() = %+v", items)
	}
	if note := src.Status().Note; note != pageview.BootcUpdateSubtitle(false, "") {
		t.Errorf("Note = %q", note)
	}
	if jobs := src.Jobs(); len(jobs) != 1 || jobs[0].Source != string(SourceImage) || jobs[0].Key != imageItemKey {
		t.Errorf("Jobs() = %+v", jobs)
	}
}

func TestSystemUptimeAdvancesWithoutReading(t *testing.T) {
	env, q := sysEnv(t, sysConfig(), false)
	s := sysStart(t, env, q, newSysFake())
	now := sysFixedNow
	env.Now = func() time.Time { return now.Add(2 * time.Hour) }
	if got := s.Info().Uptime; got != "up 3d 6h 12m" {
		t.Errorf("Uptime two hours later = %q", got)
	}
}

func TestSystemStagedImageHasNoItem(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	f := newSysFake()
	f.bootcHost, f.bootcScript = true, true
	f.status = sysBootcStatus("2026.11.05", "2026.11.08", "2026.10.29")
	s := sysStart(t, env, q, f)

	if got := s.Info().Staged; !got.Present || got.Version != "2026.11.08" {
		t.Errorf("Staged = %+v", got)
	}
	src := s.Source()
	if items := src.Items(); len(items) != 0 {
		t.Errorf("a staged image still offers %+v", items)
	}
	if note := src.Status().Note; note != "Staged 2026.11.08 · applies on restart" {
		t.Errorf("Note = %q", note)
	}
	if jobs := src.Jobs(); len(jobs) != 0 {
		t.Errorf("Update everything would stage again: %+v", jobs)
	}
	// The old page's button stays: the source's extra offers a new check.
	tt := ui.NewTester(func(c *ui.Context) { src.Extra(c) }, 900, 200)
	if !tt.HasText("Check and stage") {
		t.Errorf("no stage button in the extra: %q", tt.Texts())
	}
}

func TestSystemStageJobStagesAndRecords(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	f := newSysFake()
	f.bootcHost, f.bootcScript = true, true
	f.status = sysBootcStatus("2026.11.05", "", "2026.10.29")
	f.stageLines = []string{"Pulling ghcr.io/frostyard/snow:stable", "Staged sha256:2222"}
	f.after = sysBootcStatus("2026.11.05", "2026.11.08", "2026.10.29")
	s := sysStart(t, env, q, f)

	sysRunJobs(t, env, q, s.Source().Jobs())
	if _, stages := f.counts(); stages != 1 {
		t.Errorf("stage ran %d times", stages)
	}
	if got := s.Info().Staged; !got.Present || got.Version != "2026.11.08" {
		t.Errorf("the hero does not show the staged image: %+v", got)
	}
	if items := s.Source().Items(); len(items) != 0 {
		t.Errorf("items after staging: %+v", items)
	}
	toasts := env.TakeToasts()
	if len(toasts) != 1 || toasts[0].Title != actionmsg.BootcStage(false, true) || toasts[0].Type != "success" {
		t.Errorf("toasts = %+v", toasts)
	}
	recent := env.Activity.Recent(5)
	if len(recent) != 1 || recent[0].Source != "image" || recent[0].Title != "Staged Snow Linux 2026.11.08" {
		t.Errorf("activity = %+v", recent)
	}
	// The job's status followed the script's lines.
	hist := env.Jobs.History()
	if len(hist) != 1 || hist[0].State != frostui.JobDone {
		t.Fatalf("history = %+v", hist)
	}
}

func TestSystemStageThatFindsNothingClearsTheItem(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	f := newSysFake()
	f.bootcHost, f.bootcScript = true, true
	f.status = sysBootcStatus("2026.11.05", "", "")
	f.stageLines = []string{"Already on the newest image"}
	s := sysStart(t, env, q, f)

	sysRunJobs(t, env, q, s.Source().Jobs())
	src := s.Source()
	if items := src.Items(); len(items) != 0 {
		t.Errorf("the host is current but the source offers %+v", items)
	}
	if note := src.Status().Note; note != "Already on the newest image" {
		t.Errorf("Note = %q", note)
	}
	if toasts := env.TakeToasts(); len(toasts) != 1 || toasts[0].Title != "System is up to date" {
		t.Errorf("toasts = %+v", toasts)
	}
	if recent := env.Activity.Recent(5); len(recent) != 0 {
		t.Errorf("nothing changed, but the activity has %+v", recent)
	}
	// A status read keeps what the run found.
	src.Refresh()
	q.until(t, "the refresh", func() bool { return !s.checking })
	if items := src.Items(); len(items) != 0 {
		t.Errorf("a refresh brought back %+v", items)
	}
}

func TestSystemStageDryRunChangesNothing(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), true)
	f := newSysFake()
	f.bootcHost, f.bootcScript, f.dryRun = true, true, true
	f.status = sysBootcStatus("2026.11.05", "", "")
	f.after = sysBootcStatus("2026.11.05", "2026.11.08", "")
	s := sysStart(t, env, q, f)

	sysRunJobs(t, env, q, s.Source().Jobs())
	if items := s.Source().Items(); len(items) != 1 {
		t.Errorf("a dry run changed the items: %+v", items)
	}
	if s.Info().Staged.Present {
		t.Error("a dry run staged an image")
	}
	if toasts := env.TakeToasts(); len(toasts) != 1 || toasts[0].Title != actionmsg.BootcStage(true, false) {
		t.Errorf("toasts = %+v", toasts)
	}
	if recent := env.Activity.Recent(5); len(recent) != 0 {
		t.Errorf("a dry run recorded %+v", recent)
	}
}

func TestSystemStageFailureKeepsTheItem(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	f := newSysFake()
	f.bootcHost, f.bootcScript = true, true
	f.status = sysBootcStatus("2026.11.05", "", "")
	f.stageLines = []string{"Error: registry unreachable"}
	f.stageErr = &bootc.Error{Message: "update staging failed (exit 1): Error: registry unreachable"}
	s := sysStart(t, env, q, f)

	sysRunJobs(t, env, q, s.Source().Jobs())
	src := s.Source()
	want := "Update failed: update staging failed (exit 1): Error: registry unreachable"
	if st := src.Status(); st.Err != want {
		t.Errorf("Err = %q", st.Err)
	}
	if items := src.Items(); len(items) != 1 {
		t.Errorf("a failed stage dropped the item: %+v", items)
	}
	if toasts := env.TakeToasts(); len(toasts) != 1 || toasts[0].Title != want || toasts[0].Type != "error" {
		t.Errorf("toasts = %+v", toasts)
	}
	if hist := env.Jobs.History(); len(hist) != 1 || hist[0].State != frostui.JobFailed {
		t.Errorf("history = %+v", hist)
	}
}

func TestSystemBootcStatusErrorIsShown(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	f := newSysFake()
	f.bootcHost, f.bootcScript = true, true
	f.statusErr = errors.New("bootc status failed (exit 1): no")
	s := sysStart(t, env, q, f)

	if got := s.DeploymentsError(); got != "Error: bootc status failed (exit 1): no" {
		t.Errorf("DeploymentsError = %q", got)
	}
	src := s.Source()
	if st := src.Status(); !st.Available || st.Err == "" {
		t.Errorf("Status = %+v", st)
	}
	if items := src.Items(); len(items) != 0 {
		t.Errorf("items without a status: %+v", items)
	}
}

func sysNativeStatus(outcome sysupdate.CheckOutcome, staged string) sysupdate.Status {
	st := sysupdate.Status{Check: &sysupdate.UpdateCheck{
		Outcome: outcome, CheckedAt: "2026-11-10T13:05:00Z", Image: "ghcr.io/frostyard/snow-native:stable",
		RunningVersion: "20261105101952",
	}}
	if staged != "" {
		st.Staged = &sysupdate.StagedUpdate{Image: "ghcr.io/frostyard/snow-native:stable", Version: staged, StagedAt: "2026-11-10T13:05:00Z"}
	}
	return st
}

func TestSystemNativeABSource(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	f := newSysFake()
	f.nativeHost, f.nativeScript = true, true
	f.suStatus = sysNativeStatus(sysupdate.OutcomeFailed, "")
	f.rollback = "20261020000000"
	f.suAfter = &sysupdate.Status{}
	*f.suAfter = sysNativeStatus(sysupdate.OutcomeStaged, "20261108000000")
	s := sysStart(t, env, q, f)

	info := s.Info()
	if info.Kind != KindNativeAB || info.Channel != "stable" || info.Booted.Version != "20261105101952" ||
		info.Rollback.Version != "20261020000000" || info.Staged.Present {
		t.Errorf("Info() = %+v", info)
	}
	src := s.Source()
	if note := src.Status().Note; note != pageview.SysupdateUpdateSubtitle("failed", "", "") {
		t.Errorf("Note = %q", note)
	}
	if items := src.Items(); len(items) != 1 {
		t.Fatalf("a failed check offers no stage: %+v", items)
	}
	tt := ui.NewTester(func(c *ui.Context) { src.Extra(c) }, 1000, 200)
	if !tt.HasText(pageview.SysupdateRollbackSubtitle("20261020000000")) {
		t.Errorf("no rollback line: %q", tt.Texts())
	}

	sysRunJobs(t, env, q, src.Jobs())
	if got := s.Info().Staged; !got.Present || got.Version != "20261108000000" {
		t.Errorf("Staged = %+v", got)
	}
	if toasts := env.TakeToasts(); len(toasts) != 1 || toasts[0].Title != actionmsg.SysupdateStage(false, true) {
		t.Errorf("toasts = %+v", toasts)
	}
	if recent := env.Activity.Recent(1); len(recent) != 1 || recent[0].Title != "Staged Snow Linux 20261108000000" {
		t.Errorf("activity = %+v", recent)
	}
}

func TestSystemNativeABCurrentOffersNothing(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	f := newSysFake()
	f.nativeHost, f.nativeScript = true, true
	f.suStatus = sysNativeStatus(sysupdate.OutcomeCurrent, "")
	s := sysStart(t, env, q, f)

	src := s.Source()
	if items := src.Items(); len(items) != 0 {
		t.Errorf("items = %+v", items)
	}
	if note := src.Status().Note; !strings.HasPrefix(note, "System is up to date (checked ") {
		t.Errorf("Note = %q", note)
	}
	// The stage button stays reachable.
	if jobs := src.Jobs(imageItemKey); len(jobs) != 1 {
		t.Errorf("Jobs(image) = %+v", jobs)
	}
	if jobs := src.Jobs(); len(jobs) != 0 {
		t.Errorf("Update everything would stage: %+v", jobs)
	}
}

func TestSystemNeitherKindIsUnavailable(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	s := sysStart(t, env, q, newSysFake())

	src := s.Source()
	if src == nil {
		t.Fatal("Source() is nil with the groups on")
	}
	if st := src.Status(); st.Available {
		t.Errorf("the source applies on a plain host: %+v", st)
	}
	if items, jobs := src.Items(), src.Jobs(imageItemKey); len(items) != 0 || len(jobs) != 0 {
		t.Errorf("items %+v jobs %+v", items, jobs)
	}
	if info := s.Info(); info.Kind != "" || info.Booted.Present || info.Hostname != "frosty-x1" {
		t.Errorf("Info() = %+v", info)
	}
}

func TestSystemGroupsGateTheSource(t *testing.T) {
	t.Run("no updates group", func(t *testing.T) {
		env, q := sysEnv(t, sysConfig("system_page/bootc_status_group"), false)
		f := newSysFake()
		f.bootcHost, f.bootcScript = true, true
		f.status = sysBootcStatus("2026.11.05", "", "")
		s := sysStart(t, env, q, f)
		if s.Source() != nil {
			t.Error("Source() is not nil with both updates groups off")
		}
		if !s.Info().Booted.Present {
			t.Error("the hero's group did not read the deployments")
		}
	})
	t.Run("nothing reads the deployments", func(t *testing.T) {
		env, q := sysEnv(t, sysConfig("system_page/system_info_group"), false)
		f := newSysFake()
		f.bootcHost, f.bootcScript = true, true
		f.status = sysBootcStatus("2026.11.05", "", "")
		s := sysStart(t, env, q, f)
		if reads, _ := f.counts(); reads != 0 {
			t.Errorf("bootc status ran %d times with its groups off", reads)
		}
		if info := s.Info(); info.Booted.Present || info.Hostname == "" {
			t.Errorf("Info() = %+v", info)
		}
	})
	t.Run("the other kind's group", func(t *testing.T) {
		env, q := sysEnv(t, sysConfig("updates_page/sysupdate_updates_group"), false)
		f := newSysFake()
		f.bootcHost, f.bootcScript = true, true
		f.status = sysBootcStatus("2026.11.05", "", "")
		s := sysStart(t, env, q, f)
		if src := s.Source(); src == nil || src.Status().Available || len(src.Items()) != 0 {
			t.Errorf("a bootc host offers a stage under the native group")
		}
	})
	t.Run("no stage script", func(t *testing.T) {
		env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
		f := newSysFake()
		f.bootcHost = true
		f.status = sysBootcStatus("2026.11.05", "", "")
		s := sysStart(t, env, q, f)
		if st := s.Source().Status(); st.Available {
			t.Error("the source applies without its stage script")
		}
		if !s.Info().Booted.Present {
			t.Error("the hero lost the deployments")
		}
	})
}

func TestSystemDroppedStageKeepsTheDeployments(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	f := newSysFake()
	f.bootcHost, f.bootcScript = true, true
	f.status = sysBootcStatus("2026.11.05", "", "2026.10.29")
	s := sysStart(t, env, q, f)

	release := make(chan struct{})
	env.Jobs.Enqueue(&frostui.Job{Source: "flatpak", Key: "app", Title: "Updating an app",
		Run: func(ctx context.Context, _ *frostui.Progress) error {
			select {
			case <-release:
			case <-ctx.Done():
			}
			return ctx.Err()
		}})
	env.Jobs.Enqueue(s.Source().Jobs()...)
	env.Jobs.CancelAll()
	close(release)
	q.until(t, "the queue to empty", func() bool { return !env.Jobs.Busy() })

	if _, stages := f.counts(); stages != 0 {
		t.Errorf("a dropped stage ran %d times", stages)
	}
	if info := s.Info(); !info.Booted.Present || info.Booted.Version != "2026.11.05" || !info.Rollback.Present {
		t.Errorf("a dropped stage changed the deployments: %+v", info)
	}
	if items := s.Source().Items(); len(items) != 1 {
		t.Errorf("items after a dropped stage: %+v", items)
	}
	for _, toast := range env.TakeToasts() {
		if toast.Type == "error" {
			t.Errorf("error toast %q", toast.Title)
		}
	}
}

func TestSystemUpdateQueuesOneStageAtATime(t *testing.T) {
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	f := newSysFake()
	f.bootcHost, f.bootcScript = true, true
	f.status = sysBootcStatus("2026.11.05", "", "")
	s := sysStart(t, env, q, f)

	src := s.Source()
	src.Update(imageItemKey)
	src.Update(imageItemKey)
	if n := len(env.Jobs.Queued()); n != 0 || env.Jobs.Current() == nil {
		t.Errorf("queued %d more stage jobs behind the running one", n)
	}
	if jobs := src.Jobs(); len(jobs) != 0 {
		t.Errorf("Jobs() while a stage runs = %+v", jobs)
	}
	q.until(t, "the stage to end", func() bool { return !env.Jobs.Busy() })
}

func TestSystemReadsBootcFromSnosiWhenStatusNeedsRoot(t *testing.T) {
	f := newSysFake()
	f.bootcHost = false // `bootc status` refused: it needs root
	f.bootcScript = true
	f.suStatus = sysupdate.Status{
		Check: &sysupdate.UpdateCheck{Outcome: sysupdate.OutcomeStaged, CheckedAt: "2026-11-10T11:00:57-05:00",
			Image: "ghcr.io/frostyard/snow:latest", RunningVersion: "20261110101952", RemoteVersion: "20261110150546"},
		Staged: &sysupdate.StagedUpdate{Image: "ghcr.io/frostyard/snow:latest",
			Digest: "sha256:fd9d0a3d966db12fe9f66c6e6f6c4e88075d858a81d1771a1c4edaddde7bb0a9", StagedAt: "2026-11-10T11:00:57-05:00"},
	}
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	s := sysStart(t, env, q, f)
	info := s.Info()
	if info.Kind != KindBootc || info.ImageRef != "ghcr.io/frostyard/snow:latest" || info.Channel != "latest" {
		t.Errorf("info = %+v", info)
	}
	if !info.Booted.Present || info.Booted.Version != "20261110101952" {
		t.Errorf("booted = %+v", info.Booted)
	}
	if !info.Staged.Present || info.Staged.Version != "20261110150546" || info.Staged.Digest == "" {
		t.Errorf("staged = %+v", info.Staged)
	}
	if info.Rollback.Present || !info.RollbackUnknown || rollbackEmpty(info) != "unknown" {
		t.Errorf("rollback = %+v (unknown %v), want unknown", info.Rollback, info.RollbackUnknown)
	}
	src := s.Source()
	if src == nil || !src.Status().Available || len(src.Items()) != 0 {
		t.Fatalf("source with a staged image: available %v items %+v", src != nil && src.Status().Available, src.Items())
	}

	// The stager found the host current: nothing to offer.
	f.suStatus = sysupdate.Status{Check: &sysupdate.UpdateCheck{Outcome: sysupdate.OutcomeCurrent, RunningVersion: "20261110101952"}}
	env2, q2 := sysEnv(t, sysConfig(sysAllGroups...), false)
	s2 := sysStart(t, env2, q2, f)
	if s2.Info().Kind != KindBootc || len(s2.Source().Items()) != 0 {
		t.Errorf("a current host: kind %q items %+v", s2.Info().Kind, s2.Source().Items())
	}

	// No state files, as after a reboot: still a bootc host with its
	// stager, so Stage stays offered and the booted version comes from
	// os-release.
	f.suStatus = sysupdate.Status{}
	env3, q3 := sysEnv(t, sysConfig(sysAllGroups...), false)
	s3 := sysStart(t, env3, q3, f)
	info3 := s3.Info()
	if info3.Kind != KindBootc || !info3.Booted.Present || info3.Booted.Version == "" || info3.Staged.Present {
		t.Errorf("after a reboot: %+v", info3)
	}
	if src := s3.Source(); src == nil || !src.Status().Available || len(src.Items()) != 1 {
		t.Errorf("after a reboot the image source offers %+v", src.Items())
	}

	// Without the stager the host is not one ChairLift can stage.
	f.bootcScript = false
	env4, q4 := sysEnv(t, sysConfig(sysAllGroups...), false)
	if s4 := sysStart(t, env4, q4, f); s4.Info().Kind != "" {
		t.Errorf("without the stager: %+v", s4.Info())
	}
}

func TestSystemFailedStatusKeepsTheKnownDeployments(t *testing.T) {
	f := newSysFake()
	f.bootcHost, f.bootcScript = true, true
	f.status = sysBootcStatus("2026.11.05", "", "2026.10.29")
	env, q := sysEnv(t, sysConfig(sysAllGroups...), false)
	s := sysStart(t, env, q, f)
	if !s.Info().Booted.Present || len(s.Source().Items()) != 1 {
		t.Fatalf("first load: %+v items %+v", s.Info(), s.Source().Items())
	}
	f.mu.Lock()
	f.status, f.statusErr = nil, errors.New("bootc status timed out")
	f.mu.Unlock()
	s.Source().Refresh()
	q.until(t, "the refresh", func() bool { return !s.checking })
	info := s.Info()
	if info.Booted.Version != "2026.11.05" || info.Rollback.Version != "2026.10.29" {
		t.Errorf("a failed refresh dropped the deployments: %+v", info)
	}
	if s.DeploymentsError() != "" || !strings.Contains(s.DeploymentsRefreshError(), "timed out") {
		t.Errorf("errors: %q / %q", s.DeploymentsError(), s.DeploymentsRefreshError())
	}
	st := s.Source().Status()
	if !strings.Contains(st.Err, "timed out") || len(s.Source().Items()) != 1 {
		t.Errorf("status %+v items %+v: want the error shown and the item kept", st, s.Source().Items())
	}
	// A good read clears the error.
	f.mu.Lock()
	f.status, f.statusErr = sysBootcStatus("2026.11.05", "", "2026.10.29"), nil
	f.mu.Unlock()
	s.Source().Refresh()
	q.until(t, "the refresh", func() bool { return !s.checking })
	if s.DeploymentsRefreshError() != "" {
		t.Errorf("a good read kept the error %q", s.DeploymentsRefreshError())
	}
}
