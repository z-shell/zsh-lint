package survey

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/parse"
)

// structuralOracleKnownDifferences lists the ok-* corpus fixtures whose
// printed tree Zsh does not read as the same program, with the reason (#541).
// The list only shrinks: a fixture that starts to agree fails the test until
// its entry is removed, and a new fixture must agree. The test reports the
// first difference in a file, so a listed fixture can hide a second cause
// until the first is fixed.
var structuralOracleKnownDifferences = map[string]string{
	"ok-function-keyword-empty-body.zsh": "printer: `function name` and a non-brace body on one line read as more names",
	"ok-function-non-brace-body.zsh":     "printer: `function name` and a non-brace body on one line read as more names",
	"ok-brace-words.zsh":                 "printer: a case pattern holding `}` loses its opening parenthesis",
	"ok-ansic-heredoc.zsh":               "printer: a $'...' here-document delimiter is closed by its quoted form",
	// The tree keeps only the first name of a multi-name loop.
	"ok-foreach-end.zsh":                      "tree: WordIter keeps one loop name",
	"ok-heredoc-quote-before-brace-forms.zsh": "tree: WordIter keeps one loop name",
	"ok-loop-short-and-alternate-forms.zsh":   "tree: WordIter keeps one loop name",
	"ok-multi-name-loop.zsh":                  "tree: WordIter keeps one loop name",
	"ok-multi-name-paren-for.zsh":             "tree: WordIter keeps one loop name",
	"ok-select-paren-list.zsh":                "tree: `select o () list` is read as an empty word list, not an anonymous-function body",
	"ok-dangling-and-or.zsh":                  "tree: the dangling-operator adapter drops the `&&` that Zsh joins across `;`",
	// The try/always block loses its always keyword (#273).
	"ok-alternate-if-while-try-always.zsh":       "tree: always keyword lost (#273)",
	"ok-brace-decl-termination.zsh":              "tree: always keyword lost (#273)",
	"ok-here-string-in-always-block.zsh":         "tree: always keyword lost (#273)",
	"ok-quote-in-double-quoted-substitution.zsh": "tree: always keyword lost (#273)",
	"ok-try-always.zsh":                          "tree: always keyword lost (#273)",
	// The construct is held in parse.File metadata, which the printer does
	// not see.
	"ok-alternate-for-then-anonymous-args-in-function.zsh": "metadata: File.AnonymousInvocations",
	"ok-anonymous-function-arguments.zsh":                  "metadata: File.AnonymousInvocations",
	"ok-assign-always.zsh":                                 "metadata: File.AssignAlwaysExpansions",
	"ok-flag-pattern-bracket-before-operator.zsh":          "metadata: File.SecondSubscripts",
	"ok-flag-pattern-quoted-nested-key.zsh":                "metadata: File.SecondSubscripts",
	"ok-flag-pattern-subscripted-expansion.zsh":            "metadata: File.SecondSubscripts",
	"ok-flagged-second-subscript-bracket.zsh":              "metadata: File.SecondSubscripts",
	"ok-multi-subscript.zsh":                               "metadata: File.SecondSubscripts",
	"ok-math-function-call-group.zsh":                      "metadata: File.MathFunctionCalls",
	"ok-math-function-call-ternary.zsh":                    "metadata: File.MathFunctionCalls",
	"ok-math-function-call.zsh":                            "metadata: File.MathFunctionCalls",
}

// zshDeparseScript installs standard input as the body of a function through
// the $functions parameter and prints what Zsh deparses from the stored word
// code. Zsh parses the body but never runs it, and a body holding an
// unbalanced `}` cannot close the function early, as it could in
// `f() { ... }`: Zsh reports "invalid function definition" and stores
// nothing. The input is read verbatim, not with `$(<file)`, which strips the
// trailing newline that a brace-form `if` at end of input needs to parse.
// `read -d ”` returns 1 at end of input, so its status is not tested.
const zshDeparseScript = `IFS= read -r -d '' src; functions[f]=$src && print -rn -- "$functions[f]"`

// deparser runs Zsh's deparse and caches it by source. harnessError reports
// a source the comparison cannot split as Zsh does; it is t.Errorf except in
// the test that checks those reports.
type deparser struct {
	t            *testing.T
	zsh          string
	cache        map[string]deparsed
	harnessError func(format string, args ...any)
}

func newDeparser(t *testing.T, zsh string) *deparser {
	return &deparser{t: t, zsh: zsh, cache: map[string]deparsed{}, harnessError: t.Errorf}
}

type deparsed struct {
	text string
	err  error
}

// deparse returns Zsh's canonical deparse of src, or an error carrying Zsh's
// diagnostic when Zsh rejects src as a function body.
func (d *deparser) deparse(src string) (string, error) {
	if r, ok := d.cache[src]; ok {
		return r.text, r.err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, d.zsh, "-f", "-c", zshDeparseScript)
	cmd.Stdin = strings.NewReader(src)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	if ctx.Err() != nil {
		d.t.Fatalf("zsh deparse did not finish: %v", ctx.Err())
	}
	r := deparsed{text: stdout.String()}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		r = deparsed{err: fmt.Errorf("%s", msg)}
	}
	d.cache[src] = r
	return r.text, r.err
}

// normalize makes a deparse comparable with another. Zsh's deparse is
// canonical for commands, one command per line, but it keeps the source text
// of arithmetic and of command substitutions verbatim, while the printer
// reformats both and writes a backquote substitution as `$( )`. So the body
// of every `$( )` and backquote substitution is replaced by the normalized
// Zsh deparse of that body, and blanks and `;` are dropped inside `(( ))`
// and `$(( ))`. Line breaks stay significant, so a tree that joins or splits
// commands still differs; indentation and blank lines do not, and every
// other run of blanks becomes one space. The rewriting depends only on the
// text, so text both sides share normalizes alike.
func (d *deparser) normalize(s string) string {
	var b strings.Builder
	double := false // inside double quotes, where `'` is literal
	for i := 0; i < len(s); {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			b.WriteString(s[i : i+2])
			i += 2
		case s[i] == '"':
			double = !double
			b.WriteByte('"')
			i++
		case s[i] == '\'' && !double:
			// Single-quoted text, or `$'...'` text with backslash escapes,
			// expands nothing.
			end := singleQuoteEnd(s, i+1, i > 0 && s[i-1] == '$')
			b.WriteString(s[i:min(end+1, len(s))])
			i = end + 1
		case s[i] == '`':
			end := backquoteEnd(s, i+1)
			b.WriteString("$(" + d.substitution(unescapeBackquote(s[i+1:end])) + ")")
			i = end + 1
		case (strings.HasPrefix(s[i:], "$((") || strings.HasPrefix(s[i:], "((")) && isArithmetic(s, i):
			start := strings.IndexByte(s[i:], '(') + i
			end := parenEnd(s, start+1)
			inner := s[start+1 : min(end, len(s))]
			b.WriteString(s[i:start] + "(" + strings.Map(dropBlankAndSemicolon, d.normalize(inner)) + ")")
			i = end + 1
		case strings.HasPrefix(s[i:], "$("):
			end := d.commandEnd(s, i+2)
			if end >= len(s) {
				d.harnessError("comparison harness: no closing parenthesis found for the command substitution at %q", s[i:min(len(s), i+60)])
			}
			b.WriteString("$(" + d.substitution(s[i+2:min(end, len(s))]) + ")")
			i = end + 1
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	// Zsh's deparse keeps the blanks around `|` in a case pattern that has no
	// optional opening parenthesis, although matching ignores them.
	var lines []string
	for _, line := range strings.Split(b.String(), "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			line = strings.ReplaceAll(strings.ReplaceAll(line, " |", "|"), "| ", "|")
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// substitution normalizes a command substitution body through Zsh's deparse.
// The body comes from Zsh's own deparse of a source it accepted, so Zsh
// rejecting it means the scanner cut the body in the wrong place, which would
// leave the rest of the file compared only coarsely; the test fails instead.
// A `$( )` body is cut only where Zsh accepts it, so this reports a backquote
// body.
func (d *deparser) substitution(body string) string {
	text, err := d.deparse(body)
	if err != nil {
		d.harnessError("comparison harness: zsh rejects the command substitution body %q: %v", body[:min(len(body), 60)], err)
		return strings.Map(dropBlankAndSemicolon, body)
	}
	return d.normalize(text)
}

func dropBlankAndSemicolon(r rune) rune {
	switch r {
	case ' ', '\t', '\n', ';':
		return -1
	}
	return r
}

// singleQuoteEnd returns the index of the `'` closing single-quoted text
// that starts at start, or len(s). In `$'...'` text a backslash escapes the
// next byte.
func singleQuoteEnd(s string, start int, ansi bool) int {
	for i := start; i < len(s); i++ {
		switch {
		case ansi && s[i] == '\\':
			i++
		case s[i] == '\'':
			return i
		}
	}
	return len(s)
}

// backquoteEnd returns the index of the backquote closing a substitution
// whose body starts at start, or len(s).
func backquoteEnd(s string, start int) int {
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '`':
			return i
		}
	}
	return len(s)
}

// unescapeBackquote removes the backslashes that quote `\`, “ ` ” and `$`
// inside a backquote substitution, leaving the command the body holds.
func unescapeBackquote(body string) string {
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) && strings.IndexByte("\\`$", body[i+1]) >= 0 {
			i++
		}
		b.WriteByte(body[i])
	}
	return b.String()
}

// isArithmetic reports whether the `((` at or just after s[i] opens
// arithmetic: its two parentheses close together, as in `$(( x ))`. Zsh's
// deparse keeps `$((cmd) )` verbatim, a command substitution whose body
// starts with a subshell, and there they do not.
func isArithmetic(s string, i int) bool {
	open := strings.Index(s[i:], "((") + i
	outer := parenEnd(s, open+1)
	return outer < len(s) && parenEnd(s, open+2) == outer-1
}

// parenEnd returns the index of the parenthesis closing one opened just
// before start, counting parentheses only, or len(s).
func parenEnd(s string, start int) int {
	depth := 1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return len(s)
}

// commandEnd returns the index of the `)` closing a command substitution
// whose body starts at start, or len(s). Zsh ends the body at the first `)`
// that completes a command list, so the candidates are tried in order and
// the first whose preceding text Zsh accepts as a function body wins; that
// settles case patterns, subshells and arithmetic without guessing at
// keywords. A `)` that is quoted, inside a `${ }` expansion or a nested
// `$( )`, or in a comment is not a candidate, since text Zsh accepts can
// still end there.
func (d *deparser) commandEnd(s string, start int) int {
	braces := 0
	var quote byte
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
			continue
		case quote == 'a':
			// `$'...'` text, where a backslash escapes the next byte.
			switch c {
			case '\\':
				i++
			case '\'':
				quote = 0
			}
			continue
		case c == '\\':
			i++
			continue
		case c == '$' && i+1 < len(s) && s[i+1] == '(':
			// A nested command substitution, quoted or not, holds its own
			// quotes and parentheses.
			i = d.commandEnd(s, i+2)
			continue
		case quote == '"' && c == '"':
			quote = 0
			continue
		case c == '$' && i+1 < len(s) && s[i+1] == '{':
			braces++
			i++
			continue
		case braces > 0:
			if c == '}' {
				braces--
			}
			continue
		case quote == '"':
			continue
		}
		switch {
		case c == '#' && (i == start || strings.IndexByte(" \t\n;&|", s[i-1]) >= 0):
			// A comment runs to the end of the line and may hold a `)`. A `#`
			// after `(` is a globbing flag such as `(#b)`, not a comment.
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '\'' && i > 0 && s[i-1] == '$':
			quote = 'a'
		case c == '\'' || c == '"':
			quote = c
		case c == ')':
			if _, err := d.deparse(s[start:i]); err == nil {
				return i
			}
		}
	}
	return len(s)
}

// TestStructuralOracle checks that the parser keeps the structure of every
// ok-* corpus fixture (#541). The native oracle judges only accept or reject,
// so a wrong tree for valid Zsh passes it. Zsh can report its own tree: the
// deparse of a function body is canonical, turning brace, short and
// alternate forms into `do`/`done`, `then`/`fi` and `in`/`esac`. For a
// fixture S with printed tree P, the parser preserved the structure when the
// normalized deparse of S equals that of P. It skips when zsh is not
// installed; Go CI installs it.
func TestStructuralOracle(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for the structural oracle test")
	}
	d := newDeparser(t, zsh)

	corpusDir := filepath.Join("testdata", "corpus")
	seen := map[string]bool{}
	for _, name := range fixtureNames(t, corpusDir, "ok-", ".zsh") {
		seen[name] = true
		reason, known := structuralOracleKnownDifferences[name]
		diff := d.structuralDifference(filepath.Join(corpusDir, name))
		switch {
		case diff != "" && !known:
			t.Errorf("%s: the printed tree is not the program Zsh reads: %s", name, diff)
		case diff == "" && known:
			t.Errorf("%s: the printed tree now agrees with Zsh; remove it from "+
				"structuralOracleKnownDifferences (%s)", name, reason)
		}
	}

	var stale []string
	for name := range structuralOracleKnownDifferences {
		if !seen[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		t.Errorf("%s: listed in structuralOracleKnownDifferences but not present in %s", name, corpusDir)
	}
}

// structuralDifference returns "" when Zsh deparses the fixture at path and
// the parser's printed tree of it to the same program, and otherwise a
// description of the first difference.
func (d *deparser) structuralDifference(path string) string {
	d.t.Helper()

	src, err := os.ReadFile(path)
	if err != nil {
		d.t.Fatalf("reading %s: %v", path, err)
	}
	f, err := parse.Parse(bytes.NewReader(src), filepath.Base(path))
	if err != nil {
		return fmt.Sprintf("parse error: %v", err)
	}
	var printed bytes.Buffer
	if err := syntax.NewPrinter().Print(&printed, f.AST()); err != nil {
		return fmt.Sprintf("printer error: %v", err)
	}
	return d.compare(path, string(src), printed.String())
}

// compare returns "" when Zsh deparses src and printed to the same program,
// and otherwise a description of the first difference.
func (d *deparser) compare(name, src, printed string) string {
	d.t.Helper()

	want, err := d.deparse(src)
	if err != nil {
		d.t.Fatalf("%s: zsh rejects a source recorded as valid Zsh as a function body: %v", name, err)
	}
	got, err := d.deparse(printed)
	if err != nil {
		return fmt.Sprintf("zsh rejects the printed tree: %v", err)
	}
	w, g := d.normalize(want), d.normalize(got)
	if w == g {
		return ""
	}
	return firstDifference(w, g)
}

// TestStructuralOracleComparison pins what the comparison treats as the same
// program, so a normalization change cannot quietly hide tree differences.
func TestStructuralOracleComparison(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for the structural oracle test")
	}
	d := newDeparser(t, zsh)

	same := []struct{ src, printed string }{
		{"for x (a b) { print $x }\n", "for x in a b; do print $x; done\n"},
		{"if [[ -n $x ]] { print a }\n", "if [[ -n $x ]]; then print a; fi\n"},
		{"repeat 2 print a\n", "repeat 2; do print a; done\n"},
		{"print $(( 1 + 2 ))\n(( x  =  1 ))\n", "print $((1 + 2))\n((x = 1))\n"},
		{"x=`print a`\n", "x=$(print a)\n"},
		{"x=$(repeat 2 print a)\n", "x=$(repeat 2; do print a; done)\n"},
		{"print ${$((print sub) )}\n", "print ${$( (print sub) )}\n"},
		{"case $x in a | b) : ;; esac\n", "case $x in (a|b) : ;; esac\n"},
		{"x=$(print a # )\n)\n", "x=$(print a)\n"},
		{"x=\"$(print \"$(print \"it's\")\")\"\nprint a\n", "x=\"$(print \"$(print \"it's\")\")\"\nprint a\n"},
		{"x=$(case y in y) print one;; esac)\n", "x=$(case y in (y) print one ;; esac)\n"},
		{"x=$([[ a == (#b)(*) ]] && print \"it's\")\nprint a\n", "x=$([[ a == (#b)(*) ]] && print \"it's\")\nprint a\n"},
		{"x=$(print '$(')\nprint a\n", "x=$(print '$(')\nprint a\n"},
		{"print '$(' \"it's $(print ')')\" $'\\'$('\n", "print '$(' \"it's $(print ')')\" $'\\'$('\n"},
		{"x=$(print $'\\'')\nprint a\n", "x=$(print $'\\'')\nprint a\n"},
	}
	for _, c := range same {
		if diff := d.compare(c.src, c.src, c.printed); diff != "" {
			t.Errorf("%q and %q are the same program, but the comparison reports: %s", c.src, c.printed, diff)
		}
	}

	different := []struct{ src, printed string }{
		{"for k v in a 1 b 2; do print $k $v; done\n", "for k in a 1 b 2; do print $k $v; done\n"},
		{"false &&\n;\nprint b\n", "false\nprint b\n"},
		{"{ : } always { : }\n", "{ :; }\n{ :; }\n"},
		{"print $(( 1 + 2 ))\n", "print $(( 1 - 2 ))\n"},
		{"x=$(print a && print b)\n", "x=$(print a; print b)\n"},
		{"x=`print a b`\n", "x=$(print ab)\n"},
		{"print a b\n", "print ab\n"},
		{"print a; print b\n", "print a print b\n"},
		{"print ';'\n", "print ''\n"},
		{"x=$(print a; print b)\n", "x=$(print a print b)\n"},
		{"case $x in (a|b) : ;; esac\n", "case $x in (a) : ;; esac\n"},
		{"x=\"$(print \"it's\")\"\nprint a b\n", "x=\"$(print \"it's\")\"\nprint ab\n"},
		{"print '$(print a b)'\n", "print '$(print ab)'\n"},
		{"print ${x:-;}\n", "print ${x:-}\n"},
	}
	for _, c := range different {
		if diff := d.compare(c.src, c.src, c.printed); diff == "" {
			t.Errorf("%q and %q are different programs, but the comparison reports them equal", c.src, c.printed)
		}
	}
}

// firstDifference quotes both normalized deparses around their first
// differing byte.
func firstDifference(want, got string) string {
	i := 0
	for i < len(want) && i < len(got) && want[i] == got[i] {
		i++
	}
	const context = 40
	lo := max(i-context, 0)
	return fmt.Sprintf("at byte %d: zsh reads %q, printed tree reads %q",
		i, want[lo:min(i+context, len(want))], got[lo:min(i+context, len(got))])
}

// TestStructuralOracleHarnessErrors checks that a command substitution the
// comparison cannot split as Zsh does is reported rather than compared
// coarsely.
func TestStructuralOracleHarnessErrors(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for the structural oracle test")
	}
	for _, s := range []string{"x=$(print a", "x=$(fi)", "x=`fi`"} {
		d := newDeparser(t, zsh)
		var reports []string
		d.harnessError = func(format string, args ...any) {
			reports = append(reports, fmt.Sprintf(format, args...))
		}
		d.normalize(s)
		if len(reports) == 0 {
			t.Errorf("%q: the comparison accepted a command substitution Zsh cannot close", s)
		}
	}
}
