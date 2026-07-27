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
//   - A top-level mapping is accepted structurally; stage 4 then decodes
//     the effective document into a *rawConfig via yaml.Node.Decode.
//     Per-entry classification of page/group/action content within an
//     accepted mapping starts at a later chunk ("c2 on") and is out of
//     this function's scope.
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
		var raw rawConfig
		if err := effective.Decode(&raw); err != nil {
			return nil, validatorDecodeError(path, err)
		}
		return &raw, nil
	default:
		return nil, validatorShapeError(path, top)
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
	var found string
	switch top.Kind {
	case yaml.SequenceNode:
		found = "a YAML sequence"
	case yaml.ScalarNode:
		found = "a YAML scalar"
	default:
		found = fmt.Sprintf("a YAML node of kind %d", top.Kind)
	}
	return sourceGraphError(path, fmt.Sprintf(
		"top-level document must be a YAML mapping, found %s (line %d)",
		found, effectiveNodeLine(top),
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
