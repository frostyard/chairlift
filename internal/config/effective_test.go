package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestResolveEffectiveNilDocumentReturnsNilNil confirms nil doc is the
// valid empty-input result for resolveEffective, exactly like
// parseYAMLDocument and validateSourceGraph.
func TestResolveEffectiveNilDocumentReturnsNilNil(t *testing.T) {
	node, err := resolveEffective("cfg.yml", nil)
	if node != nil {
		t.Fatalf("resolveEffective(\"cfg.yml\", nil) node = %v, want nil", node)
	}
	if err != nil {
		t.Fatalf("resolveEffective(\"cfg.yml\", nil) err = %v, want nil", err)
	}
}

// docOf wraps content in a well-formed yaml.DocumentNode root.
func docOf(content *yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{content}}
}

// TestResolveEffectiveValidatesSourceGraphFirst is table-driven over four
// already-rejected source graphs (an alias cycle, an alias with a nil
// target, a duplicate explicit key, and a merge operand that is a scalar)
// and confirms resolveEffective returns a nil *yaml.Node and passes
// validateSourceGraph's Detail and Path through byte-identically, proving
// resolveEffective cannot be used to bypass source-graph validation.
func TestResolveEffectiveValidatesSourceGraphFirst(t *testing.T) {
	const path = "/etc/chairlift/rejected.yml"

	aliasCycle := &yaml.Node{Kind: yaml.MappingNode, Anchor: "self"}
	selfAlias := &yaml.Node{Kind: yaml.AliasNode, Alias: aliasCycle}
	aliasCycle.Content = []*yaml.Node{scalarNode("key"), selfAlias}

	aliasNilTarget := &yaml.Node{Kind: yaml.AliasNode, Alias: nil}

	duplicateKey := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{scalarNode("dup"), scalarNode("first"), scalarNode("dup"), scalarNode("second")},
	}

	mergeScalarOperand := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{mergeKeyNode(), scalarNode("not-a-mapping")},
	}

	tests := []struct {
		name string
		root *yaml.Node
	}{
		{name: "alias cycle", root: aliasCycle},
		{name: "alias with nil target", root: aliasNilTarget},
		{name: "duplicate explicit key", root: duplicateKey},
		{name: "merge operand is a scalar", root: mergeScalarOperand},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := docOf(tt.root)

			wantErr := validateSourceGraph(path, doc)
			wantParseType(t, wantErr, path)

			gotNode, gotErr := resolveEffective(path, doc)
			if gotNode != nil {
				t.Fatalf("resolveEffective(%q, ...) node = %v, want nil", path, gotNode)
			}
			if gotErr == nil {
				t.Fatalf("resolveEffective(%q, ...) err = nil, want a *LoadError", path)
			}
			if gotErr.Detail != wantErr.Detail {
				t.Fatalf("resolveEffective Detail = %q, want %q (from validateSourceGraph)", gotErr.Detail, wantErr.Detail)
			}
			if gotErr.Path != wantErr.Path {
				t.Fatalf("resolveEffective Path = %q, want %q (from validateSourceGraph)", gotErr.Path, wantErr.Path)
			}
		})
	}
}

// walkEffectiveNodes calls visit for n and every node reachable from n
// through Content, in Content slice order. It never follows an Alias
// field, since a well-formed resolveEffective result has no AliasNode to
// follow in the first place.
func walkEffectiveNodes(n *yaml.Node, visit func(*yaml.Node)) {
	if n == nil {
		return
	}
	visit(n)
	for _, child := range n.Content {
		walkEffectiveNodes(child, visit)
	}
}

// TestResolveEffectiveOutputHasNoAnchorsOrAliases builds a fixture using
// anchors and aliases in both mapping key and mapping value position and
// confirms every node in resolveEffective's output — document, mappings,
// sequences, keys, and values — has no Anchor, no Alias, and is never a
// yaml.AliasNode.
func TestResolveEffectiveOutputHasNoAnchorsOrAliases(t *testing.T) {
	anchoredKey := &yaml.Node{Kind: yaml.ScalarNode, Anchor: "k", Value: "anchored-key"}
	anchoredKeyTarget := &yaml.Node{Kind: yaml.ScalarNode, Anchor: "k2", Value: "aliased-key"}
	anchoredValue := &yaml.Node{Kind: yaml.MappingNode, Anchor: "v", Content: []*yaml.Node{scalarNode("inner"), scalarNode("value")}}

	// aliasKey targets a distinct anchored scalar (not anchoredKey) so its
	// effectiveKeyIdentity does not collide with anchoredKey's: this test
	// exercises alias-in-key-position metadata copying, not the separate
	// effective-identity collision rule (see effectiveidentity_test.go).
	aliasKey := &yaml.Node{Kind: yaml.AliasNode, Alias: anchoredKeyTarget}
	aliasValue := &yaml.Node{Kind: yaml.AliasNode, Alias: anchoredValue}

	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			anchoredKey, scalarNode("first-value"),
			scalarNode("second-key"), anchoredValue,
			aliasKey, scalarNode("via-alias-key"),
			scalarNode("third-key"), aliasValue,
			scalarNode("seq"), {Kind: yaml.SequenceNode, Content: []*yaml.Node{aliasValue, scalarNode("plain")}},
		},
	}

	doc := docOf(root)
	out, err := resolveEffective("anchors.yml", doc)
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}

	count := 0
	walkEffectiveNodes(out, func(n *yaml.Node) {
		count++
		if n.Anchor != "" {
			t.Errorf("node %+v has non-empty Anchor %q", n, n.Anchor)
		}
		if n.Alias != nil {
			t.Errorf("node %+v has non-nil Alias", n)
		}
		if n.Kind == yaml.AliasNode {
			t.Errorf("node %+v has Kind == yaml.AliasNode", n)
		}
	})
	if count == 0 {
		t.Fatalf("walkEffectiveNodes visited no nodes")
	}
}

// TestResolveEffectivePreservesNodeMetadata confirms that for a scalar, a
// sequence, and a mapping node, the emitted copy shares Kind, Style, Tag,
// Value, Line, Column, HeadComment, LineComment, and FootComment with the
// source node, and that the emitted node is a distinct pointer (mutating
// the result does not mutate the input).
func TestResolveEffectivePreservesNodeMetadata(t *testing.T) {
	assertMetadataPreserved := func(t *testing.T, source, emitted *yaml.Node) {
		t.Helper()
		if emitted == source {
			t.Fatalf("emitted node is the same pointer as the source node")
		}
		if emitted.Kind != source.Kind {
			t.Errorf("Kind = %v, want %v", emitted.Kind, source.Kind)
		}
		if emitted.Style != source.Style {
			t.Errorf("Style = %v, want %v", emitted.Style, source.Style)
		}
		if emitted.Tag != source.Tag {
			t.Errorf("Tag = %q, want %q", emitted.Tag, source.Tag)
		}
		if emitted.Value != source.Value {
			t.Errorf("Value = %q, want %q", emitted.Value, source.Value)
		}
		if emitted.Line != source.Line {
			t.Errorf("Line = %d, want %d", emitted.Line, source.Line)
		}
		if emitted.Column != source.Column {
			t.Errorf("Column = %d, want %d", emitted.Column, source.Column)
		}
		if emitted.HeadComment != source.HeadComment {
			t.Errorf("HeadComment = %q, want %q", emitted.HeadComment, source.HeadComment)
		}
		if emitted.LineComment != source.LineComment {
			t.Errorf("LineComment = %q, want %q", emitted.LineComment, source.LineComment)
		}
		if emitted.FootComment != source.FootComment {
			t.Errorf("FootComment = %q, want %q", emitted.FootComment, source.FootComment)
		}
		// Mutating the emitted node must not mutate the source.
		originalValue := source.Value
		emitted.Value = "mutated"
		if source.Value != originalValue {
			t.Fatalf("mutating emitted node mutated the source node")
		}
	}

	t.Run("scalar", func(t *testing.T) {
		source := &yaml.Node{
			Kind: yaml.ScalarNode, Style: yaml.DoubleQuotedStyle, Tag: "!!str", Value: "hello",
			Line: 3, Column: 5, HeadComment: "# head", LineComment: "# line", FootComment: "# foot",
		}
		out, err := resolveEffective("scalar.yml", docOf(source))
		if err != nil {
			t.Fatalf("resolveEffective returned unexpected error: %v", err)
		}
		assertMetadataPreserved(t, source, out.Content[0])
	})

	t.Run("sequence", func(t *testing.T) {
		source := &yaml.Node{
			Kind: yaml.SequenceNode, Style: yaml.FlowStyle, Tag: "!!seq",
			Line: 7, Column: 2, HeadComment: "# seqhead", LineComment: "# seqline", FootComment: "# seqfoot",
			Content: []*yaml.Node{scalarNode("a"), scalarNode("b")},
		}
		out, err := resolveEffective("sequence.yml", docOf(source))
		if err != nil {
			t.Fatalf("resolveEffective returned unexpected error: %v", err)
		}
		assertMetadataPreserved(t, source, out.Content[0])
	})

	t.Run("mapping", func(t *testing.T) {
		source := &yaml.Node{
			Kind: yaml.MappingNode, Style: yaml.FlowStyle, Tag: "!!map",
			Line: 11, Column: 9, HeadComment: "# maphead", LineComment: "# mapline", FootComment: "# mapfoot",
			Content: []*yaml.Node{scalarNode("k"), scalarNode("v")},
		}
		out, err := resolveEffective("mapping.yml", docOf(source))
		if err != nil {
			t.Fatalf("resolveEffective returned unexpected error: %v", err)
		}
		assertMetadataPreserved(t, source, out.Content[0])
	})
}

// TestResolveEffectiveAliasExpansionCopiesTargetMetadata confirms that an
// expanded alias's emitted node carries the dereferenced target's
// Line/Column/Tag/Style/comments, not the alias node's own (near-zero)
// metadata, both for an alias in mapping value position and for an alias
// used as a mapping key.
func TestResolveEffectiveAliasExpansionCopiesTargetMetadata(t *testing.T) {
	target := &yaml.Node{
		Kind: yaml.ScalarNode, Style: yaml.DoubleQuotedStyle, Tag: "!!str", Value: "target-value",
		Anchor: "shared", Line: 42, Column: 17,
		HeadComment: "# target head", LineComment: "# target line", FootComment: "# target foot",
	}
	// The alias node itself carries no meaningful metadata of its own -
	// deliberately different Line/Column so a bug that copies the alias
	// node's own metadata instead of the target's would be caught.
	aliasAsValue := &yaml.Node{Kind: yaml.AliasNode, Alias: target, Line: 1, Column: 1}
	aliasAsKey := &yaml.Node{Kind: yaml.AliasNode, Alias: target, Line: 2, Column: 2}

	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode("anchored"), target,
			scalarNode("via-value"), aliasAsValue,
			aliasAsKey, scalarNode("via-key"),
		},
	}

	out, err := resolveEffective("alias-metadata.yml", docOf(root))
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}

	outRoot := out.Content[0]
	assertTargetMetadata := func(t *testing.T, n *yaml.Node) {
		t.Helper()
		if n.Line != target.Line || n.Column != target.Column {
			t.Fatalf("Line/Column = %d/%d, want target's %d/%d", n.Line, n.Column, target.Line, target.Column)
		}
		if n.Tag != target.Tag || n.Style != target.Style || n.Value != target.Value {
			t.Fatalf("Tag/Style/Value = %q/%v/%q, want target's %q/%v/%q", n.Tag, n.Style, n.Value, target.Tag, target.Style, target.Value)
		}
		if n.HeadComment != target.HeadComment || n.LineComment != target.LineComment || n.FootComment != target.FootComment {
			t.Fatalf("comments = %q/%q/%q, want target's %q/%q/%q", n.HeadComment, n.LineComment, n.FootComment, target.HeadComment, target.LineComment, target.FootComment)
		}
	}

	// entry index 2 (0-based: "via-value" key at index 2, alias value at index 3)
	assertTargetMetadata(t, outRoot.Content[3])
	// entry index 4 (alias key), index 5 ("via-key" value)
	assertTargetMetadata(t, outRoot.Content[4])
}

// TestResolveEffectiveQuotedMergeKeyIsOrdinary confirms a !!str-tagged
// "<<" key and a merge-tagged scalar whose value is not "<<" both survive
// as ordinary effective keys — a contract unaffected by merge-precedence
// work landing in a later chunk.
func TestResolveEffectiveQuotedMergeKeyIsOrdinary(t *testing.T) {
	quotedMergeKey := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "<<"}
	nonMergeValueTaggedMerge := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "not-merge"}

	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			quotedMergeKey, scalarNode("quoted-value"),
			nonMergeValueTaggedMerge, scalarNode("tagged-value"),
		},
	}

	out, err := resolveEffective("quoted-merge.yml", docOf(root))
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}

	outRoot := out.Content[0]
	if len(outRoot.Content) != 4 {
		t.Fatalf("len(outRoot.Content) = %d, want 4", len(outRoot.Content))
	}
	if outRoot.Content[0].Tag != "!!str" || outRoot.Content[0].Value != "<<" {
		t.Fatalf("quoted merge key not preserved as ordinary: %+v", outRoot.Content[0])
	}
	if outRoot.Content[2].Tag != "!!merge" || outRoot.Content[2].Value != "not-merge" {
		t.Fatalf("merge-tagged non-\"<<\" key not preserved as ordinary: %+v", outRoot.Content[2])
	}
}

// TestResolveEffectiveDocumentRootShape confirms a valid document's result
// is rooted at a node with Kind == yaml.DocumentNode and exactly one entry
// in Content.
func TestResolveEffectiveDocumentRootShape(t *testing.T) {
	out, err := resolveEffective("root-shape.yml", docOf(simpleMapping("k", "v")))
	if err != nil {
		t.Fatalf("resolveEffective returned unexpected error: %v", err)
	}
	if out.Kind != yaml.DocumentNode {
		t.Fatalf("out.Kind = %v, want yaml.DocumentNode", out.Kind)
	}
	if len(out.Content) != 1 {
		t.Fatalf("len(out.Content) = %d, want 1", len(out.Content))
	}
}
