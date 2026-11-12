package views

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/views/featurestatus"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/frostui"
)

// featuresBlurb says what features are, under the page's title.
const featuresBlurb = "system extensions that add to /usr without changing the image"

// featureIcons are the icons of the features Snow Linux ships, by feature
// name. A feature not listed gets the generic add-on icon.
var featureIcons = map[string]string{
	"docker":        "package-x-generic",
	"podman":        "package-x-generic",
	"incus":         "computer",
	"k3s":           "network-workgroup",
	"tailscale":     "network-vpn",
	"dev":           "applications-engineering",
	"debdev":        "applications-engineering",
	"nix":           "applications-engineering",
	"vscode":        "applications-engineering",
	"code-server":   "applications-engineering",
	"coder":         "applications-engineering",
	"1password":     "dialog-password",
	"1password-cli": "dialog-password",
	"bitwarden":     "dialog-password",
	"himmelblau":    "avatar-default",
	"moonlight":     "input-gaming",
	"sunshine":      "input-gaming",
	"localsend":     "send-to",
	"maild":         "mail-unread",
	"readd":         "application-rss+xml",
	"notesd":        "x-office-document",
	"obsidian":      "x-office-document",
	"voxtype":       "audio-input-microphone",
}

// featureIcon is the icon of the feature called name.
func featureIcon(name string) string {
	if icon, ok := featureIcons[name]; ok {
		return icon
	}
	return "application-x-addon"
}

// featureTitle is what a feature is called: its description, as the old
// page titled its rows, or its name when it has none.
func featureTitle(row featureRow) string {
	if title := pageview.Feature(row.Name, row.Description).Title; title != "" {
		return title
	}
	return row.Name
}

// featureCountDescription is the list's description before a check ended.
func featureCountDescription(total int) string {
	return pageview.FeatureGroupDescription(total)
}

// featureLine turns a row subtitle into the line under the row's title:
// featurestatus starts its subtitles with the feature's name, which the
// row already shows, and gives a feature that is up to date as its
// version, which the row shows in its own column. version is that version,
// "" when the subtitle has none.
func featureLine(name, subtitle string) (line, version string) {
	s := strings.TrimPrefix(subtitle, name+" — ")
	if len(s) > 1 && s[0] == 'v' && s[1] >= '0' && s[1] <= '9' {
		return "Up to date", s[1:]
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s, ""
	}
	return string(unicode.ToUpper(r)) + s[size:], ""
}

// featuresView is the Features page: a tile per feature to turn it on or
// off at a glance, and a list with each feature's state, version, update
// and switch.
type featuresView struct {
	f *Features
}

// NewFeaturesPage returns the Features page.
func NewFeaturesPage(s *Store) Page { return &featuresView{f: s.Features} }

// View draws the page.
func (p *featuresView) View(c *ui.Context) {
	f := p.f
	subtitle := strings.ToUpper(featuresBlurb[:1]) + featuresBlurb[1:]
	if f.available() && f.loaded {
		subtitle = fmt.Sprintf("%d of %d on · %s", f.onCount(), len(f.features), featuresBlurb)
	}
	frostui.PageHeader(c, "Features", subtitle, func() { p.actions(c) })

	switch {
	case !f.enabled():
		frostui.Card(c).Children(func() {
			frostui.Empty(c, "application-x-addon", "Features are turned off", "The configuration turns off the features group.")
		})
	case f.probed && f.missing:
		frostui.Card(c).Children(func() {
			frostui.Empty(c, "application-x-addon", "Feature Manager Not Available", "System features are not configured on this system")
		})
	case !f.loaded && f.listErr != "":
		frostui.Card(c).Children(func() {
			frostui.Problem(c, "Error: "+f.listErr)
			ui.Row(c).Children(func() {
				if ui.Button(c, "Try again").Disabled(f.loading).Clicked() {
					f.Refresh()
				}
			})
		})
	case !f.loaded:
		frostui.Card(c).Children(func() {
			frostui.Loading(c, "Loading features…")
		})
	case len(f.features) == 0:
		frostui.Card(c).Children(func() {
			frostui.Empty(c, "application-x-addon", "No features available", "This image offers no system extensions.")
		})
	default:
		if f.listErr != "" {
			if acted, _ := frostui.Banner(c, frostui.Danger, "dialog-warning", "Could not list the features again: "+f.listErr, "Try again", false); acted {
				f.Refresh()
			}
		}
		p.tiles(c)
		p.list(c)
	}
}

// actions are the header's buttons.
func (p *featuresView) actions(c *ui.Context) {
	f := p.f
	if !f.available() || !f.loaded || len(f.features) == 0 {
		return
	}
	checking := f.loading || f.checking
	label := "Check for updates"
	if checking {
		label = "Checking…"
	}
	if frostui.LabeledButton(c, "view-refresh", label, false).Disabled(checking).Clicked() {
		f.Refresh()
	}
	if len(f.updatable()) > 0 {
		label := "Update features"
		if f.updating() {
			label = "Updating…"
		}
		if frostui.LabeledButton(c, "software-update-available", label, true).Disabled(f.updating()).Clicked() {
			f.UpdateAll()
		}
	}
}

// tileRows are the features that get a tile: those that are on, and those
// with a job waiting or running, so a feature being turned on does not
// vanish from the grid until its job ended. An image ships dozens of
// features; the list below has all of them.
func (p *featuresView) tileRows() []featureRow {
	var out []featureRow
	for _, row := range p.f.features {
		if row.On || p.f.busy(row.Name) {
			out = append(out, row)
		}
	}
	return out
}

// tiles draws a toggle tile per feature that is on (see tileRows). A click
// asks first; the tile shows the listing until the job ran.
func (p *featuresView) tiles(c *ui.Context) {
	f := p.f
	rows := p.tileRows()
	if len(rows) == 0 {
		frostui.Muted(c, "No features are on. Turn one on in the list below.")
		return
	}
	ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
		frostui.TileGrid(c).Children(func() {
			for _, row := range rows {
				busy := f.busy(row.Name)
				if frostui.ToggleTile(c.Key("tile:"+row.Name), featureIcon(row.Name), featureTitle(row), p.tileSub(row, busy), row.On, busy) {
					f.AskToggle(row.Name, !row.On)
				}
			}
		})
		if more := len(f.features) - len(rows); more > 0 {
			frostui.Muted(c, fmt.Sprintf("%d more in the list below", more))
		}
	})
}

// tileSub is the line under a tile's title, as "On · 28.5.1 · update
// ready".
func (p *featuresView) tileSub(row featureRow, busy bool) string {
	switch {
	case busy:
		return "Working…"
	case row.Masked:
		return "Masked"
	case !row.On:
		return "Off"
	}
	parts := []string{"On"}
	status, ok := p.f.status(row.Name)
	if v := p.version(row, status, ok); v != "" {
		parts = append(parts, v)
	}
	if ok {
		switch {
		case status.WaitingForImage:
			parts = append(parts, "needs newer OS")
		case status.HasUpdate:
			parts = append(parts, "update ready")
		case status.PendingReboot:
			parts = append(parts, "applies after reboot")
		}
	}
	return strings.Join(parts, " · ")
}

// version is the version a row shows: the listing's, or the one the check
// agreed on.
func (p *featuresView) version(row featureRow, status featurestatus.Status, ok bool) string {
	if row.Version != "" {
		return row.Version
	}
	if !ok {
		return ""
	}
	_, v := featureLine(row.Name, status.Subtitle)
	return v
}

// list draws the card with a row per feature.
func (p *featuresView) list(c *ui.Context) {
	f := p.f
	t := c.Theme()
	frostui.Card(c).Gap(0).Padding(6, 18).Children(func() {
		ui.Row(c).Padding(frostui.SpaceS, 0, frostui.SpaceM, 0).Children(func() {
			frostui.CardHeader(c, "All features", f.groupDescription(), nil)
		})
		for _, row := range f.features {
			frostui.RowDivider(c.Key("divider:" + row.Name))
			p.listRow(c.Key("row:"+row.Name), t, row)
		}
	})
}

// listRow draws one feature of the list.
func (p *featuresView) listRow(c *ui.Context, t *ui.Theme, row featureRow) {
	f := p.f
	title := featureTitle(row)
	busy := f.busy(row.Name)
	status, checked := f.status(row.Name)
	line, _ := featureLine(row.Name, f.subtitle(row.Name))
	ui.Row(c).Gap(frostui.SpaceM).Padding(10, 0).AlignItems(ui.Center).Children(func() {
		tone := frostui.Neutral
		if row.On {
			tone = frostui.Accent
		}
		frostui.IconTile(c, featureIcon(row.Name), tone, 32)
		ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Baseline).Children(func() {
				frostui.Strong(c, title).SingleLine().Shrink(1)
				frostui.Caption(c, row.Name).Font("monospace").SingleLine().Shrink(0)
			})
			if line == "" {
				// Nothing to say beyond the switch and the version.
				return
			}
			lineTone := frostui.Neutral
			switch {
			case checked && status.WaitingForImage:
				lineTone = frostui.Warning
			case checked && status.HasUpdate:
				lineTone = frostui.Accent
			}
			frostui.Toned(c, line, lineTone).Wrap()
		})
		frostui.Mono(c, p.version(row, status, checked)).TextColor(t.TextMuted).Width(110).SingleLine()
		ui.Row(c).Width(170).Gap(frostui.SpaceS).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			if checked && status.HasUpdate && row.On {
				if f.updating() {
					ui.Button(c, "Updating…").Disabled(true)
				} else if ui.Button(c, "Update").Label("Update " + title).Tooltip("Updates every feature that is on").Clicked() {
					f.UpdateAll()
				}
			}
			if busy {
				ui.Spinner(c).Label("Working on " + title)
				return
			}
			on := row.On
			sw := ui.Switch(c, &on).Label(title + " (" + row.Name + ")").Disabled(row.Masked)
			if row.Masked {
				sw.Tooltip("Masked by the administrator")
			}
			if sw.Changed() {
				f.AskToggle(row.Name, on)
			}
		})
	})
}

// Commands are the page's palette entries: turn each feature on or off
// (they ask first), check for updates and update.
func (p *featuresView) Commands() []frostui.Command {
	f := p.f
	if !f.available() || !f.loaded {
		return nil
	}
	cmds := make([]frostui.Command, 0, len(f.features)+2)
	check := frostui.Command{ID: "features.check", Title: "Check features for updates", Group: "Features", Icon: "view-refresh", Hint: "updex sysext", Run: f.Refresh}
	if f.loading || f.checking {
		check.Disabled, check.Reason = true, "Checking"
	}
	update := frostui.Command{ID: "features.update", Title: "Update features", Group: "Features", Icon: "software-update-available", Hint: "updex sysext", Run: f.UpdateAll}
	switch {
	case f.updating():
		update.Disabled, update.Reason = true, "Updating"
	case len(f.updatable()) == 0:
		update.Disabled, update.Reason = true, "No updates"
	}
	cmds = append(cmds, check, update)
	for _, row := range f.features {
		if row.Masked {
			continue
		}
		name, on := row.Name, !row.On
		verb := "Turn on "
		if row.On {
			verb = "Turn off "
		}
		cmd := frostui.Command{
			ID:    "features.toggle." + row.Name,
			Title: verb + featureTitle(row),
			Group: "Features",
			Icon:  featureIcon(row.Name),
			Hint:  row.Name,
			Run:   func() { f.AskToggle(name, on) },
		}
		if f.busy(row.Name) {
			cmd.Disabled, cmd.Reason = true, "Working"
		}
		cmds = append(cmds, cmd)
	}
	return cmds
}

// Badge is the number of features with an update.
func (p *featuresView) Badge() (string, frostui.Tone) {
	f := p.f
	if !f.available() {
		return "", frostui.Neutral
	}
	if n := len(f.updatable()); n > 0 {
		return strconv.Itoa(n), frostui.Accent
	}
	return "", frostui.Neutral
}

// Extra says on the Updates page that one update covers every feature,
// and which features wait for a newer OS image.
func (s *featureSource) Extra(c *ui.Context) {
	f := s.f
	if !f.available() {
		return
	}
	if len(f.updatable()) > 0 {
		frostui.Muted(c, "One update downloads every feature that is on. Changes apply after a reboot.").Wrap()
	}
	var waiting []string
	for _, row := range f.features {
		if status, ok := f.status(row.Name); ok && status.WaitingForImage {
			waiting = append(waiting, row.Name)
		}
	}
	if len(waiting) > 0 {
		frostui.Toned(c, "Waiting for a newer OS image: "+strings.Join(waiting, ", "), frostui.Warning).Wrap()
	}
}
