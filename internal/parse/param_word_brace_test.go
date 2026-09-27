package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #518: an unclosed `{` in the word of an unquoted parameter expansion
// leaves the expansion open, as Zsh reads it.
func TestParamWordBraceRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-518-param-word-brace.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-518.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "reached EOF without matching `${` with `}`" || pe.Pos.Line() != 5 || pe.Pos.Col() != 7 {
		t.Fatalf("error = %v, want 5:7: reached EOF without matching `${` with `}`", err)
	}
}
