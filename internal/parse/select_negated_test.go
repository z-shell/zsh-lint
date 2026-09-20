package parse

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// negatedStmts lists every negated statement in tree as `pos type`, with
// pos on the `!` byte and type the statement's command node.
func negatedStmts(tree *syntax.File) []string {
	var found []string
	syntax.Walk(tree, func(node syntax.Node) bool {
		if stmt, ok := node.(*syntax.Stmt); ok && stmt.Negated {
			found = append(found, fmt.Sprintf("%s %T", stmt.Pos(), stmt.Cmd))
		}
		return true
	})
	return found
}

// wordLoops describes every for and select loop in tree as
// `for:do:done/count`.
func wordLoops(tree *syntax.File) []string {
	var found []string
	syntax.Walk(tree, func(node syntax.Node) bool {
		if loop, ok := node.(*syntax.ForClause); ok {
			found = append(found, fmt.Sprintf("%s:%s:%s/%d", loop.ForPos, loop.DoPos, loop.DonePos, len(loop.Do)))
		}
		return true
	})
	return found
}

// Issue #321: `!` may precede any pipeline, so a negated select in its
// sublist (#212), brace (#301), parenthesized-list (#303), empty-body
// (#302, #319) and do forms, and a negated for in its brace form, are
// valid. The adapters' command-position probes skip the `!` and the loop
// keeps the positions of its un-negated form; the statement holding the
// loop is negated and starts on the `!`. A `!` before a pipeline negates
// the pipeline, as in native Zsh, so the parser hangs Negated on the
// pipeline statement rather than on the loop's own statement.
func TestParseSelectNegated(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		negated []string
		loops   []string
	}{
		{"sublist", "! select o in a b; break\n",
			[]string{"1:1 *syntax.ForClause"}, []string{"1:3:1:20:1:25/1"}},
		{"tab after the bang", "!\tselect o in a b; break\n",
			[]string{"1:1 *syntax.ForClause"}, []string{"1:3:1:20:1:25/1"}},
		{"brace body", "! select o in a b; { print $o; break }\n",
			[]string{"1:1 *syntax.ForClause"}, []string{"1:3:1:20:1:38/2"}},
		{"paren list sublist", "! select o (a b) break\n",
			[]string{"1:1 *syntax.ForClause"}, []string{"1:3:1:18:1:23/1"}},
		{"paren list brace body", "! select o (a b) { break }\n",
			[]string{"1:1 *syntax.ForClause"}, []string{"1:3:1:18:1:26/1"}},
		{"paren list separator then sublist", "! select o (a b); break\n",
			[]string{"1:1 *syntax.ForClause"}, []string{"1:3:1:19:1:24/1"}},
		{"paren list do body", "! select o (a b) do break; done\n",
			[]string{"1:1 *syntax.ForClause"}, []string{"1:3:1:18:1:28/1"}},
		{"empty body before brace", "f() { ! select o in a b; }\n",
			[]string{"1:7 *syntax.ForClause"}, []string{"1:9:1:26:1:26/0"}},
		{"empty body before pipe negates the pipeline", "! select o in a b; | cat\n",
			[]string{"1:1 *syntax.BinaryCmd"}, []string{"1:3:1:20:1:20/0"}},
		{"do body", "! select o in a b; do break; done\n",
			[]string{"1:1 *syntax.ForClause"}, []string{"1:3:1:20:1:30/1"}},
		{"for brace body", "! for x (a b) { print $x }\n",
			[]string{"1:1 *syntax.ForClause"}, []string{"1:3:1:15:1:26/1"}},
		{"after a list operator", "true && ! select o in a b; break\n",
			[]string{"1:9 *syntax.ForClause"}, []string{"1:11:1:28:1:33/1"}},
		{"as an if condition", "if ! select o in a b; break; then :; fi\n",
			[]string{"1:4 *syntax.ForClause"}, []string{"1:6:1:23:1:28/1"}},
		{"second site on the line", "select o in a b; break; ! select p in c d; break\n",
			[]string{"1:25 *syntax.ForClause"}, []string{"1:1:1:18:1:23/1", "1:27:1:44:1:49/1"}},
		{"negated brace body is the sublist", "! select o in a b; ! { break }\n",
			[]string{"1:1 *syntax.ForClause", "1:20 *syntax.Block"}, []string{"1:3:1:20:1:31/1"}},
		{"tail binds to the loop", "! select o in a b; break && print x\n",
			[]string{"1:1 *syntax.ForClause"}, []string{"1:3:1:20:1:36/1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			tree := file.AST()
			assertLiteralsMatchSource(t, tree, test.src)
			if got := negatedStmts(tree); strings.Join(got, "|") != strings.Join(test.negated, "|") {
				t.Errorf("negated statements = %v, want %v", got, test.negated)
			}
			if got := wordLoops(tree); strings.Join(got, "|") != strings.Join(test.loops, "|") {
				t.Errorf("loops = %v, want %v", got, test.loops)
			}
			syntax.Walk(tree, func(node syntax.Node) bool {
				if stmt, ok := node.(*syntax.Stmt); ok && stmt.Negated {
					if got := test.src[stmt.Pos().Offset()]; got != '!' {
						t.Errorf("negated statement at %s reads %q, want '!'", stmt.Pos(), got)
					}
				}
				return true
			})
		})
	}
}

// A `!` negates at most once: the second `!` is a word the loop keyword
// then cannot follow, and the parser reports the double negation.
func TestParseSelectNegatedDeclines(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		line    uint
		col     uint
	}{
		{"invalid-321-double-negation.txt", "cannot negate a command multiple times", 1, 1},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + test.fixture)
			if err != nil {
				t.Fatal(err)
			}
			assertParseErrorAt(t, src, test.text, test.line, test.col)
		})
	}
}
