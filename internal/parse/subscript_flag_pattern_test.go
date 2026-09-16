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

func TestSubscriptFlagBracketPatternRejectsUnbalanced(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-237-empty-bracket-flag-pattern.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range [][]byte{
		src,
		[]byte("print -r -- ${line[(i)[a]]\n"),
		[]byte("print -r -- ${line[(i${x})[a]]}\n"),
	} {
		_, err := Parse(bytes.NewReader(source), "invalid-237.zsh")
		if err == nil {
			t.Fatalf("Parse(%q) unexpectedly succeeded", source)
		}
		var parseErr syntax.ParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("Parse(%q) error type = %T, want syntax.ParseError", source, err)
		}
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
