package config

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// longYAMLTagPrefix is the "tag:yaml.org,2002:" canonical-tag prefix that
// gopkg.in/yaml.v3 v3.0.1 resolve.go defines as longTagPrefix and that
// shortYAMLTag strips when normalizing a tag to its short "!!" form.
const longYAMLTagPrefix = "tag:yaml.org,2002:"

// shortYAMLTag reproduces gopkg.in/yaml.v3 v3.0.1 resolve.go:shortTag's
// rewrite of a canonical "tag:yaml.org,2002:xxx" tag to yaml.v3's short
// "!!xxx" form. A tag that does not carry the long prefix — including the
// empty tag, the bare "!" non-specific tag, an already-short "!!xxx" tag, a
// custom "!xxx" tag, or a long tag under a different authority
// (e.g. "tag:example.com,2020:merge") — is returned unchanged, exactly as
// shortTag leaves it. yaml.v3's shortTag additionally special-cases a small
// set of well-known tags (null, bool, str, int, float, timestamp, seq, map,
// binary, merge) through a lookup table, but that table always yields the
// same "!!" + suffix result the plain prefix-strip produces for those tags,
// so this reproduction needs no such table to match its behavior exactly.
func shortYAMLTag(tag string) string {
	if strings.HasPrefix(tag, longYAMLTagPrefix) {
		return "!!" + tag[len(longYAMLTagPrefix):]
	}
	return tag
}

// isMergeKey reproduces gopkg.in/yaml.v3 v3.0.1 decode.go:isMerge's
// predicate for recognizing a YAML merge key ("<<"): n must be a
// yaml.ScalarNode whose Value is exactly "<<", and whose Tag is either
// absent ("", the implicit/unresolved tag), the bare non-specific tag
// ("!"), or resolves via shortYAMLTag to the short merge tag ("!!merge") —
// which accepts both that short form directly and its canonical long form
// ("tag:yaml.org,2002:merge"). A quoted "<<" (explicitly tagged "!!str") is
// therefore an ordinary key, not a merge key, and so is any node whose
// Value isn't literally "<<" regardless of its tag. n may be nil; isMergeKey
// reports false rather than panicking.
func isMergeKey(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	return n.Kind == yaml.ScalarNode && n.Value == "<<" &&
		(n.Tag == "" || n.Tag == "!" || shortYAMLTag(n.Tag) == "!!merge")
}

// nodeState tracks a *yaml.Node's traversal progress by node identity (map
// key is the pointer itself, not any decoded value) so that a node reached
// through more than one parent is walked exactly once: unseen nodes are
// absent from the map, visiting marks a node currently on the active
// depth-first path (re-encountering one is an alias cycle), and done marks
// a node that was already fully validated and whose result may be reused.
type nodeState int

const (
	visiting nodeState = iota
	done
)

// sourceGraphError builds a KindParseType *LoadError for a structural
// defect found while validating a source graph. These are pure shape
// rejections with no underlying Go error to preserve, so Err is left nil;
// LoadError.Error() still renders the "config parse/type error: <path>:
// <detail>" wording from Path and Detail alone.
func sourceGraphError(path, detail string) *LoadError {
	return &LoadError{Path: path, Kind: KindParseType, Detail: detail}
}

// validateSourceGraph walks every node and content edge reachable from doc
// — through document content, mapping keys, mapping values, sequence
// entries, and alias targets, in each node's Content slice order — and
// rejects malformed source graphs as a KindParseType *LoadError without
// panicking, even when doc is a synthetic *yaml.Node tree unreachable from
// real YAML text.
//
// doc == nil succeeds: it is parseYAMLDocument's valid empty-input result.
// Otherwise doc must be a yaml.DocumentNode with exactly one non-nil child;
// every reachable mapping must have an even number of non-nil key/value
// content entries; every reachable sequence must have only non-nil
// entries; every reachable scalar must have no content children; every
// reachable alias must have no content children and a non-nil Alias
// target; and every reachable node's Kind must be one of the supported
// yaml.v3 kinds. Node identity (not value equality) drives cycle
// detection, so a node reachable through several parents is validated once
// and re-encountering a node still on the active traversal path is
// rejected as an alias cycle. Every mapping entry whose key is a merge key
// (isMergeKey) additionally has its value validated as a merge operand
// (validateMergeOperand): a mapping, an alias whose immediate target is a
// mapping, or a sequence of those, checked wherever such an entry is
// reachable — including a branch a later merge-precedence pass would
// discard, and inside a complex mapping key's own subtree. This validation
// covers structural well-formedness and merge-operand shape; duplicate-key
// detection and the alias-hop/path-visit bounds are out of this function's
// scope.
func validateSourceGraph(path string, doc *yaml.Node) *LoadError {
	if doc == nil {
		return nil
	}
	if doc.Kind != yaml.DocumentNode {
		return sourceGraphError(path, "source document root is not a YAML document node")
	}
	if len(doc.Content) != 1 {
		return sourceGraphError(path, "source document must contain exactly one top-level node")
	}
	if doc.Content[0] == nil {
		return sourceGraphError(path, "source document top-level node is nil")
	}

	states := make(map[*yaml.Node]nodeState)
	return walkSourceNode(path, doc.Content[0], states)
}

// walkSourceNode validates a single reachable node and, on success,
// recurses into its content edges in Content slice order. states records
// each node's traversal progress by pointer identity; a node absent from
// states is unseen, one present with visiting is on the active
// depth-first path (so re-encountering it is a cycle), and one present
// with done was already fully validated.
func walkSourceNode(path string, n *yaml.Node, states map[*yaml.Node]nodeState) *LoadError {
	if n == nil {
		return sourceGraphError(path, "source graph has a nil node")
	}
	if state, seen := states[n]; seen {
		if state == visiting {
			return sourceGraphError(path, "source graph contains an alias cycle")
		}
		return nil
	}
	states[n] = visiting

	switch n.Kind {
	case yaml.MappingNode:
		if len(n.Content)%2 != 0 {
			return sourceGraphError(path, "mapping node has an odd number of content entries")
		}
		for i := 0; i < len(n.Content); i += 2 {
			key, value := n.Content[i], n.Content[i+1]
			if key == nil {
				return sourceGraphError(path, "mapping node has a nil key")
			}
			if value == nil {
				return sourceGraphError(path, "mapping node has a nil value")
			}
			if err := walkSourceNode(path, key, states); err != nil {
				return err
			}
			if err := walkSourceNode(path, value, states); err != nil {
				return err
			}
			if isMergeKey(key) {
				if err := validateMergeOperand(path, value); err != nil {
					return err
				}
			}
		}
	case yaml.SequenceNode:
		for _, entry := range n.Content {
			if entry == nil {
				return sourceGraphError(path, "sequence node has a nil entry")
			}
			if err := walkSourceNode(path, entry, states); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if len(n.Content) != 0 {
			return sourceGraphError(path, "scalar node has unexpected content")
		}
	case yaml.AliasNode:
		if len(n.Content) != 0 {
			return sourceGraphError(path, "alias node has unexpected content")
		}
		if n.Alias == nil {
			return sourceGraphError(path, "alias node has no target")
		}
		if err := walkSourceNode(path, n.Alias, states); err != nil {
			return err
		}
	default:
		return sourceGraphError(path, "source graph node has an unsupported kind")
	}

	states[n] = done
	return nil
}

// validateMergeOperand rejects a "<<" merge key's value unless it has one
// of the shapes gopkg.in/yaml.v3 v3.0.1 decode.go's merge/failWantMap logic
// accepts: a MappingNode; an AliasNode whose immediate Alias target is a
// MappingNode (an alias-to-alias-to-mapping chain is well-formed ordinary
// YAML content but is rejected here, because yaml.v3 unwraps only one alias
// hop for a merge operand); or a SequenceNode each of whose entries is
// itself one of the two prior shapes. Everything else — a scalar, an alias
// to a sequence or scalar, or a sequence containing a non-mapping entry —
// is a KindParseType error. value's own structural well-formedness (nil
// entries, unsupported kinds, cycles) has already been proven by the
// caller's walkSourceNode call before this runs, so only operand shape is
// checked here.
func validateMergeOperand(path string, value *yaml.Node) *LoadError {
	if isMergeOperandMapping(value) {
		return nil
	}
	if value != nil && value.Kind == yaml.SequenceNode {
		for _, entry := range value.Content {
			if !isMergeOperandMapping(entry) {
				return sourceGraphError(path, "merge key sequence entry is not a mapping or an alias to a mapping")
			}
		}
		return nil
	}
	return sourceGraphError(path, "merge key value must be a mapping, an alias to a mapping, or a sequence of those")
}

// isMergeOperandMapping reports whether n is directly a yaml.MappingNode or
// a yaml.AliasNode whose immediate Alias target is a yaml.MappingNode. It
// deliberately follows at most one alias hop: an alias-to-alias-to-mapping
// chain returns false here even though it is a valid ordinary YAML value,
// reproducing yaml.v3 v3.0.1's merge-operand rule exactly (interpretation:
// merge alias operands use the immediate target only).
func isMergeOperandMapping(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	if n.Kind == yaml.MappingNode {
		return true
	}
	return n.Kind == yaml.AliasNode && n.Alias != nil && n.Alias.Kind == yaml.MappingNode
}
