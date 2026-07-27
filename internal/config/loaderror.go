package config

import "strings"

// ErrorKind classifies why loading or validating a configuration file
// failed. The string values are part of the stable diagnostic vocabulary
// surfaced by LoadError.Error() and must not change.
type ErrorKind string

const (
	// KindRead marks a failure to read the config file from disk (e.g. a
	// permissions error or an I/O failure other than "file does not
	// exist").
	KindRead ErrorKind = "read"
	// KindParseType marks a failure to parse the file as YAML or to
	// decode a value into its expected Go type.
	KindParseType ErrorKind = "parse/type"
	// KindSchema marks a failure detected by shape/semantic validation
	// after the document parsed successfully — e.g. an unknown key or a
	// value of the right YAML kind but the wrong shape. A KindSchema
	// error may legitimately carry a nil Err.
	KindSchema ErrorKind = "schema"
)

// LoadError describes a single configuration load/validation failure.
//
// Path is the config file path involved, if any. Kind is the stable
// classification above. Detail is a short human-readable description of
// what went wrong. Err is the underlying cause, if any; KindSchema errors
// detected purely by shape inspection may leave Err nil.
type LoadError struct {
	Path   string
	Kind   ErrorKind
	Detail string
	Err    error
}

// Error renders the diagnostic fields in a fixed, documented layout:
//
//  1. "config <kind> error" when Kind is set, otherwise "config error";
//  2. ": <Path>" appended when Path is non-empty;
//  3. ": <Detail>" appended when Detail is non-empty;
//  4. ": <Err.Error()>" appended when Err is non-nil.
func (e *LoadError) Error() string {
	var b strings.Builder
	if e.Kind != "" {
		b.WriteString("config ")
		b.WriteString(string(e.Kind))
		b.WriteString(" error")
	} else {
		b.WriteString("config error")
	}
	if e.Path != "" {
		b.WriteString(": ")
		b.WriteString(e.Path)
	}
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// Unwrap returns the underlying cause, enabling errors.Is/errors.As to see
// through a *LoadError to Err (which may be nil when there is no cause).
func (e *LoadError) Unwrap() error {
	return e.Err
}
