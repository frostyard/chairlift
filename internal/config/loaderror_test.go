package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestLoadErrorErrorExactStrings(t *testing.T) {
	cause := errors.New("permission denied")

	tests := []struct {
		name string
		err  *LoadError
		want string
	}{
		{
			name: "read kind with path detail and err",
			err: &LoadError{
				Path:   "/etc/chairlift/config.yml",
				Kind:   KindRead,
				Detail: "opening config file",
				Err:    cause,
			},
			want: "config read error: /etc/chairlift/config.yml: opening config file: permission denied",
		},
		{
			name: "parse type kind with path detail and err",
			err: &LoadError{
				Path:   "/etc/chairlift/config.yml",
				Kind:   KindParseType,
				Detail: "decoding page map",
				Err:    errors.New("yaml: line 3: mapping values are not allowed in this context"),
			},
			want: "config parse/type error: /etc/chairlift/config.yml: decoding page map: yaml: line 3: mapping values are not allowed in this context",
		},
		{
			name: "schema kind with path only and nil err",
			err: &LoadError{
				Path: "/etc/chairlift/config.yml",
				Kind: KindSchema,
			},
			want: "config schema error: /etc/chairlift/config.yml",
		},
		{
			name: "schema kind with path and detail and nil err",
			err: &LoadError{
				Path:   "/etc/chairlift/config.yml",
				Kind:   KindSchema,
				Detail: "unknown key \"bogus_group\"",
			},
			want: "config schema error: /etc/chairlift/config.yml: unknown key \"bogus_group\"",
		},
		{
			name: "empty path with detail and err",
			err: &LoadError{
				Kind:   KindRead,
				Detail: "opening config file",
				Err:    cause,
			},
			want: "config read error: opening config file: permission denied",
		},
		{
			name: "empty kind",
			err: &LoadError{
				Path:   "/etc/chairlift/config.yml",
				Detail: "something went wrong",
				Err:    cause,
			},
			want: "config error: /etc/chairlift/config.yml: something went wrong: permission denied",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.err.Error()
			if got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoadErrorContainsKindLiteral(t *testing.T) {
	for _, kind := range []ErrorKind{KindRead, KindParseType, KindSchema} {
		kind := kind
		t.Run(string(kind), func(t *testing.T) {
			err := &LoadError{Path: "/some/path", Kind: kind, Detail: "detail"}
			if !strings.Contains(err.Error(), string(kind)) {
				t.Errorf("Error() = %q, want it to contain kind literal %q", err.Error(), string(kind))
			}
		})
	}
}

func TestLoadErrorUnwrap(t *testing.T) {
	if got := (&LoadError{Err: nil}).Unwrap(); got != nil {
		t.Errorf("Unwrap() with nil Err = %v, want nil", got)
	}

	cause := errors.New("boom")
	if got := (&LoadError{Err: cause}).Unwrap(); got != cause {
		t.Errorf("Unwrap() = %v, want %v", got, cause)
	}
}

func TestLoadErrorErrorsIs(t *testing.T) {
	readErr := &LoadError{Kind: KindRead, Err: os.ErrNotExist}
	if !errors.Is(readErr, os.ErrNotExist) {
		t.Errorf("errors.Is(readErr, os.ErrNotExist) = false, want true")
	}

	schemaErr := &LoadError{Kind: KindSchema}
	if errors.Is(schemaErr, os.ErrNotExist) {
		t.Errorf("errors.Is(schemaErr, os.ErrNotExist) = true, want false")
	}
}

func TestLoadErrorErrorsAs(t *testing.T) {
	original := &LoadError{Path: "/etc/chairlift/config.yml", Kind: KindSchema, Detail: "unknown key"}
	wrapped := fmt.Errorf("wrapped: %w", original)

	var target *LoadError
	if !errors.As(wrapped, &target) {
		t.Fatalf("errors.As(wrapped, &target) = false, want true")
	}
	if target.Kind != original.Kind {
		t.Errorf("target.Kind = %q, want %q", target.Kind, original.Kind)
	}
}
