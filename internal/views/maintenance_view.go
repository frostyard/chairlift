package views

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/sysinfo"
	"github.com/frostyard/chairlift/internal/views/pageview"
)

// maintenancePage draws Maintenance: the cleanups, the free space and the
// deployments.
type maintenancePage struct{ s *Store }

// NewMaintenancePage returns the Maintenance page.
func NewMaintenancePage(s *Store) Page { return &maintenancePage{s: s} }

func (p *maintenancePage) View(c *ui.Context) {
	m := p.s.Maint
	disk := m.disk()
	sub := "Clean up what the system no longer needs"
	if disk.Total > 0 {
		sub = fmt.Sprintf("%s free of %s", sysinfo.FormatBytes(disk.Free), sysinfo.FormatBytes(disk.Total))
	}
	cleanups := m.Cleanups()
	idle := m.idle()
	frostui.PageHeader(c, "Maintenance", sub, func() {
		if len(m.ready()) == 0 {
			return
		}
		b := frostui.LabeledButton(c, "user-trash", "Clean everything", true).Disabled(len(idle) == 0)
		if len(idle) == 0 {
			b.Tooltip("Every cleanup is running or waiting")
		}
		if b.Clicked() {
			m.AskEverything()
		}
	})
	info := m.info()
	deployments := m.env.Enabled("system_page", "bootc_status_group") && info.Booted.Present
	ui.Row(c).Gap(maintCardGap).AlignItems(ui.Start).Wrap().Children(func() {
		frostui.Card(c).Grow(2).Basis(0).MinWidth(440).Children(func() {
			p.cleanupCard(c, cleanups)
		})
		if disk.Total == 0 && !deployments {
			return
		}
		ui.Column(c).Grow(1).Basis(0).MinWidth(300).Gap(maintCardGap).AlignItems(ui.Stretch).Children(func() {
			if disk.Total > 0 {
				maintFreeSpaceCard(c, disk)
			}
			if deployments {
				maintDeploymentsCard(c, info)
			}
		})
	})
}

// maintCardGap is the gap between the page's cards.
const maintCardGap = 16

func (p *maintenancePage) cleanupCard(c *ui.Context, cleanups []cleanup) {
	frostui.CardHeader(c, "Clean up", "Each cleanup asks first, then runs as a job; its output goes to the console", nil)
	if len(cleanups) == 0 {
		frostui.Empty(c, "user-trash", "Nothing to clean up here", "No cleanup is set up, and neither Homebrew nor Flatpak is installed.")
		return
	}
	for _, cl := range cleanups {
		ui.Column(c.Key(cl.Key)).Gap(frostui.SpaceS + 2).AlignItems(ui.Stretch).Children(func() {
			frostui.RowDivider(c)
			p.cleanupRow(c, cl)
		})
	}
}

func (p *maintenancePage) cleanupRow(c *ui.Context, cl cleanup) {
	m := p.s.Maint
	frostui.CommandRow(c, frostui.CommandItem{
		Icon: cl.Icon, Title: cl.Title, Command: strings.Join(cl.Argv, " "), Admin: cl.Admin,
		Status: func() { frostui.JobStatus(c, p.cleanupStatus(cl)) },
		Action: func() {
			if cl.Checking {
				return
			}
			b := ui.Button(c, "Clean").Label("Clean: " + cl.Title).Disabled(m.Pending(cl.Key))
			if b.Clicked() {
				m.Ask(cl.Key)
			}
		},
	})
}

// cleanupStatus is the state at the end of a cleanup's row: being
// checked, waiting or running, or how the last run ended; nothing before
// a first run.
func (p *maintenancePage) cleanupStatus(cl cleanup) frostui.RowStatus {
	m := p.s.Maint
	if cl.Checking {
		return frostui.RowStatus{State: frostui.StepRunning, Text: "Checking…", Label: "Checking"}
	}
	if j := m.env.Jobs.Find(maintSource, cl.Key); j != nil && !j.State.Ended() {
		if j.State == frostui.JobRunning {
			return frostui.RowStatus{State: frostui.StepRunning, Text: "Cleaning…"}
		}
		return frostui.RowStatus{State: frostui.StepQueued, Text: "Waiting"}
	}
	r := m.Result(cl.Key)
	when := ""
	if !r.At.IsZero() {
		when = r.At.Format("15:04")
	}
	switch r.Outcome {
	case cleanupDone:
		return frostui.RowStatus{State: frostui.StepDone, Text: "Cleaned", Tooltip: "Cleaned at " + when}
	case cleanupPreviewed:
		return frostui.RowStatus{State: frostui.StepSkipped, Text: "Previewed", Tooltip: "Dry run at " + when + ": nothing changed"}
	case cleanupFailed:
		return frostui.RowStatus{State: frostui.StepFailed, Text: "Failed", Tooltip: r.Err}
	case cleanupCanceled:
		return frostui.RowStatus{State: frostui.StepSkipped, Text: "Canceled"}
	}
	return frostui.RowStatus{}
}

// maintFreeSpaceCard draws the free space of the disk Live samples as a gauge.
func maintFreeSpaceCard(c *ui.Context, d sysinfo.Disk) {
	free := 100 - d.UsedPercent()
	tone := frostui.Success
	switch {
	case free < 10:
		tone = frostui.Danger
	case free < 25:
		tone = frostui.Warning
	}
	frostui.Card(c).AlignItems(ui.Stretch).Children(func() {
		frostui.CardHeader(c, "Free space", d.Path, nil)
		ui.Row(c).Justify(ui.Center).Padding(0, 0, frostui.SpaceS, 0).Children(func() {
			frostui.Gauge(c, float64(d.Free), float64(d.Total), sysinfo.FormatBytes(d.Free),
				"free of "+sysinfo.FormatBytes(d.Total), tone, 196)
		})
	})
}

// maintDeploymentsCard draws the deployment chain, read-only: staged, booted,
// rollback.
func maintDeploymentsCard(c *ui.Context, info SystemInfo) {
	sub := "The image running now, the one for the next boot and the one before"
	if info.Kind == "bootc" {
		sub = "bootc keeps the version before this one, ready to boot"
	}
	frostui.Card(c).Children(func() {
		frostui.CardHeader(c, "Deployments", sub, nil)
		ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
			maintDeploymentRow(c, "Staged · next boot", "nothing yet", "folder-download", info.Staged, false)
			maintDeploymentRow(c, "Booted · running now", "", "object-select", info.Booted, true)
			maintDeploymentRow(c, "Rollback · in the boot menu", "none", "document-revert", info.Rollback, false)
		})
	})
}

// maintDeploymentRow is one deployment of the chain; empty is what an empty
// slot says. The booted one is highlighted.
func maintDeploymentRow(c *ui.Context, role, empty, icon string, d Deployment, booted bool) {
	tile := frostui.Tile{Icon: icon, Caption: role, Value: empty, Current: booted, Dim: !d.Present}
	if d.Present {
		tile.Value = d.Version
		if tile.Value == "" {
			tile.Value = "unknown version"
		}
		if d.Digest != "" {
			tile.Detail, tile.DetailTooltip = pageview.ShortDigest(d.Digest), d.Digest
		}
		if d.Pinned {
			tile.Pill, tile.PillIcon = "pinned", "changes-prevent"
		}
	}
	frostui.StepTile(c, tile)
}

// Commands are one "Clean up …" per cleanup that can run now, then "Clean
// everything". Each one asks first, as the page's buttons do.
func (p *maintenancePage) Commands() []frostui.Command {
	m := p.s.Maint
	ready := m.ready()
	if len(ready) == 0 {
		return nil
	}
	cmds := make([]frostui.Command, 0, len(ready)+1)
	for _, cl := range ready {
		key := cl.Key
		cmd := frostui.Command{
			ID:    "maintenance.clean." + key,
			Title: cl.paletteTitle(),
			Group: "Maintenance",
			Icon:  cl.Icon,
			Hint:  "clean up " + strings.Join(cl.Argv, " "),
			Run:   func() { m.Ask(key) },
		}
		if m.Pending(key) {
			cmd.Disabled, cmd.Reason = true, "Running"
		}
		cmds = append(cmds, cmd)
	}
	all := frostui.Command{
		ID:    "maintenance.clean-everything",
		Title: "Clean everything",
		Group: "Maintenance",
		Icon:  "user-trash",
		Hint:  "clean up all maintenance",
		Run:   m.AskEverything,
	}
	if len(m.idle()) == 0 {
		all.Disabled, all.Reason = true, "Running"
	}
	return append(cmds, all)
}

// Badge is none: Maintenance counts nothing.
func (p *maintenancePage) Badge() (string, frostui.Tone) { return "", frostui.Neutral }
