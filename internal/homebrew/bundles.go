package homebrew

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxBundleDescriptionBytes = 64 * 1024

// Bundle describes one installable Brewfile discovered from configured
// bundle directories.
type Bundle struct {
	Name        string
	Description string
	Path        string
}

// AvailableBundles discovers regular *.Brewfile entries immediately inside
// each configured directory. Missing directories are ignored because bundle
// paths may target a different distribution variant. Other path and file
// errors are joined and returned alongside any bundles that were discovered.
//
// Exact duplicate paths are emitted once. Same-named files in different
// directories remain distinct and are ordered by name, then absolute path.
func AvailableBundles(paths []string) ([]Bundle, error) {
	var (
		bundles  []Bundle
		problems []error
		seen     = make(map[string]struct{})
	)

	for _, configuredPath := range paths {
		if configuredPath == "" {
			problems = append(problems, errors.New("bundle directory path is empty"))
			continue
		}

		dir, err := filepath.Abs(configuredPath)
		if err != nil {
			problems = append(problems, fmt.Errorf("resolve bundle directory %q: %w", configuredPath, err))
			continue
		}
		dir = filepath.Clean(dir)

		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			problems = append(problems, fmt.Errorf("read bundle directory %q: %w", dir, err))
			continue
		}

		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".Brewfile") {
				continue
			}

			path := filepath.Join(dir, entry.Name())
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}

			info, err := os.Stat(path)
			if err != nil {
				problems = append(problems, fmt.Errorf("inspect Brewfile %q: %w", path, err))
				continue
			}
			if !info.Mode().IsRegular() {
				continue
			}

			description, err := readBundleDescription(path)
			if err != nil {
				problems = append(problems, fmt.Errorf("read Brewfile %q: %w", path, err))
				continue
			}

			name := strings.TrimSuffix(entry.Name(), ".Brewfile")
			if name == "" {
				name = entry.Name()
			}
			bundles = append(bundles, Bundle{
				Name:        name,
				Description: description,
				Path:        path,
			})
		}
	}

	sort.Slice(bundles, func(i, j int) bool {
		if bundles[i].Name != bundles[j].Name {
			return bundles[i].Name < bundles[j].Name
		}
		return bundles[i].Path < bundles[j].Path
	})

	return bundles, errors.Join(problems...)
}

func readBundleDescription(path string) (description string, resultErr error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
	}()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), maxBundleDescriptionBytes)
	if !scanner.Scan() {
		return "", scanner.Err()
	}

	line := strings.TrimSpace(scanner.Text())
	if !strings.HasPrefix(line, "#") {
		return "", nil
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "#")), nil
}
