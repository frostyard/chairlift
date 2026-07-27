package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// parseAndValidate is the package-private entry point tying together the
// four validation stages built up across this slice: stage 1
// (parseYAMLDocument, source.go) parses data as YAML; stage 2
// (resolveEffective, effective.go) validates the resulting source graph
// (validateSourceGraph, sourcegraph.go, including its
// checkSourceGraphBounds pass, sourcebounds.go) and, on success, emits a
// fresh alias-free, merge-resolved effective document; this function's own
// document-level shape check (stage 3's document-level slice) classifies
// what kind of top-level node the effective document has; and stage 4
// decodes an accepted mapping document into a *rawConfig.
//
// A stage 1 or stage 2 *LoadError is returned unchanged: this entry point
// can never bypass parser, source-graph, or bound validation by skipping
// past their errors.
//
// Document-level outcomes, once stage 1 and stage 2 both succeed:
//
//   - A nil effective document — parseYAMLDocument's and resolveEffective's
//     shared "empty or whitespace-only input" result — returns a non-nil,
//     zero-valued *rawConfig (every page map nil/empty) and no error: it is
//     a usable no-op overlay, matching the "absent config" semantics
//     elsewhere in this package. Stage 4's Decode is skipped entirely for
//     this case.
//   - A top-level scalar resolving to YAML's null tag ("!!null" — as
//     produced by an empty document's `null`/`~`/absent value at the
//     document root) is treated exactly like the nil-document case above:
//     a non-nil, zero-valued *rawConfig, no error, and no Decode call.
//   - A top-level scalar that is not null, or a top-level sequence, is
//     rejected as a KindParseType *LoadError (validatorShapeError) naming
//     the expected top-level mapping shape and a positive source line
//     (effectiveNodeLine). Stage 4's Decode is skipped for this case too:
//     there is no mapping to decode.
//   - A top-level mapping has each of its entries classified in effective
//     order by validatePageEntries: a mapping key whose effective node is
//     not a scalar with ShortTag() == "!!str" is KindParseType
//     (validatorKeyShapeError) before its value is ever inspected; a
//     well-formed string key that is not one of SchemaPages()'s canonical
//     page names is KindSchema (validatorSchemaError) without descending
//     into its value; a known page's null value is a no-op for that page; a
//     known page's non-null scalar or sequence value is KindParseType
//     (validatorPageValueShapeError); and a known page's mapping value is
//     accepted structurally (its own entries — groups and fields — are
//     classified starting at a later chunk). Once every entry passes, stage
//     4 decodes the effective document into a *rawConfig via
//     yaml.Node.Decode.
//
// Stage 4 is a defensive final step (interpretation I4): validateSourceGraph
// and resolveEffective already prove the effective document is a
// well-formed, alias-free, merge-resolved graph rooted at a mapping by the
// time Decode runs, so a Decode failure here is not expected to be
// reachable from ordinary input, but if yaml.v3's own Decode nonetheless
// reports an error (e.g. a genuine type mismatch decoding into rawConfig's
// field types), it is classified KindParseType with the yaml error
// preserved in Err (validatorDecodeError) rather than left unhandled.
func parseAndValidate(path string, data []byte) (*rawConfig, *LoadError) {
	doc, err := parseYAMLDocument(path, data)
	if err != nil {
		return nil, err
	}

	effective, err := resolveEffective(path, doc)
	if err != nil {
		return nil, err
	}

	if effective == nil {
		// Empty or whitespace-only input: a usable no-op overlay.
		return &rawConfig{}, nil
	}

	top := effective.Content[0]
	switch {
	case top.Kind == yaml.ScalarNode && top.Tag == "!!null":
		// A top-level null document (`null`, `~`, or an absent scalar
		// value) is a no-op overlay too, just like the nil-document case.
		return &rawConfig{}, nil
	case top.Kind == yaml.MappingNode:
		if err := validatePageEntries(path, top); err != nil {
			return nil, err
		}
		var raw rawConfig
		if err := effective.Decode(&raw); err != nil {
			return nil, validatorDecodeError(path, err)
		}
		return &raw, nil
	default:
		return nil, validatorShapeError(path, top)
	}
}

// validatePageEntries walks top's entries (top is the effective document's
// top-level mapping node) in effective Content order and classifies each
// one per interpretation I3's fixed order: (a) key shape — schemaKeyName
// requires a scalar key whose effective ShortTag() is exactly "!!str", else
// the entry is KindParseType before its name or value is inspected; (b)
// name membership — a well-formed string key that is not one of
// SchemaPages()'s canonical page names is KindSchema without descending
// into its value; (c) only then is the page's value shape inspected. The
// first failing entry's error is returned; nil means every entry passed.
//
// Page names are sourced only from SchemaPages() (reflected off Config's
// yaml tags via schema.go) — never a literal list here. SchemaPages() can
// only fail for a malformed struct tag on Config itself, which cannot
// happen for that canonical struct; that branch is still surfaced
// defensively as a KindSchema *LoadError wrapping the reflect error
// (validatorSchemaPagesError) rather than ignored or panicked on.
func validatePageEntries(path string, top *yaml.Node) *LoadError {
	pages, err := SchemaPages()
	if err != nil {
		return validatorSchemaPagesError(path, err)
	}
	known := make(map[string]bool, len(pages))
	for _, page := range pages {
		known[page] = true
	}

	for i := 0; i+1 < len(top.Content); i += 2 {
		key := top.Content[i]
		value := top.Content[i+1]

		name, ok := schemaKeyName(key)
		if !ok {
			return validatorKeyShapeError(path, key)
		}
		if !known[name] {
			return validatorSchemaError(path, key, name)
		}

		switch value.Kind {
		case yaml.ScalarNode:
			if value.Tag == "!!null" {
				continue // known page with a null value: a no-op for that page
			}
			return validatorPageValueShapeError(path, name, value)
		case yaml.SequenceNode:
			return validatorPageValueShapeError(path, name, value)
		case yaml.MappingNode:
			continue // accepted structurally; entries classified from c3 on
		default:
			return validatorPageValueShapeError(path, name, value)
		}
	}

	return nil
}

// schemaKeyName implements the spec's name rule for a mapping key: key is a
// name only when it is a yaml.ScalarNode whose effective ShortTag() is
// exactly "!!str", in which case its Value is returned with ok == true.
// Every other key shape — an integer, boolean, or null scalar; a
// custom-tagged scalar; a sequence; a mapping; or an alias to any of those
// (aliases are already dereferenced into copies of their targets by
// resolveEffective, so this rule needs no alias-specific case) — returns
// ("", false). yaml.v3's own ability to coerce a non-!!str scalar into a Go
// string (e.g. decoding an integer key into a string field) does not make
// that key a name here: the rule is the node's own resolved tag, not what
// yaml.v3 could decode it into.
func schemaKeyName(key *yaml.Node) (string, bool) {
	if key.Kind == yaml.ScalarNode && key.ShortTag() == "!!str" {
		return key.Value, true
	}
	return "", false
}

// validatorSchemaError builds a KindSchema *LoadError reporting that node
// (a mapping key) names an unrecognized name. Detail names the literal
// offending name and a positive source line (effectiveNodeLine). There is
// no underlying yaml.v3 error to preserve — this is a validator-side name
// rejection — so, like validatorShapeError's other callers, Err is left
// nil.
func validatorSchemaError(path string, node *yaml.Node, name string) *LoadError {
	return &LoadError{
		Path:   path,
		Kind:   KindSchema,
		Detail: fmt.Sprintf("unknown name %q (line %d)", name, effectiveNodeLine(node)),
	}
}

// validatorSchemaPagesError builds a KindSchema *LoadError wrapping a
// SchemaPages() error. SchemaPages() can only fail for a malformed struct
// tag on Config itself (schema.go's yamlFieldNames), which cannot happen
// for that canonical, hand-maintained struct; this exists so that
// theoretical failure is still classified rather than ignored or panicked
// on.
func validatorSchemaPagesError(path string, err error) *LoadError {
	return &LoadError{
		Path:   path,
		Kind:   KindSchema,
		Detail: fmt.Sprintf("could not determine canonical page names: %v", err),
		Err:    err,
	}
}

// validatorKeyShapeError builds a KindParseType *LoadError reporting that
// key is not a YAML string (schemaKeyName's ("", false) result). Detail
// names the actual node shape found and a positive source line
// (effectiveNodeLine). There is no underlying yaml.v3 error to preserve —
// this is a validator-side shape rejection — so Err is left nil, matching
// validatorShapeError.
func validatorKeyShapeError(path string, key *yaml.Node) *LoadError {
	return sourceGraphError(path, fmt.Sprintf(
		"mapping key must be a YAML string, found %s (line %d)",
		describeNodeShape(key), effectiveNodeLine(key),
	))
}

// validatorPageValueShapeError builds a KindParseType *LoadError reporting
// that the known page named page has a value that is neither null nor a
// mapping (i.e. a non-null scalar or a sequence). Detail names the page,
// the actual node shape found, and a positive source line
// (effectiveNodeLine).
func validatorPageValueShapeError(path, page string, value *yaml.Node) *LoadError {
	return sourceGraphError(path, fmt.Sprintf(
		"page %q must be null or a YAML mapping, found %s (line %d)",
		page, describeNodeShape(value), effectiveNodeLine(value),
	))
}

// describeNodeShape returns a short human-readable description of n's kind,
// used by the validator's shape-rejection error builders. For a scalar it
// includes the resolved tag (e.g. "a YAML scalar (!!int)") since "found a
// YAML scalar" alone would not distinguish an unwanted !!int key from an
// unwanted !!bool key.
func describeNodeShape(n *yaml.Node) string {
	switch n.Kind {
	case yaml.SequenceNode:
		return "a YAML sequence"
	case yaml.MappingNode:
		return "a YAML mapping"
	case yaml.ScalarNode:
		return fmt.Sprintf("a YAML scalar (%s)", n.ShortTag())
	default:
		return fmt.Sprintf("a YAML node of kind %d", n.Kind)
	}
}

// validatorShapeError builds a KindParseType *LoadError reporting that
// top — the effective document's top-level node — is not a YAML mapping
// (and, per parseAndValidate's null-scalar no-op case, is not treated as
// absent either). Detail names the actual node shape found and
// effectiveNodeLine(top)'s clamped, always-positive source line. There is
// no underlying yaml.v3 error to preserve here — this is a validator-side
// shape rejection, not a parser failure — so, like sourceGraphError's
// other callers, Err is left nil.
func validatorShapeError(path string, top *yaml.Node) *LoadError {
	return sourceGraphError(path, fmt.Sprintf(
		"top-level document must be a YAML mapping, found %s (line %d)",
		describeNodeShape(top), effectiveNodeLine(top),
	))
}

// validatorDecodeError builds the KindParseType *LoadError parseAndValidate
// returns when yaml.Node.Decode fails to decode an accepted top-level
// mapping document into a *rawConfig. Unlike validatorShapeError, this
// wraps a real yaml.v3 error (typically a *yaml.TypeError): Detail is that
// error's own message and Err preserves it, matching parseFailure's
// (source.go) treatment of a stage 1 parser error.
func validatorDecodeError(path string, err error) *LoadError {
	return &LoadError{
		Path:   path,
		Kind:   KindParseType,
		Detail: err.Error(),
		Err:    err,
	}
}

// effectiveNodeLine returns n's Line, clamped to 1 when non-positive. It
// mirrors effectiveOutputLimitError's (effective.go) own clamp, applied
// there to a line looked up from attributeSourceLines: a node's Line can be
// zero (or, in principle for a synthetic node, negative) when yaml.v3 left
// it unset, and a reported "line 0" or negative line would be a confusing
// diagnostic, so the lowest value ever reported here is 1.
func effectiveNodeLine(n *yaml.Node) int {
	if n.Line <= 0 {
		return 1
	}
	return n.Line
}
