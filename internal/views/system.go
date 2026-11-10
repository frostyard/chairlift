package views

// System holds the deployments, the system-image update source and the
// host facts. STUB: replaced by the Overview port.
type System struct{ env *Env }

// NewSystem returns the system state; nothing loads until Start.
func NewSystem(env *Env) *System { return &System{env: env} }

// Start begins the first load.
func (s *System) Start() {}

// Info is what the Overview hero, status bar and Maintenance show.
func (s *System) Info() SystemInfo { return SystemInfo{} }

// Source is the system-image update source (bootc or native A/B), nil when
// neither applies.
func (s *System) Source() UpdateSource { return nil }
