package problems

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The vendored schema: a byte copy of core's v1.json at this commit.
const (
	vendoredSchemaSHA256 = "6c053ee3a9b4fd399d1d011b2be833dc6b17f25e727161fc576b0ce64cc14c3a"
	vendoredCoreCommit   = "f1b62e0692adc1da6050c6f4733814f06b7476c6"
)

func TestVendoredSchemaIsPinned(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "schema", "v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != vendoredSchemaSHA256 {
		t.Fatalf("testdata/schema/v1.json sha256 = %s, want %s (core %s); an accepted schema never changes", got, vendoredSchemaSHA256, vendoredCoreCommit)
	}
	readme, err := os.ReadFile(filepath.Join("testdata", "schema", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{vendoredCoreCommit, vendoredSchemaSHA256} {
		if !strings.Contains(string(readme), want) {
			t.Errorf("testdata/schema/README.md does not record %s", want)
		}
	}
	v := loadSchema(t)
	if title := v.Document()["title"]; title != SchemaID {
		t.Errorf("schema title = %v, want %s", title, SchemaID)
	}
}

// TestEnumsMatchSchema compares every enum of the package with the enum of
// the same field in v1.json.
func TestEnumsMatchSchema(t *testing.T) {
	v := loadSchema(t)
	if got := schemaEnum(t, v, "", "kind"); !reflect.DeepEqual(got, Kinds) {
		t.Errorf("Kinds = %v, schema kind enum = %v", Kinds, got)
	}
	if got := schemaEnum(t, v, "", "image"); !reflect.DeepEqual(got, imageIDs) {
		t.Errorf("imageIDs = %v, schema = %v", imageIDs, got)
	}
	for _, kind := range Kinds {
		def, ok := v.Def(kind)
		if !ok {
			t.Fatalf("schema has no $defs entry %s", kind)
		}
		props := def["properties"].(map[string]any)
		rules := detailRules[kind]
		if len(rules) != len(props) {
			t.Errorf("%s: %d rules, schema has %d properties", kind, len(rules), len(props))
		}
		required := map[string]bool{}
		for _, r := range def["required"].([]any) {
			required[r.(string)] = true
		}
		for _, rule := range rules {
			prop, ok := props[rule.name].(map[string]any)
			if !ok {
				t.Errorf("%s.%s is not in the schema", kind, rule.name)
				continue
			}
			if rule.required != required[rule.name] {
				t.Errorf("%s.%s required = %v, schema %v", kind, rule.name, rule.required, required[rule.name])
			}
			if rule.enum != nil {
				if got := schemaEnum(t, v, kind, rule.name); !reflect.DeepEqual(got, rule.enum) {
					t.Errorf("%s.%s enum = %v, schema = %v", kind, rule.name, rule.enum, got)
				}
			} else if _, has := prop["enum"]; has {
				t.Errorf("%s.%s has a schema enum but no rule enum", kind, rule.name)
			}
			if rule.pattern != nil {
				if got := schemaPattern(t, v, kind, rule.name); got != rule.pattern.String() {
					t.Errorf("%s.%s pattern = %s, schema = %s", kind, rule.name, rule.pattern, got)
				}
			}
		}
	}
}

// TestPatternsMatchSchema compares the envelope patterns with v1.json.
func TestPatternsMatchSchema(t *testing.T) {
	v := loadSchema(t)
	for _, tt := range []struct {
		def, property, got string
	}{
		{"", "report_id", reportIDRe.String()},
		{"", "week", weekRe.String()},
		{"", "client", clientRe.String()},
		{"image_version", "", imageVersionRe.String()},
		{"semver", "", semverRe.String()},
	} {
		if want := schemaPattern(t, v, tt.def, tt.property); tt.got != want {
			t.Errorf("%s.%s pattern = %s, schema = %s", tt.def, tt.property, tt.got, want)
		}
	}
}

func readExamples(t *testing.T) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "schema", "examples", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != len(Kinds) {
		t.Fatalf("found %d examples, want 1 for each of the %d kinds", len(paths), len(Kinds))
	}
	out := map[string][]byte{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[strings.TrimSuffix(filepath.Base(p), ".json")] = data
	}
	return out
}

// TestValidatorAcceptsEveryExample runs the subset validator over core's 9
// examples: each is valid, and its kind is its file name.
func TestValidatorAcceptsEveryExample(t *testing.T) {
	v := loadSchema(t)
	for name, data := range readExamples(t) {
		t.Run(name, func(t *testing.T) {
			doc := decodeJSONNumber(t, data)
			if errs := validate(t, v, doc); len(errs) > 0 {
				t.Fatalf("example %s is not valid: %v", name, errs)
			}
			if kind := doc.(map[string]any)["kind"]; kind != name {
				t.Errorf("example %s has kind %v", name, kind)
			}
		})
	}
}

// TestValidatorRejectsNegativeCases proves the validator enforces each rule
// kind that BuildPayload's outputs rely on, so its acceptance of a payload
// means something.
func TestValidatorRejectsNegativeCases(t *testing.T) {
	v := loadSchema(t)
	examples := readExamples(t)
	mutate := func(example string, change func(doc map[string]any)) any {
		doc := decodeJSONNumber(t, examples[example]).(map[string]any)
		change(doc)
		return doc
	}
	detail := func(doc map[string]any) map[string]any { return doc["detail"].(map[string]any) }
	frames := func(n int, frame string) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = frame
		}
		return out
	}
	cases := map[string]any{
		"extra top-level field":   mutate("count", func(d map[string]any) { d["hostname"] = "frosty" }),
		"missing envelope field":  mutate("count", func(d map[string]any) { delete(d, "secure") }),
		"wrong schema":            mutate("count", func(d map[string]any) { d["schema"] = "frostyard.problem/2" }),
		"uppercase report id":     mutate("count", func(d map[string]any) { d["report_id"] = strings.ToUpper(d["report_id"].(string)) }),
		"week with newline":       mutate("count", func(d map[string]any) { d["week"] = "2026-W45\n" }),
		"week 54":                 mutate("count", func(d map[string]any) { d["week"] = "2026-W54" }),
		"image id off the enum":   mutate("count", func(d map[string]any) { d["image"].(map[string]any)["id"] = "ubuntu" }),
		"image extra field":       mutate("count", func(d map[string]any) { d["image"].(map[string]any)["arch"] = "x86_64" }),
		"null image":              mutate("count", func(d map[string]any) { d["image"] = nil }),
		"client with v":           mutate("count", func(d map[string]any) { d["client"] = "chairlift/v0.14.0" }),
		"secure as text":          mutate("count", func(d map[string]any) { d["secure"] = "true" }),
		"unknown kind":            mutate("count", func(d map[string]any) { d["kind"] = "disk_full" }),
		"detail of another kind":  mutate("count", func(d map[string]any) { detail(d)["result"] = "other" }),
		"count enum":              mutate("count", func(d map[string]any) { detail(d)["install_age"] = "1y" }),
		"unit not shipped":        mutate("unit_failed", func(d map[string]any) { detail(d)["shipped"] = false }),
		"unit with instance path": mutate("unit_failed", func(d map[string]any) { detail(d)["unit"] = "home-brian.mount/x" }),
		"retries 11":              mutate("tpm_unlock", func(d map[string]any) { detail(d)["retries"] = json.Number("11") }),
		"retries 1.5":             mutate("tpm_unlock", func(d map[string]any) { detail(d)["retries"] = json.Number("1.5") }),
		"integrity count 11":      mutate("integrity_error", func(d map[string]any) { detail(d)["count"] = "11" }),
		"crash exe with path":     mutate("crash", func(d map[string]any) { detail(d)["exe"] = "/usr/bin/gnome-shell" }),
		"crash missing signal":    mutate("crash", func(d map[string]any) { delete(detail(d), "signal") }),
		"app id off the list":     mutate("app_problem", func(d map[string]any) { detail(d)["app_id"] = "org.example.App" }),
		"panic with a code":       mutate("app_problem", func(d map[string]any) { detail(d)["code"] = "x.y" }),
		"error without code": mutate("app_problem", func(d map[string]any) {
			detail(d)["class"] = "error"
			delete(detail(d), "frames")
		}),
		"error with frames":   mutate("app_problem", func(d map[string]any) { detail(d)["class"] = "error"; detail(d)["code"] = "x.y" }),
		"go with location":    mutate("app_problem", func(d map[string]any) { detail(d)["location"] = "src/main.rs:1" }),
		"rust with frames":    mutate("app_problem", func(d map[string]any) { detail(d)["runtime"] = "rust" }),
		"33 frames":           mutate("app_problem", func(d map[string]any) { detail(d)["frames"] = frames(33, "main.main") }),
		"no frames":           mutate("app_problem", func(d map[string]any) { detail(d)["frames"] = []any{} }),
		"frame of 161":        mutate("app_problem", func(d map[string]any) { detail(d)["frames"] = []any{"main." + strings.Repeat("a", 156)} }),
		"frame outside list":  mutate("app_problem", func(d map[string]any) { detail(d)["frames"] = []any{"github.com/evil/x.F"} }),
		"semver with v":       mutate("app_problem", func(d map[string]any) { detail(d)["app_version"] = "v0.9.2" }),
		"sysext extra field":  mutate("sysext_update_failed", func(d map[string]any) { detail(d)["origin"] = "image" }),
		"stage bootc version": mutate("update_stage_failed", func(d map[string]any) { detail(d)["bootc_version"] = "1.1" }),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if errs := validate(t, v, doc); len(errs) == 0 {
				t.Fatalf("validator accepted %s", name)
			}
		})
	}
}

// TestBuildPayloadOutputsValidate builds the payload of every reportable
// item of the snosi fixture and of the user fixtures, and validates each
// against v1.json. It also requires at least 1 reportable item of each
// kind that the fixtures hold.
func TestBuildPayloadOutputsValidate(t *testing.T) {
	v := loadSchema(t)
	items := reportableFixtureItems(t)
	if len(items) == 0 {
		t.Fatal("no reportable fixture item")
	}
	kinds := map[string]bool{}
	for _, item := range items {
		reportID, err := NewReportID()
		if err != nil {
			t.Fatal(err)
		}
		data, err := BuildPayload(item, reportID, "chairlift/0.14.0")
		if err != nil {
			t.Fatalf("BuildPayload(%s %s): %v", item.Kind, item.ID, err)
		}
		if errs := validate(t, v, decodeJSONNumber(t, data)); len(errs) > 0 {
			t.Errorf("payload of %s %s is not valid: %v\n%s", item.Kind, item.ID, errs, data)
		}
		kinds[item.Kind] = true
	}
	for _, kind := range []string{KindUnitFailed, KindUpdateStageFailed, KindBootedOlderImage, KindSysextUpdateFailed,
		KindTPMUnlock, KindIntegrityError, KindCrash, KindAppProblem} {
		if !kinds[kind] {
			t.Errorf("no reportable fixture item of kind %s", kind)
		}
	}
}

// TestValidatorRejectsAPayloadKeyOutsideTheSchema proves that a key that is
// not in the schema, added to a real payload, fails validation.
func TestValidatorRejectsAPayloadKeyOutsideTheSchema(t *testing.T) {
	v := loadSchema(t)
	for _, item := range reportableFixtureItems(t) {
		data, err := BuildPayload(item, "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "snosi-health/20261105100000")
		if err != nil {
			t.Fatal(err)
		}
		doc := decodeJSONNumber(t, data).(map[string]any)
		doc["detail"].(map[string]any)["message"] = "local text"
		if errs := validate(t, v, doc); len(errs) == 0 {
			t.Errorf("a payload of %s with a detail key outside the schema was accepted", item.Kind)
		}
	}
}
