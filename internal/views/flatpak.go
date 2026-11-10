package views

// Flatpak holds the installed Flatpak apps and the Flatpak update source.
// STUB.
type Flatpak struct{ env *Env }

// NewFlatpak returns the Flatpak state.
func NewFlatpak(env *Env) *Flatpak { return &Flatpak{env: env} }

// Start begins the first load.
func (f *Flatpak) Start() {}

// Source is the Flatpak update source, nil when it does not apply.
func (f *Flatpak) Source() UpdateSource { return nil }

// Apps are the installed Flatpak apps of the enabled installations, for the
// Apps table.
func (f *Flatpak) Apps() []AppEntry { return nil }

// AppsStatus is the state of the installed-apps load.
func (f *Flatpak) AppsStatus() AppsStatus { return AppsStatus{} }

// RefreshApps reloads the installed apps.
func (f *Flatpak) RefreshApps() {}

// Act does action to the entry with key, asking first where the old page
// did.
func (f *Flatpak) Act(key string, action AppAction) {}
