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

// Issue #208: `repeat count sublist`. mvdan/sh reads `repeat` as a command
// name, so the front end rewrites each loop into a WhileClause positioned at
// the `repeat` word whose only condition is the count word, records it in
// File.RepeatLoops, and keeps every other node on its original bytes. Each
// row is `zsh -f -n` valid; the body column is what native Zsh runs, checked
// with `zsh -f -c`.
func TestParseRepeat(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		want      string
		count     string
		whilePos  string
		doPos     string
		donePos   string
		semicolon string
		body      int
	}{
		{"sublist", "repeat 3 print hi\n", "while 3; do print hi; done\n", "3", "1:1", "1:10", "1:18", "", 1},
		{"sublist after semicolon", "repeat 3; print hi\n", "while 3; do print hi; done\n", "3", "1:1", "1:11", "1:19", "1:9", 1},
		{"sublist after two semicolons", "repeat 2; ; print hi\n", "while 2; do print hi; done\n", "2", "1:1", "1:13", "1:21", "1:9", 1},
		{"sublist on next line", "repeat 3\nprint hi\n", "while 3; do print hi; done\n", "3", "1:1", "2:1", "2:9", "", 1},
		{"sublist chain", "repeat 3 print a | cat && print b; print c\n", "while 3; do print a | cat && print b; done\nprint c\n", "3", "1:1", "1:10", "1:34", "", 1},
		{"sublist with redirect", "repeat 3 print hi > file\n", "while 3; do print hi >file; done\n", "3", "1:1", "1:10", "1:25", "", 1},
		{"redirect only sublist", "repeat 2 >/dev/null; print hi\n", "while 2; do >/dev/null; done\nprint hi\n", "2", "1:1", "1:10", "1:20", "", 1},
		{"sublist with leading redirect", "repeat 2 2>&1 print hi\n", "while 2; do 2>&1 print hi; done\n", "2", "1:1", "1:10", "1:23", "", 1},
		{"do form", "repeat 3; do print hi; done\n", "while 3; do print hi; done\n", "3", "1:1", "1:11", "1:24", "1:9", 1},
		{"do form without semicolon", "repeat 3 do\nprint hi\ndone\n", "while 3; do\n\tprint hi\ndone\n", "3", "1:1", "1:10", "3:1", "", 1},
		{"do form on next line", "repeat 3\ndo\nprint hi\ndone\n", "while 3; do\n\tprint hi\ndone\n", "3", "1:1", "2:1", "4:1", "", 1},
		{"do form after continuation", "repeat 3 \\\n do print hi; done\n", "while 3; do print hi; done\n", "3", "1:1", "2:2", "2:15", "", 1},
		{"brace form", "repeat 3 { print hi }\n", "while 3; do print hi; done\n", "3", "1:1", "1:10", "1:21", "", 1},
		{"brace form after semicolon", "repeat 3; { print hi }\n", "while 3; do print hi; done\n", "3", "1:1", "1:11", "1:22", "1:9", 1},
		{"brace form on next line", "repeat 3\n{ print hi }\n", "while 3; do print hi; done\n", "3", "1:1", "2:1", "2:12", "", 1},
		{"brace form with redirect", "repeat 3 { print hi } > file\n", "while 3; do print hi; done >file\n", "3", "1:1", "1:10", "1:21", "", 1},
		{"expanded count", "repeat $(( n + 1 )) print hi\n", "while $((n + 1)); do print hi; done\n", "$(( n + 1 ))", "1:1", "1:21", "1:29", "", 1},
		{"quoted count", "repeat \"$n\"; print hi\n", "while \"$n\"; do print hi; done\n", "\"$n\"", "1:1", "1:14", "1:22", "1:12", 1},
		// The count is the parameter `repeat`; the rewritten condition must
		// not be taken for a second site on the next pass.
		{"literal repeat count", "repeat repeat print hi\n", "while repeat; do print hi; done\n", "repeat", "1:1", "1:15", "1:23", "", 1},
		{"literal repeat count do form", "repeat repeat do print hi; done\n", "while repeat; do print hi; done\n", "repeat", "1:1", "1:15", "1:28", "", 1},
		{"literal repeat count brace form", "repeat repeat { print hi }\n", "while repeat; do print hi; done\n", "repeat", "1:1", "1:15", "1:26", "", 1},
		{"empty body at end of input", "repeat 2\n", "while 2; do; done\n", "2", "1:1", "1:9", "1:9", "", 0},
		{"empty body before pipe", "repeat 2 | cat; print hi\n", "while 2; do; done | cat\nprint hi\n", "2", "1:1", "1:9", "1:9", "", 0},
		{"empty body before and", "repeat 2 && print x; print y\n", "while 2; do; done && print x\nprint y\n", "2", "1:1", "1:9", "1:9", "", 0},
		{"empty body in if condition", "if repeat 2; then print t; fi\n", "if while 2; do; done; then print t; fi\n", "2", "1:4", "1:12", "1:12", "1:12", 0},
		{"body after and", "true && repeat 2; print hi\n", "true && while 2; do print hi; done\n", "2", "1:9", "1:19", "1:27", "1:17", 1},
		{"body after pipe", "print a | repeat 2; print hi\n", "print a | while 2; do print hi; done\n", "2", "1:11", "1:21", "1:29", "1:19", 1},
		{"body after time", "time repeat 2; print hi\n", "time while 2; do print hi; done\n", "2", "1:6", "1:16", "1:24", "1:14", 1},
		{"negated", "! repeat 2 false\n", "! while 2; do false; done\n", "2", "1:3", "1:12", "1:17", "", 1},
		{"negated brace form", "! repeat 3 { print hi }\n", "! while 3; do print hi; done\n", "3", "1:3", "1:12", "1:23", "", 1},
		{"negated body", "repeat 3; ! { print hi }\n", "while 3; do ! { print hi; }; done\n", "3", "1:1", "1:11", "1:25", "1:9", 1},
		{"in function", "f() { repeat 2; print hi }\n", "f() { while 2; do print hi; done; }\n", "2", "1:7", "1:17", "1:25", "1:15", 1},
		{"in case arm", "case x in (x) repeat 2; print hi ;; esac\n", "case x in x) while 2; do print hi; done ;; esac\n", "2", "1:15", "1:25", "1:33", "1:23", 1},
		{"in command substitution", "echo $(repeat 2; print hi)\n", "echo $(while 2; do print hi; done)\n", "2", "1:8", "1:18", "1:26", "1:16", 1},
		{"heredoc body", "repeat 2 cat <<EOT\nhi\nEOT\n", "while 2; do cat <<EOT\nhi\nEOT\ndone\n", "2", "1:1", "1:10", "3:4", "", 1},
		{"heredoc before redirect", "repeat 2 cat <<EOT >out\nhi\nEOT\nprint x\n", "while 2; do\n\tcat <<EOT >out\nhi\nEOT\ndone\nprint x\n", "2", "1:1", "1:10", "3:4", "", 1},
		{"heredoc before semicolon", "repeat 2 cat <<EOT;\nhi\nEOT\n", "while 2; do\n\tcat <<EOT\nhi\nEOT\ndone\n", "2", "1:1", "1:10", "3:4", "", 1},
		{"two heredocs", "repeat 2 cat <<A <<B\na\nA\nb\nB\n", "while 2; do cat <<A <<B\na\nA\nb\nB\ndone\n", "2", "1:1", "1:10", "5:2", "", 1},
		{"heredoc in block body", "repeat 2; { cat <<EOT\nhi\nEOT\n}\n", "while 2; do\n\tcat <<EOT\nhi\nEOT\ndone\n", "2", "1:1", "1:11", "4:1", "1:9", 1},
		{"heredoc in command substitution", "repeat 2 print $(cat <<EOT\nhi\nEOT\n)\n", "while 2; do print $(\n\tcat <<EOT\nhi\nEOT\n); done\n", "2", "1:1", "1:10", "4:2", "", 1},
		{"arithmetic body", "repeat 4 (( count++ ))\n", "while 4; do ((count++)); done\n", "4", "1:1", "1:10", "1:23", "", 1},
		{"conditional body", "repeat 2 [[ a == a ]]\n", "while 2; do [[ a == a ]]; done\n", "2", "1:1", "1:10", "1:22", "", 1},
		{"subshell body", "repeat 2 ( print hi )\n", "while 2; do (print hi); done\n", "2", "1:1", "1:10", "1:22", "", 1},
		{"if body", "repeat 2 if true; then print hi; fi; print end\n", "while 2; do if true; then print hi; fi; done\nprint end\n", "2", "1:1", "1:10", "1:36", "", 1},
		{"for body", "repeat 2 for x in a b; do print $x; done\n", "while 2; do for x in a b; do print $x; done; done\n", "2", "1:1", "1:10", "1:41", "", 1},
		{"case body", "repeat 2 case x in (x) print hi ;; esac\n", "while 2; do case x in x) print hi ;; esac done\n", "2", "1:1", "1:10", "1:40", "", 1},
		{"function body", "repeat 2 f() { print hi }\n", "while 2; do f() { print hi; }; done\n", "2", "1:1", "1:10", "1:26", "", 1},
		{"assignment body", "repeat 2 x=1\n", "while 2; do x=1; done\n", "2", "1:1", "1:10", "1:13", "", 1},
		{"background", "repeat 3 print hi & print x\n", "while 3; do print hi; done &\nprint x\n", "3", "1:1", "1:10", "1:19", "", 1},
		// The invocation words are metadata the tree does not span; the
		// closer follows them, and what follows them on the line attaches
		// to the loop.
		{"invocation words", "repeat 3 () { print $1 } a b\nprint y\n", "while 3; do () { print $1; }; done\nprint y\n", "3", "1:1", "1:10", "1:29", "", 1},
		{"invocation words then redirect", "repeat 3 () { print $1 } later > out\nprint x\n", "while 3; do () { print $1; }; done >out\nprint x\n", "3", "1:1", "1:10", "1:31", "", 1},
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
			loops := file.RepeatLoops()
			if len(loops) != 1 {
				t.Fatalf("RepeatLoops() = %d loops, want 1", len(loops))
			}
			loop := loops[0]
			if loop.Loop.Until {
				t.Fatalf("loop is an until loop")
			}
			if got := loop.Loop.WhilePos.String(); got != test.whilePos {
				t.Errorf("WhilePos = %s, want %s", got, test.whilePos)
			}
			if got := loop.Loop.DoPos.String(); got != test.doPos {
				t.Errorf("DoPos = %s, want %s", got, test.doPos)
			}
			if got := loop.Loop.DonePos.String(); got != test.donePos {
				t.Errorf("DonePos = %s, want %s", got, test.donePos)
			}
			if start, end := loop.Count.Pos().Offset(), loop.Count.End().Offset(); test.src[start:end] != test.count {
				t.Errorf("count bytes = %q, want %q", test.src[start:end], test.count)
			}
			if len(loop.Loop.Cond) != 1 || loop.Loop.Cond[0].Cmd.(*syntax.CallExpr).Args[0] != loop.Count {
				t.Errorf("Cond = %v, want the count word alone", loop.Loop.Cond)
			}
			if got := loop.Loop.Cond[0].Semicolon; got.IsValid() != (test.semicolon != "") || (got.IsValid() && got.String() != test.semicolon) {
				t.Errorf("count Semicolon = %v, want %q", got, test.semicolon)
			}
			if got := len(loop.Loop.Do); got != test.body {
				t.Errorf("body statements = %d, want %d", got, test.body)
			}
			if !matchSourceWord([]byte(test.src), int(loop.Loop.WhilePos.Offset()), "repeat") {
				t.Errorf("WhilePos %s is not on the repeat word", loop.Loop.WhilePos)
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
		{"repeat 2 repeat 3 print hi\n", "while 2; do while 3; do print hi; done; done\n"},
		{"repeat 2; repeat 3; print hi\n", "while 2; do while 3; do print hi; done; done\n"},
		{"repeat 2 do repeat 3; print hi; done\n", "while 2; do while 3; do print hi; done; done\n"},
		{"repeat 2 { repeat 3 { print hi } }\n", "while 2; do while 3; do print hi; done; done\n"},
		{"repeat 2 { repeat 3 do print hi; done }\n", "while 2; do while 3; do print hi; done; done\n"},
		{"repeat 2 do repeat 3 { print hi }; done\n", "while 2; do while 3; do print hi; done; done\n"},
		{"repeat 3 { print hi }; repeat 2 { print b }\n", "while 3; do print hi; done\nwhile 2; do print b; done\n"},
		{"repeat 3 do print hi; done; repeat 2 do print b; done\n", "while 3; do print hi; done\nwhile 2; do print b; done\n"},
		{"for x in a b; do repeat 2 print $x; done\n", "for x in a b; do while 2; do print $x; done; done\n"},
		{"repeat 2 repeat 3 { print hi }\n", "while 2; do while 3; do print hi; done; done\n"},
		{"repeat 2 repeat 3 do print hi; done\n", "while 2; do while 3; do print hi; done; done\n"},
		{"repeat 2 repeat 3 (( x++ ))\n", "while 2; do while 3; do ((x++)); done; done\n"},
		{"repeat 2 ( repeat 3 (( x++ )) )\n", "while 2; do (while 3; do ((x++)); done); done\n"},
		{"( repeat 3 do print hi; done )\n", "(while 3; do print hi; done)\n"},
		{"repeat 2 if true; then\n  repeat 3 { print hi }\nfi\n", "while 2; do if true; then\n\twhile 3; do print hi; done\nfi; done\n"},
		// A loop in the condition of a `while` written as such is a site.
		{"while repeat 2 print hi; do break; done\n", "while while 2; do print hi; done; do break; done\n"},
		{"while repeat 2; print hi; do break; done\n", "while while 2; do print hi; done; do break; done\n"},
		{"repeat 2 while repeat 3 print a; do break; done\n", "while 2; do while while 3; do print a; done; do break; done; done\n"},
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
			loops := file.RepeatLoops()
			if want := strings.Count(test.src, "repeat"); len(loops) != want {
				t.Fatalf("RepeatLoops() = %d loops, want %d", len(loops), want)
			}
			for index, loop := range loops {
				if index > 0 && !loop.Loop.WhilePos.After(loops[index-1].Loop.WhilePos) {
					t.Errorf("loop %d at %s is not after loop %d at %s", index, loop.Loop.WhilePos, index-1, loops[index-1].Loop.WhilePos)
				}
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
		// The closer follows the invocation words, so the comment after
		// them stays on the loop's line.
		{"invocation words", "repeat 3 () { print $1 } later # c\n", []string{"1:32  c"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if len(file.RepeatLoops()) != 1 {
				t.Fatalf("RepeatLoops() = %d loops, want 1", len(file.RepeatLoops()))
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
			if loops := file.RepeatLoops(); len(loops) != 0 {
				t.Fatalf("RepeatLoops() = %d loops, want none", len(loops))
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

// Each source is rejected by `zsh -f -n`. The tree path reports the shape at
// the `repeat` word; a separator the parser itself rejects keeps its error.
func TestParseRepeatRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		fixture  string
		wantPos  string
		wantText string
	}{
		{"invalid-208-bare-repeat.txt", "1:1", repeatShapeError},
		{"invalid-208-do-body-unterminated.txt", "1:1", repeatShapeError},
		{"invalid-208-brace-body-unterminated.txt", "1:1", repeatShapeError},
		{"invalid-208-count-then-ampersand.txt", "1:1", repeatShapeError},
		{"invalid-208-do-body-without-separator.txt", "1:1", repeatShapeError},
		{"invalid-208-assignment-prefix.txt", "1:5", repeatShapeError},
		{"invalid-208-double-semicolon.txt", "1:9", "`;;` can only be used in a case clause"},
		{"invalid-208-stray-brace-after-body.txt", "1:20", "`}` can only be used to close a block"},
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

// These sources are `zsh -f -n` valid but no rewrite of the sublist can place
// `done` correctly: a heredoc with an empty body has no node to locate its
// delimiter by, and a heredoc whose line carries more of the sublist would
// swallow the closer. The front end fails closed at the `repeat` word rather
// than guess.
func TestParseRepeatDeclinesUnplaceableHeredocs(t *testing.T) {
	for _, src := range []string{
		"repeat 2 cat <<EOT\nEOT\n",
		"repeat 2 cat <<EOT; print x\nhi\nEOT\n",
		"repeat 2; { cat <<EOT }\nhi\nEOT\n",
	} {
		t.Run(src, func(t *testing.T) {
			_, err := Parse(strings.NewReader(src), "heredoc.zsh")
			var perr syntax.ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse() error = %v, want syntax.ParseError", err)
			}
			if perr.Pos.String() != "1:1" || perr.Text != repeatShapeError {
				t.Fatalf("error = %v, want the shape error at 1:1", err)
			}
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

// The adapter is gated on position: an error before any `repeat` site, or
// one no site's edit could explain, leaves the error untouched.
func TestParseRepeatAdapterDeclinesUnrelatedErrors(t *testing.T) {
	for _, src := range []string{
		"print )\nrepeat 3 do print hi; done\n",
		"repeat 3; print hi }\n",
		"print ${x\n",
	} {
		t.Run(src, func(t *testing.T) {
			_, firstErr := parseTree([]byte(src), "unrelated.zsh")
			if firstErr == nil {
				t.Fatalf("parseTree() accepted %q", src)
			}
			calls := 0
			_, err := parseRepeatWithParser([]byte(src), "unrelated.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
				calls++
				return nil, errors.New("retry must not run")
			})
			if !errors.Is(err, firstErr) {
				t.Fatalf("error = %v, want the first error %v", err, firstErr)
			}
			if calls != 0 {
				t.Fatalf("retry ran %d times, want 0", calls)
			}
		})
	}
}

// A retry that parses but does not produce the loop the edit was made for
// is not trusted: the adapter returns the original error.
func TestParseRepeatAdapterVerifiesRetryShape(t *testing.T) {
	src := []byte("repeat 3 do print hi; done\n")
	_, firstErr := parseTree(src, "verify.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() accepted the do form")
	}
	_, err := parseRepeatWithParser(src, "verify.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
		return parseTree([]byte("print hi\n"), "verify.zsh")
	})
	if !errors.Is(err, firstErr) {
		t.Fatalf("error = %v, want the first error %v", err, firstErr)
	}
}

// A retry whose error did not move past the first one says nothing about
// the site, so the adapter keeps the first error and its position: here the
// word sits in a glob group the site scanner cannot tell from a subshell,
// and the `;` written after the count breaks the group.
func TestParseRepeatAdapterKeepsFirstErrorWhenRetryDoesNotAdvance(t *testing.T) {
	src := []byte("for x in ( repeat 3 do ); do :; done\nprint )\n")
	_, firstErr := parseTree(src, "list.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() accepted the stray parenthesis")
	}
	calls := 0
	_, err := parseRepeatWithParser(src, "list.zsh", firstErr, func(masked []byte, name string) (*syntax.File, error) {
		calls++
		return parseTree(masked, name)
	})
	if !errors.Is(err, firstErr) {
		t.Fatalf("error = %v, want the first error %v", err, firstErr)
	}
	if calls != 1 {
		t.Fatalf("retry ran %d times, want 1", calls)
	}
	_, err = Parse(bytes.NewReader(src), "list.zsh")
	var perr syntax.ParseError
	if !errors.As(err, &perr) || perr.Pos.String() != "2:7" {
		t.Fatalf("Parse() error = %v, want the stray parenthesis at 2:7", err)
	}
}

// The chain hands an error it could not place to the adapter at every level
// of its recursion. On a level whose plain parse stops before the site, an
// inner level already retried the site; the adapter must not repeat that
// retry, which would cost a whole chained parse per level.
func TestParseRepeatAdapterSkipsSitesPastPlainParseError(t *testing.T) {
	src := []byte("print ${x::=1}\nrepeat 3 do\n  print hi\ndone\nprint ${y[a,[^:]]}\n")
	_, plainErr := parseTree(src, "levels.zsh")
	var perr syntax.ParseError
	if !errors.As(plainErr, &perr) || perr.Pos.Line() != 1 {
		t.Fatalf("parseTree() error = %v, want an error on line 1", plainErr)
	}
	_, chainErr := parseWithAdapters(src, "levels.zsh")
	if !errors.As(chainErr, &perr) || perr.Pos.Line() != 5 {
		t.Fatalf("parseWithAdapters() error = %v, want the line 5 blocker", chainErr)
	}
	calls := 0
	_, err := parseRepeatWithParser(src, "levels.zsh", chainErr, func([]byte, string) (*syntax.File, error) {
		calls++
		return nil, errors.New("retry must not run")
	})
	if !errors.Is(err, chainErr) {
		t.Fatalf("error = %v, want the chain error %v", err, chainErr)
	}
	if calls != 0 {
		t.Fatalf("retry ran %d times, want 0", calls)
	}
}

// Every site in command position is found, and none elsewhere.
func TestScanRepeatSites(t *testing.T) {
	src := "repeat 1 a\n" +
		"x=1; repeat 2 b\n" +
		"if true; then repeat 3 c; fi\n" +
		"print repeat 4\n" +
		"'repeat 5' 'x'\n" +
		"$(repeat 6 d)\n" +
		"( repeat 7 e )\n" +
		"case x in (x) repeat 8 f ;; esac\n" +
		"# repeat 9 g\n" +
		"cat <<EOT\nrepeat 10 h\nEOT\n" +
		"repeatx 11 i\n" +
		"x=1 repeat 12 j\n" +
		"! repeat 13 k\n" +
		"{ repeat 14 l }\n" +
		"true && repeat 15 m\n" +
		"a=(repeat 16 n)\n" +
		"case x in (repeat|y) : ;; esac\n" +
		"print \"repeat 17 o\"\n" +
		"(( repeat 18 ))\n" +
		"$'repeat 19'\n" +
		"repeat $(cat n) p\n" +
		"repeat \"$n\"; q\n" +
		"repeat\n"
	var got []string
	for _, site := range scanRepeatSites([]byte(src)) {
		got = append(got, src[site.countStart:site.countEnd])
	}
	want := []string{"1", "2", "3", "6", "7", "8", "13", "14", "15", "$(cat n)", "\"$n\""}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sites = %v, want %v", got, want)
	}
}

func TestRepeatLoopsSliceIsIndependent(t *testing.T) {
	file, err := Parse(strings.NewReader("repeat 2; print hi\n"), "independent.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	loops := file.RepeatLoops()
	loops[0] = RepeatLoop{}
	if again := file.RepeatLoops(); again[0].Loop == nil {
		t.Fatal("RepeatLoops() shares its backing array with the caller")
	}
}

// The rewrite reparses a transformed buffer and rebases the tree and the
// anonymous invocation words back to the source; the other metadata binders
// run on the rebased tree. Every metadata node after a rewritten loop must
// still sit on its original bytes.
func TestParseRepeatKeepsLaterMetadataOnSourceBytes(t *testing.T) {
	src := "repeat 2 do\n  print hi\ndone\n" +
		"() { print $1 } arg\n" +
		"print ${x::=1} ${a[1][2]}\n" +
		"repeat 3; () { print $1 } later; print x\n" +
		"repeat 4 () { print $1 } a b & print y\n"
	file, err := Parse(strings.NewReader(src), "metadata.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if loops := file.RepeatLoops(); len(loops) != 3 {
		t.Fatalf("RepeatLoops() = %d loops, want 3", len(loops))
	}
	// The tree holds the closest typed shapes; the metadata carries `::=` and
	// the second subscript.
	want := "while 2; do\n\tprint hi\ndone\n() { print $1; }\nprint ${x:=1} ${a[1]}\n" +
		"while 3; do () { print $1; }; done\nprint x\nwhile 4; do () { print $1; }; done &\nprint y\n"
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
