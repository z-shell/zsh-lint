package survey

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/parse"
)

// requiredFixtures is the minimal baseline that must always exist. It guards
// against accidental deletion without asserting a brittle total count
// (issue #14): new fixtures can be added freely without touching this test.
var requiredFixtures = []string{
	"gap-11-param-expansion-flags.zsh",
	"gap-12-brace-termination.zsh",
	"gap-13-multi-name-loop.zsh",
	"gap-15-reverse-subscript.zsh",
	"gap-16-glob-patterns.zsh",
	"gap-53-nested-param-expansion.zsh",
	"ok-baseline.zsh",
}

// Fixture naming contract from docs/project/parser-gap-workflow.md:
// gap-<issue>-<slug>.zsh must fail to parse (a known, tracked parser gap)
// and ok-<slug>.zsh must parse.
var (
	gapName = regexp.MustCompile(`^gap-[0-9]+(-[a-z0-9]+)+\.zsh$`)
	okName  = regexp.MustCompile(`^ok(-[a-z0-9]+)+\.zsh$`)
)

// TestMinimizedCorpus scans the testdata/corpus directory and enforces the
// fixture naming contract above. Fixtures are discovered by listing the
// directory so the corpus can grow without test edits.
func TestMinimizedCorpus(t *testing.T) {
	dir := filepath.Join("testdata", "corpus")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".zsh") {
			continue
		}
		seen[name] = true
		path := filepath.Join(dir, name)
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening %s: %v", path, err)
		}
		_, perr := parse.Parse(f, path)
		f.Close()
		switch {
		case gapName.MatchString(name):
			if perr == nil {
				t.Errorf("%s: parsed cleanly, but gap-* fixtures must fail; "+
					"the parser gap may be fixed — promote to ok-* and close the issue", name)
			}
		case okName.MatchString(name):
			if perr != nil {
				t.Errorf("%s: failed to parse, but ok-* fixtures must parse: %v", name, perr)
			}
		default:
			t.Errorf("%s: fixture name must match gap-<issue>-<slug>.zsh or ok-<slug>.zsh", name)
		}
	}
	for _, want := range requiredFixtures {
		if !seen[want] {
			t.Errorf("required baseline fixture missing: %s", want)
		}
	}
}
