package homebrew

import (
	"errors"
	"reflect"
	"testing"
)

// installedInfoJSON is a trimmed `brew info --installed --json=v2` document
// carrying every field parsePackagesJSON reads: an installed formula with two
// installed versions, a formula that is known to brew but not installed, and
// two casks. Both namespaces are present in the same document because brew
// always emits both keys, and each parse direction must ignore the other.
const installedInfoJSON = `{
  "formulae": [
    {
      "name": "ripgrep",
      "versions": {"stable": "14.1.1"},
      "installed": [
        {"version": "14.1.0", "installed_on_request": true},
        {"version": "13.0.0", "installed_on_request": false}
      ],
      "pinned": true,
      "outdated": true
    },
    {
      "name": "not-installed",
      "versions": {"stable": "1.0.0"},
      "installed": [],
      "pinned": false,
      "outdated": false
    },
    {
      "name": "jq",
      "versions": {"stable": "1.7.1"},
      "installed": [{"version": "1.7.1", "installed_on_request": false}],
      "pinned": false,
      "outdated": false
    }
  ],
  "casks": [
    {"token": "firefox", "version": "142.0", "installed": "141.0", "outdated": true},
    {"token": "obsidian", "version": "1.8.10", "installed": "1.8.10", "outdated": false}
  ]
}`

func TestParsePackagesJSONReadsInstalledFormulae(t *testing.T) {
	got, err := parsePackagesJSON(installedInfoJSON, true)
	if err != nil {
		t.Fatalf("parsePackagesJSON returned error: %v", err)
	}

	want := []Package{
		{Name: "ripgrep", Version: "14.1.0", InstalledOnRequest: true, Pinned: true, Outdated: true},
		{Name: "jq", Version: "1.7.1", InstalledOnRequest: false, Pinned: false, Outdated: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePackagesJSON(formulae) = %+v, want %+v", got, want)
	}
}

// A formula brew reports without any installed entry must be dropped rather
// than surfaced as an installed package with an empty version.
func TestParsePackagesJSONSkipsFormulaeWithNoInstalledEntry(t *testing.T) {
	got, err := parsePackagesJSON(installedInfoJSON, true)
	if err != nil {
		t.Fatalf("parsePackagesJSON returned error: %v", err)
	}

	for _, pkg := range got {
		if pkg.Name == "not-installed" {
			t.Fatalf("parsePackagesJSON returned uninstalled formula %q: %+v", pkg.Name, got)
		}
	}
}

func TestParsePackagesJSONReadsInstalledCasks(t *testing.T) {
	got, err := parsePackagesJSON(installedInfoJSON, false)
	if err != nil {
		t.Fatalf("parsePackagesJSON returned error: %v", err)
	}

	// Casks carry their name in "token" and their installed version in
	// "installed"; brew reports no pin or on-request state for them.
	want := []Package{
		{Name: "firefox", Version: "141.0", Outdated: true},
		{Name: "obsidian", Version: "1.8.10", Outdated: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePackagesJSON(casks) = %+v, want %+v", got, want)
	}
}

// Each parse direction must read only its own namespace, so a document
// containing both never leaks casks into the formula listing or vice versa.
func TestParsePackagesJSONIgnoresTheOtherNamespace(t *testing.T) {
	formulae, err := parsePackagesJSON(installedInfoJSON, true)
	if err != nil {
		t.Fatalf("parsePackagesJSON(formulae) returned error: %v", err)
	}
	casks, err := parsePackagesJSON(installedInfoJSON, false)
	if err != nil {
		t.Fatalf("parsePackagesJSON(casks) returned error: %v", err)
	}

	for _, pkg := range formulae {
		if pkg.Name == "firefox" || pkg.Name == "obsidian" {
			t.Errorf("formula listing contains cask %q", pkg.Name)
		}
	}
	for _, pkg := range casks {
		if pkg.Name == "ripgrep" || pkg.Name == "jq" {
			t.Errorf("cask listing contains formula %q", pkg.Name)
		}
	}
}

func TestParsePackagesJSONEmptyDocumentReturnsNoPackages(t *testing.T) {
	for _, tc := range []struct {
		name      string
		isFormula bool
	}{
		{name: "formulae", isFormula: true},
		{name: "casks", isFormula: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePackagesJSON(`{"formulae": [], "casks": []}`, tc.isFormula)
			if err != nil {
				t.Fatalf("parsePackagesJSON returned error: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("parsePackagesJSON = %+v, want no packages", got)
			}
		})
	}
}

// Unparseable brew output must fail closed with a homebrew.Error rather than
// returning a partially populated listing, so callers can keep their last
// known good rows instead of rendering an invented empty inventory.
func TestParsePackagesJSONMalformedOutputReturnsError(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
	}{
		{name: "truncated object", data: `{"formulae": [`},
		{name: "not json", data: "Error: brew is not installed"},
		{name: "empty output", data: ""},
		{name: "wrong shape", data: `{"formulae": {"name": "ripgrep"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePackagesJSON(tc.data, true)
			if err == nil {
				t.Fatalf("parsePackagesJSON(%q) succeeded with %+v, want error", tc.data, got)
			}
			if got != nil {
				t.Errorf("parsePackagesJSON(%q) returned packages %+v alongside error, want nil", tc.data, got)
			}

			var brewErr *Error
			if !errors.As(err, &brewErr) {
				t.Fatalf("parsePackagesJSON(%q) error is %T, want *homebrew.Error", tc.data, err)
			}
			if brewErr.Message == "" {
				t.Error("homebrew.Error carries an empty message")
			}
		})
	}
}
