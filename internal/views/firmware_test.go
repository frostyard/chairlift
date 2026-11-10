package views

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/fwupd"
)

var (
	fwDbx = fwupd.Update{
		DeviceID: "362301da643102b9f38477387e2193e57abaa590", DeviceName: "UEFI dbx", Vendor: "Linux Foundation",
		CurrentVersion: "371", NewVersion: "377", NeedsReboot: true, Installable: true,
	}
	fwDock = fwupd.Update{
		DeviceID: "8e9cb71aeca70d2faedb5b8aaa263f6175086b2e", DeviceName: "Thunderbolt 4 Dock", Vendor: "Lenovo",
		CurrentVersion: "1.0.5", NewVersion: "1.0.9", Installable: true,
	}
	fwEC = fwupd.Update{
		DeviceID: "4bde70ba4e39b28f9eab1628f9dd6e6244c03027", DeviceName: "Embedded Controller", Vendor: "Lenovo",
		CurrentVersion: "1.32", NewVersion: "1.35", Reason: "Needs attention: lid-is-closed",
	}
)

// fwFake stands in for fwupd: no test runs fwupdmgr.
type fwFake struct {
	mu        sync.Mutex
	present   bool
	updates   []fwupd.Update
	listErr   error
	listGate  chan struct{} // when set, the next list waits for it
	listCalls int
	installed []string
	report    fwupd.InstallReport
	err       error
	dry       bool
}

func (f *fwFake) wire(fw *Firmware) {
	fw.present = func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.present
	}
	fw.list = func() ([]fwupd.Update, error) {
		f.mu.Lock()
		f.listCalls++
		gate := f.listGate
		f.listGate = nil
		updates, err := slices.Clone(f.updates), f.listErr
		f.mu.Unlock()
		if gate != nil {
			<-gate
		}
		return updates, err
	}
	fw.install = func(u fwupd.Update) (fwupd.InstallReport, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.installed = append(f.installed, u.DeviceID)
		return f.report, f.err
	}
	fw.dryRun = func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.dry
	}
}

func (f *fwFake) installs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.installed)
}

func (f *fwFake) listCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

func (f *fwFake) setReport(r fwupd.InstallReport) {
	f.mu.Lock()
	f.report = r
	f.mu.Unlock()
}

func (f *fwFake) setList(updates []fwupd.Update, err error) {
	f.mu.Lock()
	f.updates, f.listErr = updates, err
	f.mu.Unlock()
}

// fwLoaded returns a Firmware over a fake fwupd that lists updates, after
// its first load.
func fwLoaded(t *testing.T, env *Env, q *updPost, updates ...fwupd.Update) (*Firmware, *fwFake) {
	t.Helper()
	ff := &fwFake{present: true, updates: updates}
	fw := NewFirmware(env)
	ff.wire(fw)
	fw.Start()
	q.until(t, func() bool { return fw.loads == 0 && fw.listed })
	return fw, ff
}

// fwAsk clicks a device's install and answers its confirmation.
func fwAsk(t *testing.T, env *Env, fw *Firmware, key, answer string) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) { env.Dialogs.View(c) }, 900, 700)
	fw.Source().Update(key)
	tt.Frame()
	if !env.Dialogs.Showing() {
		t.Fatalf("no confirmation for %s", key)
	}
	if answer != "" {
		if err := tt.Click(answer); err != nil {
			t.Fatal(err)
		}
	}
	return tt
}

func TestFirmwareListsDevicesAndHoldsBlockedOnes(t *testing.T) {
	env, q := updEnv(t, false, "firmware_updates_group")
	fw, _ := fwLoaded(t, env, q, fwDock, fwEC)
	src := fw.Source()
	st := src.Status()
	if !st.Available || st.Checking || st.Err != "" || st.CheckedAt.IsZero() || st.Note != "2 firmware updates available" {
		t.Errorf("status = %+v", st)
	}
	items := src.Items()
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	dock, ec := items[0], items[1]
	if dock.Key != fwDock.DeviceID || dock.Title != "Thunderbolt 4 Dock" || dock.Detail != "Lenovo" ||
		dock.From != "1.0.5" || dock.To != "1.0.9" || !dock.Confirm || dock.Held || dock.Action != "Install…" {
		t.Errorf("installable item = %+v", dock)
	}
	if !ec.Held || ec.HeldWhy != fwEC.Reason || ec.Detail != "Lenovo · Needs attention: lid-is-closed" || !ec.Confirm {
		t.Errorf("blocked item = %+v", ec)
	}
	if src.ID() != SourceFirmware || src.Title() != "Firmware" {
		t.Errorf("source = %s %q", src.ID(), src.Title())
	}
}

func TestFirmwareNeedsItsGroupAndFwupdmgr(t *testing.T) {
	env, q := updEnv(t, false)
	env.Config.UpdatesPage["firmware_updates_group"] = config.GroupConfig{Enabled: false}
	fw := NewFirmware(env)
	ff := &fwFake{present: true}
	ff.wire(fw)
	fw.Start()
	q.flush()
	if fw.Source() != nil || ff.listCount() != 0 {
		t.Errorf("a disabled group gave a source or loaded (%d lists)", ff.listCount())
	}

	env, q = updEnv(t, false, "firmware_updates_group")
	fw = NewFirmware(env)
	ff = &fwFake{present: false}
	ff.wire(fw)
	fw.Start()
	q.until(t, func() bool { return fw.probed })
	if fw.Source().Status().Available || ff.listCount() != 0 {
		t.Error("the source is available without fwupdmgr")
	}
	fw.Source().Refresh()
	q.flush()
	if ff.listCount() != 0 || fw.loads != 0 {
		t.Error("a refresh without fwupdmgr loads")
	}
}

func TestFirmwareFailedCheckKeepsTheRows(t *testing.T) {
	env, q := updEnv(t, false, "firmware_updates_group")
	fw, ff := fwLoaded(t, env, q, fwDock)
	ff.setList(nil, errors.New("daemon not running"))
	fw.Source().Refresh()
	q.until(t, func() bool { return fw.loads == 0 })
	st := fw.Source().Status()
	if st.Err != "Error checking firmware updates: daemon not running" || st.Note != "" {
		t.Errorf("status = %+v", st)
	}
	if items := fw.Source().Items(); len(items) != 1 || items[0].Key != fwDock.DeviceID {
		t.Errorf("a failed check dropped the rows: %+v", items)
	}
	ff.setList(nil, nil)
	fw.Source().Refresh()
	q.until(t, func() bool { return fw.loads == 0 })
	if st := fw.Source().Status(); st.Err != "" || st.Note != "Firmware is up to date" || len(fw.Source().Items()) != 0 {
		t.Errorf("after an empty list: %+v %+v", st, fw.Source().Items())
	}
}

func TestFirmwareStaleCheckCannotReplaceAVerifiedList(t *testing.T) {
	env, q := updEnv(t, false, "firmware_updates_group")
	fw, ff := fwLoaded(t, env, q, fwDock, fwDbx)
	// A check starts and hangs with the old list...
	gate := make(chan struct{})
	ff.mu.Lock()
	ff.listGate = gate
	ff.mu.Unlock()
	fw.Source().Refresh()
	if !fw.Source().Status().Checking {
		t.Error("the source does not say it checks")
	}
	fw.Source().Refresh() // one check at a time
	// ...while an install publishes the verified list without the dock.
	ff.setReport(fwupd.InstallReport{Result: fwupd.InstallFlashed, Updates: []fwupd.Update{fwDbx}})
	fwAsk(t, env, fw, fwDock.DeviceID, "Install")
	q.until(t, func() bool { return !env.Jobs.Busy() })
	close(gate)
	q.until(t, func() bool { return fw.loads == 0 })
	if items := fw.Source().Items(); len(items) != 1 || items[0].Key != fwDbx.DeviceID {
		t.Errorf("the stale check replaced the verified list: %+v", items)
	}
	if ff.listCount() != 2 {
		t.Errorf("%d checks ran, want 2", ff.listCount())
	}
}

func TestFirmwareInstallAsksThenInstalls(t *testing.T) {
	env, q := updEnv(t, false, "firmware_updates_group")
	fw, ff := fwLoaded(t, env, q, fwDbx, fwDock)
	ff.setReport(fwupd.InstallReport{Result: fwupd.InstallFlashed, Updates: []fwupd.Update{fwDock}})
	tt := fwAsk(t, env, fw, fwDbx.DeviceID, "")
	for _, want := range []string{"Install firmware for UEFI dbx?", "371 → 377", "Install", "Cancel"} {
		if !tt.HasText(want) {
			t.Errorf("confirmation lacks %q: %q", want, tt.Texts())
		}
	}
	body := strings.Join(tt.Texts(), " ")
	for _, want := range []string{"Keep the power connected", "must restart", "ChairLift will not restart the computer"} {
		if !strings.Contains(body, want) {
			t.Errorf("confirmation lacks %q", want)
		}
	}
	if env.Jobs.Busy() || len(ff.installs()) != 0 {
		t.Fatal("the install started before the confirmation")
	}
	if err := tt.Click("Install"); err != nil {
		t.Fatal(err)
	}
	q.until(t, func() bool { return !env.Jobs.Busy() })
	if got := ff.installs(); !slices.Equal(got, []string{fwDbx.DeviceID}) {
		t.Errorf("installs = %v", got)
	}
	toasts := env.TakeToasts()
	if len(toasts) != 1 || toasts[0].Title != "Firmware for UEFI dbx installed. Restart to finish." || toasts[0].Type != "success" {
		t.Errorf("toasts = %+v", toasts)
	}
	if items := fw.Source().Items(); len(items) != 1 || items[0].Key != fwDock.DeviceID {
		t.Errorf("the verified list was not published: %+v", items)
	}
	rec := env.Activity.Recent(5)
	if len(rec) != 1 || rec[0].Source != "firmware" || rec[0].Title != "Installed firmware for UEFI dbx" || rec[0].Detail != "371 → 377" {
		t.Errorf("activity = %+v", rec)
	}
	// The gate is free again.
	fwAsk(t, env, fw, fwDock.DeviceID, "Cancel")
}

func TestFirmwareOneInstallAtATime(t *testing.T) {
	env, q := updEnv(t, false, "firmware_updates_group")
	fw, ff := fwLoaded(t, env, q, fwDbx, fwDock)
	tt := fwAsk(t, env, fw, fwDbx.DeviceID, "")
	fw.Source().Update(fwDock.DeviceID)
	if toasts := env.TakeToasts(); len(toasts) != 1 || toasts[0].Title != "Another firmware update is installing" {
		t.Errorf("toasts = %+v", toasts)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if env.Dialogs.Showing() || len(ff.installs()) != 0 {
		t.Error("a second confirmation was queued, or Cancel installed")
	}
	// Cancel released the gate: the other device can ask now.
	fwAsk(t, env, fw, fwDock.DeviceID, "Cancel")
	if len(env.TakeToasts()) != 0 {
		t.Error("the gate stayed held after Cancel")
	}
	// Blocked devices never ask.
	fw2, _ := fwLoaded(t, env, q, fwEC)
	fw2.Source().Update(fwEC.DeviceID)
	if env.Dialogs.Showing() {
		t.Error("a blocked device asked to install")
	}
}

func TestFirmwareInstallOutcomes(t *testing.T) {
	listErr := errors.New("daemon restarted")
	cases := []struct {
		name      string
		report    fwupd.InstallReport
		err       error
		toast     string
		typ       string
		activity  string
		keys      []string // the rows afterwards
		heldLabel string   // the dbx row's label when it stays held
		srcErr    string
	}{
		{
			name: "install failed", err: errors.New("authorization denied"),
			toast: "Firmware install failed: authorization denied", typ: "error",
			keys: []string{fwDbx.DeviceID, fwDock.DeviceID},
		},
		{
			name: "not applied", report: fwupd.InstallReport{Result: fwupd.InstallNotApplied, Reason: "fwupd did not install the update", Updates: []fwupd.Update{fwDbx, fwDock}},
			toast: "Firmware for UEFI dbx was not installed: fwupd did not install the update", typ: "error",
			keys: []string{fwDbx.DeviceID, fwDock.DeviceID},
		},
		{
			name: "unverified", report: fwupd.InstallReport{Result: fwupd.InstallUnverified, Reason: "fwupd does not show the device now; it can be restarting", Updates: []fwupd.Update{fwDock}},
			toast: "fwupdmgr finished for UEFI dbx, but ChairLift could not check the result: fwupd does not show the device now; it can be restarting", typ: "error",
			keys: []string{fwDock.DeviceID},
		},
		{
			name: "staged", report: fwupd.InstallReport{Result: fwupd.InstallStaged, Updates: []fwupd.Update{fwDock}},
			toast: "Firmware for UEFI dbx is ready. Restart the computer to finish.", typ: "success",
			activity: "Staged firmware for UEFI dbx", keys: []string{fwDock.DeviceID},
		},
		{
			name: "flashed, list unreadable", report: fwupd.InstallReport{Result: fwupd.InstallFlashed, ListErr: listErr},
			toast: "Firmware for UEFI dbx installed. Restart to finish.", typ: "success",
			activity: "Installed firmware for UEFI dbx", keys: []string{fwDbx.DeviceID, fwDock.DeviceID},
			heldLabel: "Installed", srcErr: "Error checking firmware updates: daemon restarted",
		},
		{
			name: "not applied, list unreadable", report: fwupd.InstallReport{Result: fwupd.InstallNotApplied, Reason: "lid closed", ListErr: listErr},
			toast: "Firmware for UEFI dbx was not installed: lid closed", typ: "error",
			keys: []string{fwDbx.DeviceID, fwDock.DeviceID}, srcErr: "Error checking firmware updates: daemon restarted",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, q := updEnv(t, false, "firmware_updates_group")
			fw, ff := fwLoaded(t, env, q, fwDbx, fwDock)
			ff.mu.Lock()
			ff.report, ff.err = tc.report, tc.err
			ff.mu.Unlock()
			fwAsk(t, env, fw, fwDbx.DeviceID, "Install")
			q.until(t, func() bool { return !env.Jobs.Busy() })
			toasts := env.TakeToasts()
			if len(toasts) != 1 || toasts[0].Title != tc.toast || toasts[0].Type != tc.typ {
				t.Errorf("toasts = %+v\nwant %q (%s)", toasts, tc.toast, tc.typ)
			}
			var keys []string
			var dbx UpdateItem
			for _, it := range fw.Source().Items() {
				keys = append(keys, it.Key)
				if it.Key == fwDbx.DeviceID {
					dbx = it
				}
			}
			if !slices.Equal(keys, tc.keys) {
				t.Errorf("rows = %v, want %v", keys, tc.keys)
			}
			if tc.heldLabel != "" {
				if !dbx.Held || dbx.Action != tc.heldLabel {
					t.Errorf("finished row = %+v, want held as %q", dbx, tc.heldLabel)
				}
				if jobs := fw.Source().Jobs(fwDbx.DeviceID); len(jobs) != 0 {
					t.Error("a finished install can be queued again")
				}
			} else if dbx.Key != "" && dbx.Held {
				t.Errorf("row stays held: %+v", dbx)
			}
			if got := fw.Source().Status().Err; got != tc.srcErr {
				t.Errorf("source error = %q, want %q", got, tc.srcErr)
			}
			rec := env.Activity.Recent(5)
			switch {
			case tc.activity == "" && len(rec) != 0:
				t.Errorf("activity recorded a failure: %+v", rec)
			case tc.activity != "" && (len(rec) != 1 || rec[0].Title != tc.activity):
				t.Errorf("activity = %+v, want %q", rec, tc.activity)
			}
			// Every outcome releases the gate.
			fwAsk(t, env, fw, fwDock.DeviceID, "Cancel")
		})
	}
}

func TestFirmwareDryRunChangesNothing(t *testing.T) {
	env, q := updEnv(t, true, "firmware_updates_group")
	fw, ff := fwLoaded(t, env, q, fwDbx, fwDock)
	ff.mu.Lock()
	ff.dry = true // fwupd.Install logs and returns; InstallAndVerify reports nothing
	ff.mu.Unlock()
	before := fw.Source().Items()
	fwAsk(t, env, fw, fwDock.DeviceID, "Install")
	q.until(t, func() bool { return !env.Jobs.Busy() })
	toasts := env.TakeToasts()
	if len(toasts) != 1 || toasts[0].Title != "[DRY-RUN] Preview: firmware for Thunderbolt 4 Dock would be installed — no changes made" || toasts[0].Type != "info" {
		t.Errorf("toasts = %+v", toasts)
	}
	if after := fw.Source().Items(); !slices.Equal(after, before) {
		t.Errorf("a dry run changed the rows: %+v", after)
	}
	if rec := env.Activity.Recent(5); len(rec) != 0 {
		t.Errorf("a dry run recorded activity: %+v", rec)
	}
	fwAsk(t, env, fw, fwDock.DeviceID, "Cancel")
}

func TestFirmwareIsNeverPartOfUpdateEverything(t *testing.T) {
	env, q := updEnv(t, false, "firmware_updates_group")
	fw, _ := fwLoaded(t, env, q, fwDbx, fwDock, fwEC)
	src := fw.Source()
	if jobs := src.Jobs(); len(jobs) != 0 {
		t.Errorf("Jobs() = %d jobs, want none", len(jobs))
	}
	jobs := src.Jobs(fwDock.DeviceID, fwEC.DeviceID, "unknown")
	if got := updKeys(jobs); !slices.Equal(got, []string{"firmware/" + fwDock.DeviceID}) {
		t.Errorf("Jobs(keys) = %v, want the installable dock only", got)
	}
	for _, it := range src.Items() {
		if !it.Confirm {
			t.Errorf("%s does not ask first", it.Title)
		}
	}
}

func TestFirmwareCanceledInstallReleasesTheGate(t *testing.T) {
	env, q := updEnv(t, false, "firmware_updates_group")
	fw, ff := fwLoaded(t, env, q, fwDbx, fwDock)
	// Something else runs, so the install waits in the queue...
	release := make(chan struct{})
	image, _, _, _ := updFakes(env)
	image.hold = map[string]chan struct{}{"snow": release}
	env.Jobs.Enqueue(image.Jobs()...)
	fwAsk(t, env, fw, fwDbx.DeviceID, "Install")
	if !env.Jobs.Pending("firmware", fwDbx.DeviceID) {
		t.Fatal("the install is not queued")
	}
	// ...and is dropped.
	env.Jobs.CancelAll()
	q.until(t, func() bool { return !env.Jobs.Busy() })
	if len(ff.installs()) != 0 {
		t.Error("a dropped install ran")
	}
	if toasts := env.TakeToasts(); len(toasts) != 0 {
		t.Errorf("a dropped install toasted: %+v", toasts)
	}
	fwAsk(t, env, fw, fwDock.DeviceID, "Cancel")
	close(release)
}
