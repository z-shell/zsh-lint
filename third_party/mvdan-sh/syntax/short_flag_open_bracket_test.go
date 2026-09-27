package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #538. Zsh counts the unescaped `[` bytes of a short `$x[...]`
// subscript's flag argument too, so a `[` that no later `]` of the word
// closes leaves the subscript unclosed: `$x[(r)a[]` is an invalid subscript.
// The flag-pattern adapters read a balanced pattern such as `a[b]c` past
// the first `]`, so only a word that ends first is rejected. Native verdicts
// use newline-terminated files and zsh -f -n.
func TestZshShortFlagOpenBracketReject(t *testing.T) {
	const open = "a `[` in a subscript flag argument must be closed before the subscript's `]`"
	for _, tc := range []struct{ src, err string }{
		{"print $x[(r)a[]", "1:14: " + open},
		{"print $x[(r)[]", "1:13: " + open},
		{"print $x[(r)a[b]", "1:14: " + open},
		{"print $x[(r)'['a]", "1:14: " + open},
		{"print $x[(r)a[b] c]", "1:14: " + open},
		{"print $x[(r)a[b];c]", "1:14: " + open},
		{"print $x[(r)a[b]|c]", "1:14: " + open},
		{"print $x[(r)(a[)]", "1:15: " + open},
		{"print \"$x[(r)a[]\"", "1:15: " + open},
		{"print $x[(r)a[[b]]", "1:14: " + open},
		{"print $x[(r)(a[b]) c]", "1:15: " + open},
		// The word goes on past a numeric glob's `>` and a double quote's end,
		// and a `]` inside a nested expansion or substitution is its own.
		{"print $x[(r)a[b]<1-2> c]", "1:14: " + open},
		{"print \"$x[(r)a[]\" \"]\"", "1:15: " + open},
		{"print $x[(r)a[b]${y:-]}", "1:14: " + open},
		{"print $x[(r)a[b]$(echo ])", "1:14: " + open},
		{"print $x[(r)a[b]`echo ]`", "1:14: " + open},
		{"print $x[(r)a[b]<a>]", "1:14: " + open},
		{"print $x[(r)a[b][c]", "1:14: " + open},
		{"print $x[(r)a[b]<12>]", "1:14: " + open},
		{"print $x[(r)'('a[b]|c]", "1:17: " + open},
		{"print $x[(r)\\(a[b]|c]", "1:16: " + open},
		// Run, the substitution's body fails the same way; zsh -n does not
		// parse it.
		{"print $(print $x[(r)a[b])", "1:22: " + open},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

// These parse only through the flag-pattern adapters, so they are checked in
// internal/parse; here the parser must not report the `[` they close later.
func TestZshShortFlagOpenBracketNotReported(t *testing.T) {
	for _, src := range []string{
		"print $x[(r)a[b]c]",
		"print $x[(r)[a]]",
		"print $x[(r)a[]]",
		"print $x[(r)a[],2]",
		"print $x[(r)[[:alpha:]]]",
		"print $x[(r)\\[]",
		"print \"$x[(r)a[b]c]\"",
		"print \"$x[(r)a[b] c]\"",
		"print $x[(r)(a|[b])]",
		"print $x[(r)(a[b]|c)]",
		"print $x[(r)(a[b] |c)]",
		"print $x[(r)a[b]\"c\"]",
		"print $x[(r)a[[b]]]",
		"print $x[(r)[a]<1-2>]",
		"print $x[(r)a[b]<->]",
		"print \"$x[(r)[a]\"c\"]\"",
		"print $x[(r)a[b]${y:-]}]",
		"print $x[(r)a[b]$(echo ])]",
		"print $x[(r)a[b]$(echo (a))]",
		"print $x[(r)a[b](c|$(d))]",
		"print $x[(r)a[b]${y:- }]",
		"print $x[(r)a[b]$(echo \" \")]",
		"print $x[(r)a[b]\\ ]",
		"print $x[(r)a[b]' ']",
		"print $x[(r)a[b]<(echo)]",
		"print $x[(r)a[b]`echo []`]",
		"print $x[(r)a[b]`echo \" \"`]",
	} {
		t.Run(src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), "")
			if err != nil && strings.Contains(err.Error(), "must be closed before the subscript's") {
				t.Fatal(err)
			}
		})
	}
}

// A word that ends at the end of the input, with no newline, ends the
// subscript unclosed too.
func TestZshShortFlagOpenBracketAtEOF(t *testing.T) {
	_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader("print $x[(r)a[b]"), "")
	const want = "1:14: a `[` in a subscript flag argument must be closed before the subscript's `]`"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %s", err, want)
	}
}

// A word longer than the read-ahead buffer is taken to close its `[`, so a
// valid one is never rejected.
func TestZshShortFlagOpenBracketLongWord(t *testing.T) {
	src := "print $x[(r)a[b]" + strings.Repeat("c", 9000) + "]\n"
	if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src), ""); err != nil && strings.Contains(err.Error(), "must be closed before the subscript's") {
		t.Fatal(err)
	}
}
