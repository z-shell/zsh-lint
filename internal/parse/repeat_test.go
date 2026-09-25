package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func renderTree(t *testing.T, tree *syntax.File) string {
	t.Helper()
	var rendered bytes.Buffer
	if err := syntax.NewPrinter().Print(&rendered, tree); err != nil {
		t.Fatalf("print tree: %v", err)
	}
	return rendered.String()
}

// repeatClauses returns every repeat loop in tree, in source order.
func repeatClauses(tree *syntax.File) []*syntax.RepeatClause {
	var loops []*syntax.RepeatClause
	syntax.Walk(tree, func(node syntax.Node) bool {
		if loop, ok := node.(*syntax.RepeatClause); ok {
			loops = append(loops, loop)
		}
		return true
	})
	return loops
}

// Issue #208: `repeat count sublist`. The parser fork reads the loop as a
// RepeatClause at the `repeat` word whose Count is the count word (#281).
// Each row is `zsh -f -n` valid; the body column is what native Zsh runs,
// checked with `zsh -f -c`.
func TestParseRepeat(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		want      string
		count     string
		repeatPos string
		doPos     string
		donePos   string
		body      int
	}{
		{"sublist", "repeat 3 print hi\n", "repeat 3; do print hi; done\n", "3", "1:1", "1:10", "1:18", 1},
		{"sublist after semicolon", "repeat 3; print hi\n", "repeat 3; do print hi; done\n", "3", "1:1", "1:11", "1:19", 1},
		{"sublist after two semicolons", "repeat 2; ; print hi\n", "repeat 2; do print hi; done\n", "2", "1:1", "1:13", "1:21", 1},
		{"sublist on next line", "repeat 3\nprint hi\n", "repeat 3; do print hi; done\n", "3", "1:1", "2:1", "2:9", 1},
		{"sublist chain", "repeat 3 print a | cat && print b; print c\n", "repeat 3; do print a | cat && print b; done\nprint c\n", "3", "1:1", "1:10", "1:34", 1},
		{"sublist with redirect", "repeat 3 print hi > file\n", "repeat 3; do print hi >file; done\n", "3", "1:1", "1:10", "1:25", 1},
		{"redirect only sublist", "repeat 2 >/dev/null; print hi\n", "repeat 2; do >/dev/null; done\nprint hi\n", "2", "1:1", "1:10", "1:20", 1},
		{"sublist with leading redirect", "repeat 2 2>&1 print hi\n", "repeat 2; do 2>&1 print hi; done\n", "2", "1:1", "1:10", "1:23", 1},
		{"do form", "repeat 3; do print hi; done\n", "repeat 3; do print hi; done\n", "3", "1:1", "1:11", "1:24", 1},
		{"do form without semicolon", "repeat 3 do\nprint hi\ndone\n", "repeat 3; do\n\tprint hi\ndone\n", "3", "1:1", "1:10", "3:1", 1},
		{"do form on next line", "repeat 3\ndo\nprint hi\ndone\n", "repeat 3; do\n\tprint hi\ndone\n", "3", "1:1", "2:1", "4:1", 1},
		{"do form after continuation", "repeat 3 \\\n do print hi; done\n", "repeat 3; do print hi; done\n", "3", "1:1", "2:2", "2:15", 1},
		{"brace form", "repeat 3 { print hi }\n", "repeat 3; do print hi; done\n", "3", "1:1", "1:10", "1:21", 1},
		{"brace form after semicolon", "repeat 3; { print hi }\n", "repeat 3; do print hi; done\n", "3", "1:1", "1:11", "1:22", 1},
		{"brace form on next line", "repeat 3\n{ print hi }\n", "repeat 3; do print hi; done\n", "3", "1:1", "2:1", "2:12", 1},
		{"brace form with redirect", "repeat 3 { print hi } > file\n", "repeat 3; do print hi; done >file\n", "3", "1:1", "1:10", "1:21", 1},
		{"expanded count", "repeat $(( n + 1 )) print hi\n", "repeat $((n + 1)); do print hi; done\n", "$(( n + 1 ))", "1:1", "1:21", "1:29", 1},
		{"quoted count", "repeat \"$n\"; print hi\n", "repeat \"$n\"; do print hi; done\n", "\"$n\"", "1:1", "1:14", "1:22", 1},
		// The count is the parameter `repeat`; the rewritten condition must
		// not be taken for a second site on the next pass.
		{"literal repeat count", "repeat repeat print hi\n", "repeat repeat; do print hi; done\n", "repeat", "1:1", "1:15", "1:23", 1},
		{"literal repeat count do form", "repeat repeat do print hi; done\n", "repeat repeat; do print hi; done\n", "repeat", "1:1", "1:15", "1:28", 1},
		{"literal repeat count brace form", "repeat repeat { print hi }\n", "repeat repeat; do print hi; done\n", "repeat", "1:1", "1:15", "1:26", 1},
		{"empty body at end of input", "repeat 2\n", "repeat 2; do; done\n", "2", "1:1", "2:1", "2:1", 0},
		{"empty body before pipe", "repeat 2 | cat; print hi\n", "repeat 2; do; done | cat\nprint hi\n", "2", "1:1", "1:10", "1:10", 0},
		{"empty body before and", "repeat 2 && print x; print y\n", "repeat 2; do; done && print x\nprint y\n", "2", "1:1", "1:10", "1:10", 0},
		{"empty body in if condition", "if repeat 2; then print t; fi\n", "if repeat 2; do; done; then print t; fi\n", "2", "1:4", "1:14", "1:14", 0},
		{"body after and", "true && repeat 2; print hi\n", "true && repeat 2; do print hi; done\n", "2", "1:9", "1:19", "1:27", 1},
		{"body after pipe", "print a | repeat 2; print hi\n", "print a | repeat 2; do print hi; done\n", "2", "1:11", "1:21", "1:29", 1},
		{"body after time", "time repeat 2; print hi\n", "time repeat 2; do print hi; done\n", "2", "1:6", "1:16", "1:24", 1},
		{"negated", "! repeat 2 false\n", "! repeat 2; do false; done\n", "2", "1:3", "1:12", "1:17", 1},
		{"negated brace form", "! repeat 3 { print hi }\n", "! repeat 3; do print hi; done\n", "3", "1:3", "1:12", "1:23", 1},
		{"negated body", "repeat 3; ! { print hi }\n", "repeat 3; do ! { print hi; }; done\n", "3", "1:1", "1:11", "1:25", 1},
		{"in function", "f() { repeat 2; print hi }\n", "f() { repeat 2; do print hi; done; }\n", "2", "1:7", "1:17", "1:25", 1},
		{"in case arm", "case x in (x) repeat 2; print hi ;; esac\n", "case x in x) repeat 2; do print hi; done ;; esac\n", "2", "1:15", "1:25", "1:33", 1},
		{"in command substitution", "echo $(repeat 2; print hi)\n", "echo $(repeat 2; do print hi; done)\n", "2", "1:8", "1:18", "1:26", 1},
		{"heredoc body", "repeat 2 cat <<EOT\nhi\nEOT\n", "repeat 2; do cat <<EOT\nhi\nEOT\ndone\n", "2", "1:1", "1:10", "3:4", 1},
		{"heredoc before redirect", "repeat 2 cat <<EOT >out\nhi\nEOT\nprint x\n", "repeat 2; do cat <<EOT >out; done\nhi\nEOT\nprint x\n", "2", "1:1", "1:10", "1:24", 1},
		{"heredoc before semicolon", "repeat 2 cat <<EOT;\nhi\nEOT\n", "repeat 2; do cat <<EOT; done\nhi\nEOT\n", "2", "1:1", "1:10", "1:19", 1},
		{"two heredocs", "repeat 2 cat <<A <<B\na\nA\nb\nB\n", "repeat 2; do cat <<A <<B\na\nA\nb\nB\ndone\n", "2", "1:1", "1:10", "5:2", 1},
		// The sublist ends in a closing keyword another adapter synthesized;
		// its rebased position does not carry the keyword's length (#300).
		{"sublist ending in alternate for", "repeat 3 for i (a b) { print $i }\nprint after\n", "repeat 3; do for i in a b; do print $i; done; done\nprint after\n", "3", "1:1", "1:10", "1:34", 1},
		{"sublist ending in alternate for at end of input", "repeat 3 for i (a b) { print $i }\n", "repeat 3; do for i in a b; do print $i; done; done\n", "3", "1:1", "1:10", "1:34", 1},
		{"sublist ending in brace if", "repeat 2 if (( 1 )) { print hi }\nprint after\n", "repeat 2; do if ((1)); then print hi; fi; done\nprint after\n", "2", "1:1", "1:10", "1:33", 1},
		{"sublist ending in short if", "repeat 2 if (( 1 )) print a\nprint after\n", "repeat 2; do if ((1)); then print a; fi; done\nprint after\n", "2", "1:1", "1:10", "1:28", 1},
		{"body after separator ending in alternate for", "repeat 2; for i (a b) { print $i }\nprint after\n", "repeat 2; do for i in a b; do print $i; done; done\nprint after\n", "2", "1:1", "1:11", "1:35", 1},
		{"alternate for body in function", "f() { repeat 2 for i (a b) { print $i } }\n", "f() { repeat 2; do for i in a b; do print $i; done; done; }\n", "2", "1:7", "1:16", "1:40", 1},
		{"heredoc in block body", "repeat 2; { cat <<EOT\nhi\nEOT\n}\n", "repeat 2; do\n\tcat <<EOT\nhi\nEOT\ndone\n", "2", "1:1", "1:11", "4:1", 1},
		{"heredoc in command substitution", "repeat 2 print $(cat <<EOT\nhi\nEOT\n)\n", "repeat 2; do print $(\n\tcat <<EOT\nhi\nEOT\n); done\n", "2", "1:1", "1:10", "4:2", 1},
		{"arithmetic body", "repeat 4 (( count++ ))\n", "repeat 4; do ((count++)); done\n", "4", "1:1", "1:10", "1:23", 1},
		{"conditional body", "repeat 2 [[ a == a ]]\n", "repeat 2; do [[ a == a ]]; done\n", "2", "1:1", "1:10", "1:22", 1},
		{"subshell body", "repeat 2 ( print hi )\n", "repeat 2; do (print hi); done\n", "2", "1:1", "1:10", "1:22", 1},
		{"if body", "repeat 2 if true; then print hi; fi; print end\n", "repeat 2; do if true; then print hi; fi; done\nprint end\n", "2", "1:1", "1:10", "1:36", 1},
		{"for body", "repeat 2 for x in a b; do print $x; done\n", "repeat 2; do for x in a b; do print $x; done; done\n", "2", "1:1", "1:10", "1:41", 1},
		{"case body", "repeat 2 case x in (x) print hi ;; esac\n", "repeat 2; do case x in x) print hi ;; esac done\n", "2", "1:1", "1:10", "1:40", 1},
		{"function body", "repeat 2 f() { print hi }\n", "repeat 2; do f() { print hi; }; done\n", "2", "1:1", "1:10", "1:26", 1},
		{"assignment body", "repeat 2 x=1\n", "repeat 2; do x=1; done\n", "2", "1:1", "1:10", "1:13", 1},
		{"background", "repeat 3 print hi & print x\n", "repeat 3; do print hi; done &\nprint x\n", "3", "1:1", "1:10", "1:18", 1},
		// The invocation words are metadata the tree does not span. What
		// follows them on the line is dropped in every context (#279), so
		// no row puts anything there.
		{"invocation words", "repeat 3 () { print $1 } a b\nprint y\n", "repeat 3; do () { print $1; }; done\nprint y\n", "3", "1:1", "1:10", "1:25", 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if got := renderTree(t, file.AST()); got != test.want {
				t.Fatalf("rendered tree = %q, want %q", got, test.want)
			}
			loops := repeatClauses(file.AST())
			if len(loops) != 1 {
				t.Fatalf("repeat loops = %d, want 1", len(loops))
			}
			loop := loops[0]
			if got := loop.RepeatPos.String(); got != test.repeatPos {
				t.Errorf("RepeatPos = %s, want %s", got, test.repeatPos)
			}
			if got := loop.DoPos.String(); got != test.doPos {
				t.Errorf("DoPos = %s, want %s", got, test.doPos)
			}
			if got := loop.DonePos.String(); got != test.donePos {
				t.Errorf("DonePos = %s, want %s", got, test.donePos)
			}
			if start, end := loop.Count.Pos().Offset(), loop.Count.End().Offset(); test.src[start:end] != test.count {
				t.Errorf("count bytes = %q, want %q", test.src[start:end], test.count)
			}
			if got := len(loop.Do); got != test.body {
				t.Errorf("body statements = %d, want %d", got, test.body)
			}
			if !matchSourceWord([]byte(test.src), int(loop.RepeatPos.Offset()), "repeat") {
				t.Errorf("RepeatPos %s is not on the repeat word", loop.RepeatPos)
			}
			assertLiteralsMatchSource(t, file.AST(), test.src)
		})
	}
}

// Nested loops are rewritten innermost first, so each loop is closed before
// the loop around it, whatever body forms the two use.
func TestParseRepeatNested(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"repeat 2 repeat 3 print hi\n", "repeat 2; do repeat 3; do print hi; done; done\n"},
		{"repeat 2; repeat 3; print hi\n", "repeat 2; do repeat 3; do print hi; done; done\n"},
		{"repeat 2 do repeat 3; print hi; done\n", "repeat 2; do repeat 3; do print hi; done; done\n"},
		{"repeat 2 { repeat 3 { print hi } }\n", "repeat 2; do repeat 3; do print hi; done; done\n"},
		{"repeat 2 { repeat 3 do print hi; done }\n", "repeat 2; do repeat 3; do print hi; done; done\n"},
		{"repeat 2 do repeat 3 { print hi }; done\n", "repeat 2; do repeat 3; do print hi; done; done\n"},
		{"repeat 3 { print hi }; repeat 2 { print b }\n", "repeat 3; do print hi; done\nrepeat 2; do print b; done\n"},
		{"repeat 3 do print hi; done; repeat 2 do print b; done\n", "repeat 3; do print hi; done\nrepeat 2; do print b; done\n"},
		{"for x in a b; do repeat 2 print $x; done\n", "for x in a b; do repeat 2; do print $x; done; done\n"},
		{"repeat 2 repeat 3 { print hi }\n", "repeat 2; do repeat 3; do print hi; done; done\n"},
		{"repeat 2 repeat 3 do print hi; done\n", "repeat 2; do repeat 3; do print hi; done; done\n"},
		{"repeat 2 repeat 3 (( x++ ))\n", "repeat 2; do repeat 3; do ((x++)); done; done\n"},
		{"repeat 2 ( repeat 3 (( x++ )) )\n", "repeat 2; do (repeat 3; do ((x++)); done); done\n"},
		{"( repeat 3 do print hi; done )\n", "(repeat 3; do print hi; done)\n"},
		{"repeat 2 if true; then\n  repeat 3 { print hi }\nfi\n", "repeat 2; do if true; then\n\trepeat 3; do print hi; done\nfi; done\n"},
		// A loop in the condition of a `while` written as such is a site.
		{"while repeat 2 print hi; do break; done\n", "while repeat 2; do print hi; done; do break; done\n"},
		{"while repeat 2; print hi; do break; done\n", "while repeat 2; do print hi; done; do break; done\n"},
		{"repeat 2 while repeat 3 print a; do break; done\n", "repeat 2; do while repeat 3; do print a; done; do break; done; done\n"},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), "nested.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if got := renderTree(t, file.AST()); got != test.want {
				t.Fatalf("rendered tree = %q, want %q", got, test.want)
			}
			if got, want := len(repeatClauses(file.AST())), strings.Count(test.src, "repeat"); got != want {
				t.Fatalf("repeat loops = %d, want %d", got, want)
			}
			assertLiteralsMatchSource(t, file.AST(), test.src)
		})
	}
}

// A comment between the count and the body, or inside the body, must survive
// on its original position: the suppression pass reads comment nodes.
func TestParseRepeatKeepsComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"after semicolon", "repeat 2; # c\nprint hi\n", []string{"1:11  c"}},
		{"after count", "repeat 2 # c\nprint hi\n", []string{"1:10  c"}},
		{"do form", "repeat 3 do # c\nprint hi # d\ndone\n", []string{"1:13  c", "2:10  d"}},
		{"brace form", "repeat 3 { # c\nprint hi\n}\n", []string{"1:12  c"}},
		{"sublist", "repeat 3 print hi # c\n", []string{"1:19  c"}},
		// A comment after anonymous function invocation words is dropped in
		// every context (#279), so it has no row here.
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if got := len(repeatClauses(file.AST())); got != 1 {
				t.Fatalf("repeat loops = %d, want 1", got)
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

// The word is only the reserved word in command position, and only as a bare
// literal; these rows keep their ordinary tree and record no loop.
func TestParseRepeatKeepsOrdinaryUses(t *testing.T) {
	for _, src := range []string{
		"print repeat 3\n",
		"\\repeat 3\n",
		"'repeat' 3\n",
		"x=repeat\n",
		"command repeat 3\n",
		"print \"repeat 3 do\"\n",
		"print ${repeat:-3}\n",
		"# repeat 3 do\n",
		"cat <<EOT\nrepeat 3 do\nEOT\n",
	} {
		t.Run(src, func(t *testing.T) {
			file, err := Parse(strings.NewReader(src), "ordinary.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if loops := repeatClauses(file.AST()); len(loops) != 0 {
				t.Fatalf("repeat loops = %d, want none", len(loops))
			}
			plain, err := parseTree([]byte(src), "ordinary.zsh")
			if err != nil {
				t.Fatalf("parseTree() error: %v", err)
			}
			if got, want := renderTree(t, file.AST()), renderTree(t, plain); got != want {
				t.Fatalf("rendered tree = %q, want the plain parse %q", got, want)
			}
		})
	}
}

// Each source is rejected by `zsh -f -n`, and the parser fork rejects it at
// the point it stops reading the loop.
func TestParseRepeatRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		fixture  string
		wantPos  string
		wantText string
	}{
		{"invalid-208-bare-repeat.txt", "3:1", "`repeat` must be followed by a count word"},
		{"invalid-208-do-body-unterminated.txt", "3:1", "`repeat` statement must end with `done`"},
		{"invalid-208-brace-body-unterminated.txt", "3:10", "`{` must be followed by `}`"},
		{"invalid-208-count-then-ampersand.txt", "3:10", "repeat loop body must be a command"},
		{"invalid-208-do-body-without-separator.txt", "3:1", "`repeat` statement must end with `done`"},
		{"invalid-208-assignment-prefix.txt", "3:5", assignedRepeatError},
		{"invalid-208-double-semicolon.txt", "3:9", "`;;` can only be used in a case clause"},
		{"invalid-208-stray-brace-after-body.txt", "3:20", "`}` can only be used to close a block"},
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

// These sources are `zsh -f -n` valid. The former rewrite could not place its
// synthetic `done` around them and failed closed; the parser fork reads them
// as it reads any heredoc in a body (#281). A heredoc node spans its body
// and delimiter, as upstream records it.
func TestParseRepeatHeredocBodies(t *testing.T) {
	for _, tt := range []struct {
		src   string
		hdocs []string
	}{
		{"repeat 2 cat <<EOT\nEOT\n", []string{""}},
		{"repeat 2 cat <<EOT; print x\nhi\nEOT\n", []string{"hi\nEOT"}},
		{"repeat 2; { cat <<EOT }\nhi\nEOT\n", []string{"hi\nEOT"}},
	} {
		t.Run(tt.src, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tt.src), "heredoc.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if got := len(repeatClauses(file.AST())); got != 1 {
				t.Fatalf("repeat loops = %d, want 1", got)
			}
			var hdocs []string
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if redirect, ok := node.(*syntax.Redirect); ok && redirect.Hdoc != nil {
					hdocs = append(hdocs, nodeText(tt.src, redirect.Hdoc))
				} else if ok && redirect.Op == syntax.Hdoc {
					hdocs = append(hdocs, "")
				}
				return true
			})
			if strings.Join(hdocs, "|") != strings.Join(tt.hdocs, "|") {
				t.Fatalf("heredoc bodies = %q, want %q", hdocs, tt.hdocs)
			}
			assertLiteralsMatchSource(t, file.AST(), tt.src)
		})
	}
}

// A later, unrelated parser gap after a rewritten loop is reported on its
// own original position, through the source map the rewrite builds.
func TestParseRepeatReportsLaterBlocker(t *testing.T) {
	tests := []struct {
		src     string
		wantPos string
	}{
		{"repeat 3 do\nprint hi\ndone\nprint )\n", "4:7"},
		{"repeat 3; print hi\nrepeat 2\n{ print b }\nforeach v ($a)\nend\n", "4:1"},
		{"repeat 3 { print hi }\n  print )\n", "2:9"},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			_, err := Parse(strings.NewReader(test.src), "later.zsh")
			var perr syntax.ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse() error = %v, want syntax.ParseError", err)
			}
			if got := perr.Pos.String(); got != test.wantPos {
				t.Fatalf("position = %s, want %s", got, test.wantPos)
			}
		})
	}
}

// Every metadata node after a repeat loop must sit on its original bytes;
// the loop once went through a rewrite that rebased the tree.
func TestParseRepeatKeepsLaterMetadataOnSourceBytes(t *testing.T) {
	src := "repeat 2 do\n  print hi\ndone\n" +
		"() { print $1 } arg\n" +
		"print ${x::=1} ${a[1][2]}\n" +
		"repeat 3; () { print $1 } later; print x\n" +
		"repeat 4 () { print $1 } a b\nprint y\n"
	file, err := Parse(strings.NewReader(src), "metadata.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if loops := repeatClauses(file.AST()); len(loops) != 3 {
		t.Fatalf("repeat loops = %d, want 3", len(loops))
	}
	// The tree holds the closest typed shapes; the metadata carries `::=` and
	// the second subscript.
	want := "repeat 2; do\n\tprint hi\ndone\n() { print $1; }\nprint ${x:=1} ${a[1]}\n" +
		"repeat 3; do () { print $1; }; done\nprint x\nrepeat 4; do () { print $1; }; done\nprint y\n"
	if got := renderTree(t, file.AST()); got != want {
		t.Errorf("rendered tree = %q, want %q", got, want)
	}
	invocations := file.AnonymousInvocations()
	if len(invocations) != 3 {
		t.Fatalf("AnonymousInvocations() = %d, want 3", len(invocations))
	}
	for index, want := range []string{"arg", "later", "a b"} {
		var texts []string
		for _, word := range invocations[index].Words {
			texts = append(texts, nodeText(src, word))
		}
		if got := strings.Join(texts, " "); got != want {
			t.Errorf("invocation %d words = %q, want %q on their source bytes", index, got, want)
		}
		if got := nodeText(src, invocations[index].Function); !strings.HasPrefix(got, "() { print $1 }") {
			t.Errorf("invocation %d function text = %q", index, got)
		}
	}
	always := file.AssignAlwaysExpansions()
	if len(always) != 1 || nodeText(src, always[0]) != "${x::=1}" {
		t.Errorf("AssignAlwaysExpansions() = %v, want ${x::=1} on its source bytes", always)
	}
	second := file.SecondSubscripts()
	if len(second) != 1 || nodeText(src, second[0].Expansion) != "${a[1][2]}" ||
		len(second[0].Subscripts) != 1 || nodeText(src, second[0].Subscripts[0]) != "2" {
		t.Errorf("SecondSubscripts() = %v, want ${a[1][2]} with subscript 2 on its source bytes", second)
	}
	assertLiteralsMatchSource(t, file.AST(), src)
}
