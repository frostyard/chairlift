package installcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakefileBumpGuard(t *testing.T) {
	for _, tool := range []string{"make", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed; skipping Makefile bump guard behavior check", tool)
		}
	}

	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "main at origin"},
		{name: "feature", want: "only on main"},
		{name: "detached", want: "only on main"},
		{name: "ahead", want: "HEAD is not origin/main"},
		{name: "behind", want: "HEAD is not origin/main"},
		{name: "fetch fails", want: "git fetch origin main failed"},
		{name: "tag origin/main", want: "HEAD is not origin/main"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Exclude inherited Git and make controls so only these temporary
			// repositories can be selected and the real recipe always executes.
			var env []string
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				if strings.HasPrefix(key, "GIT_") || key == "MAKEFLAGS" || key == "MFLAGS" || key == "MAKEFILES" {
					continue
				}
				env = append(env, entry)
			}
			env = append(env,
				"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
				"GIT_AUTHOR_NAME=Bump Guard Test", "GIT_AUTHOR_EMAIL=bump@example.invalid",
				"GIT_COMMITTER_NAME=Bump Guard Test", "GIT_COMMITTER_EMAIL=bump@example.invalid",
			)
			runGit := func(dir string, args ...string) {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = dir
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
				}
			}
			root := t.TempDir()
			origin := filepath.Join(root, "origin.git")
			seed := filepath.Join(root, "seed")
			work := filepath.Join(root, "work")
			runGit(root, "init", "--bare", "-b", "main", origin)
			// Protocol v0 does not advertise the bare repository's unborn HEAD, so
			// the clone uses init.defaultBranch, as Git before 2.31 does.
			runGit(root, "-c", "init.defaultBranch=trunk", "-c", "protocol.version=0", "clone", origin, seed)
			runGit(seed, "checkout", "-B", "main")
			commitFile := func(dir, content string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				runGit(dir, "add", "file.txt")
				runGit(dir, "commit", "-m", "Update fixture")
			}
			commitFile(seed, "initial\n")
			runGit(seed, "push", "origin", "main")
			runGit(root, "clone", origin, work)
			switch tc.name {
			case "feature":
				runGit(work, "checkout", "-b", "feature")
			case "detached":
				runGit(work, "checkout", "--detach")
			case "ahead":
				commitFile(work, "local\n")
			case "behind":
				commitFile(seed, "remote\n")
				runGit(seed, "push", "origin", "main")
			case "fetch fails":
				runGit(work, "remote", "set-url", "origin", filepath.Join(root, "missing.git"))
			case "tag origin/main":
				commitFile(work, "local\n")
				runGit(work, "tag", "origin/main")
			}

			cmd := exec.Command("make", "-f", filepath.Join(RepoRoot(), "Makefile"), "bump-guard")
			cmd.Dir = RepoRoot()
			cmd.Env = append(env, "GIT_DIR="+filepath.Join(work, ".git"), "GIT_WORK_TREE="+work)
			out, err := cmd.CombinedOutput()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("bump-guard rejected main at origin/main: %v\n%s", err, out)
				}
			} else if err == nil || !strings.Contains(string(out), tc.want) {
				t.Fatalf("bump-guard error = %v, output = %s; want failure containing %q", err, out, tc.want)
			}
		})
	}
}

func TestMakefileBumpRunsGuardFirst(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(RepoRoot(), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	var recipe []string
	inBump := false
	for _, line := range strings.Split(string(data), "\n") {
		if !inBump {
			if strings.HasPrefix(line, "bump:") {
				inBump = true
			}
			continue
		}
		if !strings.HasPrefix(line, "\t") {
			break
		}
		recipe = append(recipe, strings.TrimPrefix(line, "\t"))
	}
	if len(recipe) == 0 || recipe[0] != "@$(MAKE) bump-guard" {
		t.Fatalf("bump must run bump-guard first; recipe = %q", recipe)
	}
	for i, line := range recipe {
		if strings.Contains(line, "svu next") {
			if i == 0 {
				t.Fatal("bump-guard must run before svu next")
			}
			return
		}
	}
	t.Fatal("bump recipe has no svu next line")
}
