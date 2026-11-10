package views

// Firmware holds fwupd's devices with updates and the firmware update
// source. STUB.
type Firmware struct{ env *Env }

// NewFirmware returns the firmware state.
func NewFirmware(env *Env) *Firmware { return &Firmware{env: env} }

// Start begins the first load.
func (f *Firmware) Start() {}

// Source is the firmware update source, nil when it does not apply.
func (f *Firmware) Source() UpdateSource { return nil }
