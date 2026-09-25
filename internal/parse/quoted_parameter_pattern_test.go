package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// A `${...}` inside a double-quoted word opens its own quoting context, so a
// nested string with a quote character in it no longer hides the pattern
// group that follows (#401). Each row is native-valid; the quoted word must
// come back byte for byte, including a `)` the adapter masks for the retry.
func TestQuotedStringInQuotedParameterPattern(t *testing.T) {
	for _, tt := range []struct{ src, word string }{
		{`[[ "${x:-"it's"}" == (a|(b|c)) ]]`, `"${x:-"it's"}"`},
		{`[[ "${x:-")"}" == (a|(b|c)) ]]`, `"${x:-")"}"`},
		{`[[ a == (a|(b|"${x:-)}")) ]]`, `"${x:-)}"`},
		{`[[ a == (a|(b|"${x:-"it's"}")) ]]`, `"${x:-"it's"}"`},
	} {
		file, err := Parse(strings.NewReader(tt.src+"\n"), "t.zsh")
		if err != nil {
			t.Fatalf("%s: %v", tt.src, err)
		}
		var operands []string
		syntax.Walk(file.tree, func(node syntax.Node) bool {
			if test, ok := node.(*syntax.BinaryTest); ok {
				for _, operand := range []syntax.TestExpr{test.X, test.Y} {
					var printed strings.Builder
					if err := syntax.NewPrinter().Print(&printed, operand); err != nil {
						t.Fatalf("%s: print: %v", tt.src, err)
					}
					operands = append(operands, printed.String())
				}
			}
			return true
		})
		if len(operands) != 2 || !strings.Contains(strings.Join(operands, " "), tt.word) {
			t.Errorf("%s: test operands %q, want %q in one of them", tt.src, operands, tt.word)
		}
	}
}

// An unterminated nested string stays a parse error, as `zsh -f -n` has it.
func TestQuotedStringInQuotedParameterPatternRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-401-unterminated-nested-string.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-401.zsh")
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("Parse() error = %v, want a parse error", err)
	}
}

// A `$` that ends the source inside a double-quoted word, or a `${` whose
// expansion never closes, is an unterminated string: a parse error, never a
// panic or a skipped extent.
func TestQuotedParameterPatternAtEndOfSource(t *testing.T) {
	for _, src := range []string{
		`[[ a == (a|(b|c)) ]] && print "$`,
		`[[ a == (a|(b|c)) ]] && print "${`,
		`[[ a == (a|(b|"${x:-`,
	} {
		_, err := Parse(strings.NewReader(src), "t.zsh")
		var parseErr syntax.ParseError
		if !errors.As(err, &parseErr) {
			t.Errorf("%q: Parse() error = %v, want a parse error", src, err)
		}
	}
}
