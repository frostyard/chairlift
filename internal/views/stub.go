package views

import (
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui"
)

// stubPage stands in for a page until its port lands. Remove it once no
// constructor returns it.
type stubPage struct{ title, note string }

func (p stubPage) View(c *ui.Context) {
	frostui.PageHeader(c, p.title, p.note, nil)
}
func (stubPage) Commands() []frostui.Command   { return nil }
func (stubPage) Badge() (string, frostui.Tone) { return "", frostui.Neutral }
