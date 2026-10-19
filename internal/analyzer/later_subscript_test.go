package analyzer_test

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"github.com/z-shell/zsh-lint/internal/rules"
)

// Subscripts after the first are children of their expansion that rule
// evaluation reaches once each, like the first subscript (#252).
func TestLaterSubscriptExecutableChildren(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		evals []string
	}{
		{name: "first subscript", src: `print -r -- ${a[$(eval "$code")]}`, evals: []string{`eval "$code"`}},
		{name: "later subscript", src: `print -r -- ${a[1][$(eval "$code")]}`, evals: []string{`eval "$code"`}},
		{name: "third subscript", src: `print -r -- ${a[1][2][$(eval "$code")]}`, evals: []string{`eval "$code"`}},
		{name: "nested later subscript", src: `print -r -- ${a[1][${b[2][$(eval "$code")]}]}`, evals: []string{`eval "$code"`}},
		{name: "repeated later subscripts", src: `print -r -- ${a[$(eval "$x")][1][$(eval "$y")][$(eval "$z")]}`, evals: []string{`eval "$x"`, `eval "$y"`, `eval "$z"`}},
		{name: "double-quoted expansion", src: `print -r -- "${a[1][$(eval "$code")]}"`, evals: []string{`eval "$code"`}},
		{name: "length prefix", src: `print -r -- ${#a[1][$(eval "$code")]}`, evals: []string{`eval "$code"`}},
		{name: "backquoted command", src: "print -r -- ${a[1][`eval x`]}", evals: []string{"eval x"}},
		{name: "assignment value", src: `x=${a[1][$(eval "$code")]}`, evals: []string{`eval "$code"`}},
		{name: "after a literal later subscript", src: `print -r -- ${a[(r)x][^b]}; eval "$code"`, evals: []string{`eval "$code"`}},
		{name: "quoted literal first subscript", src: `print -r -- ${a['$(eval "$code")']}`},
		{name: "quoted literal later subscript", src: `print -r -- ${a[1]['$(eval "$code")']}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := test.src + "\n"
			file, err := parse.Parse(strings.NewReader(src), "test.zsh")
			if err != nil {
				t.Fatal(err)
			}
			got := findByID(analyzer.New(rules.EvalUsage{}).Analyze(file, "test.zsh"), "security/eval")
			if len(got) != len(test.evals) {
				t.Fatalf("security/eval = %+v, want %d", got, len(test.evals))
			}
			from := 0
			for i, call := range test.evals {
				start := from + strings.Index(src[from:], call)
				if got[i].Range.Start.Offset != start || got[i].Range.End.Offset != start+len(call) {
					t.Errorf("finding %d range = %d-%d, want %d-%d (%s)", i, got[i].Range.Start.Offset, got[i].Range.End.Offset, start, start+len(call), call)
				}
				from = start + len(call)
			}
		})
	}
}

// Rule evaluation visits each expansion once, however deeply later
// subscripts nest.
func TestLaterSubscriptExpansionsVisitedOnce(t *testing.T) {
	const src = "print ${a[1][${b[2][${c[3][4]}]}][$d]}\n"
	file, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]int)
	for _, d := range analyzer.New(&expansionRule{}).Analyze(file, "test.zsh") {
		seen[d.Message]++
	}
	for _, name := range []string{"a", "b", "c", "d"} {
		if seen["expansion "+name] != 1 {
			t.Errorf("expansion %s visited %d times, want 1 (%v)", name, seen["expansion "+name], seen)
		}
	}
	if len(seen) != 4 {
		t.Errorf("expansions = %v, want a, b, c and d", seen)
	}
}

// Preceding and trailing directives apply once to a finding in a later
// subscript, and directives inside its command substitution are collected
// with their ordinary line scope and unused-directive reporting.
func TestLaterSubscriptSuppression(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		evals  []int // lines of unsuppressed findings
		unused []int // lines of unused-suppression findings
	}{
		{
			name:  "unsuppressed",
			src:   "print -r -- ${a[1][$(\n  eval \"$code\"\n)]}\n",
			evals: []int{2},
		},
		{
			name: "preceding",
			src:  "# zsh-lint disable=security/eval -- trusted table\nprint -r -- ${a[1][$(eval \"$code\")]}\n",
		},
		{
			name: "trailing",
			src:  "print -r -- ${a[1][$(eval \"$code\")]} # zsh-lint disable=security/eval -- trusted table\n",
		},
		{
			name: "directive inside the command substitution",
			src:  "print -r -- ${a[1][$(\n  # zsh-lint disable=security/eval -- trusted table\n  eval \"$code\"\n)]}\n",
		},
		{
			name: "trailing directive inside the command substitution",
			src:  "print -r -- ${a[1][$(\n  eval \"$code\" # zsh-lint disable=security/eval -- trusted table\n)]}\n",
		},
		{
			name:   "unused directive inside the command substitution",
			src:    "print -r -- ${a[1][$(\n  # zsh-lint disable=security/eval -- stale\n  print -r -- x\n)]}\n",
			unused: []int{2},
		},
		{
			name:   "directive suppresses only its own line",
			src:    "print -r -- ${a[1][$(\n  # zsh-lint disable=security/eval -- trusted table\n  eval \"$x\"\n  eval \"$y\"\n)]}\n",
			evals:  []int{4},
			unused: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "test.zsh")
			if err != nil {
				t.Fatal(err)
			}
			diagnostics := analyzer.New(rules.EvalUsage{}).Analyze(file, "test.zsh")
			assertLines(t, "security/eval", findByID(diagnostics, "security/eval"), test.evals)
			assertLines(t, "meta/unused-suppression", findByID(diagnostics, "meta/unused-suppression"), test.unused)
		})
	}
}

func assertLines(t *testing.T, id string, got []diag.Diagnostic, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %+v, want lines %v", id, got, want)
	}
	for i, line := range want {
		if got[i].Range.Start.Line != line {
			t.Errorf("%s[%d] line = %d, want %d", id, i, got[i].Range.Start.Line, line)
		}
	}
}

// The issue's $0 example reports in a configured plugin sourced library,
// like its first-subscript control; unconfigured sources stay silent.
func TestLaterSubscriptZeroHandling(t *testing.T) {
	library := projectconfig.SourceContext{
		ConfigVersion: projectconfig.CurrentVersion,
		ProjectKind:   projectconfig.KindPlugin,
		Profile:       projectconfig.ProfileSourcedLibrary,
	}
	tests := []struct {
		src     string
		context projectconfig.SourceContext
		column  int
	}{
		{src: "print ${a[$0]}\n", context: library, column: 11},
		{src: "print ${a[1][$0]}\n", context: library, column: 14},
		{src: "print ${a[1][2][$0]}\n", context: library, column: 17},
		{src: "print ${a[$0]}\n"},
		{src: "print ${a[1][$0]}\n"},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "example.plugin.zsh")
			if err != nil {
				t.Fatal(err)
			}
			got := findByID(analyzer.New(rules.ZeroHandling{}).AnalyzeSource(file, "example.plugin.zsh", test.context), "plugin/zero-handling")
			if test.column == 0 {
				if len(got) != 0 {
					t.Fatalf("unconfigured findings = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 || got[0].Range.Start.Line != 1 || got[0].Range.Start.Column != test.column ||
				got[0].Range.End.Column != test.column+2 {
				t.Fatalf("findings = %+v, want one $0 at 1:%d", got, test.column)
			}
		})
	}
}

// A declaration inside a command substitution, in any subscript or anywhere
// else, belongs to that subshell: it informs lookups inside it, but not the
// enclosing scope (#252). The unquoted-var rule is the scope consumer.
func TestCommandSubstitutionDeclarationScope(t *testing.T) {
	library := projectconfig.SourceContext{
		ConfigVersion: projectconfig.CurrentVersion,
		ProjectKind:   projectconfig.KindPlugin,
		Profile:       projectconfig.ProfileStandaloneExecutable,
	}
	const plain = "Variable expansion should be double-quoted"
	const array = `Array expansion should preserve elements with "${arr[@]}"`
	tests := []struct {
		name string
		src  string
		want map[int]string // line -> message of the unquoted-var finding on $n or $arr
	}{
		{name: "no declaration", src: "print $n\n", want: map[int]string{1: plain}},
		{name: "outer integer", src: "integer n=1\nprint $n\n"},
		{name: "assignment value", src: "x=$(integer n=1)\nprint $n\n", want: map[int]string{2: plain}},
		{name: "backquotes", src: "x=`integer n=1`\nprint $n\n", want: map[int]string{2: plain}},
		{name: "first subscript", src: "print \"${a[$(integer n=1)]}\"\nprint $n\n", want: map[int]string{2: plain}},
		{name: "later subscript", src: "print \"${a[1][$(integer n=1)]}\"\nprint $n\n", want: map[int]string{2: plain}},
		{name: "inside the same substitution", src: "x=$(integer n=1; print $n)\n"},
		{name: "outer declaration inside a substitution", src: "integer n=1\nx=$(print $n)\n"},
		{name: "nested substitution", src: "x=$(y=$(integer n=1); print $n)\n", want: map[int]string{1: plain}},
		{name: "inside a function", src: "f() {\n  x=$(integer n=1)\n  print $n\n}\n", want: map[int]string{3: plain}},
		{name: "unset inside a substitution", src: "integer n=1\nx=$(unset n)\nprint $n\n"},
		{name: "unset in the same substitution", src: "x=$(integer n=1; unset n; print $n)\n", want: map[int]string{1: plain}},
		{name: "array in a substitution", src: "x=$(typeset -a arr)\nprint $arr\n", want: map[int]string{2: plain}},
		{name: "array inside the same substitution", src: "x=$(typeset -a arr; print $arr)\n", want: map[int]string{1: array}},
		{name: "outer array", src: "typeset -a arr\nx=\"$(print $arr)\"\n", want: map[int]string{2: array}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "tool.zsh")
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[int]string)
			for _, d := range findByID(analyzer.New(rules.UnquotedVar{}).AnalyzeSource(file, "tool.zsh", library), "quoting/unquoted-var") {
				text := lineText(test.src, d.Range.Start.Line)
				name := text[d.Range.Start.Column-1 : d.Range.End.Column-1]
				if name != "$n" && name != "$arr" {
					continue
				}
				if _, dup := got[d.Range.Start.Line]; dup {
					t.Fatalf("two findings on line %d", d.Range.Start.Line)
				}
				got[d.Range.Start.Line] = d.Message
			}
			if len(got) != len(test.want) {
				t.Fatalf("findings = %v, want %v", got, test.want)
			}
			for line, message := range test.want {
				if got[line] != message {
					t.Errorf("line %d = %q, want %q", line, got[line], message)
				}
			}
		})
	}
}

func lineText(src string, line int) string {
	return strings.Split(src, "\n")[line-1]
}
