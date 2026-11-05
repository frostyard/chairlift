package reporting

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

// MaxBodyBytes is the intake API's body limit: a body of 8192 bytes or more
// is refused before it is sent (frostyard/problem-intake
// docs/specs/intake-api.md).
const MaxBodyBytes = 8192

// ErrLocalOnly is returned for a report that never leaves this computer. A
// kind crash report is local only in this phase: the sender refuses it, and
// the page offers no Report button for it.
var ErrLocalOnly = errors.New("reporting: this report stays on this computer")

// Report is 1 report as the dialog shows it and as the sender posts it.
// Body is the single buffer: the dialog shows Text(), and Send posts Body.
type Report struct {
	Kind      string
	ProblemID string
	Week      string
	ReportID  string
	Body      []byte
}

// Text returns the report as the dialog shows it.
func (r Report) Text() string {
	return string(r.Body)
}

// The enum values of schema v1's count detail. They are the closed enums of
// the vendored schema (internal/problems/testdata/schema/v1.json).
var (
	installAges = []string{"<1w", "<1m", "<6m", ">=6m"}
	channels    = []string{"latest", "other"}
)

// countImage is the envelope's image object.
type countImage struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// countPayload is a frostyard.problem/1 document of kind count, in schema
// order.
type countPayload struct {
	Schema   string         `json:"schema"`
	ReportID string         `json:"report_id"`
	Week     string         `json:"week"`
	Image    countImage     `json:"image"`
	Secure   bool           `json:"secure"`
	Client   string         `json:"client"`
	Kind     string         `json:"kind"`
	Detail   map[string]any `json:"detail"`
}

// BuildCount returns the compact frostyard.problem/1 document of the weekly
// count: the install age bucket and the update channel of this host, and
// nothing else. It refuses an invalid envelope, an install age or channel
// outside the v1 enums, a week, report ID or client that fails its v1
// pattern, and a document that is not under MaxBodyBytes.
func BuildCount(env problems.Envelope, week, installAge, channel, reportID, client string) ([]byte, error) {
	if !env.Valid {
		return nil, errors.New("reporting: the image envelope is not valid")
	}
	if !problems.ValidWeek(week) {
		return nil, fmt.Errorf("reporting: week %q does not match the v1 week pattern", week)
	}
	if !contains(installAges, installAge) {
		return nil, fmt.Errorf("reporting: install age %q is not a v1 value", installAge)
	}
	if !contains(channels, channel) {
		return nil, fmt.Errorf("reporting: channel %q is not a v1 value", channel)
	}
	if !problems.ValidReportID(reportID) {
		return nil, fmt.Errorf("reporting: report id %q is not a lowercase UUIDv4", reportID)
	}
	if !problems.ValidClient(client) {
		return nil, fmt.Errorf("reporting: client %q does not match the v1 client pattern", client)
	}
	data, err := marshalDocument(countPayload{
		Schema:   problems.SchemaID,
		ReportID: reportID,
		Week:     week,
		Image:    countImage{ID: env.ImageID, Version: env.ImageVersion},
		Secure:   env.Secure,
		Client:   client,
		Kind:     problems.KindCount,
		Detail:   map[string]any{"install_age": installAge, "channel": channel},
	})
	if err != nil {
		return nil, err
	}
	if len(data) >= MaxBodyBytes {
		return nil, fmt.Errorf("reporting: report of %d bytes is not under %d", len(data), MaxBodyBytes)
	}
	return data, nil
}

// marshalDocument encodes v as compact JSON without HTML escaping, so the
// count's "<1w" and ">=6m" stay readable in the dialog and in the request.
// The trailing newline of the encoder is removed.
func marshalDocument(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}

// Prepare returns the Report for a compact body: Body is the document
// indented with 2 spaces when that stays under MaxBodyBytes, so the dialog
// shows readable JSON, and else the compact bytes.
func Prepare(kind, problemID, week, reportID string, compact []byte) Report {
	r := Report{Kind: kind, ProblemID: problemID, Week: week, ReportID: reportID, Body: compact}
	var indented bytes.Buffer
	if err := json.Indent(&indented, compact, "", "  "); err == nil && indented.Len() < MaxBodyBytes {
		r.Body = indented.Bytes()
	}
	return r
}

// KindOf reads the kind of a report body.
func KindOf(body []byte) (string, error) {
	var doc struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("reporting: not a report document: %w", err)
	}
	if doc.Kind == "" {
		return "", errors.New("reporting: the report document has no kind")
	}
	return doc.Kind, nil
}

// ForItem returns the report of a reportable item: a new report ID, the
// item's payload with only validated schema fields, and the dialog's text.
// It refuses kind crash with ErrLocalOnly before it builds anything.
func ForItem(it problems.Item, client string) (Report, error) {
	if it.Kind == problems.KindCrash {
		return Report{}, ErrLocalOnly
	}
	reportID, err := problems.NewReportID()
	if err != nil {
		return Report{}, err
	}
	compact, err := problems.BuildPayload(it, reportID, client)
	if err != nil {
		return Report{}, err
	}
	return Prepare(it.Kind, it.ID, it.Week, reportID, compact), nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// NewCountReport returns this week's count report: a new report ID, the v1
// count document of env, and the dialog's text. now decides the ISO week.
func NewCountReport(env problems.Envelope, client, installAge, channel string, now time.Time) (Report, error) {
	week := problems.ISOWeek(now)
	reportID, err := problems.NewReportID()
	if err != nil {
		return Report{}, err
	}
	body, err := BuildCount(env, week, installAge, channel, reportID, client)
	if err != nil {
		return Report{}, err
	}
	return Prepare(problems.KindCount, "", week, reportID, body), nil
}
