package config

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// lineNumberPattern extracts the N in a yaml.v3-style "line N" fragment, as
// rendered both by the library's own parser errors and by this package's
// extraDocumentFailure Detail text.
var lineNumberPattern = regexp.MustCompile(`line (\d+)`)

// lineNumber returns the first "line N" number found in s, failing the test
// if none is present.
func lineNumber(t *testing.T, s string) int {
	t.Helper()
	m := lineNumberPattern.FindStringSubmatch(s)
	if m == nil {
		t.Fatalf("no \"line N\" found in %q", s)
	}
	n := 0
	for _, r := range m[1] {
		n = n*10 + int(r-'0')
	}
	return n
}

func TestParseYAMLDocumentNilDataReturnsNil(t *testing.T) {
	node, err := parseYAMLDocument("cfg.yml", nil)
	if node != nil || err != nil {
		t.Fatalf("got (%v, %v), want (nil, nil)", node, err)
	}
}

func TestParseYAMLDocumentEmptyDataReturnsNil(t *testing.T) {
	node, err := parseYAMLDocument("cfg.yml", []byte(""))
	if node != nil || err != nil {
		t.Fatalf("got (%v, %v), want (nil, nil)", node, err)
	}
}

func TestParseYAMLDocumentWhitespaceOnlyReturnsNil(t *testing.T) {
	node, err := parseYAMLDocument("cfg.yml", []byte("   \n\n"))
	if node != nil || err != nil {
		t.Fatalf("got (%v, %v), want (nil, nil)", node, err)
	}
}

func TestParseYAMLDocumentValidSingleDocument(t *testing.T) {
	node, err := parseYAMLDocument("cfg.yml", []byte("a: 1\nb: 2\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node == nil {
		t.Fatal("got nil node for a valid document")
	}
	if node.Kind != yaml.DocumentNode {
		t.Fatalf("node.Kind = %v, want yaml.DocumentNode", node.Kind)
	}
	if len(node.Content) != 1 {
		t.Fatalf("len(node.Content) = %d, want 1", len(node.Content))
	}
}

func TestParseYAMLDocumentMalformedFirstDocument(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"badIndentAfterSequenceItem", "a:\n- b\n  c: d\n"},
		{"unterminatedFlowSequence", "foo: [1, 2\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := parseYAMLDocument("cfg.yml", []byte(tc.data))
			if node != nil {
				t.Fatalf("got non-nil node %v for malformed input", node)
			}
			if err == nil {
				t.Fatal("got nil error for malformed input")
			}
			if err.Kind != KindParseType {
				t.Fatalf("err.Kind = %v, want KindParseType", err.Kind)
			}
			if err.Path != "cfg.yml" {
				t.Fatalf("err.Path = %q, want %q", err.Path, "cfg.yml")
			}
			if err.Err == nil {
				t.Fatal("err.Err is nil, want the yaml parser error")
			}
			if !strings.Contains(err.Detail, "line ") {
				t.Fatalf("err.Detail = %q, want it to contain \"line \"", err.Detail)
			}
			wantLine := lineNumber(t, err.Err.Error())
			gotLine := lineNumber(t, err.Detail)
			if gotLine != wantLine {
				t.Fatalf("err.Detail line = %d, err.Err line = %d, want equal", gotLine, wantLine)
			}
		})
	}
}

func TestParseYAMLDocumentTwoValidDocumentsRejected(t *testing.T) {
	node, err := parseYAMLDocument("cfg.yml", []byte("a: 1\n---\nb: 2\n"))
	if node != nil {
		t.Fatalf("got non-nil node %v for a second document", node)
	}
	if err == nil {
		t.Fatal("got nil error for a second document")
	}
	if err.Kind != KindParseType {
		t.Fatalf("err.Kind = %v, want KindParseType", err.Kind)
	}
	if err.Path != "cfg.yml" {
		t.Fatalf("err.Path = %q, want %q", err.Path, "cfg.yml")
	}

	// The second document ("b: 2") starts at the "---" separator, line 2.
	const wantLine = 2
	gotLine := lineNumber(t, err.Detail)
	if gotLine != wantLine {
		t.Fatalf("err.Detail line = %d, want %d", gotLine, wantLine)
	}
}

func TestParseYAMLDocumentTrailingBareDashesRejected(t *testing.T) {
	// yaml.v3 decodes a trailing bare "---" as a second, null document -
	// not as a harmless end-of-stream marker for the first document.
	node, err := parseYAMLDocument("cfg.yml", []byte("a: 1\n---\n"))
	if node != nil {
		t.Fatalf("got non-nil node %v for a trailing bare ---", node)
	}
	if err == nil {
		t.Fatal("got nil error for a trailing bare ---")
	}
	if err.Kind != KindParseType {
		t.Fatalf("err.Kind = %v, want KindParseType", err.Kind)
	}
	gotLine := lineNumber(t, err.Detail)
	if gotLine <= 0 {
		t.Fatalf("err.Detail line = %d, want a positive line number", gotLine)
	}
}

func TestParseYAMLDocumentMalformedSecondDocument(t *testing.T) {
	node, err := parseYAMLDocument("cfg.yml", []byte("a: 1\n---\nb: [1,\n"))
	if node != nil {
		t.Fatalf("got non-nil node %v for a malformed second document", node)
	}
	if err == nil {
		t.Fatal("got nil error for a malformed second document")
	}
	if err.Kind != KindParseType {
		t.Fatalf("err.Kind = %v, want KindParseType", err.Kind)
	}
	if err.Path != "cfg.yml" {
		t.Fatalf("err.Path = %q, want %q", err.Path, "cfg.yml")
	}
	if err.Err == nil {
		t.Fatal("err.Err is nil, want the second document's yaml parser error")
	}
	if !strings.Contains(err.Err.Error(), "line ") {
		t.Fatalf("err.Err.Error() = %q, want it to contain \"line \"", err.Err.Error())
	}
	wantLine := lineNumber(t, err.Err.Error())
	gotLine := lineNumber(t, err.Detail)
	if gotLine != wantLine {
		t.Fatalf("err.Detail line = %d, err.Err line = %d, want equal", gotLine, wantLine)
	}
}

func TestParseYAMLDocumentReturnedErrorUnwraps(t *testing.T) {
	_, err := parseYAMLDocument("cfg.yml", []byte("foo: [1, 2\n"))
	if err == nil {
		t.Fatal("got nil error for malformed input")
	}
	if !errors.Is(error(err), err.Err) {
		t.Fatalf("errors.Is(err, err.Err) = false, want true")
	}
}
