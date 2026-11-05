package problems

import (
	"strings"
	"testing"

	"github.com/frostyard/chairlift/internal/selfreport"
)

// hintedEnums are the fields whose every v1 enum value needs a hint: S1
// result, S2 error_class, S3 how, S4 outcome and error_class, S5 outcome and
// tpm_rc, S6 subsystem, S7 signal, S8 class.
var hintedEnums = []struct{ kind, field string }{
	{KindUnitFailed, "result"},
	{KindUpdateStageFailed, "error_class"},
	{KindBootedOlderImage, "how"},
	{KindSysextUpdateFailed, "outcome"},
	{KindSysextUpdateFailed, "error_class"},
	{KindTPMUnlock, "outcome"},
	{KindTPMUnlock, "tpm_rc"},
	{KindIntegrityError, "subsystem"},
	{KindCrash, "signal"},
	{KindAppProblem, "class"},
}

// TestEveryEnumValueHasAHint parses the enums out of the vendored v1.json
// and fails when a value has no hint, or a hint names a value that the
// schema does not have.
func TestEveryEnumValueHasAHint(t *testing.T) {
	v := loadSchema(t)
	want := map[string]bool{}
	for _, e := range hintedEnums {
		for _, value := range schemaEnum(t, v, e.kind, e.field) {
			key := e.kind + "." + e.field + "=" + value
			want[key] = true
			if strings.TrimSpace(valueHints[key]) == "" {
				t.Errorf("no hint for %s", key)
			}
		}
	}
	for key := range valueHints {
		if !want[key] {
			t.Errorf("hint %s names no v1 enum value", key)
		}
	}
	for _, kind := range Kinds {
		if strings.TrimSpace(kindHints[kind]) == "" || strings.TrimSpace(kindTitles[kind]) == "" {
			t.Errorf("kind %s has no hint or title", kind)
		}
		if _, ok := hintFields[kind]; !ok && kind != KindCount {
			t.Errorf("kind %s names no hinted field", kind)
		}
	}
}

// firstWaveCodes are the first-wave codes of plan T857.
var firstWaveCodes = []string{
	"store.migrate_failed", "sync.apply_failed", "mime.parse_failed", "secrets.unseal_failed",
	"bridge.socket_lost", "merge.failed", "vault.write_failed", "watch.overflow_rescan",
	"sync.protocol_error", "extract.failed", "render.too_large", "lock.held", "blobs.sweep_failed",
}

// TestEveryKnownCodeHasAHint requires a hint for each of ChairLift's own
// codes and each first-wave code, and that every hinted code is valid.
func TestEveryKnownCodeHasAHint(t *testing.T) {
	codes := append(append([]string(nil), selfreport.Codes...), firstWaveCodes...)
	for _, code := range codes {
		if strings.TrimSpace(codeHints[code]) == "" {
			t.Errorf("no hint for code %s", code)
		}
	}
	if got := KnownCodes(); len(got) != len(codes) {
		t.Errorf("KnownCodes() = %d codes, want %d", len(got), len(codes))
	}
	for _, code := range KnownCodes() {
		if !ValidCode(code) {
			t.Errorf("hinted code %q fails the v1 code pattern", code)
		}
	}
}

func appItemFor(class, code string) Item {
	fields := []Field{
		{Name: "app_id", Display: "org.frostyard.Frostmail", Value: "org.frostyard.Frostmail", Valid: true},
		{Name: "class", Display: class, Value: class, Valid: true},
	}
	if code != "" {
		fields = append(fields, Field{Name: "code", Display: code, Value: code, Valid: true})
	}
	return Item{Kind: KindAppProblem, KnownKind: true, Fields: fields}
}

func TestHintOfAppCodes(t *testing.T) {
	known := Hint(appItemFor("error", "store.migrate_failed"))
	if !strings.Contains(known, codeHints["store.migrate_failed"]) || !strings.Contains(known, valueHints["app_problem.class=error"]) {
		t.Errorf("known code hint = %q", known)
	}
	unknown := Hint(appItemFor("error", "made.up_code"))
	if unknown != kindHints[KindAppProblem]+" "+valueHints["app_problem.class=error"] {
		t.Errorf("unknown code hint = %q, want the generic hint of class error", unknown)
	}
	panicHint := Hint(appItemFor("panic", ""))
	if panicHint != kindHints[KindAppProblem]+" "+valueHints["app_problem.class=panic"] {
		t.Errorf("panic hint = %q", panicHint)
	}
}

func TestHintOfSystemKinds(t *testing.T) {
	file := fixtureSystem(t)
	sysext := itemByID(t, file.Items, "192ac23ef1369eed")
	if got := Hint(sysext); !strings.Contains(got, valueHints["sysext_update_failed.outcome=failed"]) ||
		!strings.Contains(got, valueHints["sysext_update_failed.error_class=network"]) {
		t.Errorf("sysext hint = %q", got)
	}
	tpm := itemByID(t, file.Items, "955a8fafd851cd82")
	if got := Hint(tpm); !strings.Contains(got, valueHints["tpm_unlock.tpm_rc=PCR_CHANGED"]) {
		t.Errorf("tpm hint = %q", got)
	}
	// A field that failed its rule gives no value hint.
	forged := ParseSystem([]byte(systemDoc(problemJSON("00000000000000aa", KindCrash, `{"exe": "x", "signal": "SIGKILL"}`)))).Items[0]
	if got := Hint(forged); got != kindHints[KindCrash] {
		t.Errorf("forged hint = %q", got)
	}
	if Hint(Item{Kind: "x", KnownKind: false}) != unknownKindHint {
		t.Error("unknown kind hint")
	}
}

func TestTitles(t *testing.T) {
	file := fixtureSystem(t)
	want := map[string]string{
		"2b4cfb6309b48807": "The computer started an older system version",
		"27d5ac0722e592bf": "A system service failed",
		"955a8fafd851cd82": "The disk needed extra steps to unlock",
		"b9022b0f28891801": "System files failed an integrity check",
		"2b32f07ed829da93": "Pilothouse stopped unexpectedly",
		"2d92745c3c95a97f": "Frostmail reported an error",
		"658d8b6145a2afc2": "A program stopped unexpectedly",
		"7353e332c0c70754": "gnome-shell stopped unexpectedly",
		"8d1b483b03cda85a": "An update could not be prepared",
		"061b613f82107709": "A system feature could not be updated",
	}
	for id, title := range want {
		if got := Title(itemByID(t, file.Items, id)); got != title {
			t.Errorf("Title(%s) = %q, want %q", id, got, title)
		}
	}
	count := Item{Kind: KindCount, KnownKind: true}
	if Title(count) != "A usage count was recorded" {
		t.Errorf("count title = %q", Title(count))
	}
	offList := appItemFor("panic", "")
	offList.Fields[0].Valid = false
	if Title(offList) != "An app reported a problem" {
		t.Errorf("title of an app off the list = %q", Title(offList))
	}
	for id, name := range appNames {
		if AppName(id) != name || name == "" {
			t.Errorf("AppName(%s) = %q", id, AppName(id))
		}
	}
	if len(appNames) != len(appIDs) {
		t.Errorf("appNames has %d names, the allowlist %d IDs", len(appNames), len(appIDs))
	}
}
