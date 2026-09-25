package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// forLoops returns every for loop in tree in source order.
func forLoops(tree *syntax.File) []*syntax.ForClause {
	var loops []*syntax.ForClause
	syntax.Walk(tree, func(node syntax.Node) bool {
		if loop, ok := node.(*syntax.ForClause); ok && !loop.Select {
			loops = append(loops, loop)
		}
		return true
	})
	return loops
}

// Issue #211: `for name [in word ...] term sublist` and `for name ( word ... ) sublist`.
// The parser requires `do`, so the front end inserts source-mapped `do` and `done` around
// the sublist native Zsh runs and keeps every other node on its original bytes.
// Each row is `zsh -f -n` valid; the body column is what native Zsh runs.
func TestParseForShortForm(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		want       string
		forPos     string
		doPos      string
		donePos    string
		body       int
		background bool
	}{
		{"semicolon term", "for t in a b c; consume_task $t\n", "for t in a b c; do consume_task $t; done\n", "1:1", "1:17", "1:32", 1, false},
		{"newline term", "for t in a b c\nconsume_task $t\n", "for t in a b c; do consume_task $t; done\n", "1:1", "2:1", "2:16", 1, false},
		{"semicolon then newline", "for t in a b c;\nconsume_task $t\n", "for t in a b c; do consume_task $t; done\n", "1:1", "2:1", "2:16", 1, false},
		{"paren list", "for i (a b) print $i\n", "for i in a b; do print $i; done\n", "1:1", "1:13", "1:21", 1, false},
		{"paren list with semicolon", "for i (a b); print $i\n", "for i in a b; do print $i; done\n", "1:1", "1:14", "1:22", 1, false},
		{"paren list with newline", "for i (a b)\nprint $i\n", "for i in a b; do print $i; done\n", "1:1", "2:1", "2:9", 1, false},
		{"positional with semicolon", "for i; print $i\n", "for i; do print $i; done\n", "1:1", "1:8", "1:16", 1, false},
		{"positional with newline", "for i\nprint $i\n", "for i; do print $i; done\n", "1:1", "2:1", "2:9", 1, false},
		{"and chain", "for i in a b; print $i && break\n", "for i in a b; do print $i && break; done\n", "1:1", "1:15", "1:32", 1, false},
		{"pipeline", "for i in a b; print $i | cat\n", "for i in a b; do print $i | cat; done\n", "1:1", "1:15", "1:29", 1, false},
		{"background binds to the loop", "for i in a b; print $i &\n", "for i in a b; do print $i; done &\n", "1:1", "1:15", "1:23", 1, true},
		{"in function", "f() { for i in a b; print $i }\n", "f() { for i in a b; do print $i; done; }\n", "1:7", "1:21", "1:29", 1, false},
		{"in if body", "if true; then for i in a b; print $i; fi\n", "if true; then for i in a b; do print $i; done; fi\n", "1:15", "1:29", "1:37", 1, false},
		{"in case body", "case x in (x) for i in a b; print $i ;; esac\n", "case x in x) for i in a b; do print $i; done ;; esac\n", "1:15", "1:29", "1:37", 1, false},
		{"trailing comment stays with the body", "for i in a b; print $i # note\nprint after\n", "for i in a b; do print $i # note\ndone\nprint after\n", "1:1", "1:15", "1:23", 1, false},
		{"heredoc body", "for i in a b; cat <<EOT\n$i\nEOT\nprint after\n", "for i in a b; do cat <<EOT\n$i\nEOT\ndone\nprint after\n", "1:1", "1:15", "3:4", 1, false},
		{"nested sites", "for i in a b; for j in c d; print $i$j\n", "for i in a b; do for j in c d; do print $i$j; done; done\n", "1:1", "1:15", "1:39", 1, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if got := renderTree(t, file.AST()); got != test.want {
				t.Fatalf("tree =\n%s\nwant\n%s", got, test.want)
			}
			loops := forLoops(file.AST())
			if len(loops) == 0 {
				t.Fatalf("for loops = 0, want at least 1")
			}
			loop := loops[0]
			if got := loop.ForPos.String(); got != test.forPos {
				t.Errorf("ForPos = %s, want %s", got, test.forPos)
			}
			if got := loop.DoPos.String(); got != test.doPos {
				t.Errorf("DoPos = %s, want %s", got, test.doPos)
			}
			if got := loop.DonePos.String(); got != test.donePos {
				t.Errorf("DonePos = %s, want %s", got, test.donePos)
			}
			if len(loop.Do) != test.body {
				t.Errorf("len(Do) = %d, want %d", len(loop.Do), test.body)
			}
			if len(loop.Do) > 0 && loop.Do[0].Pos() != loop.DoPos {
				t.Errorf("Do[0].Pos() = %s, want the do position %s", loop.Do[0].Pos(), loop.DoPos)
			}
			var background bool
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if stmt, ok := node.(*syntax.Stmt); ok && stmt.Cmd == loop {
					background = stmt.Background
				}
				return true
			})
			if background != test.background {
				t.Errorf("Background = %v, want %v", background, test.background)
			}
			if got := strings.Join(file.Lines(), "\n") + "\n"; got != test.src {
				t.Errorf("Lines() = %q, want the source", got)
			}
		})
	}
}

// The list words and the body keep their original bytes and positions.
func TestParseForShortFormPositions(t *testing.T) {
	src := "for i in \"a b\" $(print c) 'd'\n  print -r -- \"$i\" && break\n"
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	loops := forLoops(file.AST())
	if len(loops) != 1 {
		t.Fatalf("for loops = %d, want 1", len(loops))
	}
	iter, ok := loops[0].Loop.(*syntax.WordIter)
	if !ok {
		t.Fatalf("Loop = %T, want *syntax.WordIter", loops[0].Loop)
	}
	if iter.Name.Value != "i" || iter.Name.Pos().String() != "1:5" {
		t.Errorf("Name = %q at %s, want i at 1:5", iter.Name.Value, iter.Name.Pos())
	}
	if got := iter.InPos.String(); got != "1:7" {
		t.Errorf("InPos = %s, want 1:7", got)
	}
	wantItems := []string{"1:10 \"a b\"", "1:16 $(print c)", "1:27 'd'"}
	var items []string
	for _, item := range iter.Items {
		items = append(items, item.Pos().String()+" "+src[item.Pos().Offset():item.End().Offset()])
	}
	if strings.Join(items, "|") != strings.Join(wantItems, "|") {
		t.Errorf("items = %v, want %v", items, wantItems)
	}
	body := loops[0].Do[0]
	if got := src[body.Pos().Offset():body.End().Offset()]; got != "print -r -- \"$i\" && break" {
		t.Errorf("body text = %q", got)
	}
	if got := loops[0].DonePos.Offset(); int(got) != len(src)-1 {
		t.Errorf("DonePos offset = %d, want %d (the final newline)", got, len(src)-1)
	}
}

// Comments in the header gap and after the body keep their nodes and
// positions, so the suppression pass reads them where they are.
func TestParseForShortFormKeepsComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"after list", "for i in a b # c\nbreak\n", []string{"1:14  c"}},
		{"after semicolon", "for i in a b; # c\nbreak\n", []string{"1:15  c"}},
		{"after body", "for i in a b; break # c\nprint x\n", []string{"1:21  c"}},
		{"inside list", "for i in a $(print b # c\n) d; break\n", []string{"1:22  c"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if len(forLoops(file.AST())) != 1 {
				t.Fatalf("for loops = %d, want 1", len(forLoops(file.AST())))
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

// Each fixture is rejected by `zsh -f -n`. A header the adapter does not
// read keeps the parser's error at the `for` word.
func TestParseForShortFormRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		fixture  string
		wantPos  string
		wantText string
	}{
		{"invalid-211-for-without-term.txt", "2:13", "for loop names must be literal names"},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + test.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), test.fixture)
			var perr syntax.ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse() error = %v, want syntax.ParseError", err)
			}
			if got := perr.Pos.String(); got != test.wantPos {
				t.Errorf("position = %s, want %s", got, test.wantPos)
			}
			if perr.Text != test.wantText {
				t.Errorf("text = %q, want %q", perr.Text, test.wantText)
			}
		})
	}
}
