package config

import (
	"strconv"
	"strings"
	"testing"
)

// buildSourcePathDepthDocument returns the YAML source text
// "nope: " followed by d nested flow sequences ("[" * d + "x" + "]" * d),
// all on a single line. Counting rule (measured against the merged
// parseYAMLDocument + resolveEffective resolver, not estimated):
// pathVisitCount (sourcebounds.go) charges one visit per node along a
// root-to-leaf path, and a mapping key is a sibling of its value rather
// than an extra hop, so this document's deepest path costs
// 1 (document) + 1 (top mapping) + d (nested flow sequences) + 1 (leaf
// scalar "x") = d + 3 visits against maxSourcePathVisits = 128. "nope" is
// deliberately not a canonical page name, so a document at or below the
// bound still reaches stage 3's page-name check and returns KindSchema —
// proving the source-path bounds pass was cleared, not merely unreached.
func buildSourcePathDepthDocument(d int) string {
	return "nope: " + strings.Repeat("[", d) + "x" + strings.Repeat("]", d) + "\n"
}

// buildDoublingAliasChainDocument returns the YAML source text for the
// doubling alias chain l0: &l0 [x]; lN: &lN [*l(N-1), *l(N-1)] through
// level n. Each level's sequence aliases the previous level's anchor
// twice, so resolveEffective's alias-free emission of level n contains two
// full copies of level n-1's emission, doubling the emitted node count per
// level while charging only one alias hop per node (nowhere near
// maxAliasHops = 64) and keeping the source graph's own path-visit count
// at roughly 33 (nowhere near maxSourcePathVisits = 128) — so this fixture
// exercises maxEffectiveOutputNodes = 100000 in isolation. Every level
// name (l0, l1, ...) is deliberately not a canonical page name, so a chain
// at or below the bound still reaches stage 3's page-name check on its
// first entry, l0, and returns KindSchema.
func buildDoublingAliasChainDocument(n int) string {
	var b strings.Builder
	b.WriteString("l0: &l0 [x]\n")
	for i := 1; i <= n; i++ {
		prev := "l" + strconv.Itoa(i-1)
		cur := "l" + strconv.Itoa(i)
		b.WriteString(cur + ": &" + cur + " [*" + prev + ", *" + prev + "]\n")
	}
	return b.String()
}

// TestParseAndValidateSourcePathVisitBoundary drives maxSourcePathVisits
// (128) through parseAndValidate using buildSourcePathDepthDocument,
// measured exactly against the merged resolver: d = 125 (128 visits)
// passes the source-path bounds pass and is rejected only afterward, by
// stage 3, as KindSchema for the unknown page "nope" — proving the bounds
// pass ran and cleared. d = 126 (129 visits) fails the bounds pass itself,
// before stage 3 ever runs, as KindParseType naming the "128" bound.
//
// maxAliasHops (64) is deliberately not exercised by any fixture in this
// file: it is unreachable from parser-produced text without an alias, and
// sourcebounds_test.go's existing direct checkAliasHopLimit/synthetic-graph
// tests already cover that boundary exactly as the spec directs.
func TestParseAndValidateSourcePathVisitBoundary(t *testing.T) {
	const path = "/etc/chairlift/source-path-boundary.yml"

	t.Run("d=125 clears the bounds pass", func(t *testing.T) {
		raw, err := parseAndValidate(path, []byte(buildSourcePathDepthDocument(125)))
		if raw != nil {
			t.Fatalf("parseAndValidate(d=125) rawConfig = %+v, want nil", raw)
		}
		wantSchema(t, err, path)
		if !strings.Contains(err.Detail, "nope") {
			t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "nope")
		}
		if got := sourceDetailLine(t, err); got != 1 {
			t.Fatalf("attributed line = %d, want exactly 1", got)
		}
	})

	t.Run("d=126 fails the bounds pass", func(t *testing.T) {
		raw, err := parseAndValidate(path, []byte(buildSourcePathDepthDocument(126)))
		if raw != nil {
			t.Fatalf("parseAndValidate(d=126) rawConfig = %+v, want nil", raw)
		}
		wantParseType(t, err, path)
		if err.Err != nil {
			t.Fatalf("err.Err = %v, want nil (a validator-side bounds rejection, not a wrapped yaml.v3 error)", err.Err)
		}
		wantDetail := "maximum of 128 source-node path visits"
		if !strings.Contains(err.Detail, wantDetail) {
			t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, wantDetail)
		}
		if got := sourceDetailLine(t, err); got != 1 {
			t.Fatalf("attributed line = %d, want exactly 1", got)
		}
	})
}

// TestParseAndValidateEffectiveOutputNodeBoundary drives
// maxEffectiveOutputNodes (100000) through parseAndValidate using
// buildDoublingAliasChainDocument, measured exactly against the merged
// resolver: n = 14 passes both source-graph bounds passes and
// resolveEffective's own emission budget, then is rejected only
// afterward, by stage 3, as KindSchema for the unknown page "l0" —
// proving the effective-output bound was cleared. n = 15 fails
// resolveEffective's emission budget itself, before stage 3 ever runs, as
// KindParseType naming the "100000" bound.
func TestParseAndValidateEffectiveOutputNodeBoundary(t *testing.T) {
	const path = "/etc/chairlift/effective-output-boundary.yml"

	t.Run("n=14 clears the bounds pass", func(t *testing.T) {
		raw, err := parseAndValidate(path, []byte(buildDoublingAliasChainDocument(14)))
		if raw != nil {
			t.Fatalf("parseAndValidate(n=14) rawConfig = %+v, want nil", raw)
		}
		wantSchema(t, err, path)
		if !strings.Contains(err.Detail, "l0") {
			t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "l0")
		}
	})

	t.Run("n=15 fails the bounds pass", func(t *testing.T) {
		raw, err := parseAndValidate(path, []byte(buildDoublingAliasChainDocument(15)))
		if raw != nil {
			t.Fatalf("parseAndValidate(n=15) rawConfig = %+v, want nil", raw)
		}
		wantParseType(t, err, path)
		if err.Err != nil {
			t.Fatalf("err.Err = %v, want nil (a validator-side bounds rejection, not a wrapped yaml.v3 error)", err.Err)
		}
		wantDetail := "maximum of 100000 emitted nodes"
		if !strings.Contains(err.Detail, wantDetail) {
			t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, wantDetail)
		}
		if got := sourceDetailLine(t, err); got != 2 {
			t.Fatalf("attributed line = %d, want exactly 2", got)
		}
	})
}

// TestParseAndValidatePrecedenceOverKindSchema confirms that a document
// simultaneously containing an unknown top-level page name and a stage
// 1/stage 2 failure is always classified KindParseType, never KindSchema:
// stage 3's page-name check (which would otherwise report "nope" as an
// unknown page, KindSchema) is never reached once an earlier stage fails.
func TestParseAndValidatePrecedenceOverKindSchema(t *testing.T) {
	const path = "/etc/chairlift/precedence.yml"

	t.Run("unknown page plus a second YAML document", func(t *testing.T) {
		raw, err := parseAndValidate(path, []byte("nope: 1\n---\nb: 2\n"))
		if raw != nil {
			t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
		}
		wantParseType(t, err, path)
		if strings.Contains(err.Detail, "unknown name") {
			t.Fatalf("err.Detail = %q, want the parser's second-document rejection, not the schema-name rejection", err.Detail)
		}
	})

	t.Run("unknown page plus an over-budget nested value", func(t *testing.T) {
		// The unknown top-level page name itself ("nope") carries the
		// over-budget nested flow sequence as its own value, so this is
		// the same document buildSourcePathDepthDocument(126) builds:
		// both conditions (unknown page, over-budget path) hold at once,
		// and only KindParseType is observed.
		raw, err := parseAndValidate(path, []byte(buildSourcePathDepthDocument(126)))
		if raw != nil {
			t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
		}
		wantParseType(t, err, path)
		if strings.Contains(err.Detail, "unknown name") {
			t.Fatalf("err.Detail = %q, want the bounds rejection, not the schema-name rejection", err.Detail)
		}
	})

	t.Run("unknown page plus a source-duplicate key", func(t *testing.T) {
		raw, err := parseAndValidate(path, []byte("nope:\n  a: 1\n  a: 2\n"))
		if raw != nil {
			t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
		}
		wantParseType(t, err, path)
		if strings.Contains(err.Detail, "unknown name") {
			t.Fatalf("err.Detail = %q, want the duplicate-key rejection, not the schema-name rejection", err.Detail)
		}
		if !strings.Contains(err.Detail, "duplicated") {
			t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "duplicated")
		}
	})
}
