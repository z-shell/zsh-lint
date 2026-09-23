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
	"ok-alternate-for-multiline-parens.zsh",
	"ok-alternate-for-then-anonymous-args-in-function.zsh",
	"ok-alternate-if-brace-continuation.zsh",
	"ok-alternate-if-brace-length-expansion.zsh",
	"ok-alternate-if-brace.zsh",
	"ok-alternate-if-bracket-class-close.zsh",
	"ok-alternate-if-condition-brace.zsh",
	"ok-alternate-if-nested-in-classic-if.zsh",
	"ok-anonymous-function-arguments.zsh",
	"ok-ansic-heredoc.zsh",
	"ok-arith-for-sublist.zsh",
	"ok-assign-always.zsh",
	"ok-assoc-key-dot-assignment.zsh",
	"ok-assoc-key-doubled-sign.zsh",
	"ok-assoc-key-expansion-colon.zsh",
	"ok-assoc-key-hyphen-punctuation.zsh",
	"ok-assoc-key-leading-angle.zsh",
	"ok-assoc-subscript-keys.zsh",
	"ok-baseline.zsh",
	"ok-brace-decl-termination.zsh",
	"ok-brace-termination.zsh",
	"ok-cond-group-quoted-paren.zsh",
	"ok-decl-brace-close-in-body.zsh",
	"ok-dangling-and-or.zsh",
	"ok-do-leading-separator.zsh",
	"ok-fd-var-redirect.zsh",
	"ok-flag-pattern-bracket-before-operator.zsh",
	"ok-flagged-second-subscript-bracket.zsh",
	"ok-function-non-brace-body.zsh",
	"ok-function-semicolon-body.zsh",
	"ok-glob-patterns.zsh",
	"ok-grouped-case-pattern.zsh",
	"ok-if-short-form.zsh",
	"ok-loop-short-form.zsh",
	"ok-math-function-call-ternary.zsh",
	"ok-math-function-call.zsh",
	"ok-multi-name-function.zsh",
	"ok-multi-name-loop.zsh",
	"ok-multi-name-paren-for.zsh",
	"ok-nested-arithmetic-expansion.zsh",
	"ok-nested-conditional-alternation.zsh",
	"ok-nested-param-expansion.zsh",
	"ok-param-expansion-flags.zsh",
	"ok-param-glob-toggle.zsh",
	"ok-paren-semicolon-body.zsh",
	"ok-rc-expand-caret.zsh",
	"ok-redundant-separator.zsh",
	"ok-repeat.zsh",
	"ok-repeat-synthesized-closer.zsh",
	"ok-reverse-subscript-pattern-slash.zsh",
	"ok-reverse-subscript.zsh",
	"ok-select-short-form.zsh",
	"ok-subscript-pattern-after-comma.zsh",
	"ok-try-always.zsh",
	"ok-while-condition-list.zsh",
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
		_ = f.Close()
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
