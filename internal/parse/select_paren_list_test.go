package parse

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// parenSelectLoops describes every select loop in tree as
// `select:in:do:done/count[items]`, with the `in` position on the `(` byte
// and each item as its position and source text.
func parenSelectLoops(tree *syntax.File, src string) []string {
	var found []string
	for _, loop := range selectLoops(tree) {
		iter, _ := loop.Loop.(*syntax.WordIter)
		var items []string
		for _, item := range iter.Items {
			items = append(items, item.Pos().String()+" "+src[item.Pos().Offset():item.End().Offset()])
		}
		found = append(found, fmt.Sprintf("%s:%s:%s:%s/%d[%s]", loop.ForPos, iter.InPos, loop.DoPos, loop.DonePos, len(loop.Do), strings.Join(items, ", ")))
	}
	return found
}

// Issue #303: `select name ( word ... )` with every body the loop takes.
// The list is rewritten into the `in` form the parser reads and the body
// then lands on the do form, the short form (#212), the brace body (#301)
// or the empty body (#302); the `in` sits on the `(` and every item and
// body node keeps its original position.
func TestParseSelectParenList(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		loops []string
	}{
		{"sublist", "select o (a b c) break\n", []string{"1:1:1:10:1:18:1:23/1[1:11 a, 1:13 b, 1:15 c]"}},
		{"separator then sublist", "select o (a b c); break\n", []string{"1:1:1:10:1:19:1:24/1[1:11 a, 1:13 b, 1:15 c]"}},
		{"brace body", "select o (a b c) { print $o; break }\n", []string{"1:1:1:10:1:18:1:36/2[1:11 a, 1:13 b, 1:15 c]"}},
		{"do body", "select o (a b c) do break; done\n", []string{"1:1:1:10:1:18:1:28/1[1:11 a, 1:13 b, 1:15 c]"}},
		{"separator then do body", "select o (a b c); do break; done\n", []string{"1:1:1:10:1:19:1:29/1[1:11 a, 1:13 b, 1:15 c]"}},
		{"empty body at end of file", "select o (a b c)\n", []string{"1:1:1:10:2:1:2:1/0[1:11 a, 1:13 b, 1:15 c]"}},
		{"empty body before brace", "f() { select o (a b c) }\n", []string{"1:7:1:16:1:24:1:24/0[1:17 a, 1:19 b, 1:21 c]"}},
		{"blanks inside parens", "select o ( a b c ) break\n", []string{"1:1:1:10:1:20:1:25/1[1:12 a, 1:14 b, 1:16 c]"}},
		{"multi-line list", "select o (a\n  b\n  c) break\n", []string{"1:1:1:10:3:6:3:11/1[1:11 a, 2:3 b, 3:3 c]"}},
		{"quoted and expanded words", "select o (\"a b\" $(print c) 'd' ${x:-e}) break\n", []string{"1:1:1:10:1:41:1:46/1[1:11 \"a b\", 1:17 $(print c), 1:28 'd', 1:32 ${x:-e}]"}},
		{"quoted paren in the list", "select o (')' \"x\") break\n", []string{"1:1:1:10:1:20:1:25/1[1:11 ')', 1:15 \"x\"]"}},
		// The lexer's extent of the last word takes in the `\`-newline.
		{"continuation before the closer", "select o (a b c\\\n) break\n", []string{"1:1:1:10:2:3:2:8/1[1:11 a, 1:13 b, 1:15 c\\\n]"}},
		{"empty list", "select o () break\n", []string{"1:1:1:10:1:13:1:18/1[]"}},
		{"two sites", "select o1 (a b) break; select o2 (c d) break\n",
			[]string{"1:1:1:11:1:17:1:22/1[1:12 a, 1:14 b]", "1:24:1:34:1:40:1:45/1[1:35 c, 1:37 d]"}},
		{"site whose body is a site", "select o (a b) select p (c d) break\n",
			[]string{"1:1:1:10:1:16:1:36/1[1:11 a, 1:13 b]", "1:16:1:25:1:31:1:36/1[1:26 c, 1:28 d]"}},
		{"tail binds to the loop", "select o (a b c) { print $o } && print x\n", []string{"1:1:1:10:1:18:1:29/1[1:11 a, 1:13 b, 1:15 c]"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			tree := file.AST()
			assertLiteralsMatchSource(t, tree, test.src)
			if got := parenSelectLoops(tree, test.src); strings.Join(got, "|") != strings.Join(test.loops, "|") {
				t.Fatalf("loops =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(test.loops, "\n  "))
			}
			for _, loop := range selectLoops(tree) {
				iter := loop.Loop.(*syntax.WordIter)
				if got := test.src[iter.InPos.Offset()]; got != '(' {
					t.Errorf("InPos %s reads %q, want '('", iter.InPos, got)
				}
			}
		})
	}
}

// Native Zsh rejects each fixture (`zsh -f -n`); the parser error is kept
// or the retry's own blocker is reported on its byte.
func TestParseSelectParenListRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		line    uint
		col     uint
	}{
		{"invalid-303-unterminated-list.txt", "reached EOF without matching `(` with `)`", 1, 10},
		{"invalid-303-extra-closer.txt", "statements must be separated by &, ; or a newline", 1, 17},
		// The rewritten header `select o in a b c ;;` is itself the parser's
		// header error at the `select` word, so the site keeps its own.
		{"invalid-303-case-terminator-after-list.txt", "`;;` can only be used in a case clause", 1, 17},
		{"invalid-303-ampersand-opening-body.txt", "select loop body must be a command", 1, 18},
		{"invalid-303-missing-name.txt", "`select` must be followed by a literal", 1, 1},
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
