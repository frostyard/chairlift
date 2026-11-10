package sysinfo

import (
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseCPUReadsTheAggregateLine(t *testing.T) {
	stat := "cpu  100 0 50 800 50 0 0 0 0 0\ncpu0 1 2 3 4 5 6 7 8 9 10\nintr 1\n"
	got, err := ParseCPU(strings.NewReader(stat))
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1000 || got.Busy != 150 {
		t.Errorf("ParseCPU = %+v, want busy 150 of 1000", got)
	}
	if _, err := ParseCPU(strings.NewReader("intr 1\n")); err == nil {
		t.Error("ParseCPU without a cpu line returned no error")
	}
	if _, err := ParseCPU(strings.NewReader("cpu 1 2 x 4 5\n")); err == nil {
		t.Error("ParseCPU with a bad field returned no error")
	}
}

func TestCPUPercent(t *testing.T) {
	cases := []struct {
		name      string
		prev, now CPUTimes
		want      float64
	}{
		{"quarter busy", CPUTimes{100, 1000}, CPUTimes{150, 1200}, 25},
		{"no time passed", CPUTimes{100, 1000}, CPUTimes{100, 1000}, 0},
		{"counters reset", CPUTimes{100, 1000}, CPUTimes{10, 50}, 0},
		{"all busy", CPUTimes{0, 0}, CPUTimes{10, 10}, 100},
	}
	for _, tc := range cases {
		if got := CPUPercent(tc.prev, tc.now); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: CPUPercent = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestParseMemory(t *testing.T) {
	info := "MemTotal:       16000000 kB\nMemFree:  1 kB\nMemAvailable:    4000000 kB\n"
	m, err := ParseMemory(strings.NewReader(info))
	if err != nil {
		t.Fatal(err)
	}
	if m.Total != 16000000*1024 || m.Available != 4000000*1024 {
		t.Errorf("ParseMemory = %+v", m)
	}
	if got := m.UsedPercent(); got != 75 {
		t.Errorf("UsedPercent = %v, want 75", got)
	}
	if _, err := ParseMemory(strings.NewReader("MemTotal: 1 kB\n")); err == nil {
		t.Error("ParseMemory without MemAvailable returned no error")
	}
	if (Memory{Total: 1, Available: 2}).UsedPercent() != 0 {
		t.Error("UsedPercent with more available than total is not 0")
	}
}

func TestDiskShares(t *testing.T) {
	d := Disk{Free: 250, Total: 1000}
	if d.Used() != 750 || d.UsedPercent() != 75 {
		t.Errorf("Disk %+v: used %d (%v%%)", d, d.Used(), d.UsedPercent())
	}
	if (Disk{}).UsedPercent() != 0 || (Disk{Free: 5, Total: 1}).Used() != 0 {
		t.Error("an empty or inconsistent disk does not read as 0 used")
	}
	dir := t.TempDir()
	got, err := ReadDisk(dir)
	if err != nil {
		t.Fatalf("ReadDisk(%s): %v", dir, err)
	}
	if got.Total == 0 || got.Free > got.Total {
		t.Errorf("ReadDisk = %+v", got)
	}
	if _, err := ReadDisk(dir + "/missing"); err == nil {
		t.Error("ReadDisk of a missing path returned no error")
	}
}

func TestUptime(t *testing.T) {
	up, err := ParseUptime([]byte("274322.51 1000.00\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatUptime(up); got != "up 3d 4h 12m" {
		t.Errorf("FormatUptime = %q", got)
	}
	if got := FormatUptime(4*time.Hour + 2*time.Minute); got != "up 4h 2m" {
		t.Errorf("FormatUptime = %q", got)
	}
	if got := FormatUptime(-time.Second); got != "up 0m" {
		t.Errorf("FormatUptime of a negative duration = %q", got)
	}
	for _, bad := range []string{"", "x 1", "-5 1"} {
		if _, err := ParseUptime([]byte(bad)); err == nil {
			t.Errorf("ParseUptime(%q) returned no error", bad)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[uint64]string{
		0:                 "0 B",
		999:               "999 B",
		1000:              "1 kB",
		3_400_000:         "3.4 MB",
		288_400_000_000:   "288 GB",
		1_200_000_000_000: "1.2 TB",
		99_960_000:        "100 MB",
	}
	for n, want := range cases {
		if got := FormatBytes(n); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestModelPrefersTheMarketingName(t *testing.T) {
	cases := []struct{ name, version, want string }{
		{"21KC0013US", "ThinkPad X1 Carbon Gen 12", "ThinkPad X1 Carbon Gen 12"},
		{"Laptop 13 (AMD Ryzen 7040Series)", "A6", "Laptop 13 (AMD Ryzen 7040Series)"},
		{"To Be Filled By O.E.M.", "Default string", ""},
		{"XPS 13 9340", "XPS 13 9340", "XPS 13 9340"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := Model(tc.name, tc.version); got != tc.want {
			t.Errorf("Model(%q, %q) = %q, want %q", tc.name, tc.version, got, tc.want)
		}
	}
}

func TestParseSecureBoot(t *testing.T) {
	if ParseSecureBoot([]byte{6, 0, 0, 0, 1}) != SecureBootOn {
		t.Error("value 1 is not on")
	}
	if ParseSecureBoot([]byte{6, 0, 0, 0, 0}) != SecureBootOff {
		t.Error("value 0 is not off")
	}
	if ParseSecureBoot([]byte{6, 0}) != SecureBootUnknown {
		t.Error("a short variable is not unknown")
	}
}

func TestSamplerNeedsTwoReadingsForCPU(t *testing.T) {
	stats := []string{"cpu 100 0 0 900 0\n", "cpu 150 0 0 1050 0\n"}
	calls := 0
	s := &Sampler{
		ReadFile: func(path string) ([]byte, error) {
			switch path {
			case "/proc/stat":
				data := stats[min(calls, len(stats)-1)]
				calls++
				return []byte(data), nil
			case "/proc/meminfo":
				return []byte("MemTotal: 100 kB\nMemAvailable: 50 kB\n"), nil
			case "/proc/uptime":
				return []byte("60.0 1.0"), nil
			}
			return nil, os.ErrNotExist
		},
		Statfs:   func(string) (Disk, error) { return Disk{Free: 1, Total: 4}, nil },
		DiskPath: "/var/home",
	}
	first := s.Next()
	if first.CPU != 0 {
		t.Errorf("first CPU = %v, want 0", first.CPU)
	}
	second := s.Next()
	if second.CPU != 25 {
		t.Errorf("second CPU = %v, want 25", second.CPU)
	}
	if second.Memory.UsedPercent() != 50 || second.Uptime != time.Minute || second.Disk.Total != 4 {
		t.Errorf("second sample = %+v", second)
	}

	broken := &Sampler{
		ReadFile: func(string) ([]byte, error) { return nil, errors.New("no /proc") },
		Statfs:   func(string) (Disk, error) { return Disk{}, errors.New("no disk") },
		DiskPath: "/",
	}
	if got := broken.Next(); got != (Sample{}) {
		t.Errorf("sample without sources = %+v, want zero", got)
	}
}

func TestKernelIsNotEmptyOnLinux(t *testing.T) {
	if Kernel() == "" {
		t.Error("Kernel() is empty")
	}
}

func TestReadTrimmed(t *testing.T) {
	path := t.TempDir() + "/product_name"
	if err := os.WriteFile(path, []byte("  Framework Laptop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ReadTrimmed(path); got != "Framework Laptop" {
		t.Errorf("ReadTrimmed = %q", got)
	}
	if got := ReadTrimmed(path + ".missing"); got != "" {
		t.Errorf("ReadTrimmed of a missing file = %q", got)
	}
}
