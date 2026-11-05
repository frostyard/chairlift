package problems

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// subsetValidator is a JSON Schema validator for the subset of draft
// 2020-12 that core's v1.json uses: type, const, enum, pattern, required,
// properties, additionalProperties false, items, $ref, allOf with if/then,
// boolean schemas, minimum/maximum, minItems/maxItems and maxLength. A
// keyword it does not know fails the test, so it cannot silently skip a
// rule of a later schema file.
type subsetValidator struct {
	t    *testing.T
	root map[string]any
}

// annotationKeywords carry no rule.
var annotationKeywords = map[string]bool{
	"$schema": true, "$id": true, "title": true, "description": true, "$defs": true,
}

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

func loadSchema(t *testing.T) *subsetValidator {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "schema", "v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	root, ok := decodeJSONNumber(t, data).(map[string]any)
	if !ok {
		t.Fatal("v1.json is not an object")
	}
	return &subsetValidator{t: t, root: root}
}

// Validate returns the errors of instance against the root schema.
func (v *subsetValidator) Validate(instance any) []string {
	return v.check(v.root, instance, "$")
}

func (v *subsetValidator) check(schema any, inst any, at string) []string {
	switch s := schema.(type) {
	case bool:
		if s {
			return nil
		}
		return []string{at + ": false schema"}
	case map[string]any:
		return v.checkObject(s, inst, at)
	default:
		v.t.Fatalf("schema at %s is %T", at, schema)
		return nil
	}
}

func (v *subsetValidator) checkObject(s map[string]any, inst any, at string) []string {
	var errs []string
	for key := range s {
		switch key {
		case "type", "const", "enum", "pattern", "required", "properties", "additionalProperties",
			"items", "$ref", "allOf", "if", "then", "minimum", "maximum", "minItems", "maxItems", "maxLength":
		default:
			if !annotationKeywords[key] {
				v.t.Fatalf("schema keyword %q at %s is not in the validator subset", key, at)
			}
		}
	}
	if ref, ok := s["$ref"].(string); ok {
		errs = append(errs, v.check(v.resolve(ref), inst, at)...)
	}
	if typ, ok := s["type"].(string); ok && !hasType(inst, typ) {
		return append(errs, fmt.Sprintf("%s: not of type %s", at, typ))
	}
	if c, ok := s["const"]; ok && !jsonEqual(c, inst) {
		errs = append(errs, fmt.Sprintf("%s: not the const %v", at, c))
	}
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if jsonEqual(e, inst) {
				found = true
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("%s: %v not in enum", at, inst))
		}
	}
	if str, ok := inst.(string); ok {
		if p, ok := s["pattern"].(string); ok && !regexp.MustCompile(p).MatchString(str) {
			errs = append(errs, fmt.Sprintf("%s: %q does not match %s", at, str, p))
		}
		if n, ok := s["maxLength"].(json.Number); ok && int64(utf8.RuneCountInString(str)) > mustInt(v.t, n) {
			errs = append(errs, fmt.Sprintf("%s: longer than %s", at, n))
		}
	}
	if num, ok := inst.(json.Number); ok {
		if n, ok := s["minimum"].(json.Number); ok && compareNumbers(num, n) < 0 {
			errs = append(errs, fmt.Sprintf("%s: below %s", at, n))
		}
		if n, ok := s["maximum"].(json.Number); ok && compareNumbers(num, n) > 0 {
			errs = append(errs, fmt.Sprintf("%s: above %s", at, n))
		}
	}
	if arr, ok := inst.([]any); ok {
		if n, ok := s["minItems"].(json.Number); ok && int64(len(arr)) < mustInt(v.t, n) {
			errs = append(errs, fmt.Sprintf("%s: fewer than %s items", at, n))
		}
		if n, ok := s["maxItems"].(json.Number); ok && int64(len(arr)) > mustInt(v.t, n) {
			errs = append(errs, fmt.Sprintf("%s: more than %s items", at, n))
		}
		if items, ok := s["items"]; ok {
			for i, e := range arr {
				errs = append(errs, v.check(items, e, fmt.Sprintf("%s[%d]", at, i))...)
			}
		}
	}
	if obj, ok := inst.(map[string]any); ok {
		if req, ok := s["required"].([]any); ok {
			for _, r := range req {
				if _, present := obj[r.(string)]; !present {
					errs = append(errs, fmt.Sprintf("%s: missing %s", at, r))
				}
			}
		}
		props, _ := s["properties"].(map[string]any)
		for name, sub := range props {
			if value, present := obj[name]; present {
				errs = append(errs, v.check(sub, value, at+"."+name)...)
			}
		}
		if ap, ok := s["additionalProperties"]; ok {
			if ap != false {
				v.t.Fatalf("additionalProperties at %s is %v; the subset knows only false", at, ap)
			}
			for name := range obj {
				if _, named := props[name]; !named {
					errs = append(errs, fmt.Sprintf("%s: field %s is not in the schema", at, name))
				}
			}
		}
	}
	if all, ok := s["allOf"].([]any); ok {
		for i, sub := range all {
			errs = append(errs, v.check(sub, inst, fmt.Sprintf("%s(allOf %d)", at, i))...)
		}
	}
	if cond, ok := s["if"]; ok {
		if len(v.check(cond, inst, at)) == 0 {
			if then, ok := s["then"]; ok {
				errs = append(errs, v.check(then, inst, at)...)
			}
		}
	}
	return errs
}

func (v *subsetValidator) resolve(ref string) any {
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		v.t.Fatalf("$ref %q is not a local $defs reference", ref)
	}
	defs, _ := v.root["$defs"].(map[string]any)
	target, ok := defs[strings.TrimPrefix(ref, prefix)]
	if !ok {
		v.t.Fatalf("$ref %q has no target", ref)
	}
	return target
}

func hasType(inst any, typ string) bool {
	switch typ {
	case "object":
		_, ok := inst.(map[string]any)
		return ok
	case "string":
		_, ok := inst.(string)
		return ok
	case "boolean":
		_, ok := inst.(bool)
		return ok
	case "array":
		_, ok := inst.([]any)
		return ok
	case "integer":
		n, ok := inst.(json.Number)
		if !ok {
			return false
		}
		r, ok := new(big.Rat).SetString(n.String())
		return ok && r.IsInt()
	default:
		return false
	}
}

func compareNumbers(a, b json.Number) int {
	x, _ := new(big.Rat).SetString(a.String())
	y, _ := new(big.Rat).SetString(b.String())
	return x.Cmp(y)
}

func mustInt(t *testing.T, n json.Number) int64 {
	i, err := n.Int64()
	if err != nil {
		t.Fatalf("%s is not an integer", n)
	}
	return i
}

// jsonEqual compares 2 decoded JSON values; numbers compare by value.
func jsonEqual(a, b any) bool {
	an, aok := a.(json.Number)
	bn, bok := b.(json.Number)
	if aok && bok {
		return compareNumbers(an, bn) == 0
	}
	return reflect.DeepEqual(a, b)
}

// schemaEnum returns the enum of the property at a path of property names
// under a $defs entry (or the root when def is "").
func schemaEnum(t *testing.T, v *subsetValidator, def, property string) []string {
	t.Helper()
	node := v.root
	if def != "" {
		node = v.resolve("#/$defs/" + def).(map[string]any)
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
func schemaPattern(t *testing.T, v *subsetValidator, def, property string) string {
	t.Helper()
	node := v.root
	if def != "" {
		node = v.resolve("#/$defs/" + def).(map[string]any)
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
		m = v.resolve(ref).(map[string]any)
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
