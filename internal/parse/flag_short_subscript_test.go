package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #532: a short subscript with flags inside a subscript flag argument
// is text of the argument, and the flag-pattern adapters still accept an
// unflagged one as before.
func TestFlagShortSubscriptThroughAdapters(t *testing.T) {
	for _, src := range []string{
		"print ${x[(r)$y[(r)a]]}\n",
		"print ${x[(r)$y[(r)a[b]c]]}\n",
		"print ${x[(r)$y[1]]}\n",
		"print ${x[(r)$y[1,2]]}\n",
		"print ${x[(r)a[b]c]}\n",
		// An unflagged short subscript followed by a bracket expression.
		"print ${x[(r)$y[1]x[b]]}\n",
	} {
		if _, err := Parse(bytes.NewReader([]byte(src)), ""); err != nil {
			t.Errorf("Parse(%q) = %v", src, err)
		}
	}
}

func TestFlagShortSubscriptRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-532-flag-short-subscript.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-532.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "reached EOF without matching `[` with `]`" || pe.Pos.Line() != 5 || pe.Pos.Col() != 10 {
		t.Fatalf("error = %v, want 5:10: reached EOF without matching `[` with `]`", err)
	}
}
