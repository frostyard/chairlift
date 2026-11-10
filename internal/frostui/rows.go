package frostui

import (
	"github.com/egoist/mygo/ui"
)

// RowStatus is what JobStatus shows at the end of a row: a job that waits
// or runs, how it ended, or a note.
type RowStatus struct {
	// State picks the look: StepQueued and StepRunning show a spinner,
	// StepDone a check in the success color, StepFailed an error icon in
	// the danger color, and any other state Text alone, muted.
	State StepState
	// Text says the state in a word or two: "Queued", "Updating",
	// "Cleaned".
	Text string
	// Label names the spinner, and a compact failure's icon, for screen
	// readers, as "Updating Firefox": Text when empty.
	Label string
	// Tooltip says more on hover: when it ended, or the error.
	Tooltip string
	// Compact fits a dense row: a running job's text as a caption, a job
	// that worked as a pill, and a failure as its icon alone, which leaves
	// room for the row's button after it.
	Compact bool
}

// JobStatus draws s into the row it is in, at the end of a row of a list,
// before or instead of the row's button: its parts follow the row's gap.
// It draws nothing for a note without text.
func JobStatus(c *ui.Context, s RowStatus) {
	t := c.Theme()
	label := s.Label
	if label == "" {
		label = s.Text
	}
	switch s.State {
	case StepQueued, StepRunning:
		ui.Spinner(c).Label(label)
		if s.Compact {
			Caption(c, s.Text).SingleLine()
		} else {
			Muted(c, s.Text).SingleLine()
		}
	case StepDone:
		if s.Compact {
			IconPill(c, "object-select", s.Text, Success).Tooltip(s.Tooltip)
			return
		}
		Icon(c, "object-select").TextColor(t.Success)
		Toned(c, s.Text, Success).SingleLine().Tooltip(s.Tooltip)
	case StepFailed:
		if s.Compact {
			Icon(c, "dialog-error").TextColor(t.Danger).Label(label).Tooltip(s.Tooltip)
			return
		}
		Icon(c, "dialog-error").TextColor(t.Danger)
		Toned(c, s.Text, Danger).SingleLine().Tooltip(s.Tooltip)
	default:
		switch {
		case s.Text == "":
		case s.Compact:
			Caption(c, s.Text).SingleLine().Tooltip(s.Tooltip)
		default:
			Muted(c, s.Text).SingleLine().Tooltip(s.Tooltip)
		}
	}
}

// VersionChange shows a change of version, "1.2 → 1.3": the old version
// muted and the new one, both in monospace. Either may be empty; the arrow
// shows only between two.
func VersionChange(c *ui.Context, from, to string) ui.Element {
	t := c.Theme()
	return ui.Row(c).Gap(6).AlignItems(ui.Center).Justify(ui.End).Children(func() {
		if from != "" {
			Mono(c, from).TextColor(t.TextMuted).SingleLine()
		}
		if from != "" && to != "" {
			ui.Text(c, "→").FontSize(12.5).TextColor(t.TextMuted)
		}
		if to != "" {
			Mono(c, to).SingleLine()
		}
	})
}

// CommandItem is what a CommandRow shows.
type CommandItem struct {
	Icon, Title string
	// Command is the command line, as it runs.
	Command string
	// Admin shows an "admin" pill after the title: the command asks for
	// administrator authentication.
	Admin bool
	// Status draws the command's state (JobStatus) in a cell before the
	// action; Action draws its button. Either may be nil.
	Status, Action func()
}

// CommandRow is a row for a command the app runs on request: an icon tile,
// the title with its command line under it in monospace, the command's
// state and its button. Rows of a list line their state and buttons up in
// cells of the same width.
func CommandRow(c *ui.Context, it CommandItem) ui.Element {
	t := c.Theme()
	return ui.Row(c).Gap(SpaceM).Padding(2, 0).AlignItems(ui.Center).Children(func() {
		IconTile(c, it.Icon, Neutral, 32)
		ui.Column(c).Grow(1).Basis(0).Gap(3).Children(func() {
			ui.Row(c).Gap(SpaceS).AlignItems(ui.Center).Children(func() {
				ui.Text(c, it.Title).FontSize(13.5).FontWeight(600).SingleLine().Shrink(1)
				if it.Admin {
					IconPill(c, "changes-prevent", "admin", Neutral).
						Tooltip("Asks for administrator authentication")
				}
			})
			Mono(c, it.Command).FontSize(11.5).TextColor(t.TextMuted).SingleLine().Selectable()
		})
		ui.Row(c).Width(132).Gap(6).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			if it.Status != nil {
				it.Status()
			}
		})
		ui.Row(c).Width(72).Justify(ui.End).Children(func() {
			if it.Action != nil {
				it.Action()
			}
		})
	})
}

// Tile is what a StepTile shows.
type Tile struct {
	Icon string
	// Caption names the tile's place above its value: "Booted · running
	// now".
	Caption string
	// Value is the tile's value, in monospace: a version.
	Value string
	// Detail is a smaller monospaced line under the value, as a short
	// digest, and DetailTooltip what hovering it shows: the digest in full.
	Detail, DetailTooltip string
	// Pill is a neutral pill at the end, with PillIcon before its text:
	// "pinned".
	Pill, PillIcon string
	// Current highlights the tile in the accent: the one in use.
	Current bool
	// Dim fades the tile: an empty place.
	Dim bool
}

// StepTile is one tile of a short chain of values read top to bottom, as
// a system's deployments (staged, booted, rollback): an icon, a caption
// over the value and a smaller line under it. The current one is in the
// accent, an empty one dimmed.
func StepTile(c *ui.Context, s Tile) ui.Element {
	t := c.Theme()
	bg, border, iconColor := t.Background, t.Border, t.TextMuted
	if s.Current {
		bg, border, iconColor = t.Accent.Alpha(0.14), t.Accent, t.Accent
	}
	row := ui.Row(c).Gap(SpaceM).Padding(9, 14).Radius(10).Background(bg).Border(1, border).AlignItems(ui.Center)
	if s.Dim {
		row.Opacity(0.6)
	}
	return row.Children(func() {
		Icon(c, s.Icon).TextColor(iconColor)
		ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
			Caption(c, s.Caption).SingleLine()
			Mono(c, s.Value).FontSize(13).SingleLine().Selectable()
			if s.Detail != "" {
				Mono(c, s.Detail).FontSize(11).TextColor(t.TextMuted).SingleLine().Selectable().Tooltip(s.DetailTooltip)
			}
		})
		if s.Pill != "" {
			IconPill(c, s.PillIcon, s.Pill, Neutral)
		}
	})
}

// SwitchRow is a setting: its title, an optional line under it, and a
// switch at the end, labelled with the title. Without a subtitle the
// title is plain body text; with one it is bold over the muted subtitle.
// It returns the switch, for Disabled, Tooltip and Changed. Key it with
// c.Key, which keys the row.
func SwitchRow(c *ui.Context, title, subtitle string, on *bool) ui.Element {
	var sw ui.Element
	ui.Row(c).Gap(SpaceM).AlignItems(ui.Center).Children(func() {
		if subtitle == "" {
			Body(c, title).Grow(1)
		} else {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				Strong(c, title)
				Muted(c, subtitle)
			})
		}
		sw = ui.Switch(c, on).Label(title)
	})
	return sw
}

// Disclosure is what a DisclosureCard shows.
type Disclosure struct {
	Icon            string
	Title, Subtitle string
	// Trailing draws at the end of the head, before the chevron: a count
	// pill.
	Trailing func()
	// Body draws the open card's content, under a hairline.
	Body func()
}

// DisclosureCard is a card whose head opens and closes it: an icon, a
// title over a subtitle, Trailing, and a chevron; open, Body shows under
// the head. *open says whether it is open; a click on the head flips it,
// and the card shows the new state in the same frame. It reports the
// click. Key it with c.Key when it is one of a list.
func DisclosureCard(c *ui.Context, open *bool, d Disclosure) (toggled bool) {
	t := c.Theme()
	border := t.Border
	if *open {
		border = t.Border.Mix(t.TextMuted, 0.35)
	}
	ui.Column(c).Radius(RadiusCard).Background(Panel(t)).Border(1, border).AlignItems(ui.Stretch).Clip().Children(func() {
		head := ui.ButtonBase(c).Padding(12, 16).Gap(SpaceM).AlignItems(ui.Center).
			Label(d.Title).Description(d.Subtitle).Expanded(*open).Cursor(ui.CursorPointer)
		if head.Hovered() {
			head.Background(t.SurfaceHover.Alpha(0.5))
		}
		if head.Clicked() {
			*open = !*open
			toggled = true
		}
		head.Children(func() {
			Icon(c, d.Icon).FontSize(18).TextColor(t.TextMuted)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				Strong(c, d.Title).SingleLine()
				Muted(c, d.Subtitle).SingleLine()
			})
			if d.Trailing != nil {
				d.Trailing()
			}
			chevron := "pan-down"
			if *open {
				chevron = "pan-up"
			}
			Icon(c, chevron).TextColor(t.TextMuted)
		})
		if *open && d.Body != nil {
			RowDivider(c)
			ui.Column(c).Padding(14, 16).Gap(SpaceM).AlignItems(ui.Stretch).Children(d.Body)
		}
	})
	return toggled
}

// TileGrid lays tiles out in rows that wrap, as ToggleTiles: chain
// Children to fill it.
func TileGrid(c *ui.Context) ui.Element {
	return ui.Row(c).Wrap().Gap(SpaceM).AlignItems(ui.Start)
}
