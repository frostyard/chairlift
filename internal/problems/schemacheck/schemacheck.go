// Package schemacheck validates a decoded JSON document against the subset
// of JSON Schema draft 2020-12 that the vendored frostyard.problem schema
// uses: type, const, enum, pattern, required, properties,
// additionalProperties false, items, $ref, allOf with if/then, boolean
// schemas, minimum/maximum, minItems/maxItems and maxLength.
//
// A keyword outside that subset, a $ref that is not a local $defs
// reference, or an additionalProperties value other than false is an error,
// not a silent skip: a later schema file that uses a rule this package does
// not know fails loudly instead of passing.
//
// The package imports no GTK binding and no network package. It exists so
// that both internal/problems and internal/reporting can check a report
// body against the same pinned schema with one implementation
// (docs/specs/problem-reporting.md).
package schemacheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Validator is a loaded JSON Schema document.
type Validator struct {
	root map[string]any
}

// annotationKeywords carry no rule.
var annotationKeywords = map[string]bool{
	"$schema": true, "$id": true, "title": true, "description": true, "$defs": true,
}

// Load reads and decodes the schema document at path.
func Load(path string) (*Validator, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadData(data)
}

// LoadData decodes a schema document.
func LoadData(data []byte) (*Validator, error) {
	root, ok := Decode(data).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schemacheck: the schema is not a JSON object")
	}
	return &Validator{root: root}, nil
}

// Decode decodes 1 JSON value, keeping numbers as json.Number so an integer
// is not rounded through float64.
func Decode(data []byte) any {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil
	}
	if dec.More() {
		return nil
	}
	return v
}

// Document returns the schema's root object. Callers use it to read a
// property the way the schema writes it; they must not modify it.
func (v *Validator) Document() map[string]any {
	return v.root
}

// Def returns the $defs entry named name.
func (v *Validator) Def(name string) (map[string]any, bool) {
	defs, _ := v.root["$defs"].(map[string]any)
	def, ok := defs[name].(map[string]any)
	return def, ok
}

// Validate returns every error of instance against the root schema, or a
// structural error when the schema itself uses a rule outside the subset.
func (v *Validator) Validate(instance any) ([]string, error) {
	return v.check(v.root, instance, "$")
}

func (v *Validator) check(schema any, inst any, at string) ([]string, error) {
	switch s := schema.(type) {
	case bool:
		if s {
			return nil, nil
		}
		return []string{at + ": false schema"}, nil
	case map[string]any:
		return v.checkObject(s, inst, at)
	default:
		return nil, fmt.Errorf("schemacheck: schema at %s is %T", at, schema)
	}
}

func (v *Validator) checkObject(s map[string]any, inst any, at string) ([]string, error) {
	var errs []string
	for key := range s {
		switch key {
		case "type", "const", "enum", "pattern", "required", "properties", "additionalProperties",
			"items", "$ref", "allOf", "if", "then", "minimum", "maximum", "minItems", "maxItems", "maxLength":
		default:
			if !annotationKeywords[key] {
				return nil, fmt.Errorf("schemacheck: schema keyword %q at %s is not in the validator subset", key, at)
			}
		}
	}
	if ref, ok := s["$ref"].(string); ok {
		target, err := v.resolve(ref)
		if err != nil {
			return nil, err
		}
		sub, err := v.check(target, inst, at)
		if err != nil {
			return nil, err
		}
		errs = append(errs, sub...)
	}
	if typ, ok := s["type"].(string); ok && !hasType(inst, typ) {
		return append(errs, fmt.Sprintf("%s: not of type %s", at, typ)), nil
	}
	if c, ok := s["const"]; ok && !equal(c, inst) {
		errs = append(errs, fmt.Sprintf("%s: not the const %v", at, c))
	}
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if equal(e, inst) {
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
		if n, ok := s["maxLength"].(json.Number); ok {
			limit, err := n.Int64()
			if err != nil {
				return nil, fmt.Errorf("schemacheck: maxLength at %s is not an integer", at)
			}
			if int64(utf8.RuneCountInString(str)) > limit {
				errs = append(errs, fmt.Sprintf("%s: longer than %s", at, n))
			}
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
		if n, ok := s["minItems"].(json.Number); ok {
			limit, err := n.Int64()
			if err != nil {
				return nil, fmt.Errorf("schemacheck: minItems at %s is not an integer", at)
			}
			if int64(len(arr)) < limit {
				errs = append(errs, fmt.Sprintf("%s: fewer than %s items", at, n))
			}
		}
		if n, ok := s["maxItems"].(json.Number); ok {
			limit, err := n.Int64()
			if err != nil {
				return nil, fmt.Errorf("schemacheck: maxItems at %s is not an integer", at)
			}
			if int64(len(arr)) > limit {
				errs = append(errs, fmt.Sprintf("%s: more than %s items", at, n))
			}
		}
		if items, ok := s["items"]; ok {
			for i, e := range arr {
				sub, err := v.check(items, e, fmt.Sprintf("%s[%d]", at, i))
				if err != nil {
					return nil, err
				}
				errs = append(errs, sub...)
			}
		}
	}
	if obj, ok := inst.(map[string]any); ok {
		if req, ok := s["required"].([]any); ok {
			for _, r := range req {
				name, _ := r.(string)
				if _, present := obj[name]; !present {
					errs = append(errs, fmt.Sprintf("%s: missing %s", at, r))
				}
			}
		}
		props, _ := s["properties"].(map[string]any)
		for name, sub := range props {
			if value, present := obj[name]; present {
				subErrs, err := v.check(sub, value, at+"."+name)
				if err != nil {
					return nil, err
				}
				errs = append(errs, subErrs...)
			}
		}
		if ap, ok := s["additionalProperties"]; ok {
			if ap != false {
				return nil, fmt.Errorf("schemacheck: additionalProperties at %s is %v; the subset knows only false", at, ap)
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
			subErrs, err := v.check(sub, inst, fmt.Sprintf("%s(allOf %d)", at, i))
			if err != nil {
				return nil, err
			}
			errs = append(errs, subErrs...)
		}
	}
	if cond, ok := s["if"]; ok {
		condErrs, err := v.check(cond, inst, at)
		if err != nil {
			return nil, err
		}
		if len(condErrs) == 0 {
			if then, ok := s["then"]; ok {
				subErrs, err := v.check(then, inst, at)
				if err != nil {
					return nil, err
				}
				errs = append(errs, subErrs...)
			}
		}
	}
	return errs, nil
}

// resolve follows a local $ref such as "#/$defs/count".
func (v *Validator) resolve(ref string) (any, error) {
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, fmt.Errorf("schemacheck: $ref %q is not a local $defs reference", ref)
	}
	defs, _ := v.root["$defs"].(map[string]any)
	target, ok := defs[strings.TrimPrefix(ref, prefix)]
	if !ok {
		return nil, fmt.Errorf("schemacheck: $ref %q has no target", ref)
	}
	return target, nil
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

// equal compares 2 decoded JSON values; numbers compare by value.
func equal(a, b any) bool {
	an, aok := a.(json.Number)
	bn, bok := b.(json.Number)
	if aok && bok {
		return compareNumbers(an, bn) == 0
	}
	return reflect.DeepEqual(a, b)
}
