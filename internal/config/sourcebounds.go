package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// maxAliasHops is the maximum number of consecutive alias hops
// checkAliasHopLimit accepts. Following an alias to its target adds one
// hop; descending from any non-alias node into ordinary content resets
// the count for that child path. Exactly maxAliasHops succeeds;
// maxAliasHops+1 fails.
const maxAliasHops = 64

// sourceEdge is one directed parent->child content edge recorded by
// collectSourceInventory: a mapping key or value, a sequence entry, or an
// alias node's target. A node reached through several parents contributes
// one sourceEdge per parent, even though collectSourceInventory's walk
// visits the node's own content only once.
type sourceEdge struct {
	parent *yaml.Node
	child  *yaml.Node
}

// sourceInventory is the result of one deterministic depth-first walk of a
// source graph already proven acyclic and well-formed by walkSourceNode:
// every reachable node in discovery order, every parent->child content
// edge reachable in the graph (including edges into a node reached more
// than once, which is what lets attributeSourceLines consider every
// root-reachable path rather than only the first one discovered), each
// node's discovery index (for deterministic tie-breaking), and each node's
// first-encounter active nearest positive-line ancestor (the ancestor
// interpretation 11's tie-break falls back to when two candidates are
// otherwise equally near).
type sourceInventory struct {
	nodes          []*yaml.Node
	discoveryIndex map[*yaml.Node]int
	edges          []sourceEdge
	firstAncestor  map[*yaml.Node]*yaml.Node
}

// collectSourceInventory walks root exactly once per unique node, in
// Content slice order, recording every node and every parent->child
// content edge reachable from it. root must already be proven acyclic
// (walkSourceNode's job): collectSourceInventory tracks only a simple
// visited set, not the visiting/done cycle guard, and would recurse
// forever on a graph containing a cycle. A node's discovery index and
// first-encounter active ancestor are fixed the first time the walk
// reaches it; every edge into it — including from parents reached after
// that first visit — is still appended to inv.edges, so attributeSourceLines
// can later consider every path into a shared node, not just the one the
// walk happened to take first.
func collectSourceInventory(root *yaml.Node) *sourceInventory {
	inv := &sourceInventory{
		discoveryIndex: make(map[*yaml.Node]int),
		firstAncestor:  make(map[*yaml.Node]*yaml.Node),
	}
	visited := make(map[*yaml.Node]bool)

	var walk func(n *yaml.Node, activeAncestor *yaml.Node)
	walk = func(n *yaml.Node, activeAncestor *yaml.Node) {
		if visited[n] {
			return
		}
		visited[n] = true
		inv.discoveryIndex[n] = len(inv.nodes)
		inv.nodes = append(inv.nodes, n)
		inv.firstAncestor[n] = activeAncestor

		nextAncestor := activeAncestor
		if n.Line > 0 {
			nextAncestor = n
		}

		switch n.Kind {
		case yaml.MappingNode, yaml.SequenceNode:
			for _, child := range n.Content {
				inv.edges = append(inv.edges, sourceEdge{parent: n, child: child})
				walk(child, nextAncestor)
			}
		case yaml.AliasNode:
			if n.Alias != nil {
				inv.edges = append(inv.edges, sourceEdge{parent: n, child: n.Alias})
				walk(n.Alias, nextAncestor)
			}
		}
	}
	walk(root, nil)
	return inv
}

// attributeSourceLines computes, for every node in inv.nodes, the source
// line a bounds-limit error should report for it (interpretation 11): a
// node's own Line when positive; otherwise the line of a positive-line
// ancestor at minimum content-edge distance over all root-reachable paths,
// not merely over the path of the node's first DFS encounter; otherwise 1,
// for a node with no positive-line ancestor reachable on any path (a
// wholly synthetic graph).
//
// This is a multi-source BFS in the parent->child direction, seeded with
// every reachable positive-line node at distance 0 attributing its own
// line, relaxing each edge parent->child with (dist(parent)+1,
// line(parent)). Because inv already lists every reachable node and edge
// exactly once, this pass touches each at most once and is O(V+E) — it
// never re-expands a shared subgraph once per path, which is what keeps a
// compact alias-sharing DAG linear instead of exponential.
//
// When two positive-line ancestors tie at the same minimum distance to a
// node, the one that was that node's first-encounter active ancestor
// (inv.firstAncestor) wins if it is among the tied candidates; otherwise
// the candidate reached through the parent with the lowest discovery
// index wins, with each parent's own children considered in Content slice
// order (both already implied by inv.edges' construction order).
func attributeSourceLines(inv *sourceInventory) map[*yaml.Node]int {
	children := make(map[*yaml.Node][]*yaml.Node, len(inv.nodes))
	for _, e := range inv.edges {
		children[e.parent] = append(children[e.parent], e.child)
	}

	dist := make(map[*yaml.Node]int, len(inv.nodes))
	ancestor := make(map[*yaml.Node]*yaml.Node, len(inv.nodes))
	viaParent := make(map[*yaml.Node]*yaml.Node, len(inv.nodes))

	queue := make([]*yaml.Node, 0, len(inv.nodes))
	for _, n := range inv.nodes {
		if n.Line > 0 {
			dist[n] = 0
			ancestor[n] = n
			queue = append(queue, n)
		}
	}

	for i := 0; i < len(queue); i++ {
		p := queue[i]
		nd := dist[p] + 1
		for _, c := range children[p] {
			if c.Line > 0 {
				// c is itself a positive-line seed; it is never
				// attributed from an ancestor.
				continue
			}
			existing, seen := dist[c]
			switch {
			case !seen:
				dist[c] = nd
				ancestor[c] = ancestor[p]
				viaParent[c] = p
				queue = append(queue, c)
			case nd == existing:
				if preferSourceLineAncestor(inv, c, ancestor[c], viaParent[c], ancestor[p], p) {
					ancestor[c] = ancestor[p]
					viaParent[c] = p
				}
			}
		}
	}

	lines := make(map[*yaml.Node]int, len(inv.nodes))
	for _, n := range inv.nodes {
		switch {
		case n.Line > 0:
			lines[n] = n.Line
		case ancestor[n] != nil:
			lines[n] = ancestor[n].Line
		default:
			lines[n] = 1
		}
	}
	return lines
}

// preferSourceLineAncestor decides, for child c already tentatively
// attributed to curAncestor (reached via curParent) at the minimum known
// distance, whether a newly discovered equally-near candidate
// (newAncestor, reached via newParent) should replace it. It implements
// interpretation 11's tie-break: the child's first-encounter active
// ancestor wins if it is one of the tied candidates; otherwise the
// candidate reached through the lower-discovery-index parent wins.
func preferSourceLineAncestor(inv *sourceInventory, c, curAncestor, curParent, newAncestor, newParent *yaml.Node) bool {
	firstEncountered := inv.firstAncestor[c]
	if newAncestor == firstEncountered {
		return true
	}
	if curAncestor == firstEncountered {
		return false
	}
	return inv.discoveryIndex[newParent] < inv.discoveryIndex[curParent]
}

// aliasHopCount is the memoized consecutive-alias-hop DP: hops(n) = 1 +
// hops(n.Alias) when n is a yaml.AliasNode, and 0 for every other kind, so
// descending into ordinary (non-alias) content always resets the count for
// that child path. memo is keyed by node identity and shared across calls
// for the same graph, so a node reachable through many parents (the
// compact alias-sharing DAG the timeout test exercises) is computed once
// rather than once per path.
func aliasHopCount(n *yaml.Node, memo map[*yaml.Node]int) int {
	if n == nil || n.Kind != yaml.AliasNode {
		return 0
	}
	if v, ok := memo[n]; ok {
		return v
	}
	v := 1 + aliasHopCount(n.Alias, memo)
	memo[n] = v
	return v
}

// checkAliasHopLimit rejects a source graph containing a node whose
// aliasHopCount exceeds maxAliasHops, naming the limit and reporting
// lines[n] (the offending node's own positive line, its nearest
// positive-line ancestor over all paths, or 1 for a wholly synthetic
// graph — see attributeSourceLines) as the error's source line. inv.nodes
// is visited in discovery order, so the result is deterministic even when
// more than one node exceeds the limit.
func checkAliasHopLimit(path string, inv *sourceInventory, lines map[*yaml.Node]int) *LoadError {
	memo := make(map[*yaml.Node]int, len(inv.nodes))
	for _, n := range inv.nodes {
		if aliasHopCount(n, memo) > maxAliasHops {
			line := lines[n]
			if line <= 0 {
				line = 1
			}
			detail := fmt.Sprintf(
				"source graph exceeds the maximum of %d consecutive alias hops (line %d)",
				maxAliasHops, line,
			)
			return sourceGraphError(path, detail)
		}
	}
	return nil
}

// checkSourceGraphBounds is this package's single post-acyclicity source-graph
// bounds pass: it collects the reachable inventory, attributes a source line to
// every reachable node, and rejects a graph exceeding maxAliasHops
// consecutive alias hops. It must only ever be called on a root already
// proven acyclic by walkSourceNode — collectSourceInventory and
// aliasHopCount both assume that and would not terminate otherwise.
func checkSourceGraphBounds(path string, root *yaml.Node) *LoadError {
	inv := collectSourceInventory(root)
	lines := attributeSourceLines(inv)
	return checkAliasHopLimit(path, inv, lines)
}
