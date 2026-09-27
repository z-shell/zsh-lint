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

func TestConditionRejectsLoneDash(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-512-lone-dash.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-512-lone-dash.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "condition expected: -" || pe.Pos.Line() != 3 || pe.Pos.Col() != 4 {
		t.Fatalf("error = %v, want 3:4: condition expected: -", err)
	}
}

func TestConditionRejectsCmpOperand(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-512-cmp-operand.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-512-cmp-operand.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "not a valid test operator: `!`" || pe.Pos.Line() != 3 || pe.Pos.Col() != 8 {
		t.Fatalf("error = %v, want 3:8: not a valid test operator: `!`", err)
	}
}
