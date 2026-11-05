package problems

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/frostyard/chairlift/internal/problems/schemacheck"
)

// The subset validator that checks a report document against the vendored
// v1.json lives in internal/problems/schemacheck, so internal/reporting
// checks its bodies with the same implementation.

// decodeJSONNumber decodes data as 1 JSON value with json.Number, so an
// integer is not rounded through float64.
func decodeJSONNumber(t *testing.T, data []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if dec.More() {
		t.Fatal("trailing data after the JSON value")
	}
	return v
}

// loadSchema loads the vendored v1.json into a validator.
func loadSchema(t *testing.T) *schemacheck.Validator {
	t.Helper()
	v, err := schemacheck.Load(filepath.Join("testdata", "schema", "v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// validate returns the schema errors of instance, failing the test on a
// structural problem in the schema itself.
func validate(t *testing.T, v *schemacheck.Validator, instance any) []string {
	t.Helper()
	errs, err := v.Validate(instance)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	return errs
}

// schemaEnum returns the enum of the property at a path of property names
// under a $defs entry (or the root when def is "").
func schemaEnum(t *testing.T, v *schemacheck.Validator, def, property string) []string {
	t.Helper()
	node := v.Document()
	if def != "" {
		named, ok := v.Def(def)
		if !ok {
			t.Fatalf("schema has no $defs entry %s", def)
		}
		node = named
	}
	props, _ := node["properties"].(map[string]any)
	prop, ok := props[property].(map[string]any)
	if !ok {
		t.Fatalf("schema has no property %s.%s", def, property)
	}
	if def == "" && property == "image" {
		prop = prop["properties"].(map[string]any)["id"].(map[string]any)
	}
	raw, ok := prop["enum"].([]any)
	if !ok {
		t.Fatalf("%s.%s has no enum", def, property)
	}
	out := make([]string, len(raw))
	for i, e := range raw {
		out[i] = e.(string)
	}
	return out
}

// schemaPattern returns the pattern of a property, following $ref and the
// items of an array.
func schemaPattern(t *testing.T, v *schemacheck.Validator, def, property string) string {
	t.Helper()
	node := v.Document()
	if def != "" {
		named, ok := v.Def(def)
		if !ok {
			t.Fatalf("schema has no $defs entry %s", def)
		}
		node = named
	}
	var prop any = node
	if property != "" {
		prop = node["properties"].(map[string]any)[property]
	}
	m, ok := prop.(map[string]any)
	if !ok {
		t.Fatalf("no schema for %s.%s", def, property)
	}
	if ref, ok := m["$ref"].(string); ok {
		const prefix = "#/$defs/"
		named, found := v.Def(ref[len(prefix):])
		if !found {
			t.Fatalf("$ref %q has no target", ref)
		}
		m = named
	}
	if items, ok := m["items"].(map[string]any); ok {
		m = items
	}
	p, ok := m["pattern"].(string)
	if !ok {
		t.Fatalf("%s.%s has no pattern", def, property)
	}
	return p
}
