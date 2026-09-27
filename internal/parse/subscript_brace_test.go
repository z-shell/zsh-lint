package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #521: an unclosed `{` in the subscript of an unquoted ${...} leaves
// the expansion open, as Zsh's lexer reads it.
func TestSubscriptBraceRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-521-subscript-brace.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-521.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "a `{` in a subscript must be closed before its `]`" || pe.Pos.Line() != 5 || pe.Pos.Col() != 11 {
		t.Fatalf("error = %v, want 5:11: a `{` in a subscript must be closed before its `]`", err)
	}
}
