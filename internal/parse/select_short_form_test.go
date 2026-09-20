package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// selectLoops returns every select loop in tree in source order.
func selectLoops(tree *syntax.File) []*syntax.ForClause {
	var loops []*syntax.ForClause
	syntax.Walk(tree, func(node syntax.Node) bool {
		if loop, ok := node.(*syntax.ForClause); ok && loop.Select {
			loops = append(loops, loop)
		}
		return true
	})
	return loops
}

// Issue #212: `select name [in word ...] term sublist`. The parser requires
// `do`, so the front end inserts source-mapped `do` and `done` around the
// sublist native Zsh runs and keeps every other node on its original bytes.
// Each row is `zsh -f -n` valid; the body column is what native Zsh runs,
// checked with `functions f` on a function holding the row.
func TestParseSelectShortForm(t *testing.T) {
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
		{"semicolon term", "select o in a b c; break\n", "select o in a b c; do break; done\n", "1:1", "1:20", "1:25", 1, false},
		{"newline term", "select o in a b c\nbreak\n", "select o in a b c; do break; done\n", "1:1", "2:1", "2:6", 1, false},
		{"semicolon then newline", "select o in a b c;\nbreak\n", "select o in a b c; do break; done\n", "1:1", "2:1", "2:6", 1, false},
		{"blank line and comment before body", "select o in a b c\n\n  # note\n  break\n", "select o in a b c; do break # note\ndone\n", "1:1", "4:3", "4:8", 1, false},
		{"two semicolons", "select o in a b c; ; break\n", "select o in a b c; do break; done\n", "1:1", "1:22", "1:27", 1, false},
		{"continuation before body", "select o in a b c; \\\n  break\n", "select o in a b c; do break; done\n", "1:1", "2:3", "2:8", 1, false},
		{"continuation inside list", "select o in a b \\\n c; break\n", "select o in a b \\\n\tc; do break; done\n", "1:1", "2:5", "2:10", 1, false},
		{"empty list", "select o in; break\n", "select o in; do break; done\n", "1:1", "1:14", "1:19", 1, false},
		{"quoted and expanded words", "select o in \"a b\" $(print c) 'd'; break\n", "select o in \"a b\" $(print c) 'd'; do break; done\n", "1:1", "1:35", "1:40", 1, false},
		{"positional parameters after semicolon", "select o; break\n", "select o; do break; done\n", "1:1", "1:11", "1:16", 1, false},
		{"positional parameters after newline", "select o\nbreak\n", "select o; do break; done\n", "1:1", "2:1", "2:6", 1, false},
		{"positional parameters without separator", "select o break\n", "select o; do break; done\n", "1:1", "1:10", "1:15", 1, false},
		{"and chain", "select o in a b c; print $o && break; print after\n", "select o in a b c; do print $o && break; done\nprint after\n", "1:1", "1:20", "1:37", 1, false},
		{"pipeline", "select o in a b c; print $o | cat\n", "select o in a b c; do print $o | cat; done\n", "1:1", "1:20", "1:34", 1, false},
		{"background binds to the loop", "select o in a b c; print $o &\n", "select o in a b c; do print $o; done &\n", "1:1", "1:20", "1:28", 1, true},
		{"body after and", "true && select o in a b; break && print x\n", "true && select o in a b; do break && print x; done\n", "1:9", "1:26", "1:42", 1, false},
		{"body after time", "time select o in a b; break\n", "time select o in a b; do break; done\n", "1:6", "1:23", "1:28", 1, false},
		{"compound body", "select o in a b c; if [[ $o == a ]]; then break; fi\n", "select o in a b c; do if [[ $o == a ]]; then break; fi; done\n", "1:1", "1:20", "1:52", 1, false},
		{"case body", "select o in a b c; case $o in a) break ;; esac\n", "select o in a b c; do case $o in a) break ;; esac done\n", "1:1", "1:20", "1:47", 1, false},
		{"subshell body", "select o in a b c; (print $o)\n", "select o in a b c; do (print $o); done\n", "1:1", "1:20", "1:30", 1, false},
		{"in function", "f() { select o in a b c; break }\n", "f() { select o in a b c; do break; done; }\n", "1:7", "1:26", "1:31", 1, false},
		{"in command substitution", "x=$(select o in a b; break)\n", "x=$(select o in a b; do break; done)\n", "1:5", "1:22", "1:27", 1, false},
		{"in case arm", "case x in (x) select o in a b; break ;; esac\n", "case x in x) select o in a b; do break; done ;; esac\n", "1:15", "1:32", "1:37", 1, false},
		{"in if body", "if true; then select o in a b; break; fi\n", "if true; then select o in a b; do break; done; fi\n", "1:15", "1:32", "1:37", 1, false},
		{"trailing comment stays with the body", "select o in a b c; break # tail\nprint after\n", "select o in a b c; do break # tail\ndone\nprint after\n", "1:1", "1:20", "1:32", 1, false},
		// The body ends in a closing keyword another adapter synthesized; the
		// end is scanned back from the source, so `done` follows the real `}`.
		{"body ending in brace if", "select o in a b c; if (( 1 )) { break }\nprint after\n", "select o in a b c; do if ((1)); then break; fi; done\nprint after\n", "1:1", "1:20", "1:40", 1, false},
		{"body ending in short if", "select o in a b c; if (( 1 )) break\nprint after\n", "select o in a b c; do if ((1)); then break; fi; done\nprint after\n", "1:1", "1:20", "1:36", 1, false},
		{"body ending in alternate for", "select o in a b c; for i (a b) { print $i }\nprint after\n", "select o in a b c; do for i in a b; do print $i; done; done\nprint after\n", "1:1", "1:20", "1:44", 1, false},
		{"heredoc body", "select o in a b c; cat <<EOT\n$o\nEOT\nprint after\n", "select o in a b c; do cat <<EOT\n$o\nEOT\ndone\nprint after\n", "1:1", "1:20", "3:4", 1, false},
		{"body needing another adapter", "select o in a b c; print ${x::=y}\n", "select o in a b c; do print ${x:=y}; done\n", "1:1", "1:20", "1:34", 1, false},
		{"reporter idiom", "select o in a b c; break\ncase $o in\n  (a) print a ;;\nesac\n", "select o in a b c; do break; done\ncase $o in\na) print a ;;\nesac\n", "1:1", "1:20", "1:25", 1, false},
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
			loops := selectLoops(file.AST())
			if len(loops) != 1 {
				t.Fatalf("select loops = %d, want 1", len(loops))
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
func TestParseSelectShortFormPositions(t *testing.T) {
	src := "select o in \"a b\" $(print c) 'd'\n  print -r -- \"$o\" && break\n"
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	loops := selectLoops(file.AST())
	if len(loops) != 1 {
		t.Fatalf("select loops = %d, want 1", len(loops))
	}
	iter, ok := loops[0].Loop.(*syntax.WordIter)
	if !ok {
		t.Fatalf("Loop = %T, want *syntax.WordIter", loops[0].Loop)
	}
	if iter.Name.Value != "o" || iter.Name.Pos().String() != "1:8" {
		t.Errorf("Name = %q at %s, want o at 1:8", iter.Name.Value, iter.Name.Pos())
	}
	if got := iter.InPos.String(); got != "1:10" {
		t.Errorf("InPos = %s, want 1:10", got)
	}
	wantItems := []string{"1:13 \"a b\"", "1:19 $(print c)", "1:30 'd'"}
	var items []string
	for _, item := range iter.Items {
		items = append(items, item.Pos().String()+" "+src[item.Pos().Offset():item.End().Offset()])
	}
	if strings.Join(items, "|") != strings.Join(wantItems, "|") {
		t.Errorf("items = %v, want %v", items, wantItems)
	}
	body := loops[0].Do[0]
	if got := src[body.Pos().Offset():body.End().Offset()]; got != "print -r -- \"$o\" && break" {
		t.Errorf("body text = %q", got)
	}
	if got := loops[0].DonePos.Offset(); int(got) != len(src)-1 {
		t.Errorf("DonePos offset = %d, want %d (the final newline)", got, len(src)-1)
	}
}

// Two sites in one file, and a site whose body is another site: the later
// site is rewritten first, so the enclosing loop's body is the whole inner
// loop and both close on the same byte.
func TestParseSelectShortFormSites(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
		pos  []string
	}{
		{
			"two sites",
			"select o in a b c; break\nselect p in x y; break\n",
			"select o in a b c; do break; done\nselect p in x y; do break; done\n",
			[]string{"1:1 1:20 1:25", "2:1 2:18 2:23"},
		},
		{
			"nested sites",
			"select o in a b c; select p in x y; break\nprint after\n",
			"select o in a b c; do select p in x y; do break; done; done\nprint after\n",
			[]string{"1:1 1:20 1:42", "1:20 1:37 1:42"},
		},
		{
			"short form after do form",
			"select o in a b; do break; done\nselect p in x y; break\n",
			"select o in a b; do break; done\nselect p in x y; do break; done\n",
			[]string{"1:1 1:18 1:28", "2:1 2:18 2:23"},
		},
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
			var pos []string
			for _, loop := range selectLoops(file.AST()) {
				pos = append(pos, loop.ForPos.String()+" "+loop.DoPos.String()+" "+loop.DonePos.String())
			}
			if strings.Join(pos, "|") != strings.Join(test.pos, "|") {
				t.Errorf("positions = %v, want %v", pos, test.pos)
			}
		})
	}
}

// An anonymous function invocation as the body parses through the retry
// that runs outside the chain: the probe reports the invocation as the next
// blocker, that retry masks its words and re-enters the adapter, and the
// words stay metadata on their original bytes.
func TestParseSelectShortFormAnonymousInvocationBody(t *testing.T) {
	src := "select o in a b c; () { print $1 } $o\n"
	file, err := Parse(strings.NewReader(src), "anonymous.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got, want := renderTree(t, file.AST()), "select o in a b c; do () { print $1; }; done\n"; got != want {
		t.Fatalf("tree = %q, want %q", got, want)
	}
	invocations := file.AnonymousInvocations()
	if len(invocations) != 1 || len(invocations[0].Words) != 1 {
		t.Fatalf("AnonymousInvocations() = %+v, want one invocation with one word", invocations)
	}
	if got := invocations[0].Words[0].Pos().String(); got != "1:36" {
		t.Errorf("invocation word at %s, want 1:36", got)
	}
	if got := selectLoops(file.AST())[0].DonePos.String(); got != "1:38" {
		t.Errorf("DonePos = %s, want 1:38 (after the invocation words)", got)
	}
}

// Comments in the header gap and after the body keep their nodes and
// positions, so the suppression pass reads them where they are.
func TestParseSelectShortFormKeepsComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"after list", "select o in a b # c\nbreak\n", []string{"1:17  c"}},
		{"after semicolon", "select o in a b; # c\nbreak\n", []string{"1:18  c"}},
		{"after body", "select o in a b; break # c\nprint x\n", []string{"1:24  c"}},
		{"inside list", "select o in a $(print b # c\n) d; break\n", []string{"1:25  c"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if len(selectLoops(file.AST())) != 1 {
				t.Fatalf("select loops = %d, want 1", len(selectLoops(file.AST())))
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

// Shapes native Zsh accepts as other productions of the loop, or that the
// adapter cannot place, keep the parser's own error at the `select` word.
// The empty body before a closer or at the end of input parses since #302
// (TestParseSelectEmptyBody) and the brace body since #301
// (TestParseSelectBraceBody).
func TestParseSelectShortFormDeclines(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantPos string
	}{
		{"parenthesized list", "select o (a b c) break\n", "1:1"},
		{"negated loop", "! select o in a b; break\n", "1:3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			var perr syntax.ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse() error = %v, want syntax.ParseError", err)
			}
			if got := perr.Pos.String(); got != test.wantPos {
				t.Errorf("position = %s, want %s", got, test.wantPos)
			}
			if !strings.HasPrefix(perr.Text, "`select foo") {
				t.Errorf("text = %q, want the parser's select error", perr.Text)
			}
		})
	}
}

// Each fixture is rejected by `zsh -f -n`. A header the adapter does not
// read keeps the parser's error at the `select` word; a body the probe
// rejects reports that blocker at its own position.
func TestParseSelectShortFormRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		fixture  string
		wantPos  string
		wantText string
	}{
		{"invalid-212-double-semicolon-after-list.txt", "1:1", "`select foo [in words]` must be followed by `do`"},
		{"invalid-212-list-ended-by-ampersand.txt", "1:1", "`select foo [in words]` must be followed by `do`"},
		{"invalid-212-list-without-term.txt", "1:1", "`select foo [in words]` must be followed by `do`"},
		{"invalid-212-then-body.txt", "1:20", "`then` can only be used in an `if`"},
		{"invalid-212-brace-close-body.txt", "1:20", "`}` can only be used to close a block"},
		{"invalid-212-brace-close-in-body.txt", "1:26", "`}` can only be used to close a block"},
		{"invalid-302-double-semicolon-after-header.txt", "1:1", "`select foo [in words]` must be followed by `do`"},
		{"invalid-302-double-semicolon-after-separator.txt", "1:20", "`;;` can only be used in a case clause"},
		{"invalid-302-ampersand-opening-body.txt", "1:20", "`&` can only immediately follow a statement"},
		{"invalid-302-unterminated-function.txt", "1:7", "`select foo [in words]` must be followed by `do`"},
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

// The adapter only acts on the parser's select errors at a `select` word.
func TestParseSelectShortFormAdapterDeclinesUnrelatedErrors(t *testing.T) {
	src := []byte("select o in a b c; break\n")
	other := syntax.ParseError{Filename: "x.zsh", Pos: syntax.NewPos(0, 1, 1), Text: "`for foo [in words]` must be followed by `do`"}
	if _, err := parseSelectShortFormWithParser(src, "x.zsh", other, parseWithAdapters); !errors.Is(err, other) {
		t.Errorf("unrelated text: error = %v, want the incoming error", err)
	}
	elsewhere := syntax.ParseError{Filename: "x.zsh", Pos: syntax.NewPos(7, 1, 8), Text: "`select foo [in words]` must be followed by `do`"}
	if _, err := parseSelectShortFormWithParser(src, "x.zsh", elsewhere, parseWithAdapters); !errors.Is(err, elsewhere) {
		t.Errorf("position off the word: error = %v, want the incoming error", err)
	}
}

// scanSelectSites finds headers in command position only and reads the
// list with the word lexer.
func TestScanSelectSites(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []int
	}{
		{"top level", "select o in a b; break\n", []int{0}},
		{"do form is not a site", "select o in a b; do break; done\n", nil},
		{"after separator and in function", "x; select o in a b; break\nf() { select p; break }\n", []int{3, 32}},
		{"argument position", "print select o in a b\n", nil},
		{"quoted", "print 'select o in a; b'\n", nil},
		{"comment", "# select o in a; b\n", nil},
		{"list word", "select o in select; break\n", []int{0}},
		{"parenthesized list", "select o (a b) break\n", nil},
		{"list ended by pipe", "select o in a | cat\n", nil},
		{"nested", "select o in a; select p in b; break\n", []int{0, 15}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got []int
			for _, site := range scanSelectSites([]byte(test.src)) {
				got = append(got, site.start)
			}
			if len(got) != len(test.want) {
				t.Fatalf("sites = %v, want %v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("sites = %v, want %v", got, test.want)
				}
			}
		})
	}
}
