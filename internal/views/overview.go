package views

import (
	_ "embed"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/activity"
	"github.com/frostyard/chairlift/internal/navigation"
	"github.com/frostyard/chairlift/internal/sysinfo"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/frostui"
)

// defaultHealthApp is the system monitor the health group opens when its
// configuration names none, as the old System page did.
const defaultHealthApp = "io.missioncenter.MissionCenter"

// overviewPage is the Overview: the system image hero with the one main
// action, figures of the other pages, the live chart, the disk, the recent
// activity and the system details. It belongs to the UI thread.
type overviewPage struct {
	s *Store
	// sources are the update sources the hero and the figures count:
	// Store.Sources, which tests replace.
	sources func() []UpdateSource
	// launch starts a desktop application by its ID; it runs on a
	// goroutine (Env.launchApp).
	launch               func(appID string) error
	healthID, healthName string
	healthResolved       bool
	healthLoading        bool
}

// NewOverviewPage returns the Overview page.
func NewOverviewPage(s *Store) Page {
	return &overviewPage{s: s, sources: s.Sources, launch: launchDesktopApp}
}

// heroState is what the hero offers now.
type heroState int

const (
	// heroNone offers nothing: no update source is configured.
	heroNone heroState = iota
	// heroUpToDate: nothing to update.
	heroUpToDate
	// heroWorking: jobs run; the hero follows the batch.
	heroWorking
	// heroUpdate: items "Update everything" covers exist.
	heroUpdate
	// heroRestart: an image is staged and applies on restart. ChairLift
	// never restarts the computer itself.
	heroRestart
	// heroReview: only items that wait for the user remain (firmware to
	// confirm, held packages).
	heroReview
)

// updateTally counts what the update sources offer.
type updateTally struct {
	items, sources          int
	runnable, confirm, held int
	checking                bool
	// checkedAt is the oldest last check of the sources that ended one.
	checkedAt time.Time
	// applicable counts the sources; failed names those whose last check
	// failed. "Up to date" is claimed only when none failed.
	applicable int
	failed     []string
}

func tallyUpdates(sources []UpdateSource) updateTally {
	var t updateTally
	for _, src := range sources {
		st := src.Status()
		t.applicable++
		if st.Err != "" && !st.Checking {
			t.failed = append(t.failed, src.Title())
		}
		t.checking = t.checking || st.Checking
		if !st.CheckedAt.IsZero() && (t.checkedAt.IsZero() || st.CheckedAt.Before(t.checkedAt)) {
			t.checkedAt = st.CheckedAt
		}
		items := src.Items()
		if len(items) > 0 {
			t.sources++
		}
		for _, it := range items {
			t.items++
			switch {
			case it.Held:
				t.held++
			case it.Confirm:
				t.confirm++
			default:
				t.runnable++
			}
		}
	}
	return t
}

// heroStateOf decides the hero's state. Running jobs come first, then what
// "Update everything" would do, then a staged image, then what waits for
// the user.
func heroStateOf(updates, busy, staged bool, t updateTally) heroState {
	switch {
	case busy:
		return heroWorking
	case updates && t.runnable > 0:
		return heroUpdate
	case staged:
		return heroRestart
	case !updates:
		return heroNone
	case t.confirm+t.held > 0:
		return heroReview
	}
	return heroUpToDate
}

func overviewPlural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// overviewChecked says when the sources were last checked.
func overviewChecked(at, now time.Time) string {
	if at.IsZero() {
		return "Not checked yet"
	}
	w := activity.When(at, now)
	switch {
	case w == "just now" || strings.HasSuffix(w, " ago"):
		return "Checked " + w
	case w == "Yesterday":
		return "Checked yesterday"
	}
	return "Checked on " + w
}

// waitingNote says what waits for the user, "" for nothing.
func waitingNote(t updateTally) string {
	switch {
	case t.confirm > 0 && t.held == 0:
		return overviewPlural(t.confirm, "firmware update waits for you", "firmware updates wait for you")
	case t.confirm+t.held > 0:
		return overviewPlural(t.confirm+t.held, "update waits for you", "updates wait for you")
	}
	return ""
}

// heroText is the hero's headline and the line under it for a state.
func heroText(state heroState, info SystemInfo, t updateTally, now time.Time) (title, note string) {
	checked := overviewChecked(t.checkedAt, now)
	if t.checking {
		checked = "Checking for updates…"
	}
	join := func(parts ...string) string {
		var out []string
		for _, p := range parts {
			if p != "" {
				out = append(out, p)
			}
		}
		return strings.Join(out, " · ")
	}
	switch state {
	case heroUpdate:
		return overviewPlural(t.runnable, "update ready", "updates ready"), join(checked, waitingNote(t))
	case heroRestart:
		name := strings.TrimSpace(info.OSName + " " + info.Staged.Version)
		if name == "" {
			name = "A new system image"
		}
		note := name + " is staged and applies when the computer restarts."
		if w := waitingNote(t); w != "" {
			note += " " + strings.ToUpper(w[:1]) + w[1:] + "."
		}
		return "Restart to finish", note
	case heroReview:
		if t.held == 0 {
			return overviewPlural(t.confirm, "firmware update", "firmware updates"), join("Each device updates after you confirm it", checked)
		}
		return overviewPlural(t.confirm+t.held, "update waits", "updates wait"), join("Held packages and firmware wait for you", checked)
	case heroUpToDate:
		switch {
		case t.checking:
			return "Checking for updates", ""
		case len(t.failed) > 0 && len(t.failed) == t.applicable:
			// Every check failed (offline, say): nothing is known.
			return "Could not check for updates", "Check now tries again"
		case len(t.failed) > 0:
			return "Some checks failed", join("Could not check "+strings.Join(t.failed, ", "), checked)
		case t.checkedAt.IsZero():
			// Nothing answered yet: "Up to date" would be a guess.
			return "Not checked yet", "Check now asks every source what is new"
		}
		return "Up to date", checked
	}
	return "", ""
}

// overviewSubtitle is the page header's line: the host, the system and its
// channel.
func overviewSubtitle(info SystemInfo) string {
	var parts []string
	if info.Hostname != "" {
		parts = append(parts, info.Hostname)
	}
	if info.OSName != "" {
		sys := info.OSName
		if info.Channel != "" {
			sys += ", " + info.Channel + " channel"
		}
		parts = append(parts, sys)
	}
	return strings.Join(parts, " · ")
}

// overviewImageLine is the hero's line under the name: the image, the kind
// of host and the kernel.
func overviewImageLine(info SystemInfo) string {
	var parts []string
	if info.ImageRef != "" {
		parts = append(parts, info.ImageRef)
	}
	if info.Kind != "" {
		parts = append(parts, info.Kind)
	}
	if info.Kernel != "" {
		parts = append(parts, "kernel "+info.Kernel)
	}
	return strings.Join(parts, " · ")
}

func (p *overviewPage) visible(page string) bool {
	_, ok := navigation.Resolve(page, navigation.VisibleItems(p.s.Env.Enabled))
	return ok
}

func (p *overviewPage) healthApp() (id, name string) {
	if p.healthResolved {
		return p.healthID, p.healthName
	}
	id = defaultHealthApp
	if g := p.s.Env.Group("system_page", "health_group"); g != nil && g.AppID != "" {
		id = g.AppID
	}
	return id, pageview.AppDisplayName(id)
}

func (p *overviewPage) loadHealthApp() {
	env := p.s.Env
	if !env.Enabled("system_page", "health_group") || env.DesktopApps == nil {
		return
	}
	group := env.Group("system_page", "health_group")
	ids := []string{defaultHealthApp, "org.gnome.SystemMonitor", "org.kde.plasma-systemmonitor"}
	if group != nil && len(group.AppIDs) != 0 && (len(group.AppIDs) != 1 || group.AppIDs[0] != "") {
		ids = append([]string{}, group.AppIDs...)
	} else if group != nil && group.AppID != "" {
		ids = []string{group.AppID}
	}
	list := env.DesktopApps
	p.healthResolved = true
	p.healthLoading = true
	go func() {
		apps, err := list()
		var id, name string
		if err == nil {
			for _, candidate := range ids {
				for _, app := range apps {
					if app.ID == strings.TrimSuffix(candidate, ".desktop")+".desktop" {
						id, name = candidate, app.Name
						break
					}
				}
				if id != "" {
					break
				}
			}
		} else {
			log.Printf("system monitor lookup: %v", err)
		}
		if id != "" && name == "" {
			name = pageview.AppDisplayName(id)
		}
		env.Post(func() { p.healthID, p.healthName, p.healthLoading = id, name, false })
	}()
}

func (p *overviewPage) openHealthApp() {
	id, _ := p.healthApp()
	p.s.Env.launchApp(p.launch, id)
}

func (p *overviewPage) state(t updateTally, info SystemInfo) heroState {
	return heroStateOf(p.visible(navigation.Updates), p.s.Env.Jobs.Busy(), info.Staged.Present, t)
}

// The content widths, in DIPs, under which the Overview stacks: the hero's
// action goes under the system's name, and cards side by side go one under
// the other.
const (
	overviewStackHero  = 1000
	overviewStackCards = 960
)

// overviewContentWidth is the width the shell gives a page in a window of
// c's size: the window less the rail and the page's padding, at most the
// shell's widest content.
func overviewContentWidth(c *ui.Context) float32 {
	w, _ := c.Size()
	return min(w-frostui.RailWidth-1-2*28, 1320)
}

// cardRow lays cards side by side when wide, else one under the other.
func cardRow(c *ui.Context, wide bool, fn func()) {
	if wide {
		ui.Row(c).Gap(frostui.SpaceL).AlignItems(ui.Stretch).Children(fn)
		return
	}
	ui.Column(c).Gap(18).AlignItems(ui.Stretch).Children(fn)
}

// sideCard is a card that takes share of a wide row; in a stacked column
// it keeps its own height.
func sideCard(c *ui.Context, wide bool, share float32) ui.Element {
	card := frostui.Card(c)
	if wide {
		card.Grow(share).Basis(0)
	}
	return card
}

// View draws the page.
func (p *overviewPage) View(c *ui.Context) {
	env := p.s.Env
	info := p.s.System.Info()
	tally := tallyUpdates(p.sources())
	state := p.state(tally, info)
	hero := env.Enabled("system_page", "bootc_status_group")
	width := overviewContentWidth(c)
	wide := width >= overviewStackCards

	var actions func()
	if !hero {
		actions = func() { p.actionButtons(c, state, tally) }
	}
	frostui.PageHeader(c, "Overview", overviewSubtitle(info), actions)
	if hero {
		p.heroCard(c, info, tally, state, width >= overviewStackHero)
	}
	if figs := p.figures(tally); len(figs) > 0 {
		frostui.Figures(c, figs)
	}

	health := env.Enabled("system_page", "health_group")
	disk := p.s.Live.Latest().Disk
	if health || disk.Total > 0 {
		cardRow(c, wide, func() {
			if health {
				p.liveCard(c, wide)
			}
			if disk.Total > 0 {
				p.diskCard(c, disk, wide)
			}
		})
	}
	cardRow(c, wide, func() {
		if env.Enabled("system_page", "system_info_group") {
			p.detailsCard(c, info, wide)
		}
		p.activityCard(c, wide)
	})
}

func (p *overviewPage) heroCard(c *ui.Context, info SystemInfo, tally updateTally, state heroState, wide bool) {
	t := c.Theme()
	frostui.Card(c).Padding(22).Gap(18).Children(func() {
		ui.Row(c).Gap(18).AlignItems(ui.Center).Children(func() {
			// frostyard's deep-blue panel, the Frostyard theme's card face,
			// in both themes: the pale snowflake reads on it in light and
			// dark alike. Its corners are the brand tile's (the theme's
			// Radius), its outline the accent's line.
			ui.Box(c).Size(heroLogo, heroLogo).Radius(t.Radius).Background(heroLogoFace()).Border(1, frostui.ToneLine(t, frostui.Accent)).Center().Children(func() {
				if bmp := overviewSnowflake(); bmp != nil {
					ui.Image(c, bmp).Size(heroLogo-2, heroLogo-2).Label("Snowflake")
				} else {
					// Sized to the tile, as frostui.IconTile sizes its
					// icon, not to the text.
					frostui.Icon(c, "weather-snow").FontSize(heroLogo * 9 / 16).TextColor(t.Accent)
				}
			})
			ui.Column(c).Grow(1).Gap(8).AlignItems(ui.Start).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					name := info.OSName
					if name == "" {
						name = "This computer"
					}
					frostui.Title(c, name).SingleLine().Role(ui.RoleHeading)
					if info.Channel != "" {
						frostui.Pill(c, info.Channel, frostui.Accent)
					}
					switch info.SecureBoot {
					case "on":
						frostui.IconPill(c, "channel-secure", "Secure Boot", frostui.Success)
					case "off":
						frostui.IconPill(c, "security-low", "Secure Boot off", frostui.Warning)
					}
				})
				if line := overviewImageLine(info); line != "" {
					frostui.Muted(c, line).SingleLine()
				}
				p.deploymentChain(c, info)
			})
			if wide {
				p.heroStatus(c, info, tally, state, ui.End)
			}
		})
		if !wide && state != heroNone {
			frostui.RowDivider(c)
			p.heroStatus(c, info, tally, state, ui.Start)
		}
	})
}

// heroLogo is the side of the hero's snowflake tile, in DIPs.
const heroLogo = 64

// heroLogoFace is the snowflake tile's face: the Frostyard (dark) theme's
// card face, whatever the window's theme.
func heroLogoFace() ui.Color {
	return frostui.ColorsOf(frostui.FrostyardTheme(true)).PanelFrom
}

// overviewSnowflake is ChairLift's snowflake, its application icon
// (data/icons/hicolor/scalable/apps/org.frostyard.ChairLift.svg) drawn
// once into a PNG: the hero shows it for the operating system.
//
//go:embed assets/chairlift-snowflake.png
var overviewSnowflakePNG []byte

var (
	overviewSnowflakeOnce sync.Once
	overviewSnowflakeBmp  *ui.Bitmap
)

// overviewSnowflake decodes the snowflake once; nil if it cannot.
func overviewSnowflake() *ui.Bitmap {
	overviewSnowflakeOnce.Do(func() {
		bmp, err := ui.DecodeBitmap(overviewSnowflakePNG)
		if err != nil {
			log.Printf("overview: the snowflake does not decode: %v", err)
			return
		}
		overviewSnowflakeBmp = bmp
	})
	return overviewSnowflakeBmp
}

// deploymentChain draws Rollback → Booted → Staged on an image-based host.
func (p *overviewPage) deploymentChain(c *ui.Context, info SystemInfo) {
	sys := p.s.System
	switch {
	case sys.DeploymentsLoading():
		frostui.Loading(c, "Reading the deployments")
		return
	case sys.DeploymentsError() != "":
		frostui.Problem(c, sys.DeploymentsError())
		return
	case info.Kind == "":
		return
	}
	if e := sys.DeploymentsRefreshError(); e != "" {
		defer frostui.Problem(c, "Showing the last known deployments: "+e)
	}
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Role(ui.RoleList).Label("Deployments").Children(func() {
		deploymentChip(c.Key("rollback"), "Rollback", "document-revert", info.Rollback, false, rollbackEmpty(info))
		frostui.Icon(c.Key("arrow1"), "go-next").TextColor(t.TextMuted)
		deploymentChip(c.Key("booted"), "Booted", "object-select", info.Booted, true, "unknown")
		frostui.Icon(c.Key("arrow2"), "go-next").TextColor(t.TextMuted)
		deploymentChip(c.Key("staged"), "Staged", "folder-download", info.Staged, false, "nothing yet")
	})
}

// deploymentChip is one deployment of the chain. The booted one is drawn
// in the accent: the kit's accent fill inside an accent hairline.
func deploymentChip(c *ui.Context, role, icon string, d Deployment, booted bool, empty string) {
	t := c.Theme()
	bg, border, iconColor := t.Background, t.Border, t.TextMuted
	if booted {
		bg, border, iconColor = frostui.ToneFill(t, frostui.Accent), t.Accent, t.Accent
	}
	version := d.Version
	if version == "" {
		version = d.Digest
	}
	if !d.Present || version == "" {
		version = empty
	}
	label := role + ": " + version
	if d.Pinned {
		label += ", pinned"
	}
	row := ui.Row(c).Gap(10).Padding(8, 12).Radius(frostui.RadiusChip).Background(bg).Border(1, border).AlignItems(ui.Center).
		Role(ui.RoleListItem).Label(label)
	if !d.Present {
		row.Opacity(0.55)
	}
	if d.Digest != "" {
		row.Tooltip(d.Digest)
	}
	row.Children(func() {
		frostui.Icon(c, icon).TextColor(iconColor)
		ui.Column(c).Gap(1).Children(func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				frostui.Caption(c, role)
				if d.Pinned {
					frostui.Icon(c, "view-pin").FontSize(frostui.TextSize(c, 11)).TextColor(t.TextMuted)
				}
			})
			frostui.Mono(c, version).FontSize(frostui.TextSize(c, 13)).SingleLine()
		})
	})
}

// heroStatus draws the hero's end, or its foot when stacked: the state's
// headline and its actions, aligned to align.
func (p *overviewPage) heroStatus(c *ui.Context, info SystemInfo, tally updateTally, state heroState, align ui.Align) {
	if state == heroNone {
		return
	}
	env := p.s.Env
	col := ui.Column(c).Gap(8).AlignItems(align)
	if align == ui.End {
		col.Shrink(0).MaxWidth(440)
	}
	col.Children(func() {
		if state == heroWorking {
			cur := env.Jobs.Current()
			done, total := env.Jobs.Batch()
			frostui.Muted(c, "Working")
			title := "Starting…"
			if cur != nil {
				title = cur.Title
			}
			ui.Text(c, title).FontSize(frostui.TextSize(c, 18)).Bold().SingleLine()
			ui.Progress(c, env.Jobs.BatchFraction()).Width(260).Label("Progress of the running jobs")
			status := fmt.Sprintf("Job %d of %d · output in the console", min(done+1, max(total, 1)), max(total, 1))
			frostui.Muted(c, status).SingleLine()
			if cur != nil && cur.Status != "" {
				frostui.Caption(c, cur.Status).SingleLine().MaxWidth(360)
			}
			return
		}
		title, note := heroText(state, info, tally, env.Now())
		ui.Text(c, title).FontSize(frostui.TextSize(c, 22)).Bold().SingleLine()
		if note != "" {
			frostui.Muted(c, note).Wrap().TextAlign(align)
		}
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Padding(4, 0, 0, 0).Children(func() {
			p.actionButtons(c, state, tally)
		})
	})
}

// actionButtons are the hero's actions for a state; the page header shows
// them when the hero's group is off.
func (p *overviewPage) actionButtons(c *ui.Context, state heroState, tally updateTally) {
	if state == heroNone || state == heroWorking {
		return
	}
	env := p.s.Env
	if p.visible(navigation.Updates) {
		check := frostui.LabeledButton(c, "view-refresh", "Check now", false).Disabled(tally.checking).Tooltip("Check every source for updates")
		if check.Clicked() {
			p.s.CheckAll()
		}
	}
	review := "Review updates"
	if tally.held == 0 {
		review = "Review firmware"
	}
	switch state {
	case heroUpdate:
		if frostui.LabeledButton(c, "software-update-available", "Update everything", true).Clicked() {
			p.s.UpdateEverything()
		}
	case heroRestart:
		if tally.confirm+tally.held > 0 && frostui.LabeledButton(c, "go-next", review, false).Clicked() {
			env.Navigate(navigation.Updates)
		}
	case heroReview:
		if frostui.LabeledButton(c, "go-next", review, true).Clicked() {
			env.Navigate(navigation.Updates)
		}
	}
}

// figures are the strip under the hero: one figure per visible page that
// has one to give, and the free space.
func (p *overviewPage) figures(t updateTally) []frostui.Figure {
	env := p.s.Env
	var figs []frostui.Figure
	if p.visible(navigation.Updates) {
		note, tone := "all current", frostui.Success
		if t.items > 0 {
			note, tone = "across "+overviewPlural(t.sources, "source", "sources"), frostui.Accent
		}
		figs = append(figs, frostui.Figure{Label: "Updates", Value: strconv.Itoa(t.items), Note: note, Tone: tone,
			OnClick: func() { env.Navigate(navigation.Updates) }})
	}
	if ps := p.s.Problems.Summary(); ps.Known && p.visible(navigation.Problems) {
		note, tone := "nothing needs you", frostui.Neutral
		switch {
		case ps.Attention > 0:
			note, tone = overviewPlural(ps.Attention, "needs attention", "need attention"), frostui.Danger
		case ps.Count == 0:
			note, tone = "all clear", frostui.Success
		}
		figs = append(figs, frostui.Figure{Label: "Problems", Value: strconv.Itoa(ps.Count), Note: note, Tone: tone,
			OnClick: func() { env.Navigate(navigation.Problems) }})
	}
	if fs := p.s.Features.Summary(); fs.Known && p.visible(navigation.Features) {
		figs = append(figs, frostui.Figure{Label: "Features on", Value: fmt.Sprintf("%d of %d", fs.On, fs.Total), Note: "system extensions",
			OnClick: func() { env.Navigate(navigation.Features) }})
	}
	if d := p.s.Live.Latest().Disk; d.Total > 0 {
		fig := frostui.Figure{Label: "Free space", Value: sysinfo.FormatBytes(d.Free), Note: "of " + sysinfo.FormatBytes(d.Total) + " on " + d.Path}
		if d.UsedPercent() >= 90 {
			fig.Tone = frostui.Warning
		}
		if p.visible(navigation.Maintenance) {
			fig.OnClick = func() { env.Navigate(navigation.Maintenance) }
		}
		figs = append(figs, fig)
	}
	return figs
}

func lastValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

// liveCard is the health group's card: processor and memory over the last
// minute, and the button that opens the configured system monitor.
func (p *overviewPage) liveCard(c *ui.Context, wide bool) {
	cpu, mem := p.s.Live.CPU(), p.s.Live.Memory()
	id, app := p.healthApp()
	sideCard(c, wide, 2).Children(func() {
		frostui.CardHeader(c, "Right now", "Processor and memory, last 60 seconds", func() {
			frostui.Pill(c, "live", frostui.Success)
			if id != "" && frostui.LabeledButton(c, "send-to", "Open "+app, false).Tooltip("Monitor processor, memory and system resources").Clicked() {
				p.openHealthApp()
			}
		})
		ui.Row(c).Gap(frostui.SpaceL).AlignItems(ui.Center).Children(func() {
			frostui.Legend(c, frostui.Accent, fmt.Sprintf("CPU %.0f%%", lastValue(cpu)))
			frostui.Legend(c, frostui.Warning, fmt.Sprintf("Memory %.0f%%", lastValue(mem)))
		})
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Start).Children(func() {
			chartScale(c, liveChartHeight)
			// Memory first, so the processor's line and area draw over it.
			frostui.LiveChart(c, "Processor and memory over the last minute", liveChartHeight,
				frostui.Series{Name: "Memory", Values: mem, Tone: frostui.Warning},
				frostui.Series{Name: "CPU", Values: cpu, Tone: frostui.Accent}).Grow(1)
		})
	})
}

const liveChartHeight = 170

// chartScale labels LiveChart's grid lines (0, 25, 50, 75 and 100 %) at
// their heights, beside a chart height tall. The labels and their line
// height follow the text size; the chart's height does not.
func chartScale(c *ui.Context, height float32) {
	label := frostui.TextSize(c, 14) // a label's line height
	ui.Column(c).Height(height+label).Margin(-label/2, 0, -label/2, 0).Justify(ui.SpaceBetween).AlignItems(ui.End).
		Children(func() {
			for i, l := range []string{"100%", "75%", "50%", "25%", "0%"} {
				frostui.Caption(c.Key(i), l).FontSize(frostui.TextSize(c, 11)).Height(label).SingleLine()
			}
		})
}

// diskCard shows the free space of the home directory's file system.
func (p *overviewPage) diskCard(c *ui.Context, d sysinfo.Disk, wide bool) {
	env := p.s.Env
	used := d.UsedPercent()
	tone := frostui.Accent
	switch {
	case used >= 95:
		tone = frostui.Danger
	case used >= 80:
		tone = frostui.Warning
	}
	sideCard(c, wide, 1).Children(func() {
		var trailing func()
		if p.visible(navigation.Maintenance) {
			trailing = func() {
				if ui.Button(c, "Free up space").Clicked() {
					env.Navigate(navigation.Maintenance)
				}
			}
		}
		frostui.CardHeader(c, "Disk", "The file system of your home folder", trailing)
		ui.Row(c).Grow(1).Gap(frostui.SpaceL).AlignItems(ui.Center).Children(func() {
			frostui.Gauge(c, float64(d.Used()), float64(d.Total), fmt.Sprintf("%.0f%%", used), "in use", tone, 132)
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Text(c, sysinfo.FormatBytes(d.Free)+" free").FontSize(frostui.TextSize(c, 20)).Bold().SingleLine()
				frostui.Muted(c, sysinfo.FormatBytes(d.Used())+" used of "+sysinfo.FormatBytes(d.Total))
				frostui.Mono(c, d.Path).TextColor(c.Theme().TextMuted).SingleLine()
			})
		})
	})
}

// activityIcon is the icon of an activity entry's source.
func activityIcon(source string) string {
	switch source {
	case string(SourceImage):
		return "drive-harddisk-system"
	case string(SourceFeatures):
		return "application-x-addon"
	case string(SourceFlatpak), string(SourceBrew):
		return "package-x-generic"
	case string(SourceFirmware):
		return "application-x-firmware"
	case "maintenance":
		return "edit-clear-all"
	case "problems":
		return "dialog-warning"
	}
	return "object-select"
}

// activityCard lists what ChairLift itself changed, newest first.
func (p *overviewPage) activityCard(c *ui.Context, wide bool) {
	env := p.s.Env
	entries := env.Activity.Recent(5)
	now := env.Now()
	sideCard(c, wide, 1).Children(func() {
		frostui.CardHeader(c, "Recent activity", "What ChairLift changed on this computer", nil)
		if len(entries) == 0 {
			frostui.Empty(c, "document-open-recent", "Nothing yet", "Updates, features and cleanups that ChairLift runs show here.")
			return
		}
		for i, e := range entries {
			ui.Row(c.Key(i)).Gap(12).AlignItems(ui.Start).Children(func() {
				frostui.IconTile(c, activityIcon(e.Source), frostui.Accent, 30)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					frostui.Strong(c, e.Title).Wrap()
					if e.Detail != "" {
						frostui.Muted(c, e.Detail).Wrap()
					}
				})
				frostui.Caption(c, activity.When(e.At, now)).SingleLine()
			})
		}
	})
}

// overviewBuilt shows an image's build time in local time; a time it cannot
// read stays as bootc wrote it.
func overviewBuilt(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Local().Format("2 Jan 2006, 15:04")
	}
	return ts
}

// computerRows are the System details card's facts of the machine and of
// the booted image (the old System page's Image, Built and Digest rows).
func computerRows(info SystemInfo, built string) []frostui.KeyValue {
	var rows []frostui.KeyValue
	add := func(key, value string, mono bool) {
		if value != "" {
			rows = append(rows, frostui.KeyValue{Key: key, Value: value, Mono: mono})
		}
	}
	add("Host name", info.Hostname, false)
	add("Model", info.Model, false)
	add("Kernel", info.Kernel, true)
	switch info.SecureBoot {
	case "on":
		add("Secure Boot", "On", false)
	case "off":
		add("Secure Boot", "Off", false)
	}
	add("Running", strings.TrimPrefix(info.Uptime, "up "), false)
	add("Image", info.ImageRef, true)
	if built != "" {
		add("Built", overviewBuilt(built), false)
	}
	add("Digest", info.Booted.Digest, true)
	add("Updates with", info.Kind, false)
	return rows
}

// detailsCard is the system_info_group's card: the machine's facts and the
// operating system's os-release, whose URLs open in the browser.
func (p *overviewPage) detailsCard(c *ui.Context, info SystemInfo, wide bool) {
	entries, ok := p.s.System.OSRelease()
	sideCard(c, wide, 2).Children(func() {
		frostui.CardHeader(c, "System details", "This computer and its operating system", nil)
		ui.Row(c).Gap(frostui.SpaceXL).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).Basis(0).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
				frostui.Caption(c, "This computer")
				if rows := computerRows(info, p.s.System.ImageBuilt()); len(rows) > 0 {
					frostui.KeyValues(c, rows, detailsStyle(c))
				} else {
					frostui.Muted(c, "Not available")
				}
			})
			ui.Column(c).Grow(1).Basis(0).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
				frostui.Caption(c, "Operating system")
				osReleaseRows(c, entries)
				if !ok {
					frostui.Muted(c, "Not available")
				}
			})
		})
	})
}

// detailsStyle is the System details card's KeyValues style: the kit's
// default rows, with the values at 13 DIPs scaled to the text size.
func detailsStyle(c *ui.Context) frostui.KeyValuesStyle {
	return frostui.KeyValuesStyle{KeyWidth: 120, Gap: 6, FontSize: frostui.TextSize(c, 13)}
}

// osReleaseRows draws os-release's fields as KeyValues does in
// detailsStyle, with the URL fields as links.
func osReleaseRows(c *ui.Context, entries []pageview.OSReleaseEntry) {
	st := detailsStyle(c)
	ui.Column(c).Gap(st.Gap).AlignItems(ui.Stretch).Children(func() {
		for i, e := range entries {
			ui.Row(c.Key(i)).Gap(frostui.SpaceM).AlignItems(ui.Start).Children(func() {
				frostui.Muted(c, e.Title).Width(st.KeyWidth)
				if e.IsURL {
					ui.Link(c, e.Value, e.Value).FontSize(st.FontSize)
					return
				}
				ui.Text(c, e.Value).FontSize(st.FontSize).Grow(1).Wrap().Selectable()
			})
		}
	})
}

// Commands are the Overview's palette entries. "Check for updates" is the
// window's.
func (p *overviewPage) Commands() []frostui.Command {
	env := p.s.Env
	var cmds []frostui.Command
	if p.visible(navigation.Updates) {
		t := tallyUpdates(p.sources())
		cmd := frostui.Command{ID: "update-everything", Title: "Update everything", Group: "Actions", Icon: "software-update-available",
			Hint: "upgrade all apps packages features system image", Run: p.s.UpdateEverything}
		switch {
		case env.Jobs.Busy():
			cmd.Disabled, cmd.Reason = true, "a job is running"
		case t.runnable == 0:
			cmd.Disabled, cmd.Reason = true, "nothing to update"
		}
		cmds = append(cmds, cmd)
	}
	if src := p.s.System.Source(); src != nil && src.Status().Available && len(src.Items()) > 0 {
		cmd := frostui.Command{ID: "stage-system-image", Title: "Stage the system update", Group: "Actions", Icon: "drive-harddisk-system",
			Hint: "bootc sysupdate image upgrade", Run: func() { src.Update(imageItemKey) }}
		if env.Jobs.Pending(string(SourceImage), imageItemKey) {
			cmd.Disabled, cmd.Reason = true, "staging"
		}
		cmds = append(cmds, cmd)
	}
	if id, app := p.healthApp(); env.Enabled("system_page", "health_group") && id != "" {
		cmds = append(cmds, frostui.Command{ID: "open-health-app", Title: "Open " + app, Group: "Actions", Icon: "send-to",
			Hint: "system monitor performance processor memory", Run: p.openHealthApp})
	}
	return cmds
}

// Badge is none: the Overview sums the others up.
func (p *overviewPage) Badge() (string, frostui.Tone) { return "", frostui.Neutral }
