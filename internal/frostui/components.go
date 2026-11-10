package frostui

import (
	"hash/fnv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// Card is a panel: a column with padding, a hairline border and rounded
// corners, on the panel color. Chain Children to fill it.
func Card(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(18).Gap(SpaceM).Radius(RadiusCard).
		Background(Panel(t)).Border(1, t.Border).AlignItems(ui.Stretch)
}

// CardHeader is a card's title row: a heading, an optional subtitle under
// it, and trailing controls at the end.
func CardHeader(c *ui.Context, title, subtitle string, trailing func()) {
	ui.Row(c).Gap(SpaceM).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			Heading(c, title)
			if subtitle != "" {
				Muted(c, subtitle)
			}
		})
		if trailing != nil {
			trailing()
		}
	})
}

// PageHeader is the top of a page: its title, a subtitle that says the
// page's state in one line, and the page's main actions at the end.
func PageHeader(c *ui.Context, title, subtitle string, actions func()) {
	ui.Row(c).Gap(SpaceM).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			Title(c, title).Role(ui.RoleHeading)
			if subtitle != "" {
				Muted(c, subtitle).FontSize(13.5)
			}
		})
		if actions != nil {
			ui.Row(c).Gap(SpaceS).AlignItems(ui.Center).Children(actions)
		}
	})
}

// Pill is a small rounded tag in a tone.
func Pill(c *ui.Context, text string, tone Tone) ui.Element {
	t := c.Theme()
	return ui.Row(c).Padding(2, 8).Radius(RadiusChip).Background(ToneFill(t, tone)).Children(func() {
		ui.Text(c, text).FontSize(11.5).TextColor(ToneColor(t, tone)).SingleLine()
	})
}

// IconPill is a pill that starts with an icon.
func IconPill(c *ui.Context, icon, text string, tone Tone) ui.Element {
	t := c.Theme()
	color := ToneColor(t, tone)
	return ui.Row(c).Gap(5).Padding(2, 8).Radius(RadiusChip).Background(ToneFill(t, tone)).AlignItems(ui.Center).Children(func() {
		Icon(c, icon).TextColor(color).FontSize(12)
		ui.Text(c, text).FontSize(11.5).TextColor(color).SingleLine()
	})
}

// ColorPill is a pill in a color of the app's own rather than a tone, as
// one color per kind of thing. Pass the color from the page's theme: under
// SoftSelection the accent of c's theme is a tint.
func ColorPill(c *ui.Context, text string, color ui.Color) ui.Element {
	return ui.Row(c).Padding(2, 8).Radius(RadiusChip).Background(color.Alpha(0.16)).Children(func() {
		ui.Text(c, text).FontSize(11.5).TextColor(color).SingleLine()
	})
}

// DotPill is a pill that starts with a dot in its color, as a state that
// waits for the user: "Update".
func DotPill(c *ui.Context, text string, color ui.Color) ui.Element {
	return ui.Row(c).Gap(5).Padding(2, 8).Radius(RadiusChip).Background(color.Alpha(0.16)).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(color)
		ui.Text(c, text).FontSize(11.5).TextColor(color).SingleLine()
	})
}

// SquareAvatar is a rounded square with the initials of name on a color
// the name picks, the same color for the same name in every frame: an app
// or a package without an icon of its own. size is the square's side.
func SquareAvatar(c *ui.Context, name string, size float32) ui.Element {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(name)))
	hue := float32(h.Sum32() % 360)
	bg, fg := ui.Oklch(0.92, 0.05, hue), ui.Oklch(0.42, 0.12, hue)
	if c.Theme().Dark {
		bg, fg = ui.Oklch(0.36, 0.07, hue), ui.Oklch(0.88, 0.09, hue)
	}
	return ui.Box(c).Size(size, size).Radius(size / 4).Background(bg).Center().Shrink(0).Children(func() {
		ui.Text(c, Initials(name)).FontSize(size * 0.36).Bold().TextColor(fg).SingleLine()
	})
}

// Initials are the first letters of name's first two words, upper case:
// "Mission Center" is "MC", "fzf" is "F", and a name without a letter or
// digit is "?".
func Initials(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var initials []rune
	for _, w := range words {
		if len(initials) == 2 {
			break
		}
		r, _ := utf8.DecodeRuneInString(w)
		initials = append(initials, unicode.ToUpper(r))
	}
	if len(initials) == 0 {
		return "?"
	}
	return string(initials)
}

// Tab is one tab of PillTabs.
type Tab struct {
	Title, Icon string
}

// PillTabs draws tabs as a row of pills, each an icon and a title, the
// chosen one filled with the surface color. *selected is the chosen index;
// a click writes it. It returns the tab list.
func PillTabs(c *ui.Context, selected *int, tabs []Tab) ui.Element {
	t := c.Theme()
	parts := ui.TabsBase(c, selected, len(tabs))
	return parts.List.Gap(SpaceXS).Children(func() {
		for i, tab := range tabs {
			el := parts.Tab(i).Row().Gap(6).Padding(6, 12).Radius(RadiusChip).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			switch {
			case i == *selected:
				el.Background(t.Surface)
			case el.Hovered():
				el.Background(t.SurfaceHover)
			}
			el.Children(func() {
				Icon(c, tab.Icon)
				ui.Text(c, tab.Title).FontSize(13.5).SingleLine()
			})
		}
	})
}

// SoftSelection is t with the accent of chosen rows softened to a tint of
// it on a card, and the text on them left as it is, so the pills on a
// chosen row of a table still show. Draw the table under it with
// ui.Themed, and take the cells' colors from t.
func SoftSelection(t *ui.Theme) *ui.Theme {
	tt := *t
	tint := Panel(t).Mix(t.Accent, 0.2)
	if !t.Dark {
		tint = Panel(t).Mix(t.Accent, 0.14)
	}
	tt.Accent, tt.InactiveAccent, tt.AccentText = tint, tint, t.Text
	return &tt
}

// IconTile is an icon on a rounded square, as rows and cards lead with.
// size is the square's side.
func IconTile(c *ui.Context, icon string, tone Tone, size float32) ui.Element {
	t := c.Theme()
	return ui.Box(c).Size(size, size).Radius(size / 4).Background(ToneFill(t, tone)).Center().Children(func() {
		Icon(c, icon).TextColor(ToneColor(t, tone)).FontSize(size * 0.5)
	})
}

// IconButton is a flat button showing only an icon. label names it for
// screen readers and its tooltip; it must not be empty.
func IconButton(c *ui.Context, icon, label string) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(label).Children(func() {
		Icon(c, icon)
	})
}

// LabeledButton is a button with an icon before its label. primary makes
// it the accent button.
func LabeledButton(c *ui.Context, icon, label string, primary bool) ui.Element {
	var b ui.Element
	if primary {
		b = ui.PrimaryButton(c, "")
	} else {
		b = ui.Button(c, "")
	}
	return b.Label(label).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			Icon(c, icon)
			ui.Text(c, label).SingleLine()
		})
	})
}

// Banner is a full-width notice in a tone, with an optional action and a
// close button. It reports a click on the action and on close.
func Banner(c *ui.Context, tone Tone, icon, text, action string, closable bool) (acted, closed bool) {
	t := c.Theme()
	color := ToneColor(t, tone)
	ui.Row(c).Gap(SpaceM).Padding(SpaceM, SpaceL).Radius(RadiusCard).
		Background(ToneFill(t, tone)).Border(1, color.Alpha(0.35)).AlignItems(ui.Center).Role(ui.RoleAlert).
		Children(func() {
			Icon(c, icon).TextColor(color)
			ui.Text(c, text).FontSize(13).Grow(1).Wrap()
			if action != "" && ui.Button(c, action).Clicked() {
				acted = true
			}
			if closable && IconButton(c, "window-close", "Dismiss").Clicked() {
				closed = true
			}
		})
	return acted, closed
}

// Empty is the placeholder of a list with nothing in it.
func Empty(c *ui.Context, icon, title, body string) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(SpaceXL).Gap(SpaceS).AlignItems(ui.Center).Children(func() {
		Icon(c, icon).FontSize(28).TextColor(t.TextMuted)
		Strong(c, title)
		if body != "" {
			Muted(c, body).Wrap()
		}
	})
}

// Loading is a spinner with a line saying what loads.
func Loading(c *ui.Context, text string) ui.Element {
	return ui.Row(c).Gap(SpaceS).Padding(SpaceM, 0).AlignItems(ui.Center).Children(func() {
		ui.Spinner(c).Label(text)
		Muted(c, text)
	})
}

// Problem is an inline error line in a card.
func Problem(c *ui.Context, text string) ui.Element {
	t := c.Theme()
	return ui.Row(c).Gap(SpaceS).AlignItems(ui.Start).Children(func() {
		Icon(c, "dialog-warning").TextColor(t.Danger)
		ui.Text(c, text).FontSize(12.5).TextColor(t.Danger).Wrap().Grow(1).Selectable()
	})
}

// KeyValue is one line of a KeyValues list.
type KeyValue struct {
	Key, Value string
	// Mono shows the value in the monospace font.
	Mono bool
}

// KeyValuesStyle fits a KeyValues list to its place; its zero value is
// the list of a dialog. A narrow side panel takes a narrower names column
// and smaller values, as an inspector beside a table: KeyWidth 72, Gap 10,
// FontSize 12.5.
type KeyValuesStyle struct {
	// KeyWidth is the width of the names' column: 120 when zero.
	KeyWidth float32
	// Gap is the space between the lines: 6 when zero.
	Gap float32
	// FontSize is the size of the values: 13 when zero. A monospaced value
	// is half a point smaller.
	FontSize float32
}

// KeyValues is a two-column list of names and values. An optional style
// sets its metrics.
func KeyValues(c *ui.Context, rows []KeyValue, style ...KeyValuesStyle) ui.Element {
	s := KeyValuesStyle{KeyWidth: 120, Gap: 6, FontSize: 13}
	if len(style) > 0 {
		if st := style[0]; st.KeyWidth > 0 {
			s.KeyWidth = st.KeyWidth
		}
		if st := style[0]; st.Gap > 0 {
			s.Gap = st.Gap
		}
		if st := style[0]; st.FontSize > 0 {
			s.FontSize = st.FontSize
		}
	}
	return ui.Column(c).Gap(s.Gap).AlignItems(ui.Stretch).Children(func() {
		for i, kv := range rows {
			ui.Row(c.Key(i)).Gap(SpaceM).AlignItems(ui.Start).Children(func() {
				Muted(c, kv.Key).Width(s.KeyWidth).Shrink(0)
				v := ui.Text(c, kv.Value).FontSize(s.FontSize).Grow(1).MinWidth(0).Wrap().Selectable()
				if kv.Mono {
					v.Font("monospace").FontSize(s.FontSize - 0.5)
				}
			})
		}
	})
}

// Figure is one figure of a Figures strip.
type Figure struct {
	Label, Value, Note string
	// Tone colors the note.
	Tone Tone
	// OnClick makes the figure a button, as one that opens its page.
	OnClick func()
}

// Figures is one strip of figures side by side, divided by hairlines.
func Figures(c *ui.Context, figs []Figure) ui.Element {
	t := c.Theme()
	return ui.Row(c).Padding(14, 0).Radius(RadiusCard).Background(Panel(t)).Border(1, t.Border).AlignItems(ui.Stretch).Children(func() {
		for i, f := range figs {
			if i > 0 {
				ui.Box(c.Key(i * 2)).Width(1).Background(t.Border)
			}
			box := ui.Column(c.Key(i*2+1)).Grow(1).Basis(0).Padding(4, 20).Gap(4)
			if f.OnClick != nil {
				box.Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).Label(f.Label + ": " + f.Value)
				if box.Clicked() {
					f.OnClick()
				}
			}
			box.Children(func() {
				Muted(c, f.Label)
				ui.Text(c, f.Value).FontSize(26).Bold().SingleLine()
				if f.Note != "" {
					ui.Text(c, f.Note).FontSize(12).TextColor(ToneColor(t, f.Tone)).SingleLine()
				}
			})
		}
	})
}

// Section is a labelled group inside a page: an icon, a title and a muted
// note on one line (SectionHeader), then fn's content.
func Section(c *ui.Context, icon, title, note string, fn func()) {
	ui.Column(c).Gap(SpaceS).AlignItems(ui.Stretch).Children(func() {
		SectionHeader(c, SectionHead{Icon: icon, Title: title, Note: note})
		fn()
	})
}

// SectionHead is what heads a group of a page.
type SectionHead struct {
	Icon, Title string
	// Note follows the title, muted, on one line.
	Note string
	// Aside is a caption at the end of the line: counts, a status.
	Aside string
	// Quiet sets the title in the muted color too, for a group whose items
	// carry the weight.
	Quiet bool
}

// SectionHeader is the line that heads a group of a page: an icon, a bold
// title, a note after it and an aside at the end. Section draws one over
// its content; a page that lays its group out itself draws it alone.
func SectionHeader(c *ui.Context, h SectionHead) ui.Element {
	t := c.Theme()
	return ui.Row(c).Gap(SpaceS).AlignItems(ui.Center).Children(func() {
		if h.Icon != "" {
			Icon(c, h.Icon).TextColor(t.TextMuted)
		}
		title := ui.Text(c, h.Title).FontSize(13.5).Bold()
		if h.Quiet {
			title.TextColor(t.TextMuted)
		}
		switch {
		case h.Note != "":
			Muted(c, h.Note).Grow(1).SingleLine()
		case h.Aside != "":
			ui.Spacer(c)
		}
		if h.Aside != "" {
			Caption(c, h.Aside).SingleLine()
		}
	})
}

// RowDivider is the hairline between rows of a card.
func RowDivider(c *ui.Context) ui.Element {
	return ui.Box(c).Height(1).Background(c.Theme().Border)
}

// Kbd draws a keyboard key, as in "Ctrl K".
func Kbd(c *ui.Context, key string) ui.Element {
	t := c.Theme()
	return ui.Box(c).Padding(1, 6).Radius(4).Border(1, t.Border).Background(t.Background).Children(func() {
		ui.Text(c, key).FontSize(11).TextColor(t.TextMuted).SingleLine()
	})
}

// Shortcut draws a key combination, as "Ctrl" "K".
func Shortcut(c *ui.Context, keys ...string) ui.Element {
	return ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
		for i, k := range keys {
			Kbd(c.Key(i), k)
		}
	})
}
