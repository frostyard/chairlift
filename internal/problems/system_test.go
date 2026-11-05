package problems

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// systemDoc returns a problems.json with a valid envelope and problems.
func systemDoc(problems ...string) string {
	return `{"format": 1, "schema": "frostyard.problem/1", "generated": "2026-11-05T03:56:38Z",
"image": {"id": "floe", "version": "20261031093822"}, "secure": false,
"client": "snosi-health/20261031093822", "extra_top_level": {"ignored": true},
"problems": [` + strings.Join(problems, ",") + `]}`
}

// problemJSON returns 1 problems[] entry that the file calls reportable.
func problemJSON(id, kind, detail string) string {
	return fmt.Sprintf(`{"id": %q, "kind": %q, "detail": %s, "week": "2026-W45",
"first_seen": "2026-11-05T01:00:00Z", "last_seen": "2026-11-05T02:00:00Z", "occurrences": 3, "reportable": true}`, id, kind, detail)
}

func TestReadSystemOutcomes(t *testing.T) {
	dir := t.TempDir()
	big := systemDoc(problemJSON("0000000000000001", "count", `{"install_age": "<1w", "channel": "latest"}`)) +
		strings.Repeat(" ", MaxProblemsBytes)
	tests := []struct {
		name string
		path string
		want SourceOutcome
		err  string
	}{
		{"valid", fixturePath("snosi", "problems.json"), SourceOK, ""},
		{"missing", filepath.Join(dir, "absent.json"), SourceMissing, ""},
		{"unreadable directory", dir, SourceUnreadable, "is a directory"},
		{"bad JSON", writeTestFile(t, dir, "bad.json", `{"format": 1,`), SourceMalformed, "not a JSON object"},
		{"not an object", writeTestFile(t, dir, "array.json", `[]`), SourceMalformed, "not a JSON object"},
		{"trailing data", writeTestFile(t, dir, "trailing.json", systemDoc()+" {}"), SourceMalformed, "not a JSON object"},
		{"format 2", writeTestFile(t, dir, "format.json", strings.Replace(systemDoc(), `"format": 1`, `"format": 2`, 1)), SourceMalformed, "format is not 1"},
		{"no format", writeTestFile(t, dir, "noformat.json", strings.Replace(systemDoc(), `"format": 1,`, ``, 1)), SourceMalformed, "format is not 1"},
		{"format as text", writeTestFile(t, dir, "format-text.json", strings.Replace(systemDoc(), `"format": 1`, `"format": "1"`, 1)), SourceMalformed, "format is not 1"},
		{"format 1.0", writeTestFile(t, dir, "format-float.json", strings.Replace(systemDoc(), `"format": 1`, `"format": 1.0`, 1)), SourceMalformed, "format is not 1"},
		{"wrong schema", writeTestFile(t, dir, "schema.json", strings.Replace(systemDoc(), "frostyard.problem/1", "frostyard.problem/2", 1)), SourceMalformed, "schema is not"},
		{"problems not a list", writeTestFile(t, dir, "list.json", `{"format": 1, "schema": "frostyard.problem/1", "problems": {}}`), SourceMalformed, "problems is not a list"},
		{"problems null", writeTestFile(t, dir, "null.json", `{"format": 1, "schema": "frostyard.problem/1", "problems": null}`), SourceMalformed, "problems is not a list"},
		{"too large", writeTestFile(t, dir, "large.json", big), SourceMalformed, "larger than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadSystem(tt.path)
			if got.Outcome != tt.want {
				t.Fatalf("outcome = %s (%v), want %s", got.Outcome, got.Err, tt.want)
			}
			if tt.err == "" && got.Err != nil {
				t.Errorf("err = %v, want nil", got.Err)
			}
			if tt.err != "" && (got.Err == nil || !strings.Contains(got.Err.Error(), tt.err)) {
				t.Errorf("err = %v, want %q", got.Err, tt.err)
			}
			if got.Outcome != SourceOK && len(got.Items) != 0 {
				t.Errorf("outcome %s has %d items", got.Outcome, len(got.Items))
			}
		})
	}
}

func TestSourceOutcomeNames(t *testing.T) {
	want := map[SourceOutcome]string{
		SourceMissing:     "missing",
		SourceUnreadable:  "unreadable",
		SourceMalformed:   "malformed",
		SourceOK:          "ok",
		SourceOutcome(99): "unknown",
	}
	for outcome, name := range want {
		if got := outcome.String(); got != name {
			t.Errorf("SourceOutcome(%d).String() = %q, want %q", outcome, got, name)
		}
	}
	if SourceMissing != 0 {
		t.Error("the zero SourceOutcome must be SourceMissing")
	}
}

// TestSnosiFixtureItems checks every item of the real snosi output: 17
// items, the envelope, and which items ChairLift finds reportable.
func TestSnosiFixtureItems(t *testing.T) {
	file := fixtureSystem(t)
	if len(file.Items) != 17 || file.Dropped != 0 {
		t.Fatalf("items = %d, dropped = %d; want 17 and 0", len(file.Items), file.Dropped)
	}
	wantEnv := Envelope{ImageID: "snow", ImageVersion: "20261105100000", Secure: true, Valid: true}
	if file.Envelope != wantEnv {
		t.Errorf("envelope = %+v, want %+v", file.Envelope, wantEnv)
	}
	if file.Client != "snosi-health/20261105100000" {
		t.Errorf("client = %q", file.Client)
	}
	if want := time.Date(2026, 11, 5, 3, 56, 38, 0, time.UTC); !file.Generated.Equal(want) {
		t.Errorf("generated = %v, want %v", file.Generated, want)
	}
	notReportable := map[string]string{
		"e1e8cd1cde14da30": "unit",    // unit_failed, unit not shipped
		"a6d8d298c66718de": "unit",    // unit_failed, unit not shipped
		"658d8b6145a2afc2": "exe",     // crash outside /usr
		"19c19a6303a39c9a": "version", // sysext without a version
	}
	for _, it := range file.Items {
		missing, local := notReportable[it.ID]
		if it.Reportable == local {
			t.Errorf("%s (%s) reportable = %v, want %v", it.ID, it.Kind, it.Reportable, !local)
		}
		if it.FileReportable != !local {
			t.Errorf("%s file reportable = %v", it.ID, it.FileReportable)
		}
		if local && (len(it.Missing) == 0 || it.Missing[0] != missing) {
			t.Errorf("%s missing = %v, want %s first", it.ID, it.Missing, missing)
		}
		if !it.KnownKind || it.Origin != OriginSystem || it.Count != 1 || !it.WeekValid {
			t.Errorf("%s = %+v", it.ID, it)
		}
	}
	// Newest last_seen first.
	for i := 1; i < len(file.Items); i++ {
		if file.Items[i].LastSeen.After(file.Items[i-1].LastSeen) {
			t.Fatalf("items are not sorted newest first at %d", i)
		}
	}
}

// TestForgedValuesAreShownButNotReportable gives problems.json a value that
// fails its v1 rule for each rule kind, in an item the file calls
// reportable. ChairLift shows the item with that field marked not valid
// and does not trust the file's flag.
func TestForgedValuesAreShownButNotReportable(t *testing.T) {
	tests := []struct {
		name, kind, detail, field string
	}{
		{"unit pattern", KindUnitFailed, `{"unit": "home/brian.mount", "result": "timeout", "shipped": true}`, "unit"},
		{"result enum", KindUnitFailed, `{"unit": "a.service", "result": "crashed", "shipped": true}`, "result"},
		{"shipped false", KindUnitFailed, `{"unit": "a.service", "result": "timeout", "shipped": false}`, "shipped"},
		{"shipped as text", KindUnitFailed, `{"unit": "a.service", "result": "timeout", "shipped": "true"}`, "shipped"},
		{"result null", KindUnitFailed, `{"unit": "a.service", "result": null, "shipped": true}`, "result"},
		{"target version", KindUpdateStageFailed, `{"target_version": "2026", "error_class": "lock", "bootc_version": "1.1.6"}`, "target_version"},
		{"bootc semver", KindUpdateStageFailed, `{"target_version": "20261104081517", "error_class": "lock", "bootc_version": "v1.1.6"}`, "bootc_version"},
		{"how enum", KindBootedOlderImage, `{"newer_version": "20261104081517", "how": "magic"}`, "how"},
		{"feature pattern", KindSysextUpdateFailed, `{"feature": "/home/brian", "version": "1", "outcome": "failed"}`, "feature"},
		{"sysext error class", KindSysextUpdateFailed, `{"feature": "docker", "version": "1", "outcome": "failed", "error_class": "dns"}`, "error_class"},
		{"retries 11", KindTPMUnlock, `{"outcome": "recovery_key", "tpm_rc": "LOCKOUT", "retries": 11}`, "retries"},
		{"retries -1", KindTPMUnlock, `{"outcome": "recovery_key", "tpm_rc": "LOCKOUT", "retries": -1}`, "retries"},
		{"retries 1.5", KindTPMUnlock, `{"outcome": "recovery_key", "tpm_rc": "LOCKOUT", "retries": 1.5}`, "retries"},
		{"retries as text", KindTPMUnlock, `{"outcome": "recovery_key", "tpm_rc": "LOCKOUT", "retries": "1"}`, "retries"},
		{"retries as text 0", KindTPMUnlock, `{"outcome": "recovery_key", "tpm_rc": "LOCKOUT", "retries": "0"}`, "retries"},
		{"retries as boolean", KindTPMUnlock, `{"outcome": "recovery_key", "tpm_rc": "LOCKOUT", "retries": true}`, "retries"},
		{"retries as list", KindTPMUnlock, `{"outcome": "recovery_key", "tpm_rc": "LOCKOUT", "retries": [1]}`, "retries"},
		{"retries null", KindTPMUnlock, `{"outcome": "recovery_key", "tpm_rc": "LOCKOUT", "retries": null}`, "retries"},
		{"shipped as number", KindUnitFailed, `{"unit": "a.service", "result": "timeout", "shipped": 1}`, "shipped"},
		{"unit as number", KindUnitFailed, `{"unit": 5, "result": "timeout", "shipped": true}`, "unit"},
		{"frames as text", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "go", "component": "gui", "class": "panic", "frames": "main.main"}`, "frames"},
		{"tpm rc", KindTPMUnlock, `{"outcome": "recovery_key", "tpm_rc": "pcr_changed", "retries": 1}`, "tpm_rc"},
		{"subsystem", KindIntegrityError, `{"subsystem": "btrfs", "count": "1"}`, "subsystem"},
		{"count 11", KindIntegrityError, `{"subsystem": "composefs", "count": "11"}`, "count"},
		{"exe path", KindCrash, `{"exe": "/usr/bin/x", "signal": "SIGSEGV"}`, "exe"},
		{"package", KindCrash, `{"exe": "x", "signal": "SIGSEGV", "package": "X"}`, "package"},
		{"package version", KindCrash, `{"exe": "x", "signal": "SIGSEGV", "package_version": "a1"}`, "package_version"},
		{"signal", KindCrash, `{"exe": "x", "signal": "SIGKILL"}`, "signal"},
		{"app id", KindAppProblem, `{"app_id": "org.example.App", "app_version": "1.0.0", "runtime": "go", "component": "gui", "class": "panic"}`, "app_id"},
		{"app version", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0", "runtime": "go", "component": "gui", "class": "panic"}`, "app_version"},
		{"component", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "go", "component": "tray", "class": "panic"}`, "component"},
		{"code pattern", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "go", "component": "gui", "class": "error", "code": "Bad Code"}`, "code"},
		{"panic with code", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "go", "component": "gui", "class": "panic", "code": "a.b"}`, "code"},
		{"error with frames", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "go", "component": "gui", "class": "error", "code": "a.b", "frames": ["main.main"]}`, "frames"},
		{"go with location", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "go", "component": "gui", "class": "panic", "location": "src/a.rs:1"}`, "location"},
		{"rust with frames", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "rust", "component": "gui", "class": "panic", "frames": ["main.main"]}`, "frames"},
		{"frame path", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "go", "component": "gui", "class": "panic", "frames": ["/home/brian/x.go"]}`, "frames"},
		{"frame too long", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "go", "component": "gui", "class": "panic", "frames": ["main.` + strings.Repeat("a", 160) + `"]}`, "frames"},
		{"33 frames", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "go", "component": "gui", "class": "panic", "frames": [` + strings.TrimSuffix(strings.Repeat(`"main.main",`, 33), ",") + `]}`, "frames"},
		{"empty frames", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "go", "component": "gui", "class": "panic", "frames": []}`, "frames"},
		{"location pattern", KindAppProblem, `{"app_id": "org.frostyard.nsl", "app_version": "1.0.0", "runtime": "rust", "component": "gui", "class": "panic", "location": "src/../main.rs:1"}`, "location"},
		{"install age", KindCount, `{"install_age": "1y", "channel": "latest"}`, "install_age"},
		{"extra field", KindCount, `{"install_age": "<1w", "channel": "latest", "hostname": "frosty"}`, "hostname"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := ParseSystem([]byte(systemDoc(problemJSON("00000000000000aa", tt.kind, tt.detail))))
			if file.Outcome != SourceOK || len(file.Items) != 1 {
				t.Fatalf("outcome %s, %d items; want ok and 1", file.Outcome, len(file.Items))
			}
			it := file.Items[0]
			if it.Reportable {
				t.Fatalf("an item with a forged %s is reportable", tt.field)
			}
			if !it.FileReportable {
				t.Fatal("the file's reportable flag was lost")
			}
			f, ok := it.Field(tt.field)
			if !ok {
				t.Fatalf("field %s is not shown: %+v", tt.field, it.Fields)
			}
			if f.Valid || f.Value != nil || f.Reason == "" {
				t.Errorf("field %s = %+v, want not valid with a reason and no value", tt.field, f)
			}
			if _, err := BuildPayload(it, "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "chairlift/0.14.0"); err != ErrNotReportable {
				t.Errorf("BuildPayload err = %v, want ErrNotReportable", err)
			}
		})
	}
}

// TestValidDetailOfEachKindIsReportable proves the forged cases fail for
// their value alone: the same items with valid values are reportable.
func TestValidDetailOfEachKindIsReportable(t *testing.T) {
	for _, kind := range Kinds {
		t.Run(kind, func(t *testing.T) {
			data, err := os.ReadFile(fixturePath("schema", "examples", kind+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Detail json.RawMessage `json:"detail"`
			}
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			file := ParseSystem([]byte(systemDoc(problemJSON("00000000000000bb", kind, string(doc.Detail)))))
			if len(file.Items) != 1 || !file.Items[0].Reportable {
				t.Fatalf("items = %+v, want 1 reportable item", file.Items)
			}
		})
	}
}

func TestReportableNeedsFileFlagWeekAndEnvelope(t *testing.T) {
	detail := `{"install_age": "<1w", "channel": "latest"}`
	tests := []struct {
		name string
		doc  string
	}{
		{"file says not reportable", systemDoc(strings.Replace(problemJSON("0000000000000001", "count", detail), `"reportable": true`, `"reportable": false`, 1))},
		{"no reportable flag", systemDoc(strings.Replace(problemJSON("0000000000000001", "count", detail), `, "reportable": true`, ``, 1))},
		{"bad week", systemDoc(strings.Replace(problemJSON("0000000000000001", "count", detail), "2026-W45", "2026-45", 1))},
		{"null image", strings.Replace(systemDoc(problemJSON("0000000000000001", "count", detail)), `{"id": "floe", "version": "20261031093822"}`, `null`, 1)},
		{"null image id", strings.Replace(systemDoc(problemJSON("0000000000000001", "count", detail)), `"id": "floe"`, `"id": null`, 1)},
		{"image id off the enum", strings.Replace(systemDoc(problemJSON("0000000000000001", "count", detail)), `"id": "floe"`, `"id": "ubuntu"`, 1)},
		{"bad image version", strings.Replace(systemDoc(problemJSON("0000000000000001", "count", detail)), `"version": "20261031093822"`, `"version": "1.0"`, 1)},
		{"secure missing", strings.Replace(systemDoc(problemJSON("0000000000000001", "count", detail)), `"secure": false,`, ``, 1)},
		{"detail not an object", systemDoc(problemJSON("0000000000000001", "count", `"x"`))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := ParseSystem([]byte(tt.doc))
			if file.Outcome != SourceOK || len(file.Items) != 1 {
				t.Fatalf("outcome %s, %d items", file.Outcome, len(file.Items))
			}
			if file.Items[0].Reportable {
				t.Fatal("item is reportable")
			}
		})
	}
}

// TestUnknownKind: a kind that matches the kind name pattern but is not a
// v1 kind is shown as an unknown problem with its name and never
// reportable.
func TestUnknownKind(t *testing.T) {
	file := ParseSystem([]byte(systemDoc(problemJSON("00000000000000cc", "disk_full", `{"free": "1%"}`))))
	if len(file.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(file.Items))
	}
	it := file.Items[0]
	if it.KnownKind || it.Reportable || it.Kind != "disk_full" {
		t.Fatalf("item = %+v, want an unknown, not reportable disk_full", it)
	}
	if got := Title(it); got != "An unknown problem was recorded" {
		t.Errorf("title = %q", got)
	}
	if !strings.Contains(DetailsText(it), "Kind: disk_full") {
		t.Error("details do not name the unknown kind")
	}
	if _, err := BuildPayload(it, "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "chairlift/0.14.0"); err != ErrNotReportable {
		t.Errorf("BuildPayload err = %v", err)
	}
}

// TestDroppedProblems: an entry with a bad id, a kind that fails the kind
// name pattern, or a time that is not RFC 3339 is dropped and counted.
func TestDroppedProblems(t *testing.T) {
	good := problemJSON("00000000000000dd", "count", `{"install_age": "<1w", "channel": "latest"}`)
	bad := []string{
		`"not an object"`,
		strings.Replace(good, "00000000000000dd", "00000000000000DD", 1),
		strings.Replace(good, "00000000000000dd", "0000000000000dd", 1),
		strings.Replace(good, `"id": "00000000000000dd", `, ``, 1),
		strings.Replace(good, `"kind": "count"`, `"kind": "Count"`, 1),
		strings.Replace(good, `"kind": "count"`, `"kind": "../count"`, 1),
		strings.Replace(good, `"kind": "count"`, `"kind": 3`, 1),
		strings.Replace(good, "2026-11-05T01:00:00Z", "yesterday", 1),
		strings.Replace(good, "2026-11-05T02:00:00Z", "2026-11-05 02:00", 1),
		strings.Replace(good, `"first_seen": "2026-11-05T01:00:00Z", `, ``, 1),
	}
	file := ParseSystem([]byte(systemDoc(append([]string{good}, bad...)...)))
	if len(file.Items) != 1 || file.Dropped != len(bad) {
		t.Fatalf("items = %d, dropped = %d; want 1 and %d", len(file.Items), file.Dropped, len(bad))
	}
	if file.Items[0].Count != 3 {
		t.Errorf("count = %d, want 3", file.Items[0].Count)
	}
}

func TestSystemItemsAreCapped(t *testing.T) {
	entries := make([]string, MaxItems+5)
	for i := range entries {
		entries[i] = strings.Replace(
			problemJSON(fmt.Sprintf("%016x", i), "count", `{"install_age": "<1w", "channel": "latest"}`),
			"2026-11-05T02:00:00Z", time.Date(2026, 11, 5, 0, 0, i, 0, time.UTC).Format(time.RFC3339), 1)
	}
	file := ParseSystem([]byte(systemDoc(entries...)))
	if len(file.Items) != MaxItems || file.Truncated != 5 {
		t.Fatalf("items = %d, truncated = %d; want %d and 5", len(file.Items), file.Truncated, MaxItems)
	}
	if file.Items[0].ID != fmt.Sprintf("%016x", MaxItems+4) {
		t.Errorf("first item = %s, want the newest", file.Items[0].ID)
	}
}

func TestGeneratedAndClientOptional(t *testing.T) {
	doc := `{"format": 1, "schema": "frostyard.problem/1", "generated": "now", "image": {"id": null, "version": null},
"secure": false, "client": null, "problems": []}`
	file := ParseSystem([]byte(doc))
	if file.Outcome != SourceOK || !file.Generated.IsZero() || file.Client != "" || file.Envelope.Valid {
		t.Fatalf("file = %+v", file)
	}
}

func TestFieldDisplayIsCutAndCleaned(t *testing.T) {
	long := strings.Repeat("x", 500)
	file := ParseSystem([]byte(systemDoc(problemJSON("00000000000000ee", "unit_failed",
		`{"unit": "`+long+`", "result": "a\u0007b", "shipped": true}`))))
	it := file.Items[0]
	unit, _ := it.Field("unit")
	if n := len([]rune(unit.Display)); n != maxDisplay+1 || !strings.HasSuffix(unit.Display, "…") {
		t.Errorf("unit display has %d characters, want %d and an ellipsis", n, maxDisplay+1)
	}
	result, _ := it.Field("result")
	if strings.ContainsRune(result.Display, '\u0007') {
		t.Errorf("result display %q holds a control character", result.Display)
	}
}

func TestOriginNames(t *testing.T) {
	for origin, name := range map[Origin]string{
		OriginSystem: "system", OriginUserCrash: "user-crash", OriginAppRecord: "app-record", Origin(9): "unknown",
	} {
		if origin.String() != name {
			t.Errorf("%d.String() = %q, want %q", origin, origin.String(), name)
		}
	}
}

// TestQuotedNumbersAndBooleansAreNotTrusted: a number or boolean of the
// file given as JSON text is the wrong type. occurrences falls back to 1, a
// textual reportable flag or secure flag is not true, and the item is not
// reportable.
func TestQuotedNumbersAndBooleansAreNotTrusted(t *testing.T) {
	detail := `{"install_age": "<1w", "channel": "latest"}`
	good := problemJSON("0000000000000001", "count", detail)
	if file := ParseSystem([]byte(systemDoc(good))); !file.Items[0].Reportable || file.Items[0].Count != 3 {
		t.Fatalf("the control item = %+v, want reportable with count 3", file.Items[0])
	}
	occurrences := ParseSystem([]byte(systemDoc(strings.Replace(good, `"occurrences": 3`, `"occurrences": "3"`, 1)))).Items[0]
	if occurrences.Count != 1 {
		t.Errorf("a textual occurrences gave count %d, want 1", occurrences.Count)
	}
	for name, doc := range map[string]string{
		"reportable as text": systemDoc(strings.Replace(good, `"reportable": true`, `"reportable": "true"`, 1)),
		"reportable as 1":    systemDoc(strings.Replace(good, `"reportable": true`, `"reportable": 1`, 1)),
		"secure as text":     strings.Replace(systemDoc(good), `"secure": false`, `"secure": "false"`, 1),
		"secure as 0":        strings.Replace(systemDoc(good), `"secure": false`, `"secure": 0`, 1),
	} {
		it := ParseSystem([]byte(doc)).Items[0]
		if it.Reportable {
			t.Errorf("%s: the item is reportable", name)
		}
		if _, err := BuildPayload(it, "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "chairlift/0.14.0"); err != ErrNotReportable {
			t.Errorf("%s: BuildPayload err = %v", name, err)
		}
	}
}

func TestDecodeNumber(t *testing.T) {
	for raw, ok := range map[string]bool{`1`: true, `-3`: true, `1.5`: true, `"1"`: false, `true`: false, `null`: false, `[1]`: false, ``: false} {
		if _, got := decodeNumber(json.RawMessage(raw)); got != ok {
			t.Errorf("decodeNumber(%s) ok = %v, want %v", raw, got, ok)
		}
	}
}
