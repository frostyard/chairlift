package views

// Features holds updex's features and the features update source. STUB.
type Features struct{ env *Env }

// NewFeatures returns the features state.
func NewFeatures(env *Env) *Features { return &Features{env: env} }

// Start begins the first load.
func (f *Features) Start() {}

// Source is the features update source, nil when it does not apply.
func (f *Features) Source() UpdateSource { return nil }

// Summary is what the Overview shows.
func (f *Features) Summary() FeatureSummary { return FeatureSummary{} }

// NewFeaturesPage returns the Features page. STUB.
func NewFeaturesPage(s *Store) Page { return stubPage{title: "Features"} }
