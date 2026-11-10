package views

import "github.com/frostyard/chairlift/internal/sysinfo"

// Live samples processor, memory and disk each second. STUB: replaced by
// the Overview port.
type Live struct{ env *Env }

// NewLive returns the sampler; it starts with Start.
func NewLive(env *Env) *Live { return &Live{env: env} }

// Start begins sampling.
func (l *Live) Start() {}

// Latest is the newest sample.
func (l *Live) Latest() sysinfo.Sample { return sysinfo.Sample{} }

// CPU and Memory are the last 60 seconds, oldest first, 0 to 100.
func (l *Live) CPU() []float64    { return nil }
func (l *Live) Memory() []float64 { return nil }
