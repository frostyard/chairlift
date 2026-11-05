// Package reporting sends the problem reports that an opted-in user allows
// (ADR-0015, docs/specs/problem-reporting.md). It is a pure leaf package:
// it imports no GTK binding, and only sender.go imports net/http.
//
// Phase 2 keeps the rule that nothing leaves the computer until the user
// turns reporting on. A report is sent only when all of these hold: the
// problems_reporting_group is enabled, the per-user consent file says yes
// for the current schema generation, and either the user asked for this
// report (the Report dialog) or the consent mode is "auto" and internal
// budget rules allow it. A kind crash report is never sent in this phase.
package reporting

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

// Mode is the consent's automatic-send choice.
type Mode string

const (
	// ModeAsk shows every report before it is sent: nothing is sent without
	// the Report dialog.
	ModeAsk Mode = "ask"
	// ModeAuto sends pending reports without asking, and lists each one in
	// "Sent reports".
	ModeAuto Mode = "auto"
)

const (
	// consentFormat is the only consent file format this package reads.
	consentFormat = 1
	// maxConsentBytes bounds the consent file that LoadConsent reads.
	maxConsentBytes = 64 << 10
	// CurrentSchema is the schema version that this build agrees to send.
	CurrentSchema = problems.SchemaID
)

// dataGeneration is the data generation of each schema version. A schema
// that adds a kind or a field adds data, so it gets a new number and an old
// consent stops sending until the user agrees again; a schema that only
// adds enum values keeps the generation, so it keeps sending.
var dataGeneration = map[string]int{
	"frostyard.problem/1": 1,
}

// Consent is the per-user problem-reporting decision.
type Consent struct {
	// Format is 1.
	Format int `json:"format"`
	// Enabled is the user's switch. Off is the default: a missing file, a
	// malformed file or an unknown mode all read as off.
	Enabled bool `json:"enabled"`
	// Mode is ModeAsk or ModeAuto.
	Mode Mode `json:"mode"`
	// DecidedAt is when the user made this decision.
	DecidedAt time.Time `json:"decided_at"`
	// Schema is the schema version the user agreed to.
	Schema string `json:"schema"`
}

// ConsentPath returns the consent file of this user:
// $XDG_CONFIG_HOME/chairlift/reporting.json, else
// $HOME/.config/chairlift/reporting.json. Only absolute values count.
// getenv is os.Getenv in production.
func ConsentPath(getenv func(string) string) (string, error) {
	if dir := getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "chairlift", "reporting.json"), nil
	}
	if home := getenv("HOME"); filepath.IsAbs(home) {
		return filepath.Join(home, ".config", "chairlift", "reporting.json"), nil
	}
	return "", errors.New("reporting: neither XDG_CONFIG_HOME nor HOME is an absolute path")
}

// LoadConsent reads the consent file at path. A missing file is the zero
// Consent (off) with no error. A file that is unreadable, larger than
// 64 KiB, not format 1, or holds an unknown mode gives the zero Consent
// (off) and the error: reporting fails closed.
func LoadConsent(path string) (Consent, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Consent{}, nil
	}
	if err != nil {
		return Consent{}, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxConsentBytes+1))
	if err != nil {
		return Consent{}, err
	}
	if len(data) > maxConsentBytes {
		return Consent{}, fmt.Errorf("%s: larger than %d bytes", path, maxConsentBytes)
	}
	var c Consent
	if err := json.Unmarshal(data, &c); err != nil {
		return Consent{}, fmt.Errorf("%s: not a problem reporting consent: %w", path, err)
	}
	if c.Format != consentFormat {
		return Consent{}, fmt.Errorf("%s: not a format %d problem reporting consent", path, consentFormat)
	}
	if c.Mode != ModeAsk && c.Mode != ModeAuto {
		return Consent{}, fmt.Errorf("%s: %q is not a problem reporting mode", path, string(c.Mode))
	}
	return c, nil
}

// SaveConsent writes the consent to path: a temporary file in the same
// folder (mode 0600), then a rename. The folder is made with mode 0700.
func SaveConsent(path string, c Consent) error {
	if path == "" {
		return errors.New("reporting: the consent file has no path")
	}
	if c.Format == 0 {
		c.Format = consentFormat
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, data, ".reporting-*.json")
}

// writeFile writes data to path with the temporary-file-then-rename rule
// that the consent and the sent log share.
func writeFile(path string, data []byte, pattern string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// Allows reports whether this consent sends anything at all: reporting is
// on, and the schema the user agreed to has the data generation of the
// current schema.
func (c Consent) Allows() bool {
	return c.allows(dataGeneration, CurrentSchema)
}

// allows is Allows with explicit generations, so a test can model a later
// schema without changing the package's table.
func (c Consent) allows(gens map[string]int, current string) bool {
	if !c.Enabled {
		return false
	}
	agreed, ok := gens[c.Schema]
	if !ok {
		return false
	}
	want, ok := gens[current]
	if !ok {
		return false
	}
	return agreed == want
}

// Stale reports whether reporting is on but the current schema adds data
// the user has not agreed to. The page shows the switch off and says "The
// report format changed. Turn reporting on to agree again."
func (c Consent) Stale() bool {
	return c.Enabled && !c.Allows()
}

// SwitchChanged reports whether the user changed the consent switch: active
// is not the state the switch shows for stored. The switch shows Allows(),
// not Enabled, because a stale consent (on, but of an older data generation)
// shows off and must be renewable by turning the switch on.
func SwitchChanged(stored Consent, active bool) bool {
	return active != stored.Allows()
}

// Decide returns the consent record to save for a switch state and a mode.
// The record names the current schema and the decision time (UTC); a mode
// that is not ModeAuto is recorded as ModeAsk.
func Decide(enabled bool, mode Mode, now time.Time) Consent {
	if mode != ModeAuto {
		mode = ModeAsk
	}
	return Consent{
		Format:    consentFormat,
		Enabled:   enabled,
		Mode:      mode,
		DecidedAt: now.UTC(),
		Schema:    CurrentSchema,
	}
}
