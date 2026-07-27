package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

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
