package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestResolveEffectiveExplicitOverridesMerged confirms an explicit key
// wins over the same key inherited through "<<", both when the "<<"
// directive precedes the explicit key in source Content order and when it
// follows it — proving explicit-over-merged precedence does not depend on
// where the merge directive appears.
func TestResolveEffectiveExplicitOverridesMerged(t *testing.T) {
	t.Run("merge directive precedes explicit key", func(t *testing.T) {
		operand := simpleMapping("k", "merge-value")
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				mergeKeyNode(), operand,
				scalarNode("k"), scalarNode("explicit-value"),
			},
		}

		out, err := resolveEffective("explicit-after-merge.yml", docOf(root))
		if err != nil {
			t.Fatalf("resolveEffective returned unexpected error: %v", err)
		}
		outRoot := out.Content[0]
		if len(outRoot.Content) != 2 {
			t.Fatalf("len(outRoot.Content) = %d, want 2 (single winning entry): %+v", len(outRoot.Content), outRoot.Content)
		}
		if outRoot.Content[0].Value != "k" || outRoot.Content[1].Value != "explicit-value" {
			t.Fatalf("got key=%q value=%q, want k=explicit-value", outRoot.Content[0].Value, outRoot.Content[1].Value)
		}
	})

	t.Run("merge directive follows explicit key", func(t *testing.T) {
		operand := simpleMapping("k", "merge-value2")
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				scalarNode("k"), scalarNode("explicit-value2"),
				mergeKeyNode(), operand,
			},
		}

		out, err := resolveEffective("explicit-before-merge.yml", docOf(root))
		if err != nil {
			t.Fatalf("resolveEffective returned unexpected error: %v", err)
		}
		outRoot := out.Content[0]
		if len(outRoot.Content) != 2 {
			t.Fatalf("len(outRoot.Content) = %d, want 2 (single winning entry): %+v", len(outRoot.Content), outRoot.Content)
		}
		if outRoot.Content[0].Value != "k" || outRoot.Content[1].Value != "explicit-value2" {
			t.Fatalf("got key=%q value=%q, want k=explicit-value2", outRoot.Content[0].Value, outRoot.Content[1].Value)
		}
	})
}

// TestResolveEffectiveEarlierSequenceOperandWins confirms that for
// "<<: [*a, *b]" where both operands define the same key, the value from
// *a appears in the result and *b's value does not appear anywhere in the
// result.
func TestResolveEffectiveEarlierSequenceOperandWins(t *testing.T) {
	a := simpleMapping("k", "from-a")
	b := simpleMapping("k", "from-b")
	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
		newSourceAliasNode(a), newSourceAliasNode(b),
	}}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{mergeKeyNode(), seq}}

	out, err := resolveEffective("seq-operand-precedence.yml", docOf(root))
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	outRoot := out.Content[0]
	if len(outRoot.Content) != 2 {
		t.Fatalf("len(outRoot.Content) = %d, want 2: %+v", len(outRoot.Content), outRoot.Content)
	}
	if outRoot.Content[0].Value != "k" || outRoot.Content[1].Value != "from-a" {
		t.Fatalf("got key=%q value=%q, want k=from-a", outRoot.Content[0].Value, outRoot.Content[1].Value)
	}
	walkEffectiveNodes(out, func(n *yaml.Node) {
		if n.Value == "from-b" {
			t.Fatalf("later sequence operand's value %q leaked into the result", n.Value)
		}
	})
}

// TestResolveEffectiveResultIsMergeFree walks every node of a result
// combining a mapping-literal merge operand, an alias merge operand, a
// sequence merge operand, a nested merge inside a retained value, and a
// merge inside a retained mapping key, and asserts isMergeKey reports
// false for every emitted mapping key and no emitted node is a
// yaml.AliasNode.
func TestResolveEffectiveResultIsMergeFree(t *testing.T) {
	litOperand := simpleMapping("lit", "lit-value")
	aliasTarget := simpleMapping("ali", "ali-value")
	seqValue := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
		litOperand,
		newSourceAliasNode(aliasTarget),
	}}

	nestedOperand := simpleMapping("nx", "nested-from-merge")
	nestedValue := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			mergeKeyNode(), nestedOperand,
			scalarNode("own"), scalarNode("own-value"),
		},
	}

	keyOperand := simpleMapping("kk-merged", "kk-merged-value")
	complexKey := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			mergeKeyNode(), keyOperand,
			scalarNode("kk"), scalarNode("kk-value"),
		},
	}

	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			mergeKeyNode(), seqValue,
			scalarNode("nested"), nestedValue,
			complexKey, scalarNode("value-for-complex-key"),
		},
	}

	out, err := resolveEffective("merge-free-result.yml", docOf(root))
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}

	visited := 0
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		visited++
		if n.Kind == yaml.AliasNode {
			t.Fatalf("result contains a yaml.AliasNode: %+v", n)
		}
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				if isMergeKey(n.Content[i]) {
					t.Fatalf("result mapping key is a recognized merge key: %+v", n.Content[i])
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(out)
	if visited == 0 {
		t.Fatalf("walk visited no nodes")
	}
}

// TestResolveEffectiveNestedMergeInRetainedValue asserts a retained value
// that is itself a mapping with its own "<<" is fully resolved, with that
// inner mapping's own explicit-over-merged and earlier-operand precedence
// applied: an explicit "x" wins over both sequence operands' "x", the
// first operand's "y" (unique to it) is inherited, and the second
// operand's "z" (unique to it) is inherited too.
func TestResolveEffectiveNestedMergeInRetainedValue(t *testing.T) {
	a := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		scalarNode("x"), scalarNode("x-from-a"),
		scalarNode("y"), scalarNode("y-from-a"),
	}}
	b := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		scalarNode("x"), scalarNode("x-from-b"),
		scalarNode("z"), scalarNode("z-from-b"),
	}}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
		newSourceAliasNode(a), newSourceAliasNode(b),
	}}
	inner := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			mergeKeyNode(), seq,
			scalarNode("x"), scalarNode("explicit-x"),
		},
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("outer"), inner}}

	out, err := resolveEffective("nested-merge-in-value.yml", docOf(root))
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	outRoot := out.Content[0]
	if len(outRoot.Content) != 2 || outRoot.Content[0].Value != "outer" {
		t.Fatalf("unexpected outer shape: %+v", outRoot.Content)
	}

	innerOut := outRoot.Content[1]
	got := map[string]string{}
	for i := 0; i+1 < len(innerOut.Content); i += 2 {
		got[innerOut.Content[i].Value] = innerOut.Content[i+1].Value
	}
	want := map[string]string{"x": "explicit-x", "y": "y-from-a", "z": "z-from-b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("got[%q] = %q, want %q (full: %v)", k, got[k], v, got)
		}
	}
}

// TestResolveEffectiveEntryOrderIsDeterministic asserts the emitted
// mapping's key order is exactly: retained explicit entries in source
// Content order (merge directives excluded), then inherited winners in
// merge-candidate discovery order (directives in Content order, sequence
// operands first to last, each operand's own effective entry order). It
// runs the same fixture twice and asserts identical order both times.
func TestResolveEffectiveEntryOrderIsDeterministic(t *testing.T) {
	opA := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		scalarNode("p"), scalarNode("p-from-a"),
		scalarNode("q"), scalarNode("q-from-a"),
	}}
	opB := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		scalarNode("q"), scalarNode("q-from-b"),
		scalarNode("r"), scalarNode("r-from-b"),
	}}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{opA, opB}}
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode("explicit1"), scalarNode("e1"),
			mergeKeyNode(), seq,
			scalarNode("explicit2"), scalarNode("e2"),
		},
	}
	doc := docOf(root)

	want := []string{"explicit1", "explicit2", "p", "q", "r"}

	for run := 0; run < 2; run++ {
		out, err := resolveEffective("order-deterministic.yml", doc)
		if err != nil {
			t.Fatalf("run %d: resolveEffective returned unexpected error: %v", run, err)
		}
		outRoot := out.Content[0]
		var got []string
		for i := 0; i+1 < len(outRoot.Content); i += 2 {
			got = append(got, outRoot.Content[i].Value)
		}
		if len(got) != len(want) {
			t.Fatalf("run %d: got key order %v, want %v", run, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("run %d: got key order %v, want %v", run, got, want)
			}
		}
	}
}

// TestResolveEffectiveEntryOrderIsDeterministicThroughNestedOperand asserts
// the same ordering contract as
// TestResolveEffectiveEntryOrderIsDeterministic still holds when a merge
// operand's own effective entries are themselves recursively computed
// from a nested merge, rather than being only explicit entries: opA here
// merges from opAInner, so opA's own effective order (its explicit "p"
// and "q" first, then its inherited "s") must be discovered correctly
// before it is spliced into root's inherited entries in operand order.
func TestResolveEffectiveEntryOrderIsDeterministicThroughNestedOperand(t *testing.T) {
	opAInner := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		scalarNode("s"), scalarNode("s-from-inner"),
	}}
	opA := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		mergeKeyNode(), opAInner,
		scalarNode("p"), scalarNode("p-from-a"),
		scalarNode("q"), scalarNode("q-from-a"),
	}}
	opB := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		scalarNode("q"), scalarNode("q-from-b"),
		scalarNode("r"), scalarNode("r-from-b"),
	}}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{opA, opB}}
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode("explicit1"), scalarNode("e1"),
			mergeKeyNode(), seq,
			scalarNode("explicit2"), scalarNode("e2"),
		},
	}
	doc := docOf(root)

	want := []string{"explicit1", "explicit2", "p", "q", "s", "r"}

	out, err := resolveEffective("order-deterministic-nested.yml", doc)
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	outRoot := out.Content[0]
	var got []string
	for i := 0; i+1 < len(outRoot.Content); i += 2 {
		got = append(got, outRoot.Content[i].Value)
	}
	if len(got) != len(want) {
		t.Fatalf("got key order %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got key order %v, want %v", got, want)
		}
	}
}

// TestResolveEffectiveMergedKeyIdentityIsTagAware asserts an inherited
// integer key `1` and an explicit quoted string key "1" both survive as
// separate entries (proving effectiveKeyIdentity, not sourceKeyID, drives
// merge precedence), and that an inherited bare `1` is suppressed by an
// explicit !!int-tagged `1`.
func TestResolveEffectiveMergedKeyIdentityIsTagAware(t *testing.T) {
	t.Run("distinct tags both survive", func(t *testing.T) {
		bareOneOperand := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			scalarNode("1"), scalarNode("from-merge"),
		}}
		explicitQuotedOne := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "1"}
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				mergeKeyNode(), bareOneOperand,
				explicitQuotedOne, scalarNode("explicit-value"),
			},
		}

		out, err := resolveEffective("tag-aware-distinct.yml", docOf(root))
		if err != nil {
			t.Fatalf("resolveEffective returned unexpected error: %v", err)
		}
		outRoot := out.Content[0]
		if len(outRoot.Content) != 4 {
			t.Fatalf("len(outRoot.Content) = %d, want 4 (both survive as distinct entries): %+v", len(outRoot.Content), outRoot.Content)
		}
		var sawBareInt, sawQuotedStr bool
		for i := 0; i+1 < len(outRoot.Content); i += 2 {
			key, value := outRoot.Content[i], outRoot.Content[i+1]
			switch {
			case key.Tag == "!!str" && key.Value == "1":
				sawQuotedStr = true
				if value.Value != "explicit-value" {
					t.Fatalf("quoted \"1\" entry value = %q, want %q", value.Value, "explicit-value")
				}
			case key.Value == "1":
				sawBareInt = true
				if value.Value != "from-merge" {
					t.Fatalf("bare 1 entry value = %q, want %q", value.Value, "from-merge")
				}
			}
		}
		if !sawBareInt || !sawQuotedStr {
			t.Fatalf("outRoot.Content = %+v, want both a bare-1 and a quoted-\"1\" entry", outRoot.Content)
		}
	})

	t.Run("matching tag suppresses inherited", func(t *testing.T) {
		bareOneOperand := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			scalarNode("1"), scalarNode("from-merge2"),
		}}
		explicitTaggedIntOne := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"}
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				explicitTaggedIntOne, scalarNode("explicit-value2"),
				mergeKeyNode(), bareOneOperand,
			},
		}

		out, err := resolveEffective("tag-aware-suppressed.yml", docOf(root))
		if err != nil {
			t.Fatalf("resolveEffective returned unexpected error: %v", err)
		}
		outRoot := out.Content[0]
		if len(outRoot.Content) != 2 {
			t.Fatalf("len(outRoot.Content) = %d, want 2 (inherited bare 1 suppressed): %+v", len(outRoot.Content), outRoot.Content)
		}
		if outRoot.Content[0].Value != "1" || outRoot.Content[1].Value != "explicit-value2" {
			t.Fatalf("got key=%q value=%q, want 1=explicit-value2", outRoot.Content[0].Value, outRoot.Content[1].Value)
		}
		walkEffectiveNodes(out, func(n *yaml.Node) {
			if n.Value == "from-merge2" {
				t.Fatalf("suppressed inherited value %q leaked into the result", n.Value)
			}
		})
	})
}

// buildMergeSharingDAG returns a well-formed yaml.MappingNode built as a
// chain of levels, each merging two aliases to the same previous level via
// a sequence merge operand: level 0 is a small mapping with one key/value
// pair, and level i (i>0) is "<<: [*level(i-1), *level(i-1)]". Its
// *source* graph is a tiny, compact DAG well within
// checkSourceGraphBounds' 64-consecutive-alias-hop and
// 128-source-node-path-visit limits (mirroring buildSourceSharingDAG's
// construction), but a naive, non-memoized computation of a mapping's
// effectiveEntries would recompute level (i-1)'s inventory twice per
// level — 2^levels total recursive computations — while a memoized one
// (keyed by mapping-node pointer identity) computes each level's
// inventory exactly once, giving
// TestResolveEffectiveSharedOperandInventoriedOnce a bounded runtime only
// the memoized implementation can meet.
func buildMergeSharingDAG(levels int) *yaml.Node {
	level := simpleMapping("leaf", "leaf-value")
	for i := 0; i < levels; i++ {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
			newSourceAliasNode(level), newSourceAliasNode(level),
		}}
		level = &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{mergeKeyNode(), seq}}
	}
	return level
}

// TestResolveEffectiveSharedOperandInventoriedOnce builds one operand
// mapping merged into many parent mappings (buildMergeSharingDAG's
// 40-level doubling chain) and asserts resolveEffective completes within a
// bounded timeout, demonstrating the memoized inventory: without
// memoization this would require on the order of 2^40 recursive
// effectiveEntries computations and would not return within any
// reasonable timeout.
func TestResolveEffectiveSharedOperandInventoriedOnce(t *testing.T) {
	const path = "/etc/chairlift/shared-operand-inventoried-once.yml"
	doc := newSourceDocNode(buildMergeSharingDAG(40))

	out, err := runResolveEffectiveWithin(t, path, doc, 5*time.Second)
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	outRoot := out.Content[0]
	if len(outRoot.Content) != 2 || outRoot.Content[0].Value != "leaf" || outRoot.Content[1].Value != "leaf-value" {
		t.Fatalf("outRoot.Content = %+v, want a single leaf=leaf-value entry", outRoot.Content)
	}
}

// TestResolveEffectiveSharedOperandMemoSpansEmittedMappings proves the
// candidate-inventory memo belongs to the whole resolveEffective call, not
// merely to one top-level mapping inventory. Thousands of distinct retained
// parent mappings all merge the same expensive shared operand. Each parent
// explicitly suppresses the operand's sole effective key, keeping output
// small; without a resolver-wide memo, the shared operand's wide inventory
// would be rebuilt once per parent and the test would exceed its timeout.
func TestResolveEffectiveSharedOperandMemoSpansEmittedMappings(t *testing.T) {
	const (
		path   = "shared-operand-across-emitted-mappings.yml"
		fanout = 8000
	)

	operandMappings := make([]*yaml.Node, 0, fanout)
	for i := 0; i < fanout; i++ {
		operandMappings = append(operandMappings, &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				scalarNode("shared"), scalarNode("losing"),
			},
		})
	}
	sharedOperand := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			mergeKeyNode(), {Kind: yaml.SequenceNode, Content: operandMappings},
		},
	}

	parents := make([]*yaml.Node, 0, fanout)
	for i := 0; i < fanout; i++ {
		parents = append(parents, &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				mergeKeyNode(), {Kind: yaml.AliasNode, Alias: sharedOperand},
				scalarNode("shared"), scalarNode("winner"),
			},
		})
	}
	doc := docOf(&yaml.Node{Kind: yaml.SequenceNode, Content: parents})

	out, err := runResolveEffectiveWithin(t, path, doc, 5*time.Second)
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	if got := len(out.Content[0].Content); got != fanout {
		t.Fatalf("emitted parent count = %d, want %d", got, fanout)
	}
	for i, parent := range out.Content[0].Content {
		if len(parent.Content) != 2 || parent.Content[0].Value != "shared" || parent.Content[1].Value != "winner" {
			t.Fatalf("parent %d content = %#v, want only explicit shared: winner", i, parent.Content)
		}
	}
}
