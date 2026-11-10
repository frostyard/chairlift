package frostui

import (
	"github.com/egoist/mygo/ui"
)

// ConsoleView is the console pane: every command the app ran and its
// output, as it ran, with filters by level and text, follow, copy and
// clear.
type ConsoleView struct {
	Open   bool
	Query  string
	Levels LevelSet
	// Subtitle follows the pane's title; empty shows "every command the
	// app runs, as it runs".
	Subtitle string
	list     ui.ListState
	init     bool
}

// View draws the pane for con.
func (v *ConsoleView) View(c *ui.Context, con *Console) {
	if !v.init {
		v.init = true
		if v.Levels == 0 {
			v.Levels = DefaultLevels
		}
		v.list.FollowEnd = true
	}
	t := c.Theme()
	lines := con.Lines(&v.Levels, v.Query)
	ui.Column(c).Fill().Background(Chrome(t)).AlignItems(ui.Stretch).Role(ui.RoleRegion).Label("Console").Children(func() {
		ui.Row(c).Gap(SpaceS).Padding(SpaceS, SpaceL).AlignItems(ui.Center).Children(func() {
			Icon(c, "utilities-terminal").TextColor(t.TextMuted)
			ui.Text(c, "Console").Bold().FontSize(13)
			sub := v.Subtitle
			if sub == "" {
				sub = "every command the app runs, as it runs"
			}
			Muted(c, sub).SingleLine()
			ui.Spacer(c)
			ui.ToggleGroup(c, func() {
				for l := Level(0); l < levelCount; l++ {
					on := v.Levels.Has(l)
					if ui.Toggle(c.Key(int(l)), &on, LevelNames[l]).Changed() {
						v.Levels.Toggle(l)
					}
				}
			}).Label("Levels")
			ui.SearchField(c, &v.Query).Label("Filter lines").Placeholder("Filter lines").Width(200)
			if b := IconButton(c, "go-bottom", "Follow the newest line").Selected(v.list.FollowEnd); b.Clicked() {
				v.list.FollowEnd = !v.list.FollowEnd
				if v.list.FollowEnd {
					v.list.ScrollToEnd()
				}
			}
			if IconButton(c, "edit-copy", "Copy the lines shown").Clicked() {
				c.WriteClipboard(Text(lines))
				c.Announce("Console copied")
			}
			if IconButton(c, "edit-clear-all", "Clear the console").Clicked() {
				con.Clear()
			}
			if IconButton(c, "window-close", "Close the console").Clicked() {
				v.Open = false
			}
		})
		ui.Box(c).Height(1).Background(t.Border)
		if len(lines) == 0 {
			Empty(c, "utilities-terminal", "Nothing yet", "Commands and their output show here as they run.").Grow(1)
			return
		}
		v.list.Key = func(i int) any { return lines[i].Seq }
		v.list.Label = func(i int) string { return lines[i].Text }
		ui.List(c, &v.list, len(lines), func(i int) {
			l := lines[i]
			color := t.Text
			switch l.Level {
			case LevelDebug, LevelOutput:
				color = t.Text.Alpha(0.82)
			case LevelCommand, LevelInfo:
				color = t.Accent
			case LevelDryRun, LevelWarn:
				color = t.Warning
			case LevelError:
				color = t.Danger
			}
			ui.Row(c).Gap(SpaceM).Padding(1, SpaceL).AlignItems(ui.Start).Children(func() {
				ui.Text(c, l.At.Format("15:04:05")).Font("monospace").FontSize(12).TextColor(t.TextMuted).SingleLine()
				ui.Text(c, LevelNames[l.Level]).Font("monospace").FontSize(12).TextColor(color).Width(64).SingleLine()
				text := l.Text
				if l.Level == LevelCommand {
					text = "$ " + text
				}
				ui.Text(c, text).Font("monospace").FontSize(12).TextColor(color).Wrap().Grow(1).Selectable()
			})
		}).Grow(1).Padding(SpaceXS, 0)
	})
}
