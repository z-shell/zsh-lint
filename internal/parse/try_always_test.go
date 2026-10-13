package parse

import (
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseTryAlwaysAcceptsNativeSyntax(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "top level",
			src:  "{\n  print try\n} always {\n  print cleanup\n}\n",
		},
		{
			name: "tab separated boundary",
			src:  "{\n  print try\n}\talways\t{\n  print cleanup\n}\n",
		},
		{
			name: "nested in function",
			src:  "cleanup() {\n  {\n    print try\n  } always {\n    print cleanup\n  }\n}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if len(file.AST().Stmts) == 0 {
				t.Fatal("Parse() returned no statements")
			}
		})
	}
}

func TestParseTryAlwaysRejectsInvalidSeparators(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "newline before always",
			src:  "{\n  print try\n}\nalways {\n  print cleanup\n}\n",
		},
		{
			name: "semicolon before always",
			src:  "{\n  print try\n}; always {\n  print cleanup\n}\n",
		},
		{
			name: "newline before always after length expansion",
			src:  "{ print -r -- ${#x}; }\nalways { : }\n",
		},
		{
			name: "semicolon before always after length expansion",
			src:  "{ print -r -- ${#x}; }; always { : }\n",
		},
		{
			// The `}` of `${x}` closes the expansion, not a block, so the
			// line has an unmatched `}` (#667).
			name: "parameter expansion closer before always",
			src:  "{ :; } always { : }\nprint ${x} always { a }\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err == nil {
				t.Fatal("Parse() succeeded for native-invalid try/always separator")
			}
		})
	}
}

func TestParseTryAlwaysPreservesHeredocBody(t *testing.T) {
	src := "cat <<'EOF'\n} always {\nEOF\n{\n  print try\n} always {\n  print cleanup\n}\n"
	file, err := Parse(strings.NewReader(src), "heredoc.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	var rendered strings.Builder
	if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
		t.Fatalf("printing AST: %v", err)
	}
	if !strings.Contains(rendered.String(), "} always {") {
		t.Fatalf("rendered heredoc body = %q, want original try/always text", rendered.String())
	}
}

func TestParseTryAlwaysPreservesInactiveLookalikes(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{name: "comment", prefix: "# } always {\n"},
		{name: "single quoted word", prefix: "print '} always {'\n"},
		{name: "command substitution", prefix: "value=$(print '} always {')\n"},
		{name: "legacy backtick substitution", prefix: "value=`print '} always {'`\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := test.prefix + "{\n  print try\n} always {\n  print cleanup\n}\n"
			file, err := Parse(strings.NewReader(src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var rendered strings.Builder
			if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
				t.Fatalf("printing AST: %v", err)
			}
			if !strings.Contains(rendered.String(), "} always {") {
				t.Fatalf("rendered source = %q, want inactive lookalike preserved", rendered.String())
			}
		})
	}
}

// A comment is a word that begins with `#` (zshmisc COMMENTS), so a `#` glued
// to word text before it, as in `${#x}`, `${x}#y` or `a{#b`, is not a comment
// and the boundary later on the line is still active. (#643)
func TestParseTryAlwaysAfterHashInsideWord(t *testing.T) {
	tests := []struct {
		name string
		src  string
		// word is the last word of the try block's last statement.
		word string
	}{
		{
			name: "nested length expansion between quotes",
			src:  "{ print -r -- \"x\"${#${(M)x:#(a|b)}}\"x\"; } always { : }\n",
			word: "\"x\"${#${(M)x:#(a|b)}}\"x\"",
		},
		{name: "length expansion", src: "{ print ${#x}; } always { : }\n", word: "${#x}"},
		{name: "length of positional count", src: "{ print ${##}; } always { : }\n", word: "${##}"},
		{name: "length in nested default", src: "{ print ${x:-${#y}}; } always { : }\n", word: "${x:-${#y}}"},
		{name: "hash after braced expansion", src: "{ print ${x}#y; } always { : }\n", word: "${x}#y"},
		{name: "hash after closing brace in word", src: "{ print a}#b; } always { : }\n", word: "a}#b"},
		{name: "hash after opening brace in word", src: "{ print a{#b; } always { : }\n", word: "a{#b"},
		{name: "hash after escaped brace", src: "{ print \\{#b; } always { : }\n", word: "\\{#b"},
		{name: "hash after brace in command word", src: "{ : ;}#c } always { : }\n", word: "}#c"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			tree := file.AST()
			assertLiteralsMatchSource(t, tree, test.src)
			if len(tree.Stmts) != 2 {
				t.Fatalf("statement count = %d, want the try and always blocks", len(tree.Stmts))
			}
			try, ok := tree.Stmts[0].Cmd.(*syntax.Block)
			if !ok {
				t.Fatalf("first statement = %T, want *syntax.Block", tree.Stmts[0].Cmd)
			}
			if _, ok := tree.Stmts[1].Cmd.(*syntax.Block); !ok {
				t.Fatalf("second statement = %T, want *syntax.Block", tree.Stmts[1].Cmd)
			}
			call, ok := try.Stmts[len(try.Stmts)-1].Cmd.(*syntax.CallExpr)
			if !ok {
				t.Fatalf("last try statement = %T, want *syntax.CallExpr", try.Stmts[len(try.Stmts)-1].Cmd)
			}
			word := call.Args[len(call.Args)-1]
			if got := test.src[word.Pos().Offset():word.End().Offset()]; got != test.word {
				t.Errorf("last word spans %q, want %q", got, test.word)
			}
		})
	}
}

// A `#` directly after a `{` that opens a brace group, in command position,
// still begins a comment, so a lookalike boundary in it stays inactive.
func TestParseTryAlwaysKeepsCommentAfterGroupBrace(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{name: "start of file", prefix: "{# } always {\n:\n}\n"},
		{name: "after semicolon", prefix: ":;{# } always {\n:\n}\n"},
		{name: "after background", prefix: ":&{# } always {\n:\n}\n"},
		{name: "after pipe", prefix: ": |{# } always {\ncat\n}\n"},
		{name: "in subshell", prefix: "({# } always {\n:\n})\n"},
		{name: "in command substitution", prefix: "x=$({# } always {\n:\n})\n"},
		{name: "in backquotes", prefix: "x=`{# } always {\n:\n}`\n"},
		{name: "nested group", prefix: "{{# } always {\n:\n}}\n"},
		{name: "function body", prefix: "f(){# } always {\n:\n}\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := test.prefix + "{\n  print try\n} always {\n  print cleanup\n}\n"
			file, err := Parse(strings.NewReader(src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var comments []string
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if comment, ok := node.(*syntax.Comment); ok {
					comments = append(comments, comment.Text)
				}
				return true
			})
			if len(comments) != 1 || comments[0] != " } always {" {
				t.Fatalf("comments = %q, want the inactive lookalike kept as one comment", comments)
			}
		})
	}
}

// The `}` that ends an unquoted `${...}` belongs to the expansion, so a
// following ` always {` is ordinary text even when the file holds a real
// try/always block that makes the adapter scan it (#667). Each source passes
// `zsh -f -n`, and the last line prints its words.
func TestParseTryAlwaysIgnoresParameterExpansionCloser(t *testing.T) {
	tests := []struct {
		name string
		line string
		args []string
	}{
		{name: "plain", line: "print ${x} always {a}", args: []string{"print", "${x}", "always", "{a}"}},
		{name: "modifier", line: "print ${x:-y} always {b}", args: []string{"print", "${x:-y}", "always", "{b}"}},
		{name: "nested", line: "print ${x:-${y}} always {c}", args: []string{"print", "${x:-${y}}", "always", "{c}"}},
		{name: "glued word", line: "print a${x} always {d}", args: []string{"print", "a${x}", "always", "{d}"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := "{ :; } always { : }\n" + test.line + "\n"
			file, err := Parse(strings.NewReader(src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			stmts := file.AST().Stmts
			call, ok := stmts[len(stmts)-1].Cmd.(*syntax.CallExpr)
			if !ok {
				t.Fatalf("last statement = %T, want *syntax.CallExpr", stmts[len(stmts)-1].Cmd)
			}
			var args []string
			for _, word := range call.Args {
				args = append(args, src[word.Pos().Offset():word.End().Offset()])
			}
			if strings.Join(args, " | ") != strings.Join(test.args, " | ") {
				t.Fatalf("last command words = %q, want %q", args, test.args)
			}
		})
	}
}

func TestParseTryAlwaysAfterArithmeticShift(t *testing.T) {
	src := "(( mask = 1 << 2 ))\n{\n  print try\n} always {\n  print cleanup\n}\n"
	file, err := Parse(strings.NewReader(src), "arithmetic-shift.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(file.AST().Stmts) != 3 {
		t.Fatalf("statement count = %d, want 3", len(file.AST().Stmts))
	}
}

// A word beginning with `#` starts a comment (zshmisc, COMMENTS), and a `#`
// inside a `${...}` begins no word: it is the length flag of `${#x}` or text
// of the expansion's word, even after a blank, a `;` or a `{` there. The scan
// must read on to the try/always boundary after it, while a `#` that begins
// a word, including one in a command substitution inside an expansion,
// still starts a comment whose text the retry leaves alone.
func TestParseTryAlwaysAfterHashInParameterExpansion(t *testing.T) {
	tests := []struct {
		name string
		src  string
		// expansion is source text the printed tree must keep.
		expansion string
		// comments are the comment texts the tree must hold.
		comments []string
	}{
		{
			name:      "length of nested match",
			src:       "{ print -r -- \"x\"${#${(M)x:#(a|b)}}\"x\"; } always { : }\n",
			expansion: "${#${(M)x:#(a|b)}}",
		},
		{name: "length", src: "{ print -r -- ${#x}; } always { : }\n", expansion: "${#x}"},
		{name: "length without separator", src: "{ : ${#x} } always { : }\n", expansion: "${#x}"},
		{name: "length in nested block", src: "{ { print -r -- ${#x}; } } always { : }\n", expansion: "${#x}"},
		{name: "length in arithmetic", src: "(( n = ${#x} )); { :; } always { : }\n", expansion: "${#x}"},
		{name: "blank before hash in word", src: "{ print -r -- ${x:- #a}; } always { : }\n", expansion: "${x:- #a}"},
		{name: "tab before hash in word", src: "{ print -r -- ${x:-\t#a}; } always { : }\n", expansion: "${x:-\t#a}"},
		{name: "semicolon before hash in word", src: "{ print -r -- ${x:-;#a}; } always { : }\n", expansion: "${x:-;#a}"},
		{name: "brace before hash in word", src: "{ print -r -- ${x:-{#a}}; } always { : }\n", expansion: "${x:-{#a}}"},
		{name: "blank before hash in flag", src: "{ print -r -- ${(j: #:)x}; } always { : }\n", expansion: "${(j: #:)x}"},
		// Each of these reads the expansion's extent by one shared rule: braces
		// nest and quotes quote in an unquoted `${...}`, a parameter or a
		// closed backquoted command inside it leaves its word open, and an
		// empty `${}` ends at once.
		{name: "blank before hash after nested braces", src: "{ print -r -- ${x:-{a} #b}; } always { : }\n", expansion: "${x:-{a} #b}"},
		{name: "blank before hash after quoted brace", src: "{ print -r -- ${x:-'}' #a}; } always { : }\n", expansion: "${x:-'}' #a}"},
		{name: "blank before hash after parameter", src: "{ print -r -- ${x:-$y #a}; } always { : }\n", expansion: "${x:-$y #a}"},
		{
			name:      "blank before hash after backquoted command",
			src:       "{ print -r -- ${x:-`print -r -- a` #b}; } always { : }\n",
			expansion: " #b}",
		},
		{
			name:      "comment after empty expansion",
			src:       "{ print -r -- ${} # <<EOF\n} always { : }\n",
			expansion: "${}",
			comments:  []string{" <<EOF"},
		},
		{
			name:      "comment after expansions",
			src:       "{ : ${#x}; } always { : } # ${#x} } always {\n",
			expansion: "${#x}",
			comments:  []string{" ${#x} } always {"},
		},
		{
			name:      "comment in substitution in expansion",
			src:       "{ print -r -- ${x:-$(print -r -- a #it's\n)}; } always { : ${#x}; }\n",
			expansion: "${#x}",
			comments:  []string{"it's"},
		},
		{
			name:      "comment in backquoted command in expansion",
			src:       "{ print -r -- ${x:-`print -r -- a #it's`}\n} always { : ${#x}; }\n",
			expansion: "${#x}",
			comments:  []string{"it's"},
		},
		{
			name:      "comment after glued brace",
			src:       "{#c ${#x}\n  print -r -- ${#x}\n} always { : }\n",
			expansion: "${#x}",
			comments:  []string{"c ${#x}"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			tree := file.AST()
			stmts := tree.Stmts
			if len(stmts) < 2 {
				t.Fatalf("statements = %d, want the try block then the always block", len(stmts))
			}
			last, ok := stmts[len(stmts)-1].Cmd.(*syntax.Block)
			if !ok {
				t.Fatalf("last statement is %T, want the always block", stmts[len(stmts)-1].Cmd)
			}
			if _, ok := stmts[len(stmts)-2].Cmd.(*syntax.Block); !ok {
				t.Fatalf("statement before the always block is %T, want the try block", stmts[len(stmts)-2].Cmd)
			}
			alwaysBody := strings.Index(test.src, "} always {") + len("} always ")
			if got := int(last.Lbrace.Offset()); got != alwaysBody {
				t.Errorf("always block opens at %d, want %d", got, alwaysBody)
			}
			var rendered strings.Builder
			if err := syntax.NewPrinter().Print(&rendered, tree); err != nil {
				t.Fatalf("printing AST: %v", err)
			}
			if !strings.Contains(rendered.String(), test.expansion) {
				t.Errorf("rendered source = %q, want it to keep %q", rendered.String(), test.expansion)
			}
			var comments []string
			syntax.Walk(tree, func(node syntax.Node) bool {
				if comment, ok := node.(*syntax.Comment); ok {
					comments = append(comments, comment.Text)
				}
				return true
			})
			if strings.Join(comments, "\n") != strings.Join(test.comments, "\n") {
				t.Errorf("comments = %q, want %q", comments, test.comments)
			}
		})
	}
}

func TestParseTryAlwaysPreservesBodyPositions(t *testing.T) {
	src := "{\n  print try\n}  always {\n  print cleanup\n}\n"
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	var positions []syntax.Pos
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 || call.Args[0].Lit() != "print" {
			return true
		}
		positions = append(positions, call.Pos())
		return true
	})
	if len(positions) != 2 {
		t.Fatalf("print positions = %v, want two", positions)
	}
	for i, want := range []struct{ line, col uint }{{2, 3}, {4, 3}} {
		if positions[i].Line() != want.line || positions[i].Col() != want.col {
			t.Errorf("print[%d].Pos() = %d:%d, want %d:%d", i, positions[i].Line(), positions[i].Col(), want.line, want.col)
		}
	}
}

func TestParseTryAlwaysRetriesOnce(t *testing.T) {
	src := []byte("{\n  :\n} always {\n  :\n}\n")
	_, firstErr := parseTree(src, "retry-once.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the original source")
	}

	attempts := 0
	_, err := parseTryAlwaysWithParser(src, "retry-once.zsh", firstErr, func(transformed []byte, name string) (*syntax.File, error) {
		attempts++
		if len(transformed) != len(src) {
			t.Fatalf("transformed length = %d, want %d", len(transformed), len(src))
		}
		return parseTree(transformed, name)
	})
	if err != nil {
		t.Fatalf("parseTryAlwaysWithParser() error: %v", err)
	}
	if attempts != 1 {
		t.Errorf("compatibility parser attempts = %d, want 1", attempts)
	}

	_, err = parseTryAlwaysWithParser(src, "wrong-error.zsh", errors.New("other parse error"), parseTree)
	if err == nil {
		t.Fatal("parseTryAlwaysWithParser() unexpectedly accepted a non-parser error")
	}
}

func TestScanTryAlwaysEditsRequiresErrorAtBoundary(t *testing.T) {
	src := []byte("{\n  :\n} always {\n  :\n}\n")
	if edits, ok := scanTryAlwaysEdits(src, 0); ok || len(edits) != 1 {
		t.Fatalf("scanTryAlwaysEdits() = (%v, %t), want one edit and false", edits, ok)
	}
}
