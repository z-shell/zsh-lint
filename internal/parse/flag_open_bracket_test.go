package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #534: a `[` still open at the `]` of a `${...}` subscript flag
// argument leaves the subscript unclosed.
func TestFlagOpenBracketRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-534-flag-open-bracket.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-534.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "a `[` in a subscript flag argument must be closed before the subscript's `]`" || pe.Pos.Line() != 5 || pe.Pos.Col() != 15 {
		t.Fatalf("error = %v, want 5:15: a `[` in a subscript flag argument must be closed before the subscript's `]`", err)
	}
}

// Balanced bracket expressions, which the flag-pattern adapters read, and
// brackets of a nested expansion keep parsing.
func TestFlagOpenBracketThroughAdapters(t *testing.T) {
	for _, src := range []string{
		"print ${x[(r)a[b]c]}\n",
		"print ${x[(r)[a],2]}\n",
		// A `[` open at a `,` is left as before.
		"print ${x[(r)a[],2]}\n",
		"print ${x[(r)[[:alpha:]]]}\n",
		"print ${x[(r)a[]b]}\n",
		"print ${m[(I)[${y[2]}]]}\n",
		"local -i idx=${matching[(I)[${${1:-$KEYS}[2]}]]}\n",
	} {
		if _, err := Parse(bytes.NewReader([]byte(src)), ""); err != nil {
			t.Errorf("Parse(%q) = %v", src, err)
		}
	}
}
