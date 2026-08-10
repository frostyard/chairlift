package installcheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAutoQATuningConfigurationIsValidJSON(t *testing.T) {
	path := filepath.Join(RepoRoot(), ".github", "auto-qa-tuning.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Auto-QA tuning configuration: %v", err)
	}
	if !json.Valid(data) {
		t.Fatal("Auto-QA tuning configuration is not valid JSON")
	}
}
