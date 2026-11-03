package fwupd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deviceID is a well-formed fwupd device id: 40 lowercase hexadecimal
// characters.
const deviceID = "1111111111111111111111111111111111111111"

// useFakeExecutable points the package executable at script for the duration
// of the test.
func useFakeExecutable(t *testing.T, script string) {
	t.Helper()
	previous := executable
	executable = script
	t.Cleanup(func() { executable = previous })
}

// fakeFwupdmgr writes an executable shell script standing in for the fwupdmgr
// client, following the fake-script pattern of internal/flatpak's
// runner_test.go and internal/bootc's stage_test.go. The script records its
// own PID, and a t.Cleanup force-kills that PID (and its process group) if it
// is somehow still alive when the test ends.
func fakeFwupdmgr(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "self.pid")
	script := filepath.Join(dir, "fake-fwupdmgr")
	content := "#!/bin/sh\necho $$ > \"" + pidFile + "\"\n" + body + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pid, err := readPID(pidFile)
		if err != nil {
			return
		}
		killPID(pid)
	})
	return script
}

// updateFixture is a realistic multi-device `get-updates --json` document: one
// updatable device with two releases (newest first) and the needs-reboot flag,
// one device with no releases, one device whose id is not a fwupd id, one
// device with the needs-shutdown flag and no vendor or current version, and one
// device with no Releases key at all.
const updateFixture = `{
  "Devices": [
    {
      "DeviceId": "1111111111111111111111111111111111111111",
      "Name": "ThinkPad Embedded Controller",
      "Vendor": "Lenovo",
      "Version": "1.2",
      "Flags": ["updatable", "internal", "needs-reboot"],
      "Releases": [
        {"Version": "1.3", "Summary": "Fix battery charging", "Name": "EC 1.3", "Size": 1048576, "Flags": []},
        {"Version": "1.2.1", "Summary": "Older fix", "Size": 1048576, "Flags": []}
      ]
    },
    {
      "DeviceId": "2222222222222222222222222222222222222222",
      "Name": "UEFI dbx",
      "Vendor": "Lenovo",
      "Version": "371",
      "Flags": ["updatable"],
      "Releases": []
    },
    {
      "DeviceId": "not-a-valid-device-id",
      "Name": "Bogus device",
      "Vendor": "Frostyard",
      "Version": "1.0",
      "Flags": ["updatable"],
      "Releases": [{"Version": "9.9", "Summary": "Should never be offered"}]
    },
    {
      "DeviceId": "3333333333333333333333333333333333333333",
      "Name": "Thunderbolt Dock",
      "Vendor": "",
      "Version": "",
      "Flags": ["updatable", "needs-shutdown"],
      "Releases": [{"Version": "2.0", "Summary": "Update the dock firmware"}]
    },
    {
      "DeviceId": "4444444444444444444444444444444444444444",
      "Name": "No releases key",
      "Vendor": "Frostyard",
      "Version": "1.0",
      "Flags": ["updatable"]
    }
  ]
}`

func TestParseFwupdUpdates(t *testing.T) {
	updates, err := ParseUpdates([]byte(updateFixture))
	if err != nil {
		t.Fatalf("ParseUpdates: %v", err)
	}

	want := []Update{
		{
			DeviceID:       "1111111111111111111111111111111111111111",
			DeviceName:     "ThinkPad Embedded Controller",
			Vendor:         "Lenovo",
			CurrentVersion: "1.2",
			NewVersion:     "1.3",
			Summary:        "Fix battery charging",
			NeedsReboot:    true,
			NeedsShutdown:  false,
			Installable:    true,
		},
		{
			DeviceID:       "3333333333333333333333333333333333333333",
			DeviceName:     "Thunderbolt Dock",
			Vendor:         "",
			CurrentVersion: "",
			NewVersion:     "2.0",
			Summary:        "Update the dock firmware",
			NeedsReboot:    false,
			NeedsShutdown:  true,
			Installable:    true,
		},
	}
	if len(updates) != len(want) {
		t.Fatalf("ParseUpdates returned %d updates, want %d: %#v", len(updates), len(want), updates)
	}
	for i := range want {
		if updates[i] != want[i] {
			t.Errorf("updates[%d] = %#v, want %#v", i, updates[i], want[i])
		}
	}
}

// TestParseFwupdBlockedDevices covers the three ways fwupd reports a device
// that has an update but cannot apply it now. Such a device must stay in the
// list, marked not installable, with a reason, because `fwupdmgr update` would
// skip it and exit 0 without a flash.
func TestParseFwupdBlockedDevices(t *testing.T) {
	data := `{
  "Devices": [
    {
      "DeviceId": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "Name": "Lid-closed laptop panel",
      "Vendor": "Frostyard",
      "Version": "1.0",
      "Flags": ["updatable-hidden", "needs-reboot"],
      "Releases": [{"Version": "1.1", "Summary": "Panel firmware"}]
    },
    {
      "DeviceId": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "Name": "Battery-limited webcam",
      "Vendor": "Frostyard",
      "Version": "2.0",
      "Flags": ["updatable"],
      "Problems": ["Not enough battery power", "System is on battery"],
      "Releases": [{"Version": "2.1", "Summary": "Webcam firmware"}]
    },
    {
      "DeviceId": "cccccccccccccccccccccccccccccccccccccccc",
      "Name": "Failed dock",
      "Vendor": "Frostyard",
      "Version": "3.0",
      "Flags": ["updatable"],
      "UpdateError": "failed to decompress firmware",
      "Releases": [{"Version": "3.1", "Summary": "Dock firmware"}]
    }
  ]
}`

	updates, err := ParseUpdates([]byte(data))
	if err != nil {
		t.Fatalf("ParseUpdates: %v", err)
	}
	want := []struct {
		name        string
		installable bool
		reason      string
	}{
		{
			name:   "Lid-closed laptop panel",
			reason: "This device cannot be updated now",
		},
		{
			name:   "Battery-limited webcam",
			reason: "Needs attention: Not enough battery power, System is on battery",
		},
		{
			name:   "Failed dock",
			reason: "failed to decompress firmware",
		},
	}
	if len(updates) != len(want) {
		t.Fatalf("ParseUpdates returned %d updates, want %d: %#v", len(updates), len(want), updates)
	}
	for i, expected := range want {
		if updates[i].DeviceName != expected.name {
			t.Errorf("updates[%d].DeviceName = %q, want %q", i, updates[i].DeviceName, expected.name)
		}
		if updates[i].Installable != expected.installable {
			t.Errorf("updates[%d].Installable = %v, want %v", i, updates[i].Installable, expected.installable)
		}
		if updates[i].Reason != expected.reason {
			t.Errorf("updates[%d].Reason = %q, want %q", i, updates[i].Reason, expected.reason)
		}
	}
}

func TestParseFwupdBlockedDeviceKeepsNeedsRebootFlag(t *testing.T) {
	// A blocked device still reports the flags that select the confirmation
	// text, so the row and any later install read the same data.
	updates, err := ParseUpdates([]byte(`{"Devices":[{"DeviceId":"` + deviceID + `","Name":"Panel","Flags":["updatable-hidden","needs-reboot"],"Releases":[{"Version":"1.1"}]}]}`))
	if err != nil {
		t.Fatalf("ParseUpdates: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("ParseUpdates returned %d updates, want 1: %#v", len(updates), updates)
	}
	if !updates[0].NeedsReboot {
		t.Error("NeedsReboot = false for a needs-reboot device, want true")
	}
	if updates[0].Installable {
		t.Error("Installable = true for an updatable-hidden device, want false")
	}
}

func TestParseFwupdUpdatesEmptyDeviceList(t *testing.T) {
	updates, err := ParseUpdates([]byte(`{"Devices" : []}`))
	if err != nil {
		t.Fatalf("ParseUpdates: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("ParseUpdates = %#v, want no updates", updates)
	}
}

func TestParseFwupdUpdatesRejectsBadJSON(t *testing.T) {
	for _, data := range []string{"", "not json", `{"Devices": "nope"}`} {
		if _, err := ParseUpdates([]byte(data)); err == nil {
			t.Errorf("ParseUpdates(%q) = nil error, want a parse failure", data)
		}
	}
}

func TestListFwupdUpdatesReturnsParsedDevice(t *testing.T) {
	useFakeExecutable(t, fakeFwupdmgr(t, `cat <<'JSON'
{"Devices":[{"DeviceId":"`+deviceID+`","Name":"Dock","Vendor":"Frostyard","Version":"1.0","Flags":["updatable"],"Releases":[{"Version":"2.0","Summary":"Firmware"}]}]}
JSON`))

	updates, err := ListUpdates()
	if err != nil {
		t.Fatalf("ListUpdates: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("ListUpdates returned %d updates, want 1: %#v", len(updates), updates)
	}
	if updates[0].DeviceID != deviceID || updates[0].NewVersion != "2.0" {
		t.Errorf("updates[0] = %#v", updates[0])
	}
}

func TestListFwupdUpdatesTreatsNothingToDoAsEmpty(t *testing.T) {
	useFakeExecutable(t, fakeFwupdmgr(t, `echo "No updatable devices" >&2
exit 2`))

	updates, err := ListUpdates()
	if err != nil {
		t.Fatalf("ListUpdates = %v, want nil error for exit status 2", err)
	}
	if len(updates) != 0 {
		t.Fatalf("ListUpdates = %#v, want no updates", updates)
	}
}

func TestListFwupdUpdatesReportsFailure(t *testing.T) {
	useFakeExecutable(t, fakeFwupdmgr(t, `echo "failed to connect to daemon" >&2
exit 1`))

	_, err := ListUpdates()
	if err == nil {
		t.Fatal("ListUpdates = nil error, want a failure")
	}
	var fwupdErr *Error
	if !errors.As(err, &fwupdErr) {
		t.Fatalf("err = %T (%v), want *Error", err, err)
	}
	if !strings.Contains(fwupdErr.Error(), "failed to connect to daemon") {
		t.Errorf("message = %q, want it to contain the script's stderr", fwupdErr.Error())
	}
}

func TestFwupdInstallPassesExactArguments(t *testing.T) {
	argvFile := filepath.Join(t.TempDir(), "argv.txt")
	useFakeExecutable(t, fakeFwupdmgr(t, `printf '%s\n' "$@" > "`+argvFile+`"`))

	if err := Install(deviceID); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("read recorded argv: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{"update", "--no-reboot-check", deviceID}
	if len(got) != len(want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFwupdInstallRejectsInvalidDeviceID(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	useFakeExecutable(t, fakeFwupdmgr(t, `touch "`+marker+`"`))

	invalid := []string{
		"--force",
		"",
		"111111111111111111111111111111111111111",   // 39 characters
		"11111111111111111111111111111111111111111", // 41 characters
		"ABCDEF1111111111111111111111111111111111",  // uppercase
		"111111111111111111111111111111111111111g",  // not hexadecimal
		"1111111111111111111111111111111111111111 --force",
	}
	for _, id := range invalid {
		t.Run(id, func(t *testing.T) {
			if err := Install(id); err == nil {
				t.Fatalf("Install(%q) = nil error, want a rejection", id)
			}
		})
	}

	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the fake fwupdmgr ran for an invalid device id (stat %s = %v)", marker, err)
	}
}

func TestFwupdInstallDryRunDoesNotRunScript(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	useFakeExecutable(t, fakeFwupdmgr(t, `touch "`+marker+`"`))
	SetDryRun(true)
	t.Cleanup(func() { SetDryRun(false) })

	if err := Install(deviceID); err != nil {
		t.Fatalf("Install under dry-run = %v, want nil", err)
	}
	if !IsDryRun() {
		t.Error("IsDryRun() = false, want true")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the fake fwupdmgr ran under dry-run (stat %s = %v)", marker, err)
	}
}

func TestFwupdIsInstalledMissingBinary(t *testing.T) {
	previous := executable
	executable = "chairlift-no-such-fwupdmgr-command"
	t.Cleanup(func() { executable = previous })

	if IsInstalled() {
		t.Error("IsInstalled() = true for a missing fwupdmgr, want false")
	}
}
