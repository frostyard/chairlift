package frostui

import (
	"github.com/egoist/mygo/ui"
)

// NavItem is one page of the left rail.
type NavItem struct {
	ID, Title, Icon string
	// Badge is a count shown at the end of the item, "" for none.
	Badge     string
	BadgeTone Tone
	// Shortcut names the keys that open the page, for its tooltip.
	Shortcut string
}

// NavSection is a titled group of rail items.
type NavSection struct {
	Title string
	Items []NavItem
}

// StatusItem is one entry of the status bar.
type StatusItem struct {
	Icon, Text string
	Tone       Tone
	// Tooltip explains the item; it also names it for screen readers.
	Tooltip string
	// OnClick makes the item a button.
	OnClick func()
	// Progress, 0..1, draws a small bar after the text; a negative value
	// draws none.
	Progress float64
}

// Brand is the app's identity at the top of the rail.
type Brand struct {
	Name, Tagline string
	// Logo is drawn white on the accent.
	Logo *ui.SVG
}

// Shell is the command-center window: a top bar with the brand, a field
// that opens the command palette and the app's actions; a left rail of
// pages with a footer; the page; an optional console under it; and a
// status bar. Keep one Shell in the app's state and call View from the
// root view every frame.
type Shell struct {
	Brand    Brand
	Selected string
	Palette  Palette
	Console  ConsoleView
	// Dialogs are the confirmations the shell draws; share one with the
	// app's pages. Nil draws none.
	Dialogs *Dialogs
	Info    Info

	splitPercent float32
}

// Frame is what the app gives the shell to draw each frame.
type Frame struct {
	Nav []NavSection
	// Actions draws the top bar's buttons (IconButton) before the menu.
	Actions func()
	// Menu builds the top bar's menu, nil for none.
	Menu func(m *ui.Menu)
	// RailFooter draws under the rail's pages: the running job, the host.
	RailFooter func()
	// Content draws the selected page.
	Content func()
	// Commands are the palette's commands.
	Commands []Command
	// Status and StatusEnd are the status bar's items at its start and
	// at its end.
	Status, StatusEnd []StatusItem
	// ConsoleData is the console the console pane shows, nil for no
	// console.
	ConsoleData *Console
}

// View draws the shell.
func (s *Shell) View(c *ui.Context, f Frame) {
	t := Adapt(c)
	s.Palette.RunPending()
	if s.splitPercent == 0 {
		s.splitPercent = 64
	}
	c.OnWindowShortcut(ui.Cmd, ui.KeyK, func() { s.Palette.Show() })
	if f.ConsoleData != nil {
		c.OnWindowShortcut(ui.Cmd, ui.KeyJ, func() { s.Console.Open = !s.Console.Open })
	}
	ui.Column(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		s.topBar(c, f)
		ui.Box(c).Height(1).Background(t.Border)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			s.rail(c, f)
			ui.Box(c).Width(1).Background(t.Border)
			ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				content := func() {
					ui.Scroll(c).Grow(1).Fill().Padding(SpaceXL, 28).Gap(18).Children(func() {
						ui.Column(c).Gap(18).AlignItems(ui.Stretch).MaxWidth(1320).Children(f.Content)
					})
				}
				if s.Console.Open && f.ConsoleData != nil {
					ui.SplitVerticalPercent(c, &s.splitPercent, content, func() { s.Console.View(c, f.ConsoleData) }).Grow(1)
				} else {
					content()
				}
			})
		})
		ui.Box(c).Height(1).Background(t.Border)
		s.statusBar(c, f)
	})
	if s.Dialogs != nil {
		s.Dialogs.View(c)
	}
	s.Info.View(c)
	s.Palette.View(c, f.Commands)
}

func (s *Shell) topBar(c *ui.Context, f Frame) {
	t := c.Theme()
	tb := c.TitleBar()
	ui.Row(c).Height(TopBarHeight).Padding(0, SpaceL+tb.Right, 0, SpaceL+tb.Left).Gap(SpaceM).AlignItems(ui.Center).
		Background(Chrome(t)).DragWindow().Children(func() {
		ui.Row(c).Width(RailWidth - SpaceL).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(30, 30).Radius(8).Background(t.Accent).Center().Children(func() {
				if s.Brand.Logo != nil {
					ui.Icon(c, s.Brand.Logo).FontSize(19).TextColor(t.AccentText)
				}
			})
			ui.Column(c).Children(func() {
				ui.Text(c, s.Brand.Name).Bold().SingleLine()
				if s.Brand.Tagline != "" {
					Caption(c, s.Brand.Tagline).SingleLine()
				}
			})
		})
		ui.Spacer(c)
		// Looks like a search field and opens the palette.
		field := ui.Row(c).Width(380).Height(32).Padding(0, 10).Gap(SpaceS).Radius(16).AlignItems(ui.Center).
			Background(t.Background).Border(1, t.Border).Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).
			Label("Search or run a command").Tooltip("Command palette (Ctrl K)")
		if field.Hovered() {
			field.Border(1, t.Accent.Alpha(0.6))
		}
		if field.Clicked() {
			s.Palette.Show()
		}
		field.Children(func() {
			Icon(c, "system-search").TextColor(t.TextMuted)
			Muted(c, "Search or run a command").Grow(1).SingleLine()
			Shortcut(c, "Ctrl", "K")
		})
		ui.Spacer(c)
		ui.Row(c).Gap(SpaceXS).AlignItems(ui.Center).Children(func() {
			if f.Actions != nil {
				f.Actions()
			}
			if f.ConsoleData != nil {
				b := IconButton(c, "utilities-terminal", "Console (Ctrl J)").Selected(s.Console.Open)
				if b.Clicked() {
					s.Console.Open = !s.Console.Open
				}
			}
			if f.Menu != nil {
				ui.MenuButton(c, "", f.Menu).Label("Main menu").Tooltip("Main menu").Children(func() {
					Icon(c, "open-menu")
				})
			}
		})
	})
}

func (s *Shell) rail(c *ui.Context, f Frame) {
	t := c.Theme()
	ui.Column(c).Width(RailWidth).AlignItems(ui.Stretch).Background(Chrome(t)).Children(func() {
		selected := s.Selected
		ui.Sidebar(c, &selected, func() {
			for si, sec := range f.Nav {
				ui.SidebarSection(c.Key(si), sec.Title, nil, func() {
					for _, item := range sec.Items {
						it := ui.SidebarItem(c.Key(item.ID), item.ID, IconSVG(item.Icon), item.Title)
						if item.Shortcut != "" {
							it.Tooltip(item.Title + " (" + item.Shortcut + ")")
						}
						if item.Badge != "" {
							it.Children(func() {
								Pill(c, item.Badge, item.BadgeTone)
							})
						}
					}
				})
			}
		}).Grow(1).Padding(SpaceS, SpaceS).Background(Chrome(t))
		s.Selected = selected
		if f.RailFooter != nil {
			ui.Column(c).Gap(SpaceM).Padding(SpaceM, SpaceL, SpaceL, SpaceL).AlignItems(ui.Stretch).Children(f.RailFooter)
		}
	})
}

func (s *Shell) statusBar(c *ui.Context, f Frame) {
	t := c.Theme()
	ui.Row(c).Height(28).Padding(0, SpaceM).Gap(SpaceL).AlignItems(ui.Center).Background(Chrome(t)).Role(ui.RoleStatus).Children(func() {
		for i, it := range f.Status {
			statusItem(c.Key([2]any{"s", i}), it)
		}
		ui.Spacer(c)
		for i, it := range f.StatusEnd {
			statusItem(c.Key([2]any{"e", i}), it)
		}
		if f.ConsoleData != nil {
			statusItem(c.Key("console"), StatusItem{Icon: "utilities-terminal", Text: "Console", Tooltip: "Show or hide the console (Ctrl J)", Progress: -1,
				OnClick: func() { s.Console.Open = !s.Console.Open }})
		}
	})
}

func statusItem(c *ui.Context, it StatusItem) {
	t := c.Theme()
	color := t.TextMuted
	if it.Tone != Neutral {
		color = ToneColor(t, it.Tone)
	}
	row := ui.Row(c).Gap(6).Padding(2, 4).Radius(4).AlignItems(ui.Center)
	if it.Tooltip != "" {
		row.Tooltip(it.Tooltip)
	}
	if it.OnClick != nil {
		label := it.Text
		if it.Tooltip != "" {
			label = it.Tooltip
		}
		row.Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).Label(label)
		if row.Hovered() {
			row.Background(t.SurfaceHover)
		}
		if row.Clicked() {
			it.OnClick()
		}
	}
	row.Children(func() {
		if it.Icon != "" {
			Icon(c, it.Icon).FontSize(12).TextColor(color)
		}
		ui.Text(c, it.Text).FontSize(12).TextColor(color).SingleLine()
		if it.Progress >= 0 && it.Progress <= 1 && it.Progress != 0 {
			ui.Progress(c, it.Progress).Width(90).Label(it.Text)
		}
	})
}
