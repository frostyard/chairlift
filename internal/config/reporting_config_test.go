package config

import (
	"os"
	"path/filepath"
	"testing"
)

// endpointConfig is a configuration file that sets the problem-report
// endpoint of problems_reporting_group.
const endpointConfig = "problems_page:\n  problems_reporting_group:\n    endpoint: https://example.invalid/v1/report\n"

// TestReportingEndpointLandsOnGroupConfig proves the endpoint key is part of
// the group schema, so it decodes and overlays like every other group field.
func TestReportingEndpointLandsOnGroupConfig(t *testing.T) {
	cfg, err := loadFromPath(writeConfigFile(t, endpointConfig))
	if err != nil {
		t.Fatalf("loadFromPath: %v", err)
	}
	group := cfg.GetGroupConfig("problems_page", "problems_reporting_group")
	if group == nil {
		t.Fatal("GetGroupConfig(problems_page, problems_reporting_group) = nil")
	}
	if group.Endpoint != "https://example.invalid/v1/report" {
		t.Errorf("endpoint = %q, want the configured value", group.Endpoint)
	}
}

// TestReportingEndpointFromUserWritableFileIsIgnored is the rule of
// ADR-0015: the development fallback beside the executable is user-writable,
// so a value it sets must never choose where a report goes.
func TestReportingEndpointFromUserWritableFileIsIgnored(t *testing.T) {
	cfg, err := loadFromPath(writeConfigFile(t, endpointConfig))
	if err != nil {
		t.Fatalf("loadFromPath: %v", err)
	}
	endpoint, adminOwned := cfg.ReportEndpoint()
	if adminOwned || endpoint != "" {
		t.Fatalf("ReportEndpoint() = (%q, %v), want (\"\", false) for a user-writable file", endpoint, adminOwned)
	}
}

// TestReportingEndpointFromAFallbackInTheWorkingDirectoryIsIgnored proves
// the relative "config.yml" candidate cannot set the endpoint: it resolves
// to a user-writable path under the working directory.
func TestReportingEndpointFromAFallbackInTheWorkingDirectoryIsIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(endpointConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("loadFromPath: %v", err)
	}
	if endpoint, adminOwned := cfg.ReportEndpoint(); adminOwned || endpoint != "" {
		t.Fatalf("ReportEndpoint() = (%q, %v), want (\"\", false)", endpoint, adminOwned)
	}
}

// TestReportingEndpointFromAnAdminOwnedFileIsTrusted proves the two
// administrator-owned paths do reach the caller.
func TestReportingEndpointFromAnAdminOwnedFileIsTrusted(t *testing.T) {
	for _, admin := range adminConfigPaths {
		t.Run(admin, func(t *testing.T) {
			cfg, err := loadFromPath(writeConfigFile(t, endpointConfig))
			if err != nil {
				t.Fatalf("loadFromPath: %v", err)
			}
			// The test cannot write /etc, so it names the source the loader
			// would have recorded for that candidate.
			cfg.source = admin
			endpoint, adminOwned := cfg.ReportEndpoint()
			if !adminOwned {
				t.Fatal("ReportEndpoint() adminOwned = false, want true")
			}
			if endpoint != "https://example.invalid/v1/report" {
				t.Errorf("endpoint = %q, want the configured value", endpoint)
			}
		})
	}
}

// TestReportingEndpointAbsentMeansNoOverride proves an empty endpoint is not
// an override, so the caller keeps the default.
func TestReportingEndpointAbsentMeansNoOverride(t *testing.T) {
	cfg := defaultConfig()
	if endpoint, adminOwned := cfg.ReportEndpoint(); adminOwned || endpoint != "" {
		t.Fatalf("ReportEndpoint() = (%q, %v), want (\"\", false)", endpoint, adminOwned)
	}
	cfg.source = adminConfigPaths[0]
	if endpoint, adminOwned := cfg.ReportEndpoint(); adminOwned || endpoint != "" {
		t.Fatalf("ReportEndpoint() = (%q, %v), want (\"\", false) with no configured endpoint", endpoint, adminOwned)
	}
}

// TestReportingEndpointRejectsAWrongType proves the endpoint takes part in
// the type validation of the schema, like every other group field.
func TestReportingEndpointRejectsAWrongType(t *testing.T) {
	_, err := loadFromPath(writeConfigFile(t, "problems_page:\n  problems_reporting_group:\n    endpoint: [a, b]\n"))
	if err == nil {
		t.Fatal("loadFromPath accepted a sequence endpoint")
	}
	if err.Kind != KindParseType {
		t.Errorf("error kind = %v, want KindParseType", err.Kind)
	}
}
