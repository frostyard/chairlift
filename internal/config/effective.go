package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// maxEffectiveOutputNodes is the maximum number of nodes
// resolveEffective's emit path (emitEffectiveNode) may allocate for one
// call. Every retained document, mapping, sequence, key and value node
// counts exactly once, mirroring sourcebounds.go's pathVisitCount
// accounting rule that "key", "value" and "alias target" only name which
// child is traversed next and add no separate charge; each independent
// copy produced by expanding a shared alias target is charged separately,
// since the emitted tree is alias-free and each such copy is a distinct
// allocation. The budget check happens immediately before a node would be
// allocated — the first statements of the emit path for that node — so
// the 100,001st attempted emission fails before its (and any descendant's)
// allocation rather than after materializing an oversized tree; this is
// what makes an exponential alias expansion abort at the boundary instead
// of running to completion. Exactly maxEffectiveOutputNodes succeeds and
// maxEffectiveOutputNodes+1 fails.
const maxEffectiveOutputNodes = 100000

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
// A mapping node's Content is not copied verbatim: it is replaced by its
// effective entries (effectiveEntries, effectivemerge.go), so only winning
// keys and winning values — explicit-over-merged, earlier-sequence-operand-
// over-later, in effectiveEntries' documented deterministic order — are
// ever resolved and emitted. A losing candidate's value is never cloned
// and never charged against maxEffectiveOutputNodes below. A recognized
// "<<" merge key is therefore never itself emitted as a key, and the
// result contains no recognized merge directive anywhere, including
// within a retained mapping key's own subtree or a retained value's own
// nested merge.
//
// The emit path is bounded by maxEffectiveOutputNodes: if emitting doc
// would allocate more than that many nodes (an exponential alias
// expansion is the mechanism that makes this reachable from a small,
// well-formed source graph — see checkSourceGraphBounds' hop and
// path-visit limits, which bound the *source* graph but not its expanded
// output), resolveEffective returns a nil tree and a KindParseType
// *LoadError naming the bound instead of completing the allocation.
func resolveEffective(path string, doc *yaml.Node) (*yaml.Node, *LoadError) {
	if err := validateSourceGraph(path, doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}
	st := &effectiveEmitState{}
	out := emitEffectiveNode(doc, st)
	if st.overflowed {
		return nil, effectiveOutputLimitError(path, doc, st.overflowNode)
	}
	return out, nil
}

// effectiveEmitState tracks emitEffectiveNode's per-call node budget: count
// is the number of nodes allocated so far, and overflowed/overflowNode
// record the first node whose emission would exceed
// maxEffectiveOutputNodes, so the recursion can unwind without doing any
// further allocation or descending into any further Content.
type effectiveEmitState struct {
	count        int
	overflowed   bool
	overflowNode *yaml.Node
}

// emitEffectiveNode dereferences n through dereferenceAliasTarget and
// returns a fresh copy of the resulting non-alias node: its metadata via
// emitNodeMetadata, plus its Content recursively emitted in the same
// order — except for a mapping node, whose emitted entries come from
// effectiveEntries instead of its raw Content, so only winning keys and
// values are ever resolved (see resolveEffective's doc comment).
// effectiveEntries' own memo (effectivemerge.go) is scoped to that one
// call and its recursion into merge operands, so a mapping node reached
// as a merge operand through several parents or aliases within the same
// mapping's inventory computation is inventoried once, not once per
// parent. It assumes n comes from a graph validateSourceGraph has already
// proven acyclic and well-formed, so it never needs its own cycle guard.
//
// Before allocating anything for the dereferenced target, it increments
// st.count and checks it against maxEffectiveOutputNodes; once st.count
// would exceed the budget, it records the offending node and returns nil
// immediately, without allocating a node for it or recursing into its
// Content. Once st.overflowed is set (by this call or an earlier sibling
// call), every subsequent call returns nil immediately without doing any
// further work, so an oversized or exponential expansion is bounded by the
// budget rather than run to completion.
func emitEffectiveNode(n *yaml.Node, st *effectiveEmitState) *yaml.Node {
	if st.overflowed {
		return nil
	}
	target := dereferenceAliasTarget(n)
	st.count++
	if st.count > maxEffectiveOutputNodes {
		st.overflowed = true
		st.overflowNode = target
		return nil
	}
	out := emitNodeMetadata(target)
	if target.Kind == yaml.MappingNode {
		entries := effectiveEntries(target)
		if len(entries) == 0 {
			return out
		}
		out.Content = make([]*yaml.Node, 0, len(entries)*2)
		for _, e := range entries {
			keyOut := emitEffectiveNode(e.key, st)
			if st.overflowed {
				return nil
			}
			valOut := emitEffectiveNode(e.value, st)
			if st.overflowed {
				return nil
			}
			out.Content = append(out.Content, keyOut, valOut)
		}
		return out
	}
	if len(target.Content) == 0 {
		return out
	}
	out.Content = make([]*yaml.Node, len(target.Content))
	for i, child := range target.Content {
		out.Content[i] = emitEffectiveNode(child, st)
		if st.overflowed {
			return nil
		}
	}
	return out
}

// effectiveOutputLimitError builds the KindParseType *LoadError
// resolveEffective returns when emitting doc would exceed
// maxEffectiveOutputNodes. It reuses sourcebounds.go's
// collectSourceInventory and attributeSourceLines on the validated source
// document doc — the same all-paths line attribution
// checkSourceGraphBounds uses (a node's own line, else its nearest
// positive-line ancestor over all paths, else 1) — computing that
// inventory exactly once for the call, rather than writing a second line
// attribution implementation, so the reported line for overflowNode (the
// source node whose emission would have been the 100,001st) is always
// positive.
func effectiveOutputLimitError(path string, doc *yaml.Node, overflowNode *yaml.Node) *LoadError {
	inv := collectSourceInventory(doc)
	lines := attributeSourceLines(inv)
	line := lines[overflowNode]
	if line <= 0 {
		line = 1
	}
	detail := fmt.Sprintf(
		"effective output exceeds the maximum of %d emitted nodes (line %d)",
		maxEffectiveOutputNodes, line,
	)
	return sourceGraphError(path, detail)
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
