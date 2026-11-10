package views

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/bootc"
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/sysinfo"
	"github.com/frostyard/chairlift/internal/sysupdate"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
	"github.com/frostyard/chairlift/internal/views/pageview"
)

// The kinds of image-based hosts, as SystemInfo.Kind names them.
const (
	KindBootc    = "bootc"
	KindNativeAB = "native A/B"
)

// imageItemKey is the key of the system image source's one item and of
// its stage job.
const imageItemKey = "image"

// The fixed files System reads. They are never configuration.
const (
	sysOSReleasePath  = "/etc/os-release"
	sysUptimePath     = "/proc/uptime"
	sysDMIProductName = "/sys/class/dmi/id/product_name"
	sysDMIProductVer  = "/sys/class/dmi/id/product_version"
)

// systemBackends are where System reads and what it runs. NewSystem sets
// the real ones; tests replace them, so no test runs bootc, lsblk or pkexec.
type systemBackends struct {
	readFile func(path string) ([]byte, error)
	hostname func() (string, error)
	kernel   func() string

	// bootc: the boot gate, the stage script's presence, the unprivileged
	// status read and the fixed `pkexec /usr/libexec/bootc-update-stage`.
	bootcBooted         func() bool
	bootcStageAvailable func() bool
	bootcStatus         func(ctx context.Context) (*bootc.Status, error)
	bootcStage          func(ctx context.Context, ch chan<- bootc.ProgressEvent) error
	bootcDryRun         func() bool

	// native A/B: the marker gate, the stager's presence, the /run/snosi
	// state files, the rollback slot and the fixed
	// `pkexec /usr/libexec/snosi-sysupdate-stage`.
	nativeAB                func() bool
	sysupdateStageAvailable func() bool
	sysupdateStatus         func() sysupdate.Status
	sysupdateRollback       func(ctx context.Context) (string, bool)
	sysupdateStage          func(ctx context.Context, ch chan<- sysupdate.ProgressEvent) error
	sysupdateDryRun         func() bool
}

func defaultSystemBackends() systemBackends {
	return systemBackends{
		readFile:                os.ReadFile,
		hostname:                os.Hostname,
		kernel:                  sysinfo.Kernel,
		bootcBooted:             bootc.IsBootcBootedCached,
		bootcStageAvailable:     bootc.StageScriptAvailable,
		bootcStatus:             bootc.GetStatus,
		bootcStage:              bootc.StageUpdate,
		bootcDryRun:             bootc.IsDryRun,
		nativeAB:                sysupdate.IsNativeABCached,
		sysupdateStageAvailable: sysupdate.StageScriptAvailable,
		sysupdateStatus:         sysupdate.GetStatus,
		sysupdateRollback:       sysupdate.RollbackVersion,
		sysupdateStage:          sysupdate.StageUpdate,
		sysupdateDryRun:         sysupdate.IsDryRun,
	}
}

// hostFacts are what one read of the host found. They are cheap local
// reads (no external command), which the status bar and the rail's host
// card need whatever the configuration.
type hostFacts struct {
	osRelease   []pageview.OSReleaseEntry
	osReleaseOK bool
	hostname    string
	kernel      string
	model       string
	secureBoot  sysinfo.SecureBootState
	uptime      time.Duration
	uptimeOK    bool
}

// deployments are what one status read found: bootc's deployments, or the
// native A/B state files and slots.
type deployments struct {
	kind      string
	stageable bool   // the stage script is installed
	err       string // the status read failed (bootc)
	imageRef  string
	builtAt   string // the booted image's build time (bootc)
	// rollbackUnknown: the source cannot tell the rollback.
	rollbackUnknown bool
	// refreshErr is the error of a read that failed after a good one: the
	// last known deployments stay, and the error shows beside them.
	refreshErr string

	booted, staged, rollback Deployment
	// stagedID identifies the staged image (its full digest or version
	// and time), so a stage run can tell a new image from one staged
	// before it.
	stagedID string

	// native A/B: the state files' presentation and the running version
	// the stager saw.
	outcome, stagedVersion, checkedAt string
	runningVersion                    string
	rollbackVersion                   string
}

// System holds the deployments, the system image update source and the
// host facts. It belongs to the UI thread.
type System struct {
	env *Env
	systemBackends

	started  bool
	loaded   bool
	facts    hostFacts
	uptimeAt time.Time

	deps        deployments
	depsLoaded  bool
	checking    bool
	checkedAt   time.Time
	refreshGate actionstate.RefreshGate

	// The last live stage run of this session that staged nothing: the
	// image was current then. Its result line is the source's note until
	// the next run.
	runCurrent bool
	lastRun    string
	// stageErr is the last stage run's failure, until a run or a status
	// read succeeds.
	stageErr string

	info SystemInfo
	src  *imageSource
}

// NewSystem returns the system state; nothing loads until Start.
func NewSystem(env *Env) *System {
	s := &System{env: env, systemBackends: defaultSystemBackends()}
	s.src = &imageSource{s: s}
	return s
}

// Start begins the first load: the host facts always, the deployments when
// a group that shows them is enabled.
func (s *System) Start() {
	if s.started {
		return
	}
	s.started = true
	Go(s.env, s.readFacts, s.applyFacts)
	s.loadDeployments()
}

// Info is what the Overview hero, status bar and Maintenance show. It
// reads nothing: the window calls it every frame.
func (s *System) Info() SystemInfo {
	info := s.info
	if s.facts.uptimeOK {
		info.Uptime = sysinfo.FormatUptime(s.facts.uptime + s.now().Sub(s.uptimeAt))
	}
	return info
}

// OSRelease is the parsed os-release, for the System details card; ok is
// false when the file could not be read or parsed in full.
func (s *System) OSRelease() (entries []pageview.OSReleaseEntry, ok bool) {
	return s.facts.osRelease, s.facts.osReleaseOK
}

// ImageBuilt is when the booted image was built, as bootc reports it ("" when
// unknown): the old System page's "Built" row.
func (s *System) ImageBuilt() string { return s.deps.builtAt }

// DeploymentsLoading reports whether the first status read still runs.
func (s *System) DeploymentsLoading() bool { return s.checking && !s.depsLoaded }

// DeploymentsError is the last status read's failure, "" when it worked.
func (s *System) DeploymentsError() string { return s.deps.err }

// DeploymentsRefreshError is the error of the latest read when it failed
// after a good one: the deployments shown are the last known.
func (s *System) DeploymentsRefreshError() string { return s.deps.refreshErr }

// Source is the system image update source (bootc or native A/B), nil when
// neither updates group is enabled. Its status is unavailable until a load
// found a host of the group's kind with its stage script.
func (s *System) Source() UpdateSource {
	if !s.env.Enabled("updates_page", "bootc_updates_group") && !s.env.Enabled("updates_page", "sysupdate_updates_group") {
		return nil
	}
	return s.src
}

func (s *System) now() time.Time {
	if s.env.Now != nil {
		return s.env.Now()
	}
	return time.Now()
}

// wantsDeployments says which status reads the configuration asks for:
// bootc's for the hero (bootc_status_group) or the bootc updates group,
// native A/B's for the hero or the sysupdate updates group.
func (s *System) wantsDeployments() (wantBootc, wantNative bool) {
	hero := s.env.Enabled("system_page", "bootc_status_group")
	return hero || s.env.Enabled("updates_page", "bootc_updates_group"),
		hero || s.env.Enabled("updates_page", "sysupdate_updates_group")
}

func (s *System) readTrimmed(path string) string {
	data, err := s.readFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// readFacts reads the host facts. It runs on a goroutine and reads no state.
func (s *System) readFacts() hostFacts {
	var f hostFacts
	if data, err := s.readFile(sysOSReleasePath); err == nil {
		entries, parseErr := pageview.ParseOSRelease(bytes.NewReader(data))
		f.osRelease, f.osReleaseOK = entries, parseErr == nil
	}
	if host, err := s.hostname(); err == nil {
		f.hostname = host
	}
	f.kernel = s.kernel()
	f.model = sysinfo.Model(s.readTrimmed(sysDMIProductName), s.readTrimmed(sysDMIProductVer))
	if data, err := s.readFile(sysinfo.SecureBootVariable); err == nil {
		f.secureBoot = sysinfo.ParseSecureBoot(data)
	}
	if data, err := s.readFile(sysUptimePath); err == nil {
		if up, err := sysinfo.ParseUptime(data); err == nil {
			f.uptime, f.uptimeOK = up, true
		}
	}
	return f
}

func (s *System) applyFacts(f hostFacts) {
	s.facts, s.uptimeAt, s.loaded = f, s.now(), true
	s.compose()
}

// loadDeployments starts a status read, unless the configuration wants
// none. A newer read wins over an older one still running.
func (s *System) loadDeployments() {
	wantBootc, wantNative := s.wantsDeployments()
	if !wantBootc && !wantNative {
		return
	}
	gen := s.refreshGate.Begin()
	s.checking = true
	Go(s.env, func() deployments {
		ctx, cancel := context.WithTimeout(context.Background(), bootc.DefaultTimeout)
		defer cancel()
		return s.readDeployments(ctx, wantBootc, wantNative)
	}, func(d deployments) {
		if !s.refreshGate.IsCurrent(gen) {
			return
		}
		s.checking = false
		s.stageErr = ""
		s.applyDeployments(d)
	})
}

// readDeployments reads bootc's status on bootc hosts and the native A/B
// state on native A/B hosts; the two gates never both hold. It runs on a
// goroutine and reads no state.
func (s *System) readDeployments(ctx context.Context, wantBootc, wantNative bool) deployments {
	var d deployments
	switch {
	case wantBootc && s.bootcBooted():
		d.kind = KindBootc
		d.stageable = s.bootcStageAvailable()
		status, err := s.bootcStatus(ctx)
		if err != nil {
			d.err = fmt.Sprintf("Error: %v", err)
			return d
		}
		d.fromBootc(status)
	case wantNative && s.nativeAB():
		d.kind = KindNativeAB
		d.stageable = s.sysupdateStageAvailable()
		d.rollbackVersion, _ = s.sysupdateRollback(ctx)
		d.fromSysupdate(s.sysupdateStatus())
	case wantBootc && !s.nativeAB() && s.bootcStageAvailable():
		// Newer bootc answers `bootc status` only as root, so the
		// unprivileged boot gate reads false on a bootc host. The snosi
		// stager records what it checked and staged in /run/snosi on bootc
		// hosts too: read the deployments from there, or nothing.
		// After a reboot /run/snosi is empty until the stager's timer
		// runs: the host is still a bootc host with its stager (core
		// ADR-0005), so Stage stays offered and the booted version comes
		// from os-release.
		d.kind = KindBootc
		d.stageable = true
		d.fromSnosiBootc(s.sysupdateStatus())
	}
	return d
}

// fromSnosiBootc reads a bootc host from the snosi stager's state files,
// for when `bootc status` is root-only: the image, the running version, and
// what is staged. The rollback is unknown there.
func (d *deployments) fromSnosiBootc(st sysupdate.Status) {
	d.outcome, d.stagedVersion, d.checkedAt = st.Presentation()
	if st.Check != nil {
		d.imageRef, d.runningVersion = st.Check.Image, st.Check.RunningVersion
	}
	d.booted = Deployment{Present: true, Version: d.runningVersion}
	d.rollbackUnknown = true
	if !st.IsStaged() {
		return
	}
	version := d.stagedVersion
	if c := st.Check; c != nil && c.Outcome == sysupdate.OutcomeStaged && c.RemoteVersion != "" {
		version = c.RemoteVersion
	}
	d.staged = Deployment{Present: true, Version: version}
	if staged := st.Staged; staged != nil {
		if d.imageRef == "" {
			d.imageRef = staged.Image
		}
		d.staged.Digest = pageview.ShortDigest(staged.Digest)
		d.stagedID = staged.Version + staged.Digest + staged.StagedAt
	} else {
		d.stagedID = version
	}
}

func (d *deployments) fromBootc(status *bootc.Status) {
	if status == nil {
		return
	}
	booted := status.Status.Booted
	d.imageRef = booted.ImageRef()
	if d.imageRef == "" && status.Spec.Image != nil {
		d.imageRef = status.Spec.Image.Image
	}
	d.builtAt = booted.Timestamp()
	d.booted = bootcDeployment(booted)
	d.staged = bootcDeployment(status.Status.Staged)
	d.rollback = bootcDeployment(status.Status.Rollback)
	d.stagedID = status.Status.Staged.Digest()
}

func bootcDeployment(dep *bootc.Deployment) Deployment {
	if dep == nil {
		return Deployment{}
	}
	return Deployment{Present: true, Version: dep.Version(), Digest: pageview.ShortDigest(dep.Digest()), Pinned: dep.Pinned}
}

func (d *deployments) fromSysupdate(status sysupdate.Status) {
	d.outcome, d.stagedVersion, d.checkedAt = status.Presentation()
	if status.Check != nil {
		d.imageRef, d.runningVersion = status.Check.Image, status.Check.RunningVersion
	}
	d.booted = Deployment{Present: true}
	if status.IsStaged() {
		d.staged = Deployment{Present: true, Version: d.stagedVersion}
		if st := status.Staged; st != nil {
			if d.imageRef == "" {
				d.imageRef = st.Image
			}
			d.staged.Digest = pageview.ShortDigest(st.Digest)
			d.stagedID = st.Version + st.Digest + st.StagedAt
		} else {
			d.stagedID = d.stagedVersion
		}
	}
	if d.rollbackVersion != "" {
		d.rollback = Deployment{Present: true, Version: d.rollbackVersion}
	}
}

func (s *System) applyDeployments(d deployments) {
	if d.err != "" && s.depsLoaded && s.deps.err == "" && s.deps.kind == d.kind {
		// A failed read after a good one keeps what is known.
		kept := s.deps
		kept.refreshErr = d.err
		d = kept
	}
	s.deps, s.depsLoaded, s.checkedAt = d, true, s.now()
	if d.staged.Present {
		s.runCurrent = false
	}
	s.compose()
}

// osReleaseValue is the value of the os-release field ParseOSRelease titled
// title ("Name" for NAME, "Image Version" for IMAGE_VERSION).
func osReleaseValue(entries []pageview.OSReleaseEntry, titles ...string) string {
	for _, title := range titles {
		for _, e := range entries {
			if e.Title == title && e.Value != "" {
				return e.Value
			}
		}
	}
	return ""
}

// compose builds SystemInfo from the last facts and deployments.
func (s *System) compose() {
	f, d := s.facts, s.deps
	info := SystemInfo{
		Loaded:   s.loaded,
		Kind:     d.kind,
		OSName:   osReleaseValue(f.osRelease, "Name", "Pretty Name"),
		ImageRef: d.imageRef,
		Channel:  pageview.ImageChannel(d.imageRef),
		Booted:   d.booted,
		Staged:   d.staged,
		Rollback: d.rollback,
		// the snosi state files say nothing of the rollback slot
		RollbackUnknown: d.rollbackUnknown,
		Hostname:        f.hostname,
		Kernel:          f.kernel,
		Model:           f.model,
	}
	imageVersion := osReleaseValue(f.osRelease, "Image Version")
	info.OSVersion = imageVersion
	if info.OSVersion == "" {
		info.OSVersion = osReleaseValue(f.osRelease, "Version", "Version Id")
	}
	if info.Booted.Present && info.Booted.Version == "" {
		info.Booted.Version = imageVersion
		if info.Booted.Version == "" {
			info.Booted.Version = d.runningVersion
		}
	}
	switch f.secureBoot {
	case sysinfo.SecureBootOn:
		info.SecureBoot = "on"
	case sysinfo.SecureBootOff:
		info.SecureBoot = "off"
	}
	s.info = info
}

// sourceAvailable reports whether the image source applies here: the last
// load found a host of an enabled updates group's kind with its stage
// script, as the old Updates page's gates did.
func (s *System) sourceAvailable() bool {
	switch s.deps.kind {
	case KindBootc:
		return s.deps.stageable && s.env.Enabled("updates_page", "bootc_updates_group")
	case KindNativeAB:
		return s.deps.stageable && s.env.Enabled("updates_page", "sysupdate_updates_group")
	}
	return false
}

// imageOffered reports whether the source offers its item: an image can
// be staged and nothing says the host is current. The stage script is the
// only check bootc has (it needs root), so on bootc the item stands for
// "check for and stage the newest image" until a run this session found
// nothing newer. Native A/B hosts have the stager's own record of its
// last check.
func (s *System) imageOffered() bool {
	d := s.deps
	if d.err != "" || d.staged.Present {
		return false
	}
	switch d.kind {
	case KindBootc:
		// The snosi state (when bootc status is root-only) may say the
		// host is current; bootc's own status never does.
		return d.outcome != string(sysupdate.OutcomeCurrent) && !s.runCurrent
	case KindNativeAB:
		switch d.outcome {
		case string(sysupdate.OutcomeCurrent):
			return false
		case "":
			return !s.runCurrent
		}
		return true
	}
	return false
}

// imageNote is the line under the image source's title: the old Updates
// page's expander subtitle.
func (s *System) imageNote() string {
	d := s.deps
	if d.staged.Present {
		return pageview.StagedNote(d.staged.Version)
	}
	switch d.kind {
	case KindBootc:
		if s.runCurrent {
			return s.lastRun
		}
		return pageview.BootcUpdateSubtitle(false, "")
	case KindNativeAB:
		if d.outcome == "" && s.runCurrent {
			return s.lastRun
		}
		return pageview.SysupdateUpdateSubtitle(d.outcome, d.stagedVersion, d.checkedAt)
	}
	return ""
}

// stageResult is what a stage job's goroutine hands its Done.
type stageResult struct {
	// ran is false when the queue dropped the job before it ran: then
	// nothing happened and deps is empty.
	ran  bool
	deps deployments
	last string
}

// stageJob runs the fixed stage script of the host's kind through pkexec
// (bootc.StageUpdate or sysupdate.StageUpdate; never another path), shows
// its lines as the job's status, then re-reads the status so the source
// and the hero show what is staged now.
func (s *System) stageJob() *frostui.Job {
	kind := s.deps.kind
	before := s.deps.stagedID
	name := s.info.OSName
	if name == "" {
		name = "the system"
	}
	stage, dryRun := s.bootcStage, s.bootcDryRun
	if kind == KindNativeAB {
		stage, dryRun = s.sysupdateStage, s.sysupdateDryRun
	}
	res := &stageResult{}
	// The stage script runs as root through pkexec: a cancel could signal
	// only the unprivileged parent while the stage goes on, so the job is
	// Unstoppable and finishes first.
	job := &frostui.Job{Source: string(SourceImage), Key: imageItemKey, Title: "Staging the " + name + " image", Unstoppable: true}
	job.Run = func(ctx context.Context, p *frostui.Progress) error {
		p.Set(-1, "Checking for updates…")
		ctx, cancel := context.WithTimeout(ctx, bootc.DefaultTimeout)
		defer cancel()
		events := make(chan bootc.ProgressEvent)
		errc := make(chan error, 1)
		go func() { errc <- stage(ctx, events) }()
		for ev := range events {
			switch ev.Type {
			case bootc.EventMessage:
				res.last = ev.Message
				p.Set(-1, ev.Message)
			case bootc.EventComplete:
				p.Set(1, "Complete")
			}
		}
		err := <-errc
		// Re-read in both modes: it is a plain read, so the source shows
		// what is really staged rather than a guess from the output.
		readCtx, readCancel := context.WithTimeout(context.Background(), bootc.DefaultTimeout)
		defer readCancel()
		res.deps = s.readDeployments(readCtx, kind == KindBootc, kind == KindNativeAB)
		res.ran = true
		return err
	}
	job.Done = func(err error) { s.finishStage(kind, name, before, *res, err, dryRun()) }
	return job
}

// finishStage applies a stage run on the UI thread: the re-read status,
// the toast (dry-run aware, as actionmsg says) and, after a live run that
// staged a new image, the activity entry.
func (s *System) finishStage(kind, name, before string, res stageResult, err error, dryRun bool) {
	if !res.ran {
		// Dropped from the queue before it started: nothing ran.
		return
	}
	// The run's read is newer than any status read still in flight.
	s.refreshGate.Begin()
	s.checking = false
	s.applyDeployments(res.deps)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			s.env.Toast("System update canceled")
			return
		}
		s.stageErr = fmt.Sprintf("Update failed: %v", err)
		s.env.ToastError(s.stageErr)
		return
	}
	s.stageErr = ""
	staged := res.deps.staged.Present
	if !dryRun && !staged && res.deps.err == "" {
		s.runCurrent = true
		if kind == KindNativeAB {
			s.lastRun = pageview.SysupdateStageResultSubtitle(false, "", res.last)
		} else {
			s.lastRun = pageview.BootcStageResultSubtitle(false, "", res.last)
		}
	}
	message := actionmsg.BootcStage(dryRun, staged)
	if kind == KindNativeAB {
		message = actionmsg.SysupdateStage(dryRun, staged)
	}
	if !dryRun && staged {
		s.env.ToastSuccess(message)
	} else {
		s.env.Toast(message)
	}
	if !dryRun && staged && res.deps.stagedID != before {
		s.env.Record(string(SourceImage), strings.TrimSpace("Staged "+name+" "+res.deps.staged.Version), res.deps.imageRef)
	}
}

// imageSource is the "System image" update source: the image a bootc or
// native A/B host boots next. It is a view on System.
type imageSource struct{ s *System }

func (src *imageSource) ID() SourceID  { return SourceImage }
func (src *imageSource) Title() string { return "System image" }
func (src *imageSource) Icon() string  { return "drive-harddisk-system" }
func (src *imageSource) Blurb() string {
	return "Downloads and stages the newest system image; it applies when the computer restarts"
}

func (src *imageSource) Status() SourceStatus {
	s := src.s
	st := SourceStatus{Available: s.sourceAvailable(), Checking: s.checking, CheckedAt: s.checkedAt, Note: s.imageNote()}
	switch {
	case s.stageErr != "":
		st.Err = s.stageErr
	case s.deps.err != "":
		st.Err = s.deps.err
	case s.deps.refreshErr != "":
		st.Err = s.deps.refreshErr
	}
	return st
}

func (src *imageSource) Items() []UpdateItem {
	s := src.s
	if !s.sourceAvailable() || !s.imageOffered() {
		return nil
	}
	title := s.info.OSName
	if title == "" {
		title = "System image"
	}
	detail := s.deps.imageRef
	if detail == "" {
		detail = s.deps.kind
	}
	return []UpdateItem{{Key: imageItemKey, Title: title, Detail: detail, From: s.info.Booted.Version, Action: "Stage"}}
}

// Refresh re-reads the deployments. Only the stage script itself can ask
// the registry for a newer image, and it needs root, so a check here never
// prompts.
func (src *imageSource) Refresh() {
	if src.s.checking {
		return
	}
	src.s.loadDeployments()
}

func (src *imageSource) Jobs(keys ...string) []*frostui.Job {
	s := src.s
	if !s.sourceAvailable() || s.env.Jobs.Pending(string(SourceImage), imageItemKey) {
		return nil
	}
	if len(keys) == 0 {
		if !s.imageOffered() {
			return nil
		}
		return []*frostui.Job{s.stageJob()}
	}
	for _, k := range keys {
		if k == imageItemKey {
			return []*frostui.Job{s.stageJob()}
		}
	}
	return nil
}

// Update stages the image. The old Updates page ran the stage script
// without asking first; PolicyKit asks for the administrator itself.
func (src *imageSource) Update(key string) {
	if src.s.env.Jobs.Pending(string(SourceImage), imageItemKey) {
		return
	}
	if jobs := src.Jobs(key); len(jobs) > 0 {
		src.s.env.Jobs.Enqueue(jobs...)
	}
}

// Extra draws the native A/B rollback line, and the old page's
// always-there stage button while the source offers no item (an image is
// staged, or the last check found nothing newer).
func (src *imageSource) Extra(c *ui.Context) {
	s := src.s
	if !s.sourceAvailable() {
		return
	}
	if s.deps.kind == KindNativeAB {
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
			frostui.Icon(c, "document-revert").TextColor(c.Theme().TextMuted)
			frostui.Muted(c, "Previous version").Width(120)
			frostui.Body(c, pageview.SysupdateRollbackSubtitle(s.deps.rollbackVersion)).Wrap().Grow(1)
		})
	}
	if s.imageOffered() || s.env.Jobs.Pending(string(SourceImage), imageItemKey) {
		return
	}
	ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
		frostui.Muted(c, "The update check runs as administrator and stages a newer image when there is one.").Wrap().Grow(1)
		if frostui.LabeledButton(c, "view-refresh", "Check and stage", false).Clicked() {
			src.Update(imageItemKey)
		}
	})
}
