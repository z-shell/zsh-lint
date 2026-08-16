package parse

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// This catches removing the local parser-boundary compatibility adapter for
// native-valid case patterns whose inner group closes immediately before the
// case-arm terminator.
func TestParseGroupedCasePattern(t *testing.T) {
	const src = "case x in\n  (x|y)) : ;;\nesac\n"

	file, err := Parse(strings.NewReader(src), "grouped-case-pattern.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := len(file.AST().Stmts); got != 1 {
		t.Errorf("statement count = %d, want 1", got)
	}
	var rendered bytes.Buffer
	if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
		t.Fatalf("print AST: %v", err)
	}
	// The printer omits the optional case-arm opener, but the second closing
	// parenthesis remains part of the restored last pattern before the arm
	// terminator it renders.
	if got := rendered.String(); !strings.Contains(got, "x | y))") {
		t.Errorf("printed AST = %q, want original grouped case pattern", got)
	}
}

// These controls catch masking structural parentheses outside the exact case
// pattern boundary, or accepting an extra unmatched group closer.
func TestParseGroupedCasePatternControls(t *testing.T) {
	valid := []struct {
		name string
		src  string
	}{
		{name: "ungrouped alternatives", src: "case x in\n  (x|y) : ;;\nesac\n"},
		{name: "command substitution", src: "case x in\n  ($(print x)|y)) : ;;\nesac\n"},
		{name: "process substitution", src: "case x in\n  (<(print x)|y)) : ;;\nesac\n"},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(test.src), "grouped-case-control.zsh"); err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
		})
	}

	const malformed = "case x in\n  (x|y))) : ;;\nesac\n"
	if _, err := Parse(strings.NewReader(malformed), "malformed-grouped-case.zsh"); err == nil {
		t.Fatal("Parse() accepted a native-invalid grouped case pattern")
	}
}

// A deeper native-valid group currently hits a different mvdan/sh error. The
// #123 adapter must not replace that error with a secondary transformation
// failure or silently broaden its recognized grammar.
func TestParseGroupedCasePatternLeavesDifferentParserGapUntouched(t *testing.T) {
	const src = "case x in\n  ((x|y))) : ;;\nesac\n"
	_, firstErr := parseTree([]byte(src), "nested-grouped-case.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the nested grouped case")
	}
	_, err := Parse(strings.NewReader(src), "nested-grouped-case.zsh")
	if err == nil {
		t.Fatal("Parse() unexpectedly accepted the nested grouped case")
	}
	if err.Error() != firstErr.Error() {
		t.Errorf("Parse() error = %q, want original parser error %q", err, firstErr)
	}
}

// The same-length masked retry must preserve locations for syntax errors that
// occur after a successfully adapted case arm.
func TestParseGroupedCasePatternPreservesLaterErrorPosition(t *testing.T) {
	const src = "case x in\n  (x|y)) : ;;\nesac\n)\n"
	_, err := Parse(strings.NewReader(src), "later-error.zsh")
	if err == nil {
		t.Fatal("Parse() unexpectedly accepted trailing unmatched parenthesis")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error type = %T, want syntax.ParseError: %v", err, err)
	}
	if parseErr.Pos.Line() != 4 || parseErr.Pos.Col() != 1 {
		t.Errorf("error position = %d:%d, want 4:1", parseErr.Pos.Line(), parseErr.Pos.Col())
	}
}
