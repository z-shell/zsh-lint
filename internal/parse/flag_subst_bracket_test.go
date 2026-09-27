package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #529: a `[` or `]` inside a command substitution in a subscript
// flag argument is text of the substitution, and in a `${...}` those bytes
// must balance over the subscript.
func TestFlagSubstBracketRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-529-flag-subst-bracket.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-529.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "the `[` and `]` in command substitutions in a subscript must balance" || pe.Pos.Line() != 5 || pe.Pos.Col() != 21 {
		t.Fatalf("error = %v, want 5:21: the `[` and `]` in command substitutions in a subscript must balance", err)
	}
}

// Valid Zsh that the flag-pattern adapters also see: the parser now accepts
// it without their help.
func TestFlagSubstBracketThroughAdapters(t *testing.T) {
	for _, src := range []string{
		"print ${x[(r)$(echo [a])]}\n",
		"print ${x[(r)a[bc]$(echo ok)]}\n",
		"print ${m[(r)$(echo [x])##]}\n",
		"print ${x[(r)$((x[1]+2))]}\n",
	} {
		if _, err := Parse(bytes.NewReader([]byte(src)), ""); err != nil {
			t.Errorf("Parse(%q) = %v", src, err)
		}
	}
}
