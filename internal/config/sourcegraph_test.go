package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// scalarNode builds a well-formed yaml.ScalarNode with the given value, for
// use as filler content (mapping keys/values, sequence entries) in
// synthetic source-graph tests.
func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
}

// wantParseType fails the test unless err is a non-nil *LoadError with
// Kind == KindParseType and Path == path.
func wantParseType(t *testing.T, err *LoadError, path string) {
	t.Helper()
	if err == nil {
		t.Fatalf("validateSourceGraph(%q, ...) = nil, want a KindParseType error", path)
	}
	if err.Kind != KindParseType {
		t.Fatalf("err.Kind = %q, want %q", err.Kind, KindParseType)
	}
	if err.Path != path {
		t.Fatalf("err.Path = %q, want %q", err.Path, path)
	}
}

// TestValidateSourceGraphNilDoc confirms nil doc succeeds: it is
// parseYAMLDocument's valid empty-input result.
func TestValidateSourceGraphNilDoc(t *testing.T) {
	if err := validateSourceGraph("p", nil); err != nil {
		t.Fatalf("validateSourceGraph(\"p\", nil) = %v, want nil", err)
	}
}

// TestValidateSourceGraphRootShape covers the four ways a non-nil root can
// fail the top-level document shape check, each as its own case, each
// asserting KindParseType with Path == the path argument.
func TestValidateSourceGraphRootShape(t *testing.T) {
	const path = "/etc/chairlift/root-shape.yml"

	tests := []struct {
		name string
		root *yaml.Node
	}{
		{
			name: "root is not a document node",
			root: &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("k"), scalarNode("v")}},
		},
		{
			name: "document node with zero children",
			root: &yaml.Node{Kind: yaml.DocumentNode, Content: nil},
		},
		{
			name: "document node with two children",
			root: &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{scalarNode("a"), scalarNode("b")}},
		},
		{
			name: "document node with a nil child",
			root: &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{nil}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantParseType(t, validateSourceGraph(path, tt.root), path)
		})
	}
}

// TestValidateSourceGraphValidNestedDocument builds a document mixing
// mappings, sequences, scalars, and an alias to an earlier anchor, and
// confirms it validates cleanly.
func TestValidateSourceGraphValidNestedDocument(t *testing.T) {
	anchored := &yaml.Node{
		Kind:    yaml.MappingNode,
		Anchor:  "shared",
		Content: []*yaml.Node{scalarNode("name"), scalarNode("shared-value")},
	}
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: anchored}
	seq := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Content: []*yaml.Node{scalarNode("item1"), alias, scalarNode("item2")},
	}
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode("shared"), anchored,
			scalarNode("list"), seq,
			scalarNode("scalar"), scalarNode("value"),
		},
	}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}

	if err := validateSourceGraph("p", doc); err != nil {
		t.Fatalf("validateSourceGraph(valid nested document) = %v, want nil", err)
	}
}

// TestValidateSourceGraphStructuralRejections covers every synthetic
// malformed-shape case as its own subtest: none of these graphs are
// reachable from real YAML text, so a panic here fails the test just as
// surely as a wrong error would.
func TestValidateSourceGraphStructuralRejections(t *testing.T) {
	const path = "/etc/chairlift/structural.yml"

	oddMapping := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{scalarNode("onlykey")},
	}
	nilKeyMapping := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{nil, scalarNode("v")},
	}
	nilValueMapping := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{scalarNode("k"), nil},
	}
	nilEntrySequence := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Content: []*yaml.Node{scalarNode("a"), nil},
	}
	scalarWithContent := &yaml.Node{
		Kind:    yaml.ScalarNode,
		Value:   "x",
		Content: []*yaml.Node{scalarNode("unexpected")},
	}
	aliasWithContent := &yaml.Node{
		Kind:    yaml.AliasNode,
		Alias:   scalarNode("target"),
		Content: []*yaml.Node{scalarNode("unexpected")},
	}
	aliasWithNilTarget := &yaml.Node{
		Kind:  yaml.AliasNode,
		Alias: nil,
	}
	zeroKindNode := &yaml.Node{Kind: yaml.Kind(0)}
	unsupportedKindNode := &yaml.Node{Kind: yaml.Kind(99)}

	tests := []struct {
		name string
		root *yaml.Node
	}{
		{name: "mapping with odd content length", root: oddMapping},
		{name: "mapping with a nil key", root: nilKeyMapping},
		{name: "mapping with a nil value", root: nilValueMapping},
		{name: "sequence with a nil entry", root: nilEntrySequence},
		{name: "scalar node with non-empty content", root: scalarWithContent},
		{name: "alias node with non-empty content", root: aliasWithContent},
		{name: "alias node with nil Alias", root: aliasWithNilTarget},
		{name: "node with Kind 0", root: zeroKindNode},
		{name: "node with an unsupported Kind value", root: unsupportedKindNode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{tt.root}}
			wantParseType(t, validateSourceGraph(path, doc), path)
		})
	}
}

// TestValidateSourceGraphAliasCycles builds a self alias cycle (a mapping
// value aliasing its own enclosing anchored mapping) and a mutual alias
// cycle (two anchored mappings aliasing each other), and confirms each is
// rejected as KindParseType, proving the traversal terminates rather than
// hanging or overflowing the stack.
func TestValidateSourceGraphAliasCycles(t *testing.T) {
	const path = "/etc/chairlift/cycles.yml"

	t.Run("self alias cycle", func(t *testing.T) {
		self := &yaml.Node{Kind: yaml.MappingNode, Anchor: "self"}
		selfAlias := &yaml.Node{Kind: yaml.AliasNode, Alias: self}
		self.Content = []*yaml.Node{scalarNode("key"), selfAlias}

		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{self}}
		wantParseType(t, validateSourceGraph(path, doc), path)
	})

	t.Run("mutual alias cycle", func(t *testing.T) {
		m1 := &yaml.Node{Kind: yaml.MappingNode, Anchor: "m1"}
		m2 := &yaml.Node{Kind: yaml.MappingNode, Anchor: "m2"}
		aliasToM2 := &yaml.Node{Kind: yaml.AliasNode, Alias: m2}
		aliasToM1 := &yaml.Node{Kind: yaml.AliasNode, Alias: m1}
		m1.Content = []*yaml.Node{scalarNode("key1"), aliasToM2}
		m2.Content = []*yaml.Node{scalarNode("key2"), aliasToM1}

		root := &yaml.Node{
			Kind:    yaml.SequenceNode,
			Content: []*yaml.Node{m1, m2},
		}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		wantParseType(t, validateSourceGraph(path, doc), path)
	})
}

// TestValidateSourceGraphNodeIdentityNotValueEquality proves node identity,
// not value equality, drives the seen-state map: the same *yaml.Node
// pointer is a child of several parents (a shared scalar reused as several
// mapping values, and a shared mapping reused as several sequence
// entries), which must be accepted without falsely reporting a cycle.
func TestValidateSourceGraphNodeIdentityNotValueEquality(t *testing.T) {
	sharedScalar := scalarNode("shared")
	sharedMapping := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{scalarNode("k"), scalarNode("v")},
	}

	scalarParentA := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{scalarNode("a"), sharedScalar},
	}
	scalarParentB := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{scalarNode("b"), sharedScalar},
	}
	mappingParentA := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Content: []*yaml.Node{sharedMapping},
	}
	mappingParentB := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Content: []*yaml.Node{sharedMapping},
	}
	root := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Content: []*yaml.Node{scalarParentA, scalarParentB, mappingParentA, mappingParentB},
	}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}

	if err := validateSourceGraph("p", doc); err != nil {
		t.Fatalf("validateSourceGraph(shared node graph) = %v, want nil", err)
	}
}

// TestValidateSourceGraphNilErrRenders confirms a validator-detected graph
// error may have Err == nil while Error() still renders the parse/type
// wording, since these are pure shape rejections with no underlying Go
// error to preserve.
func TestValidateSourceGraphNilErrRenders(t *testing.T) {
	const path = "/etc/chairlift/nil-err.yml"

	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: nil}
	err := validateSourceGraph(path, doc)
	wantParseType(t, err, path)

	if err.Err != nil {
		t.Fatalf("err.Err = %v, want nil", err.Err)
	}
	if got, want := err.Error(), "config parse/type error: "+path; got == "" || got[:len(want)] != want {
		t.Fatalf("err.Error() = %q, want it to start with %q", got, want)
	}
}

// mergeKeyNode builds a well-formed implicit-tag "<<" merge key scalar
// node, for use as a mapping key in merge-operand source-graph tests.
func mergeKeyNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: "<<"}
}

// simpleMapping builds a one-entry well-formed yaml.MappingNode, used as a
// merge operand's mapping-shaped filler content.
func simpleMapping(key, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode(key), scalarNode(value)}}
}

// TestValidateSourceGraphMergeOperandAccepted covers every merge-operand
// shape yaml.v3 v3.0.1 accepts: a mapping literal; an alias to a mapping;
// a sequence of aliases to mappings; and a sequence mixing an inline
// mapping with an alias to a mapping.
func TestValidateSourceGraphMergeOperandAccepted(t *testing.T) {
	anchoredA := &yaml.Node{Kind: yaml.MappingNode, Anchor: "a", Content: []*yaml.Node{scalarNode("ka"), scalarNode("va")}}
	anchoredB := &yaml.Node{Kind: yaml.MappingNode, Anchor: "b", Content: []*yaml.Node{scalarNode("kb"), scalarNode("vb")}}

	tests := []struct {
		name  string
		value *yaml.Node
	}{
		{
			name:  "mapping literal",
			value: simpleMapping("k", "v"),
		},
		{
			name:  "alias to a mapping",
			value: &yaml.Node{Kind: yaml.AliasNode, Alias: anchoredA},
		},
		{
			name: "sequence of two aliases both to mappings",
			value: &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
				{Kind: yaml.AliasNode, Alias: anchoredA},
				{Kind: yaml.AliasNode, Alias: anchoredB},
			}},
		},
		{
			name: "sequence mixing an inline mapping and an alias to a mapping",
			value: &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
				simpleMapping("k", "v"),
				{Kind: yaml.AliasNode, Alias: anchoredA},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &yaml.Node{
				Kind:    yaml.MappingNode,
				Content: []*yaml.Node{mergeKeyNode(), tt.value, scalarNode("anchors"), {Kind: yaml.SequenceNode, Content: []*yaml.Node{anchoredA, anchoredB}}},
			}
			doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
			if err := validateSourceGraph("p", doc); err != nil {
				t.Fatalf("validateSourceGraph(merge operand %s) = %v, want nil", tt.name, err)
			}
		})
	}
}

// TestValidateSourceGraphMergeOperandRejected covers every merge-operand
// shape yaml.v3 v3.0.1 rejects, each its own case returning KindParseType
// with Path copied: a scalar operand; an alias to a sequence; an alias to
// a scalar; an alias whose immediate target is itself an AliasNode that
// ultimately resolves to a mapping (the immediate-target-only rule); a
// sequence containing a scalar; a sequence containing a sequence; and a
// sequence containing an alias to a non-mapping.
func TestValidateSourceGraphMergeOperandRejected(t *testing.T) {
	const path = "/etc/chairlift/merge-rejected.yml"

	anchoredMapping := &yaml.Node{Kind: yaml.MappingNode, Anchor: "m", Content: []*yaml.Node{scalarNode("k"), scalarNode("v")}}
	anchoredSeq := &yaml.Node{Kind: yaml.SequenceNode, Anchor: "s", Content: []*yaml.Node{scalarNode("item")}}
	anchoredScalar := &yaml.Node{Kind: yaml.ScalarNode, Anchor: "sc", Value: "scalar-anchor"}
	// aliasToMapping is itself anchored, so a second alias can target it
	// (an alias-to-alias) rather than the mapping directly.
	aliasToMapping := &yaml.Node{Kind: yaml.AliasNode, Anchor: "alias-to-mapping", Alias: anchoredMapping}

	tests := []struct {
		name  string
		value *yaml.Node
	}{
		{
			name:  "scalar operand",
			value: scalarNode("5"),
		},
		{
			name:  "alias to a sequence",
			value: &yaml.Node{Kind: yaml.AliasNode, Alias: anchoredSeq},
		},
		{
			name:  "alias to a scalar",
			value: &yaml.Node{Kind: yaml.AliasNode, Alias: anchoredScalar},
		},
		{
			name:  "alias whose immediate target is itself an alias that ultimately resolves to a mapping",
			value: &yaml.Node{Kind: yaml.AliasNode, Alias: aliasToMapping},
		},
		{
			name: "sequence containing a scalar",
			value: &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
				simpleMapping("k", "v"), scalarNode("not a mapping"),
			}},
		},
		{
			name: "sequence containing a sequence",
			value: &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
				simpleMapping("k", "v"),
				{Kind: yaml.SequenceNode, Content: []*yaml.Node{scalarNode("nested")}},
			}},
		},
		{
			name: "sequence containing an alias to a non-mapping",
			value: &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
				simpleMapping("k", "v"),
				{Kind: yaml.AliasNode, Alias: anchoredScalar},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &yaml.Node{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					mergeKeyNode(), tt.value,
					scalarNode("anchors"), {Kind: yaml.SequenceNode, Content: []*yaml.Node{
						anchoredMapping, anchoredSeq, anchoredScalar, aliasToMapping,
					}},
				},
			}
			doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
			wantParseType(t, validateSourceGraph(path, doc), path)
		})
	}
}

// TestValidateSourceGraphMergeAliasChainAsymmetry pins the immediate-target
// rule's asymmetry: an alias-to-alias-to-mapping chain is well-formed
// ordinary YAML content when it is an ordinary mapping value (returns nil),
// but is a malformed merge operand when the same chain is used as a "<<"
// value (returns KindParseType), because yaml.v3 v3.0.1 only unwraps one
// alias hop for a merge operand.
func TestValidateSourceGraphMergeAliasChainAsymmetry(t *testing.T) {
	buildChain := func() (mapping, innerAlias, outerAlias *yaml.Node) {
		mapping = &yaml.Node{Kind: yaml.MappingNode, Anchor: "target", Content: []*yaml.Node{scalarNode("k"), scalarNode("v")}}
		innerAlias = &yaml.Node{Kind: yaml.AliasNode, Anchor: "inner", Alias: mapping}
		outerAlias = &yaml.Node{Kind: yaml.AliasNode, Alias: innerAlias}
		return
	}

	t.Run("as an ordinary mapping value", func(t *testing.T) {
		mapping, innerAlias, outerAlias := buildChain()
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				scalarNode("target"), mapping,
				scalarNode("inner"), innerAlias,
				scalarNode("ordinary"), outerAlias,
			},
		}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		if err := validateSourceGraph("p", doc); err != nil {
			t.Fatalf("validateSourceGraph(alias chain as ordinary value) = %v, want nil", err)
		}
	})

	t.Run("as a merge operand", func(t *testing.T) {
		const path = "/etc/chairlift/merge-chain.yml"
		mapping, innerAlias, outerAlias := buildChain()
		root := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				scalarNode("target"), mapping,
				scalarNode("inner"), innerAlias,
				mergeKeyNode(), outerAlias,
			},
		}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		wantParseType(t, validateSourceGraph(path, doc), path)
	})
}

// TestValidateSourceGraphMergeCycles confirms &a {<<: *a} (a self merge)
// and a mutual merge cycle between two anchored mappings each return
// KindParseType and the test terminates rather than hanging: c3's generic
// visiting-state alias-cycle rule catches the merge operand's alias before
// any merge-specific code inspects its shape.
func TestValidateSourceGraphMergeCycles(t *testing.T) {
	const path = "/etc/chairlift/merge-cycles.yml"

	t.Run("self merge", func(t *testing.T) {
		self := &yaml.Node{Kind: yaml.MappingNode, Anchor: "a"}
		selfAlias := &yaml.Node{Kind: yaml.AliasNode, Alias: self}
		self.Content = []*yaml.Node{mergeKeyNode(), selfAlias}

		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{self}}
		wantParseType(t, validateSourceGraph(path, doc), path)
	})

	t.Run("mutual merge cycle", func(t *testing.T) {
		m1 := &yaml.Node{Kind: yaml.MappingNode, Anchor: "m1"}
		m2 := &yaml.Node{Kind: yaml.MappingNode, Anchor: "m2"}
		aliasToM2 := &yaml.Node{Kind: yaml.AliasNode, Alias: m2}
		aliasToM1 := &yaml.Node{Kind: yaml.AliasNode, Alias: m1}
		m1.Content = []*yaml.Node{mergeKeyNode(), aliasToM2}
		m2.Content = []*yaml.Node{mergeKeyNode(), aliasToM1}

		root := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{m1, m2}}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		wantParseType(t, validateSourceGraph(path, doc), path)
	})
}

// TestValidateSourceGraphMergeKeyTagForms confirms all four merge-key tag
// forms from c2 (implicit, "!", "!!merge", the canonical long tag) trigger
// merge-operand validation — each rejecting an invalid scalar operand as
// KindParseType — and that a quoted "<<" key (tagged "!!str") does not
// trigger it, so a quoted "<<" key with a scalar value validates cleanly.
func TestValidateSourceGraphMergeKeyTagForms(t *testing.T) {
	const path = "/etc/chairlift/merge-tags.yml"

	tagCases := []struct {
		name string
		tag  string
	}{
		{name: "implicit tag", tag: ""},
		{name: "bare non-specific tag", tag: "!"},
		{name: "short merge tag", tag: "!!merge"},
		{name: "canonical long merge tag", tag: "tag:yaml.org,2002:merge"},
	}

	for _, tt := range tagCases {
		t.Run(tt.name, func(t *testing.T) {
			key := &yaml.Node{Kind: yaml.ScalarNode, Value: "<<", Tag: tt.tag}
			root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{key, scalarNode("not-a-mapping")}}
			doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
			wantParseType(t, validateSourceGraph(path, doc), path)
		})
	}

	t.Run("quoted << key does not trigger merge validation", func(t *testing.T) {
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: "<<", Tag: "!!str"}
		root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{key, scalarNode("just a string value")}}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		if err := validateSourceGraph("p", doc); err != nil {
			t.Fatalf("validateSourceGraph(quoted << key with scalar value) = %v, want nil", err)
		}
	})
}

// TestValidateSourceGraphMergeInComplexKey confirms a malformed merge
// operand nested inside a complex mapping key's own subtree returns
// KindParseType, and that an otherwise well-formed complex mapping key
// validates cleanly: complex keys are traversed but not classified in this
// slice.
func TestValidateSourceGraphMergeInComplexKey(t *testing.T) {
	t.Run("malformed merge operand inside a complex key", func(t *testing.T) {
		const path = "/etc/chairlift/complex-key-bad-merge.yml"
		complexKey := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: []*yaml.Node{mergeKeyNode(), scalarNode("not-a-mapping")},
		}
		root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{complexKey, scalarNode("value")}}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		wantParseType(t, validateSourceGraph(path, doc), path)
	})

	t.Run("well-formed complex key", func(t *testing.T) {
		complexKey := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: []*yaml.Node{scalarNode("nested"), scalarNode("key-value")},
		}
		root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{complexKey, scalarNode("value")}}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		if err := validateSourceGraph("p", doc); err != nil {
			t.Fatalf("validateSourceGraph(well-formed complex key) = %v, want nil", err)
		}
	})
}

// TestValidateSourceGraphMergeDiscardedBranchStillValidated confirms a
// malformed merge operand in a mapping entry that a later merge-precedence
// pass would discard (an earlier "<<" source overridden by a later one)
// still returns KindParseType: this slice validates every reachable entry,
// not just the one an effective-merge pass would keep
// (docs/agents/skills/discarded-merge-branches-still-need-validation.md).
func TestValidateSourceGraphMergeDiscardedBranchStillValidated(t *testing.T) {
	const path = "/etc/chairlift/merge-discarded.yml"

	// The first "<<" entry (which real merge precedence would discard in
	// favor of the second, later "<<" entry) has a malformed scalar
	// operand; the second is a well-formed mapping. Duplicate merge-key
	// detection is not part of this slice (c5), so this graph reaches the
	// malformed operand before any duplicate-key check could short-circuit
	// it.
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			mergeKeyNode(), scalarNode("malformed-and-discarded"),
			mergeKeyNode(), simpleMapping("winner-key", "winner-value"),
		},
	}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	wantParseType(t, validateSourceGraph(path, doc), path)
}

// TestMergeKeyRecognition exercises isMergeKey directly against
// gopkg.in/yaml.v3 v3.0.1 decode.go:isMerge's exact predicate: a
// yaml.ScalarNode with Value "<<" and a Tag that is either absent, the bare
// "!" non-specific tag, or resolves to the short merge tag "!!merge"
// (whether already short or spelled out in canonical long form).
func TestMergeKeyRecognition(t *testing.T) {
	tests := []struct {
		name string
		node *yaml.Node
		want bool
	}{
		{
			name: "implicit tag",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: "<<", Tag: ""},
			want: true,
		},
		{
			name: "bare non-specific tag",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: "<<", Tag: "!"},
			want: true,
		},
		{
			name: "short merge tag",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: "<<", Tag: "!!merge"},
			want: true,
		},
		{
			name: "canonical long merge tag",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: "<<", Tag: "tag:yaml.org,2002:merge"},
			want: true,
		},
		{
			name: "quoted string tag is not a merge key",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: "<<", Tag: "!!str"},
			want: false,
		},
		{
			name: "merge tag with different value is not a merge key",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: "x", Tag: "!!merge"},
			want: false,
		},
		{
			name: "mapping node is not a merge key",
			node: &yaml.Node{Kind: yaml.MappingNode, Value: "<<"},
			want: false,
		},
		{
			name: "sequence node is not a merge key",
			node: &yaml.Node{Kind: yaml.SequenceNode, Value: "<<"},
			want: false,
		},
		{
			name: "alias node is not a merge key",
			node: &yaml.Node{Kind: yaml.AliasNode, Value: "<<"},
			want: false,
		},
		{
			name: "nil node does not panic and is not a merge key",
			node: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMergeKey(tt.node); got != tt.want {
				t.Fatalf("isMergeKey(%+v) = %v, want %v", tt.node, got, tt.want)
			}
		})
	}
}

// TestShortYAMLTagNormalization exercises shortYAMLTag directly against
// gopkg.in/yaml.v3 v3.0.1 resolve.go:shortTag's rewrite of the canonical
// "tag:yaml.org,2002:" prefix to yaml.v3's short "!!" form, and confirms
// tags that don't carry that prefix — including a long tag under a
// different authority — pass through unchanged.
func TestShortYAMLTagNormalization(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{name: "canonical merge tag", tag: "tag:yaml.org,2002:merge", want: "!!merge"},
		{name: "already-short merge tag", tag: "!!merge", want: "!!merge"},
		{name: "empty tag", tag: "", want: ""},
		{name: "bare non-specific tag", tag: "!", want: "!"},
		{name: "custom tag", tag: "!custom", want: "!custom"},
		{name: "canonical str tag", tag: "tag:yaml.org,2002:str", want: "!!str"},
		{name: "long tag under a different authority", tag: "tag:example.com,2020:merge", want: "tag:example.com,2020:merge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortYAMLTag(tt.tag); got != tt.want {
				t.Fatalf("shortYAMLTag(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

// duplicateKeyLinePattern extracts the "line N" phrase checkDuplicateMappingKeys
// embeds in a duplicate-key error's Detail when the parser recorded a
// positive line for the repeated key.
var duplicateKeyLinePattern = regexp.MustCompile(`line (\d+)`)

// mergeKeyNodeWithTag builds a "<<" scalar merge-key node carrying an
// explicit tag, for combining differently-tagged merge keys in
// duplicate-key tests: sourceKeyID compares only Kind and Value, so two
// "<<" keys collide as duplicates regardless of which of isMergeKey's four
// accepted tag forms each one carries.
func mergeKeyNodeWithTag(tag string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: "<<", Tag: tag}
}

// TestValidateSourceGraphDuplicateExplicitKey confirms a real, parsed
// mapping with two "a:" keys is rejected as KindParseType, with Path
// copied and a Detail that names the duplicated key's value and contains a
// positive "line N" — using parseYAMLDocument (not a synthetic node) so
// the key's Line field is the parser's own, genuinely positive line.
func TestValidateSourceGraphDuplicateExplicitKey(t *testing.T) {
	const path = "/etc/chairlift/dup.yml"
	doc, perr := parseYAMLDocument(path, []byte("a: 1\nb: 2\na: 3\n"))
	if perr != nil {
		t.Fatalf("parseYAMLDocument(%q) = %v, want nil", path, perr)
	}

	err := validateSourceGraph(path, doc)
	wantParseType(t, err, path)

	if !strings.Contains(err.Detail, `"a"`) {
		t.Fatalf("err.Detail = %q, want it to name the duplicated key %q", err.Detail, "a")
	}
	m := duplicateKeyLinePattern.FindStringSubmatch(err.Detail)
	if m == nil {
		t.Fatalf("err.Detail = %q, want it to contain a %q phrase", err.Detail, "line N")
	}
	line, convErr := strconv.Atoi(m[1])
	if convErr != nil || line <= 0 {
		t.Fatalf("err.Detail = %q parsed line %q, want a positive line number", err.Detail, m[1])
	}
}

// TestValidateSourceGraphDuplicateMergeKeys confirms two "<<" mapping
// entries are rejected as duplicates in each of the three tag combinations
// the spec calls out: implicit + implicit, implicit + "!!merge", and
// implicit + the canonical long merge tag. sourceKeyID identity never
// looks at Tag, so all three combinations collide identically even though
// isMergeKey treats all four tag forms as equally valid merge keys.
func TestValidateSourceGraphDuplicateMergeKeys(t *testing.T) {
	tagCombos := []struct {
		name string
		tagA string
		tagB string
	}{
		{name: "implicit + implicit", tagA: "", tagB: ""},
		{name: "implicit + !!merge", tagA: "", tagB: "!!merge"},
		{name: "implicit + tag:yaml.org,2002:merge", tagA: "", tagB: "tag:yaml.org,2002:merge"},
	}

	for _, tt := range tagCombos {
		t.Run(tt.name, func(t *testing.T) {
			const path = "/etc/chairlift/dup-merge.yml"
			root := &yaml.Node{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					mergeKeyNodeWithTag(tt.tagA), simpleMapping("ka", "va"),
					mergeKeyNodeWithTag(tt.tagB), simpleMapping("kb", "vb"),
				},
			}
			doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
			wantParseType(t, validateSourceGraph(path, doc), path)
		})
	}
}

// TestValidateSourceGraphDuplicateBareAndQuotedIntKey confirms a bare `1`
// and a quoted "1" are rejected as duplicates even though yaml.v3 resolves
// them to different Tags (!!int vs !!str): sourceKeyID identity compares
// only Kind and Value, so the differing Tag doesn't save them. Real,
// parsed YAML (not synthetic nodes) proves the Tags genuinely differ, the
// same way the yaml.v3 decoder itself would resolve them.
func TestValidateSourceGraphDuplicateBareAndQuotedIntKey(t *testing.T) {
	const path = "/etc/chairlift/dup-int-str.yml"
	doc, perr := parseYAMLDocument(path, []byte("1: a\n\"1\": b\n"))
	if perr != nil {
		t.Fatalf("parseYAMLDocument(%q) = %v, want nil", path, perr)
	}
	wantParseType(t, validateSourceGraph(path, doc), path)
}

// TestValidateSourceGraphDuplicateKeysArePerMappingNotGlobal confirms a
// key value shared by two different mappings — a sibling and, separately,
// a parent/nested pair — is not a duplicate: the seen-key set is scoped to
// a single mapping's own Content, not shared across the whole graph.
func TestValidateSourceGraphDuplicateKeysArePerMappingNotGlobal(t *testing.T) {
	t.Run("sibling mappings sharing a key", func(t *testing.T) {
		siblingA := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("1"), scalarNode("va")}}
		siblingB := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("1"), scalarNode("vb")}}
		root := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: []*yaml.Node{scalarNode("s1"), siblingA, scalarNode("s2"), siblingB},
		}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		if err := validateSourceGraph("p", doc); err != nil {
			t.Fatalf("sibling mappings sharing key %q = %v, want nil", "1", err)
		}
	})

	t.Run("nested mapping sharing a key with its parent", func(t *testing.T) {
		nested := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("1"), scalarNode("nested-value")}}
		root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("1"), nested}}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		if err := validateSourceGraph("p", doc); err != nil {
			t.Fatalf("nested mapping sharing key %q with parent = %v, want nil", "1", err)
		}
	})
}

// TestValidateSourceGraphDuplicateKeysRequireSameKind confirms a scalar
// key and an alias-node key whose Value happens to equal the scalar's
// Value are not duplicates: sourceKeyID also requires equal Kind, and a
// yaml.AliasNode's own Value is its anchor name, not its target's value.
func TestValidateSourceGraphDuplicateKeysRequireSameKind(t *testing.T) {
	target := scalarNode("target-value")
	aliasKey := &yaml.Node{Kind: yaml.AliasNode, Value: "x", Alias: target}
	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode("x"), scalarNode("v1"),
			aliasKey, scalarNode("v2"),
		},
	}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	if err := validateSourceGraph("p", doc); err != nil {
		t.Fatalf("scalar key %q and alias key with equal Value = %v, want nil", "x", err)
	}
}

// TestValidateSourceGraphDuplicateKeysReachableViaMergeOperand confirms a
// duplicate pair inside a mapping reachable only as a "<<" merge operand's
// value is rejected.
func TestValidateSourceGraphDuplicateKeysReachableViaMergeOperand(t *testing.T) {
	const path = "/etc/chairlift/dup-via-merge-operand.yml"
	operand := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{scalarNode("dup"), scalarNode("v1"), scalarNode("dup"), scalarNode("v2")},
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{mergeKeyNode(), operand}}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	wantParseType(t, validateSourceGraph(path, doc), path)
}

// TestValidateSourceGraphDuplicateKeysReachableViaAliasTarget confirms a
// duplicate pair inside a mapping reachable only by following an alias
// node's target (not through any merge key) is rejected.
func TestValidateSourceGraphDuplicateKeysReachableViaAliasTarget(t *testing.T) {
	const path = "/etc/chairlift/dup-via-alias.yml"
	dupMapping := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{scalarNode("dup"), scalarNode("v1"), scalarNode("dup"), scalarNode("v2")},
	}
	alias := &yaml.Node{Kind: yaml.AliasNode, Value: "a", Alias: dupMapping}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("k"), alias}}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	wantParseType(t, validateSourceGraph(path, doc), path)
}

// TestValidateSourceGraphDuplicateKeysReachableInComplexKeySubtree confirms
// a duplicate pair inside a complex (mapping-shaped) key's own subtree is
// rejected, even though the complex key itself is just an ordinary,
// unclassified mapping-node key of its parent.
func TestValidateSourceGraphDuplicateKeysReachableInComplexKeySubtree(t *testing.T) {
	const path = "/etc/chairlift/dup-in-complex-key.yml"
	complexKey := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{scalarNode("dup"), scalarNode("v1"), scalarNode("dup"), scalarNode("v2")},
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{complexKey, scalarNode("value")}}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	wantParseType(t, validateSourceGraph(path, doc), path)
}

// TestValidateSourceGraphDuplicateKeysSharedMappingCheckedOnce confirms a
// mapping node reachable through two different parent entries is checked
// exactly once: a genuine duplicate inside it still yields a single error,
// and a well-formed shared mapping referenced twice yields nil rather than
// a false duplicate manufactured by revisiting it.
func TestValidateSourceGraphDuplicateKeysSharedMappingCheckedOnce(t *testing.T) {
	t.Run("genuine duplicate in a mapping shared by two parents", func(t *testing.T) {
		const path = "/etc/chairlift/dup-shared.yml"
		shared := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: []*yaml.Node{scalarNode("dup"), scalarNode("v1"), scalarNode("dup"), scalarNode("v2")},
		}
		root := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: []*yaml.Node{scalarNode("first"), shared, scalarNode("second"), shared},
		}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		wantParseType(t, validateSourceGraph(path, doc), path)
	})

	t.Run("well-formed mapping shared by two parents", func(t *testing.T) {
		shared := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalarNode("a"), scalarNode("va")}}
		root := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: []*yaml.Node{scalarNode("first"), shared, scalarNode("second"), shared},
		}
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
		if err := validateSourceGraph("p", doc); err != nil {
			t.Fatalf("well-formed mapping shared by two parents = %v, want nil", err)
		}
	})
}

// buildDistinctKeyMapping builds a yaml.MappingNode with n distinct scalar
// keys "k0".."k(n-1)", each mapped to a fixed scalar value, for the timed
// duplicate-key-detection regression tests below.
func buildDistinctKeyMapping(n int) *yaml.Node {
	content := make([]*yaml.Node, 0, n*2)
	for i := 0; i < n; i++ {
		content = append(content, scalarNode(fmt.Sprintf("k%d", i)), scalarNode("v"))
	}
	return &yaml.Node{Kind: yaml.MappingNode, Content: content}
}

// TestValidateSourceGraphDuplicateKeyDetectionIsLinear is the discriminating
// regression the O(k) map-based design (interpretation 12) needs and a
// pairwise O(k^2) implementation cannot pass: a 100,000-distinct-key mapping
// and a 100,000-key mapping whose last two keys collide must each resolve
// within a five-second budget. A pairwise implementation needs on the order
// of 5*10^9 comparisons for either case and would not return within budget.
// Each subtest runs validateSourceGraph in a goroutine and races it against
// an explicit time.Timer via select, so a hang fails the test rather than
// blocking the suite.
func TestValidateSourceGraphDuplicateKeyDetectionIsLinear(t *testing.T) {
	const numKeys = 100000
	const budget = 5 * time.Second

	t.Run("100000 distinct keys returns nil", func(t *testing.T) {
		root := buildDistinctKeyMapping(numKeys)
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}

		result := make(chan *LoadError, 1)
		go func() { result <- validateSourceGraph("p", doc) }()

		timer := time.NewTimer(budget)
		defer timer.Stop()
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("validateSourceGraph(100000 distinct keys) = %v, want nil", err)
			}
		case <-timer.C:
			t.Fatalf("validateSourceGraph did not return within %s for 100000 distinct keys", budget)
		}
	})

	t.Run("100000 keys with the last two identical returns KindParseType", func(t *testing.T) {
		const path = "p"
		root := buildDistinctKeyMapping(numKeys)
		lastKeyIdx := len(root.Content) - 2
		prevKeyIdx := lastKeyIdx - 2
		root.Content[lastKeyIdx].Value = root.Content[prevKeyIdx].Value
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}

		result := make(chan *LoadError, 1)
		go func() { result <- validateSourceGraph(path, doc) }()

		timer := time.NewTimer(budget)
		defer timer.Stop()
		select {
		case err := <-result:
			wantParseType(t, err, path)
		case <-timer.C:
			t.Fatalf("validateSourceGraph did not return within %s for a 100000-key mapping with a trailing duplicate", budget)
		}
	})
}
