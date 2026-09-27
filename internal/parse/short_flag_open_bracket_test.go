package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #538: a `[` in a short subscript's flag argument that no later `]`
// of the word closes leaves the subscript unclosed.
func TestShortFlagOpenBracketRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-538-short-flag-open-bracket.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-538.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "a `[` in a subscript flag argument must be closed before the subscript's `]`" || pe.Pos.Line() != 5 || pe.Pos.Col() != 14 {
		t.Fatalf("error = %v, want 5:14: a `[` in a subscript flag argument must be closed before the subscript's `]`", err)
	}
}

// Balanced patterns the flag-pattern adapters read past the first `]` keep
// parsing.
func TestShortFlagOpenBracketThroughAdapters(t *testing.T) {
	for _, src := range []string{
		"print $x[(r)a[b]c]\n",
		"print $x[(r)[a]]\n",
		"print $x[(r)a[]]\n",
		"print $x[(r)a[],2]\n",
		"print $x[(r)[[:alpha:]]]\n",
		"print $x[(r)\\[]\n",
		"print \"$x[(r)a[b]c]\"\n",
		"print \"$x[(r)a[b] c]\"\n",
		"print $x[(r)(a|[b])]\n",
		"print $x[(r)(a[b]|c)]\n",
		"print $x[(r)(a[b] |c)]\n",
		"print $x[(r)a[b]\"c\"]\n",
		"print $x[(r)a[[b]]]\n",
		"print $x[(r)[a]<1-2>]\n",
		"print $x[(r)a[b]<->]\n",
		"print \"$x[(r)[a]\"c\"]\"\n",
		"print $x[(r)a[b]${y:-]}]\n",
		"print $x[(r)a[b]$(echo ])]\n",
		"print $x[(r)a[b]$(echo (a))]\n",
		"print $x[(r)a[b](c|$(d))]\n",
		"print $x[(r)a[b]${y:- }]\n",
		"print $x[(r)a[b]$(echo \" \")]\n",
		"print $x[(r)a[b]\\ ]\n",
		"print $x[(r)a[b]' ']\n",
		"print $x[(r)a[b]<(echo)]\n",
		"print $x[(r)a[b]`echo []`]\n",
		"print $x[(r)a[b]`echo \" \"`]\n",
		"print $x[(r)a[b]" + strings.Repeat("c", 9000) + "]\n",
	} {
		if _, err := Parse(bytes.NewReader([]byte(src)), ""); err != nil {
			t.Errorf("Parse(%q) = %v", src, err)
		}
	}
}
