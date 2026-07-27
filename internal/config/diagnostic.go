package config

import "fmt"

// LogMessage renders the high-signal startup diagnostic for an authoritative
// configuration failure.
func (e *LoadError) LogMessage() string {
	return fmt.Sprintf(
		"CONFIGURATION ERROR: %s; all feature groups were disabled; fix the configuration file and restart ChairLift",
		e.Error(),
	)
}

// ToastMessage renders the persistent user-facing startup diagnostic for an
// authoritative configuration failure.
func (e *LoadError) ToastMessage() string {
	return fmt.Sprintf(
		"Configuration error: %s. All feature groups are disabled. Fix the configuration file and restart ChairLift.",
		e.Error(),
	)
}
