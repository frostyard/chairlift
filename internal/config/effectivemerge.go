package config

import "gopkg.in/yaml.v3"

// effectiveEntry is one candidate mapping entry discovered while computing
// a mapping node's effective (post-merge) inventory: id is the entry key's
// effectiveKeyIdentity, used to decide whether two candidates from
// different sources are "the same key" for merge-precedence purposes; key
// and value are the source nodes (not yet emitted/copied) for that
// candidate's key and value.
type effectiveEntry struct {
	id    effectiveKeyID
	key   *yaml.Node
	value *yaml.Node
}

// effectiveMergeMemo memoizes effectiveEntries results by mapping-node
// pointer identity for the duration of one effectiveEntries call and its
// recursion into merge operands. A mapping reached as a merge operand
// through more than one parent within that recursion, or through more
// than one alias to the same anchor, has its candidate inventory computed
// once and reused, rather than recomputed once per parent — this is what
// keeps a shared-operand fan-out, or a doubling merge DAG like the one
// TestResolveEffectiveSharedOperandInventoriedOnce builds, linear in the
// number of distinct mapping nodes rather than exponential in fan-out
// depth. Memoizing after the recursive call below (rather than before) is
// safe without its own cycle guard because validateSourceGraph has already
// proven the whole source graph acyclic before resolveEffective ever
// calls this.
type effectiveMergeMemo map[*yaml.Node][]effectiveEntry

// effectiveEntries computes m's complete, ordered, deduplicated effective
// entry inventory: retained explicit entries first, in m's source Content
// order, excluding recognized merge directives (isMergeKey); then
// inherited winners in merge-candidate discovery order — discovered by
// merge directives in source Content order, sequence operands left to
// right, and each operand mapping's own effectiveEntries computed
// recursively (so a nested merge inside a merge operand is flattened
// before its candidates are considered here).
//
// Candidates are deduplicated by effectiveKeyIdentity keeping the FIRST
// candidate encountered in that discovery order. Because retained explicit
// entries are always listed first, this yields explicit-over-merged
// precedence; because merge candidates are discovered in source Content
// and sequence-operand order, it yields earlier-operand-wins precedence;
// and because the explicit pass runs before the merge pass regardless of
// where the `<<` directive appears in m's Content, an explicit key
// suppresses a matching inherited candidate whether the directive comes
// before or after that explicit key in the source.
//
// This reproduces gopkg.in/yaml.v3 v3.0.1 decode.go's decoder.mapping /
// decoder.merge behavior exactly: mapping decodes explicit entries first
// and skips isMerge keys, then merge seeds mergedFields from every
// explicit parent key before consuming operands in Content order and
// marking each merged key as it is consumed — with nested merges
// inheriting the accumulated set rather than resetting it, which is why
// this function's own recursive calls flatten rather than re-seed.
//
// effectiveEntries creates a fresh effectiveMergeMemo scoped to this one
// call (and delegates to effectiveEntriesWithMemo, which threads it
// through the recursion below): resolveEffective's emission path
// (effective.go) calls this once per mapping node it emits, so a mapping
// node reached only as a merge operand deep inside another mapping's
// inventory is inventoried at most once per top-level emitted mapping
// that reaches it, not once per parent within that recursion.
func effectiveEntries(m *yaml.Node) []effectiveEntry {
	return effectiveEntriesWithMemo(m, make(effectiveMergeMemo))
}

// effectiveEntriesWithMemo is effectiveEntries' real implementation,
// threading a shared memo across recursive merge-operand lookups so a
// mapping node's inventory is computed at most once per top-level
// effectiveEntries call no matter how many operands or aliases reach it
// within that call's recursion.
func effectiveEntriesWithMemo(m *yaml.Node, memo effectiveMergeMemo) []effectiveEntry {
	if m == nil {
		return nil
	}
	if cached, ok := memo[m]; ok {
		return cached
	}

	var candidates []effectiveEntry

	// Pass 1: retained explicit entries, source Content order, excluding
	// recognized merge directives.
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, value := m.Content[i], m.Content[i+1]
		if isMergeKey(key) {
			continue
		}
		candidates = append(candidates, effectiveEntry{
			id:    effectiveKeyIdentity(key),
			key:   key,
			value: value,
		})
	}

	// Pass 2: inherited candidates from merge directives, source Content
	// order, sequence operands left to right, each operand's own
	// effectiveEntries recursively.
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, value := m.Content[i], m.Content[i+1]
		if !isMergeKey(key) {
			continue
		}
		for _, operand := range mergeOperandMappings(value) {
			candidates = append(candidates, effectiveEntriesWithMemo(operand, memo)...)
		}
	}

	seen := make(map[effectiveKeyID]bool, len(candidates))
	result := make([]effectiveEntry, 0, len(candidates))
	for _, c := range candidates {
		if seen[c.id] {
			continue
		}
		seen[c.id] = true
		result = append(result, c)
	}

	memo[m] = result
	return result
}

// mergeOperandMappings returns, in left-to-right order, the underlying
// mapping node for each operand a validated merge value carries: a single
// entry for a direct mapping or an alias to one, or one entry per element
// for a sequence of those, per validateMergeOperand's already-established
// accepted shapes. It assumes value has already passed
// validateMergeOperand, so no shape is rejected here.
func mergeOperandMappings(value *yaml.Node) []*yaml.Node {
	if value == nil {
		return nil
	}
	if value.Kind == yaml.SequenceNode {
		out := make([]*yaml.Node, 0, len(value.Content))
		for _, entry := range value.Content {
			out = append(out, mergeOperandMapping(entry))
		}
		return out
	}
	return []*yaml.Node{mergeOperandMapping(value)}
}

// mergeOperandMapping unwraps a single merge operand (a mapping, or an
// alias whose immediate target is a mapping — validateMergeOperand's
// isMergeOperandMapping already proved this is one of those two shapes)
// to its underlying *yaml.Node mapping, following at most one alias hop,
// matching isMergeOperandMapping's own single-hop rule.
func mergeOperandMapping(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.AliasNode {
		return n.Alias
	}
	return n
}
