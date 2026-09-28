package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #540. Whether a later `]` closes a `[` left open in a short
// subscript's flag argument is decided by dquote_parse(']') over the word's
// text, not by shell quoting: a `'` is text outside backquotes, a `"` is
// text outside a `${...}`, brackets inside a `${...}` do not count, and
// parentheses must balance too. Native verdicts use newline-terminated
// files and zsh -f -n.
func TestZshShortFlagDquoteRulesReject(t *testing.T) {
	const open = "a `[` in a subscript flag argument must be closed before the subscript's `]`"
	const tick = "a backquote in a subscript must be closed before the subscript's `]`"
	for _, tc := range []struct{ src, err string }{
		{"print \"$x[(r)a[b]${y:-]}\"", "1:15: " + open},
		{"print \"$x[(r)a[b]$(echo ])\"", "1:15: " + open},
		{"print \"$x[(r)a[b]`echo ]`\"", "1:15: " + open},
		{"print \"$x[(r)a[b]`echo ]`]\"", "1:15: " + open},
		{"print $x[(r)a[b]\"${y:-]}\"", "1:14: " + open},
		{"print $x[(r)a[b]\"$(echo ])\"", "1:14: " + open},
		{"print \"$x[(r)a[b]${y:-\"]\"}\"", "1:15: " + open},
		{"print \"$x[(r)a[b]${y:-']'}\"", "1:15: " + open},
		{"print \"$x[(r)a[b]\\$(echo ])\"", "1:15: " + open},
		{"print \"$x[(r)a[b]$(echo `echo ]`)\"", "1:15: " + open},
		{"print \"$x[(r)a[b](]\"", "1:15: " + open},
		{"print $x[(r)a[b]`echo [`]", "1:14: " + open},
		{"print $x[(r)a[b]`echo ']'`]", "1:14: " + open},
		{"print \"$x[(r)a[b]${y:-${z:-'}]'}}\"", "1:15: " + open},
		{"print $x[(r)a[b]`echo \\`]\\``", "1:14: " + open},
		{"print $x[(r)a[b]`echo $(echo ])`]", "1:14: " + open},
		{"print $x[(r)a[b]`echo ${y:-)}``echo ${y:-(}`]", "1:14: " + open},
		{"print $x[(r)a[b]'`'`echo \"]\"`", "1:20: " + tick},
		{"print $x[(r)a[b]'`'${y:-`echo ]`}", "1:25: " + tick},
		{"print $x[(r)a[b]'`'\"'\"`echo ]`]", "1:23: " + tick},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

// Valid words, among them four #538 rejected: the brackets of a nested
// `${...}` or `$(...)`, and a `'` or `"` that is text to dquote_parse.
func TestZshShortFlagDquoteRulesAccept(t *testing.T) {
	for _, src := range []string{
		"print \"$x[(r)a[b]$(echo [)]\"",
		"print \"$x[(r)a[b]${y:-[}]\"",
		"print \"$x[(r)a[b]$(echo \")\")]\"",
		"print $x[(r)a[b]${y:-'}]'}",
		"print \"$x[(r)a[b]${y:-\"[\"}]\"",
		"print \"$x[(r)a[b]${y:-]}]\"",
		"print \"$x[(r)a[b]$(echo ])]\"",
		"print $x[(r)a[b]\"${y:-]}\"]",
		"print \"$x[(r)a[b]\\]]\"",
		"print \"$x[(r)a[b]`echo \\]`]\"",
		"print \"$x[(r)a[b]$((1+(2)))]\"",
		"print $x[(r)a[b]`echo \"[]\"`]",
		"print \"$x[(r)a[b]${y:-\\}}]\"",
		"print $x[(r)a[b](c)]",
		"print $x[(r)a[b]`echo \\` x\\``]",
		"print $x[(r)a[b]${y:-(}]",
		"print $x[(r)a[b]\\\"]",
		"print \"$x[(r)a[b]${y:-\\\"}]\"",
		"print $x[(r)a[b]']']",
		"print $x[(r)a[b]\\`]",
		"print $x[(r)a[(b)]]",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}
