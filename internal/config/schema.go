package config

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// yamlTagName returns the part of a yaml struct tag before the first comma,
// so tag options such as "omitempty" are stripped. An empty tag (or a tag
// consisting only of options, e.g. ",omitempty") yields an empty string.
func yamlTagName(tag string) string {
	if i := strings.Index(tag, ","); i >= 0 {
		return tag[:i]
	}
	return tag
}

// yamlFieldNames returns the yaml field names of t's exported fields, in
// struct declaration order, applying this fixed extraction rule per field:
//   - skip unexported fields (PkgPath != "")
//   - skip fields with no yaml tag or an empty tag
//   - skip yaml:"-"
//   - error if yamlTagName of a non-empty tag is empty (e.g. yaml:",omitempty")
//   - error on a duplicate name
//
// It also errors when t is not a struct kind. The returned slice is freshly
// allocated on every call.
func yamlFieldNames(t reflect.Type) ([]string, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("yamlFieldNames: %s is not a struct", t.Kind())
	}

	seen := make(map[string]bool)
	names := make([]string, 0, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}

		tag, ok := field.Tag.Lookup("yaml")
		if !ok || tag == "" {
			continue // no yaml tag
		}
		if tag == "-" {
			continue // explicitly excluded from YAML
		}

		name := yamlTagName(tag)
		if name == "" {
			return nil, fmt.Errorf("yamlFieldNames: field %s has a yaml tag with an empty name (%q)", field.Name, tag)
		}
		if seen[name] {
			return nil, fmt.Errorf("yamlFieldNames: duplicate yaml name %q (field %s)", name, field.Name)
		}
		seen[name] = true
		names = append(names, name)
	}

	return names, nil
}

// schemaPageGroups reflects over the value returned by defaultConfig(): for
// each exported Config field it takes the page name from the field's yaml
// tag and the group names from that field's PageConfig map, erroring on an
// empty group name or a duplicate page name. It returns a fresh map of
// freshly allocated, lexicographically sorted slices on every call (map
// iteration order is not deterministic, so sorting is required for a stable
// API).
func schemaPageGroups() (map[string][]string, error) {
	def := defaultConfig()
	v := reflect.ValueOf(*def)
	t := v.Type()

	result := make(map[string][]string, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}

		tag, ok := field.Tag.Lookup("yaml")
		if !ok || tag == "" || tag == "-" {
			continue
		}
		pageName := yamlTagName(tag)
		if pageName == "" {
			return nil, fmt.Errorf("schemaPageGroups: field %s has a yaml tag with an empty name (%q)", field.Name, tag)
		}
		if _, dup := result[pageName]; dup {
			return nil, fmt.Errorf("schemaPageGroups: duplicate page name %q (field %s)", pageName, field.Name)
		}

		page, ok := v.Field(i).Interface().(PageConfig)
		if !ok {
			return nil, fmt.Errorf("schemaPageGroups: field %s is not a PageConfig", field.Name)
		}

		groups := make([]string, 0, len(page))
		for name := range page {
			if name == "" {
				return nil, fmt.Errorf("schemaPageGroups: page %q has an empty group name", pageName)
			}
			groups = append(groups, name)
		}
		sort.Strings(groups)

		result[pageName] = groups
	}

	return result, nil
}

// SchemaPages returns the canonical page names, sourced from the yaml tags
// on Config's exported fields, in struct declaration order. It returns a
// freshly allocated slice on every call, so callers cannot mutate shared
// state.
func SchemaPages() ([]string, error) {
	return yamlFieldNames(reflect.TypeOf(Config{}))
}

// SchemaGroups returns the canonical group names for page, sourced from that
// page's map in defaultConfig(). It returns a non-nil error for a page name
// that is not in SchemaPages(). It returns a freshly allocated slice on
// every call, so callers cannot mutate shared state.
func SchemaGroups(page string) ([]string, error) {
	pageGroups, err := schemaPageGroups()
	if err != nil {
		return nil, err
	}

	groups, ok := pageGroups[page]
	if !ok {
		return nil, fmt.Errorf("SchemaGroups: unknown page %q", page)
	}

	// schemaPageGroups already returns freshly allocated slices per call,
	// but copy defensively so this function's contract does not depend on
	// that implementation detail.
	result := make([]string, len(groups))
	copy(result, groups)
	return result, nil
}

// SchemaGroupFields returns the canonical group-field names, sourced from
// rawGroupConfig's yaml tags in struct declaration order. rawGroupConfig,
// not GroupConfig, is used because it is the representation yaml.v3 actually
// decodes into; a parity test in schema_test.go holds rawGroupConfig's yaml
// tags to GroupConfig's, so the two cannot silently drift apart. It returns
// a freshly allocated slice on every call, so callers cannot mutate shared
// state.
func SchemaGroupFields() ([]string, error) {
	return yamlFieldNames(reflect.TypeOf(rawGroupConfig{}))
}

// SchemaActionFields returns the canonical action-field names, sourced from
// ActionConfig's yaml tags, in struct declaration order. It returns a
// freshly allocated slice on every call, so callers cannot mutate shared
// state.
func SchemaActionFields() ([]string, error) {
	return yamlFieldNames(reflect.TypeOf(ActionConfig{}))
}
