package config

import (
	"testing"

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

	root := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode("a"), sharedScalar,
			scalarNode("b"), sharedScalar,
			scalarNode("seq"), &yaml.Node{
				Kind:    yaml.SequenceNode,
				Content: []*yaml.Node{sharedMapping, sharedMapping, sharedMapping},
			},
		},
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
