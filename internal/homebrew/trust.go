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
// `brew tap-info --installed --json` output. The "trusted" key was added in
// Homebrew 6; older brew versions omit it entirely (and have no `brew
// trust` command), so a nil/missing field must be treated as trusted rather
// than defaulting to false, or every tap on pre-6 brew would be reported as
// untrusted.
func parseUntrustedTapNames(data []byte) ([]string, error) {
	var taps []struct {
		Name    string `json:"name"`
		Trusted *bool  `json:"trusted"`
	}
	if err := json.Unmarshal(data, &taps); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse tap-info JSON: %v", err)}
	}
	var names []string
	for _, t := range taps {
		if t.Trusted != nil && !*t.Trusted {
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
// reading each cask's install receipt at
// <prefix>/Caskroom/<token>/.metadata/INSTALL_RECEIPT.json (written by
// Homebrew's Cask::Tab.create) and taking its source.tap field, the
// authoritative origin recorded at install time. This mirrors
// installedFormulaeByTap: brew itself refuses to load (and therefore list)
// casks from untrusted taps. Versioned .metadata/*/*/Casks/<token>.json
// metadata is not used: a cask installed from a tap is saved as a .rb
// Caskfile, so that JSON never exists for exactly the untrusted-tap casks the
// remediation UI exists for. Tokens without a readable receipt or with an
// empty source.tap are skipped.
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
		data, err := os.ReadFile(filepath.Join(caskroomDir, token, ".metadata", "INSTALL_RECEIPT.json"))
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
		byTap[receipt.Source.Tap] = append(byTap[receipt.Source.Tap], receipt.Source.Tap+"/"+token)
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

// UntrustedTapError indicates a brew command failed because packages come
// from untrusted taps (Homebrew 6 tap trust). Views should point users at
// the Untrusted Taps UI instead of dumping raw brew output.
type UntrustedTapError struct {
	Message string
}

func (e *UntrustedTapError) Error() string {
	return e.Message
}

// isUntrustedTapMessage reports whether brew output complains about
// untrusted taps.
func isUntrustedTapMessage(s string) bool {
	if s == "" {
		return false
	}
	return strings.Contains(s, "untrusted tap") || strings.Contains(s, "taps are not trusted")
}

// TrustPackages trusts every installed package from the given tap using
// `brew trust`. Trust is per-user (~/.homebrew/trust.json); no root needed.
func TrustPackages(tap UntrustedTap) error {
	if len(tap.Formulae) > 0 {
		args := append([]string{"trust", "--formula"}, tap.Formulae...)
		if _, err := runBrewCommand(args...); err != nil {
			return err
		}
	}
	if len(tap.Casks) > 0 {
		args := append([]string{"trust", "--cask"}, tap.Casks...)
		if _, err := runBrewCommand(args...); err != nil {
			return err
		}
	}
	return nil
}
