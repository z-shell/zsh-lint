package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #540: the rest of a short subscript's word is read with
// dquote_parse's rules when deciding whether a `[` of its flag argument
// closes.
func TestShortFlagDquoteRulesRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-540-short-flag-dquote-rules.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-540.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "a `[` in a subscript flag argument must be closed before the subscript's `]`" || pe.Pos.Line() != 5 || pe.Pos.Col() != 15 {
		t.Fatalf("error = %v, want 5:15: a `[` in a subscript flag argument must be closed before the subscript's `]`", err)
	}
}

// Issue #540: the subscript's text is expanded as a string, where a
// backquote a `'` quoted in the word opens a command substitution.
func TestShortFlagOpenBackquoteRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-540-short-flag-open-backquote.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-540.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "a backquote in a subscript must be closed before the subscript's `]`" || pe.Pos.Line() != 5 || pe.Pos.Col() != 20 {
		t.Fatalf("error = %v, want 5:20: a backquote in a subscript must be closed before the subscript's `]`", err)
	}
}

// Issue #540: the flag argument's own `(` stays open past the `)` of a
// nested `${...}`, so its `[` is never closed. Zsh reports this only when
// the line runs.
func TestShortFlagOpenParenRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-540-short-flag-runtime-parens.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-540.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "a `[` in a subscript flag argument must be closed before the subscript's `]`" || pe.Pos.Line() != 6 || pe.Pos.Col() != 14 {
		t.Fatalf("error = %v, want 6:14: a `[` in a subscript flag argument must be closed before the subscript's `]`", err)
	}
}

// Valid words keep parsing through the flag-pattern adapters.
func TestShortFlagDquoteRulesThroughAdapters(t *testing.T) {
	for _, src := range []string{
		"print \"$x[(r)a[b]$(echo [)]\"\n",
		"print \"$x[(r)a[b]${y:-[}]\"\n",
		"print \"$x[(r)a[b]$(echo \")\")]\"\n",
		"print $x[(r)a[b]${y:-'}]'}\n",
		"print \"$x[(r)a[b]${y:-\"[\"}]\"\n",
		"print \"$x[(r)a[b]${y:-]}]\"\n",
		"print \"$x[(r)a[b]$(echo ])]\"\n",
		"print $x[(r)a[b]\"${y:-]}\"]\n",
		"print \"$x[(r)a[b]\\]]\"\n",
		"print \"$x[(r)a[b]`echo \\]`]\"\n",
		"print \"$x[(r)a[b]$((1+(2)))]\"\n",
		"print $x[(r)a[b]`echo \"[]\"`]\n",
		"print \"$x[(r)a[b]${y:-\\}}]\"\n",
		"print $x[(r)a[b](c)]\n",
	} {
		if _, err := Parse(bytes.NewReader([]byte(src)), ""); err != nil {
			t.Errorf("Parse(%q) = %v", src, err)
		}
	}
}
