package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #526: the body of a command substitution in a subscript flag
// argument is parsed as commands, so an invalid one is reported where it
// stands.
func TestFlagSubstBodyRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-526-flag-subst-body.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-526.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "`done` can only be used to end a loop" || pe.Pos.Line() != 5 || pe.Pos.Col() != 16 {
		t.Fatalf("error = %v, want 5:16: `done` can only be used to end a loop", err)
	}
}

// The flag-pattern adapters mask and retry a flagged subscript holding a
// bracket expression or a nested expansion; a valid substitution there
// still parses.
func TestFlagSubstBodyThroughAdapters(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"print ${x[(r)a[bc]$(echo ok)]}\n", ""},
		{"print ${m[(i)${Y[a]}]}\n", ""},
		{"print ${m[(i)${Y[a]}$(echo ok)]}\n", ""},
	} {
		_, err := Parse(bytes.NewReader([]byte(tc.src)), "")
		got := ""
		if err != nil {
			got = err.Error()
		}
		if got != tc.err {
			t.Fatalf("%q: error = %q, want %q", tc.src, got, tc.err)
		}
	}
}
