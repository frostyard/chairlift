package installcheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAutoQATuningConfiguration(t *testing.T) {
	path := filepath.Join(RepoRoot(), ".github", "auto-qa-tuning.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Auto-QA tuning configuration: %v", err)
	}

	var config struct {
		SchemaVersion int               `json:"schema_version"`
		Signals       map[string]string `json:"signals"`
		Guardrails    struct {
			RequiredChecks string `json:"required_checks"`
			SecurityChecks string `json:"security_checks"`
		} `json:"guardrails"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse Auto-QA tuning configuration: %v", err)
	}
	if config.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", config.SchemaVersion)
	}
	if got := config.Signals["pr_acceptance_rate"]; got != "docs/metrics.md#pr-acceptance-rate" {
		t.Errorf("pr_acceptance_rate signal = %q, want docs metric", got)
	}
	if config.Guardrails.RequiredChecks != "never_relax" {
		t.Errorf("required_checks guardrail = %q, want never_relax", config.Guardrails.RequiredChecks)
	}
	if config.Guardrails.SecurityChecks != "never_relax" {
		t.Errorf("security_checks guardrail = %q, want never_relax", config.Guardrails.SecurityChecks)
	}
}
