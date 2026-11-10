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
		b := maintPrimaryButton(c, "user-trash", "Clean everything").Disabled(len(idle) == 0)
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

// maintPrimaryButton is an accent button with an icon before its label.
// frostui.LabeledButton(c, icon, label, true) would do, but it builds a
// stray empty button before the primary one.
func maintPrimaryButton(c *ui.Context, icon, label string) ui.Element {
	return ui.PrimaryButton(c, "").Label(label).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			frostui.Icon(c, icon)
			ui.Text(c, label).SingleLine()
		})
	})
}

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
	t := c.Theme()
	m := p.s.Maint
	ui.Row(c).Gap(frostui.SpaceM).Padding(2, 0).AlignItems(ui.Center).Children(func() {
		frostui.IconTile(c, cl.Icon, frostui.Neutral, 32)
		ui.Column(c).Grow(1).Basis(0).Gap(3).Children(func() {
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				ui.Text(c, cl.Title).FontSize(13.5).FontWeight(600).SingleLine().Shrink(1)
				if cl.Admin {
					frostui.IconPill(c, "changes-prevent", "admin", frostui.Neutral).
						Tooltip("Asks for administrator authentication")
				}
			})
			frostui.Mono(c, strings.Join(cl.Argv, " ")).FontSize(11.5).TextColor(t.TextMuted).SingleLine().Selectable()
		})
		ui.Row(c).Width(132).Gap(6).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			p.cleanupStatus(c, cl)
		})
		ui.Row(c).Width(72).Justify(ui.End).Children(func() {
			if cl.Checking {
				return
			}
			b := ui.Button(c, "Clean").Label("Clean: " + cl.Title).Disabled(m.Pending(cl.Key))
			if b.Clicked() {
				m.Ask(cl.Key)
			}
		})
	})
}

// cleanupStatus is the state at the end of a cleanup's row: being
// checked, waiting or running, or how the last run ended.
func (p *maintenancePage) cleanupStatus(c *ui.Context, cl cleanup) {
	m := p.s.Maint
	if cl.Checking {
		ui.Spinner(c).Label("Checking")
		frostui.Muted(c, "Checking…").SingleLine()
		return
	}
	if j := m.env.Jobs.Find(maintSource, cl.Key); j != nil && !j.State.Ended() {
		text := "Waiting"
		if j.State == frostui.JobRunning {
			text = "Cleaning…"
		}
		ui.Spinner(c).Label(text)
		frostui.Muted(c, text).SingleLine()
		return
	}
	r := m.Result(cl.Key)
	when := ""
	if !r.At.IsZero() {
		when = r.At.Format("15:04")
	}
	switch r.Outcome {
	case cleanupDone:
		frostui.Icon(c, "object-select").TextColor(c.Theme().Success)
		frostui.Toned(c, "Cleaned", frostui.Success).SingleLine().Tooltip("Cleaned at " + when)
	case cleanupPreviewed:
		frostui.Muted(c, "Previewed").SingleLine().Tooltip("Dry run at " + when + ": nothing changed")
	case cleanupFailed:
		frostui.Icon(c, "dialog-error").TextColor(c.Theme().Danger)
		frostui.Toned(c, "Failed", frostui.Danger).SingleLine().Tooltip(r.Err)
	case cleanupCanceled:
		frostui.Muted(c, "Canceled").SingleLine()
	}
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
	t := c.Theme()
	bg, border, iconColor := t.Background, t.Border, t.TextMuted
	if booted {
		bg, border, iconColor = t.Accent.Alpha(0.14), t.Accent, t.Accent
	}
	row := ui.Row(c).Gap(frostui.SpaceM).Padding(9, 14).Radius(10).Background(bg).Border(1, border).AlignItems(ui.Center)
	if !d.Present {
		row.Opacity(0.6)
	}
	row.Children(func() {
		frostui.Icon(c, icon).TextColor(iconColor)
		ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
			frostui.Caption(c, role).SingleLine()
			version := empty
			if d.Present {
				version = d.Version
				if version == "" {
					version = "unknown version"
				}
			}
			frostui.Mono(c, version).FontSize(13).SingleLine().Selectable()
			if d.Present && d.Digest != "" {
				frostui.Mono(c, pageview.ShortDigest(d.Digest)).FontSize(11).TextColor(t.TextMuted).SingleLine().
					Selectable().Tooltip(d.Digest)
			}
		})
		if d.Present && d.Pinned {
			frostui.IconPill(c, "changes-prevent", "pinned", frostui.Neutral)
		}
	})
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
