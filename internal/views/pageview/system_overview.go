package pageview

import (
	"fmt"
	"strings"
	"unicode"
)

// ImageChannel is the update channel an image reference names: its tag,
// as "stable" of "ghcr.io/frostyard/snow:stable". A reference by digest, or
// one without a tag, names none ("").
func ImageChannel(ref string) string {
	ref = strings.TrimSpace(ref)
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	// A registry's port also follows a colon, so the tag is looked for in
	// the last path element only.
	last := ref
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		last = ref[i+1:]
	}
	if i := strings.LastIndex(last, ":"); i >= 0 {
		return last[i+1:]
	}
	return ""
}

// StagedNote is the line under the system image's title while an image is
// staged.
func StagedNote(version string) string {
	if version == "" {
		return "An update is staged · applies on restart"
	}
	return fmt.Sprintf("Staged %s · applies on restart", version)
}

// AppDisplayName names a desktop application by its application ID, for a
// button that opens it: the ID's last element with its words split, as
// "Mission Center" of "io.missioncenter.MissionCenter". An ID with no
// capitals stays as it is.
func AppDisplayName(appID string) string {
	name := appID
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
				b.WriteRune(' ')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}
