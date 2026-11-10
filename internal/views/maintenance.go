package views

// Maintenance holds the cleanups. STUB.
type Maintenance struct {
	env   *Env
	store *Store
}

// NewMaintenance returns the maintenance state.
func NewMaintenance(env *Env, s *Store) *Maintenance { return &Maintenance{env: env, store: s} }

// Start begins the first load.
func (m *Maintenance) Start() {}

// NewMaintenancePage returns the Maintenance page. STUB.
func NewMaintenancePage(s *Store) Page { return stubPage{title: "Maintenance"} }
