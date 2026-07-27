package config

import (
	"errors"
	"testing"
)

func TestLoadErrorDiagnosticMessages(t *testing.T) {
	loadErr := &LoadError{
		Path:   "/etc/chairlift/config.yml",
		Kind:   KindRead,
		Detail: "reading configuration file",
		Err:    errors.New("permission denied"),
	}

	const wantLog = "CONFIGURATION ERROR: config read error: /etc/chairlift/config.yml: reading configuration file: permission denied; all feature groups were disabled; fix the configuration file and restart ChairLift"
	if got := loadErr.LogMessage(); got != wantLog {
		t.Fatalf("LogMessage() = %q, want %q", got, wantLog)
	}

	const wantToast = "Configuration error: config read error: /etc/chairlift/config.yml: reading configuration file: permission denied. All feature groups are disabled. Fix the configuration file and restart ChairLift."
	if got := loadErr.ToastMessage(); got != wantToast {
		t.Fatalf("ToastMessage() = %q, want %q", got, wantToast)
	}
}
