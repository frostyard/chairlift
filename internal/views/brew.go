package views

// Brew holds Homebrew's packages, taps and bundles and the Homebrew update
// source. STUB.
type Brew struct{ env *Env }

// NewBrew returns the Homebrew state.
func NewBrew(env *Env) *Brew { return &Brew{env: env} }

// Start begins the first load.
func (b *Brew) Start() {}

// Source is the Homebrew update source, nil when it does not apply.
func (b *Brew) Source() UpdateSource { return nil }
