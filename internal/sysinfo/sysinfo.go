// Package sysinfo reads the host facts the Overview page shows: processor
// and memory use from /proc, free space with statfs, uptime, the host name,
// the kernel release, the hardware model from DMI and the Secure Boot state
// from EFI variables. Every reader takes its source as a parameter (a path
// or a function), so the parsing is tested with fixtures; the package
// imports no UI code.
package sysinfo

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CPUTimes is the cumulative processor time from the first line of
// /proc/stat, in clock ticks.
type CPUTimes struct {
	Busy  uint64
	Total uint64
}

// ParseCPU reads the aggregate "cpu" line of /proc/stat. Idle and iowait
// count as not busy.
func ParseCPU(r io.Reader) (CPUTimes, error) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		var t CPUTimes
		for i, field := range fields[1:] {
			v, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return CPUTimes{}, fmt.Errorf("parse /proc/stat field %d: %w", i+1, err)
			}
			t.Total += v
			if i != 3 && i != 4 { // idle, iowait
				t.Busy += v
			}
		}
		return t, nil
	}
	if err := s.Err(); err != nil {
		return CPUTimes{}, err
	}
	return CPUTimes{}, errors.New("no cpu line in /proc/stat")
}

// CPUPercent is the busy share between two readings, 0 to 100. It is 0 when
// no time passed or the counters went backwards.
func CPUPercent(prev, now CPUTimes) float64 {
	if now.Total <= prev.Total || now.Busy < prev.Busy {
		return 0
	}
	p := 100 * float64(now.Busy-prev.Busy) / float64(now.Total-prev.Total)
	return clamp(p)
}

// Memory is the memory reading of /proc/meminfo, in bytes.
type Memory struct {
	Total     uint64
	Available uint64
}

// UsedPercent is the share of memory in use, 0 to 100.
func (m Memory) UsedPercent() float64 {
	if m.Total == 0 || m.Available > m.Total {
		return 0
	}
	return clamp(100 * float64(m.Total-m.Available) / float64(m.Total))
}

// ParseMemory reads MemTotal and MemAvailable from /proc/meminfo.
func ParseMemory(r io.Reader) (Memory, error) {
	var m Memory
	var haveTotal, haveAvail bool
	s := bufio.NewScanner(r)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		mult := uint64(1)
		if len(fields) > 2 && fields[2] == "kB" {
			mult = 1024
		}
		switch fields[0] {
		case "MemTotal:":
			m.Total, haveTotal = v*mult, true
		case "MemAvailable:":
			m.Available, haveAvail = v*mult, true
		}
	}
	if err := s.Err(); err != nil {
		return Memory{}, err
	}
	if !haveTotal || !haveAvail {
		return Memory{}, errors.New("MemTotal or MemAvailable missing from /proc/meminfo")
	}
	return m, nil
}

// Disk is the space of one file system, in bytes.
type Disk struct {
	Path  string
	Free  uint64 // available to unprivileged users
	Total uint64
}

// Used is the space not free.
func (d Disk) Used() uint64 {
	if d.Free > d.Total {
		return 0
	}
	return d.Total - d.Free
}

// UsedPercent is the share in use, 0 to 100.
func (d Disk) UsedPercent() float64 {
	if d.Total == 0 {
		return 0
	}
	return clamp(100 * float64(d.Used()) / float64(d.Total))
}

// ReadDisk reads the file system that holds path.
func ReadDisk(path string) (Disk, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Disk{}, fmt.Errorf("statfs %s: %w", path, err)
	}
	bsize := uint64(st.Bsize) //nolint:gosec // block size is never negative
	return Disk{Path: path, Free: st.Bavail * bsize, Total: st.Blocks * bsize}, nil
}

// ParseUptime reads the first field of /proc/uptime.
func ParseUptime(data []byte) (time.Duration, error) {
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, errors.New("empty /proc/uptime")
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || secs < 0 {
		return 0, fmt.Errorf("parse /proc/uptime: %q", fields[0])
	}
	return time.Duration(secs * float64(time.Second)), nil
}

// FormatUptime gives "up 3d 4h 12m", "up 4h 12m" or "up 12m".
func FormatUptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	mins := int(d / time.Minute)
	days, hours, minutes := mins/(24*60), (mins/60)%24, mins%60
	switch {
	case days > 0:
		return fmt.Sprintf("up %dd %dh %dm", days, hours, minutes)
	case hours > 0:
		return fmt.Sprintf("up %dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("up %dm", minutes)
	}
}

// FormatBytes gives a size in decimal units with at most one decimal:
// "512 B", "3.4 MB", "288 GB", "1.2 TB".
func FormatBytes(n uint64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"kB", "MB", "GB", "TB", "PB"}
	v := float64(n) / unit
	i := 0
	for v >= unit && i < len(units)-1 {
		v /= unit
		i++
	}
	if v >= 100 {
		return fmt.Sprintf("%.0f %s", v, units[i])
	}
	s := strconv.FormatFloat(v, 'f', 1, 64)
	s = strings.TrimSuffix(s, ".0")
	return s + " " + units[i]
}

// Kernel is the release field of uname, such as "6.17.7-amd64".
func Kernel() string {
	var u syscall.Utsname
	if err := syscall.Uname(&u); err != nil {
		return ""
	}
	b := make([]byte, 0, len(u.Release))
	for _, ch := range u.Release {
		if ch == 0 {
			break
		}
		b = append(b, byte(ch))
	}
	return string(b)
}

// ReadTrimmed reads a small text file such as a DMI attribute and trims it.
// A missing or unreadable file gives "".
func ReadTrimmed(path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // fixed sysfs paths
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Model gives the hardware model from DMI, such as "ThinkPad X1 Carbon Gen
// 12". It prefers product_version when that names the model (Lenovo writes
// the marketing name there and a part number in product_name), and drops
// placeholder strings firmware vendors leave behind.
func Model(name, version string) string {
	clean := func(s string) string {
		s = strings.TrimSpace(s)
		switch strings.ToLower(s) {
		case "", "none", "default string", "to be filled by o.e.m.", "system product name", "not specified", "0", "1.0":
			return ""
		}
		return s
	}
	name, version = clean(name), clean(version)
	if version != "" && strings.ContainsAny(version, " ") && !strings.EqualFold(version, name) {
		return version
	}
	return name
}

// SecureBootVariable is the EFI variable that holds the Secure Boot state.
const SecureBootVariable = "/sys/firmware/efi/efivars/SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c"

// SecureBootState is the Secure Boot state of the firmware.
type SecureBootState int

const (
	// SecureBootUnknown means there is no EFI variable to read: a BIOS
	// boot, or no permission.
	SecureBootUnknown SecureBootState = iota
	// SecureBootOff means the firmware boots unsigned code.
	SecureBootOff
	// SecureBootOn means the firmware enforces signatures.
	SecureBootOn
)

// ParseSecureBoot reads an efivarfs file: 4 bytes of attributes, then the
// one-byte value.
func ParseSecureBoot(data []byte) SecureBootState {
	if len(data) < 5 {
		return SecureBootUnknown
	}
	if data[4] == 1 {
		return SecureBootOn
	}
	return SecureBootOff
}

// Sample is one reading of the live figures.
type Sample struct {
	CPU    float64 // percent busy since the previous reading
	Memory Memory
	Disk   Disk
	Uptime time.Duration
}

// Sampler takes Samples. Its fields are the sources, so tests can replace
// them; NewSampler sets the real ones.
type Sampler struct {
	ReadFile func(string) ([]byte, error)
	Statfs   func(string) (Disk, error)
	DiskPath string

	prev    CPUTimes
	started bool
}

// NewSampler returns a Sampler of this host that measures the file system
// holding diskPath.
func NewSampler(diskPath string) *Sampler {
	return &Sampler{ReadFile: os.ReadFile, Statfs: ReadDisk, DiskPath: diskPath}
}

// Next takes a reading. The first reading's CPU is 0, since processor use
// needs two readings.
func (s *Sampler) Next() Sample {
	var out Sample
	if data, err := s.ReadFile("/proc/stat"); err == nil {
		if now, err := ParseCPU(bytes.NewReader(data)); err == nil {
			if s.started {
				out.CPU = CPUPercent(s.prev, now)
			}
			s.prev, s.started = now, true
		}
	}
	if data, err := s.ReadFile("/proc/meminfo"); err == nil {
		if m, err := ParseMemory(bytes.NewReader(data)); err == nil {
			out.Memory = m
		}
	}
	if data, err := s.ReadFile("/proc/uptime"); err == nil {
		if up, err := ParseUptime(data); err == nil {
			out.Uptime = up
		}
	}
	if s.DiskPath != "" && s.Statfs != nil {
		if d, err := s.Statfs(s.DiskPath); err == nil {
			out.Disk = d
		}
	}
	return out
}

func clamp(p float64) float64 {
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 100
	}
	return p
}
