package views

import (
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui"
)

// Brew holds Homebrew's packages, taps and bundles and the Homebrew update
// source. STUB.
type Brew struct{ env *Env }

// NewBrew returns the Homebrew state.
func NewBrew(env *Env) *Brew { return &Brew{env: env} }

// Start begins the first load.
func (b *Brew) Start() {}

// Source is the Homebrew update source, nil when it does not apply.
func (b *Brew) Source() UpdateSource { return nil }

// Apps are the installed formulae and casks, for the Apps table.
func (b *Brew) Apps() []AppEntry { return nil }

// AppsStatus is the state of the installed-packages load.
func (b *Brew) AppsStatus() AppsStatus { return AppsStatus{} }

// RefreshApps reloads the installed packages.
func (b *Brew) RefreshApps() {}

// Act does action to the entry with key, asking first where the old page
// did.
func (b *Brew) Act(key string, action AppAction) {}

// BundlesAvailable reports whether the Apps page shows the Bundles tab
// (brew_bundles_group).
func (b *Brew) BundlesAvailable() bool { return false }

// DrawBundles draws the Bundles tab of the Apps page.
func (b *Brew) DrawBundles(c *ui.Context) {}

// SearchAvailable reports whether the Apps page shows the Find more tab
// (brew_search_group).
func (b *Brew) SearchAvailable() bool { return false }

// DrawSearch draws the Find more tab of the Apps page: Homebrew search and
// install.
func (b *Brew) DrawSearch(c *ui.Context) {}

// Commands are Homebrew's palette commands (update metadata, trust taps),
// which the Apps and Updates pages list.
func (b *Brew) Commands() []frostui.Command { return nil }
