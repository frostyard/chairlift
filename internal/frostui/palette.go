package frostui

import (
	"sort"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
)

// Command is one entry of the command palette: an action the app offers,
// or a place to go.
type Command struct {
	// ID identifies the command; it must be unique.
	ID string
	// Title is what the palette lists and matches, as "Update everything".
	Title string
	// Group heads the commands of one kind, as "Actions" or "Go to".
	Group string
	// Icon is a freedesktop icon name.
	Icon string
	// Keys is the shortcut that also runs it, as {"Ctrl", "U"}.
	Keys []string
	// Hint is matched as well as the title, but not shown: other words for
	// the command.
	Hint string
	// Disabled commands are listed, dimmed, and do not run; Reason says
	// why.
	Disabled bool
	Reason   string
	// Run runs on the UI thread after the palette closes.
	Run func()
}

// Match scores how well query matches text: every rune of query must
// appear in text in order, ignoring case. A higher score is a better
// match; runs of consecutive runes, the start of words and a match at the
// start count more. ok is false when query does not match.
func Match(query, text string) (score int, ok bool) {
	q := []rune(strings.ToLower(strings.TrimSpace(query)))
	if len(q) == 0 {
		return 0, true
	}
	t := []rune(strings.ToLower(text))
	qi, run := 0, 0
	for ti := 0; ti < len(t) && qi < len(q); ti++ {
		if t[ti] != q[qi] {
			run = 0
			continue
		}
		score++
		if ti == 0 {
			score += 8
		} else if !unicode.IsLetter(t[ti-1]) && !unicode.IsDigit(t[ti-1]) {
			score += 5
		}
		if run > 0 {
			score += 3 * run
		}
		run++
		qi++
	}
	if qi < len(q) {
		return 0, false
	}
	// shorter texts win among equals
	score -= len(t) / 16
	return score, true
}

// Filter returns the commands that match query, best first. With an empty
// query it returns every command in its order, grouped.
func Filter(cmds []Command, query string) []Command {
	if strings.TrimSpace(query) == "" {
		out := append([]Command(nil), cmds...)
		order := map[string]int{}
		for _, c := range cmds {
			if _, ok := order[c.Group]; !ok {
				order[c.Group] = len(order)
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return order[out[i].Group] < order[out[j].Group] })
		return out
	}
	type scored struct {
		cmd   Command
		score int
	}
	var hits []scored
	for _, c := range cmds {
		best, ok := Match(query, c.Title)
		if s, hok := Match(query, c.Hint); hok && c.Hint != "" && (!ok || s-4 > best) {
			best, ok = s-4, true
		}
		if ok {
			hits = append(hits, scored{c, best})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := make([]Command, len(hits))
	for i, h := range hits {
		out[i] = h.cmd
	}
	return out
}

// Palette is the command palette: a dialog with a field that filters the
// app's commands, which Up, Down and Enter choose and run.
type Palette struct {
	Open     bool
	Query    string
	Selected int
	list     ui.ListState
	pending  func()
}

// Show opens the palette with an empty query.
func (p *Palette) Show() {
	p.Open, p.Query, p.Selected = true, "", 0
}

// RunPending runs the command chosen in the last frame, once the palette
// closed. The shell calls it before it builds the page, so a command that
// changes the page or asks a question shows in the same frame.
func (p *Palette) RunPending() {
	if run := p.pending; run != nil && !p.Open {
		p.pending = nil
		run()
	}
}

// View draws the palette while it is open, and runs a chosen command
// after it closes (RunPending, when the shell has not already).
func (p *Palette) View(c *ui.Context, cmds []Command) {
	p.RunPending()
	if !p.Open {
		return
	}
	t := c.Theme()
	shown := Filter(cmds, p.Query)
	if p.Selected >= len(shown) {
		p.Selected = len(shown) - 1
	}
	if p.Selected < 0 {
		p.Selected = 0
	}
	choose := func(i int) {
		if i < 0 || i >= len(shown) || shown[i].Disabled || shown[i].Run == nil {
			return
		}
		p.pending = shown[i].Run
		p.Open = false
	}
	ui.Modal(c, &p.Open, func() {
		ui.Column(c).Width(620).Height(440).Gap(0).AlignItems(ui.Stretch).Children(func() {
			field := ui.SearchField(c, &p.Query).Label("Command").Placeholder("Type a command or search").AutoFocus()
			if field.Changed() {
				p.Selected = 0
			}
			field.OnShortcut(0, ui.KeyDown, func() {
				if p.Selected < len(shown)-1 {
					p.Selected++
					p.list.ScrollIntoView(p.Selected)
				}
			})
			field.OnShortcut(0, ui.KeyUp, func() {
				if p.Selected > 0 {
					p.Selected--
					p.list.ScrollIntoView(p.Selected)
				}
			})
			if field.Submitted() {
				choose(p.Selected)
			}
			ui.Box(c).Height(SpaceS)
			if len(shown) == 0 {
				Empty(c, "edit-find", "No command matches", "")
				return
			}
			p.list.Key = func(i int) any { return shown[i].ID }
			ui.List(c, &p.list, len(shown), func(i int) {
				cmd := shown[i]
				header := i == 0 || shown[i-1].Group != cmd.Group
				ui.Column(c).AlignItems(ui.Stretch).Children(func() {
					if header && cmd.Group != "" {
						Caption(c, cmd.Group).Padding(8, 10, 4, 10)
					}
					row := ui.ButtonBase(c).Row().Gap(SpaceM).Padding(8, 10).Radius(RadiusChip).AlignItems(ui.Center).
						Cursor(ui.CursorPointer)
					if i == p.Selected {
						row.Background(t.Accent.Alpha(0.16))
					} else if row.Hovered() {
						row.Background(t.SurfaceHover)
					}
					if cmd.Disabled {
						row.Opacity(0.5)
					}
					if row.Clicked() {
						p.Selected = i
						choose(i)
					}
					row.Children(func() {
						Icon(c, cmd.Icon).TextColor(t.TextMuted)
						ui.Text(c, cmd.Title).FontSize(13.5).Grow(1).SingleLine()
						if cmd.Disabled && cmd.Reason != "" {
							Caption(c, cmd.Reason)
						}
						if len(cmd.Keys) > 0 {
							Shortcut(c, cmd.Keys...)
						}
					})
				})
			}).Grow(1)
			ui.Row(c).Gap(SpaceM).Padding(10, 6, 0, 6).AlignItems(ui.Center).Children(func() {
				Shortcut(c, "↑", "↓")
				Caption(c, "Navigate")
				Shortcut(c, "Enter")
				Caption(c, "Run")
				Shortcut(c, "Esc")
				Caption(c, "Close")
			})
		})
	})
}
