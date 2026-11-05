package problems

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNewReportID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := NewReportID()
		if err != nil {
			t.Fatal(err)
		}
		if !reportIDRe.MatchString(id) || seen[id] {
			t.Fatalf("NewReportID() = %q: not a new lowercase UUIDv4", id)
		}
		seen[id] = true
	}
	if got := formatUUIDv4([16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}); got != "ffffffff-ffff-4fff-bfff-ffffffffffff" {
		t.Errorf("formatUUIDv4(all ones) = %s", got)
	}
	if got := formatUUIDv4([16]byte{}); got != "00000000-0000-4000-8000-000000000000" {
		t.Errorf("formatUUIDv4(zero) = %s", got)
	}
}

func TestClient(t *testing.T) {
	tests := []struct {
		version string
		ok      bool
	}{
		{"0.14.0", true},
		{"1.2.3-rc.1", true},
		{"dev", false},
		{"v0.14.0", false},
		{"abc1234-snapshot", false},
		{"0.14.0+build", false},
	}
	for _, tt := range tests {
		client, ok := Client(tt.version)
		if ok != tt.ok || client != "chairlift/"+tt.version {
			t.Errorf("Client(%q) = %q, %v; want ok %v", tt.version, client, ok, tt.ok)
		}
		if ValidSemver(tt.version) != tt.ok {
			t.Errorf("ValidSemver(%q) = %v", tt.version, !tt.ok)
		}
	}
}

// TestBuildPayloadCopiesOnlySchemaFields builds the payload of the crash
// item that has local details and checks that no local text enters it.
func TestBuildPayloadCopiesOnlySchemaFields(t *testing.T) {
	file := fixtureSystem(t)
	details, err := readDetails(fixturePath("snosi", "details.json"))
	if err != nil {
		t.Fatal(err)
	}
	applyDetails(file.Items, details)
	for _, it := range file.Items {
		if !it.Reportable {
			continue
		}
		data, err := BuildPayload(it, "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "chairlift/0.14.0")
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, local := range []string{"brian", "frosty-desk", "/usr/bin", "Esys", "user@1000"} {
			if strings.Contains(text, local) {
				t.Errorf("payload of %s holds local text %q: %s", it.ID, local, text)
			}
		}
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc) != 8 {
			t.Errorf("payload has %d top-level keys, want 8", len(doc))
		}
	}
	crash := itemByID(t, file.Items, "7353e332c0c70754")
	data, err := BuildPayload(crash, "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "snosi-health/20261105100000")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema":"frostyard.problem/1","report_id":"3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b","week":"2026-W45",` +
		`"image":{"id":"snow","version":"20261105100000"},"secure":true,"client":"snosi-health/20261105100000",` +
		`"kind":"crash","detail":{"exe":"gnome-shell","package":"gnome-shell","package_version":"48.4-1","signal":"SIGSEGV"}}`
	if string(data) != want {
		t.Errorf("payload =\n%s\nwant\n%s", data, want)
	}
}

func TestBuildPayloadErrors(t *testing.T) {
	file := fixtureSystem(t)
	good := itemByID(t, file.Items, "8d1b483b03cda85a")
	const id = "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b"
	if _, err := BuildPayload(good, id, "chairlift/0.14.0"); err != nil {
		t.Fatalf("good item: %v", err)
	}
	notReportable := good
	notReportable.Reportable = false
	badEnvelope := good
	badEnvelope.Envelope.Valid = false
	badWeek := good
	badWeek.WeekValid = false
	badField := good
	badField.Fields = append([]Field(nil), good.Fields...)
	badField.Fields[0].Valid = false
	tests := []struct {
		name           string
		item           Item
		reportID, cl   string
		notReportable  bool
		wantErrContain string
	}{
		{"not reportable", notReportable, id, "chairlift/0.14.0", true, ""},
		{"bad envelope", badEnvelope, id, "chairlift/0.14.0", true, ""},
		{"bad week", badWeek, id, "chairlift/0.14.0", true, ""},
		{"bad field", badField, id, "chairlift/0.14.0", true, ""},
		{"uppercase report id", good, strings.ToUpper(id), "chairlift/0.14.0", false, "report id"},
		{"UUIDv1 report id", good, "3f2a1b4c-5d6e-1f70-8a9b-0c1d2e3f4a5b", "chairlift/0.14.0", false, "report id"},
		{"dev client", good, id, "chairlift/dev", false, "client"},
		{"empty client", good, id, "", false, "client"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildPayload(tt.item, tt.reportID, tt.cl)
			if tt.notReportable && !errors.Is(err, ErrNotReportable) {
				t.Fatalf("err = %v, want ErrNotReportable", err)
			}
			if !tt.notReportable && (err == nil || !strings.Contains(err.Error(), tt.wantErrContain)) {
				t.Fatalf("err = %v, want %q", err, tt.wantErrContain)
			}
		})
	}
}

func TestBuildPayloadSizeBound(t *testing.T) {
	frames := make([]string, 32)
	for i := range frames {
		frames[i] = "main." + strings.Repeat("a", 155)
	}
	it := Item{
		Kind: KindAppProblem, KnownKind: true, Reportable: true, Week: "2026-W45", WeekValid: true,
		Envelope: Envelope{ImageID: "floe", ImageVersion: "20261031093822", Valid: true},
		Fields: []Field{
			{Name: "app_id", Value: "org.frostyard.Frostnotes", Valid: true},
			{Name: "app_version", Value: "999999.999999.999999-alpha.9999", Valid: true},
			{Name: "runtime", Value: "go", Valid: true},
			{Name: "component", Value: "daemon", Valid: true},
			{Name: "class", Value: "panic", Valid: true},
			{Name: "frames", Value: frames, Valid: true},
		},
	}
	data, err := BuildPayload(it, "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "chairlift/0.14.0")
	if err != nil || len(data) >= maxPayloadBytes {
		t.Fatalf("the largest S8 payload: %d bytes, %v", len(data), err)
	}
	frames = append(frames, strings.Repeat("x", maxPayloadBytes))
	it.Fields[5].Value = frames
	if _, err := BuildPayload(it, "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "chairlift/0.14.0"); err == nil {
		t.Fatal("a payload over 8 KB was built")
	}
}
