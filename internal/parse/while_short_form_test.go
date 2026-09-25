package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// whileLoops returns every while or until loop in tree in source order.
func whileLoops(tree *syntax.File) []*syntax.WhileClause {
	var loops []*syntax.WhileClause
	syntax.Walk(tree, func(node syntax.Node) bool {
		if loop, ok := node.(*syntax.WhileClause); ok {
			loops = append(loops, loop)
		}
		return true
	})
	return loops
}

// Issue #211: `while list sublist` and `until list sublist`. The parser
// requires `do`, so the front end inserts source-mapped `; do` and `done`
// around the sublist native Zsh runs and keeps every other node on its
// original bytes.
func TestParseWhileShortForm(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		want       string
		whilePos   string
		doPos      string
		donePos    string
		until      bool
		body       int
		background bool
	}{
		{"while arithmetic", "while (( i < 3 )) (( i++ ))\n", "while ((i < 3)); do ((i++)); done\n", "1:1", "1:19", "1:28", false, 1, false},
		{"until arithmetic and chain", "until (( success )) retry && success=$REPLY\n", "until ((success)); do retry && success=$REPLY; done\n", "1:1", "1:21", "1:44", true, 1, false},
		{"while test", "while [[ -n $x ]] shift\n", "while [[ -n $x ]]; do shift; done\n", "1:1", "1:19", "1:24", false, 1, false},
		{"while brace command", "while { read -r l } print -r -- $l\n", "while { read -r l; }; do print -r -- $l; done\n", "1:1", "1:21", "1:35", false, 1, false},
		{"while subshell", "while ( read -r l ) print -r -- $l\n", "while (read -r l); do print -r -- $l; done\n", "1:1", "1:21", "1:35", false, 1, false},
		{"while pipeline in sublist", "while (( 1 )) print -r -- $x | cat\n", "while ((1)); do print -r -- $x | cat; done\n", "1:1", "1:15", "1:35", false, 1, false},
		{"while background", "while (( 1 )) print hi &\n", "while ((1)); do print hi; done &\n", "1:1", "1:15", "1:23", false, 1, true},
		{"while in function", "f() { while (( 1 )) print hi }\n", "f() { while ((1)); do print hi; done; }\n", "1:7", "1:21", "1:29", false, 1, false},
		{"while in if body", "if true; then while (( 1 )) print hi; fi\n", "if true; then while ((1)); do print hi; done; fi\n", "1:15", "1:29", "1:37", false, 1, false},
		{"while in case body", "case x in (x) while (( 1 )) print hi ;; esac\n", "case x in x) while ((1)); do print hi; done ;; esac\n", "1:15", "1:29", "1:37", false, 1, false},
		{"while comment on line", "while (( 1 )) print hi # tail\nprint after\n", "while ((1)); do print hi # tail\ndone\nprint after\n", "1:1", "1:15", "1:23", false, 1, false},
		{"while heredoc body", "while (( 1 )) cat <<EOT\nhi\nEOT\nprint after\n", "while ((1)); do cat <<EOT\nhi\nEOT\ndone\nprint after\n", "1:1", "1:15", "3:4", false, 1, false},
		{"nested while sites", "while (( 1 )) while (( 2 )) print hi\n", "while ((1)); do while ((2)); do print hi; done; done\n", "1:1", "1:15", "1:37", false, 1, false},
		{"until with negated condition", "until ! (( 1 )) print hi\n", "until ! ((1)); do print hi; done\n", "1:1", "1:17", "1:25", true, 1, false},
		{"while with chained conditions", "while (( 1 )) && (( 2 )) print hi\n", "while ((1)) && ((2)); do print hi; done\n", "1:1", "1:26", "1:34", false, 1, false},
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
			loops := whileLoops(file.AST())
			if len(loops) == 0 {
				t.Fatalf("while loops = 0, want at least 1")
			}
			loop := loops[0]
			if got := loop.WhilePos.String(); got != test.whilePos {
				t.Errorf("WhilePos = %s, want %s", got, test.whilePos)
			}
			if got := loop.DoPos.String(); got != test.doPos {
				t.Errorf("DoPos = %s, want %s", got, test.doPos)
			}
			if got := loop.DonePos.String(); got != test.donePos {
				t.Errorf("DonePos = %s, want %s", got, test.donePos)
			}
			if loop.Until != test.until {
				t.Errorf("Until = %v, want %v", loop.Until, test.until)
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

// The condition and body keep their original bytes and positions.
func TestParseWhileShortFormPositions(t *testing.T) {
	src := "while (( i < 3 )) print -r -- \"$i\" && break\n"
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	loops := whileLoops(file.AST())
	if len(loops) != 1 {
		t.Fatalf("while loops = %d, want 1", len(loops))
	}
	loop := loops[0]
	if len(loop.Cond) != 1 {
		t.Fatalf("len(Cond) = %d, want 1", len(loop.Cond))
	}
	cond := loop.Cond[0]
	if got := src[cond.Pos().Offset():cond.End().Offset()]; got != "(( i < 3 ))" {
		t.Errorf("cond text = %q", got)
	}
	if len(loop.Do) != 1 {
		t.Fatalf("len(Do) = %d, want 1", len(loop.Do))
	}
	body := loop.Do[0]
	if got := src[body.Pos().Offset():body.End().Offset()]; got != "print -r -- \"$i\" && break" {
		t.Errorf("body text = %q", got)
	}
	if got := loop.DonePos.Offset(); int(got) != len(src)-1 {
		t.Errorf("DonePos offset = %d, want %d (the final newline)", got, len(src)-1)
	}
}

// Comments around the loop keep their nodes and positions.
func TestParseWhileShortFormKeepsComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"after body", "while (( 1 )) break # c\nprint x\n", []string{"1:21  c"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if len(whileLoops(file.AST())) != 1 {
				t.Fatalf("while loops = %d, want 1", len(whileLoops(file.AST())))
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

// Undelimited conditions like `while true print x` are the empty-body form
// (issue #327): the whole line is the condition, `true print x` is one
// command, and the body is the empty sublist native par_while reads at end
// of file. The delimited short form (#211) is unchanged and is asserted
// separately above.
func TestParseWhileShortFormUndelimitedIsEmptyBody(t *testing.T) {
	src := "while true print x\n"
	file, err := Parse(strings.NewReader(src), "undelimited.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	var clause *syntax.WhileClause
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		if found, ok := node.(*syntax.WhileClause); ok && clause == nil {
			clause = found
		}
		return true
	})
	if clause == nil {
		t.Fatal("no WhileClause in the tree")
	}
	if clause.Until {
		t.Error("Until = true, want false")
	}
	if len(clause.Do) != 0 {
		t.Errorf("Do holds %d statements, want 0 (empty body)", len(clause.Do))
	}
	if len(clause.Cond) != 1 {
		t.Fatalf("Cond holds %d statements, want 1", len(clause.Cond))
	}
	call, ok := clause.Cond[0].Cmd.(*syntax.CallExpr)
	if !ok {
		t.Fatalf("condition command = %T, want *syntax.CallExpr", clause.Cond[0].Cmd)
	}
	if len(call.Args) != 3 {
		t.Errorf("condition holds %d words, want 3 (`true print x` is one command)", len(call.Args))
	}
}

// Mixed file with both if short form and while short form in both orders.
func TestWhileShortFormAndIfShortFormMixed(t *testing.T) {
	cases := []string{
		"while (( 1 )) print hi\nif (( 2 )) print there\n",
		"if (( 2 )) print there\nwhile (( 1 )) print hi\n",
		"while (( 1 )) if (( 2 )) print nested\n",
	}
	for _, src := range cases {
		file, err := Parse(strings.NewReader(src), "mixed.zsh")
		if err != nil {
			t.Fatalf("mixed file must parse: %v\nsource:\n%s", err, src)
		}
		if len(whileLoops(file.AST())) != 1 {
			t.Errorf("while loops = %d, want 1", len(whileLoops(file.AST())))
		}
	}
}
