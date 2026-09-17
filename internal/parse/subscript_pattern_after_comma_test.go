package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #277: the expression after `,` in a flagged subscript may hold a
// bracket expression or start with `--`. The ok-* corpus fixture only proves
// the absence of an error; this pins the index shape on the original source:
// a `BinaryArithm` `,` whose left operand is the flagged pattern and whose
// right operand is one literal holding the expression's original bytes.
func TestParseSubscriptPatternAfterComma(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		flags      string
		pattern    string
		expression string
		col        uint
	}{
		{"bracket expression", "print -r -- ${m[(r)a,[^:]##]}\n", "r", "a", "[^:]##", 22},
		{"double dash before bracket expression", "print -r -- ${m[(r)a,--[^:]##]}\n", "r", "a", "--[^:]##", 22},
		{"corpus shape", "print -r -- ${m[(r)opt_$opt,--[^:]##]}\n", "r", "opt_$opt", "--[^:]##", 29},
		{"bracket expression after a name", "print -r -- ${m[(r)a,b[^:]]}\n", "r", "a", "b[^:]", 22},
		{"comma in bracket expression", "print -r -- ${m[(r)a,[a,b]]}\n", "r", "a", "[a,b]", 22},
		{"escaped bracket", "print -r -- ${m[(r)a,\\]x]}\n", "r", "a", "\\]x", 22},
		{"flag with argument", "print -r -- ${m[(rn:2:)a,[^:]]}\n", "rn:2:", "a", "[^:]", 26},
		{"operator after the subscript", "print -r -- ${m[(r)a,--[^:]##]:-none}\n", "r", "a", "--[^:]##", 22},
		{"leading blank", "print -r -- ${m[(r)a, [^:]]}\n", "r", "a", " [^:]", 22},
		{"index flag", "print -r -- ${m[(i)a,[^:]]}\n", "i", "a", "[^:]", 22},
		{"mask byte in expression", "print -r -- ${m[(r)a,_[^:]_]}\n", "r", "a", "_[^:]_", 22},
		{"assignment", "m[(r)a,[^:]##]=1\n", "r", "a", "[^:]##", 8},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			indexes := subscriptIndexes(file.AST())
			if len(indexes) != 1 {
				t.Fatalf("subscripts = %d, want 1", len(indexes))
			}
			binary, ok := indexes[0].(*syntax.BinaryArithm)
			if !ok || binary.Op != syntax.Comma {
				t.Fatalf("Index = %T %v, want *syntax.BinaryArithm with Op `,`", indexes[0], indexes[0])
			}
			flagged, ok := binary.X.(*syntax.FlagsArithm)
			if !ok {
				t.Fatalf("X = %T, want *syntax.FlagsArithm", binary.X)
			}
			if flagged.Flags == nil || flagged.Flags.Value != test.flags {
				t.Errorf("Flags = %v, want %q", flagged.Flags, test.flags)
			}
			if got := operandLiteral(t, flagged.X); got.Value != test.pattern {
				t.Errorf("pattern = %q, want %q", got.Value, test.pattern)
			}
			lit := operandLiteral(t, binary.Y)
			if lit.Value != test.expression {
				t.Errorf("expression = %q, want %q", lit.Value, test.expression)
			}
			if lit.Pos().Line() != 1 || lit.Pos().Col() != test.col {
				t.Errorf("expression position = %d:%d, want 1:%d", lit.Pos().Line(), lit.Pos().Col(), test.col)
			}
			if got := lit.End().Col(); got != test.col+uint(len(test.expression)) {
				t.Errorf("expression end column = %d, want %d", got, test.col+uint(len(test.expression)))
			}
			var rendered bytes.Buffer
			if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
				t.Fatalf("print AST: %v", err)
			}
			if !strings.Contains(rendered.String(), test.expression) {
				t.Errorf("printed AST = %q, want expression %q", rendered.String(), test.expression)
			}
		})
	}
}

func subscriptIndexes(tree *syntax.File) []syntax.ArithmExpr {
	var indexes []syntax.ArithmExpr
	syntax.Walk(tree, func(node syntax.Node) bool {
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
	return indexes
}

func operandLiteral(t *testing.T, expr syntax.ArithmExpr) *syntax.Lit {
	t.Helper()
	word, ok := expr.(*syntax.Word)
	if !ok || len(word.Parts) != 1 {
		t.Fatalf("operand = %T, want *syntax.Word with one part", expr)
	}
	lit, ok := word.Parts[0].(*syntax.Lit)
	if !ok {
		t.Fatalf("operand part = %T, want *syntax.Lit", word.Parts[0])
	}
	return lit
}

// Native Zsh reads the expression after `,` by the parameter's type: for a
// plain array `${a[(r)y,--x]}` decrements x, for an associative array the
// whole `y,--x` is one pattern. The parser cannot know the type, so an
// expression the parser already reads as arithmetic keeps that reading and
// only an expression it rejects becomes a literal.
func TestParseSubscriptPatternAfterCommaKeepsArithmeticReading(t *testing.T) {
	file, err := Parse(strings.NewReader("print -r -- ${a[(r)y,--x]} ${m[(r)a,b]}\n"), "arithmetic.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	indexes := subscriptIndexes(file.AST())
	if len(indexes) != 2 {
		t.Fatalf("subscripts = %d, want 2", len(indexes))
	}
	decrement, ok := indexes[0].(*syntax.BinaryArithm)
	if !ok || decrement.Op != syntax.Comma {
		t.Fatalf("first Index = %T, want *syntax.BinaryArithm with Op `,`", indexes[0])
	}
	unary, ok := decrement.Y.(*syntax.UnaryArithm)
	if !ok || unary.Op != syntax.Dec || unary.Post {
		t.Errorf("first Y = %T %v, want a *syntax.UnaryArithm pre-decrement", decrement.Y, decrement.Y)
	}
	plain, ok := indexes[1].(*syntax.BinaryArithm)
	if !ok || plain.Op != syntax.Comma {
		t.Fatalf("second Index = %T, want *syntax.BinaryArithm with Op `,`", indexes[1])
	}
	if got := operandLiteral(t, plain.Y); got.Value != "b" {
		t.Errorf("second Y = %q, want %q", got.Value, "b")
	}
}

// Two sites on one line reach the adapter one retry level apart; each level
// restores only its own mask.
func TestParseSubscriptPatternAfterCommaTwoSites(t *testing.T) {
	const src = "print -r -- ${m[(r)a,[^:]]} ${m[(r)b,--[^:]##]}\n"
	file, err := Parse(strings.NewReader(src), "two-sites.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	indexes := subscriptIndexes(file.AST())
	if len(indexes) != 2 {
		t.Fatalf("subscripts = %d, want 2", len(indexes))
	}
	for i, want := range []string{"[^:]", "--[^:]##"} {
		binary, ok := indexes[i].(*syntax.BinaryArithm)
		if !ok || binary.Op != syntax.Comma {
			t.Fatalf("Index %d = %T, want *syntax.BinaryArithm with Op `,`", i, indexes[i])
		}
		if got := operandLiteral(t, binary.Y); got.Value != want {
			t.Errorf("expression %d = %q, want %q", i, got.Value, want)
		}
	}
	var rendered bytes.Buffer
	if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
		t.Fatalf("print AST: %v", err)
	}
	if got := rendered.String(); got != src {
		t.Errorf("printed AST = %q, want %q", got, src)
	}
}

// The zi corpus line carries a second expansion with its own subscript on the
// same line; the retry must leave it and every other byte in place.
func TestParseSubscriptPatternAfterCommaCorpusLine(t *testing.T) {
	const src = "local msg=${___opt_map[$opt]#*:} txt=${___opt_map[(r)opt_$opt,--[^:]##]}\n"
	file, err := Parse(strings.NewReader(src), "corpus-line.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	indexes := subscriptIndexes(file.AST())
	if len(indexes) != 2 {
		t.Fatalf("subscripts = %d, want 2", len(indexes))
	}
	binary, ok := indexes[1].(*syntax.BinaryArithm)
	if !ok || binary.Op != syntax.Comma {
		t.Fatalf("second Index = %T, want *syntax.BinaryArithm with Op `,`", indexes[1])
	}
	if got := operandLiteral(t, binary.Y); got.Value != "--[^:]##" || got.Pos().Col() != 63 {
		t.Errorf("expression = %q at column %d, want %q at 63", got.Value, got.Pos().Col(), "--[^:]##")
	}
	var rendered bytes.Buffer
	if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
		t.Fatalf("print AST: %v", err)
	}
	if got := rendered.String(); got != src {
		t.Errorf("printed AST = %q, want %q", got, src)
	}
}

// Native-invalid sources keep the base front end's error family and original
// position; the retry must not move or replace the failure with a mask.
func TestSubscriptPatternAfterCommaRejectsUnterminated(t *testing.T) {
	fixture, err := os.ReadFile("testdata/invalid-277-unterminated-pattern-after-comma.txt")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		src  []byte
		text string
		col  uint
	}{
		{"unterminated subscript", fixture, "`[` must follow a name like a[i]", 12},
		{"unterminated expansion", []byte("x=${m[(r)a,[^:]]\n"), "not a valid parameter expansion operator: \"\\n\"", 17},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertParseErrorAt(t, test.src, test.text, 1, test.col)
		})
	}
}

// An expression whose extent the scanner cannot decide, or a subscript that is
// not the construct, keeps the base front end's error at its original position
// instead of a guessed mask.
func TestSubscriptPatternAfterCommaLeavesUncertainExpressionsAlone(t *testing.T) {
	tests := []struct {
		name string
		src  []byte
		text string
		col  uint
	}{
		{"nested expansion", []byte("x=${m[(r)a,${x}[^:]]}\n"), "`[` must follow a name like a[i]", 16},
		{"command substitution", []byte("x=${m[(r)a,[^:]$(x)]}\n"), "`[` must follow a name like a[i]", 12},
		{"backtick substitution", []byte("x=${m[(r)a,`x`[^:]]}\n"), "`[` must follow a name like a[i]", 15},
		{"second comma", []byte("x=${m[(r)a,[^:],3]}\n"), "`[` must follow a name like a[i]", 12},
		{"no flag group", []byte("x=${m[a,[^:]]}\n"), "`[` must follow a name like a[i]", 9},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertParseErrorAt(t, test.src, test.text, 1, test.col)
		})
	}
}

func TestSubscriptPatternAfterCommaPreservesLaterErrorPosition(t *testing.T) {
	const src = "x=${m[(r)a,[^:]]}\n)\n"
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
