package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #521. Zsh lexes an unquoted `${...}` with one brace count (bct in
// gettokstr, Src/lex.c), subscript included, so a balanced `{...}` in a
// subscript is text and an unclosed one leaves the expansion open. Native
// verdicts use newline-terminated files and zsh -f -n.
func TestZshSubscriptBraceReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"print ${x[{]}", "1:11: a `{` in a subscript must be closed before its `]`"},
		{"print ${x[a{]}", "1:12: a `{` in a subscript must be closed before its `]`"},
		{"x=${x[{]}", "1:7: a `{` in a subscript must be closed before its `]`"},
		{"print ${x[{]:-z}", "1:11: a `{` in a subscript must be closed before its `]`"},
		{"[[ ${x[{]} == a ]]", "1:8: a `{` in a subscript must be closed before its `]`"},
		{"f() { print ${x[{]}; }", "1:17: a `{` in a subscript must be closed before its `]`"},
		{"print ${x[1,{]}", "1:13: a `{` in a subscript must be closed before its `]`"},
		{"print ${x[$y{]}", "1:13: a `{` in a subscript must be closed before its `]`"},
		// The error names the first `{` still open.
		{"print ${x[{a{]}", "1:11: a `{` in a subscript must be closed before its `]`"},
		// Subscript flags read their argument as raw text; it counts
		// braces too.
		{"print ${x[(r){]}", "1:14: a `{` in a subscript must be closed before its `]`"},
		{"print ${x[(r)a,{]}", "1:16: a `{` in a subscript must be closed before its `]`"},
		{"print ${x[(r)$y{]}", "1:16: a `{` in a subscript must be closed before its `]`"},
		// Zsh accepts these, where text after the expansion closes the `{`;
		// the parser has no place for that text, so they are rejected (a
		// known false rejection, as for the command-word form on main).
		{"x=${y[{]}]}", "1:7: a `{` in a subscript must be closed before its `]`"},
		{"x=${x[{]}}", "1:7: a `{` in a subscript must be closed before its `]`"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

func TestZshSubscriptBraceAccept(t *testing.T) {
	for _, src := range []string{
		"print ${x[{}]}",
		"print ${x[{a}]}",
		"print ${x[a{b}c]}",
		"print ${x[{a},{b}]}",
		"print ${x[{a}]:-z}",
		"print ${#x[{a}]}",
		"print ${x:-${y[{a}]}}",
		"[[ ${x[{a}]} == a ]]",
		// Inside an open `{` blanks and operators are text.
		"print ${x[{a b}]}",
		"print ${x[{a;b}]}",
		"print ${x[{a|b}]}",
		"print ${x[{a/b}]}",
		"print ${x[{a:b}]}",
		// A nested subscript counts its own braces from zero; one directly
		// in the subscript shares its count, and one in a substitution
		// counts none.
		"print ${x[{${y[1]}}]}",
		"print ${x[y[{a}]]}",
		"print ${x[$((y[{]))]}",
		"print ${x[$(print {)]}",
		// Braces in the word of a nested expansion are its own.
		"print ${x[${y:-{}}]}",
		"print ${x[{${y[a]}}]}",
		// A `[` inside the braces is text. Zsh rejects this at the top
		// level only when it expands the subscript.
		"f() { print ${x[{a[b}]}; }",
		// Expansions, escapes and quotes inside the braces.
		"print ${x[{$y}]}",
		"print ${x[{${y}}]}",
		"print ${x[{\\}}]}",
		"print ${x[(r)'{']}",
		// A `,` inside the braces of a flag argument is text.
		"print ${x[(r){a,b}]}",
		"print ${x[(r)a,{b}]}",
		"print ${x[(r){a}]}",
		// A nested expansion in a flag argument keeps its own braces.
		"print ${m[(i)${Y[a]}]}",
		"print ${x[(r)${y%%[^a]#}]}",
		// A short `$x[...]` and a double-quoted `${...}` count no braces
		// against the expansion.
		"print $x[{]",
		"print \"${x[{]}\"",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Bash reads a subscript as before.
func TestBashSubscriptBraceUnchanged(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"echo ${x[{]}", ""},
		{"echo ${x[{a b}]}", "1:13: not a valid arithmetic operator: `b`"},
	} {
		_, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(tc.src+"\n"), "")
		got := ""
		if err != nil {
			got = err.Error()
		}
		if got != tc.err {
			t.Fatalf("%s: error = %q, want %q", tc.src, got, tc.err)
		}
	}
}
