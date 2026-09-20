package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #301: `select name [in word ...] term { list }`, the brace body of
// select. The adapter writes `do` over the `{` and `done` over the `}`, so
// the loop holds the block's list directly with `do` and `done` on the
// brace bytes, and every other node keeps its original position.
func TestParseSelectBraceBody(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		loops []string // select:do:done/count, as emptySelectLoops prints them
		first string   // position and text of the first body statement, or ""
	}{
		{"one line", "select o in a b c; { print $o; break }\n", []string{"1:1:1:20:1:38/2"}, "1:22 print $o;"},
		{"no list no term", "select o { break }\n", []string{"1:1:1:10:1:18/1"}, "1:12 break"},
		{"no list", "select o; { break }\n", []string{"1:1:1:11:1:19/1"}, "1:13 break"},
		{"newline term", "select o in a b c\n{ break }\n", []string{"1:1:2:1:2:9/1"}, "2:3 break"},
		{"separator and newline", "select o in a b c;\n{ break }\n", []string{"1:1:2:1:2:9/1"}, "2:3 break"},
		{"multi-line block with comment", "select o in a b c; {\n  # inside\n  print -r -- \"$o\"\n  break\n}\n", []string{"1:1:1:20:5:1/2"}, "3:3 print -r -- \"$o\""},
		{"quoted closer in block", "select o in a b c; {\n  print -r -- \"}\"\n  # } in a comment\n  break\n}\n", []string{"1:1:1:20:5:1/2"}, "2:3 print -r -- \"}\""},
		{"nested select and anonymous function", "select o in a b c; {\n  () { : }\n  select o2 in x; { break }\n  break\n}\n",
			[]string{"1:1:1:20:5:1/3", "3:3:3:19:3:27/1"}, "2:3 () { : }"},
		{"in a function", "f() { select o in a; { break } }\n", []string{"1:7:1:22:1:30/1"}, "1:24 break"},
		{"in a loop body", "while true; do select o in a; { break }; break; done\n", []string{"1:16:1:31:1:39/1"}, "1:33 break"},
		{"quoted braces in the list", "select o in '{' \"}\"; { break }\n", []string{"1:1:1:22:1:30/1"}, "1:24 break"},
		{"empty block", "select o in a; { }\n", []string{"1:1:1:16:1:18/0"}, ""},
		{"statement after", "select o in a b c; { break }; print after\n", []string{"1:1:1:20:1:28/1"}, "1:22 break"},
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
			if got := test.src[loop.DoPos.Offset()]; got != '{' {
				t.Errorf("DoPos %s reads %q, want '{'", loop.DoPos, got)
			}
			if got := test.src[loop.DonePos.Offset()]; got != '}' {
				t.Errorf("DonePos %s reads %q, want '}'", loop.DonePos, got)
			}
			if loop.Braces {
				t.Errorf("Braces set; the for adapter leaves it unset for a brace body")
			}
			first := ""
			if len(loop.Do) > 0 {
				stmt := loop.Do[0]
				first = stmt.Pos().String() + " " + test.src[stmt.Pos().Offset():stmt.End().Offset()]
			}
			if first != test.first {
				t.Errorf("first body statement = %q, want %q", first, test.first)
			}
		})
	}
}

// A tail after the `}` binds to the loop, not to the body, as native Zsh
// binds it: the loop is the left operand of the `&&` or `|`, whose
// operator sits after the loop's `done`. Before #301 the short form took
// the whole chain as the body.
func TestParseSelectBraceBodyTailBindsToLoop(t *testing.T) {
	for _, test := range []struct {
		src string
		op  string
	}{
		{"select o in a b c; { print $o } && print x\n", "&&"},
		{"select o in a b c; { print $o } | cat\n", "|"},
		{"select o in a b c; { print $o } || print x\n", "||"},
	} {
		t.Run(test.op, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), "tail.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			loops := selectLoops(file.AST())
			if len(loops) != 1 || len(loops[0].Do) != 1 {
				t.Fatalf("loops = %v", emptySelectLoops(file.AST()))
			}
			var binary *syntax.BinaryCmd
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
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
			if binary.X.Cmd != loops[0] {
				t.Errorf("left operand = %T, want the select loop", binary.X.Cmd)
			}
			if binary.OpPos.Offset() <= loops[0].DonePos.Offset() {
				t.Errorf("OpPos %s is not after DonePos %s", binary.OpPos, loops[0].DonePos)
			}
			body := loops[0].Do[0]
			if got := test.src[body.Pos().Offset():body.End().Offset()]; got != "print $o" {
				t.Errorf("body = %q, want the block's list only", got)
			}
		})
	}
}

// A negated block after the header is a negated sublist (the short form),
// not a brace body; a `{` glued to its first word is a word.
func TestParseSelectBraceBodyLeavesOtherBlocksToTheShortForm(t *testing.T) {
	for _, test := range []struct {
		src     string
		negated bool
	}{
		{"select o in a; ! { break }\n", true},
		{"select o in a; {break}\n", false},
	} {
		file, err := Parse(strings.NewReader(test.src), "other.zsh")
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", test.src, err)
		}
		loop := selectLoops(file.AST())[0]
		if len(loop.Do) != 1 {
			t.Fatalf("%q: body statements = %d, want 1", test.src, len(loop.Do))
		}
		body := loop.Do[0]
		if body.Negated != test.negated {
			t.Errorf("%q: Negated = %v, want %v", test.src, body.Negated, test.negated)
		}
		if test.negated {
			if _, ok := body.Cmd.(*syntax.Block); !ok {
				t.Errorf("%q: body Cmd = %T, want the negated block as the sublist", test.src, body.Cmd)
			}
		} else if _, ok := body.Cmd.(*syntax.CallExpr); !ok {
			t.Errorf("%q: body Cmd = %T, want the glued word as a call", test.src, body.Cmd)
		}
		if got := test.src[loop.DonePos.Offset()]; got == '}' {
			t.Errorf("%q: DonePos on a brace; want the short form's closer after the sublist", test.src)
		}
	}
}

// Native Zsh rejects each fixture (`zsh -f -n`); the parser error is kept.
func TestParseSelectBraceBodyRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		line    uint
		col     uint
	}{
		{"invalid-301-unterminated-block.txt", "reached EOF without matching `{` with `}`", 1, 20},
		{"invalid-301-closer-before-opener.txt", "`}` can only be used to close a block", 1, 20},
		{"invalid-301-extra-closer.txt", "`}` can only be used to close a block", 1, 30},
		{"invalid-301-case-terminator-after-block.txt", "`;;` can only be used in a case clause", 1, 29},
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

// A retry whose tree lacks the loop at the site is not trusted.
func TestParseSelectBraceBodyFailsClosed(t *testing.T) {
	src := []byte("select o in a b c; { break }\n")
	_, firstErr := parseTree(src, "closed.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the brace body")
	}
	calls := 0
	_, err := parseSelectShortFormWithParser(src, "closed.zsh", firstErr, func(masked []byte, name string) (*syntax.File, error) {
		calls++
		if calls == 1 {
			return parseTree(masked, name) // the probe
		}
		if string(masked) != "select o in a b c; do\n break \ndone\n" {
			t.Fatalf("retry source = %q", masked)
		}
		return &syntax.File{}, nil
	})
	if err != firstErr {
		t.Fatalf("error = %v, want the incoming error %v", err, firstErr)
	}
	if calls != 2 {
		t.Fatalf("parser called %d times, want 2 (probe and retry)", calls)
	}
}
