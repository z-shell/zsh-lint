package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// arithForLoops returns every arithmetic for loop in tree in source order.
func arithForLoops(tree *syntax.File) []*syntax.ForClause {
	var loops []*syntax.ForClause
	syntax.Walk(tree, func(node syntax.Node) bool {
		if loop, ok := node.(*syntax.ForClause); ok && !loop.Select {
			if _, ok := loop.Loop.(*syntax.CStyleLoop); ok {
				loops = append(loops, loop)
			}
		}
		return true
	})
	return loops
}

// Issue #241: `for (( [expr1] ; [expr2] ; [expr3] )) sublist`. Under LangZsh,
// mvdan/sh rejects the brace body with a LangError and the single-command body
// with a ParseError. The adapter inserts source-mapped `do` and `done` around
// the sublist or over the braces, keeping every node on its original bytes.
// Each row is `zsh -f -n` valid.
func TestParseArithForSublist(t *testing.T) {
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
		{
			name:       "brace body",
			src:        "for (( i = 1; i < 3; i++ )) { print $i }\n",
			want:       "for ((i = 1; i < 3; i++)); do print $i; done\n",
			forPos:     "1:1",
			doPos:      "1:29",
			donePos:    "1:40",
			body:       1,
			background: false,
		},
		{
			name:       "single command",
			src:        "for (( i = 1; i < 3; i++ )) print $i\n",
			want:       "for ((i = 1; i < 3; i++)); do print $i; done\n",
			forPos:     "1:1",
			doPos:      "1:29",
			donePos:    "1:37",
			body:       1,
			background: false,
		},
		{
			name:       "semicolon before single command",
			src:        "for (( i = 1; i < 3; i++ )); print $i\n",
			want:       "for ((i = 1; i < 3; i++)); do print $i; done\n",
			forPos:     "1:1",
			doPos:      "1:30",
			donePos:    "1:38",
			body:       1,
			background: false,
		},
		{
			name:       "newline before single command",
			src:        "for (( i = 1; i < 3; i++ ))\nprint $i\n",
			want:       "for ((i = 1; i < 3; i++)); do print $i; done\n",
			forPos:     "1:1",
			doPos:      "2:1",
			donePos:    "2:9",
			body:       1,
			background: false,
		},
		{
			name:       "empty expressions brace body",
			src:        "for (( ; ; )) { break }\n",
			want:       "for (( ; ; )); do break; done\n",
			forPos:     "1:1",
			doPos:      "1:15",
			donePos:    "1:23",
			body:       1,
			background: false,
		},
		{
			name:       "empty expressions single command",
			src:        "for (( ; ; )) break\n",
			want:       "for (( ; ; )); do break; done\n",
			forPos:     "1:1",
			doPos:      "1:15",
			donePos:    "1:20",
			body:       1,
			background: false,
		},
		{
			name:       "nested parens in expressions",
			src:        "for (( i = (1 + 2); i < (3 * 4); i++ )) print $i\n",
			want:       "for ((i = (1 + 2); i < (3 * 4); i++)); do print $i; done\n",
			forPos:     "1:1",
			doPos:      "1:41",
			donePos:    "1:49",
			body:       1,
			background: false,
		},
		{
			name:       "command substitution in expr",
			src:        "for (( i = 1; i < $(echo 3); i++ )) print $i\n",
			want:       "for ((i = 1; i < $(echo 3); i++)); do print $i; done\n",
			forPos:     "1:1",
			doPos:      "1:37",
			donePos:    "1:45",
			body:       1,
			background: false,
		},
		{
			name:       "and chain in sublist",
			src:        "for (( i = 1; i < 3; i++ )) print $i && break\n",
			want:       "for ((i = 1; i < 3; i++)); do print $i && break; done\n",
			forPos:     "1:1",
			doPos:      "1:29",
			donePos:    "1:46",
			body:       1,
			background: false,
		},
		{
			name:       "pipeline in sublist",
			src:        "for (( i = 1; i < 3; i++ )) print $i | cat\n",
			want:       "for ((i = 1; i < 3; i++)); do print $i | cat; done\n",
			forPos:     "1:1",
			doPos:      "1:29",
			donePos:    "1:43",
			body:       1,
			background: false,
		},
		{
			name:       "trailing ampersand",
			src:        "for (( i = 1; i < 3; i++ )) print $i &\n",
			want:       "for ((i = 1; i < 3; i++)); do print $i; done &\n",
			forPos:     "1:1",
			doPos:      "1:29",
			donePos:    "1:37",
			body:       1,
			background: true,
		},
		{
			name:       "inside function",
			src:        "f() { for (( i = 1; i < 3; i++ )) print $i }\n",
			want:       "f() { for ((i = 1; i < 3; i++)); do print $i; done; }\n",
			forPos:     "1:7",
			doPos:      "1:35",
			donePos:    "1:43",
			body:       1,
			background: false,
		},
		{
			name:       "inside if body",
			src:        "if true; then for (( i = 1; i < 3; i++ )) print $i; fi\n",
			want:       "if true; then for ((i = 1; i < 3; i++)); do print $i; done; fi\n",
			forPos:     "1:15",
			doPos:      "1:43",
			donePos:    "1:51",
			body:       1,
			background: false,
		},
		{
			name:       "inside case body",
			src:        "case x in (x) for (( i = 1; i < 3; i++ )) print $i ;; esac\n",
			want:       "case x in x) for ((i = 1; i < 3; i++)); do print $i; done ;; esac\n",
			forPos:     "1:15",
			doPos:      "1:43",
			donePos:    "1:51",
			body:       1,
			background: false,
		},
		{
			name:       "comment after brace",
			src:        "for (( i = 1; i < 3; i++ )) { print $i } # comment\nprint after\n",
			want:       "for ((i = 1; i < 3; i++)); do print $i; done # comment\nprint after\n",
			forPos:     "1:1",
			doPos:      "1:29",
			donePos:    "1:40",
			body:       1,
			background: false,
		},
		{
			name:       "heredoc body",
			src:        "for (( i = 1; i < 3; i++ )) cat <<EOF\n$i\nEOF\nprint after\n",
			want:       "for ((i = 1; i < 3; i++)); do cat <<EOF\n$i\nEOF\ndone\nprint after\n",
			forPos:     "1:1",
			doPos:      "1:29",
			donePos:    "3:4",
			body:       1,
			background: false,
		},
		{
			name:       "subshell body",
			src:        "for (( i = 1; i < 3; i++ )) (print $i)\n",
			want:       "for ((i = 1; i < 3; i++)); do (print $i); done\n",
			forPos:     "1:1",
			doPos:      "1:29",
			donePos:    "1:39",
			body:       1,
			background: false,
		},
		{
			name:       "compound body",
			src:        "for (( i = 1; i < 3; i++ )) if [[ $i == 1 ]]; then break; fi\n",
			want:       "for ((i = 1; i < 3; i++)); do if [[ $i == 1 ]]; then break; fi; done\n",
			forPos:     "1:1",
			doPos:      "1:29",
			donePos:    "1:61",
			body:       1,
			background: false,
		},
		{
			name:       "two semicolons in gap",
			src:        "for (( i = 1; i < 3; i++ )); ; print $i\n",
			want:       "for ((i = 1; i < 3; i++)); do print $i; done\n",
			forPos:     "1:1",
			doPos:      "1:32",
			donePos:    "1:40",
			body:       1,
			background: false,
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
			loops := arithForLoops(file.AST())
			if len(loops) != 1 {
				t.Fatalf("arith for loops = %d, want 1", len(loops))
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
			if !strings.Contains(test.name, "brace") && len(loop.Do) > 0 && loop.Do[0].Pos() != loop.DoPos {
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

// The loop expressions and the body keep their original bytes and positions.
func TestParseArithForSublistPositions(t *testing.T) {
	src := "for (( i = (1 + 2); i < $(echo 3); i++ ))\n  print -r -- \"$i\" && break\n"
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	loops := arithForLoops(file.AST())
	if len(loops) != 1 {
		t.Fatalf("arith for loops = %d, want 1", len(loops))
	}
	cstyle, ok := loops[0].Loop.(*syntax.CStyleLoop)
	if !ok {
		t.Fatalf("Loop = %T, want *syntax.CStyleLoop", loops[0].Loop)
	}
	if cstyle.Lparen.String() != "1:5" {
		t.Errorf("Lparen = %s, want 1:5", cstyle.Lparen)
	}
	if cstyle.Rparen.String() != "1:40" {
		t.Errorf("Rparen = %s, want 1:40", cstyle.Rparen)
	}
	body := loops[0].Do[0]
	if got := src[body.Pos().Offset():body.End().Offset()]; got != "print -r -- \"$i\" && break" {
		t.Errorf("body text = %q", got)
	}
	if got := loops[0].DonePos.Offset(); int(got) != len(src)-1 {
		t.Errorf("DonePos offset = %d, want %d (the final newline)", got, len(src)-1)
	}
}

// Sibling sites and nested sites in one file.
func TestParseArithForSublistSites(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
		pos  []string
	}{
		{
			name: "sibling sites",
			src:  "for (( i = 1; i < 3; i++ )) print $i\nfor (( j = 1; j < 3; j++ )) print $j\n",
			want: "for ((i = 1; i < 3; i++)); do print $i; done\nfor ((j = 1; j < 3; j++)); do print $j; done\n",
			pos:  []string{"1:1 1:29 1:37", "2:1 2:29 2:37"},
		},
		{
			name: "nested sites",
			src:  "for (( i = 1; i < 3; i++ )) for (( j = 1; j < 3; j++ )) print $i $j\nprint after\n",
			want: "for ((i = 1; i < 3; i++)); do for ((j = 1; j < 3; j++)); do print $i $j; done; done\nprint after\n",
			pos:  []string{"1:1 1:29 1:68", "1:29 1:57 1:68"},
		},
		{
			name: "short form after do form",
			src:  "for (( i = 1; i < 3; i++ )); do print $i; done\nfor (( j = 1; j < 3; j++ )) print $j\n",
			want: "for ((i = 1; i < 3; i++)); do print $i; done\nfor ((j = 1; j < 3; j++)); do print $j; done\n",
			pos:  []string{"1:1 1:30 1:43", "2:1 2:29 2:37"},
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
			for _, loop := range arithForLoops(file.AST()) {
				pos = append(pos, loop.ForPos.String()+" "+loop.DoPos.String()+" "+loop.DonePos.String())
			}
			if strings.Join(pos, "|") != strings.Join(test.pos, "|") {
				t.Errorf("positions = %v, want %v", pos, test.pos)
			}
		})
	}
}

// Comments in the header gap, after the brace and after the body keep their
// nodes and positions.
func TestParseArithForSublistKeepsComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"in header gap", "for (( i = 1; i < 3; i++ )) # note\nprint $i\n", []string{"1:29  note"}},
		{"after semicolon", "for (( i = 1; i < 3; i++ )); # note\nprint $i\n", []string{"1:30  note"}},
		{"after brace", "for (( i = 1; i < 3; i++ )) { print $i } # note\nprint x\n", []string{"1:42  note"}},
		{"after body", "for (( i = 1; i < 3; i++ )) print $i # note\nprint x\n", []string{"1:38  note"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if len(arithForLoops(file.AST())) != 1 {
				t.Fatalf("arith for loops = %d, want 1", len(arithForLoops(file.AST())))
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

// Each fixture is rejected by `zsh -f -n` and must stay rejected.
func TestParseArithForSublistRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		fixture  string
		wantPos  string
		wantText string
	}{
		{"invalid-241-missing-semicolon.txt", "1:21", "`expr` must be followed by `;`"},
		{"invalid-241-ampersand-body.txt", "1:29", "`&` can only immediately follow a statement"},
		{"invalid-241-then-body.txt", "1:29", "`then` can only be used in an `if`"},
		{"invalid-241-brace-close-body.txt", "1:29", "`}` can only be used to close a block"},
		{"invalid-241-double-semicolon-after-header.txt", "1:29", "`;;` can only be used in a case clause"},
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

// The adapter only acts on arithmetic for errors at a scanned site.
func TestParseArithForSublistAdapterDeclinesUnrelatedErrors(t *testing.T) {
	src := []byte("for (( i = 1; i < 3; i++ )) print $i\n")
	other := syntax.ParseError{Filename: "x.zsh", Pos: syntax.NewPos(0, 1, 1), Text: "`select foo [in words]` must be followed by `do`"}
	if _, err := parseArithForSublistWithParser(src, "x.zsh", other, parseWithAdapters); !errors.Is(err, other) {
		t.Errorf("unrelated text: error = %v, want the incoming error", err)
	}
	elsewhere := syntax.ParseError{Filename: "x.zsh", Pos: syntax.NewPos(7, 1, 8), Text: "`for foo [in words]` must be followed by `do`"}
	if _, err := parseArithForSublistWithParser(src, "x.zsh", elsewhere, parseWithAdapters); !errors.Is(err, elsewhere) {
		t.Errorf("position off the word: error = %v, want the incoming error", err)
	}
}

// scanArithForSites finds arithmetic for headers in command position only.
func TestScanArithForSites(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []int
	}{
		{"top level single command", "for (( i = 1; i < 3; i++ )) print $i\n", []int{0}},
		{"top level brace body", "for (( i = 1; i < 3; i++ )) { print $i }\n", []int{0}},
		{"do form is not a site", "for (( i = 1; i < 3; i++ )); do print $i; done\n", nil},
		{"for name is not a site", "for a in 1 2; do print $a; done\n", nil},
		{"after separator and in function", "x; for (( i = 0; i < 2; i++ )) print $i\nf() { for (( j = 0; j < 2; j++ )) print $j }\n", []int{3, 46}},
		{"argument position", "print for (( i = 0; i < 2; i++ ))\n", nil},
		{"quoted", "print 'for (( i = 0; i < 2; i++ ))'\n", nil},
		{"comment", "# for (( i = 0; i < 2; i++ ))\n", nil},
		{"nested", "for (( i = 0; i < 2; i++ )) for (( j = 0; j < 2; j++ )) print $i $j\n", []int{0, 28}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got []int
			for _, site := range scanArithForSites([]byte(test.src)) {
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
