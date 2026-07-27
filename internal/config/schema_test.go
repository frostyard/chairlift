package config

import (
	"reflect"
	"testing"
)

// TestSchemaPagesMatchesPageNames asserts SchemaPages() equals the
// hand-written pageNames slice already declared in config_test.go — an
// independent authority — element by element (same length, same order), and
// that a counting map shows each page name exactly once.
func TestSchemaPagesMatchesPageNames(t *testing.T) {
	got, err := SchemaPages()
	if err != nil {
		t.Fatalf("SchemaPages(): %v", err)
	}

	if len(got) != len(pageNames) {
		t.Fatalf("SchemaPages() has %d pages, want %d: got %v, want %v", len(got), len(pageNames), got, pageNames)
	}
	for i := range pageNames {
		if got[i] != pageNames[i] {
			t.Errorf("SchemaPages()[%d] = %q, want %q", i, got[i], pageNames[i])
		}
	}

	counts := make(map[string]int, len(got))
	for _, name := range got {
		counts[name]++
	}
	for _, name := range pageNames {
		if counts[name] != 1 {
			t.Errorf("SchemaPages(): page %q appears %d times, want exactly 1", name, counts[name])
		}
	}
}

// TestSchemaGroupsMatchesDefaultConfigForEveryPage loops over every page
// from SchemaPages(), calls SchemaGroups(page), and asserts the result is
// exactly the key set of pagesOf(defaultConfig())[page]: equal lengths,
// every default group present exactly once, and no name returned that
// defaultConfig() does not define.
func TestSchemaGroupsMatchesDefaultConfigForEveryPage(t *testing.T) {
	pages, err := SchemaPages()
	if err != nil {
		t.Fatalf("SchemaPages(): %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("SchemaPages() returned no pages")
	}

	defPages := pagesOf(defaultConfig())

	for _, page := range pages {
		groups, err := SchemaGroups(page)
		if err != nil {
			t.Fatalf("SchemaGroups(%q): %v", page, err)
		}
		if len(groups) == 0 {
			t.Errorf("SchemaGroups(%q) returned no groups", page)
		}

		wantGroups := defPages[page]

		if len(groups) != len(wantGroups) {
			t.Errorf("SchemaGroups(%q) has %d groups, want %d: got %v, want keys %v", page, len(groups), len(wantGroups), groups, groupKeys(wantGroups))
		}

		counts := make(map[string]int, len(groups))
		for _, name := range groups {
			counts[name]++
		}

		for name := range wantGroups {
			if counts[name] != 1 {
				t.Errorf("SchemaGroups(%q): expected group %q to appear exactly once, got %d", page, name, counts[name])
			}
		}
		for name := range counts {
			if _, ok := wantGroups[name]; !ok {
				t.Errorf("SchemaGroups(%q): returned group %q not defined by defaultConfig()", page, name)
			}
		}
	}
}

// TestSchemaGroupsUnknownPageReturnsError asserts SchemaGroups on an unknown
// page returns a non-nil error and a nil slice.
func TestSchemaGroupsUnknownPageReturnsError(t *testing.T) {
	got, err := SchemaGroups("no_such_page")
	if err == nil {
		t.Fatal("SchemaGroups(\"no_such_page\"): expected error, got nil")
	}
	if got != nil {
		t.Errorf("SchemaGroups(\"no_such_page\") = %v, want nil slice", got)
	}
}

// TestSchemaPagesReturnsFreshSlice mutates the slice returned by
// SchemaPages() and asserts a subsequent call is unaffected.
func TestSchemaPagesReturnsFreshSlice(t *testing.T) {
	first, err := SchemaPages()
	if err != nil {
		t.Fatalf("SchemaPages(): %v", err)
	}
	if len(first) == 0 {
		t.Fatal("SchemaPages() returned no pages")
	}

	original := make([]string, len(first))
	copy(original, first)

	for i := range first {
		first[i] = "MUTATED"
	}

	second, err := SchemaPages()
	if err != nil {
		t.Fatalf("SchemaPages(): %v", err)
	}
	if !reflect.DeepEqual(second, original) {
		t.Errorf("SchemaPages() second call = %v, want unaffected %v", second, original)
	}
}

// TestSchemaGroupsReturnsFreshSlice mutates the slice returned by
// SchemaGroups(page) and asserts a subsequent call is unaffected.
func TestSchemaGroupsReturnsFreshSlice(t *testing.T) {
	const page = "system_page"

	first, err := SchemaGroups(page)
	if err != nil {
		t.Fatalf("SchemaGroups(%q): %v", page, err)
	}
	if len(first) == 0 {
		t.Fatal("SchemaGroups(system_page) returned no groups")
	}

	original := make([]string, len(first))
	copy(original, first)

	for i := range first {
		first[i] = "MUTATED"
	}

	second, err := SchemaGroups(page)
	if err != nil {
		t.Fatalf("SchemaGroups(%q): %v", page, err)
	}
	if !reflect.DeepEqual(second, original) {
		t.Errorf("SchemaGroups(%q) second call = %v, want unaffected %v", page, second, original)
	}
}

// TestSchemaPageGroupsReturnsFreshMap mutates the map returned by
// schemaPageGroups() (both by adding a key and by mutating one of its
// slices) and asserts a subsequent call is unaffected.
func TestSchemaPageGroupsReturnsFreshMap(t *testing.T) {
	first, err := schemaPageGroups()
	if err != nil {
		t.Fatalf("schemaPageGroups(): %v", err)
	}
	if len(first) == 0 {
		t.Fatal("schemaPageGroups() returned no pages")
	}

	var somePage string
	for page := range first {
		somePage = page
		break
	}

	originalGroups := make([]string, len(first[somePage]))
	copy(originalGroups, first[somePage])

	// Mutate the map itself and one of its slice values.
	first["INJECTED_PAGE"] = []string{"injected"}
	if len(first[somePage]) > 0 {
		first[somePage][0] = "MUTATED"
	}

	second, err := schemaPageGroups()
	if err != nil {
		t.Fatalf("schemaPageGroups(): %v", err)
	}
	if _, ok := second["INJECTED_PAGE"]; ok {
		t.Error("schemaPageGroups(): second call reflects injected key from a previously mutated map")
	}
	if !reflect.DeepEqual(second[somePage], originalGroups) {
		t.Errorf("schemaPageGroups()[%q] second call = %v, want unaffected %v", somePage, second[somePage], originalGroups)
	}
}

// TestYamlTagName is a direct-call table test for yamlTagName.
func TestYamlTagName(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{"plain name", "system_page", "system_page"},
		{"name with omitempty", "app_id,omitempty", "app_id"},
		{"only options, no name", ",omitempty", ""},
		{"empty tag", "", ""},
		{"dash", "-", "-"},
		{"multiple options", "chat,omitempty,flow", "chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := yamlTagName(tt.tag); got != tt.want {
				t.Errorf("yamlTagName(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

// Test-only struct types for yamlFieldNames, covering each documented
// skip/error case.
type yamlFieldNamesOKStruct struct {
	Website string `yaml:"website"`
	unex    string `yaml:"unexported"` //nolint:unused // exercises PkgPath != "" skip
	NoTag   string
	Dashed  string `yaml:"-"`
	Chat    string `yaml:"chat,omitempty"`
}

type yamlFieldNamesEmptyNameStruct struct {
	Bad string `yaml:",omitempty"`
}

type yamlFieldNamesDuplicateStruct struct {
	A string `yaml:"same"`
	B string `yaml:"same"`
}

// TestYamlFieldNames is a direct-call table test for yamlFieldNames,
// covering the documented skip rules (unexported, untagged, yaml:"-") and
// the documented error cases (non-struct type, empty name after option
// stripping, duplicate names).
func TestYamlFieldNames(t *testing.T) {
	t.Run("skips unexported, untagged, and dashed fields; keeps tagged ones in order", func(t *testing.T) {
		got, err := yamlFieldNames(reflect.TypeOf(yamlFieldNamesOKStruct{}))
		if err != nil {
			t.Fatalf("yamlFieldNames: unexpected error: %v", err)
		}
		want := []string{"website", "chat"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("yamlFieldNames = %v, want %v", got, want)
		}
	})

	t.Run("non-struct type errors", func(t *testing.T) {
		got, err := yamlFieldNames(reflect.TypeOf(42))
		if err == nil {
			t.Fatal("yamlFieldNames(int): expected error, got nil")
		}
		if got != nil {
			t.Errorf("yamlFieldNames(int) = %v, want nil", got)
		}
	})

	t.Run("empty name after option stripping errors", func(t *testing.T) {
		got, err := yamlFieldNames(reflect.TypeOf(yamlFieldNamesEmptyNameStruct{}))
		if err == nil {
			t.Fatal("yamlFieldNames: expected error for yaml:\",omitempty\", got nil")
		}
		if got != nil {
			t.Errorf("yamlFieldNames = %v, want nil", got)
		}
	})

	t.Run("duplicate yaml name errors", func(t *testing.T) {
		got, err := yamlFieldNames(reflect.TypeOf(yamlFieldNamesDuplicateStruct{}))
		if err == nil {
			t.Fatal("yamlFieldNames: expected error for duplicate yaml names, got nil")
		}
		if got != nil {
			t.Errorf("yamlFieldNames = %v, want nil", got)
		}
	})
}

// TestSchemaPageGroups is a direct-call test for schemaPageGroups, asserting
// it succeeds against the real defaultConfig() and its result matches
// pagesOf(defaultConfig()) exactly (sorted).
func TestSchemaPageGroups(t *testing.T) {
	got, err := schemaPageGroups()
	if err != nil {
		t.Fatalf("schemaPageGroups(): %v", err)
	}
	if len(got) == 0 {
		t.Fatal("schemaPageGroups() returned no pages")
	}

	defPages := pagesOf(defaultConfig())
	for page, groups := range got {
		wantGroups := defPages[page]
		if len(groups) != len(wantGroups) {
			t.Errorf("schemaPageGroups()[%q] has %d groups, want %d", page, len(groups), len(wantGroups))
		}
		for _, name := range groups {
			if _, ok := wantGroups[name]; !ok {
				t.Errorf("schemaPageGroups()[%q]: group %q not in defaultConfig()", page, name)
			}
		}
		// Assert sorted order.
		for i := 1; i < len(groups); i++ {
			if groups[i-1] > groups[i] {
				t.Errorf("schemaPageGroups()[%q] not sorted: %v", page, groups)
				break
			}
		}
	}
}

// TestRawConfigMatchesConfigFields proves rawConfig's exported fields match
// Config's exactly: same field count, same field names in the same order,
// same yaml tag names in the same order. This is the parity test required
// by docs/agents/skills/derive-schema-from-canonical-struct-not-shadow-representation.md
// — rawConfig is a decoding mirror held to Config by this test, never a
// second schema authority (the derivation in this file reads Config /
// defaultConfig(), never rawConfig).
func TestRawConfigMatchesConfigFields(t *testing.T) {
	configType := reflect.TypeOf(Config{})
	rawType := reflect.TypeOf(rawConfig{})

	if configType.NumField() != rawType.NumField() {
		t.Fatalf("Config has %d fields, rawConfig has %d", configType.NumField(), rawType.NumField())
	}

	for i := 0; i < configType.NumField(); i++ {
		cf := configType.Field(i)
		rf := rawType.Field(i)

		if cf.Name != rf.Name {
			t.Errorf("field %d: Config field name %q, rawConfig field name %q", i, cf.Name, rf.Name)
		}

		cTag := yamlTagName(cf.Tag.Get("yaml"))
		rTag := yamlTagName(rf.Tag.Get("yaml"))
		if cTag != rTag {
			t.Errorf("field %d (%s): Config yaml tag %q, rawConfig yaml tag %q", i, cf.Name, cTag, rTag)
		}
	}
}
