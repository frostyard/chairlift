package installcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// makeRuleTarget matches the target list of a rule line: text from column 0
// up to a single or double colon that does not begin an assignment (`:=`,
// `::=`). Recipe lines start with a tab and comments with '#', so neither
// matches, and neither do `=`/`?=` assignments.
var makeRuleTarget = regexp.MustCompile(`^([^\s#=:][^=:]*?)::?(?:[^:=]|$)`)

// parseMakefileRules returns the target of every rule in a Makefile's source,
// in order, and every name its .PHONY rules declare. Backslash-continued
// lines are joined first, trailing comments on .PHONY lines are ignored, and
// special targets (names beginning with '.') are not reported as targets.
func parseMakefileRules(src string) (targets, phony []string) {
	src = strings.ReplaceAll(src, "\\\n", " ")
	for line := range strings.SplitSeq(src, "\n") {
		m := makeRuleTarget.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		names := strings.Fields(m[1])
		if slices.Equal(names, []string{".PHONY"}) {
			_, prereqs, _ := strings.Cut(line, ":")
			prereqs, _, _ = strings.Cut(prereqs, "#")
			phony = append(phony, strings.Fields(prereqs)...)
			continue
		}
		for _, name := range names {
			if !strings.HasPrefix(name, ".") {
				targets = append(targets, name)
			}
		}
	}
	return targets, phony
}

func TestParseMakefileRulesFixture(t *testing.T) {
	const src = `.PHONY: alpha \
	beta # trailing comment
.PHONY: gamma
VAR := value:with:colons
OTHER ::= x
PREFIX ?= /usr
alpha: beta ## help text
	@echo "recipe: not a target"
# comment: not a target
beta gamma::
.SUFFIXES:
file.txt:
`
	targets, phony := parseMakefileRules(src)

	if want := []string{"alpha", "beta", "gamma", "file.txt"}; !slices.Equal(targets, want) {
		t.Errorf("targets = %q, want %q", targets, want)
	}
	if want := []string{"alpha", "beta", "gamma"}; !slices.Equal(phony, want) {
		t.Errorf("phony = %q, want %q", phony, want)
	}
}

// TestMakefileTargetsArePhony requires the Makefile's .PHONY declarations to
// name exactly its rule targets. No target builds a file named after itself,
// so make must never compare one against the file system: before every
// target was declared, the top-level test/ directory made `make test`, and
// therefore `make bump`, print "'test' is up to date" and run no tests while
// v0.11.2 was tagged. A rule that genuinely builds a file named after its
// target would need an explicit exemption here.
func TestMakefileTargetsArePhony(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(RepoRoot(), "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	targets, phony := parseMakefileRules(string(src))

	// A parser that silently found nothing would pass the loops below.
	for _, want := range []string{"build", "test", "e2e", "ci", "bump"} {
		if !slices.Contains(targets, want) {
			t.Fatalf("parsed Makefile targets %q do not include %q; the rule parser no longer matches the Makefile", targets, want)
		}
	}

	for _, target := range targets {
		if !slices.Contains(phony, target) {
			t.Errorf("Makefile target %q is not declared .PHONY, so a file or directory named %q at the repository root can make make skip it as up to date", target, target)
		}
	}
	for _, name := range phony {
		if !slices.Contains(targets, name) {
			t.Errorf(".PHONY declares %q, but no Makefile rule defines it", name)
		}
	}
}
