package frostui

import "github.com/egoist/mygo/ui"

// Title is a page title.
func Title(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(26).Bold()
}

// Heading heads a card or section.
func Heading(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(14.5).Bold()
}

// Strong is text that stands out in a row: a name.
func Strong(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(13.5).Bold()
}

// Body is ordinary text.
func Body(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(13.5)
}

// Muted is secondary text.
func Muted(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(12.5).TextColor(c.Theme().TextMuted)
}

// Caption is small secondary text, as a label above a value.
func Caption(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(11.5).TextColor(c.Theme().TextMuted)
}

// Mono is text in the monospace font: versions, IDs, commands.
func Mono(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).Font("monospace").FontSize(12.5)
}

// Toned is text in a tone's color.
func Toned(c *ui.Context, s string, tone Tone) ui.Element {
	return ui.Text(c, s).FontSize(12.5).TextColor(ToneColor(c.Theme(), tone))
}
