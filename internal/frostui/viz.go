package frostui

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// Series is one line of a LiveChart: values from 0 to 100, oldest first.
type Series struct {
	Name   string
	Values []float64
	Tone   Tone
}

// LiveChart draws series as smoothed lines over a soft area on a 0–100
// grid, for figures sampled every second. label names it for screen
// readers.
func LiveChart(c *ui.Context, label string, height float32, series ...Series) ui.Element {
	t := c.Theme()
	return ui.Box(c).Height(height).Label(label).Role(ui.RoleImage).Draw(func(p *ui.Painter, r ui.Rect) {
		for _, f := range []float32{0, 0.25, 0.5, 0.75, 1} {
			y := chart.Y(r, f)
			p.Line(r.X, y, r.X+r.W, y, 1, t.Border.Alpha(0.7))
		}
		for _, s := range series {
			n := len(s.Values)
			if n < 2 {
				continue
			}
			xs := make([]float32, n)
			ys := make([]float32, n)
			for i, v := range s.Values {
				xs[i] = float32(i) / float32(n-1)
				ys[i] = float32(math.Max(0, math.Min(100, v)) / 100)
			}
			color := ToneColor(t, s.Tone)
			p.FillPath(chart.Area(r, xs, nil, ys, chart.CurveMonotone), color.Alpha(0.12))
			p.StrokePath(chart.Line(r, xs, ys, chart.CurveMonotone), 2, color)
		}
	})
}

// Legend draws a colored square and a text, as a chart's key.
func Legend(c *ui.Context, tone Tone, text string) ui.Element {
	t := c.Theme()
	return ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(10, 10).Radius(3).Background(ToneColor(t, tone))
		Muted(c, text)
	})
}

// Gauge draws an open ring filled to value of max, with big text in its
// middle and a caption under it: free space, a share in use.
func Gauge(c *ui.Context, value, maxValue float64, center, caption string, tone Tone, size float32) ui.Element {
	t := c.Theme()
	frac := 0.0
	if maxValue > 0 {
		frac = math.Max(0, math.Min(1, value/maxValue))
	}
	const sweep = 1.5 * math.Pi // three quarters of a circle, open at the bottom
	start := -sweep / 2
	return ui.Box(c).Size(size, size).Role(ui.RoleMeter).Label(caption + ": " + center).Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		outer := min(r.W, r.H) / 2
		inner := outer - size/11
		p.FillPath(chart.Sector(cx, cy, inner, outer, chart.Slice{Start: start, End: start + sweep}), t.Border)
		if frac > 0 {
			p.FillPath(chart.Sector(cx, cy, inner, outer, chart.Slice{Start: start, End: start + sweep*frac}), ToneColor(t, tone))
		}
	}).Center().Children(func() {
		ui.Column(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, center).FontSize(size / 7).Bold().SingleLine()
			Caption(c, caption)
		})
	})
}

// Segment is one part of a SegmentBar.
type Segment struct {
	Label string
	Value float64
	Tone  Tone
}

// SegmentBar draws parts of a whole side by side in one bar, as where the
// disk goes, with a legend under it. total is the whole; what the parts
// leave is drawn empty.
func SegmentBar(c *ui.Context, label string, total float64, parts []Segment, legend func(Segment) string) ui.Element {
	t := c.Theme()
	return ui.Column(c).Gap(SpaceS).AlignItems(ui.Stretch).Children(func() {
		ui.Box(c).Height(12).Label(label).Role(ui.RoleImage).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, t.Border, 6)
			if total <= 0 {
				return
			}
			x := r.X
			for _, s := range parts {
				w := float32(s.Value/total) * r.W
				if w <= 0 {
					continue
				}
				p.Fill(ui.Rect{X: x, Y: r.Y, W: w, H: r.H}, ToneColor(t, s.Tone), 6)
				x += w + 2
			}
		})
		ui.Row(c).Gap(SpaceL).Wrap().Children(func() {
			for i, s := range parts {
				text := s.Label
				if legend != nil {
					text = legend(s)
				}
				Legend(c.Key(i), s.Tone, text)
			}
		})
	})
}

// ToggleTile is a large tile that shows a thing on or off, as a quick
// setting: an icon, a title and a line under it, filled with the accent
// while on. It reports a click; the app decides what a click does (ask
// first). busy shows a spinner instead of the icon.
func ToggleTile(c *ui.Context, icon, title, sub string, on, busy bool) bool {
	t := c.Theme()
	bg, fg, muted := Panel(t), t.Text, t.TextMuted
	if on {
		bg, fg, muted = t.Accent, t.AccentText, t.AccentText.Alpha(0.8)
	}
	tile := ui.Column(c).Width(210).Height(112).Padding(14).Gap(SpaceS).Radius(RadiusCard).Background(bg).
		Border(1, t.Border).AlignItems(ui.Start).Role(ui.RoleSwitch).Checked(on).Focusable().Cursor(ui.CursorPointer).
		Label(title).Description(sub)
	if tile.Hovered() && !on {
		tile.Background(t.SurfaceHover)
	}
	clicked := tile.Clicked() && !busy
	tile.Children(func() {
		ui.Box(c).Size(34, 34).Radius(17).Background(fg.Alpha(0.12)).Center().Children(func() {
			if busy {
				ui.Spinner(c).Label("Working")
			} else {
				Icon(c, icon).TextColor(fg)
			}
		})
		ui.Spacer(c)
		ui.Text(c, title).Bold().FontSize(13.5).TextColor(fg).SingleLine()
		ui.Text(c, sub).FontSize(12).TextColor(muted).SingleLine()
	})
	return clicked
}
