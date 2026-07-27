package config

import (
	"fmt"
	"reflect"

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
//     (validatorPageValueShapeError); and a known page's mapping value has
//     its own entries (groups) classified the same way, one level down, by
//     validateGroupEntries, whose own known groups' mapping values have
//     their entries (group fields) classified by validateGroupFieldEntries.
//     A group or field name unknown to SchemaGroups/groupFieldTypes is
//     KindSchema; a group's non-null scalar/sequence value is KindParseType
//     (validatorGroupValueShapeError); a known non-"actions" field is
//     decoded into a fresh value of its declared Go type (groupFieldTypes),
//     a decode failure being KindParseType with the yaml.v3 error preserved
//     in Err. "actions" is recognized as known but is validated structurally
//     instead of by a generic decode (I5): its value must be null (a no-op)
//     or a sequence, else KindParseType (validatorActionsValueShapeError);
//     every sequence entry must be a YAML mapping — a null entry is
//     explicitly not a zero action — else KindParseType
//     (validatorActionEntryShapeError); and each action entry's own fields
//     are classified exactly like a group's fields, one level down, by
//     validateActionFieldEntries: key shape and name membership against
//     SchemaActionFields()/actionFieldTypes (sourced from ActionConfig),
//     then a known field decoded into a fresh value of its declared Go type,
//     a decode failure being KindParseType with the yaml.v3 error preserved
//     in Err. Once every entry at every level passes, stage 4 decodes the
//     effective document into a *rawConfig via yaml.Node.Decode.
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
			if err := validateGroupEntries(path, name, value); err != nil {
				return err
			}
		default:
			return validatorPageValueShapeError(path, name, value)
		}
	}

	return nil
}

// validateGroupEntries walks groupsNode's entries (groupsNode is a known
// page's mapping value, so each entry is a group) exactly like
// validatePageEntries does for pages, one schema level down: key shape via
// schemaKeyName, then name membership against SchemaGroups(page)'s
// canonical group names for this page (never a literal list here), then
// value shape (null is a no-op; scalar/sequence is KindParseType; a mapping
// descends into validateGroupFieldEntries). The first failing entry's error
// is returned; nil means every entry passed. SchemaGroups can only fail for
// a page name unknown to it, which cannot happen here since page was
// already validated against SchemaPages(); that branch is still surfaced
// defensively as KindSchema (validatorSchemaGroupsError).
func validateGroupEntries(path, page string, groupsNode *yaml.Node) *LoadError {
	groups, err := SchemaGroups(page)
	if err != nil {
		return validatorSchemaGroupsError(path, err)
	}
	known := make(map[string]bool, len(groups))
	for _, group := range groups {
		known[group] = true
	}

	for i := 0; i+1 < len(groupsNode.Content); i += 2 {
		key := groupsNode.Content[i]
		value := groupsNode.Content[i+1]

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
				continue // known group with a null value: a no-op for that group
			}
			return validatorGroupValueShapeError(path, name, value)
		case yaml.SequenceNode:
			return validatorGroupValueShapeError(path, name, value)
		case yaml.MappingNode:
			if err := validateGroupFieldEntries(path, name, value); err != nil {
				return err
			}
		default:
			return validatorGroupValueShapeError(path, name, value)
		}
	}

	return nil
}

// validateGroupFieldEntries walks fieldsNode's entries (fieldsNode is a
// known group's mapping value, so each entry is a group field): key shape
// via schemaKeyName, then name membership against groupFieldTypes()'s
// canonical field names (never a literal list here, aside from the literal
// "actions" itself). The special-cased "actions" name is recognized as known
// but is validated structurally by validateActionsEntries (I5) instead of by
// a generic decode into its declared Go type. Every other known field's
// effective value node is decoded into a fresh value of its declared Go
// type (reflect.New(fieldType).Interface(), I4); a decode failure is
// KindParseType with the real yaml.v3 error preserved in Err
// (validatorDecodeError, matching stage 4's own decode-failure handling).
// The first failing entry's error is returned; nil means every entry
// passed.
func validateGroupFieldEntries(path, group string, fieldsNode *yaml.Node) *LoadError {
	fieldTypes, err := groupFieldTypes()
	if err != nil {
		return validatorGroupFieldTypesError(path, err)
	}

	for i := 0; i+1 < len(fieldsNode.Content); i += 2 {
		key := fieldsNode.Content[i]
		value := fieldsNode.Content[i+1]

		name, ok := schemaKeyName(key)
		if !ok {
			return validatorKeyShapeError(path, key)
		}
		fieldType, known := fieldTypes[name]
		if !known {
			return validatorSchemaError(path, key, name)
		}
		if name == "actions" {
			if err := validateActionsEntries(path, value); err != nil {
				return err
			}
			continue
		}

		target := reflect.New(fieldType)
		if err := value.Decode(target.Interface()); err != nil {
			return validatorDecodeError(path, err)
		}
	}

	return nil
}

// validateActionsEntries implements the structural walk for a known
// "actions" field's value (I5): null is a no-op, a sequence has every entry
// validated by validateActionFieldEntries, and any other shape (non-null
// scalar or mapping) is KindParseType. This is deliberately not a generic
// decode into []ActionConfig — yaml.v3 would silently accept a null entry
// as a zero ActionConfig and silently ignore an unknown action field.
func validateActionsEntries(path string, actionsNode *yaml.Node) *LoadError {
	switch actionsNode.Kind {
	case yaml.ScalarNode:
		if actionsNode.Tag == "!!null" {
			return nil // no actions configured: a no-op
		}
		return validatorActionsValueShapeError(path, actionsNode)
	case yaml.SequenceNode:
		for _, entry := range actionsNode.Content {
			if entry.Kind != yaml.MappingNode {
				// A null entry is explicitly not a zero action: it must
				// be a mapping, like every other non-mapping shape.
				return validatorActionEntryShapeError(path, entry)
			}
			if err := validateActionFieldEntries(path, entry); err != nil {
				return err
			}
		}
		return nil
	default:
		return validatorActionsValueShapeError(path, actionsNode)
	}
}

// validateActionFieldEntries walks entryNode's entries (one action-sequence
// entry, already confirmed to be a mapping): key shape via schemaKeyName,
// name membership against actionFieldTypes()'s canonical names (from
// ActionConfig, I7), then a known field's value decoded into
// reflect.New(fieldType), a decode failure being KindParseType with the
// real yaml.v3 error preserved (I4) — matching validateGroupFieldEntries'
// non-"actions" handling one level down. The first failing entry's error is
// returned; nil means every entry passed.
func validateActionFieldEntries(path string, entryNode *yaml.Node) *LoadError {
	fieldTypes, err := actionFieldTypes()
	if err != nil {
		return validatorActionFieldTypesError(path, err)
	}

	for i := 0; i+1 < len(entryNode.Content); i += 2 {
		key := entryNode.Content[i]
		value := entryNode.Content[i+1]

		name, ok := schemaKeyName(key)
		if !ok {
			return validatorKeyShapeError(path, key)
		}
		fieldType, known := fieldTypes[name]
		if !known {
			return validatorSchemaError(path, key, name)
		}

		target := reflect.New(fieldType)
		if err := value.Decode(target.Interface()); err != nil {
			return validatorDecodeError(path, err)
		}
	}

	return nil
}

// actionFieldTypes reflects over ActionConfig's exported fields, returning
// a freshly allocated map from each field's yaml tag name to its declared
// Go type — the type-carrying counterpart to SchemaActionFields (schema.go),
// matching groupFieldTypes one schema level down.
func actionFieldTypes() (map[string]reflect.Type, error) {
	t := reflect.TypeOf(ActionConfig{})
	result := make(map[string]reflect.Type, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}

		tag, ok := field.Tag.Lookup("yaml")
		if !ok || tag == "" || tag == "-" {
			continue
		}

		name := yamlTagName(tag)
		if name == "" {
			return nil, fmt.Errorf("actionFieldTypes: field %s has a yaml tag with an empty name (%q)", field.Name, tag)
		}
		if _, dup := result[name]; dup {
			return nil, fmt.Errorf("actionFieldTypes: duplicate yaml name %q (field %s)", name, field.Name)
		}
		result[name] = field.Type
	}

	return result, nil
}

// groupFieldTypes reflects over rawGroupConfig's exported fields, returning
// a freshly allocated map from each field's yaml tag name to its declared
// Go type. It is the type-carrying counterpart to SchemaGroupFields
// (schema.go), built by walking the same struct's fields so the two cannot
// silently drift apart. It errors under the same conditions yamlFieldNames
// does, which cannot happen for rawGroupConfig's canonical field list.
func groupFieldTypes() (map[string]reflect.Type, error) {
	t := reflect.TypeOf(rawGroupConfig{})
	result := make(map[string]reflect.Type, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}

		tag, ok := field.Tag.Lookup("yaml")
		if !ok || tag == "" || tag == "-" {
			continue
		}

		name := yamlTagName(tag)
		if name == "" {
			return nil, fmt.Errorf("groupFieldTypes: field %s has a yaml tag with an empty name (%q)", field.Name, tag)
		}
		if _, dup := result[name]; dup {
			return nil, fmt.Errorf("groupFieldTypes: duplicate yaml name %q (field %s)", name, field.Name)
		}
		result[name] = field.Type
	}

	return result, nil
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

// validatorSchemaGroupsError builds a KindSchema *LoadError wrapping a
// SchemaGroups(page) error, matching validatorSchemaPagesError one level
// down; SchemaGroups can only fail for a page unknown to it, which cannot
// happen once page has passed SchemaPages() validation.
func validatorSchemaGroupsError(path string, err error) *LoadError {
	return &LoadError{
		Path:   path,
		Kind:   KindSchema,
		Detail: fmt.Sprintf("could not determine canonical group names: %v", err),
		Err:    err,
	}
}

// validatorGroupFieldTypesError builds a KindSchema *LoadError wrapping a
// groupFieldTypes error, matching validatorSchemaPagesError; it cannot
// happen for rawGroupConfig's canonical, hand-maintained field list.
func validatorGroupFieldTypesError(path string, err error) *LoadError {
	return &LoadError{
		Path:   path,
		Kind:   KindSchema,
		Detail: fmt.Sprintf("could not determine canonical group field types: %v", err),
		Err:    err,
	}
}

// validatorActionFieldTypesError builds a KindSchema *LoadError wrapping an
// actionFieldTypes error, matching validatorGroupFieldTypesError one schema
// level down; it cannot happen for ActionConfig's canonical, hand-maintained
// field list.
func validatorActionFieldTypesError(path string, err error) *LoadError {
	return &LoadError{
		Path:   path,
		Kind:   KindSchema,
		Detail: fmt.Sprintf("could not determine canonical action field types: %v", err),
		Err:    err,
	}
}

// validatorActionsValueShapeError builds a KindParseType *LoadError
// reporting that a known "actions" field's value is neither null nor a
// sequence (i.e. a non-null scalar or a mapping). Detail names the actual
// node shape found and a positive source line, matching
// validatorGroupValueShapeError's pattern one schema level down.
func validatorActionsValueShapeError(path string, value *yaml.Node) *LoadError {
	return sourceGraphError(path, fmt.Sprintf(
		"field %q must be null or a YAML sequence, found %s (line %d)",
		"actions", describeNodeShape(value), effectiveNodeLine(value),
	))
}

// validatorActionEntryShapeError builds a KindParseType *LoadError
// reporting that an "actions" sequence entry is not a YAML mapping. A null
// entry is deliberately included here (not treated as a zero action):
// entry's own actual node shape (including "a YAML scalar (!!null)" for a
// null entry) and a positive source line are named in Detail.
func validatorActionEntryShapeError(path string, entry *yaml.Node) *LoadError {
	return sourceGraphError(path, fmt.Sprintf(
		"action entry must be a YAML mapping, found %s (line %d)",
		describeNodeShape(entry), effectiveNodeLine(entry),
	))
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

// validatorGroupValueShapeError builds a KindParseType *LoadError reporting
// that the known group named group has a value that is neither null nor a
// mapping. Detail names the group, the actual node shape found, and a
// positive source line, matching validatorPageValueShapeError one level
// down.
func validatorGroupValueShapeError(path, group string, value *yaml.Node) *LoadError {
	return sourceGraphError(path, fmt.Sprintf(
		"group %q must be null or a YAML mapping, found %s (line %d)",
		group, describeNodeShape(value), effectiveNodeLine(value),
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
