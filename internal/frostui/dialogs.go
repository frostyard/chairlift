package frostui

import (
	"github.com/egoist/mygo/ui"
)

// Confirm is a question the app asks before it changes something.
type Confirm struct {
	Title string
	// Body says what will happen; Note adds a line under it, as "Needs
	// administrator authentication."
	Body, Note string
	// Details are shown as key and value lines, as versions.
	Details []KeyValue
	// Action labels the button that confirms ("Turn on"); Cancel the other
	// ("Cancel" when empty).
	Action, Cancel string
	// Destructive colors the action as a danger.
	Destructive bool
	// OnConfirm runs on the UI thread when the user confirms; OnCancel
	// when they do not.
	OnConfirm func()
	OnCancel  func()
}

// Dialogs shows the app's confirmations, one at a time, in the order they
// were asked.
type Dialogs struct {
	queue []Confirm
	open  bool
}

// Ask queues a confirmation.
func (d *Dialogs) Ask(req Confirm) {
	d.queue = append(d.queue, req)
	d.open = true
}

// Showing reports whether a confirmation is up.
func (d *Dialogs) Showing() bool { return len(d.queue) > 0 }

// View draws the first confirmation, if any.
func (d *Dialogs) View(c *ui.Context) {
	if len(d.queue) == 0 {
		return
	}
	req := d.queue[0]
	t := c.Theme()
	decided := false
	confirmed := false
	d.open = true
	ui.Modal(c, &d.open, func() {
		ui.Column(c).Width(440).Gap(SpaceM).AlignItems(ui.Stretch).Role(ui.RoleAlertDialog).Label(req.Title).Children(func() {
			ui.Text(c, req.Title).FontSize(18).Bold().Wrap()
			if req.Body != "" {
				Body(c, req.Body).Wrap().TextColor(t.TextMuted)
			}
			if len(req.Details) > 0 {
				KeyValues(c, req.Details)
			}
			if req.Note != "" {
				ui.Row(c).Gap(SpaceS).AlignItems(ui.Center).Children(func() {
					Icon(c, "dialog-password").TextColor(t.TextMuted)
					Muted(c, req.Note).Wrap()
				})
			}
			ui.Row(c).Gap(SpaceS).Justify(ui.End).Padding(SpaceS, 0, 0, 0).Children(func() {
				cancel := req.Cancel
				if cancel == "" {
					cancel = "Cancel"
				}
				if ui.Button(c, cancel).Clicked() {
					decided = true
				}
				action := ui.PrimaryButton(c, req.Action).AutoFocus()
				if req.Destructive {
					action.Background(t.Danger).TextColor(ui.RGB(255, 255, 255))
				}
				if action.Clicked() {
					decided, confirmed = true, true
				}
			})
		})
	})
	if !d.open || decided {
		d.queue = d.queue[1:]
		d.open = len(d.queue) > 0
		if confirmed {
			if req.OnConfirm != nil {
				req.OnConfirm()
			}
		} else if req.OnCancel != nil {
			req.OnCancel()
		}
	}
}

// CodeDialog asks about text the user should read exactly as it is, as a
// report before it is sent: a title, a summary, the text in a scrollable
// monospaced box with its spaces and lines kept, then Cancel and the
// action. It holds no state: draw it while the app's own state says it is
// open. It reports, in the frame they happen, a click on the action
// (acted) and a cancel: Cancel, Escape or a click outside (canceled).
func CodeDialog(c *ui.Context, title, summary, text, action string) (acted, canceled bool) {
	t := c.Theme()
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(620).Gap(SpaceM).AlignItems(ui.Stretch).Role(ui.RoleAlertDialog).Label(title).Children(func() {
			ui.Text(c, title).FontSize(18).Bold().Wrap()
			if summary != "" {
				Muted(c, summary).Wrap()
			}
			ui.ScrollBoth(c).Height(300).Radius(RadiusChip).Background(t.Background).Border(1, t.Border).Children(func() {
				ui.Text(c, text).Font("monospace").FontSize(12).NoWrap().PreserveSpaces().Selectable().Padding(10, 12)
			})
			ui.Row(c).Gap(SpaceS).Justify(ui.End).Padding(SpaceS, 0, 0, 0).Children(func() {
				if ui.Button(c, "Cancel").AutoFocus().Clicked() {
					canceled = true
				}
				if ui.PrimaryButton(c, action).Clicked() {
					acted = true
				}
			})
		})
	})
	return acted, canceled || !open
}

// Info is a dialog that only shows content and closes: About, keyboard
// shortcuts, help.
type Info struct {
	Open  bool
	title string
	body  func(c *ui.Context)
}

// Show opens the dialog with title and the content body draws.
func (i *Info) Show(title string, body func(c *ui.Context)) {
	i.Open, i.title, i.body = true, title, body
}

// View draws the dialog while it is open.
func (i *Info) View(c *ui.Context) {
	if !i.Open || i.body == nil {
		return
	}
	ui.Modal(c, &i.Open, func() {
		ui.Column(c).Width(460).Gap(SpaceM).AlignItems(ui.Stretch).Role(ui.RoleDialog).Label(i.title).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, i.title).FontSize(18).Bold().Grow(1)
				if IconButton(c, "window-close", "Close").Clicked() {
					i.Open = false
				}
			})
			i.body(c)
		})
	})
}

// ShortcutEntry is one line of a keyboard shortcuts dialog.
type ShortcutEntry struct {
	Group, Title string
	Keys         []string
}

// ShortcutsBody draws shortcut entries grouped, for Info.Show.
func ShortcutsBody(entries []ShortcutEntry) func(c *ui.Context) {
	return func(c *ui.Context) {
		group := ""
		for i, e := range entries {
			if e.Group != group {
				group = e.Group
				Caption(c.Key(i*2), group).Padding(SpaceS, 0, 0, 0)
			}
			ui.Row(c.Key(i*2 + 1)).Gap(SpaceM).AlignItems(ui.Center).Children(func() {
				Body(c, e.Title).Grow(1)
				Shortcut(c, e.Keys...)
			})
		}
	}
}

// About is what an About dialog says.
type About struct {
	Name, Version, Summary string
	Logo                   string // icon name
	Website, Issues        string
	License, Copyright     string
	Developers             []string
}

// AboutBody draws a for Info.Show.
func AboutBody(a About) func(c *ui.Context) {
	return func(c *ui.Context) {
		t := c.Theme()
		ui.Column(c).Gap(SpaceS).AlignItems(ui.Center).Padding(SpaceS, 0).Children(func() {
			if a.Logo != "" {
				ui.Box(c).Size(64, 64).Radius(16).Background(t.Accent).Center().Children(func() {
					Icon(c, a.Logo).FontSize(40).TextColor(t.AccentText)
				})
			}
			ui.Text(c, a.Name).FontSize(22).Bold()
			Pill(c, a.Version, Accent)
			if a.Summary != "" {
				Muted(c, a.Summary).Wrap()
			}
		})
		rows := []KeyValue{}
		if len(a.Developers) > 0 {
			for i, d := range a.Developers {
				key := ""
				if i == 0 {
					key = "Developers"
				}
				rows = append(rows, KeyValue{Key: key, Value: d})
			}
		}
		if a.License != "" {
			rows = append(rows, KeyValue{Key: "License", Value: a.License})
		}
		if a.Copyright != "" {
			rows = append(rows, KeyValue{Key: "Copyright", Value: a.Copyright})
		}
		KeyValues(c, rows)
		ui.Row(c).Gap(SpaceM).Justify(ui.Center).Children(func() {
			if a.Website != "" {
				ui.Link(c, "Website", a.Website)
			}
			if a.Issues != "" {
				ui.Link(c, "Report an issue", a.Issues)
			}
		})
	}
}
