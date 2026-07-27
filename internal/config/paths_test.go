package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func withPathResolutionSeams(
	t *testing.T,
	executable func() (string, error),
	cwd func() (string, error),
) {
	t.Helper()
	originalExecutablePath := executablePath
	originalWorkingDir := workingDir
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		workingDir = originalWorkingDir
	})
	executablePath = executable
	workingDir = cwd
}

func withStatPath(t *testing.T, stat func(string) (os.FileInfo, error)) {
	t.Helper()
	original := statPath
	t.Cleanup(func() { statPath = original })
	statPath = stat
}

func TestResolveCandidatePathBranches(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "bin")
	cwd := filepath.Join(root, "work")
	if err := os.MkdirAll(exeDir, 0o755); err != nil {
		t.Fatalf("creating executable directory: %v", err)
	}
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("creating working directory: %v", err)
	}
	executable := filepath.Join(exeDir, "chairlift")

	t.Run("absolute input is cleaned", func(t *testing.T) {
		withPathResolutionSeams(t,
			func() (string, error) { return executable, nil },
			func() (string, error) { return cwd, nil },
		)
		input := filepath.Join(root, "one", "..", "config.yml")
		if got, want := resolveCandidatePath(input), filepath.Clean(input); got != want {
			t.Fatalf("resolveCandidatePath(%q) = %q, want %q", input, got, want)
		}
	})

	t.Run("executable-relative file wins", func(t *testing.T) {
		withPathResolutionSeams(t,
			func() (string, error) { return executable, nil },
			func() (string, error) { return cwd, nil },
		)
		want := filepath.Join(exeDir, "config.yml")
		if err := os.WriteFile(want, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("writing executable-relative config: %v", err)
		}
		if got := resolveCandidatePath("config.yml"); got != want {
			t.Fatalf("resolveCandidatePath(config.yml) = %q, want %q", got, want)
		}
	})

	t.Run("missing executable-relative file uses cwd", func(t *testing.T) {
		missingExeDir := filepath.Join(root, "missing-bin")
		withPathResolutionSeams(t,
			func() (string, error) { return filepath.Join(missingExeDir, "chairlift"), nil },
			func() (string, error) { return cwd, nil },
		)
		want := filepath.Join(cwd, "config.yml")
		if got := resolveCandidatePath("config.yml"); got != want {
			t.Fatalf("resolveCandidatePath(config.yml) = %q, want %q", got, want)
		}
		if !filepath.IsAbs(want) {
			t.Fatalf("cwd fallback %q is not absolute", want)
		}
	})

	t.Run("inaccessible executable-relative file remains authoritative", func(t *testing.T) {
		withPathResolutionSeams(t,
			func() (string, error) { return executable, nil },
			func() (string, error) { return cwd, nil },
		)
		withStatPath(t, func(path string) (os.FileInfo, error) {
			return nil, &os.PathError{Op: "stat", Path: path, Err: fs.ErrPermission}
		})
		want := filepath.Join(exeDir, "config.yml")
		if got := resolveCandidatePath("config.yml"); got != want {
			t.Fatalf("resolveCandidatePath(config.yml) = %q, want inaccessible authoritative path %q", got, want)
		}
	})

	t.Run("working-directory failure preserves cleaned relative path", func(t *testing.T) {
		withPathResolutionSeams(t,
			func() (string, error) { return "", errors.New("no executable") },
			func() (string, error) { return "", errors.New("no working directory") },
		)
		if got, want := resolveCandidatePath("./config.yml"), "config.yml"; got != want {
			t.Fatalf("resolveCandidatePath(./config.yml) = %q, want %q", got, want)
		}
	})
}

func TestLoadFromPathReadsResolvedCandidate(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "bin")
	cwd := filepath.Join(root, "work")
	if err := os.MkdirAll(exeDir, 0o755); err != nil {
		t.Fatalf("creating executable directory: %v", err)
	}
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("creating working directory: %v", err)
	}
	withPathResolutionSeams(t,
		func() (string, error) { return filepath.Join(exeDir, "chairlift"), nil },
		func() (string, error) { return cwd, nil },
	)

	exeConfig := filepath.Join(exeDir, "config.yml")
	cwdConfig := filepath.Join(cwd, "config.yml")
	if err := os.WriteFile(exeConfig, []byte("system_page:\n  health_group:\n    app_id: exe\n"), 0o600); err != nil {
		t.Fatalf("writing executable-relative config: %v", err)
	}
	if err := os.WriteFile(cwdConfig, []byte("system_page:\n  health_group:\n    app_id: cwd\n"), 0o600); err != nil {
		t.Fatalf("writing cwd config: %v", err)
	}

	cfg, loadErr := loadFromPath("config.yml")
	if loadErr != nil {
		t.Fatalf("loadFromPath executable-relative candidate: %v", loadErr)
	}
	if got := cfg.SystemPage["health_group"].AppID; got != "exe" {
		t.Fatalf("executable-relative AppID = %q, want %q", got, "exe")
	}

	if err := os.Remove(exeConfig); err != nil {
		t.Fatalf("removing executable-relative config: %v", err)
	}
	cfg, loadErr = loadFromPath("config.yml")
	if loadErr != nil {
		t.Fatalf("loadFromPath cwd candidate: %v", loadErr)
	}
	if got := cfg.SystemPage["health_group"].AppID; got != "cwd" {
		t.Fatalf("cwd AppID = %q, want %q", got, "cwd")
	}
}
