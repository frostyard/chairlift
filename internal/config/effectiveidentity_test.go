package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestResolveEffectiveAliasKeyCollidesWithDirectScalarKey builds a mapping
// with two explicit keys — a direct scalar "foo" and an alias to an
// anchored scalar "foo" — that sourceKeyID's tag-blind Kind+Value
// comparison does not catch as a duplicate (the alias node's own Kind is
// yaml.AliasNode and its own Value is "", both different from the direct
// scalar key's yaml.ScalarNode/"foo"), so validateSourceGraph accepts the
// fixture. It confirms resolveEffective's post-alias effective-identity
// collision rule catches it anyway: a nil result and a *LoadError with
// Kind == KindParseType, Path copied from the passed path, a nil Err, and
// a Detail whose parsed line equals the LATER key's (the alias key's) own
// positive Line.
func TestResolveEffectiveAliasKeyCollidesWithDirectScalarKey(t *testing.T) {
	const path = "/etc/chairlift/alias-key-collision.yml"

	anchoredFoo := &yaml.Node{Kind: yaml.ScalarNode, Anchor: "foo-anchor", Value: "foo", Line: 5}
	directKey := &yaml.Node{Kind: yaml.ScalarNode, Value: "foo", Line: 3}
	aliasKey := &yaml.Node{Kind: yaml.AliasNode, Alias: anchoredFoo, Line: 9}
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode("def"), anchoredFoo,
			directKey, scalarNode("direct-value"),
			aliasKey, scalarNode("alias-value"),
		},
	}
	doc := docOf(root)

	if err := validateSourceGraph(path, doc); err != nil {
		t.Fatalf("validateSourceGraph unexpectedly rejected fixture: %v", err)
	}

	out, err := resolveEffective(path, doc)
	if out != nil {
		t.Fatalf("resolveEffective(...) node = %v, want nil", out)
	}
	wantParseType(t, err, path)
	if err.Err != nil {
		t.Fatalf("err.Err = %v, want nil", err.Err)
	}
	if got := sourceDetailLine(t, err); got != aliasKey.Line {
		t.Fatalf("attributed line = %d, want the later key's own line %d", got, aliasKey.Line)
	}
}

// TestResolveEffectiveCollisionLineFallsBackWhenUnpositioned confirms the
// collision error's reported line uses the same own-line / nearest-
// positive-line-ancestor / 1-fallback contract as the c3 output-limit
// error: when the later colliding key has no positive Line of its own,
// the reported line is its nearest positive-line ancestor over all paths,
// and 1 when the whole graph carries no line metadata at all. The
// reported line is > 0 in every case.
func TestResolveEffectiveCollisionLineFallsBackWhenUnpositioned(t *testing.T) {
	t.Run("falls back to nearest positive-line ancestor", func(t *testing.T) {
		const path = "/etc/chairlift/collision-line-ancestor-fallback.yml"

		anchoredFoo := &yaml.Node{Kind: yaml.ScalarNode, Value: "foo"}
		directKey := &yaml.Node{Kind: yaml.ScalarNode, Value: "foo"}
		aliasKey := &yaml.Node{Kind: yaml.AliasNode, Alias: anchoredFoo}
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Line: 11,
			Content: []*yaml.Node{
				scalarNode("def"), anchoredFoo,
				directKey, scalarNode("direct-value"),
				aliasKey, scalarNode("alias-value"),
			},
		}
		doc := docOf(root)

		if err := validateSourceGraph(path, doc); err != nil {
			t.Fatalf("validateSourceGraph unexpectedly rejected fixture: %v", err)
		}

		out, err := resolveEffective(path, doc)
		if out != nil {
			t.Fatalf("resolveEffective(...) node = %v, want nil", out)
		}
		wantParseType(t, err, path)
		if got := sourceDetailLine(t, err); got != 11 {
			t.Fatalf("attributed line = %d, want the nearest positive-line ancestor's line 11", got)
		}
	})

	t.Run("falls back to 1 when no line metadata anywhere", func(t *testing.T) {
		const path = "/etc/chairlift/collision-line-one-fallback.yml"

		anchoredFoo := &yaml.Node{Kind: yaml.ScalarNode, Value: "foo"}
		directKey := &yaml.Node{Kind: yaml.ScalarNode, Value: "foo"}
		aliasKey := &yaml.Node{Kind: yaml.AliasNode, Alias: anchoredFoo}
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				scalarNode("def"), anchoredFoo,
				directKey, scalarNode("direct-value"),
				aliasKey, scalarNode("alias-value"),
			},
		}
		doc := docOf(root)

		if err := validateSourceGraph(path, doc); err != nil {
			t.Fatalf("validateSourceGraph unexpectedly rejected fixture: %v", err)
		}

		out, err := resolveEffective(path, doc)
		if out != nil {
			t.Fatalf("resolveEffective(...) node = %v, want nil", out)
		}
		wantParseType(t, err, path)
		if got := sourceDetailLine(t, err); got != 1 {
			t.Fatalf("attributed line = %d, want the wholly synthetic fallback 1", got)
		}
	})
}

// TestResolveEffectiveInheritedCollisionIsSuppressionNotError asserts an
// explicit key colliding with an INHERITED merge candidate of the same
// effective identity returns no error and keeps the explicit value,
// proving the collision rule applies only to two EXPLICIT keys of one
// source mapping, not to an explicit key suppressing an inherited one.
func TestResolveEffectiveInheritedCollisionIsSuppressionNotError(t *testing.T) {
	operand := simpleMapping("shared", "inherited-value")
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode("shared"), scalarNode("explicit-value"),
			mergeKeyNode(), operand,
		},
	}

	out, err := resolveEffective("inherited-collision-suppression.yml", docOf(root))
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	outRoot := out.Content[0]
	if len(outRoot.Content) != 2 {
		t.Fatalf("len(outRoot.Content) = %d, want 2 (single winning entry): %+v", len(outRoot.Content), outRoot.Content)
	}
	if outRoot.Content[0].Value != "shared" || outRoot.Content[1].Value != "explicit-value" {
		t.Fatalf("got key=%q value=%q, want shared=explicit-value", outRoot.Content[0].Value, outRoot.Content[1].Value)
	}
	walkEffectiveNodes(out, func(n *yaml.Node) {
		if n.Value == "inherited-value" {
			t.Fatalf("suppressed inherited value leaked into the result")
		}
	})
}

// TestResolveEffectiveValidExplicitOverridesInvalidInherited asserts the
// explicit valid value is present in the result, and the invalid
// inherited value's content — a shape the next strict-schema slice would
// reject — appears nowhere in the emitted tree, because it is suppressed
// and so never resolved or emitted.
func TestResolveEffectiveValidExplicitOverridesInvalidInherited(t *testing.T) {
	invalidInherited := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{scalarNode("invalid-inherited-content")}}
	operand := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("k"), invalidInherited}}
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			mergeKeyNode(), operand,
			scalarNode("k"), scalarNode("valid-explicit-value"),
		},
	}

	out, err := resolveEffective("valid-overrides-invalid.yml", docOf(root))
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	outRoot := out.Content[0]
	if len(outRoot.Content) != 2 {
		t.Fatalf("len(outRoot.Content) = %d, want 2 (single winning entry): %+v", len(outRoot.Content), outRoot.Content)
	}
	if outRoot.Content[0].Value != "k" || outRoot.Content[1].Value != "valid-explicit-value" {
		t.Fatalf("got key=%q value=%q, want k=valid-explicit-value", outRoot.Content[0].Value, outRoot.Content[1].Value)
	}
	walkEffectiveNodes(out, func(n *yaml.Node) {
		if n.Value == "invalid-inherited-content" {
			t.Fatalf("invalid inherited content leaked into the result")
		}
	})
}

// TestResolveEffectiveInvalidLosingSequenceValueUnmaterialized asserts a
// later sequence operand's structurally odd/invalid-for-ChairLift value
// never appears in the result and produces no error, because a losing
// merge candidate's value is never resolved or emitted regardless of its
// own shape.
func TestResolveEffectiveInvalidLosingSequenceValueUnmaterialized(t *testing.T) {
	a := simpleMapping("k", "valid-from-a")
	invalidB := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{scalarNode("invalid-losing-content")}}
	b := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("k"), invalidB}}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
		newSourceAliasNode(a), newSourceAliasNode(b),
	}}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{mergeKeyNode(), seq}}

	out, err := resolveEffective("invalid-losing-sequence-value.yml", docOf(root))
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	outRoot := out.Content[0]
	if len(outRoot.Content) != 2 {
		t.Fatalf("len(outRoot.Content) = %d, want 2 (single winning entry): %+v", len(outRoot.Content), outRoot.Content)
	}
	if outRoot.Content[0].Value != "k" || outRoot.Content[1].Value != "valid-from-a" {
		t.Fatalf("got key=%q value=%q, want k=valid-from-a", outRoot.Content[0].Value, outRoot.Content[1].Value)
	}
	walkEffectiveNodes(out, func(n *yaml.Node) {
		if n.Value == "invalid-losing-content" {
			t.Fatalf("invalid losing operand content leaked into the result")
		}
	})
}

// TestResolveEffectiveInvalidWinningValueRetained asserts a winning value
// that the next strict-schema slice would reject (a sequence where a
// mapping is expected) is present in the result verbatim and produces no
// error from resolveEffective, which performs no schema validation of its
// own.
func TestResolveEffectiveInvalidWinningValueRetained(t *testing.T) {
	invalidWinning := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{scalarNode("unexpected-sequence-element")}}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("pages"), invalidWinning}}

	out, err := resolveEffective("invalid-winning-retained.yml", docOf(root))
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	outRoot := out.Content[0]
	if len(outRoot.Content) != 2 {
		t.Fatalf("len(outRoot.Content) = %d, want 2: %+v", len(outRoot.Content), outRoot.Content)
	}
	valueOut := outRoot.Content[1]
	if valueOut.Kind != yaml.SequenceNode || len(valueOut.Content) != 1 || valueOut.Content[0].Value != "unexpected-sequence-element" {
		t.Fatalf("winning value not retained verbatim: %+v", valueOut)
	}
}

// TestResolveEffectiveSharedAliasDAGInLosingBranchTerminates builds a
// compact alias-sharing DAG (buildSourceSharingDAG, deep enough that naive
// per-path expansion is exponential, while staying within the
// 64-alias-hop and 128-path-visit source bounds) inside a complex key of
// a merge-LOSING value: per interpretation 7, a complex key can never
// itself lose a precedence contest (distinct complex-key nodes have
// distinct pointer identities), so the DAG is placed as the complex key
// of a mapping that is itself the value of a merge candidate suppressed
// by an explicit key of the same identity. Because a losing candidate's
// value is never resolved or emitted, that mapping — and the DAG inside
// its complex key — is never inventoried, dereferenced, or expanded at
// all.
//
// It asserts the call returns a nil error and a non-nil result, that the
// result contains the winning explicit value and no node from the losing
// branch, that the whole call completes in under five seconds measured
// with runResolveEffectiveWithin (the test fails, not hangs, if the
// deadline is exceeded), and that the result's total emitted node count
// is far below maxEffectiveOutputNodes — proving the discarded temporary
// work was never charged to the output-node budget even though expanding
// it would exceed that budget.
func TestResolveEffectiveSharedAliasDAGInLosingBranchTerminates(t *testing.T) {
	const path = "/etc/chairlift/losing-branch-shared-dag.yml"

	sharedDAG := buildSourceSharingDAG(60)
	losingValue := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{sharedDAG, scalarNode("losing-complex-key-value")},
	}
	mergeOperand := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{scalarNode("shared"), losingValue},
	}
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode("shared"), scalarNode("winning-value"),
			mergeKeyNode(), mergeOperand,
		},
	}
	doc := docOf(root)

	out, err := runResolveEffectiveWithin(t, path, doc, 5*time.Second)
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	if out == nil {
		t.Fatalf("resolveEffective(...) node = nil, want a non-nil result")
	}
	outRoot := out.Content[0]
	if len(outRoot.Content) != 2 {
		t.Fatalf("len(outRoot.Content) = %d, want 2 (single winning entry): %+v", len(outRoot.Content), outRoot.Content)
	}
	if outRoot.Content[0].Value != "shared" || outRoot.Content[1].Value != "winning-value" {
		t.Fatalf("got key=%q value=%q, want shared=winning-value", outRoot.Content[0].Value, outRoot.Content[1].Value)
	}
	walkEffectiveNodes(out, func(n *yaml.Node) {
		if n.Value == "losing-complex-key-value" {
			t.Fatalf("losing branch content leaked into the result")
		}
	})
	if got := countEffectiveNodes(out); got >= maxEffectiveOutputNodes {
		t.Fatalf("countEffectiveNodes = %d, want far below maxEffectiveOutputNodes (%d)", got, maxEffectiveOutputNodes)
	}
}
