package views

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/fwupd"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/frostui"
)

// Firmware holds fwupd's devices with updates and the firmware update
// source: updates_page/firmware_updates_group, on by default (ADR-0013) and
// shown only where the fwupdmgr client is installed.
//
// Every call is unprivileged: fwupdmgr asks the fwupd daemon, which
// authorizes an install with its own PolicyKit actions (ADR-0012). Nothing
// here goes through pkexec.
//
// One device installs at a time, after a confirmation, and never as part of
// "Update everything": Jobs with no keys returns nothing. A device fwupd
// will not flash now (a closed lid, a host on battery, a recorded error)
// stays listed as a held item with its reason.
type Firmware struct {
	env *Env

	// The backend, replaceable in tests.
	present func() bool                                     // fwupd.IsInstalledCached
	list    func() ([]fwupd.Update, error)                  // fwupd.ListUpdates
	install func(fwupd.Update) (fwupd.InstallReport, error) // fwupd.InstallAndVerify
	dryRun  func() bool                                     // fwupd.IsDryRun

	// probed is true once the fwupdmgr lookup answered; found is its
	// answer. The source is unavailable until fwupdmgr is found.
	probed, found bool
	// loads counts the list queries that run; refresh makes sure only the
	// newest one publishes, so a slow query cannot replace a newer list.
	loads   int
	refresh actionstate.RefreshGate
	// installGate holds from an Install click until that install ends, so
	// only one confirmation and one install are ever under way.
	installGate actionstate.Gate

	// updates is the last list fwupd gave: the single source of the rows,
	// replaced whole by a load or a verified install, never edited by
	// device.
	updates   []fwupd.Update
	listed    bool
	err       string
	checkedAt time.Time
	// completed holds the label a finished install leaves on its device
	// ("Installed", "Restart required") when the list could not be read
	// afterwards, so the old row stays and must not offer the install again.
	completed map[string]string

	source *firmwareSource
}

// NewFirmware returns the firmware state.
func NewFirmware(env *Env) *Firmware {
	f := &Firmware{
		env:       env,
		present:   fwupd.IsInstalledCached,
		list:      fwupd.ListUpdates,
		install:   fwupd.InstallAndVerify,
		dryRun:    fwupd.IsDryRun,
		completed: map[string]string{},
	}
	f.source = &firmwareSource{f: f}
	return f
}

func (f *Firmware) enabled() bool {
	return f.env.Enabled("updates_page", "firmware_updates_group")
}

// Start begins the first load.
func (f *Firmware) Start() { f.load() }

// Source is the firmware update source, nil when its group is disabled.
// It is unavailable until fwupdmgr is found.
func (f *Firmware) Source() UpdateSource {
	if !f.enabled() {
		return nil
	}
	return f.source
}

// load starts a list query unless one runs, the group is off, or fwupdmgr
// is missing.
func (f *Firmware) load() {
	if !f.enabled() || (f.probed && !f.found) || f.loads > 0 {
		return
	}
	f.loads++
	generation := f.refresh.Begin()
	present, list, post := f.present, f.list, f.env.Post
	go func() {
		if !present() {
			post(func() {
				f.loads--
				f.probed, f.found = true, false
			})
			return
		}
		// Show the source, checking, while the list loads.
		post(func() { f.probed, f.found = true, true })
		updates, err := list()
		if err != nil {
			log.Printf("Error loading firmware updates: %v", err)
		}
		post(func() {
			f.loads--
			f.applyLoad(generation, updates, err)
		})
	}()
}

// applyLoad publishes a list query's result unless a newer query or a
// verified install made it stale. A failed query keeps the rows and says
// why: it says nothing about the devices and must never claim the firmware
// is up to date.
func (f *Firmware) applyLoad(generation uint64, updates []fwupd.Update, err error) {
	if !f.refresh.IsCurrent(generation) {
		return
	}
	if err != nil {
		f.checkedAt = f.env.Now()
		f.err = pageview.FirmwareUpdatesSubtitle(0, err, len(f.updates)).Subtitle
		return
	}
	f.publish(updates)
}

// publish replaces the list with an authoritative one, from a load or a
// verified install. It is the only writer of the rows.
func (f *Firmware) publish(updates []fwupd.Update) {
	f.updates = updates
	f.listed = true
	f.err = ""
	f.checkedAt = f.env.Now()
	f.completed = map[string]string{}
}

func (f *Firmware) find(key string) (fwupd.Update, bool) {
	for _, u := range f.updates {
		if u.DeviceID == key {
			return u, true
		}
	}
	return fwupd.Update{}, false
}

// firmwareName is the device's name, "Unknown device" when fwupd gives none.
func firmwareName(u fwupd.Update) string {
	return pageview.FirmwareUpdate(u.DeviceName, "", "", "").Title
}

// items are the devices with an update: installable ones need a
// confirmation; blocked ones are held with fwupd's reason in their detail.
func (f *Firmware) items() []UpdateItem {
	items := make([]UpdateItem, 0, len(f.updates))
	for _, u := range f.updates {
		row := pageview.FirmwareUpdate(u.DeviceName, u.Vendor, "", "")
		it := UpdateItem{
			Key: u.DeviceID, Title: row.Title, Detail: row.Subtitle,
			From: u.CurrentVersion, To: u.NewVersion,
			Confirm: true, Action: "Install…",
		}
		if !u.Installable {
			// No install control, so the reason must be visible.
			it.Detail = pageview.FirmwareBlockedUpdate(u.DeviceName, u.Vendor, "", "", u.Reason).Subtitle
			it.Held, it.HeldWhy = true, u.Reason
		}
		if label, ok := f.completed[u.DeviceID]; ok {
			// The install finished but the list could not be read: the old
			// row records it and does not offer the install again.
			it.Held, it.HeldWhy, it.Action = true, label, label
		}
		items = append(items, it)
	}
	return items
}

// jobs returns install jobs for the installable devices with keys. The
// caller has asked for each device first.
func (f *Firmware) jobs(keys []string) []*frostui.Job {
	var out []*frostui.Job
	for _, key := range keys {
		u, ok := f.find(key)
		if !ok || !u.Installable || f.completed[key] != "" || f.env.Jobs.Pending(string(SourceFirmware), key) {
			continue
		}
		out = append(out, f.installJob(u, false))
	}
	return out
}

// ask confirms one device's install, then queues it. The gate stays held
// from the click until the install ends; a click that finds it held only
// says so.
func (f *Firmware) ask(key string) {
	u, ok := f.find(key)
	if !ok || !u.Installable || f.completed[key] != "" || f.env.Jobs.Pending(string(SourceFirmware), key) {
		return
	}
	if !f.installGate.TryStart() {
		f.env.Toast("Another firmware update is installing")
		return
	}
	heading, body := pageview.FirmwareInstallConfirmation(u.DeviceName, u.NewVersion, u.NeedsReboot, u.NeedsShutdown)
	details := []frostui.KeyValue{{Key: "Device", Value: firmwareName(u)}}
	if u.Vendor != "" {
		details = append(details, frostui.KeyValue{Key: "Vendor", Value: u.Vendor})
	}
	if v := firmwareVersions(u); v != "" {
		details = append(details, frostui.KeyValue{Key: "Version", Value: v, Mono: true})
	}
	f.env.Dialogs.Ask(frostui.Confirm{
		Title:       heading,
		Body:        body,
		Details:     details,
		Note:        "fwupd may ask for administrator authentication through its own PolicyKit action.",
		Action:      "Install",
		Destructive: true,
		OnConfirm:   func() { f.env.Jobs.Enqueue(f.installJob(u, true)) },
		// Canceled: the device must offer its install again.
		OnCancel: func() { f.installGate.Reset() },
	})
}

func firmwareVersions(u fwupd.Update) string {
	switch {
	case u.CurrentVersion != "" && u.NewVersion != "":
		return u.CurrentVersion + " → " + u.NewVersion
	case u.NewVersion != "":
		return u.NewVersion
	}
	return u.CurrentVersion
}

// firmwareOutcome is what an install job found, handed from its goroutine
// to Done.
type firmwareOutcome struct {
	report   fwupd.InstallReport
	err      error
	dryRun   bool
	decision actionmsg.FirmwareInstallDecision
}

// installJob installs one device's firmware, then asks fwupd what happened.
// fwupdmgr can exit 0 without a flash, so the outcome comes from
// actionmsg.FirmwareInstall and the device's own report, never from the
// exit status. gated says the job holds installGate.
func (f *Firmware) installJob(u fwupd.Update, gated bool) *frostui.Job {
	name := firmwareName(u)
	install, dryRun := f.install, f.dryRun
	var out *firmwareOutcome
	return &frostui.Job{
		Title:  "Installing firmware for " + name,
		Source: string(SourceFirmware),
		Key:    u.DeviceID,
		// A flash must not be interrupted, and fwupd runs it as root.
		Unstoppable: true,
		Run: func(_ context.Context, p *frostui.Progress) error {
			p.Set(-1, "Installing firmware for "+name)
			p.Log(frostui.LevelInfo, "Keep the computer on power until the firmware update ends.")
			report, err := install(u)
			dry := dryRun()
			d := actionmsg.FirmwareInstall(dry, err, report, name, u.NeedsReboot, u.NeedsShutdown)
			out = &firmwareOutcome{report: report, err: err, dryRun: dry, decision: d}
			switch {
			case err != nil:
				return err
			case d.IsError:
				return errors.New(d.Toast)
			}
			p.Log(frostui.LevelInfo, "%s", d.Toast)
			return nil
		},
		Done: func(error) { f.finishInstall(u, out, gated) },
	}
}

// finishInstall applies an install's decision on the UI thread. A verified
// list is published whole (a device that still has an update stays); no
// row is removed by identity. The gate resets last, in every case.
func (f *Firmware) finishInstall(u fwupd.Update, out *firmwareOutcome, gated bool) {
	if gated {
		defer f.installGate.Reset()
	}
	if out == nil {
		// Dropped from the queue before it ran: nothing happened.
		return
	}
	d := out.decision
	if d.PublishList {
		// A newer generation makes a load still in flight stale, so it
		// cannot replace this verified list.
		f.refresh.Begin()
		f.publish(out.report.Updates)
	} else if out.report.ListErr != nil {
		// The rows cannot be rebuilt: the old ones stay, and the source
		// says why the list is out of date.
		f.err = pageview.FirmwareUpdatesSubtitle(0, out.report.ListErr, len(f.updates)).Subtitle
	}
	if d.CompletedLabel != "" {
		f.completed[u.DeviceID] = d.CompletedLabel
	}
	if d.RestoreControl {
		delete(f.completed, u.DeviceID)
	}
	switch {
	case d.IsError:
		f.env.ToastError(d.Toast)
	case out.dryRun:
		f.env.Toast(d.Toast)
	default:
		f.env.ToastSuccess(d.Toast)
	}
	if out.err == nil && !out.dryRun {
		name := firmwareName(u)
		switch out.report.Result {
		case fwupd.InstallFlashed:
			f.env.Record(string(SourceFirmware), "Installed firmware for "+name, firmwareVersions(u))
		case fwupd.InstallStaged:
			f.env.Record(string(SourceFirmware), "Staged firmware for "+name, firmwareVersions(u)+" · restart to finish")
		}
	}
}

// firmwareSource is the firmware UpdateSource.
type firmwareSource struct{ f *Firmware }

func (s *firmwareSource) ID() SourceID  { return SourceFirmware }
func (s *firmwareSource) Title() string { return "Firmware" }
func (s *firmwareSource) Icon() string  { return "application-x-firmware" }
func (s *firmwareSource) Blurb() string {
	return "Device firmware from the Linux Vendor Firmware Service, installed through fwupd one device at a time."
}

func (s *firmwareSource) Status() SourceStatus {
	f := s.f
	st := SourceStatus{
		Available: f.enabled() && f.found,
		Checking:  f.loads > 0,
		Err:       f.err,
		CheckedAt: f.checkedAt,
	}
	if f.listed && f.err == "" {
		st.Note = pageview.FirmwareUpdatesSubtitle(len(f.updates), nil, 0).Subtitle
	}
	return st
}

func (s *firmwareSource) Items() []UpdateItem { return s.f.items() }

func (s *firmwareSource) Refresh() { s.f.load() }

// Jobs returns nothing without keys: firmware is never part of "Update
// everything".
func (s *firmwareSource) Jobs(keys ...string) []*frostui.Job {
	if len(keys) == 0 {
		return nil
	}
	return s.f.jobs(keys)
}

// Update asks to install one device's firmware, then queues it.
func (s *firmwareSource) Update(key string) { s.f.ask(key) }

func (s *firmwareSource) Extra(*ui.Context) {}
