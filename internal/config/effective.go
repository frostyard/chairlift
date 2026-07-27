package config

import "gopkg.in/yaml.v3"

// resolveEffective validates doc as a source graph (validateSourceGraph)
// and, on success, emits a fresh alias-free, anchor-free copy of it rooted
// at a yaml.DocumentNode.
//
// Validation always runs first and its error, if any, is returned
// unchanged: a caller of resolveEffective cannot bypass validateSourceGraph's
// existing Detail/line contract by going through this entry point instead.
// doc == nil succeeds validation and returns (nil, nil) — parseYAMLDocument's
// valid empty-input result.
//
// On success the returned tree is a structural copy of doc: every emitted
// node copies Kind, Style, Tag, Value, Line, Column, HeadComment,
// LineComment and FootComment from the source node it was emitted for,
// leaves Anchor as "" and Alias as nil, and recurses into Content in the
// same order as the source. A yaml.AliasNode is never copied as itself;
// instead its recursively dereferenced non-alias target is emitted in its
// place, carrying the target's own metadata rather than the alias node's —
// including when the alias appears in mapping key position. Because the
// result must be alias-free, a target reached through more than one alias
// becomes more than one independent fresh copy.
//
// This is an interim result, authorized by the spec for this chunk: a
// recognized "<<" merge key (isMergeKey) is emitted as an ordinary mapping
// entry with its operand value resolved like any other value, rather than
// being consumed by merge-precedence resolution. A later chunk replaces
// this with the real merge-precedence result.
func resolveEffective(path string, doc *yaml.Node) (*yaml.Node, *LoadError) {
	if err := validateSourceGraph(path, doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}
	return emitEffectiveNode(doc), nil
}

// emitEffectiveNode dereferences n through dereferenceAliasTarget and
// returns a fresh copy of the resulting non-alias node: its metadata via
// emitNodeMetadata, plus its Content recursively emitted in the same
// order. It assumes n comes from a graph validateSourceGraph has already
// proven acyclic and well-formed, so it never needs its own cycle guard.
func emitEffectiveNode(n *yaml.Node) *yaml.Node {
	target := dereferenceAliasTarget(n)
	out := emitNodeMetadata(target)
	if len(target.Content) == 0 {
		return out
	}
	out.Content = make([]*yaml.Node, len(target.Content))
	for i, child := range target.Content {
		out.Content[i] = emitEffectiveNode(child)
	}
	return out
}

// dereferenceAliasTarget follows n's yaml.AliasNode chain (however many
// hops) to its non-alias target. It relies on validateSourceGraph having
// already proven the graph acyclic and every alias's Alias field non-nil,
// so it needs no cycle guard or nil check of its own beyond the loop
// condition.
func dereferenceAliasTarget(n *yaml.Node) *yaml.Node {
	for n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// emitNodeMetadata copies n's Kind, Style, Tag, Value, Line, Column,
// HeadComment, LineComment and FootComment into a freshly allocated
// *yaml.Node, leaving Anchor as its zero value "" and Alias as its zero
// value nil so the copy is never itself an anchor or an alias, and leaving
// Content nil for the caller to fill in.
func emitNodeMetadata(n *yaml.Node) *yaml.Node {
	return &yaml.Node{
		Kind:        n.Kind,
		Style:       n.Style,
		Tag:         n.Tag,
		Value:       n.Value,
		Line:        n.Line,
		Column:      n.Column,
		HeadComment: n.HeadComment,
		LineComment: n.LineComment,
		FootComment: n.FootComment,
	}
}
