package problems

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadDetailsFixture(t *testing.T) {
	details, err := readDetails(fixturePath("snosi", "details.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 17 {
		t.Fatalf("details = %d entries, want 17", len(details))
	}
	file := fixtureSystem(t)
	applyDetails(file.Items, details)

	unit := itemByID(t, file.Items, "a6d8d298c66718de")
	if len(unit.Local.Units) != 1 || unit.Local.Units[0] != "brian-backup.service" {
		t.Errorf("units = %v", unit.Local.Units)
	}
	if len(unit.Local.Messages) != 2 || !strings.Contains(unit.Local.Messages[0], "/home/brian") {
		t.Errorf("messages = %v", unit.Local.Messages)
	}
	if len(unit.Local.Values) != 3 || unit.Local.Values[2].Name != "unit" || unit.Local.Values[2].Display != "brian-backup.service" {
		t.Errorf("local values = %+v", unit.Local.Values)
	}
	crash := itemByID(t, file.Items, "658d8b6145a2afc2")
	if crash.Local.ExePath != "/opt/brian/tool" {
		t.Errorf("exe path = %q", crash.Local.ExePath)
	}
	if got := Title(crash); got != "tool stopped unexpectedly" {
		t.Errorf("title from the local exe path = %q", got)
	}
}

func TestReadDetailsOutcomes(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		path    string
		wantErr string
		wantLen int
	}{
		{"missing", filepath.Join(dir, "absent.json"), "", 0},
		{"bad JSON", writeTestFile(t, dir, "bad.json", `{`), "not a JSON object", 0},
		{"format 2", writeTestFile(t, dir, "f2.json", `{"format": 2, "problems": {}}`), "format is not 1", 0},
		{"problems list", writeTestFile(t, dir, "list.json", `{"format": 1, "problems": []}`), "problems is not an object", 0},
		{"too large", writeTestFile(t, dir, "big.json", `{"format": 1, "problems": {}}`+strings.Repeat(" ", MaxDetailsBytes)), "larger than", 0},
		{"directory", dir, "is a directory", 0},
		{"bad ids dropped", writeTestFile(t, dir, "ids.json", `{"format": 1, "problems": {"x": {"kind": "crash"}, "00000000000000aa": "no", "00000000000000bb": {"kind": "crash"}}}`), "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readDetails(tt.path)
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if len(got) != tt.wantLen {
				t.Errorf("entries = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

// TestReadDetailsPermissionDenied: details.json is mode 0640 root:sudo; a
// user outside sudo gets EACCES, which is normal: no details, no error.
func TestReadDetailsPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 0000 file")
	}
	path := writeTestFile(t, t.TempDir(), "details.json", `{"format": 1, "problems": {}}`)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	got, err := readDetails(path)
	if err != nil || got != nil {
		t.Fatalf("readDetails = %v, %v; want nil, nil", got, err)
	}
}

func TestDetailsLimits(t *testing.T) {
	msgs := make([]string, 7)
	for i := range msgs {
		msgs[i] = `"` + strings.Repeat("m", 2000) + `"`
	}
	units := make([]string, 20)
	for i := range units {
		units[i] = `"u.service"`
	}
	doc := `{"format": 1, "problems": {"00000000000000aa": {"kind": "unit_failed", "local": {"unit": "x\u0000y"},
"extra": {"units": [` + strings.Join(units, ",") + `], "exe_path": 5}, "messages": [` + strings.Join(msgs, ",") + `, 3]}}}`
	details, err := parseDetails([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	local := details["00000000000000aa"].local
	if len(local.Messages) != maxMessages {
		t.Errorf("messages = %d, want %d", len(local.Messages), maxMessages)
	}
	if n := len([]rune(local.Messages[0])); n != maxMessageLen+1 {
		t.Errorf("message has %d characters, want %d and an ellipsis", n, maxMessageLen+1)
	}
	if len(local.Units) != maxUnits || local.ExePath != "" {
		t.Errorf("units = %d, exe path = %q", len(local.Units), local.ExePath)
	}
	if strings.ContainsRune(local.Values[0].Display, 0) {
		t.Error("a NUL character reached the display")
	}
}

func TestApplyDetailsNeedsMatchingKind(t *testing.T) {
	items := []Item{{ID: "00000000000000aa", Kind: KindCrash}}
	applyDetails(items, map[string]localEntry{"00000000000000aa": {kind: KindUnitFailed, local: Local{ExePath: "/x"}}})
	if items[0].Local.ExePath != "" {
		t.Error("details of another kind were applied")
	}
}
