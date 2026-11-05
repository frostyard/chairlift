package problems

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
)

// maxPayloadBytes is the size bound of a report document (spec rule 9).
const maxPayloadBytes = 8 << 10

// ErrNotReportable is returned by BuildPayload for an item that is not
// reportable.
var ErrNotReportable = errors.New("problems: item is not reportable")

// payloadImage is the envelope's image object.
type payloadImage struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// payload is a frostyard.problem/1 report document, in schema order.
type payload struct {
	Schema   string         `json:"schema"`
	ReportID string         `json:"report_id"`
	Week     string         `json:"week"`
	Image    payloadImage   `json:"image"`
	Secure   bool           `json:"secure"`
	Client   string         `json:"client"`
	Kind     string         `json:"kind"`
	Detail   map[string]any `json:"detail"`
}

// BuildPayload returns the frostyard.problem/1 document of a reportable
// item. It copies only schema fields, each from its validated value: never
// details.json text, a MESSAGE, a path or a trusted journald field. It
// returns ErrNotReportable for an item that is not reportable, and an error
// for a report ID or client that fails its v1 pattern.
//
// Phase 1 sends nothing and the page does not call it; it exists for the
// opt-in sender of a later task and for the schema tests.
func BuildPayload(it Item, reportID, client string) ([]byte, error) {
	if !it.Reportable || !it.DetailValid() || !it.WeekValid || !it.Envelope.Valid {
		return nil, ErrNotReportable
	}
	if !reportIDRe.MatchString(reportID) {
		return nil, fmt.Errorf("problems: report id %q is not a lowercase UUIDv4", reportID)
	}
	if !clientRe.MatchString(client) {
		return nil, fmt.Errorf("problems: client %q does not match the v1 client pattern", client)
	}
	detail := make(map[string]any, len(it.Fields))
	for _, f := range it.Fields {
		detail[f.Name] = f.Value
	}
	data, err := marshalDocument(payload{
		Schema:   SchemaID,
		ReportID: reportID,
		Week:     it.Week,
		Image:    payloadImage{ID: it.Envelope.ImageID, Version: it.Envelope.ImageVersion},
		Secure:   it.Envelope.Secure,
		Client:   client,
		Kind:     it.Kind,
		Detail:   detail,
	})
	if err != nil {
		return nil, err
	}
	if len(data) >= maxPayloadBytes {
		return nil, fmt.Errorf("problems: report of %d bytes is not under %d", len(data), maxPayloadBytes)
	}
	return data, nil
}

// marshalDocument encodes v as compact JSON without HTML escaping, so a value
// such as "<1w" stays readable in the dialog and in the request. The trailing
// newline of the encoder is removed.
func marshalDocument(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}

// Client returns ChairLift's v1 client string for a release version, such
// as "chairlift/0.14.0", or false for a version that is not a release
// <semver> (a dev or snapshot build).
func Client(version string) (string, bool) {
	client := "chairlift/" + version
	return client, clientRe.MatchString(client)
}

// NewReportID returns a new random lowercase UUIDv4 from crypto/rand.
func NewReportID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return formatUUIDv4(b), nil
}

// formatUUIDv4 sets the version and variant bits of b and formats it.
func formatUUIDv4(b [16]byte) string {
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
