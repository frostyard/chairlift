package sysupdate

import (
	"log"
	"os"
	"regexp"
	"strings"
)

const (
	// UpdateCheckPath records the outcome of the most recent stager run.
	// World-readable, root-written, cleared by reboot (/run is tmpfs).
	UpdateCheckPath = "/run/snosi/update-check"
	// StagedSemaphorePath is the reboot-pending semaphore: present exactly
	// while a downloaded update awaits the reboot that applies it.
	StagedSemaphorePath = "/run/snosi/update-staged"
)

// CheckOutcome is the outcome field of an update-check state file.
type CheckOutcome string

const (
	OutcomeCurrent CheckOutcome = "current"
	OutcomeStaged  CheckOutcome = "staged"
	OutcomeFailed  CheckOutcome = "failed"
	// OutcomeUnknown covers an absent file or an unrecognized outcome value
	// (held-rollback is bootc-transport-only and never written on native).
	OutcomeUnknown CheckOutcome = ""
)

// UpdateCheck is the parsed /run/snosi/update-check state file.
type UpdateCheck struct {
	Outcome        CheckOutcome
	CheckedAt      string
	Image          string
	RunningVersion string
	RemoteVersion  string
}

// StagedUpdate is the parsed /run/snosi/update-staged semaphore. The file
// language is shared with the bootc transport: exactly one of Version
// (native, 14-digit) and Digest (bootc) is present.
type StagedUpdate struct {
	Image    string
	Version  string
	Digest   string
	StagedAt string
}

var versionPattern = regexp.MustCompile(`^[0-9]{14}$`)

// ValidVersion reports whether s matches the frozen native A/B version
// grammar: exactly 14 ASCII digits (UTC YYYYMMDDHHMMSS). Fixed width means
// lexicographic comparison equals numeric comparison.
func ValidVersion(s string) bool {
	return versionPattern.MatchString(s)
}

// parseKeyValues splits newline-separated key=value data. The first
// occurrence of a key wins; blank lines and lines without '=' are ignored.
func parseKeyValues(data []byte) map[string]string {
	fields := make(map[string]string)
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		if _, seen := fields[key]; !seen {
			fields[key] = value
		}
	}
	return fields
}

// ParseUpdateCheck parses update-check state file contents. Unknown keys and
// unknown outcome values are tolerated: an unrecognized outcome maps to
// OutcomeUnknown rather than an error, so a future stager field cannot break
// the status display.
func ParseUpdateCheck(data []byte) UpdateCheck {
	fields := parseKeyValues(data)
	check := UpdateCheck{
		CheckedAt:      fields["checked_at"],
		Image:          fields["image"],
		RunningVersion: fields["running_version"],
		RemoteVersion:  fields["remote_version"],
	}
	switch outcome := CheckOutcome(fields["outcome"]); outcome {
	case OutcomeCurrent, OutcomeStaged, OutcomeFailed:
		check.Outcome = outcome
	default:
		check.Outcome = OutcomeUnknown
	}
	return check
}

// ParseStagedUpdate parses update-staged semaphore contents.
func ParseStagedUpdate(data []byte) StagedUpdate {
	fields := parseKeyValues(data)
	return StagedUpdate{
		Image:    fields["image"],
		Version:  fields["version"],
		Digest:   fields["digest"],
		StagedAt: fields["staged_at"],
	}
}

// DisplayVersion returns the staged version identifier for presentation:
// the 14-digit version when valid, else a shortened digest, else "".
func (s *StagedUpdate) DisplayVersion() string {
	if s == nil {
		return ""
	}
	if ValidVersion(s.Version) {
		return s.Version
	}
	if s.Digest != "" {
		digest := strings.TrimPrefix(s.Digest, "sha256:")
		if len(digest) > 12 {
			digest = digest[:12]
		}
		return digest
	}
	return ""
}

// readUpdateCheckFrom reads and parses an update-check file. An absent file
// is a normal state (no check has run this boot), reported as (nil, nil).
func readUpdateCheckFrom(path string) (*UpdateCheck, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	check := ParseUpdateCheck(data)
	return &check, nil
}

// readStagedUpdateFrom reads and parses an update-staged semaphore. An
// absent file is a normal state (nothing staged), reported as (nil, nil).
func readStagedUpdateFrom(path string) (*StagedUpdate, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	staged := ParseStagedUpdate(data)
	return &staged, nil
}

// ReadUpdateCheck reads the fixed update-check state file.
func ReadUpdateCheck() (*UpdateCheck, error) {
	return readUpdateCheckFrom(UpdateCheckPath)
}

// ReadStagedUpdate reads the fixed update-staged semaphore.
func ReadStagedUpdate() (*StagedUpdate, error) {
	return readStagedUpdateFrom(StagedSemaphorePath)
}

// Status is the combined unprivileged update state. Either field may be nil
// when its file is absent.
type Status struct {
	Check  *UpdateCheck
	Staged *StagedUpdate
}

// GetStatus reads both state files. It never fails hard: an unreadable file
// is logged and treated as absent, so the UI degrades to the idle state
// rather than an error.
func GetStatus() Status {
	check, err := ReadUpdateCheck()
	if err != nil {
		log.Printf("sysupdate: reading %s: %v", UpdateCheckPath, err)
	}
	staged, err := ReadStagedUpdate()
	if err != nil {
		log.Printf("sysupdate: reading %s: %v", StagedSemaphorePath, err)
	}
	return Status{Check: check, Staged: staged}
}

// IsStaged reports whether an update is downloaded and pending a reboot.
// The semaphore is authoritative; outcome=staged covers the anomalous case
// of a check record without its semaphore.
func (s Status) IsStaged() bool {
	if s.Staged != nil {
		return true
	}
	return s.Check != nil && s.Check.Outcome == OutcomeStaged
}

// Presentation reduces the state files to the scalar inputs of the pure
// subtitle formatter, one row per distinct outcome:
//   - staged semaphore present        -> ("staged", staged version, "")
//   - no semaphore, outcome=staged    -> ("staged", remote_version, "")
//   - outcome=current                 -> ("current", "", checked_at)
//   - outcome=failed                  -> ("failed", "", checked_at)
//   - files absent or outcome unknown -> ("", "", "")   (idle prompt)
//
// The semaphore wins over every check outcome: it is the OS's reboot-pending
// signal and legitimately outlives later "nothing newer" check runs.
func (s Status) Presentation() (outcome, version, checkedAt string) {
	if s.Staged != nil {
		return string(OutcomeStaged), s.Staged.DisplayVersion(), ""
	}
	if s.Check == nil {
		return "", "", ""
	}
	switch s.Check.Outcome {
	case OutcomeStaged:
		return string(OutcomeStaged), s.Check.RemoteVersion, ""
	case OutcomeCurrent:
		return string(OutcomeCurrent), "", s.Check.CheckedAt
	case OutcomeFailed:
		return string(OutcomeFailed), "", s.Check.CheckedAt
	default:
		return "", "", ""
	}
}
