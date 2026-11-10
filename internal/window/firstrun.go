package window

import (
	"log"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui"
)

// FirstRun is the window of `chairlift --first-run` (ADR-0014). STUB:
// replaced by the first-run port.
type FirstRun struct {
	post func(func())
}

// NewFirstRun builds the first-run window's state. post runs a function
// on the UI thread (Window.Update).
func NewFirstRun(post func(func())) *FirstRun {
	return &FirstRun{post: post}
}

// View draws the window.
func (w *FirstRun) View(c *ui.Context) {
	frostui.Adapt(c)
	frostui.PageHeader(c, "Welcome", "", nil)
}

// CanClose reports whether closing the window now is safe: false while the
// first run's actions run, since a close would lose the completion
// markers.
func (w *FirstRun) CanClose() bool { return true }

// Presented logs the readiness marker the end-to-end test and the lab read
// (ADR-0008, ADR-0014).
func (w *FirstRun) Presented() {
	log.Println("first-run: window presented")
}
