package config

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

// TestRuntimeLoadUsesStrictValidator proves, via go/ast over config.go, that
// the resolved-file loader calls parseAndValidate. This prevents a future
// refactor from silently restoring permissive yaml.Unmarshal runtime loading.
func TestRuntimeLoadUsesStrictValidator(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "config.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing config.go: %v", err)
	}

	foundLoader := false
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name.Name != "loadResolvedPath" {
			continue
		}
		foundLoader = true
		foundValidator := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "parseAndValidate" {
				foundValidator = true
			}
			return true
		})
		if !foundValidator {
			t.Fatal("loadResolvedPath does not reference parseAndValidate")
		}
	}
	if !foundLoader {
		t.Fatal("config.go has no loadResolvedPath function")
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
// valid — the merge stays confined to system_page's own canonical group
// map, which the merge produces no unknown key from. ("health_group" is
// used, rather than an arbitrary name, because c3 now validates group
// names too: an unrecognized group name would itself be a KindSchema
// error, which is not what this test is proving.)
func TestParseAndValidateBareMergeKeyNotSchemaError(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group: &g\n    enabled: true\n  health_group:\n    <<: *g\n"

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

// TestParseAndValidateUnknownGroupRejected confirms row 8: an unrecognized
// group name is KindSchema, naming the offending name and its line.
func TestParseAndValidateUnknownGroupRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  nope_group:\n    enabled: true\n"

	raw, err := parseAndValidate(path, []byte(data))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantSchema(t, err, path)
	if !strings.Contains(err.Detail, "nope_group") {
		t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "nope_group")
	}
	if !strings.Contains(err.Detail, "line 2") {
		t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "line 2")
	}
}

// TestParseAndValidateKnownGroupNull confirms row 9: a known group with a
// null value is a no-op, returning a nil error and a non-nil *rawConfig.
func TestParseAndValidateKnownGroupNull(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group:\n"

	raw, err := parseAndValidate(path, []byte(data))
	if err != nil {
		t.Fatalf("parseAndValidate(...) error = %v, want nil", err)
	}
	if raw == nil {
		t.Fatalf("parseAndValidate(...) rawConfig = nil, want non-nil")
	}
}

// TestParseAndValidateKnownGroupWrongShape confirms row 10: a known group
// with a scalar/sequence value is KindParseType with a positive line.
func TestParseAndValidateKnownGroupWrongShape(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"scalar value", "system_page:\n  system_info_group: 3\n"},
		{"sequence value", "system_page:\n  system_info_group:\n    - a\n"},
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

// TestParseAndValidateUnknownGroupFieldRejected confirms row 11: an
// unrecognized field name is KindSchema, naming it and its line.
func TestParseAndValidateUnknownGroupFieldRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group:\n    nope_field: 1\n"

	raw, err := parseAndValidate(path, []byte(data))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantSchema(t, err, path)
	if !strings.Contains(err.Detail, "nope_field") {
		t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "nope_field")
	}
	if !strings.Contains(err.Detail, "line 3") {
		t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "line 3")
	}
}

// TestParseAndValidateGroupFieldTypeMismatchRejected confirms row 15 for
// non-"actions" fields: a value that cannot decode into the declared Go
// type is KindParseType with a non-nil Err and a positive line (I4).
func TestParseAndValidateGroupFieldTypeMismatchRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"bool field given a sequence", "system_page:\n  system_info_group:\n    enabled: [1, 2]\n"},
		{"string field given a mapping", "system_page:\n  system_info_group:\n    app_id: {a: 1}\n"},
		{"string slice field given a scalar", "system_page:\n  system_info_group:\n    bundles_paths: 5\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := parseAndValidate(path, []byte(tt.data))
			if raw != nil {
				t.Fatalf("parseAndValidate(%q) rawConfig = %+v, want nil", tt.data, raw)
			}
			wantParseType(t, err, path)
			if err.Err == nil {
				t.Fatalf("err.Err = nil, want yaml.v3's own type error")
			}
			if !strings.Contains(err.Detail, "line ") {
				t.Fatalf("err.Detail = %q, want it to contain a positive line", err.Detail)
			}
		})
	}
}

// TestParseAndValidateUnknownGroupPrecedesValueInspection confirms name
// classification precedes value inspection at group level: all four value
// shapes for an unrecognized group name return KindSchema.
func TestParseAndValidateUnknownGroupPrecedesValueInspection(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"null value", "system_page:\n  nope_group:\n"},
		{"scalar value", "system_page:\n  nope_group: 3\n"},
		{"sequence value", "system_page:\n  nope_group: [a]\n"},
		{"mapping value", "system_page:\n  nope_group: {k: 1}\n"},
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

// TestParseAndValidateUnknownGroupFieldPrecedesValueInspection confirms the
// same precedence rule one level down, for an unrecognized field name.
func TestParseAndValidateUnknownGroupFieldPrecedesValueInspection(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"null value", "system_page:\n  system_info_group:\n    nope_field:\n"},
		{"scalar value", "system_page:\n  system_info_group:\n    nope_field: 3\n"},
		{"sequence value", "system_page:\n  system_info_group:\n    nope_field: [a]\n"},
		{"mapping value", "system_page:\n  system_info_group:\n    nope_field: {k: 1}\n"},
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

// TestParseAndValidateNonStringGroupKeyRejected confirms every non-!!str
// key shape at group level is KindParseType, one level down from the page
// case.
func TestParseAndValidateNonStringGroupKeyRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"integer key", "system_page:\n  1: x\n"},
		{"boolean key", "system_page:\n  true: x\n"},
		{"null key", "system_page:\n  ~: x\n"},
		{"custom-tagged scalar key", "system_page:\n  !custom foo: x\n"},
		{"sequence key", "system_page:\n  ? [a]\n  : x\n"},
		{"mapping key", "system_page:\n  ? {a: 1}\n  : x\n"},
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

// TestParseAndValidateNonStringGroupFieldKeyRejected confirms every
// non-!!str key shape at group-field level is KindParseType.
func TestParseAndValidateNonStringGroupFieldKeyRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"integer key", "system_page:\n  system_info_group:\n    1: x\n"},
		{"boolean key", "system_page:\n  system_info_group:\n    true: x\n"},
		{"null key", "system_page:\n  system_info_group:\n    ~: x\n"},
		{"custom-tagged scalar key", "system_page:\n  system_info_group:\n    !custom foo: x\n"},
		{"sequence key", "system_page:\n  system_info_group:\n    ? [a]\n    : x\n"},
		{"mapping key", "system_page:\n  system_info_group:\n    ? {a: 1}\n    : x\n"},
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

// TestParseAndValidateAliasToNonStringGroupKeyRejected confirms an
// alias-to-non-string group key is KindParseType, one level down from the
// page case (O2), reporting the anchor's line (3), not the alias's (I6).
func TestParseAndValidateAliasToNonStringGroupKeyRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group:\n    enabled: &n true\n  *n: x\n"

	raw, err := parseAndValidate(path, []byte(data))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantParseType(t, err, path)
	if !strings.Contains(err.Detail, "line 3") {
		t.Fatalf("err.Detail = %q, want it to contain %q (the anchor's line)", err.Detail, "line 3")
	}
}

// TestParseAndValidateAliasToNonStringGroupFieldKeyRejected is the
// group-field-level counterpart, with the alias key a sibling field name
// within system_info_group's own mapping; the reported line is again 3.
func TestParseAndValidateAliasToNonStringGroupFieldKeyRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group:\n    enabled: &n true\n    *n: x\n"

	raw, err := parseAndValidate(path, []byte(data))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantParseType(t, err, path)
	if !strings.Contains(err.Detail, "line 3") {
		t.Fatalf("err.Detail = %q, want it to contain %q (the anchor's line)", err.Detail, "line 3")
	}
}

// TestParseAndValidateEveryCanonicalGroupAccepted iterates every group
// SchemaGroups(page) returns, for every SchemaPages() page, and confirms
// each is accepted with a null value (no canonical group is accidentally
// rejected as unknown).
func TestParseAndValidateEveryCanonicalGroupAccepted(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	pages, err := SchemaPages()
	if err != nil {
		t.Fatalf("SchemaPages() error = %v, want nil", err)
	}

	for _, page := range pages {
		groups, err := SchemaGroups(page)
		if err != nil {
			t.Fatalf("SchemaGroups(%q) error = %v, want nil", page, err)
		}
		for _, group := range groups {
			t.Run(page+"/"+group, func(t *testing.T) {
				data := page + ":\n  " + group + ":\n"
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
}

// sampleYAMLValueForType returns a flow-style YAML literal decoding cleanly
// into t (a rawGroupConfig field's declared, always-pointer Go type),
// derived purely from t's reflect.Kind, never a field name.
func sampleYAMLValueForType(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return "true"
	case reflect.String:
		return `"x"`
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Struct {
			// e.g. []ActionConfig: an empty sequence decodes cleanly and
			// this chunk does not yet inspect actions' value (I5) anyway.
			return "[]"
		}
		return `["x"]`
	default:
		return "null"
	}
}

// TestParseAndValidateEveryGroupFieldAccepted iterates every name
// SchemaGroupFields() returns and confirms each is accepted with a
// type-correct value (no canonical field is accidentally unknown).
func TestParseAndValidateEveryGroupFieldAccepted(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	fields, err := SchemaGroupFields()
	if err != nil {
		t.Fatalf("SchemaGroupFields() error = %v, want nil", err)
	}
	if len(fields) == 0 {
		t.Fatalf("SchemaGroupFields() = %v, want at least one field", fields)
	}

	rt := reflect.TypeOf(rawGroupConfig{})
	fieldTypes := make(map[string]reflect.Type, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag, ok := f.Tag.Lookup("yaml")
		if !ok {
			continue
		}
		if idx := strings.Index(tag, ","); idx >= 0 {
			tag = tag[:idx]
		}
		fieldTypes[tag] = f.Type
	}

	for _, field := range fields {
		fieldType, ok := fieldTypes[field]
		if !ok {
			t.Fatalf("no reflect.Type found for group field %q", field)
		}
		t.Run(field, func(t *testing.T) {
			data := "system_page:\n  system_info_group:\n    " + field + ": " + sampleYAMLValueForType(fieldType) + "\n"
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

// TestParseAndValidateActionsValueWrongShape confirms row 12: a known
// "actions" field's value that is neither null nor a sequence (a scalar or
// a mapping) is KindParseType, with Path copied and a positive line named
// in Detail. A null "actions" value is confirmed separately as a no-op.
func TestParseAndValidateActionsValueWrongShape(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"scalar value", "system_page:\n  system_info_group:\n    actions: 5\n"},
		{"mapping value", "system_page:\n  system_info_group:\n    actions:\n      k: 1\n"},
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

	t.Run("null value", func(t *testing.T) {
		const data = "system_page:\n  system_info_group:\n    actions:\n"
		raw, err := parseAndValidate(path, []byte(data))
		if err != nil {
			t.Fatalf("parseAndValidate(...) error = %v, want nil", err)
		}
		if raw == nil {
			t.Fatalf("parseAndValidate(...) rawConfig = nil, want non-nil")
		}
	})
}

// TestParseAndValidateActionEntryWrongShape confirms row 13: every
// "actions" sequence entry must be a YAML mapping. A null entry is
// explicitly not accepted as a zero action — a separate assertion confirms
// the returned *rawConfig is nil for it, not merely that an error is
// returned — and a scalar entry and a sequence entry are likewise rejected.
// All three are KindParseType.
func TestParseAndValidateActionEntryWrongShape(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	t.Run("null entry", func(t *testing.T) {
		const data = "system_page:\n  system_info_group:\n    actions:\n      - ~\n"
		raw, err := parseAndValidate(path, []byte(data))
		if raw != nil {
			t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil (a null action entry is not a zero action)", raw)
		}
		wantParseType(t, err, path)
	})
	t.Run("scalar entry", func(t *testing.T) {
		const data = "system_page:\n  system_info_group:\n    actions:\n      - 5\n"
		raw, err := parseAndValidate(path, []byte(data))
		if raw != nil {
			t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
		}
		wantParseType(t, err, path)
	})
	t.Run("sequence entry", func(t *testing.T) {
		const data = "system_page:\n  system_info_group:\n    actions:\n      - [a]\n"
		raw, err := parseAndValidate(path, []byte(data))
		if raw != nil {
			t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
		}
		wantParseType(t, err, path)
	})
}

// TestParseAndValidateUnknownActionFieldRejected confirms row 14: an
// unrecognized action field name is KindSchema with Detail containing the
// offending name and a positive line — proving the structural actions walk
// catches what yaml.v3's Node.Decode silently ignores (I5).
func TestParseAndValidateUnknownActionFieldRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group:\n    actions:\n      - bogus: 1\n"

	raw, err := parseAndValidate(path, []byte(data))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantSchema(t, err, path)
	if !strings.Contains(err.Detail, "bogus") {
		t.Fatalf("err.Detail = %q, want it to contain %q", err.Detail, "bogus")
	}
	if !strings.Contains(err.Detail, "line ") {
		t.Fatalf("err.Detail = %q, want it to contain a positive line", err.Detail)
	}
}

// TestParseAndValidateUnknownActionFieldPrecedesValueInspection confirms
// unknown-action-field classification precedes value inspection: all four
// value shapes (null, scalar, sequence, mapping) under key "bogus" return
// KindSchema, never KindParseType.
func TestParseAndValidateUnknownActionFieldPrecedesValueInspection(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"null value", "system_page:\n  system_info_group:\n    actions:\n      - bogus:\n"},
		{"scalar value", "system_page:\n  system_info_group:\n    actions:\n      - bogus: 3\n"},
		{"sequence value", "system_page:\n  system_info_group:\n    actions:\n      - bogus: [a]\n"},
		{"mapping value", "system_page:\n  system_info_group:\n    actions:\n      - bogus: {k: 1}\n"},
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

// TestParseAndValidateActionFieldTypeMismatchRejected confirms row 15 for
// action fields: a value that cannot decode into the declared Go type is
// KindParseType with a non-nil Err.
func TestParseAndValidateActionFieldTypeMismatchRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group:\n    actions:\n      - sudo: {a: 1}\n"

	raw, err := parseAndValidate(path, []byte(data))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantParseType(t, err, path)
	if err.Err == nil {
		t.Fatalf("err.Err = nil, want yaml.v3's own type error")
	}
}

// TestParseAndValidateNonStringActionFieldKeyRejected confirms every
// non-!!str key shape at action-field level is KindParseType, matching the
// page/group/group-field cases one schema level down.
func TestParseAndValidateNonStringActionFieldKeyRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"integer key", "system_page:\n  system_info_group:\n    actions:\n      - 1: x\n"},
		{"boolean key", "system_page:\n  system_info_group:\n    actions:\n      - true: x\n"},
		{"null key", "system_page:\n  system_info_group:\n    actions:\n      - ~: x\n"},
		{"custom-tagged scalar key", "system_page:\n  system_info_group:\n    actions:\n      - !custom foo: x\n"},
		{"sequence key", "system_page:\n  system_info_group:\n    actions:\n      - ? [a]\n        : x\n"},
		{"mapping key", "system_page:\n  system_info_group:\n    actions:\n      - ? {a: 1}\n        : x\n"},
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

// TestParseAndValidateAliasToNonStringActionFieldKeyRejected confirms an
// alias-to-non-string key inside an action entry is KindParseType, one
// schema level down from the group-field case (O2). The anchor is
// introduced on a structurally valid action field ("sudo") in that same
// entry so the entry's own valid field does not pre-empt the alias-key
// failure (I3); the reported line is the anchor's own line (4), not the
// alias's.
func TestParseAndValidateAliasToNonStringActionFieldKeyRejected(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group:\n    actions:\n      - sudo: &n true\n        *n: x\n"

	raw, err := parseAndValidate(path, []byte(data))
	if raw != nil {
		t.Fatalf("parseAndValidate(...) rawConfig = %+v, want nil", raw)
	}
	wantParseType(t, err, path)
	if !strings.Contains(err.Detail, "line 4") {
		t.Fatalf("err.Detail = %q, want it to contain %q (the anchor's line)", err.Detail, "line 4")
	}
}

// TestParseAndValidateEveryActionFieldAccepted iterates every name
// SchemaActionFields() returns and confirms each is accepted with a
// type-correct value (no canonical action field is accidentally unknown).
func TestParseAndValidateEveryActionFieldAccepted(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	fields, err := SchemaActionFields()
	if err != nil {
		t.Fatalf("SchemaActionFields() error = %v, want nil", err)
	}
	if len(fields) == 0 {
		t.Fatalf("SchemaActionFields() = %v, want at least one field", fields)
	}

	at := reflect.TypeOf(ActionConfig{})
	fieldTypes := make(map[string]reflect.Type, at.NumField())
	for i := 0; i < at.NumField(); i++ {
		f := at.Field(i)
		tag, ok := f.Tag.Lookup("yaml")
		if !ok {
			continue
		}
		if idx := strings.Index(tag, ","); idx >= 0 {
			tag = tag[:idx]
		}
		fieldTypes[tag] = f.Type
	}

	for _, field := range fields {
		fieldType, ok := fieldTypes[field]
		if !ok {
			t.Fatalf("no reflect.Type found for action field %q", field)
		}
		t.Run(field, func(t *testing.T) {
			data := "system_page:\n  system_info_group:\n    actions:\n      - " + field + ": " + sampleYAMLValueForType(fieldType) + "\n"
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

// TestParseAndValidateTypeErrorRegression is the errors.As(..., *yaml.TypeError)
// regression: a group-field type failure and an action-field type failure
// must both surface yaml.v3's own *yaml.TypeError, reachable through
// *LoadError.Unwrap.
func TestParseAndValidateTypeErrorRegression(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	t.Run("group field", func(t *testing.T) {
		const data = "system_page:\n  system_info_group:\n    enabled: [1, 2]\n"
		_, loadErr := parseAndValidate(path, []byte(data))
		wantParseType(t, loadErr, path)

		var typeErr *yaml.TypeError
		if !errors.As(error(loadErr), &typeErr) {
			t.Fatalf("errors.As(loadErr, &typeErr) = false, want true (loadErr = %v)", loadErr)
		}
	})

	t.Run("action field", func(t *testing.T) {
		const data = "system_page:\n  system_info_group:\n    actions:\n      - sudo: [1, 2]\n"
		_, loadErr := parseAndValidate(path, []byte(data))
		wantParseType(t, loadErr, path)

		var typeErr *yaml.TypeError
		if !errors.As(error(loadErr), &typeErr) {
			t.Fatalf("errors.As(loadErr, &typeErr) = false, want true (loadErr = %v)", loadErr)
		}
	})
}

// TestParseAndValidateExplicitZeroPreserved confirms row 16's explicit-zero
// preservation: a valid document setting enabled: false, app_id: "" and
// bundles_paths: [] decodes to a *rawConfig whose Enabled, AppID and
// BundlesPaths pointers are all non-nil, carrying false, "" and an empty
// non-nil slice respectively — proving these are preserved as explicitly
// set values, not converted to omission.
func TestParseAndValidateExplicitZeroPreserved(t *testing.T) {
	const path = "/etc/chairlift/config.yml"
	const data = "system_page:\n  system_info_group:\n    enabled: false\n    app_id: \"\"\n    bundles_paths: []\n"

	raw, err := parseAndValidate(path, []byte(data))
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
		t.Fatalf("group.Enabled = nil, want a non-nil *bool")
	}
	if *group.Enabled != false {
		t.Fatalf("*group.Enabled = %v, want false", *group.Enabled)
	}

	if group.AppID == nil {
		t.Fatalf("group.AppID = nil, want a non-nil *string")
	}
	if *group.AppID != "" {
		t.Fatalf("*group.AppID = %q, want %q", *group.AppID, "")
	}

	if group.BundlesPaths == nil {
		t.Fatalf("group.BundlesPaths = nil, want a non-nil *[]string")
	}
	if *group.BundlesPaths == nil {
		t.Fatalf("*group.BundlesPaths = nil, want a non-nil empty slice")
	}
	if len(*group.BundlesPaths) != 0 {
		t.Fatalf("*group.BundlesPaths = %v, want empty", *group.BundlesPaths)
	}
}

// TestParseAndValidatePathSchemaName covers the O1 matrix's schema-name
// category: err.Path equals the path argument for every level where an
// unknown name can arise — page, group, group field, action field. There is
// no schema-name subtest at document level (a document has no name) or at
// action-entry level (a sequence entry has no name either), matching the
// plan.md matrix.
func TestParseAndValidatePathSchemaName(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"unknown page", "not_a_page: 1\n"},
		{"unknown group", "system_page:\n  nope_group:\n    enabled: true\n"},
		{"unknown group field", "system_page:\n  system_info_group:\n    nope_field: 1\n"},
		{"unknown action field", "system_page:\n  system_info_group:\n    actions:\n      - bogus: 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAndValidate(path, []byte(tt.data))
			if err == nil {
				t.Fatalf("parseAndValidate(%q) error = nil, want non-nil", tt.data)
			}
			if err.Path != path {
				t.Fatalf("err.Path = %q, want %q", err.Path, path)
			}
		})
	}
}

// TestParseAndValidatePathValidatorShape covers the O1 matrix's
// validator-created-shape category: err.Path equals the path argument, and
// Detail contains a positive line, for every level a shape failure can
// arise at — document (a top-level sequence), page value, group value,
// actions value, and a null action entry.
func TestParseAndValidatePathValidatorShape(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"document (top-level sequence)", "- a\n- b\n"},
		{"page value (scalar)", "system_page: 3\n"},
		{"group value (scalar)", "system_page:\n  system_info_group: 3\n"},
		{"actions value (scalar)", "system_page:\n  system_info_group:\n    actions: 5\n"},
		{"null action entry", "system_page:\n  system_info_group:\n    actions:\n      - ~\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAndValidate(path, []byte(tt.data))
			if err == nil {
				t.Fatalf("parseAndValidate(%q) error = nil, want non-nil", tt.data)
			}
			if err.Path != path {
				t.Fatalf("err.Path = %q, want %q", err.Path, path)
			}
			if !strings.Contains(err.Detail, "line ") {
				t.Fatalf("err.Detail = %q, want it to contain a positive line", err.Detail)
			}
		})
	}
}

// TestParseAndValidatePathDeclaredType covers the O1 matrix's
// declared-Go-type category: err.Path equals the path argument for a
// group-field type failure and an action-field type failure. There is no
// declared-type subtest at page, group, or action-entry level — none of
// those has a single declared Go field type to decode into (a page/group is
// validated field by field, and a sequence entry has no declared type of
// its own) — matching the plan.md matrix.
func TestParseAndValidatePathDeclaredType(t *testing.T) {
	const path = "/etc/chairlift/config.yml"

	tests := []struct {
		name string
		data string
	}{
		{"group field", "system_page:\n  system_info_group:\n    enabled: [1, 2]\n"},
		{"action field", "system_page:\n  system_info_group:\n    actions:\n      - sudo: {a: 1}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAndValidate(path, []byte(tt.data))
			if err == nil {
				t.Fatalf("parseAndValidate(%q) error = nil, want non-nil", tt.data)
			}
			if err.Path != path {
				t.Fatalf("err.Path = %q, want %q", err.Path, path)
			}
		})
	}
}
