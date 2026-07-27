package config

import (
	"bytes"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// parseYAMLDocument decodes data as YAML and returns its single document
// tree. It is a pure parsing step: it never opens, reads, or otherwise
// touches the filesystem itself — path is only ever copied into a returned
// *LoadError for caller-side context.
//
// Empty or whitespace-only input is not an error: it returns (nil, nil),
// matching the "absent config" semantics the rest of this package already
// uses for a missing file. A document that fails to parse returns a
// KindParseType *LoadError wrapping the real yaml parser error, with Detail
// set to that error's own message (which yaml.v3 always renders as
// "yaml: line N: ...", so the reported line is visible in Detail without a
// second look at Err).
//
// Exactly one YAML document is accepted. After the first document decodes
// successfully, only io.EOF from a second Decode proves there is no more
// input; anything else means a second document is present — including a
// second document that itself decodes without error, such as the null
// document yaml.v3 synthesises for a trailing bare "---". That case, and a
// malformed second document, both return KindParseType too: the former
// names the second document's own starting line since there is no parser
// error to wrap, and the latter preserves that document's parser error and
// line exactly like a malformed first document would.
func parseYAMLDocument(path string, data []byte) (*yaml.Node, *LoadError) {
	dec := yaml.NewDecoder(bytes.NewReader(data))

	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if err == io.EOF {
			// No documents at all: empty or whitespace-only input.
			return nil, nil
		}
		return nil, parseFailure(path, err)
	}

	var extra yaml.Node
	switch err := dec.Decode(&extra); err {
	case io.EOF:
		// No second document: exactly one document, as required.
		return &doc, nil
	case nil:
		// A second document decoded successfully - still one too many.
		return nil, extraDocumentFailure(path, &extra)
	default:
		// A second document that is itself malformed YAML.
		return nil, parseFailure(path, err)
	}
}

// parseFailure wraps a yaml parser error into a KindParseType *LoadError,
// copying path verbatim and rendering the parser error's own message (which
// always contains "line N") into Detail.
func parseFailure(path string, err error) *LoadError {
	return &LoadError{
		Path:   path,
		Kind:   KindParseType,
		Detail: err.Error(),
		Err:    err,
	}
}

// extraDocumentFailure reports that a second, well-formed YAML document
// follows a first one that already decoded successfully. There is no
// parser error to wrap here - extra is valid YAML - so Err is left nil and
// Detail instead names the line the second document starts at.
func extraDocumentFailure(path string, extra *yaml.Node) *LoadError {
	return &LoadError{
		Path: path,
		Kind: KindParseType,
		Detail: fmt.Sprintf(
			"only a single YAML document is supported, but a second document starts at line %d",
			extra.Line,
		),
	}
}
