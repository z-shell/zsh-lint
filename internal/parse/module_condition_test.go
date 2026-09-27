package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestModuleConditionRejectsExtraInfixOperand(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-484-module-infix-arity.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-484.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "not a valid test operator: `c`" || pe.Pos.Line() != 3 || pe.Pos.Col() != 13 {
		t.Fatalf("error = %v, want 3:13: not a valid test operator: `c`", err)
	}
}
