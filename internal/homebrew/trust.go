package homebrew

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UntrustedTap describes an untrusted tap and the packages installed from it.
// Package names are fully qualified (user/tap/name), ready for `brew trust`.
type UntrustedTap struct {
	Name     string
	Formulae []string
	Casks    []string
}

// parseUntrustedTapNames extracts names of untrusted taps from
// `brew tap-info --installed --json` output (Homebrew 6 adds "trusted").
func parseUntrustedTapNames(data []byte) ([]string, error) {
	var taps []struct {
		Name    string `json:"name"`
		Trusted bool   `json:"trusted"`
	}
	if err := json.Unmarshal(data, &taps); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse tap-info JSON: %v", err)}
	}
	var names []string
	for _, t := range taps {
		if !t.Trusted {
			names = append(names, t.Name)
		}
	}
	return names, nil
}

// installedFormulaeByTap maps tap name -> qualified installed formula names
// by reading Cellar keg INSTALL_RECEIPT.json files. This is the only
// reliable source: brew itself refuses to load (and therefore list)
// formulae from untrusted taps.
func installedFormulaeByTap(cellarDir string) map[string][]string {
	byTap := make(map[string][]string)
	kegs, err := os.ReadDir(cellarDir)
	if err != nil {
		return byTap
	}
	for _, keg := range kegs {
		if !keg.IsDir() {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(cellarDir, keg.Name()))
		if err != nil {
			continue
		}
		for _, v := range versions {
			receiptPath := filepath.Join(cellarDir, keg.Name(), v.Name(), "INSTALL_RECEIPT.json")
			data, err := os.ReadFile(receiptPath)
			if err != nil {
				continue
			}
			var receipt struct {
				Source struct {
					Tap string `json:"tap"`
				} `json:"source"`
			}
			if err := json.Unmarshal(data, &receipt); err != nil || receipt.Source.Tap == "" {
				continue
			}
			byTap[receipt.Source.Tap] = append(byTap[receipt.Source.Tap], receipt.Source.Tap+"/"+keg.Name())
			break // one receipt per keg is enough
		}
	}
	return byTap
}

// installedCasksByTap maps tap name -> qualified installed cask tokens by
// reading Caskroom metadata. Casks installed from the Homebrew API have no
// Casks/<token>.json metadata and are skipped (they are homebrew/cask,
// which is always trusted).
func installedCasksByTap(caskroomDir string) map[string][]string {
	byTap := make(map[string][]string)
	entries, err := os.ReadDir(caskroomDir)
	if err != nil {
		return byTap
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		token := entry.Name()
		matches, err := filepath.Glob(filepath.Join(caskroomDir, token, ".metadata", "*", "*", "Casks", "*.json"))
		if err != nil || len(matches) == 0 {
			continue
		}
		data, err := os.ReadFile(matches[len(matches)-1]) // newest metadata last in sorted glob
		if err != nil {
			continue
		}
		var meta struct {
			Tap string `json:"tap"`
		}
		if err := json.Unmarshal(data, &meta); err != nil || meta.Tap == "" {
			continue
		}
		byTap[meta.Tap] = append(byTap[meta.Tap], meta.Tap+"/"+token)
	}
	return byTap
}

// brewPrefix returns Homebrew's installation prefix.
func brewPrefix() (string, error) {
	output, err := runBrewCommand("--prefix")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// ListUntrustedTaps returns untrusted taps that have at least one package
// installed, with qualified package names ready for `brew trust`.
func ListUntrustedTaps() ([]UntrustedTap, error) {
	output, err := runBrewCommand("tap-info", "--installed", "--json")
	if err != nil {
		return nil, err
	}
	names, err := parseUntrustedTapNames([]byte(output))
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}

	prefix, err := brewPrefix()
	if err != nil {
		return nil, err
	}
	formulaeByTap := installedFormulaeByTap(filepath.Join(prefix, "Cellar"))
	casksByTap := installedCasksByTap(filepath.Join(prefix, "Caskroom"))

	var result []UntrustedTap
	for _, name := range names {
		tap := UntrustedTap{
			Name:     name,
			Formulae: formulaeByTap[name],
			Casks:    casksByTap[name],
		}
		if len(tap.Formulae) == 0 && len(tap.Casks) == 0 {
			continue // nothing installed from this tap; not actionable
		}
		sort.Strings(tap.Formulae)
		sort.Strings(tap.Casks)
		result = append(result, tap)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
