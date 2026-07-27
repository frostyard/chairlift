package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

var (
	executablePath = os.Executable
	workingDir     = os.Getwd
	statPath       = os.Stat
)

// resolveCandidatePath returns the exact path loadFromPath will read and a
// diagnostic will report. Relative candidates prefer a file alongside the
// executable, preserving ChairLift's existing development behavior, then
// fall back to an absolute path under the current working directory.
func resolveCandidatePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	if executable, err := executablePath(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), path)
		if _, err := statPath(candidate); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return filepath.Clean(candidate)
		}
	}

	if cwd, err := workingDir(); err == nil {
		return filepath.Join(cwd, path)
	}

	// A failed Getwd leaves no absolute base to use. Keep the cleaned relative
	// path so the subsequent read still returns a classified, actionable error.
	return filepath.Clean(path)
}
