package config

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// newSourceScalarNode builds a well-formed yaml.ScalarNode with the given
// value, for use as filler content in synthetic bounds-testing graphs.
func newSourceScalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
}

// newSourceMappingNode builds a well-formed yaml.MappingNode from an
// already-interleaved key/value content slice.
func newSourceMappingNode(content ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Content: content}
}

// newSourceSequenceNode builds a well-formed yaml.SequenceNode from its
// entries.
func newSourceSequenceNode(entries ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Content: entries}
}

// newSourceAliasNode builds a well-formed yaml.AliasNode targeting target.
func newSourceAliasNode(target *yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.AliasNode, Alias: target}
}

// newSourceDocNode wraps root as a well-formed yaml.DocumentNode with
// exactly one child, ready to pass to validateSourceGraph.
func newSourceDocNode(root *yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
}

// buildSourceAliasChain returns the head of a chain of n consecutive alias
// hops terminating in an ordinary scalar: an AliasNode whose Alias is
// another AliasNode, n levels deep, whose innermost Alias targets a
// non-alias scalar (hop count 0). The returned head node has
// aliasHopCount == n and Line == 0, since none of the synthesized nodes
// set Line.
func buildSourceAliasChain(n int) *yaml.Node {
	node := newSourceScalarNode("terminal")
	for i := 0; i < n; i++ {
		node = newSourceAliasNode(node)
	}
	return node
}

// buildSourceSharingDAG returns a compact alias-sharing DAG built by a
// doubling construction: level 0 is a single scalar leaf, and each
// subsequent level is a two-entry sequence whose entries are both aliases
// of the previous level's single node. Node count grows linearly with
// levels (three new nodes per level) while the number of distinct
// root-to-leaf paths doubles per level, so a naive per-path traversal of
// buildSourceSharingDAG(40) would need to expand roughly 2^40 paths, while
// a linear (node-and-edge-bounded) traversal visits it in microseconds.
func buildSourceSharingDAG(levels int) *yaml.Node {
	level := newSourceScalarNode("leaf")
	for i := 0; i < levels; i++ {
		level = newSourceSequenceNode(newSourceAliasNode(level), newSourceAliasNode(level))
	}
	return level
}

// sourceLineDetailPattern matches the literal "line N" substring
// interpretation 7 requires every bounds-limit LoadError.Detail to
// contain.
var sourceLineDetailPattern = regexp.MustCompile(`line (\d+)`)

// sourceDetailLine extracts and returns the positive integer N from an
// err.Detail's "line N" substring, failing the test if no such substring
// is present or the captured number is not a valid positive integer.
func sourceDetailLine(t *testing.T, err *LoadError) int {
	t.Helper()
	m := sourceLineDetailPattern.FindStringSubmatch(err.Detail)
	if m == nil {
		t.Fatalf("err.Detail = %q, want a %q substring", err.Detail, "line N")
	}
	n, convErr := strconv.Atoi(m[1])
	if convErr != nil || n <= 0 {
		t.Fatalf("err.Detail = %q: parsed line %d (err=%v), want a positive integer", err.Detail, n, convErr)
	}
	return n
}

// runSourceValidateWithin runs validateSourceGraph(path, doc) on a
// goroutine and fails the test if it has not returned within timeout,
// using an explicit timer plus select rather than a bare t.Fatal after
// the call — the only way to prove a non-terminating (exponential or
// infinite) traversal fails the test instead of simply hanging the test
// binary.
func runSourceValidateWithin(t *testing.T, path string, doc *yaml.Node, timeout time.Duration) *LoadError {
	t.Helper()
	resultCh := make(chan *LoadError, 1)
	go func() {
		resultCh <- validateSourceGraph(path, doc)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-resultCh:
		return err
	case <-timer.C:
		t.Fatalf("validateSourceGraph(%q, ...) did not return within %s", path, timeout)
		return nil
	}
}

// TestValidateSourceGraphAliasHopBoundary confirms the 64/65 alias-hop
// boundary exactly: a chain of 64 consecutive alias hops succeeds, and a
// chain of 65 fails as KindParseType, naming the consecutive-alias-hop
// limit, the literal "64", and a positive "line N" — even though every
// synthesized node in the chain has Line == 0, so the reported line must
// come from the deterministic fallback rather than from any node's own
// Line.
func TestValidateSourceGraphAliasHopBoundary(t *testing.T) {
	const path = "/etc/chairlift/alias-hop-boundary.yml"

	t.Run("exactly 64 hops succeeds", func(t *testing.T) {
		doc := newSourceDocNode(buildSourceAliasChain(64))
		if err := validateSourceGraph(path, doc); err != nil {
			t.Fatalf("validateSourceGraph(64-hop chain) = %v, want nil", err)
		}
	})

	t.Run("65 hops fails", func(t *testing.T) {
		doc := newSourceDocNode(buildSourceAliasChain(65))
		err := validateSourceGraph(path, doc)
		wantParseType(t, err, path)

		if err.Err != nil {
			t.Fatalf("err.Err = %v, want nil", err.Err)
		}
		wantPrefix := "config parse/type error: " + path
		if got := err.Error(); len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
			t.Fatalf("err.Error() = %q, want it to start with %q", got, wantPrefix)
		}
		if !strings.Contains(err.Detail, "consecutive") || !strings.Contains(err.Detail, "alias hop") {
			t.Fatalf("err.Detail = %q, want wording identifying the consecutive alias-hop limit", err.Detail)
		}
		if !strings.Contains(err.Detail, "64") {
			t.Fatalf("err.Detail = %q, want the literal %q", err.Detail, "64")
		}
		// Every node in the chain has Line == 0: no ancestor and no
		// own line exists anywhere in this graph, so the fallback
		// line 1 must be reported.
		if got := sourceDetailLine(t, err); got != 1 {
			t.Fatalf("attributed line = %d, want 1 (wholly synthetic fallback)", got)
		}
	})
}

// TestValidateSourceGraphAliasHopIndependentSiblingsReset confirms more
// than 64 independent, shallow sibling aliases succeed: each alias hops
// only once to the same shared anchored scalar, so the consecutive-hop
// count resets on descending from the (non-alias) sequence node into each
// alias, and none of them individually exceeds the limit even though there
// are far more than 64 of them in the graph.
func TestValidateSourceGraphAliasHopIndependentSiblingsReset(t *testing.T) {
	shared := newSourceScalarNode("shared")
	entries := make([]*yaml.Node, 0, 100)
	for i := 0; i < 100; i++ {
		entries = append(entries, newSourceAliasNode(shared))
	}
	doc := newSourceDocNode(newSourceSequenceNode(entries...))

	if err := validateSourceGraph("p", doc); err != nil {
		t.Fatalf("validateSourceGraph(100 independent sibling aliases) = %v, want nil", err)
	}
}

// TestValidateSourceGraphAliasHopAttributionSinglePath confirms the
// single-path attribution case: the offending (over-limit) node has
// Line == 0, and its only root-reachable path passes through two
// positive-line ancestors, one directly above it (distance 1) and one
// three edges further up (distance 3). The nearer ancestor's line must be
// reported.
func TestValidateSourceGraphAliasHopAttributionSinglePath(t *testing.T) {
	const path = "/etc/chairlift/alias-hop-single-path.yml"

	offending := buildSourceAliasChain(65) // Line == 0, hop count 65

	near := &yaml.Node{ // distance 1 from offending
		Kind:    yaml.MappingNode,
		Line:    20,
		Content: []*yaml.Node{newSourceScalarNode("near-key"), offending},
	}
	mid := &yaml.Node{ // distance 2, no line of its own
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{newSourceScalarNode("mid-key"), near},
	}
	far := &yaml.Node{ // distance 3 from offending
		Kind:    yaml.MappingNode,
		Line:    10,
		Content: []*yaml.Node{newSourceScalarNode("far-key"), mid},
	}
	doc := newSourceDocNode(far)

	err := validateSourceGraph(path, doc)
	wantParseType(t, err, path)
	if got := sourceDetailLine(t, err); got != 20 {
		t.Fatalf("attributed line = %d, want 20 (the distance-1 ancestor)", got)
	}
}

// TestValidateSourceGraphAliasHopAttributionUnequalDistanceSharedNode is
// the mandatory review-round-1 case: the offending (over-limit, Line == 0)
// node is a single shared *yaml.Node reachable via two different paths —
// one where it sits three content edges below positive-line ancestor A,
// and a shorter one where it sits one content edge below a different
// positive-line ancestor B. The nearer ancestor B's line must win
// regardless of which path the deterministic depth-first walk happens to
// discover first, so this is asserted with B's edge ordered both after and
// before A's in the root mapping's Content — proving the result cannot
// depend on encounter order.
func TestValidateSourceGraphAliasHopAttributionUnequalDistanceSharedNode(t *testing.T) {
	const path = "/etc/chairlift/alias-hop-unequal-distance.yml"

	build := func(nearFirst bool) *yaml.Node {
		offending := buildSourceAliasChain(65) // shared node, Line == 0

		// Path through A: three edges from ancestorA down to offending.
		nearA2 := newSourceMappingNode(newSourceScalarNode("a2"), offending)
		nearA1 := newSourceMappingNode(newSourceScalarNode("a1"), nearA2)
		ancestorA := &yaml.Node{
			Kind:    yaml.MappingNode,
			Line:    99,
			Content: []*yaml.Node{newSourceScalarNode("a0"), nearA1},
		}

		// Path through B: one edge from ancestorB down to the same
		// shared offending node.
		ancestorB := &yaml.Node{
			Kind:    yaml.MappingNode,
			Line:    7,
			Content: []*yaml.Node{newSourceScalarNode("b0"), offending},
		}

		var root *yaml.Node
		if nearFirst {
			root = newSourceMappingNode(
				newSourceScalarNode("near"), ancestorB,
				newSourceScalarNode("far"), ancestorA,
			)
		} else {
			root = newSourceMappingNode(
				newSourceScalarNode("far"), ancestorA,
				newSourceScalarNode("near"), ancestorB,
			)
		}
		return newSourceDocNode(root)
	}

	t.Run("nearer ancestor ordered after the farther one", func(t *testing.T) {
		err := validateSourceGraph(path, build(false))
		wantParseType(t, err, path)
		if got := sourceDetailLine(t, err); got != 7 {
			t.Fatalf("attributed line = %d, want 7 (the nearer ancestor B)", got)
		}
	})

	t.Run("nearer ancestor ordered first", func(t *testing.T) {
		err := validateSourceGraph(path, build(true))
		wantParseType(t, err, path)
		if got := sourceDetailLine(t, err); got != 7 {
			t.Fatalf("attributed line = %d, want 7 (the nearer ancestor B)", got)
		}
	})
}

// TestValidateSourceGraphAliasSharingDAGTerminatesLinearly builds a
// compact alias-sharing DAG of ~40 doubling levels — a naive per-path
// traversal would need to expand on the order of 2^40 paths — and confirms
// validateSourceGraph (which runs both the pre-existing traversal and this
// chunk's inventory/attribution/hop-count passes) returns well within a
// five-second test-local timeout, proving all of them are linear in the
// graph's node and edge count rather than re-expanding shared subgraphs.
func TestValidateSourceGraphAliasSharingDAGTerminatesLinearly(t *testing.T) {
	doc := newSourceDocNode(buildSourceSharingDAG(40))
	if err := runSourceValidateWithin(t, "p", doc, 5*time.Second); err != nil {
		t.Fatalf("validateSourceGraph(compact alias-sharing DAG) = %v, want nil", err)
	}
}

// buildSourceVisitChainWithLeaf wraps leaf in wraps nested one-entry
// mapping levels, each contributing exactly one path visit (the mapping
// node itself; its scalar key is a shallower sibling branch that never
// wins the DP's max). The returned head node's pathVisitCount is
// wraps + pathVisitCount(leaf).
func buildSourceVisitChainWithLeaf(wraps int, leaf *yaml.Node) *yaml.Node {
	node := leaf
	for i := 0; i < wraps; i++ {
		node = newSourceMappingNode(newSourceScalarNode("k"), node)
	}
	return node
}

// buildSourceVisitChain returns a chain of nested one-entry mappings ending
// in an ordinary scalar leaf whose total path-visit count is exactly
// depth: depth-1 wrapping mappings, each contributing one visit, plus the
// leaf scalar's own single visit.
func buildSourceVisitChain(depth int) *yaml.Node {
	return buildSourceVisitChainWithLeaf(depth-1, newSourceScalarNode("leaf"))
}

// buildSourceVisitKeyChain returns a chain of nested mappings, each
// wrapping the previous level as its own KEY (not its value) with a
// shallow scalar value sibling, ending in leaf. Its total path-visit count
// is exactly wraps + pathVisitCount(leaf), proving the DP must consider
// mapping keys (not only values) to compute the deepest path, since the
// value branch alone never grows past 2.
func buildSourceVisitKeyChain(wraps int, leaf *yaml.Node) *yaml.Node {
	node := leaf
	for i := 0; i < wraps; i++ {
		node = newSourceMappingNode(node, newSourceScalarNode("v"))
	}
	return node
}

// TestValidateSourceGraphPathVisitBoundary confirms the 128/129
// source-node path-visit boundary exactly: a chain of exactly 128 visits
// succeeds, and a chain of 129 fails as KindParseType, naming the
// path-visit limit, the literal "128", and a positive "line N" — even
// though every synthesized node in the chain has Line == 0, so the
// reported line must come from the deterministic fallback. Every level of
// the chain is a mapping with a plain scalar key, so this also proves a
// scalar used as a mapping key counts exactly once, not twice: a
// double-counting implementation would report a different (larger) total
// and reject the exactly-128 case that this test requires to succeed.
func TestValidateSourceGraphPathVisitBoundary(t *testing.T) {
	const path = "/etc/chairlift/path-visit-boundary.yml"

	t.Run("exactly 128 visits succeeds", func(t *testing.T) {
		doc := newSourceDocNode(buildSourceVisitChain(128))
		if err := validateSourceGraph(path, doc); err != nil {
			t.Fatalf("validateSourceGraph(128-visit chain) = %v, want nil", err)
		}
	})

	t.Run("129 visits fails", func(t *testing.T) {
		doc := newSourceDocNode(buildSourceVisitChain(129))
		err := validateSourceGraph(path, doc)
		wantParseType(t, err, path)

		if err.Err != nil {
			t.Fatalf("err.Err = %v, want nil", err.Err)
		}
		if !strings.Contains(err.Detail, "path visit") && !strings.Contains(err.Detail, "path-node visit") {
			t.Fatalf("err.Detail = %q, want wording identifying the source-node path-visit limit", err.Detail)
		}
		if !strings.Contains(err.Detail, "128") {
			t.Fatalf("err.Detail = %q, want the literal %q", err.Detail, "128")
		}
		if got := sourceDetailLine(t, err); got != 1 {
			t.Fatalf("attributed line = %d, want 1 (wholly synthetic fallback)", got)
		}
	})
}

// TestValidateSourceGraphPathVisitAliasCounts confirms an alias node plus
// its target contributes two path visits, not one: two graphs identical
// in wrap-depth differ only in whether the deepest leaf is a direct scalar
// child or an alias node targeting that same scalar, and the substitution
// alone flips the result across the 128 boundary.
func TestValidateSourceGraphPathVisitAliasCounts(t *testing.T) {
	const path = "/etc/chairlift/path-visit-alias-counts.yml"
	const wraps = 127

	direct := buildSourceVisitChainWithLeaf(wraps, newSourceScalarNode("leaf"))
	if err := validateSourceGraph(path, newSourceDocNode(direct)); err != nil {
		t.Fatalf("validateSourceGraph(direct scalar leaf, %d wraps = 128 visits) = %v, want nil", wraps, err)
	}

	aliasedLeaf := newSourceAliasNode(newSourceScalarNode("leaf"))
	aliased := buildSourceVisitChainWithLeaf(wraps, aliasedLeaf)
	err := validateSourceGraph(path, newSourceDocNode(aliased))
	wantParseType(t, err, path)
	if !strings.Contains(err.Detail, "128") {
		t.Fatalf("err.Detail = %q, want the literal %q", err.Detail, "128")
	}
}

// TestValidateSourceGraphPathVisitKeyAccounting confirms mapping keys
// count toward the path-visit bound: a chain of nested mappings whose
// depth lives entirely in the KEY position (each level's value is a
// shallow sibling scalar) crosses the 128 boundary exactly like the
// value-position chain in TestValidateSourceGraphPathVisitBoundary does,
// which is only possible if the DP considers key children, not only value
// children.
func TestValidateSourceGraphPathVisitKeyAccounting(t *testing.T) {
	const path = "/etc/chairlift/path-visit-key-accounting.yml"

	t.Run("127 key-position wraps (128 visits) succeeds", func(t *testing.T) {
		doc := newSourceDocNode(buildSourceVisitKeyChain(127, newSourceScalarNode("leaf")))
		if err := validateSourceGraph(path, doc); err != nil {
			t.Fatalf("validateSourceGraph(127-deep key chain) = %v, want nil", err)
		}
	})

	t.Run("128 key-position wraps (129 visits) fails", func(t *testing.T) {
		doc := newSourceDocNode(buildSourceVisitKeyChain(128, newSourceScalarNode("leaf")))
		err := validateSourceGraph(path, doc)
		wantParseType(t, err, path)
		if !strings.Contains(err.Detail, "128") {
			t.Fatalf("err.Detail = %q, want the literal %q", err.Detail, "128")
		}
	})
}

// TestValidateSourceGraphPathVisitWideSiblingsShareDeepTarget confirms
// path visits do not accumulate globally: 200 sibling sequence entries all
// alias the same deep (but within-bound) shared subtree. If path-visit
// checking wrongly summed visits across siblings instead of memoizing by
// node identity, this would spuriously reject; instead it must succeed.
func TestValidateSourceGraphPathVisitWideSiblingsShareDeepTarget(t *testing.T) {
	shared := buildSourceVisitChain(100)
	entries := make([]*yaml.Node, 0, 200)
	for i := 0; i < 200; i++ {
		entries = append(entries, newSourceAliasNode(shared))
	}
	doc := newSourceDocNode(newSourceSequenceNode(entries...))

	if err := validateSourceGraph("p", doc); err != nil {
		t.Fatalf("validateSourceGraph(200 wide siblings sharing a deep target) = %v, want nil", err)
	}
}

// TestValidateSourceGraphPathVisitAttributionEqualDistanceTieBreak is the
// mandatory equal-distance tie-break case, observed end-to-end through a
// path-visit-limit error: the offending (over-limit, Line == 0) node is a
// single shared *yaml.Node reachable via two paths of the SAME minimum
// distance (two content edges) below two different positive-line
// ancestors, A and B. Per interpretation 11's tie-break, the winner is the
// ancestor active on the DFS stack at the node's first deterministic
// depth-first encounter — which is whichever of A's or B's branch the
// walk reaches first, i.e. whichever is ordered first in the root
// mapping's Content. This is asserted both ways: swapping the Content
// order flips which ancestor's line is reported.
func TestValidateSourceGraphPathVisitAttributionEqualDistanceTieBreak(t *testing.T) {
	const path = "/etc/chairlift/path-visit-equal-distance-tie.yml"

	build := func(aFirst bool) *yaml.Node {
		offending := buildSourceVisitChain(129) // shared node, Line == 0, over the limit on its own

		// Path through A: two content edges from ancestorA to offending.
		midA := newSourceMappingNode(newSourceScalarNode("mid-a"), offending)
		ancestorA := &yaml.Node{
			Kind:    yaml.MappingNode,
			Line:    50,
			Content: []*yaml.Node{newSourceScalarNode("a0"), midA},
		}

		// Path through B: also two content edges, to the same shared
		// offending node — an equal minimum distance to A's path.
		midB := newSourceMappingNode(newSourceScalarNode("mid-b"), offending)
		ancestorB := &yaml.Node{
			Kind:    yaml.MappingNode,
			Line:    60,
			Content: []*yaml.Node{newSourceScalarNode("b0"), midB},
		}

		var root *yaml.Node
		if aFirst {
			root = newSourceMappingNode(
				newSourceScalarNode("first"), ancestorA,
				newSourceScalarNode("second"), ancestorB,
			)
		} else {
			root = newSourceMappingNode(
				newSourceScalarNode("first"), ancestorB,
				newSourceScalarNode("second"), ancestorA,
			)
		}
		return newSourceDocNode(root)
	}

	t.Run("A ordered first in Content reports A's line", func(t *testing.T) {
		err := validateSourceGraph(path, build(true))
		wantParseType(t, err, path)
		if got := sourceDetailLine(t, err); got != 50 {
			t.Fatalf("attributed line = %d, want 50 (A, discovered first at equal distance)", got)
		}
	})

	t.Run("B ordered first in Content reports B's line", func(t *testing.T) {
		err := validateSourceGraph(path, build(false))
		wantParseType(t, err, path)
		if got := sourceDetailLine(t, err); got != 60 {
			t.Fatalf("attributed line = %d, want 60 (B, discovered first at equal distance)", got)
		}
	})
}

// TestValidateSourceGraphPathVisitAttributionAliasEdge confirms alias
// edges participate in attribution distance: the offending (over-limit,
// Line == 0) node is reachable both deep in ordinary content (distance 3
// from a positive-line ancestor) and, via an alias edge, one content edge
// below a different positive-line ancestor at distance 2. The nearer
// (alias-mediated) line must be reported.
func TestValidateSourceGraphPathVisitAttributionAliasEdge(t *testing.T) {
	const path = "/etc/chairlift/path-visit-alias-edge.yml"

	offending := buildSourceVisitChain(129) // shared node, Line == 0, over the limit on its own

	// Deep, alias-free path: distance 3 from a positive-line ancestor.
	deepMid2 := newSourceMappingNode(newSourceScalarNode("mid2-key"), offending)
	deepMid1 := newSourceMappingNode(newSourceScalarNode("mid1-key"), deepMid2)
	deepFar := &yaml.Node{
		Kind:    yaml.MappingNode,
		Line:    500,
		Content: []*yaml.Node{newSourceScalarNode("far-key"), deepMid1},
	}

	// Near path: an alias edge puts the same offending node at distance
	// 2 from a different, nearer positive-line ancestor.
	aliasNear := newSourceAliasNode(offending)
	nearAncestor := &yaml.Node{
		Kind:    yaml.MappingNode,
		Line:    3,
		Content: []*yaml.Node{newSourceScalarNode("near-key"), aliasNear},
	}

	root := newSourceMappingNode(
		newSourceScalarNode("deep"), deepFar,
		newSourceScalarNode("near"), nearAncestor,
	)
	doc := newSourceDocNode(root)

	err := validateSourceGraph(path, doc)
	wantParseType(t, err, path)
	if got := sourceDetailLine(t, err); got != 3 {
		t.Fatalf("attributed line = %d, want 3 (the nearer ancestor reached via the alias edge)", got)
	}
}

// TestValidateSourceGraphBothBoundsViolatedReportsAliasHopDeterministically
// confirms a graph violating both the alias-hop bound and the path-visit
// bound returns a single, deterministic KindParseType error naming the
// alias-hop limit (checked first by checkSourceGraphBounds) rather than
// panicking or somehow reporting twice: 70 consecutive alias hops (over
// the 64 limit) wrapped in 60 further ordinary-content levels (pushing the
// total path-visit count to 131, over the 128 limit) still yields exactly
// one error, and it is the alias-hop one.
func TestValidateSourceGraphBothBoundsViolatedReportsAliasHopDeterministically(t *testing.T) {
	const path = "/etc/chairlift/both-bounds-violated.yml"

	aliasChain := buildSourceAliasChain(70)                  // 70 alias hops, 71 path visits on its own
	wrapped := buildSourceVisitChainWithLeaf(60, aliasChain) // + 60 more visits = 131

	err := validateSourceGraph(path, newSourceDocNode(wrapped))
	wantParseType(t, err, path)
	if !strings.Contains(err.Detail, "consecutive") || !strings.Contains(err.Detail, "alias hop") {
		t.Fatalf("err.Detail = %q, want wording identifying the consecutive alias-hop limit (checked first)", err.Detail)
	}
	if strings.Contains(err.Detail, "path visit") {
		t.Fatalf("err.Detail = %q, want only the alias-hop limit named, not the path-visit limit too", err.Detail)
	}
}
