package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestEffectiveKeyIdentityScalarTagAware exercises effectiveKeyIdentity
// directly against scalar nodes, proving identity is tag-aware (Kind +
// resolved ShortTag + Value), not Value alone
// (docs/agents/skills/yaml-scalar-key-identity-needs-tag-not-just-value.md).
func TestEffectiveKeyIdentityScalarTagAware(t *testing.T) {
	bareOne := scalarNode("1")
	taggedIntOne := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"}
	taggedStrOne := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "1"}
	zeroOne := scalarNode("01")

	bareID := effectiveKeyIdentity(bareOne)
	taggedIntID := effectiveKeyIdentity(taggedIntOne)
	taggedStrID := effectiveKeyIdentity(taggedStrOne)
	zeroOneID := effectiveKeyIdentity(zeroOne)

	if bareID != taggedIntID {
		t.Fatalf("effectiveKeyIdentity(bare 1) = %+v, effectiveKeyIdentity(!!int 1) = %+v, want equal", bareID, taggedIntID)
	}
	if bareID == taggedStrID {
		t.Fatalf("effectiveKeyIdentity(bare 1) = %+v, effectiveKeyIdentity(!!str \"1\") = %+v, want unequal", bareID, taggedStrID)
	}
	if bareID == zeroOneID {
		t.Fatalf("effectiveKeyIdentity(1) = %+v, effectiveKeyIdentity(01) = %+v, want unequal", bareID, zeroOneID)
	}

	// effectiveKeyID must be usable directly as a map key.
	seen := map[effectiveKeyID]struct{}{
		bareID:      {},
		taggedStrID: {},
		zeroOneID:   {},
	}
	if len(seen) != 3 {
		t.Fatalf("map[effectiveKeyID]struct{} collapsed distinct identities: got %d entries, want 3", len(seen))
	}
	if _, ok := seen[taggedIntID]; !ok {
		t.Fatalf("map[effectiveKeyID]struct{} lookup for equal identity (bare 1 vs !!int 1) missed")
	}
}

// TestEffectiveKeyIdentityAliasToScalarMatchesDirectScalar asserts that
// effectiveKeyIdentity dereferences alias chains to their scalar target
// before computing identity, including a two-hop alias-to-alias-to-scalar
// chain.
func TestEffectiveKeyIdentityAliasToScalarMatchesDirectScalar(t *testing.T) {
	direct := scalarNode("foo")
	target := scalarNode("foo")
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: target}

	directID := effectiveKeyIdentity(direct)
	aliasID := effectiveKeyIdentity(alias)
	if directID != aliasID {
		t.Fatalf("effectiveKeyIdentity(direct scalar) = %+v, effectiveKeyIdentity(alias->scalar) = %+v, want equal", directID, aliasID)
	}

	aliasToAlias := &yaml.Node{Kind: yaml.AliasNode, Alias: alias}
	twoHopID := effectiveKeyIdentity(aliasToAlias)
	if directID != twoHopID {
		t.Fatalf("effectiveKeyIdentity(direct scalar) = %+v, effectiveKeyIdentity(alias->alias->scalar) = %+v, want equal", directID, twoHopID)
	}
}

// TestEffectiveKeyIdentityComplexKeysUsePointerIdentity asserts that
// mapping/sequence key identity is pointer-based with no structural
// expansion: distinct-but-identical-looking complex nodes are unequal,
// aliases sharing one complex target are equal, a sequence and a mapping
// with the same (empty) shape stay distinct, and a complex identity never
// equals any scalar identity.
func TestEffectiveKeyIdentityComplexKeysUsePointerIdentity(t *testing.T) {
	m1 := &yaml.Node{Kind: yaml.MappingNode}
	m2 := &yaml.Node{Kind: yaml.MappingNode}
	if effectiveKeyIdentity(m1) == effectiveKeyIdentity(m2) {
		t.Fatalf("two distinct structurally-identical mapping nodes produced equal identities, want unequal")
	}

	aliasToM1a := &yaml.Node{Kind: yaml.AliasNode, Alias: m1}
	aliasToM1b := &yaml.Node{Kind: yaml.AliasNode, Alias: m1}
	if effectiveKeyIdentity(aliasToM1a) != effectiveKeyIdentity(aliasToM1b) {
		t.Fatalf("two aliases pointing at the same complex target produced unequal identities, want equal")
	}

	seq := &yaml.Node{Kind: yaml.SequenceNode}
	mapNode := &yaml.Node{Kind: yaml.MappingNode}
	if effectiveKeyIdentity(seq) == effectiveKeyIdentity(mapNode) {
		t.Fatalf("a sequence key and a mapping key produced equal identities, want distinct (pointer identity)")
	}

	scalar := scalarNode("")
	if effectiveKeyIdentity(m1) == effectiveKeyIdentity(scalar) {
		t.Fatalf("complex identity equaled a scalar identity, want always distinct")
	}
}

// TestEffectiveKeyIdentitySelfAliasTerminates proves the local seen-set
// guard lets effectiveKeyIdentity terminate on a synthetic self-referential
// alias handed directly to it, rather than hanging. Real callers only ever
// pass nodes from a graph validateSourceGraph has already proven acyclic;
// this test covers the defense-in-depth backstop for synthetic input.
func TestEffectiveKeyIdentitySelfAliasTerminates(t *testing.T) {
	self := &yaml.Node{Kind: yaml.AliasNode}
	self.Alias = self

	done := make(chan effectiveKeyID, 1)
	go func() { done <- effectiveKeyIdentity(self) }()

	select {
	case got := <-done:
		if got != (effectiveKeyID{}) {
			t.Fatalf("effectiveKeyIdentity(self-referential alias) = %+v, want zero value", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("effectiveKeyIdentity(self-referential alias) did not return within 2s")
	}
}
