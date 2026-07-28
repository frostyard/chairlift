package homebrew

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestCommandTimeout(t *testing.T) {
	if len(stateChangingCommands) != 10 {
		t.Fatalf("stateChangingCommands has %d entries, want 10: update this test when the map changes", len(stateChangingCommands))
	}

	for cmd := range stateChangingCommands {
		t.Run("state-changing/"+cmd, func(t *testing.T) {
			if got := commandTimeout([]string{cmd, "somepkg"}); got != mutationTimeout {
				t.Errorf("commandTimeout(%q) = %v, want %v", cmd, got, mutationTimeout)
			}
		})
	}

	readCases := []struct {
		name string
		args []string
	}{
		{name: "read/outdated", args: []string{"outdated", "--json=v2"}},
		{name: "read/info", args: []string{"info", "--installed", "--json=v2", "--formula"}},
		{name: "read/search", args: []string{"search", "--formula", "ripgrep"}},
		{name: "empty args", args: nil},
	}

	for _, tc := range readCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandTimeout(tc.args); got != readTimeout {
				t.Errorf("commandTimeout(%v) = %v, want %v", tc.args, got, readTimeout)
			}
		})
	}
}

func TestSearchWithReturnsTypedFormulaeAndCasks(t *testing.T) {
	var calls [][]string
	run := func(args ...string) (string, error) {
		calls = append(calls, append([]string(nil), args...))
		switch args[1] {
		case "--formula":
			return "ripgrep\nripgrep-all\n", nil
		case "--cask":
			return "font-ripgrep\n", nil
		default:
			t.Fatalf("unexpected search flag in args %v", args)
			return "", nil
		}
	}

	got, err := searchWith(run, "  ripgrep  ")
	if err != nil {
		t.Fatalf("searchWith: %v", err)
	}
	want := []SearchResult{
		{Name: "ripgrep", Kind: Formula},
		{Name: "ripgrep-all", Kind: Formula},
		{Name: "font-ripgrep", Kind: Cask},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("searchWith results = %#v, want %#v", got, want)
	}

	wantCalls := [][]string{
		{"search", "--formula", "ripgrep"},
		{"search", "--cask", "ripgrep"},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Errorf("searchWith calls = %v, want %v", calls, wantCalls)
	}
}

func TestSearchWithTreatsNoMatchesAsEmptyCategory(t *testing.T) {
	noMatches := &Error{Message: `Brew command failed: Error: No formulae or casks found for "demo".`}
	run := func(args ...string) (string, error) {
		if args[1] == "--formula" {
			return "", noMatches
		}
		return "demo-app\n", nil
	}

	got, err := searchWith(run, "demo")
	if err != nil {
		t.Fatalf("searchWith: %v", err)
	}
	want := []SearchResult{{Name: "demo-app", Kind: Cask}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("searchWith results = %#v, want %#v", got, want)
	}
}

func TestSearchWithPropagatesOperationalErrors(t *testing.T) {
	wantErr := errors.New("brew unavailable")
	calls := 0
	run := func(args ...string) (string, error) {
		calls++
		return "", wantErr
	}

	if _, err := searchWith(run, "demo"); !errors.Is(err, wantErr) {
		t.Fatalf("searchWith error = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Errorf("searchWith calls = %d, want 1 after formula search failure", calls)
	}
}

func TestSearchWithEmptyQueryDoesNotRunBrew(t *testing.T) {
	run := func(args ...string) (string, error) {
		t.Fatalf("unexpected brew call: %v", args)
		return "", nil
	}
	got, err := searchWith(run, " \t ")
	if err != nil {
		t.Fatalf("searchWith: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("searchWith empty query = %#v, want no results", got)
	}
}

func TestParseSearchOutputFiltersHeadersAndBlankLines(t *testing.T) {
	got := parseSearchOutput("\n==> Formulae\nalpha\n\nbeta\n", Formula)
	want := []SearchResult{
		{Name: "alpha", Kind: Formula},
		{Name: "beta", Kind: Formula},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseSearchOutput = %#v, want %#v", got, want)
	}
}

func TestPackageKindDisplayName(t *testing.T) {
	if got := Formula.DisplayName(); got != "Formula" {
		t.Errorf("Formula.DisplayName() = %q, want Formula", got)
	}
	if got := Cask.DisplayName(); got != "Cask" {
		t.Errorf("Cask.DisplayName() = %q, want Cask", got)
	}
}

func TestTimeoutConstants(t *testing.T) {
	if readTimeout != 30*time.Second {
		t.Errorf("readTimeout = %v, want 30s", readTimeout)
	}
	if mutationTimeout != 30*time.Minute {
		t.Errorf("mutationTimeout = %v, want 30m", mutationTimeout)
	}
}
