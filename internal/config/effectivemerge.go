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
// pointer identity for one complete resolveEffective call. It is stored on
// effectiveEmitState, so a mapping reached as a merge operand through more
// than one emitted parent, or through more than one alias to the same anchor,
// has its candidate inventory computed once and reused. This keeps both
// shared-operand fan-out and doubling merge DAGs linear in the number of
// distinct mapping nodes. Memoizing after the recursive call below is safe
// without a separate cycle guard because validateSourceGraph has already
// proven the whole source graph acyclic.
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
// effectiveEntries delegates to effectiveEntriesWithMemo using the memo
// carried by st for the entire resolveEffective call. The defensive nil
// initialization also keeps the helper safe if a future internal caller
// constructs an effectiveEmitState without using resolveEffective.
//
// It also threads st (effective.go's effectiveEmitState) through the
// recursion so an effective-identity collision between two of m's own
// explicit keys — or between two explicit keys of any mapping reached as
// a merge operand within this recursion — is detected and recorded on st
// exactly like emitEffectiveNode's own node-budget overflow, aborting the
// rest of the inventory computation without inventorying or emitting
// anything further.
func effectiveEntries(m *yaml.Node, st *effectiveEmitState) []effectiveEntry {
	if st.mergeMemo == nil {
		st.mergeMemo = make(effectiveMergeMemo)
	}
	return effectiveEntriesWithMemo(m, st.mergeMemo, st)
}

// effectiveEntriesWithMemo is effectiveEntries' real implementation,
// threading a shared memo across every emitted mapping and recursive
// merge-operand lookup so a mapping node's inventory is computed at most
// once per resolveEffective call, and threading st so an identity collision
// discovered anywhere in that recursion halts the whole computation.
func effectiveEntriesWithMemo(m *yaml.Node, memo effectiveMergeMemo, st *effectiveEmitState) []effectiveEntry {
	if m == nil || st.collided {
		return nil
	}
	if cached, ok := memo[m]; ok {
		return cached
	}

	if laterKey, ok := explicitKeyCollision(m); ok {
		st.collided = true
		st.collisionKey = laterKey
		return nil
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
			candidates = append(candidates, effectiveEntriesWithMemo(operand, memo, st)...)
			if st.collided {
				return nil
			}
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

// explicitKeyCollision scans m's retained explicit entries (source
// Content order, excluding recognized merge directives, exactly the same
// entries effectiveEntriesWithMemo's pass 1 collects) for two whose
// effectiveKeyIdentity compare equal, returning the SECOND (later) key
// node of the first such pair found, and true. This is a post-alias rule:
// sourcegraph.go's checkDuplicateMappingKeys already rejects two explicit
// keys with the same tag-blind Kind+Value before resolveEffective is ever
// reached, but does not catch two explicit keys whose Kind and Value
// differ syntactically yet dereference (through one or more aliases) to
// the same effectiveKeyIdentity — e.g. a direct scalar key and an alias
// key targeting an anchored scalar with the same resolved tag and value.
//
// It reports nothing about a collision between an explicit key and a
// merge-inherited candidate of the same identity: that case is ordinary
// suppression, handled by effectiveEntriesWithMemo's dedup loop below,
// not this error condition, since only two EXPLICIT keys of one source
// mapping are ever compared here.
func explicitKeyCollision(m *yaml.Node) (*yaml.Node, bool) {
	seen := make(map[effectiveKeyID]bool)
	for i := 0; i+1 < len(m.Content); i += 2 {
		key := m.Content[i]
		if isMergeKey(key) {
			continue
		}
		id := effectiveKeyIdentity(key)
		if seen[id] {
			return key, true
		}
		seen[id] = true
	}
	return nil, false
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
