package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// wantSchema fails the test unless err is a non-nil *LoadError with
// Kind == KindSchema and Path == path.
func wantSchema(t *testing.T, err *LoadError, path string) {
	t.Helper()
	if err == nil {
		t.Fatalf("parseAndValidate(%q, ...) = nil, want a KindSchema error", path)
	}
	if err.Kind != KindSchema {
		t.Fatalf("err.Kind = %q, want %q", err.Kind, KindSchema)
	}
	if err.Path != path {
		t.Fatalf("err.Path = %q, want %q", err.Path, path)
	}
}

// wantNoopRawConfig fails the test unless parseAndValidate(path, data)
// returns a non-nil, zero-valued *rawConfig (every page map nil/empty) and
// a nil error, as required for a document-level no-op overlay.
func wantNoopRawConfig(t *testing.T, path string, data []byte) {
	t.Helper()
	raw, err := parseAndValidate(path, data)
	if err != nil {
		t.Fatalf("parseAndValidate(%q, %q) error = %v, want nil", path, data, err)
	}
	if raw == nil {
		t.Fatalf("parseAndValidate(%q, %q) rawConfig = nil, want non-nil", path, data)
	}
	pages := map[string]rawPageConfig{
		"system_page":       raw.SystemPage,
		"updates_page":      raw.UpdatesPage,
		"applications_page": raw.ApplicationsPage,
		"maintenance_page":  raw.MaintenancePage,
		"features_page":     raw.FeaturesPage,
		"help_page":         raw.HelpPage,
	}
	for name, page := range pages {
		if len(page) != 0 {
			t.Fatalf("parseAndValidate(%q, %q) rawConfig.%s = %+v, want nil/empty", path, data, name, page)
		}
	}
}

// TestParseAndValidateNoopOverlay confirms that empty input, whitespace-only
// input, and a top-level null document (`null` or `~`) all succeed with a
// non-nil, zero-valued *rawConfig and no error: each is usable as a no-op
// overlay.
func TestParseAndValidateNoopOverlay(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	t.Run("empty input", func(t *testing.T) {
		wantNoopRawConfig(t, path, []byte(""))
	})
	t.Run("whitespace-only input", func(t *testing.T) {
		wantNoopRawConfig(t, path, []byte("   \n   \n"))
	})
	t.Run("top-level null scalar", func(t *testing.T) {
		wantNoopRawConfig(t, path, []byte("null\n"))
	})
	t.Run("top-level tilde null scalar", func(t *testing.T) {
		wantNoopRawConfig(t, path, []byte("~\n"))
	})
}

// TestParseAndValidateTopLevelScalarRejected confirms a non-null top-level
// scalar document is rejected as KindParseType, with Path copied and a
// positive line named in Detail.
func TestParseAndValidateTopLevelScalarRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	raw, err := parseAndValidate(path, []byte("just a scalar\n"))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantParseType(t, err, path)
	if !strings.Contains(err.Detail, "line 1") {
		t.Fatalf("err.Detail = %q, want it to contain a positive %q", err.Detail, "line 1")
	}
}

// TestParseAndValidateTopLevelSequenceRejected confirms a top-level
// sequence document is rejected as KindParseType, with Path copied and a
// positive line named in Detail.
func TestParseAndValidateTopLevelSequenceRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	raw, err := parseAndValidate(path, []byte("- a\n- b\n"))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantParseType(t, err, path)
	if !strings.Contains(err.Detail, "line 1") {
		t.Fatalf("err.Detail = %q, want it to contain a positive %q", err.Detail, "line 1")
	}
}

// TestParseAndValidateMalformedYAML confirms a stage 1 (parseYAMLDocument)
// parser error is surfaced unchanged: KindParseType with a non-nil Err
// wrapping yaml.v3's own parser error.
func TestParseAndValidateMalformedYAML(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	raw, err := parseAndValidate(path, []byte("a: [\n"))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantParseType(t, err, path)
	if err.Err == nil {
		t.Fatalf("err.Err = nil, want yaml.v3's own parser error")
	}
}

// TestParseAndValidateExtraDocumentRejected confirms a stage 1
// (parseYAMLDocument) "only one document supported" error is surfaced
// unchanged as KindParseType.
func TestParseAndValidateExtraDocumentRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	raw, err := parseAndValidate(path, []byte("a: 1\n---\nb: 2\n"))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantParseType(t, err, path)
}

// TestParseAndValidateMalformedSourceGraphRejected confirms a stage 2
// (resolveEffective -> validateSourceGraph) source-graph rejection is
// surfaced unchanged as KindParseType. The input is syntactically valid
// YAML (stage 1 succeeds) whose merge key ("<<") value is a plain scalar,
// which validateMergeOperand (sourcegraph.go) rejects as a malformed
// source graph.
func TestParseAndValidateMalformedSourceGraphRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	raw, err := parseAndValidate(path, []byte("a:\n  <<: 5\n"))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantParseType(t, err, path)
}

// TestParseAndValidateMinimalMapping confirms a minimal valid document
// decodes into a *rawConfig with the expected field set, and returns no
// error.
func TestParseAndValidateMinimalMapping(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	raw, err := parseAndValidate(path, []byte("system_page:\n  system_info_group:\n    enabled: false\n"))
	if err != nil {
		t.Fatalf("parseAndValidate(...) error = %v, want nil", err)
	}
	if raw == nil {
		t.Fatalf("parseAndValidate(...) rawConfig = nil, want non-nil")
	}
	group, ok := raw.SystemPage["system_info_group"]
	if !ok {
		t.Fatalf("raw.SystemPage[%q] missing", "system_info_group")
	}
	if group.Enabled == nil {
		t.Fatalf("raw.SystemPage[%q].Enabled = nil, want a non-nil *bool", "system_info_group")
	}
	if *group.Enabled != false {
		t.Fatalf("*raw.SystemPage[%q].Enabled = %v, want false", "system_info_group", *group.Enabled)
	}
}

// TestParseAndValidateStaysOutOfRuntimeLoad proves, via go/ast over
// internal/config/config.go, that neither Load nor loadFromPath's function
// body references parseAndValidate: this chunk adds parseAndValidate as a
// standalone capability without wiring it into the runtime config-loading
// path.
func TestParseAndValidateStaysOutOfRuntimeLoad(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "config.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing config.go: %v", err)
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if fn.Name.Name != "Load" && fn.Name.Name != "loadFromPath" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "parseAndValidate" {
				t.Fatalf("%s references parseAndValidate, want it left unwired in this chunk", fn.Name.Name)
			}
			return true
		})
	}
}

// TestParseAndValidateUnknownPageRejected confirms decision-table row 5: an
// unrecognized top-level page name is KindSchema, with Path copied and
// Detail containing both the literal offending name and a positive line.
func TestParseAndValidateUnknownPageRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	raw, err := parseAndValidate(path, []byte("not_a_page: 1\n"))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantSchema(t, err, path)
	if !strings.Contains(err.Detail, "not_a_page") {
		t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "not_a_page")
	}
	if !strings.Contains(err.Detail, "line 1") {
		t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "line 1")
	}
}

// TestParseAndValidateKnownPageNull confirms decision-table row 6: a known
// page with a null value is accepted (a no-op for that page), returning a
// nil error and a non-nil *rawConfig.
func TestParseAndValidateKnownPageNull(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	raw, err := parseAndValidate(path, []byte("system_page:\n"))
	if err != nil {
		t.Fatalf("parseAndValidate(...) error = %v, want nil", err)
	}
	if raw == nil {
		t.Fatalf("parseAndValidate(...) rawConfig = nil, want non-nil")
	}
}

// TestParseAndValidateKnownPageWrongShape confirms decision-table row 7: a
// known page whose value is a scalar or a sequence is KindParseType, with
// Path copied and a positive line named in Detail.
func TestParseAndValidateKnownPageWrongShape(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"scalar value", "system_page: 3\n"},
		{"sequence value", "system_page:\n  - a\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := parseAndValidate(path, []byte(tt.data))
			if raw != nil {
				t.Fatalf("parseAndValidate(%q) rawConfig = %+v, want nil", tt.data, raw)
			}
			wantParseType(t, err, path)
			if !strings.Contains(err.Detail, "line ") {
				t.Fatalf("err.Detail = %q, want it to contain a positive line", err.Detail)
			}
		})
	}
}

// TestParseAndValidateEveryCanonicalPageAccepted iterates every page name
// SchemaPages() returns and confirms each is accepted with a null value, so
// no canonical page is accidentally rejected by validatePageEntries's known-
// page check (docs/agents/skills/regression-tests-must-cover-every-
// collection-entry.md).
func TestParseAndValidateEveryCanonicalPageAccepted(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	pages, err := SchemaPages()
	if err != nil {
		t.Fatalf("SchemaPages() error = %v, want nil", err)
	}
	if len(pages) == 0 {
		t.Fatalf("SchemaPages() = %v, want at least one page", pages)
	}

	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			data := page + ":\n"
			raw, loadErr := parseAndValidate(path, []byte(data))
			if loadErr != nil {
				t.Fatalf("parseAndValidate(%q) error = %v, want nil", data, loadErr)
			}
			if raw == nil {
				t.Fatalf("parseAndValidate(%q) rawConfig = nil, want non-nil", data)
			}
		})
	}
}

// TestParseAndValidateNonStringPageKeyRejected confirms every non-!!str
// mapping-key shape at page level is KindParseType: schemaKeyName's name
// rule requires a scalar whose effective ShortTag() is exactly "!!str", so
// an integer, boolean, or null scalar key, a custom-tagged scalar key, a
// sequence key, and a mapping key are all rejected before any name lookup.
func TestParseAndValidateNonStringPageKeyRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"integer key", "1: x\n"},
		{"boolean key", "true: x\n"},
		{"null key", "~: x\n"},
		{"custom-tagged scalar key", "!custom foo: x\n"},
		{"sequence key", "? [a]\n: x\n"},
		{"mapping key", "? {a: 1}\n: x\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := parseAndValidate(path, []byte(tt.data))
			if raw != nil {
				t.Fatalf("parseAndValidate(%q) rawConfig = %+v, want nil", tt.data, raw)
			}
			wantParseType(t, err, path)
		})
	}
}

// TestParseAndValidateAliasToNonStringPageKeyRejected confirms that an
// alias-to-non-string page key is KindParseType, not KindSchema, proving the
// fixture reaches the alias key rather than failing earlier on an unknown
// page name. The fixture introduces the anchor through a structurally valid
// canonical subtree (system_page/system_info_group/enabled, a *bool field,
// so its own entry is valid) so the alias key really is the first failing
// entry (interpretation I3/O2).
//
// The reported line is 3 — the anchor's own line ("enabled: &n true"), not
// the alias's line (4, "*n: x") — because resolveEffective dereferences an
// alias into a copy of its anchor's node, carrying the anchor's Line
// (interpretation I6).
func TestParseAndValidateAliasToNonStringPageKeyRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group:\n    enabled: &n true\n*n: x\n"

	raw, err := parseAndValidate(path, []byte(data))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantParseType(t, err, path)
	if !strings.Contains(err.Detail, "line 3") {
		t.Fatalf("err.Detail = %q, want it to contain %q (the anchor's line)", err.Detail, "line 3")
	}
}

// TestParseAndValidateQuotedMergeKeyIsSchemaError confirms a quoted "<<" key
// is an ordinary !!str name and therefore KindSchema: the resolver leaves a
// quoted merge key's effective node tagged "!!str" (a bare merge key would
// instead carry "!!merge" and be consumed by the resolver before reaching
// the validator; see the next test).
func TestParseAndValidateQuotedMergeKeyIsSchemaError(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	raw, err := parseAndValidate(path, []byte("\"<<\": x\n"))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantSchema(t, err, path)
	if !strings.Contains(err.Detail, "<<") {
		t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "<<")
	}
}

// TestParseAndValidateBareMergeKeyNotSchemaError is a regression test: a
// document using a bare "<<: *anchor" merge key inside a valid canonical
// subtree must not produce a KindSchema "<<" error. resolveEffective's
// merge-key handling consumes the bare merge key entirely before the
// validator's per-entry walk ever sees it, so this document is simply
// valid — the merge stays confined to system_page's own (unvalidated at
// this chunk) group-level mapping, which the merge produces no unknown
// top-level key from.
func TestParseAndValidateBareMergeKeyNotSchemaError(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group: &g\n    enabled: true\n  other_group:\n    <<: *g\n"

	_, err := parseAndValidate(path, []byte(data))
	if err != nil {
		t.Fatalf("parseAndValidate(...) error = %v, want nil", err)
	}
}

// TestParseAndValidateUnknownPagePrecedesValueInspection confirms that
// unknown-name classification precedes value inspection at page level: all
// four value shapes (null, scalar, sequence, mapping) for an unrecognized
// page name return KindSchema, never KindParseType.
func TestParseAndValidateUnknownPagePrecedesValueInspection(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"null value", "nope:\n"},
		{"scalar value", "nope: 3\n"},
		{"sequence value", "nope:\n  - a\n"},
		{"mapping value", "nope:\n  k: 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := parseAndValidate(path, []byte(tt.data))
			if raw != nil {
				t.Fatalf("parseAndValidate(%q) rawConfig = %+v, want nil", tt.data, raw)
			}
			wantSchema(t, err, path)
		})
	}
}

// TestParseAndValidateEntryOrderConsequence confirms interpretation I3's
// per-entry ordering consequence: entries are classified in effective
// Content order, so which error surfaces depends on which failing entry
// comes first. "nope: x\nsystem_page: 3\n" fails on "nope" (KindSchema)
// before "system_page: 3\n"'s shape is ever inspected; reversing the entries
// makes "system_page: 3\n" the first failing entry (KindParseType) before
// "nope" is ever looked up.
func TestParseAndValidateEntryOrderConsequence(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	t.Run("unknown page first", func(t *testing.T) {
		_, err := parseAndValidate(path, []byte("nope: x\nsystem_page: 3\n"))
		wantSchema(t, err, path)
	})
	t.Run("known page with wrong shape first", func(t *testing.T) {
		_, err := parseAndValidate(path, []byte("system_page: 3\nnope: x\n"))
		wantParseType(t, err, path)
	})
}
