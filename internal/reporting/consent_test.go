package reporting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsentPathUsesAbsoluteEnvironmentOnly(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
		err  bool
	}{
		{"xdg config home", map[string]string{"XDG_CONFIG_HOME": "/home/ada/.config", "HOME": "/home/ada"}, "/home/ada/.config/chairlift/reporting.json", false},
		{"home fallback", map[string]string{"HOME": "/home/ada"}, "/home/ada/.config/chairlift/reporting.json", false},
		{"relative values are ignored", map[string]string{"XDG_CONFIG_HOME": "config", "HOME": "home"}, "", true},
		{"nothing set", map[string]string{}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConsentPath(func(name string) string { return tt.env[name] })
			if tt.err {
				if err == nil {
					t.Fatalf("ConsentPath() = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ConsentPath(): %v", err)
			}
			if got != tt.want {
				t.Errorf("ConsentPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestConsentDefaultsToOff proves every broken consent file reads as off
// with an error, so reporting fails closed.
func TestConsentDefaultsToOff(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{"missing", "", false},
		{"off", `{"format":1,"enabled":false,"mode":"ask","decided_at":"2026-11-04T10:00:00Z","schema":"frostyard.problem/1"}`, false},
		{"on", `{"format":1,"enabled":true,"mode":"auto","decided_at":"2026-11-04T10:00:00Z","schema":"frostyard.problem/1"}`, false},
		{"malformed JSON", `{"format":1,`, true},
		{"wrong format", `{"format":2,"enabled":true,"mode":"auto","schema":"frostyard.problem/1"}`, true},
		{"unknown mode", `{"format":1,"enabled":true,"mode":"sometimes","schema":"frostyard.problem/1"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "-")+".json")
			if tt.content != "" {
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			c, err := LoadConsent(path)
			if tt.wantErr && err == nil {
				t.Fatalf("LoadConsent(%s) = %+v, want an error", tt.name, c)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("LoadConsent(%s): %v", tt.name, err)
			}
			if err != nil && c.Enabled {
				t.Fatalf("LoadConsent(%s) kept enabled = true on an error: %+v", tt.name, c)
			}
			if tt.wantErr && c != (Consent{}) {
				t.Errorf("LoadConsent(%s) = %+v, want the zero Consent", tt.name, c)
			}
		})
	}
}

// TestConsentTooLargeIsOff proves the 64 KiB bound.
func TestConsentTooLargeIsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reporting.json")
	big := `{"format":1,"enabled":true,"mode":"auto","schema":"` + strings.Repeat("x", maxConsentBytes) + `"}`
	if err := os.WriteFile(path, []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConsent(path)
	if err == nil || c != (Consent{}) {
		t.Fatalf("LoadConsent(too large) = %+v, %v; want the zero Consent and an error", c, err)
	}
}

func TestSaveConsentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "reporting.json")
	want := Decide(true, ModeAuto, testStart)
	if err := SaveConsent(path, want); err != nil {
		t.Fatalf("SaveConsent: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("consent file mode = %o, want 600", perm)
	}
	folder, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := folder.Mode().Perm(); perm != 0o700 {
		t.Errorf("consent folder mode = %o, want 700", perm)
	}
	got, err := LoadConsent(path)
	if err != nil {
		t.Fatalf("LoadConsent: %v", err)
	}
	if got != want {
		t.Errorf("LoadConsent() = %+v, want %+v", got, want)
	}
	if !got.DecidedAt.Equal(testStart.UTC()) {
		t.Errorf("DecidedAt = %v, want %v", got.DecidedAt, testStart.UTC())
	}
}

// TestConsentAllowsCurrentSchemaGeneration pins the schema-generation rule:
// a schema that adds a kind or a field (a new data generation) turns sending
// off until the user agrees again, and a schema that only adds enum values
// keeps it on.
func TestConsentAllowsCurrentSchemaGeneration(t *testing.T) {
	gens := map[string]int{"frostyard.problem/1": 1, "frostyard.problem/2": 1}
	dataGens := map[string]int{"frostyard.problem/1": 1, "frostyard.problem/2": 2}
	on := Consent{Enabled: true, Mode: ModeAuto, Schema: "frostyard.problem/1"}
	off := Consent{Enabled: false, Mode: ModeAuto, Schema: "frostyard.problem/1"}
	unknown := Consent{Enabled: true, Mode: ModeAuto, Schema: "frostyard.problem/9"}

	tests := []struct {
		name    string
		c       Consent
		gens    map[string]int
		current string
		want    bool
	}{
		{"same schema", on, gens, "frostyard.problem/1", true},
		{"enum-only later schema keeps sending", on, gens, "frostyard.problem/2", true},
		{"data-adding later schema stops sending", on, dataGens, "frostyard.problem/2", false},
		{"off stays off", off, gens, "frostyard.problem/2", false},
		{"unknown agreed schema", unknown, gens, "frostyard.problem/2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.allows(tt.gens, tt.current); got != tt.want {
				t.Errorf("allows() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConsentAllowsUsesThePackageTable proves the package's own table and
// the current schema send, so a drift between them fails here.
func TestConsentAllowsUsesThePackageTable(t *testing.T) {
	c := Decide(true, ModeAuto, testStart)
	if !c.Allows() {
		t.Fatal("Allows() = false for a fresh consent of the current schema")
	}
	if c.Stale() {
		t.Error("Stale() = true for a fresh consent")
	}
	old := c
	old.Schema = "frostyard.problem/0"
	if old.Allows() {
		t.Error("Allows() = true for a schema that is not in the generation table")
	}
	if !old.Stale() {
		t.Error("Stale() = false for an enabled consent of an older schema")
	}
}

// TestSwitchChanged proves the switch's comparison: the switch shows Allows,
// so a stale consent shows off and turning it on is a change.
func TestSwitchChanged(t *testing.T) {
	stale := Consent{Format: consentFormat, Enabled: true, Mode: ModeAuto, Schema: "frostyard.problem/0"}
	tests := []struct {
		name   string
		stored Consent
		active bool
		want   bool
	}{
		{"off, switched on", Consent{}, true, true},
		{"off, still off", Consent{}, false, false},
		{"on, switched off", Decide(true, ModeAuto, testStart), false, true},
		{"on, still on", Decide(true, ModeAsk, testStart), true, false},
		{"stale shows off, switched on", stale, true, true},
		{"stale shows off, still off", stale, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SwitchChanged(tt.stored, tt.active); got != tt.want {
				t.Errorf("SwitchChanged(%+v, %v) = %v, want %v", tt.stored, tt.active, got, tt.want)
			}
		})
	}
}

func TestDecideRecordsTheCurrentSchemaAndTime(t *testing.T) {
	got := Decide(true, "nonsense", testStart)
	if got.Format != consentFormat || !got.Enabled || got.Mode != ModeAsk || got.Schema != CurrentSchema {
		t.Errorf("Decide(true, nonsense) = %+v", got)
	}
	if !got.DecidedAt.Equal(testStart) {
		t.Errorf("DecidedAt = %v, want %v", got.DecidedAt, testStart)
	}
	if off := Decide(false, ModeAuto, testStart); off.Enabled || off.Mode != ModeAuto {
		t.Errorf("Decide(false, auto) = %+v", off)
	}
}

// TestFileConsentReloads proves the reader a live send uses: it reads the
// file on every call, so a change by another ChairLift process takes effect
// before the next request, and a read failure is off.
func TestFileConsentReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reporting.json")
	on := Decide(true, ModeAuto, testStart)
	off := Decide(false, ModeAsk, testStart)
	if err := SaveConsent(path, on); err != nil {
		t.Fatal(err)
	}
	consent := FileConsent(path)
	if got := consent(); got != on {
		t.Fatalf("FileConsent() = %+v, want the file's value", got)
	}
	if err := SaveConsent(path, off); err != nil {
		t.Fatal(err)
	}
	if got := consent(); got != off {
		t.Errorf("FileConsent() after a change = %+v, want the new value", got)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := consent(); got != (Consent{}) {
		t.Errorf("FileConsent() for a broken file = %+v, want off", got)
	}
	if got := FileConsent("")(); got != (Consent{}) {
		t.Errorf("FileConsent(\"\")() = %+v, want off", got)
	}
}
