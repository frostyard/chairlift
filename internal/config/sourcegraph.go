package config

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// longYAMLTagPrefix is the "tag:yaml.org,2002:" canonical-tag prefix that
// gopkg.in/yaml.v3 v3.0.1 resolve.go defines as longTagPrefix and that
// shortYAMLTag strips when normalizing a tag to its short "!!" form.
const longYAMLTagPrefix = "tag:yaml.org,2002:"

// shortYAMLTag reproduces gopkg.in/yaml.v3 v3.0.1 resolve.go:shortTag's
// rewrite of a canonical "tag:yaml.org,2002:xxx" tag to yaml.v3's short
// "!!xxx" form. A tag that does not carry the long prefix — including the
// empty tag, the bare "!" non-specific tag, an already-short "!!xxx" tag, a
// custom "!xxx" tag, or a long tag under a different authority
// (e.g. "tag:example.com,2020:merge") — is returned unchanged, exactly as
// shortTag leaves it. yaml.v3's shortTag additionally special-cases a small
// set of well-known tags (null, bool, str, int, float, timestamp, seq, map,
// binary, merge) through a lookup table, but that table always yields the
// same "!!" + suffix result the plain prefix-strip produces for those tags,
// so this reproduction needs no such table to match its behavior exactly.
func shortYAMLTag(tag string) string {
	if strings.HasPrefix(tag, longYAMLTagPrefix) {
		return "!!" + tag[len(longYAMLTagPrefix):]
	}
	return tag
}

// isMergeKey reproduces gopkg.in/yaml.v3 v3.0.1 decode.go:isMerge's
// predicate for recognizing a YAML merge key ("<<"): n must be a
// yaml.ScalarNode whose Value is exactly "<<", and whose Tag is either
// absent ("", the implicit/unresolved tag), the bare non-specific tag
// ("!"), or resolves via shortYAMLTag to the short merge tag ("!!merge") —
// which accepts both that short form directly and its canonical long form
// ("tag:yaml.org,2002:merge"). A quoted "<<" (explicitly tagged "!!str") is
// therefore an ordinary key, not a merge key, and so is any node whose
// Value isn't literally "<<" regardless of its tag. n may be nil; isMergeKey
// reports false rather than panicking.
func isMergeKey(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	return n.Kind == yaml.ScalarNode && n.Value == "<<" &&
		(n.Tag == "" || n.Tag == "!" || shortYAMLTag(n.Tag) == "!!merge")
}
