package sysupdate

import "testing"

func TestImageIdentityParsing(t *testing.T) {
	data := `PRETTY_NAME="Snow Linux"
NAME="Snow Linux"
ID="snow"
IMAGE_ID="snow"
IMAGE_VERSION="20260810191856"
`
	imageID, version := imageIdentity([]byte(data))
	if imageID != "snow" || version != "20260810191856" {
		t.Errorf("imageIdentity = (%q, %q), want (snow, 20260810191856)", imageID, version)
	}
}

func TestImageIdentityUnquotedValues(t *testing.T) {
	imageID, version := imageIdentity([]byte("IMAGE_ID=cayo\nIMAGE_VERSION=20260101000000\n"))
	if imageID != "cayo" || version != "20260101000000" {
		t.Errorf("imageIdentity = (%q, %q), want (cayo, 20260101000000)", imageID, version)
	}
}

// lsblkFixture builds lsblk -J output with partitions nested under a disk's
// children array, the shape real lsblk emits.
func lsblkFixture(labels ...string) []byte {
	out := `{"blockdevices":[{"path":"/dev/nvme0n1","partlabel":null,"children":[`
	for i, label := range labels {
		if i > 0 {
			out += ","
		}
		out += `{"path":"/dev/nvme0n1p` + string(rune('1'+i)) + `","partlabel":"` + label + `"}`
	}
	return []byte(out + `]}]}`)
}

func TestOtherSlotFromLsblk(t *testing.T) {
	tests := []struct {
		name           string
		labels         []string
		imageID        string
		runningVersion string
		want           string
		wantOK         bool
	}{
		{
			name:           "older version in other slot",
			labels:         []string{"esp", "snow_20260810191856_r", "snow_20260810191856_v", "snow_20260801000000_r", "snow_20260801000000_v", "var"},
			imageID:        "snow",
			runningVersion: "20260810191856",
			want:           "20260801000000",
			wantOK:         true,
		},
		{
			name:           "fresh install other slot empty",
			labels:         []string{"esp", "snow_20260810191856_r", "snow_20260810191856_v", "_empty", "_empty", "var"},
			imageID:        "snow",
			runningVersion: "20260810191856",
			wantOK:         false,
		},
		{
			name:           "running slot excluded by label not mount source",
			labels:         []string{"snow_20260810191856_r"},
			imageID:        "snow",
			runningVersion: "20260810191856",
			wantOK:         false,
		},
		{
			name:           "verity slots never match",
			labels:         []string{"snow_20260810191856_r", "snow_20260801000000_v"},
			imageID:        "snow",
			runningVersion: "20260810191856",
			wantOK:         false,
		},
		{
			name:           "empty image id yields nothing",
			labels:         []string{"snow_20260801000000_r"},
			imageID:        "",
			runningVersion: "20260810191856",
			wantOK:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := otherSlotFromLsblk(lsblkFixture(tt.labels...), tt.imageID, tt.runningVersion)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("otherSlotFromLsblk = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestOtherSlotFromLsblkMalformedJSON(t *testing.T) {
	if _, ok := otherSlotFromLsblk([]byte("not json"), "snow", "20260810191856"); ok {
		t.Error("otherSlotFromLsblk accepted malformed JSON")
	}
}

func TestRollbackCandidateRule(t *testing.T) {
	tests := []struct {
		name           string
		other, running string
		want           string
		wantOK         bool
	}{
		{
			name:  "older other slot is a rollback candidate",
			other: "20260801000000", running: "20260810191856",
			want: "20260801000000", wantOK: true,
		},
		{
			// After a stage the inactive slot holds the newer pending
			// version; that is not a rollback target.
			name:  "newer other slot is pending not rollback",
			other: "20260811000000", running: "20260810191856",
			wantOK: false,
		},
		{
			name:  "equal versions never qualify",
			other: "20260810191856", running: "20260810191856",
			wantOK: false,
		},
		{
			name:  "invalid other version",
			other: "", running: "20260810191856",
			wantOK: false,
		},
		{
			name:  "invalid running version",
			other: "20260801000000", running: "",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RollbackCandidate(tt.other, tt.running)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("RollbackCandidate(%q, %q) = (%q, %v), want (%q, %v)",
					tt.other, tt.running, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
