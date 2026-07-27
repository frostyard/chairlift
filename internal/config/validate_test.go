package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

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
