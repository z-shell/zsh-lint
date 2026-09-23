package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #237: a `]` inside a flagged subscript pattern must not close the
// subscript. The ok-* corpus fixture only proves the absence of an error; this
// pins the flag and the pattern literal at their original positions.
func TestParseSubscriptFlagBracketPattern(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		flags   string
		pattern string
		col     uint
	}{
		{"bracket expression", "print -r -- ${line[(i)[a]]}\n", "i", "[a]", 23},
		{"escaped quote in bracket expression", "print -r -- ${line[(I)[\\']]}\n", "I", "[\\']", 23},
		{"bracket expression before star", "print -r -- ${line[(r)[ab]*]}\n", "r", "[ab]*", 23},
		{"two bracket expressions", "print -r -- ${line[(i)x[a]y[b]]}\n", "i", "x[a]y[b]", 23},
		{"character class", "print -r -- ${line[(i)[[:alpha:]]*]}\n", "i", "[[:alpha:]]*", 23},
		{"comma in bracket expression", "print -r -- ${line[(i)[a,b]]}\n", "i", "[a,b]", 23},
		{"negated bracket expression with operator", "print -r -- ${line[(i)[^a]]:-none}\n", "i", "[^a]", 23},
		{"escaped bracket outside", "print -r -- ${line[(i)a\\]b]}\n", "i", "a\\]b", 23},
		{"flag with argument", "print -r -- ${line[(n:2:)[a]]}\n", "n:2:", "[a]", 26},
		{"assignment", "line[(i)[a]]=x\n", "i", "[a]", 9},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var indexes []syntax.ArithmExpr
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				switch n := node.(type) {
				case *syntax.ParamExp:
					if n.Index != nil {
						indexes = append(indexes, n.Index)
					}
				case *syntax.Assign:
					if n.Index != nil {
						indexes = append(indexes, n.Index)
					}
				}
				return true
			})
			if len(indexes) != 1 {
				t.Fatalf("subscripts = %d, want 1", len(indexes))
			}
			flagged, ok := indexes[0].(*syntax.FlagsArithm)
			if !ok {
				t.Fatalf("Index = %T, want *syntax.FlagsArithm", indexes[0])
			}
			if flagged.Flags == nil || flagged.Flags.Value != test.flags {
				t.Errorf("Flags = %v, want %q", flagged.Flags, test.flags)
			}
			word, ok := flagged.X.(*syntax.Word)
			if !ok || len(word.Parts) != 1 {
				t.Fatalf("X = %T, want *syntax.Word with one part", flagged.X)
			}
			lit, ok := word.Parts[0].(*syntax.Lit)
			if !ok {
				t.Fatalf("X part = %T, want *syntax.Lit", word.Parts[0])
			}
			if lit.Value != test.pattern {
				t.Errorf("pattern = %q, want %q", lit.Value, test.pattern)
			}
			if lit.Pos().Line() != 1 || lit.Pos().Col() != test.col {
				t.Errorf("pattern position = %d:%d, want 1:%d", lit.Pos().Line(), lit.Pos().Col(), test.col)
			}
			if got := lit.End().Col(); got != test.col+uint(len(test.pattern)) {
				t.Errorf("pattern end column = %d, want %d", got, test.col+uint(len(test.pattern)))
			}
			var rendered bytes.Buffer
			if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
				t.Fatalf("print AST: %v", err)
			}
			if !strings.Contains(rendered.String(), test.pattern) {
				t.Errorf("printed AST = %q, want pattern %q", rendered.String(), test.pattern)
			}
		})
	}
}

// The zunit corpus line nests two flagged searches inside an arithmetic range.
// The range bounds still take the wrong shape from #246, so this only pins
// that every masked byte is restored and the line round-trips.
func TestParseSubscriptFlagBracketPatternNested(t *testing.T) {
	const src = "testname=\"${line[(( ${line[(i)[\\']]}+1 )),(( ${line[(I)[\\']]}-1 ))]}\"\n"
	file, err := Parse(strings.NewReader(src), "nested.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	var rendered bytes.Buffer
	if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
		t.Fatalf("print AST: %v", err)
	}
	for _, want := range []string{"${line[(i)[\\']]}", "${line[(I)[\\']]}"} {
		if !strings.Contains(rendered.String(), want) {
			t.Errorf("printed AST = %q, want %q", rendered.String(), want)
		}
	}
}

// Native-invalid sources keep the base front end's error family and original
// position; the retry must not move or replace the failure.
func TestSubscriptFlagBracketPatternRejectsUnbalanced(t *testing.T) {
	fixture, err := os.ReadFile("testdata/invalid-237-empty-bracket-flag-pattern.txt")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		src  []byte
		text string
		col  uint
	}{
		{"empty bracket expression", fixture, "not a valid parameter expansion operator: `]`", 27},
		{"unterminated expansion", []byte("print -r -- ${line[(i)[a]]\n"), "not a valid parameter expansion operator: \"\\n\"", 27},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertParseErrorAt(t, test.src, test.text, 1, test.col)
		})
	}
}

// A pattern whose extent the scanner cannot decide (an expansion in the flags,
// a command substitution in the pattern) keeps the base front end's error at
// its original position instead of a guessed mask.
func TestSubscriptFlagBracketPatternLeavesUncertainPatternsAlone(t *testing.T) {
	tests := []struct {
		name string
		src  []byte
		text string
		col  uint
	}{
		{"expansion in flags", []byte("print -r -- ${line[(i${x})[a]]}\n"), "not a valid parameter expansion operator: `]`", 30},
		{"backtick substitution in pattern", []byte("print -r -- ${line[(i)`echo [x]`]}\n"), "not a valid parameter expansion operator: \"`\"", 32},
		{"dollar substitution in pattern", []byte("print -r -- ${line[(i)$(echo [x])]}\n"), "not a valid parameter expansion operator: `)`", 33},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertParseErrorAt(t, test.src, test.text, 1, test.col)
		})
	}
}

// maskNestedExpansion decides where a nested expansion inside a flagged
// pattern ends. Tested directly: an unbalanced expansion cannot reach it
// through Parse, because the parser fails at the `$` before the retry is
// seeded. The mask callback is exercised by TestMaskNestedExpansion; this
// covers the extent alone.
func TestMaskNestedExpansionExtent(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int // offset past the closing `}`, or -1 for refused
	}{
		{"simple", "${s}x", 4},
		{"nested braces", "${a${b}c}x", 9},
		{"unbalanced", "${s", -1},
		{"newline before close", "${s\n}", -1},
		{"escaped brace", "${a\\}b}x", 7},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// The `{` is at offset 1 in each source.
			end, ok := maskNestedExpansion([]byte(test.src), 1, func(int) {})
			if test.want < 0 {
				if ok {
					t.Errorf("maskNestedExpansion(%q) = %d, want refused", test.src, end)
				}
				return
			}
			if !ok || end != test.want {
				t.Errorf("maskNestedExpansion(%q) = %d, %v, want %d, true", test.src, end, ok, test.want)
			}
		})
	}
}

func assertParseErrorAt(t *testing.T, src []byte, text string, line, col uint) {
	t.Helper()
	_, err := Parse(bytes.NewReader(src), "flag-pattern.zsh")
	if err == nil {
		t.Fatalf("Parse(%q) unexpectedly succeeded", src)
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("Parse(%q) error type = %T, want syntax.ParseError", src, err)
	}
	if parseErr.Text != text {
		t.Errorf("error text = %q, want %q", parseErr.Text, text)
	}
	if parseErr.Pos.Line() != line || parseErr.Pos.Col() != col {
		t.Errorf("error position = %d:%d, want %d:%d", parseErr.Pos.Line(), parseErr.Pos.Col(), line, col)
	}
}

func TestSubscriptFlagBracketPatternPreservesLaterErrorPosition(t *testing.T) {
	const src = "print -r -- ${line[(i)[a]]}\n)\n"
	_, err := Parse(strings.NewReader(src), "later-error.zsh")
	if err == nil {
		t.Fatal("Parse() unexpectedly accepted a trailing unmatched parenthesis")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error type = %T, want syntax.ParseError: %v", err, err)
	}
	if parseErr.Pos.Line() != 2 || parseErr.Pos.Col() != 1 {
		t.Errorf("error position = %d:%d, want 2:1", parseErr.Pos.Line(), parseErr.Pos.Col())
	}
}
