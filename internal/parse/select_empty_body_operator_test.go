package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #319: a select header with an empty body may be the left operand
// of `|`, `|&`, `||` or `&&`. The loop keeps the empty-body shape of #302
// (`do` and `done` on the body's first byte, no statements) and is the
// operator's left operand, whose operator sits at or after the loop's
// `done`; the right operand keeps its own position. The parenthesized form
// (#303) reaches the same path.
func TestParseSelectEmptyBodyOperator(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		loops []string // select:do:done/count
		op    string
		right string // position and text of the right operand
	}{
		{"pipe", "select o in a b c; | cat\n", []string{"1:1:1:20:1:20/0"}, "|", "1:22 cat"},
		{"and", "select o in a b c; && print x\n", []string{"1:1:1:20:1:20/0"}, "&&", "1:23 print x"},
		{"or", "select o in a b c; || print x\n", []string{"1:1:1:20:1:20/0"}, "||", "1:23 print x"},
		{"pipe stderr", "select o in a b c; |& cat\n", []string{"1:1:1:20:1:20/0"}, "|&", "1:23 cat"},
		{"operator on the next line", "select o in a b c;\n| cat\n", []string{"1:1:2:1:2:1/0"}, "|", "2:3 cat"},
		{"paren list glued pipe", "select o (a b c)|cat\n", []string{"1:1:1:17:1:17/0"}, "|", "1:18 cat"},
		{"paren list glued and", "select o (a b c)&&print x\n", []string{"1:1:1:17:1:17/0"}, "&&", "1:19 print x"},
		{"paren list and", "select o (a b c) && print x\n", []string{"1:1:1:18:1:18/0"}, "&&", "1:21 print x"},
		{"in a command substitution", "x=$(select o in a; | cat)\n", []string{"1:5:1:20:1:20/0"}, "|", "1:22 cat"},
		{"negated", "! select o in a; | cat\n", []string{"1:3:1:18:1:18/0"}, "|", "1:20 cat"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			tree := file.AST()
			assertLiteralsMatchSource(t, tree, test.src)
			if got := emptySelectLoops(tree); strings.Join(got, "|") != strings.Join(test.loops, "|") {
				t.Fatalf("loops = %v, want %v", got, test.loops)
			}
			loop := selectLoops(tree)[0]
			var binary *syntax.BinaryCmd
			syntax.Walk(tree, func(node syntax.Node) bool {
				if b, ok := node.(*syntax.BinaryCmd); ok && binary == nil {
					binary = b
				}
				return true
			})
			if binary == nil {
				t.Fatal("no BinaryCmd in the tree")
			}
			if got := binary.Op.String(); got != test.op {
				t.Errorf("Op = %s, want %s", got, test.op)
			}
			if binary.X.Cmd != loop {
				t.Errorf("left operand = %T, want the select loop", binary.X.Cmd)
			}
			if binary.OpPos.Offset() < loop.DonePos.Offset() {
				t.Errorf("OpPos %s is before DonePos %s", binary.OpPos, loop.DonePos)
			}
			if got := test.src[binary.OpPos.Offset() : binary.OpPos.Offset()+uint(len(test.op))]; got != test.op {
				t.Errorf("OpPos %s reads %q, want %q", binary.OpPos, got, test.op)
			}
			right := binary.Y.Pos().String() + " " + test.src[binary.Y.Pos().Offset():binary.Y.End().Offset()]
			if right != test.right {
				t.Errorf("right operand = %q, want %q", right, test.right)
			}
		})
	}
}

// A chain of two empty-bodied selects: the second header is the first
// loop's right operand and itself an empty-bodied loop piped on.
func TestParseSelectEmptyBodyOperatorChain(t *testing.T) {
	src := "select o in a; | select p in b; | cat\n"
	file, err := Parse(strings.NewReader(src), "chain.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := emptySelectLoops(file.AST()); strings.Join(got, "|") != "1:1:1:16:1:16/0|1:18:1:33:1:33/0" {
		t.Fatalf("loops = %v", got)
	}
}

// Native Zsh rejects each fixture (`zsh -f -n` reports a parse error): a
// lone `&` would background an empty sublist, and `;|` and `;&` are case
// terminators.
func TestParseSelectEmptyBodyOperatorRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		line    uint
		col     uint
	}{
		{"invalid-319-background-empty-body.txt", "select loop body must be a command", 1, 20},
		{"invalid-319-paren-background.txt", "select loop body must be a command", 1, 17},
		{"invalid-319-glued-pipe.txt", "`select foo in words` must be followed by `;` or a newline", 1, 10},
		{"invalid-319-glued-ampersand.txt", "`select foo in words` must be followed by `;` or a newline", 1, 10},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + test.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			assertParseErrorAt(t, src, test.text, test.line, test.col)
		})
	}
}
