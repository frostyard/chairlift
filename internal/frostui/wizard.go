package frostui

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// HeroSize is the side of a Hero's icon tile, in DIPs.
const HeroSize = 64

// HeroIcon is what tops a Hero: a named icon on a tile in a tone (the
// accent for Neutral), the app's logo on the accent, or a spinner while
// busy. Logo wins over Busy, Busy over Name.
type HeroIcon struct {
	Name string
	Tone Tone
	Logo *ui.SVG
	Busy bool
}

// Hero is the centered top of a wizard's step or of a page that says one
// thing: a large icon, a title and a description under it.
func Hero(c *ui.Context, icon HeroIcon, title, description string) ui.Element {
	t := c.Theme()
	return ui.Column(c).Gap(10).AlignItems(ui.Center).Children(func() {
		switch {
		case icon.Logo != nil:
			ui.Box(c).Size(HeroSize, HeroSize).Radius(18).Background(t.Accent).Center().Children(func() {
				ui.Icon(c, icon.Logo).FontSize(38).TextColor(t.AccentText)
			})
		case icon.Busy:
			ui.Box(c).Size(HeroSize, HeroSize).Radius(18).Background(ToneFill(t, Accent)).Center().Children(func() {
				ui.Spinner(c).Size(30, 30).Label(title)
			})
		default:
			tone := icon.Tone
			if tone == Neutral {
				tone = Accent
			}
			IconTile(c, icon.Name, tone, HeroSize)
		}
		ui.Text(c, title).FontSize(24).Bold().TextAlign(ui.Center).Role(ui.RoleHeading)
		if description != "" {
			ui.Text(c, description).FontSize(13.5).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(500)
		}
	})
}

// StepDots is a wizard's step indicator: one dot per step, the current one
// (counted from 0) drawn long in the accent, the steps before it in a
// lighter accent. Screen readers hear it as a progress, "2 / 5".
func StepDots(c *ui.Context, n, current int) ui.Element {
	t := c.Theme()
	return ui.Row(c).Gap(6).Justify(ui.Center).AlignItems(ui.Center).Role(ui.RoleProgress).
		Range(1, float64(n), float64(current+1)).Label(fmt.Sprintf("%d / %d", current+1, n)).
		Children(func() {
			for i := range n {
				dot := ui.Box(c.Key(i)).Height(6).Radius(3)
				switch {
				case i == current:
					dot.Width(22).Background(t.Accent)
				case i < current:
					dot.Width(6).Background(t.Accent.Alpha(0.45))
				default:
					dot.Width(6).Background(t.Border)
				}
			}
		})
}
