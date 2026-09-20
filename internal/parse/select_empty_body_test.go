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
		{"end of file", "select o in a b c;\n", []string{"1:1:1:20:1:20/0"}, nil},
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

// The empty body is read only before a closer or at the end of the file:
// a byte the probe could not turn into a statement anywhere else is not
// a body the adapter understands, and the parser error stays.
func TestSelectEmptyBodyAt(t *testing.T) {
	src := []byte("x } ) done fi esac elif else ;; & done2 fis then do")
	for _, test := range []struct {
		at   int
		want bool
	}{
		{0, false}, {2, true}, {4, true}, {6, true}, {11, true}, {14, true}, {19, true}, {24, true},
		{29, false}, {32, false}, {34, false}, {40, false}, {44, true}, {49, false}, {len(src), true},
	} {
		if got := selectEmptyBodyAt(src, test.at); got != test.want {
			t.Errorf("selectEmptyBodyAt(%q) = %v, want %v", src[test.at:], got, test.want)
		}
	}
}

// A retry whose tree lacks the empty loop at the site is not trusted.
func TestParseSelectEmptyBodyFailsClosed(t *testing.T) {
	src := []byte("f() { select o in a b c; }\n")
	_, firstErr := parseTree(src, "closed.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the empty body")
	}
	calls := 0
	_, err := parseSelectShortFormWithParser(src, "closed.zsh", firstErr, func(masked []byte, name string) (*syntax.File, error) {
		calls++
		if calls == 1 {
			return parseTree(masked, name) // the probe
		}
		if string(masked) != "f() { select o in a b c; do\ndone\n}\n" {
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
