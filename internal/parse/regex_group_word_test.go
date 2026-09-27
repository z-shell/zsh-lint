package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #517: an `=~` operand ends inside its group at `;`, as Zsh's word
// lexer ends it.
func TestRegexGroupWordEndRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-517-regex-group-word-end.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-517.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "not a valid test operator: `;`" || pe.Pos.Line() != 4 || pe.Pos.Col() != 12 {
		t.Fatalf("error = %v, want 4:12: not a valid test operator: `;`", err)
	}
}
