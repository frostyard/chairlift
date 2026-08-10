package installcheck

import (
	"path/filepath"
	"regexp"
	"testing"
)

func TestReleasePublisherUsesImmutableCommit(t *testing.T) {
	workflow := readRepoFile(t, filepath.Join(".github", "workflows", "release.yml"))
	actionPattern := regexp.MustCompile(`(?m)^\s*uses:\s+frostyard/repogen/\.github/actions/publish-to-r2@([^\s#]+)`)
	matches := actionPattern.FindAllStringSubmatch(workflow, -1)
	if len(matches) != 1 {
		t.Fatalf("release workflow has %d repogen publisher references, want 1", len(matches))
	}

	ref := matches[0][1]
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(ref) {
		t.Errorf("repogen publisher ref %q is mutable; want a full commit SHA", ref)
	}
}
