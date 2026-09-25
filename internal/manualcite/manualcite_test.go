// Package manualcite checks that rules and parser fixtures cite the released
// Zsh manual (docs/project/rule-policy.md, "Manual grounding";
// docs/project/parser-gap-workflow.md, "Classify"). It holds tests only.
package manualcite

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/rules"
)

// manualBase is the released manual. Its index page names Zsh 5.9.2, the
// baseline recorded in z-shell/.github lib/zsh-standard-policy.json.
const manualBase = "https://zsh.sourceforge.io/Doc/Release/"

// pluginStandard is the other grounding the rule policy accepts.
const pluginStandard = "https://wiki.zshell.dev/community/zsh_plugin_standard"

// manualPages lists every chapter page linked from the Zsh 5.9.2 manual index
// (https://zsh.sourceforge.io/Doc/Release/index.html), without the index
// pages. A citation to any other page is a typo or an invented reference.
// Refresh the list when the baseline release changes.
var manualPages = map[string]bool{
	"Arithmetic-Evaluation.html":    true,
	"Calendar-Function-System.html": true,
	"Command-Execution.html":        true,
	"Completion-System.html":        true,
	"Completion-Using-compctl.html": true,
	"Completion-Widgets.html":       true,
	"Conditional-Expressions.html":  true,
	"Expansion.html":                true,
	"Files.html":                    true,
	"Functions.html":                true,
	"Introduction.html":             true,
	"Invocation.html":               true,
	"Jobs-_0026-Signals.html":       true,
	"Options.html":                  true,
	"Parameters.html":               true,
	"Prompt-Expansion.html":         true,
	"Redirection.html":              true,
	"Roadmap.html":                  true,
	"Shell-Builtin-Commands.html":   true,
	"Shell-Grammar.html":            true,
	"TCP-Function-System.html":      true,
	"The-Z-Shell-Manual.html":       true,
	"User-Contributions.html":       true,
	"Zftp-Function-System.html":     true,
	"Zsh-Line-Editor.html":          true,
	"Zsh-Modules.html":              true,
}

var (
	reURL            = regexp.MustCompile(`https?://[^\s<>()"'` + "`" + `]+`)
	reDocID          = regexp.MustCompile("\nID: `([^`]+)`")
	reFixtureCite    = regexp.MustCompile(`^#\s*Manual:\s*(\S+)\s*$`)
	reCitedFixture   = regexp.MustCompile(`^(gap|ok)-.*\.zsh$|^invalid-.*\.txt$`)
	exemptionsRecord = filepath.Join("testdata", "fixture-exemptions.txt")
)

// exemptionCeiling is the most entries fixture-exemptions.txt may hold. Lower
// it when fixtures gain citations; never raise it.
const exemptionCeiling = 144

// isPluginStandardURL reports whether raw links the Plugin Standard page.
func isPluginStandardURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "wiki.zshell.dev" &&
		strings.TrimSuffix(u.Path, "/") == "/community/zsh_plugin_standard"
}

// checkManualURL reports why raw is not a citation of a released manual page.
func checkManualURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.Host != "zsh.sourceforge.io" || !strings.HasPrefix(u.Path, "/Doc/Release/") {
		return fmt.Errorf("%s is not under %s", raw, manualBase)
	}
	page := strings.TrimPrefix(u.Path, "/Doc/Release/")
	if !manualPages[page] {
		return fmt.Errorf("%s names no chapter page of the Zsh 5.9.2 manual", raw)
	}
	return nil
}

func repositoryPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate manual citation test")
	}
	root := filepath.Join(filepath.Dir(testFile), "..", "..")
	return filepath.Join(append([]string{root}, parts...)...)
}

func TestCheckManualURL(t *testing.T) {
	tests := []struct {
		raw string
		ok  bool
	}{
		{"https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands", true},
		{"https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html", true},
		{"http://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html", false},
		{"https://zsh.sourceforge.io/Doc/Release/Shell-Grammer.html", false},
		{"https://zsh.sourceforge.io/Guide/zshguide02.html", false},
		{"https://www.gnu.org/software/bash/manual/bash.html", false},
	}
	for _, tt := range tests {
		if err := checkManualURL(tt.raw); (err == nil) != tt.ok {
			t.Errorf("checkManualURL(%q) = %v, want ok=%v", tt.raw, err, tt.ok)
		}
	}
}

// registeredRuleIDs returns the IDs of every rule the default set or the
// current project profile registers.
func registeredRuleIDs(t *testing.T) map[string]bool {
	t.Helper()
	profile, err := rules.ForProfile(rules.CurrentProjectProfile)
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]bool)
	for _, rule := range append(rules.Default(), profile...) {
		ids[string(rule.ID())] = true
	}
	return ids
}

// TestRulesCiteGrounding requires every registered rule to carry a documented
// type in internal/rules that links the Zsh manual or the Plugin Standard, and
// every manual link to name a real chapter page.
func TestRulesCiteGrounding(t *testing.T) {
	dir := repositoryPath(t, "internal", "rules")
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	documented := make(map[string]bool)
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE || gen.Doc == nil {
				continue
			}
			doc := gen.Doc.Text()
			m := reDocID.FindStringSubmatch(doc)
			if m == nil {
				continue
			}
			name := gen.Specs[0].(*ast.TypeSpec).Name.Name
			documented[m[1]] = true
			grounded := false
			for _, raw := range reURL.FindAllString(doc, -1) {
				raw = strings.TrimRight(raw, ".,;:")
				switch {
				case isPluginStandardURL(raw):
					grounded = true
				case strings.Contains(raw, "zsh.sourceforge.io"):
					if err := checkManualURL(raw); err != nil {
						t.Errorf("%s (%s): %v", name, filepath.Base(path), err)
						continue
					}
					grounded = true
				}
			}
			if !grounded {
				t.Errorf("%s (%s): doc comment cites neither %s nor %s", name, filepath.Base(path), manualBase, pluginStandard)
			}
		}
	}
	for id := range registeredRuleIDs(t) {
		if !documented[id] {
			t.Errorf("registered rule %s has no doc comment with an `ID: `%s`` line, so its grounding is unchecked", id, id)
		}
	}
}

// fixturePaths returns the parser fixtures that must cite the manual, relative
// to internal/.
func fixturePaths(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, dir := range []string{
		filepath.Join("survey", "testdata", "corpus"),
		filepath.Join("parse", "testdata"),
	} {
		entries, err := os.ReadDir(repositoryPath(t, "internal", dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && reCitedFixture.MatchString(entry.Name()) {
				out = append(out, filepath.ToSlash(filepath.Join(dir, entry.Name())))
			}
		}
	}
	sort.Strings(out)
	return out
}

// fixtureCitations returns the `# Manual: <url>` lines of a fixture.
func fixtureCitations(t *testing.T, rel string) []string {
	t.Helper()
	data, err := os.ReadFile(repositoryPath(t, "internal", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	var cites []string
	for _, line := range strings.Split(string(data), "\n") {
		if m := reFixtureCite.FindStringSubmatch(line); m != nil {
			cites = append(cites, m[1])
		}
	}
	return cites
}

func readExemptions(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(exemptionsRecord)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if out[line] {
			t.Errorf("%s lists %s twice", exemptionsRecord, line)
		}
		out[line] = true
	}
	return out
}

// TestFixturesCiteManual requires every gap-, ok- and invalid- parser fixture
// to carry a `# Manual: <url>` line naming a released manual page. Fixtures
// that predate the requirement are listed in testdata/fixture-exemptions.txt.
// The list only shrinks: an entry that gains a citation or loses its file
// fails until it is removed.
func TestFixturesCiteManual(t *testing.T) {
	exempt := readExemptions(t)
	if len(exempt) > exemptionCeiling {
		t.Errorf("%s holds %d entries, more than the ceiling of %d; cite the manual in new fixtures instead of exempting them", exemptionsRecord, len(exempt), exemptionCeiling)
	}
	seen := make(map[string]bool)
	for _, rel := range fixturePaths(t) {
		seen[rel] = true
		cites := fixtureCitations(t, rel)
		for _, raw := range cites {
			if err := checkManualURL(raw); err != nil {
				t.Errorf("%s: %v", rel, err)
			}
		}
		switch {
		case len(cites) == 0 && !exempt[rel]:
			t.Errorf("%s: add a `# Manual: %s<Page>.html#<Section>` line naming the section that defines the construct", rel, manualBase)
		case len(cites) > 0 && exempt[rel]:
			t.Errorf("%s now cites the manual; remove it from %s", rel, exemptionsRecord)
		}
	}
	for rel := range exempt {
		if !seen[rel] {
			t.Errorf("%s lists %s, which is not a fixture; remove it", exemptionsRecord, rel)
		}
	}
}
