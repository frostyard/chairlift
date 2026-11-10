package views

// Problems holds the problem records and the reporting state. STUB.
type Problems struct{ env *Env }

// NewProblems returns the problems state.
func NewProblems(env *Env) *Problems { return &Problems{env: env} }

// Start begins the first load.
func (p *Problems) Start() {}

// Summary is what the Overview and the status bar show.
func (p *Problems) Summary() ProblemSummary { return ProblemSummary{} }

// NewProblemsPage returns the Problems page. STUB.
func NewProblemsPage(s *Store) Page { return stubPage{title: "Problems"} }
