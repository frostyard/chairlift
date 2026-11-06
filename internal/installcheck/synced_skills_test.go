package installcheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// syncedSkillMarker is the file frostyard/core's skills sync writes into each
// skill directory it manages (core ADR-0026).
const syncedSkillMarker = ".synced-from-core"

// TestSyncedSkillsLiveInAgentsSkills pins where core's skills sync may write.
// The sync resolves the .agents path of the consumer when it runs. A sync
// branch cut while .agents was a symlink to docs/agents wrote a new skill to
// docs/agents/skills/; it merged after .agents became a real directory, so the
// skill sat outside .agents/skills/ and the docs-index gate rejected its
// SKILL.md. .agents must stay a real directory, and every marker must sit at
// .agents/skills/<name>/.synced-from-core, with a Source line for that name.
func TestSyncedSkillsLiveInAgentsSkills(t *testing.T) {
	root := RepoRoot()

	info, err := os.Lstat(filepath.Join(root, ".agents"))
	if err != nil {
		t.Fatalf("lstat .agents: %v", err)
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		t.Errorf(".agents must be a real directory, not %v: core's skills sync follows a symlink to its target", info.Mode().Type())
	}

	var markers []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			// A nested checkout (an agent worktree under .claude/worktrees/)
			// is another tree, not part of this one.
			if path != root {
				if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if d.Name() == syncedSkillMarker {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			markers = append(markers, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	if len(markers) == 0 {
		t.Fatal("found no .synced-from-core marker; the walk or the skills layout changed")
	}

	for _, rel := range markers {
		parts := strings.Split(rel, "/")
		if len(parts) != 4 || parts[0] != ".agents" || parts[1] != "skills" {
			t.Errorf("%s: a synced skill must be at .agents/skills/<name>/%s", rel, syncedSkillMarker)
			continue
		}
		name := parts[2]
		marker := readRepoFile(t, rel)
		wantSource := "Source: https://github.com/frostyard/core/tree/main/.agents/skills/" + name
		if !slices.Contains(strings.Split(marker, "\n"), wantSource) {
			t.Errorf("%s: marker has no line %q", rel, wantSource)
		}
	}
}
