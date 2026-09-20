package parse

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// doLoops describes every `do ... done` loop in the tree in source order as
// `kind do:firstStmt/count`, where kind names the loop, do is the position
// of its `do`, firstStmt the position of its first body statement (`-` for
// an empty body) and count the number of body statements.
func doLoops(tree *syntax.File) []string {
	var found []string
	describe := func(kind string, do syntax.Pos, body []*syntax.Stmt) {
		first := "-"
		if len(body) > 0 {
			first = body[0].Pos().String()
		}
		found = append(found, fmt.Sprintf("%s %s:%s/%d", kind, do, first, len(body)))
	}
	syntax.Walk(tree, func(node syntax.Node) bool {
		switch loop := node.(type) {
		case *syntax.WhileClause:
			kind := "while"
			if loop.Until {
				kind = "until"
			}
			describe(kind, loop.DoPos, loop.Do)
		case *syntax.ForClause:
			kind := "for"
			if loop.Select {
				kind = "select"
			}
			if !loop.Braces {
				describe(kind, loop.DoPos, loop.Do)
			}
		}
		return true
	})
	return found
}

// Issue #238: a loop body may begin with a separator directly after `do`.
// Native Zsh reads the leading `;` as an empty sublist; the parser reads it
// as the whole body and then demands `done`. The adapter must accept every
// loop kind, keep every position, and leave the body statements where the
// source has them.
func TestParseDoLeadingSeparator(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		loops []string
	}{
		{"while", "while (( $# )); do; shift; done\n", []string{"while 1:17:1:21/1"}},
		{"for", "for x in a b; do; print $x; done\n", []string{"for 1:15:1:19/1"}},
		{"until", "until (( $# )); do; shift; done\n", []string{"until 1:17:1:21/1"}},
		{"select", "select o in a b c; do; print $o; break; done\n", []string{"select 1:20:1:24/2"}},
		{"c-style for", "for ((i = 0; i < 2; i++)); do; print $i; done\n", []string{"for 1:28:1:32/1"}},
		{"corpus site", "freload() { while (( $# )); do; unfunction $1; autoload -U $1; shift; done }\n", []string{"while 1:29:1:33/3"}},
		{"glued to statement", "while true; do;break; done\n", []string{"while 1:13:1:16/1"}},
		{"blank before separator", "while true; do ; break; done\n", []string{"while 1:13:1:18/1"}},
		{"tab before separator", "while true; do\t;break; done\n", []string{"while 1:13:1:17/1"}},
		{"two separators", "while true; do; ; break; done\n", []string{"while 1:13:1:19/1"}},
		{"separator on next line", "while true; do\n; break; done\n", []string{"while 1:13:2:3/1"}},
		{"separators on both lines", "while true; do;\n; break; done\n", []string{"while 1:13:2:3/1"}},
		{"comment before separator", "while true; do # comment\n; break; done\n", []string{"while 1:13:2:3/1"}},
		{"comment after separator", "while true; do; # comment\nbreak; done\n", []string{"while 1:13:2:1/1"}},
		{"nested loops", "while true; do; while true; do; break; done; break; done\n", []string{"while 1:13:1:17/2", "while 1:29:1:33/1"}},
		{"two loops", "while true; do; break; done; while true; do; break; done\n", []string{"while 1:13:1:17/1", "while 1:42:1:46/1"}},
		{"loop in pipeline", "while true; do; break; done | cat\n", []string{"while 1:13:1:17/1"}},
		{"loop in substitution", "while true; do; print $(while true; do; break; done); break; done\n", []string{"while 1:13:1:17/2", "while 1:37:1:41/1"}},
		{"do as a word in the list", "for x in do; do; print $x; done\n", []string{"for 1:14:1:18/1"}},
		{"do as an argument in the condition", "while print do; do; break; done\n", []string{"while 1:17:1:21/1"}},
		{"function in body", "while true; do; f() { ; }; done\n", []string{"while 1:13:1:17/1"}},
		{"heredoc lookalike in body", "while true; do; cat <<EOF\ndo;\nEOF\nbreak; done\n", []string{"while 1:13:1:17/2"}},
		{"quoted lookalikes in body", "while true; do; print 'do; x' \"do; y\"; break; done\n", []string{"while 1:13:1:17/2"}},
		{"after a parsed empty body", "while true; do; done\nwhile true; do; break; done\n", []string{"while 1:13:-/0", "while 2:13:2:17/1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := []byte(test.src)
			_, firstErr := parseTree(src, test.name+".zsh")
			var parseErr syntax.ParseError
			if !errors.As(firstErr, &parseErr) || !isDoEmptyBodyError(parseErr.Text) {
				t.Fatalf("parseTree() error = %v, want an empty body error", firstErr)
			}

			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			tree := file.AST()
			assertLiteralsMatchSource(t, tree, test.src)

			if got := doLoops(tree); strings.Join(got, "|") != strings.Join(test.loops, "|") {
				t.Fatalf("loops = %v, want %v", got, test.loops)
			}
			syntax.Walk(tree, func(node syntax.Node) bool {
				var do syntax.Pos
				switch loop := node.(type) {
				case *syntax.WhileClause:
					do = loop.DoPos
				case *syntax.ForClause:
					do = loop.DoPos
				default:
					return true
				}
				if got := test.src[do.Offset() : do.Offset()+2]; got != "do" {
					t.Errorf("DoPos %s reads %q in the source, want \"do\"", do, got)
				}
				return true
			})
		})
	}
}

// The adapter must not move any other node: the tree of a body with a
// leading separator matches the tree of the same source with a blank in its
// place, byte for byte.
func TestParseDoLeadingSeparatorMatchesBlank(t *testing.T) {
	tests := []struct {
		name string
		src  string
		twin string
	}{
		{"same line", "while true; do; print a; break; done\nprint b\n", "while true; do  print a; break; done\nprint b\n"},
		{"next line", "for x in a b; do\n  ; print $x\ndone\n", "for x in a b; do\n    print $x\ndone\n"},
		{"after comment", "until false; do # c\n; print x\ndone\n", "until false; do # c\n  print x\ndone\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if len(test.twin) != len(test.src) {
				t.Fatalf("twin length %d, want %d", len(test.twin), len(test.src))
			}
			file, err := Parse(strings.NewReader(test.src), "separator.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			twinFile, err := Parse(strings.NewReader(test.twin), "blank.zsh")
			if err != nil {
				t.Fatalf("Parse(twin) error: %v", err)
			}
			var got, want []string
			collect := func(tree *syntax.File, into *[]string) {
				syntax.Walk(tree, func(node syntax.Node) bool {
					if node == nil {
						return true
					}
					*into = append(*into, fmt.Sprintf("%T %s %s", node, node.Pos(), node.End()))
					return true
				})
			}
			collect(file.AST(), &got)
			collect(twinFile.AST(), &want)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("nodes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// The mask never touches a comment byte, so every comment around the
// separator survives at its own position for suppression directives.
func TestParseDoLeadingSeparatorKeepsComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"before separator", "while true; do # c\n; break # d\ndone\n", []string{"1:16  c", "2:9  d"}},
		{"after separator", "while true; do; # c\nbreak\ndone\n", []string{"1:17  c"}},
		{"between separators", "while true; do; # c\n; break\ndone\n", []string{"1:17  c"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var got []string
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if comment, ok := node.(*syntax.Comment); ok {
					got = append(got, comment.Pos().String()+" "+comment.Text)
				}
				return true
			})
			if strings.Join(got, "|") != strings.Join(test.want, "|") {
				t.Fatalf("comments = %v, want %v", got, test.want)
			}
		})
	}
}

// Native Zsh rejects each fixture (`zsh -f -n`), so the front end must too,
// with the parser's own error family at the original position.
func TestParseDoLeadingSeparatorRejectsInvalidSources(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		line    uint
		col     uint
	}{
		{"invalid-238-double-semicolon.txt", "`;;` can only be used in a case clause", 1, 15},
		{"invalid-238-ampersand-after-separator.txt", "`&` can only immediately follow a statement", 1, 17},
		{"invalid-238-do-outside-loop.txt", "`do` can only be used in a loop", 1, 1},
		{"invalid-238-unterminated-body.txt", "`while` statement must end with `done`", 1, 1},
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

// A `;` that opens an `if` branch fails the same way in the parser but is a
// different construct; the adapter finds no `do` site and hands the error on
// without a retry.
func TestParseDoLeadingSeparatorLeavesOtherErrorsUntouched(t *testing.T) {
	for _, src := range []string{"if true; then; print x; fi\n", "print x }\n"} {
		src := []byte(src)
		_, firstErr := parseTree(src, "other.zsh")
		if firstErr == nil {
			t.Fatalf("parseTree(%q) unexpectedly succeeded", src)
		}
		calls := 0
		_, err := parseDoLeadingSeparatorWithParser(src, "other.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
			calls++
			return nil, nil
		})
		if err != firstErr {
			t.Fatalf("error = %v, want the incoming error %v", err, firstErr)
		}
		if calls != 0 {
			t.Fatalf("parser called %d times, want 0", calls)
		}
	}
}

// A retry whose tree does not hold the loop at the masked site is not
// trusted: the parser error is returned instead.
func TestParseDoLeadingSeparatorFailsClosedWithoutLoop(t *testing.T) {
	src := []byte("while true; do; break; done\n")
	_, firstErr := parseTree(src, "closed.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the leading separator")
	}
	_, err := parseDoLeadingSeparatorWithParser(src, "closed.zsh", firstErr, func(masked []byte, _ string) (*syntax.File, error) {
		if string(masked) != "while true; do  break; done\n" {
			t.Fatalf("masked source = %q", masked)
		}
		return &syntax.File{}, nil
	})
	if err != firstErr {
		t.Fatalf("error = %v, want the incoming error %v", err, firstErr)
	}
}
