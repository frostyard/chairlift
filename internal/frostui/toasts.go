package frostui

import "github.com/egoist/mygo/ui"

// ToastWidth is the widest a TopToasts toast grows, in DIPs.
const ToastWidth = 640

// TopToasts gives a window's toasts their look: at the top of the window,
// centered, each with a close button named closeLabel (translated by the
// app). MyGo's own toasts sit at the bottom, where they cover a wizard's
// Back and Next, and have no close button, so a toast that stays until
// closed would stay for good; resting the pointer on a toast also stops
// its time. A toast of Type "error" leads with a warning icon. Plain text
// only (the Title). Call it once per frame, as ui.ToastViewportBase.
func TopToasts(c *ui.Context, closeLabel string) {
	t := c.Theme()
	fill, text := t.Inverse, t.InverseText
	if fill.A == 0 {
		fill, text = t.Text, t.Background
	}
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Justify(ui.Start).AlignItems(ui.Center).Padding(SpaceM, SpaceXL).Gap(SpaceS)
		for _, toast := range toasts {
			p := ui.ToastBase(c, toast)
			p.Root.Row().Gap(SpaceM).Padding(8, 8, 8, 16).Radius(10).MaxWidth(ToastWidth).AlignItems(ui.Center).
				Background(fill).TextColor(text).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.25))
			p.Root.Children(func() {
				if toast.Type == "error" {
					Icon(c, "dialog-warning").TextColor(t.Warning)
				}
				ui.Text(c, toast.Title).FontSize(13).Grow(1).Shrink(1)
				p.CloseButton().Label(closeLabel).Tooltip(closeLabel).Padding(6).Radius(6).Children(func() {
					Icon(c, "window-close").TextColor(text)
				})
			})
		}
	})
}
