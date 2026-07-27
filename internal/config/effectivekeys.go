package config

import "gopkg.in/yaml.v3"

// effectiveKeyID is a comparable identity for a mapping key used by
// merge-precedence resolution (a later chunk's resolveEffective), as
// distinct from sourceKeyID's tag-blind duplicate-key detection in
// sourcegraph.go. Two effectiveKeyID values compare equal, and therefore
// collide as map keys, exactly when effectiveKeyIdentity considers their
// source nodes "the same key" for merge-precedence purposes: see
// effectiveKeyIdentity's doc comment for the scalar and complex identity
// rules, and
// docs/agents/skills/yaml-scalar-key-identity-needs-tag-not-just-value.md
// for why a scalar identity must include its resolved tag, not just Value.
type effectiveKeyID struct {
	kind    yaml.Kind
	tag     string
	value   string
	complex *yaml.Node
}

// effectiveKeyIdentity computes n's effectiveKeyID for merge-precedence
// key comparison. It first follows n's yaml.AliasNode chain to its
// non-alias target, guarded by a local seen-set keyed on node pointer so a
// synthetic self-referential (or otherwise cyclic) alias handed directly
// to this helper terminates instead of looping forever; real callers only
// ever pass nodes from a graph validateSourceGraph has already proven
// acyclic, so that guard is a defense-in-depth backstop, not a path any
// production caller is expected to hit. A nil n, or an alias chain that
// dead-ends on a nil Alias, yields the zero effectiveKeyID.
//
// Once resolved to a non-alias target: a yaml.ScalarNode target yields
// {kind: yaml.ScalarNode, tag: target.ShortTag(), value: target.Value} —
// target.ShortTag() is used (rather than the raw Tag field) because
// gopkg.in/yaml.v3 v3.0.1's yaml.go implements ShortTag to resolve an
// unset or "!" tag from the node's own properties (resolve("", n.Value)
// for scalars), so two scalars with the same Kind, resolved tag, and
// Value are the same key regardless of whether either wrote its tag
// explicitly. A yaml.MappingNode or yaml.SequenceNode target yields
// {complex: target}: identity is the target node's pointer alone, with no
// structural expansion of the complex key's contents — two distinct nodes
// that happen to look alike are different keys, and two aliases sharing
// one target are the same key.
func effectiveKeyIdentity(n *yaml.Node) effectiveKeyID {
	seen := make(map[*yaml.Node]bool)
	for n != nil && n.Kind == yaml.AliasNode {
		if seen[n] {
			return effectiveKeyID{}
		}
		seen[n] = true
		n = n.Alias
	}
	if n == nil {
		return effectiveKeyID{}
	}
	if n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode {
		return effectiveKeyID{complex: n}
	}
	return effectiveKeyID{kind: n.Kind, tag: n.ShortTag(), value: n.Value}
}
