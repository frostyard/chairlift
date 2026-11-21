package installcheck

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// removedAIReviewMarkers name the Claude code review workflow that the
// maintainer removed on 2026-11-21. No workflow or composite action may bring
// it back without a new decision (.memory/corrections record of that date).
var removedAIReviewMarkers = []string{
	"anthropics/claude-code-action",
	"ANTHROPIC_API_KEY",
	"CLAUDE_CODE_",
}

// removedAIReviewProblems returns each marker that text names, prefixed by rel.
func removedAIReviewProblems(rel, text string) []string {
	var problems []string
	for _, marker := range removedAIReviewMarkers {
		if strings.Contains(text, marker) {
			problems = append(problems, rel+": names "+marker)
		}
	}
	return problems
}

func TestRemovedAIReviewProblems(t *testing.T) {
	cases := map[string]int{
		"steps:\n  - uses: anthropics/claude-code-action@0123 # v1.0.0\n": 1,
		"env:\n  KEY: ${{ secrets.ANTHROPIC_API_KEY }}\n":                 1,
		"env:\n  CLAUDE_CODE_OAUTH_TOKEN: x\n":                            1,
		"steps:\n  - uses: actions/checkout@0123 # v7.0.1\n":              0,
	}
	for text, want := range cases {
		if got := len(removedAIReviewProblems("f.yml", text)); got != want {
			t.Errorf("removedAIReviewProblems(%q) = %d problems, want %d", text, got, want)
		}
	}
}

func TestClaudeCodeReviewStaysRemoved(t *testing.T) {
	root := RepoRoot()
	if _, err := os.Stat(filepath.Join(root, ".github", "workflows", "claude-code-review.yml")); err == nil {
		t.Error(".github/workflows/claude-code-review.yml exists; it was removed on 2026-11-21")
	}

	var problems []string
	files := 0
	for _, dir := range []string{filepath.Join(root, ".github", "workflows"), filepath.Join(root, ".github", "actions")} {
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files++
			rel, _ := filepath.Rel(root, path)
			problems = append(problems, removedAIReviewProblems(filepath.ToSlash(rel), string(data))...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if files == 0 {
		t.Fatal("found no workflow file to check")
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}
