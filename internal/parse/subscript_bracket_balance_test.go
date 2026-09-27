package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #531: the `[` and `]` bytes inside the command substitutions of a
// `${...}` subscript must balance over the whole subscript, across a `,`.
func TestSubscriptBracketBalanceRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-531-subscript-bracket-balance.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-531.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "the `[` and `]` in command substitutions in a subscript must balance" || pe.Pos.Line() != 5 || pe.Pos.Col() != 21 {
		t.Fatalf("error = %v, want 5:21: the `[` and `]` in command substitutions in a subscript must balance", err)
	}
}

// Valid Zsh that the flag-pattern adapters also see keeps parsing.
func TestSubscriptBracketBalanceThroughAdapters(t *testing.T) {
	for _, src := range []string{
		"print ${x[(r)a[b]c,2]}\n",
		"print ${x[(r)$(echo [),$(echo ])]}\n",
		"print ${m[(r)$(echo [x])##]}\n",
		"print ${x[(r)a[bc]$(echo ok),2]}\n",
	} {
		if _, err := Parse(bytes.NewReader([]byte(src)), ""); err != nil {
			t.Errorf("Parse(%q) = %v", src, err)
		}
	}
}
