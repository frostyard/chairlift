package problems

import (
	"fmt"
	"strings"
	"time"
)

// DetailsText returns the plain text that "Copy details" puts on the
// clipboard: the title, when, the hint, the validated fields, and the local
// details when present (details.json values, units, executable path and
// messages; an app record's MESSAGE and trusted fields). It is local text:
// a copy can hold names and paths that a report never holds.
func DetailsText(it Item) string {
	var b strings.Builder
	line := func(format string, args ...any) {
		fmt.Fprintf(&b, format+"\n", args...)
	}
	line("%s", Title(it))
	if !it.KnownKind {
		line("Kind: %s", it.Kind)
	}
	line("First seen: %s", formatTime(it.FirstSeen))
	line("Last seen: %s", formatTime(it.LastSeen))
	line("Times: %d", it.Count)
	line("Source: %s", originText(it.Origin))
	line("")
	line("%s", Hint(it))

	if len(it.Fields) > 0 || len(it.Missing) > 0 {
		line("")
		line("Recorded values:")
		for _, f := range it.Fields {
			if f.Valid {
				line("  %s: %s", f.Name, f.Display)
			} else {
				line("  %s: %s (not valid: %s)", f.Name, f.Display, f.Reason)
			}
		}
		for _, name := range it.Missing {
			line("  %s: (not recorded)", name)
		}
	}
	if it.Week != "" {
		line("  week: %s", it.Week)
	}
	if it.Reportable {
		line("Reportable: yes (ChairLift sends nothing)")
	} else {
		line("Reportable: no")
	}

	local := it.Local
	if len(local.Values)+len(local.Units)+len(local.Messages) > 0 || local.ExePath != "" ||
		local.Message != "" || local.Comm != "" || local.Unit != "" {
		line("")
		line("Local details (never sent):")
		for _, f := range local.Values {
			line("  %s: %s", f.Name, f.Display)
		}
		if len(local.Units) > 0 {
			line("  Units: %s", strings.Join(local.Units, ", "))
		}
		if local.ExePath != "" {
			line("  Program: %s", local.ExePath)
		}
		if local.Comm != "" {
			line("  Process: %s", local.Comm)
		}
		if local.Unit != "" {
			line("  Unit: %s", local.Unit)
		}
		if local.Message != "" {
			line("  Message: %s", local.Message)
		}
		for _, msg := range local.Messages {
			line("  Log: %s", msg)
		}
	}
	return b.String()
}

// formatTime gives an RFC 3339 UTC time, or "unknown" for a zero time.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format(time.RFC3339)
}

func originText(o Origin) string {
	switch o {
	case OriginSystem:
		return "system checks (snosi-health)"
	case OriginUserCrash:
		return "your crash records (coredumpctl)"
	case OriginAppRecord:
		return "app problem records (your journal)"
	default:
		return "unknown"
	}
}
