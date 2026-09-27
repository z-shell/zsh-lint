package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #534. In a `${...}` subscript Zsh counts the unescaped `[` bytes of
// a flag argument, quoted or not, so one still open at the subscript's `]`
// leaves the subscript unclosed: `${x[(r)a[]}` is an invalid subscript.
// Native verdicts use newline-terminated files and zsh -f -n.
func TestZshFlagOpenBracketReject(t *testing.T) {
	const open = "a `[` in a subscript flag argument must be closed before the subscript's `]`"
	for _, tc := range []struct{ src, err string }{
		{"print ${x[(r)a[]}", "1:15: " + open},
		{"print ${x[(r)[]}", "1:14: " + open},
		{"print ${x[(i)[]}", "1:14: " + open},
		{"print ${x[(r)a[b]}", "1:15: " + open},
		// The first open `[` is the one reported.
		{"print ${x[(r)a[b[]}", "1:15: " + open},
		{"print ${x[(r)[[]}", "1:14: " + open},
		// Quoted, it still counts.
		{"print ${x[(r)'['a]}", "1:15: " + open},
		{"print ${x[(r)\"[\"]}", "1:15: " + open},
		{"print \"${x[(r)a[]}\"", "1:16: " + open},
		// After a short subscript or a nested expansion too.
		{"print ${x[(r)'$y[(r)a]'[]}", "1:24: " + open},
		{"print ${x[(r)${y}[]}", "1:18: " + open},
		{"print ${m[(i)${y:-{}}[]}", "1:22: " + open},
		// In double quotes a nested expansion is skipped the same way.
		{"print \"${m[(i)${y}[]}\"", "1:19: " + open},
		{"print \"${m[(i)${y:-{a}}[]}\"", "1:24: " + open},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

func TestZshFlagOpenBracketAccept(t *testing.T) {
	for _, src := range []string{
		// An escaped `[` is text, in double quotes too.
		"print ${x[(r)\\[]}",
		"print \"${x[(r)\\[]}\"",
		"print \"${x[(r)a\\[]}\"",
		// Inside a short subscript, a `${...}` counts no brackets against
		// it: Zsh does not count them there.
		"print $x[${y[(r)a[]}]",
		// The brackets of a nested expansion are its own, and a brace pair
		// inside it does not end it early.
		"print ${m[(i)${Y[a]}]}",
		"print ${m[(i)${y:-{a}[b]}]}",
		"print ${m[(i)${y:-{}[]}]}",
		"print \"${m[(i)${Y[a]}]}\"",
		"print \"${m[(I)[${y[2]}]]}\"",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}
