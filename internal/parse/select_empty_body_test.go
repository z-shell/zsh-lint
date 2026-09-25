package parse

import (
	"fmt"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// emptySelectLoops describes every select loop in tree as
// `select:do:done/count`, the positions of its `select`, `do` and `done`
// words and the number of body statements.
func emptySelectLoops(tree *syntax.File) []string {
	var found []string
	for _, loop := range selectLoops(tree) {
		found = append(found, fmt.Sprintf("%s:%s:%s/%d", loop.ForPos, loop.DoPos, loop.DonePos, len(loop.Do)))
	}
	return found
}

// Issue #302: a select header followed by an empty body. Native par_for
// reads one sublist after the header and par_sublist accepts an empty one,
// so a header at the end of the file or directly before a closer is valid.
// The adapter puts `do` and `done` on the body's first byte, so the loop's
// `do` and `done` share that position and the loop holds no statement;
// every other node keeps its original position.
func TestParseSelectEmptyBody(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		loops []string
		rest  []string // positions and text of the statements after the loop
	}{
		{"end of file", "select o in a b c;\n", []string{"1:1:2:1:2:1/0"}, nil},
		{"end of file without newline", "select o in a b c;", []string{"1:1:1:19:1:19/0"}, nil},
		{"before brace", "f() { select o in a b c; }\n", []string{"1:7:1:26:1:26/0"}, nil},
		{"before subshell close", "( select o in a b c; )\n", []string{"1:3:1:22:1:22/0"}, nil},
		{"no list", "g() { select o; }\n", []string{"1:7:1:17:1:17/0"}, nil},
		{"blank and comment lines", "h() {\n  select o in a b c;\n\n  # nothing here\n}\n", []string{"2:3:5:1:5:1/0"}, nil},
		{"newline term", "i() { select o in a b c\n}\n", []string{"1:7:2:1:2:1/0"}, nil},
		// Native Zsh reads the second header as the first loop's one sublist
		// (`functions j` shows the nesting), so only the inner body is empty.
		{"two in a row", "j() { select o in a; select p in b; }\n", []string{"1:7:1:22:1:37/1", "1:22:1:37:1:37/0"}, nil},
		{"statement after closer", "k() { select o in a b c; }; print after\n", []string{"1:7:1:26:1:26/0"}, []string{"1:1 k() { select o in a b c; };", "1:29 print after"}},
		{"before done", "while true; do select o in a; done\n", []string{"1:16:1:31:1:31/0"}, nil},
		{"before fi", "if true; then select o in a; fi\n", []string{"1:15:1:30:1:30/0"}, nil},
		{"as if condition", "if select o in a; then :; fi\n", []string{"1:4:1:19:1:19/0"}, nil},
		{"as if condition with newline term", "if select o in a\nthen :; fi\n", []string{"1:4:2:1:2:1/0"}, nil},
		{"quoted closer in list", "l() { select o in '}' \"x\"; }\n", []string{"1:7:1:28:1:28/0"}, nil},
		{"short form still parses", "select o in a b c; break\n", []string{"1:1:1:20:1:25/1"}, nil},
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
			for _, loop := range selectLoops(tree) {
				if got := test.src[loop.ForPos.Offset() : loop.ForPos.Offset()+6]; got != "select" {
					t.Errorf("ForPos %s reads %q, want \"select\"", loop.ForPos, got)
				}
			}
			if test.rest != nil {
				var got []string
				for _, stmt := range tree.Stmts {
					got = append(got, stmt.Pos().String()+" "+test.src[stmt.Pos().Offset():stmt.End().Offset()])
				}
				if strings.Join(got, "|") != strings.Join(test.rest, "|") {
					t.Errorf("statements = %v, want %v", got, test.rest)
				}
			}
		})
	}
}
