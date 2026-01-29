package adwutil

import (
	"errors"
	"strings"
	"testing"
)

func TestUserError_Error(t *testing.T) {
	err := &UserError{Summary: "Couldn't install Firefox"}
	if err.Error() != "Couldn't install Firefox" {
		t.Errorf("Error() = %q, want %q", err.Error(), "Couldn't install Firefox")
	}
}

func TestUserError_Unwrap(t *testing.T) {
	underlying := errors.New("network timeout")
	err := &UserError{Technical: underlying}

	if err.Unwrap() != underlying {
		t.Error("Unwrap() did not return Technical error")
	}

	// Verify errors.Is works
	if !errors.Is(err, underlying) {
		t.Error("errors.Is should find underlying error")
	}
}

func TestUserError_Unwrap_Nil(t *testing.T) {
	err := &UserError{Summary: "Error without technical details"}

	if err.Unwrap() != nil {
		t.Error("Unwrap() should return nil when Technical is nil")
	}
}

func TestUserError_FormatForUser(t *testing.T) {
	tests := map[string]struct {
		err  *UserError
		want string
	}{
		"summary only": {
			err:  &UserError{Summary: "Couldn't install Firefox"},
			want: "Couldn't install Firefox",
		},
		"with hint": {
			err:  &UserError{Summary: "Couldn't connect", Hint: "Check your internet"},
			want: "Couldn't connect: Check your internet",
		},
		"empty hint": {
			err:  &UserError{Summary: "Error", Hint: ""},
			want: "Error",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.err.FormatForUser()
			if got != tc.want {
				t.Errorf("FormatForUser() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUserError_FormatWithDetails(t *testing.T) {
	tests := map[string]struct {
		err     *UserError
		contain string
	}{
		"without technical": {
			err:     &UserError{Summary: "Error"},
			contain: "Error",
		},
		"with technical": {
			err:     &UserError{Summary: "Error", Technical: errors.New("timeout")},
			contain: "Details: timeout",
		},
		"with hint and technical": {
			err:     &UserError{Summary: "Error", Hint: "Try again", Technical: errors.New("timeout")},
			contain: "Details: timeout",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.err.FormatWithDetails()
			if !strings.Contains(got, tc.contain) {
				t.Errorf("FormatWithDetails() = %q, want to contain %q", got, tc.contain)
			}
		})
	}
}

func TestUserError_FormatWithDetails_IncludesUserMessage(t *testing.T) {
	err := &UserError{
		Summary:   "Couldn't connect",
		Hint:      "Check internet",
		Technical: errors.New("dial timeout"),
	}

	got := err.FormatWithDetails()

	// Should contain the user message
	if !strings.Contains(got, "Couldn't connect: Check internet") {
		t.Errorf("FormatWithDetails() = %q, should contain user message", got)
	}

	// Should contain the technical details
	if !strings.Contains(got, "Details: dial timeout") {
		t.Errorf("FormatWithDetails() = %q, should contain technical details", got)
	}
}

func TestNewUserError(t *testing.T) {
	underlying := errors.New("network error")
	err := NewUserError("Couldn't connect", underlying)

	if err.Summary != "Couldn't connect" {
		t.Errorf("Summary = %q, want %q", err.Summary, "Couldn't connect")
	}
	if err.Technical != underlying {
		t.Error("Technical not set correctly")
	}
	if err.Hint != "" {
		t.Errorf("Hint = %q, want empty", err.Hint)
	}
}

func TestNewUserErrorWithHint(t *testing.T) {
	underlying := errors.New("network error")
	err := NewUserErrorWithHint("Couldn't connect", "Check internet", underlying)

	if err.Summary != "Couldn't connect" {
		t.Errorf("Summary = %q, want %q", err.Summary, "Couldn't connect")
	}
	if err.Hint != "Check internet" {
		t.Errorf("Hint = %q, want %q", err.Hint, "Check internet")
	}
	if err.Technical != underlying {
		t.Error("Technical not set correctly")
	}
}

func TestNewUserError_NilTechnical(t *testing.T) {
	err := NewUserError("Couldn't load data", nil)

	if err.Summary != "Couldn't load data" {
		t.Errorf("Summary = %q, want %q", err.Summary, "Couldn't load data")
	}
	if err.Technical != nil {
		t.Error("Technical should be nil")
	}
	if err.Hint != "" {
		t.Errorf("Hint = %q, want empty", err.Hint)
	}
}

func TestUserError_SatisfiesErrorInterface(t *testing.T) {
	// Compile-time check that UserError satisfies error interface
	var _ error = &UserError{}

	// Runtime check
	err := &UserError{Summary: "test"}
	var errInterface error = err
	if errInterface.Error() != "test" {
		t.Error("UserError should satisfy error interface")
	}
}
