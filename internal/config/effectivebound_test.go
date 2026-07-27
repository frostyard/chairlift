package config

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// buildWideMapping returns a well-formed yaml.MappingNode with exactly
// pairs key/value entries, each a distinct plain scalar with no Line set,
// for exercising resolveEffective's maxEffectiveOutputNodes boundary: the
// emitted node count for docOf(buildWideMapping(pairs)) is exactly
// 1 (document) + 1 (mapping) + 2*pairs (keys and values).
func buildWideMapping(pairs int) *yaml.Node {
	content := make([]*yaml.Node, 0, pairs*2)
	for i := 0; i < pairs; i++ {
		content = append(content,
			newSourceScalarNode("k"+strconv.Itoa(i)), newSourceScalarNode("v"),
		)
	}
	return newSourceMappingNode(content...)
}

// countEffectiveNodes returns the number of nodes reachable from n via
// walkEffectiveNodes.
func countEffectiveNodes(n *yaml.Node) int {
	count := 0
	walkEffectiveNodes(n, func(*yaml.Node) { count++ })
	return count
}

// TestResolveEffectiveOutputNodeLimitBoundary confirms the
// 100,000/100,001 emitted-node boundary exactly: a document containing a
// single mapping of exactly 49,999 key/value pairs emits exactly
// 1 (document) + 1 (mapping) + 2*49,999 = 100,000 nodes and succeeds,
// while 50,000 pairs (100,002 nodes) fails with a nil result and a
// non-nil *LoadError.
func TestResolveEffectiveOutputNodeLimitBoundary(t *testing.T) {
	const path = "/etc/chairlift/output-node-boundary.yml"

	t.Run("exactly 100000 nodes succeeds", func(t *testing.T) {
		doc := newSourceDocNode(buildWideMapping(49999))
		out, err := resolveEffective(path, doc)
		if err != nil {
			t.Fatalf("resolveEffective(49999 pairs) err = %v, want nil", err)
		}
		if got := countEffectiveNodes(out); got != maxEffectiveOutputNodes {
			t.Fatalf("countEffectiveNodes = %d, want %d", got, maxEffectiveOutputNodes)
		}
	})

	t.Run("100002 nodes fails", func(t *testing.T) {
		doc := newSourceDocNode(buildWideMapping(50000))
		out, err := resolveEffective(path, doc)
		if out != nil {
			t.Fatalf("resolveEffective(50000 pairs) node = %v, want nil", out)
		}
		if err == nil {
			t.Fatalf("resolveEffective(50000 pairs) err = nil, want a *LoadError")
		}
	})
}

// TestResolveEffectiveOutputLimitErrorShape confirms the shape of the
// *LoadError resolveEffective returns when the emitted-node budget is
// exceeded: KindParseType, Path copied from the path argument, a nil Err,
// a Detail naming the maxEffectiveOutputNodes bound, and a positive "line
// N" substring in Detail.
func TestResolveEffectiveOutputLimitErrorShape(t *testing.T) {
	const path = "/etc/chairlift/output-limit-shape.yml"

	doc := newSourceDocNode(buildWideMapping(50000))
	out, err := resolveEffective(path, doc)
	if out != nil {
		t.Fatalf("resolveEffective node = %v, want nil", out)
	}
	wantParseType(t, err, path)
	if err.Err != nil {
		t.Fatalf("err.Err = %v, want nil", err.Err)
	}
	if err.Path != path {
		t.Fatalf("err.Path = %q, want %q", err.Path, path)
	}
	if !strings.Contains(err.Detail, "100000") {
		t.Fatalf("err.Detail = %q, want the literal %q", err.Detail, "100000")
	}
	if got := sourceDetailLine(t, err); got <= 0 {
		t.Fatalf("attributed line = %d, want > 0", got)
	}
}

// runResolveEffectiveWithin runs resolveEffective(path, doc) on a
// goroutine and fails the test if it has not returned within timeout,
// mirroring runSourceValidateWithin: the only way to prove a
// non-terminating (exponential) emission fails the test instead of simply
// hanging the test binary.
func runResolveEffectiveWithin(t *testing.T, path string, doc *yaml.Node, timeout time.Duration) (*yaml.Node, *LoadError) {
	t.Helper()
	type result struct {
		node *yaml.Node
		err  *LoadError
	}
	resultCh := make(chan result, 1)
	go func() {
		node, err := resolveEffective(path, doc)
		resultCh <- result{node: node, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-resultCh:
		return r.node, r.err
	case <-timer.C:
		t.Fatalf("resolveEffective(%q, ...) did not return within %s", path, timeout)
		return nil, nil
	}
}

// TestResolveEffectiveAliasExpansionChargedPerCopy builds a compact
// alias-sharing DAG (buildSourceSharingDAG) of 40 doubling levels: its
// *source* graph is tiny (three nodes per level, well under both
// maxSourcePathVisits=128 and maxAliasHops=64), but its alias-free
// *expanded* output would need on the order of 2^40 nodes if fully
// materialized, since each level's two aliases both dereference to (and
// so each independently re-emit a full copy of) the same shared previous
// level. This confirms resolveEffective returns the output-limit error —
// not a hang and not success — and, because emitEffectiveNode aborts as
// soon as its budget is exceeded rather than continuing to expand, does
// so well within a five-second bounded timeout.
func TestResolveEffectiveAliasExpansionChargedPerCopy(t *testing.T) {
	const path = "/etc/chairlift/alias-expansion-charged.yml"

	doc := newSourceDocNode(buildSourceSharingDAG(40))
	out, err := runResolveEffectiveWithin(t, path, doc, 5*time.Second)
	if out != nil {
		t.Fatalf("resolveEffective(compact alias-sharing DAG) node = %v, want nil", out)
	}
	wantParseType(t, err, path)
	if !strings.Contains(err.Detail, "100000") {
		t.Fatalf("err.Detail = %q, want the literal %q", err.Detail, "100000")
	}
}

// TestResolveEffectiveOutputLimitLineFallsBackToOne confirms that for a
// wholly synthetic over-limit graph with no positive Line metadata
// anywhere, the reported line falls back to 1 — attributeSourceLines'
// documented fallback for a graph with no positive-line node reachable on
// any path — proving the error's line is always positive even when no
// source node offers one.
func TestResolveEffectiveOutputLimitLineFallsBackToOne(t *testing.T) {
	const path = "/etc/chairlift/output-limit-line-fallback.yml"

	doc := newSourceDocNode(buildWideMapping(50000))
	_, err := resolveEffective(path, doc)
	wantParseType(t, err, path)
	if got := sourceDetailLine(t, err); got != 1 {
		t.Fatalf("attributed line = %d, want 1 (wholly synthetic fallback)", got)
	}
}
