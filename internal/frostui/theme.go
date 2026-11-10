package frostui

import "github.com/egoist/mygo/ui"

// Metrics of the kit, in DIPs. Pages use them instead of numbers of their
// own so that every frostyard app has the same rhythm.
const (
	SpaceXS = 4
	SpaceS  = 8
	SpaceM  = 12
	SpaceL  = 16
	SpaceXL = 24

	// RadiusCard rounds cards and panels; RadiusChip rounds chips, pills
	// and small tiles.
	RadiusCard = 12
	RadiusChip = 8

	// RailWidth is the width of the left rail.
	RailWidth = 232
	// TopBarHeight is the height of the top bar.
	TopBarHeight = 52
)

// Tone is the meaning a color carries.
type Tone int

const (
	// Neutral is the default: muted text on the surface.
	Neutral Tone = iota
	// Accent is the desktop's accent.
	Accent
	// Success is what went well or is on.
	Success
	// Warning is what needs attention.
	Warning
	// Danger is a failure or a destructive action.
	Danger
)

// ToneColor is the foreground color of tone.
func ToneColor(t *ui.Theme, tone Tone) ui.Color {
	switch tone {
	case Accent:
		return t.Accent
	case Success:
		return t.Success
	case Warning:
		return t.Warning
	case Danger:
		return t.Danger
	}
	return t.TextMuted
}

// ToneFill is the soft background behind tone's foreground, as a pill's.
func ToneFill(t *ui.Theme, tone Tone) ui.Color {
	if tone == Neutral {
		return t.Border.Alpha(0.6)
	}
	return ToneColor(t, tone).Alpha(0.16)
}

// Adapt makes MyGo's theme behave as GNOME's and KDE's do and returns it:
// the accent stays in windows without the focus (MyGo fades it to gray, as
// macOS does). The rest still follows the desktop: light or dark, accent,
// contrast and text size. Call it first in the root view.
func Adapt(c *ui.Context) *ui.Theme {
	t := c.Theme()
	if t.InactiveAccent != t.Accent {
		linux := *t
		linux.InactiveAccent = linux.Accent
		c.SetTheme(&linux)
		return &linux
	}
	return t
}

// Chrome is the color of the top bar, rail and status bar: a shade between
// the window's background and the surface of controls.
func Chrome(t *ui.Theme) ui.Color {
	return t.Background.Mix(t.Surface, 0.55)
}

// Panel is the face of cards.
func Panel(t *ui.Theme) ui.Color {
	return t.Background.Mix(t.Surface, 0.7)
}
