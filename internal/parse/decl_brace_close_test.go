package parse

import (
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// declarationBlocks returns every block whose last statement is a declaration
// or let clause, in source order.
func declarationBlocks(tree *syntax.File) []*syntax.Block {
	var found []*syntax.Block
	syntax.Walk(tree, func(node syntax.Node) bool {
		block, ok := node.(*syntax.Block)
		if !ok || len(block.Stmts) == 0 {
			return true
		}
		if isDeclarationCommand(block.Stmts[len(block.Stmts)-1].Cmd) {
			found = append(found, block)
		}
		return true
	})
	return found
}

// assertLiteralsMatchSource fails when any literal in the tree does not read
// back from the original bytes at its own position, which catches a leaked
// transformed byte or a shifted offset.
func assertLiteralsMatchSource(t *testing.T, tree *syntax.File, src string) {
	t.Helper()
	syntax.Walk(tree, func(node syntax.Node) bool {
		lit, ok := node.(*syntax.Lit)
		if !ok {
			return true
		}
		// A heredoc body literal ends past its delimiter, so only the
		// start is compared.
		start := int(lit.Pos().Offset())
		if !strings.HasPrefix(src[start:], lit.Value) {
			t.Errorf("literal %q at %s does not match the source there", lit.Value, lit.Pos())
		}
		return true
	})
}

// Issue #231 and #259: a declaration builtin or `let` may be the last command
// of a brace block with no separator before `}`. The parser reads the clause
// up to a stop token and a `}` word is not one, so the block never closes.
// The adapter must accept every declaration word, keep every position, and
// leave the statement without a separator, as the original source has none.
func TestParseDeclarationBraceClose(t *testing.T) {
	tests := []struct {
		name string
		src  string
		// braces lists the position of every `}` closing a block whose last
		// statement is a declaration or let clause.
		braces []string
		// lastWord is the command word of the first such clause.
		lastWord string
	}{
		{"typeset with flag", "{ typeset -g C=1 }\n", []string{"1:18"}, "typeset"},
		{"local", "{ local C=1 }\n", []string{"1:13"}, "local"},
		{"export", "{ export C=1 }\n", []string{"1:14"}, "export"},
		{"readonly", "{ readonly C=1 }\n", []string{"1:16"}, "readonly"},
		{"declare", "{ declare C=1 }\n", []string{"1:15"}, "declare"},
		{"nameref", "{ nameref x=y }\n", []string{"1:15"}, "nameref"},
		{"let", "{ let x=1 }\n", []string{"1:11"}, "let"},
		{"let increment in function body", "f() { let i++ }\n", []string{"1:15"}, "let"},
		{"corpus site with redirect", "{ typeset -g COLS=\"$(tput cols)\" } 2>/dev/null\n", []string{"1:34"}, "typeset"},
		{"function body", "f() { local x=1 }\n", []string{"1:17"}, "local"},
		{"anonymous function body", "() { local x=1 }\n", []string{"1:16"}, "local"},
		{"after another statement", "f() { print a; local x=1 }\n", []string{"1:26"}, "local"},
		{"two blocks on one line", "{ local x=1 }; { local y=2 }\n", []string{"1:13", "1:28"}, "local"},
		{"two blocks on two lines", "{ local x=1 }\n{ local y=2 }\n", []string{"1:13", "2:13"}, "local"},
		{"and list", "{ local x=1 } && print ok\n", []string{"1:13"}, "local"},
		{"and list of two blocks", "{ typeset -g C=1 } && { typeset -g D=2 }\n", []string{"1:18", "1:40"}, "typeset"},
		{"tab before brace", "{ local x=1\t}\n", []string{"1:13"}, "local"},
		{"name without value", "{ local x }\n", []string{"1:11"}, "local"},
		{"flag with several names", "{ local -a x y }\n", []string{"1:16"}, "local"},
		{"double quoted brace value", "{ typeset x=\"}\" }\n", []string{"1:17"}, "typeset"},
		{"single quoted brace value", "{ typeset x='}' }\n", []string{"1:17"}, "typeset"},
		{"quoted assignment", "{ local \"x=1\" }\n", []string{"1:15"}, "local"},
		{"array value", "{ local x=(1 2) }\n", []string{"1:17"}, "local"},
		{"braced expansion value", "{ local x=${y} }\n", []string{"1:16"}, "local"},
		{"pipe in substitution value", "{ typeset -g x=$(a | b) }\n", []string{"1:25"}, "typeset"},
		{"list in substitution value", "{ local x=$(f; g) }\n", []string{"1:19"}, "local"},
		{"newline in substitution value", "{ local x=$(f\n g) }\n", []string{"2:5"}, "local"},
		{"comment in substitution value", "{ local x=$(f # c\n g) }\n", []string{"2:5"}, "local"},
		{"legacy substitution value", "{ local x=`cmd` }\n", []string{"1:17"}, "local"},
		{"pipe in legacy substitution value", "{ local x=`a | b` }\n", []string{"1:19"}, "local"},
		{"both substitution values", "{ local x=$(f) y=`g` }\n", []string{"1:22"}, "local"},
		{"substitution in legacy substitution", "x=`{ local y=$(a | b) }`\n", []string{"1:23"}, "local"},
		{"quoted brace in substitution value", "{ local x=$(print \"}\") }\n", []string{"1:24"}, "local"},
		{"path expansion value", "{ export PATH=$HOME/bin:$PATH }\n", []string{"1:31"}, "export"},
		{"try always", "{ local x=1 } always { print y }\n", []string{"1:13"}, "local"},
		{"try always both declarations", "{ local x=1 } always { local y=2 }\n", []string{"1:13", "1:34"}, "local"},
		{"nested block", "{ { local x=1 } }\n", []string{"1:15"}, "local"},
		{"pipe", "{ local x=1 } | cat\n", []string{"1:13"}, "local"},
		{"glued pipe", "{ local x=1 }|cat\n", []string{"1:13"}, "local"},
		{"background", "{ local x=1 }&\n", []string{"1:13"}, "local"},
		{"redirect", "{ local x=1 } > out\n", []string{"1:13"}, "local"},
		{"glued output redirect", "{ local x=1 }>out\n", []string{"1:13"}, "local"},
		{"glued input redirect", "{ local x=1 }<in\n", []string{"1:13"}, "local"},
		{"stderr redirect then pipe", "{ local x=1 } 2>&1 | cat\n", []string{"1:13"}, "local"},
		{"trailing semicolon", "{ local x=1 };\n", []string{"1:13"}, "local"},
		{"semicolon then command", "{ local x=1 } ; print done\n", []string{"1:13"}, "local"},
		{"trailing comment", "{ local x=1 } # comment\n", []string{"1:13"}, "local"},
		{"two declarations", "{ local x=1; typeset -g y=2 }\n", []string{"1:29"}, "typeset"},
		{"subshell", "( { local x=1 } )\n", []string{"1:15"}, "local"},
		{"command substitution", "x=$({ local y=1 })\n", []string{"1:17"}, "local"},
		{"legacy substitution", "x=`{ local y=1 }`\n", []string{"1:16"}, "local"},
		{"case arm", "case x in a) { local y=1 } ;; esac\n", []string{"1:26"}, "local"},
		{"after heredoc", "cat <<EOF\n} local x=1 }\nEOF\n{ local x=1 }\n", []string{"4:13"}, "local"},
		{"after arithmetic", "(( x = 1 ))\n{ local x=1 }\n", []string{"2:13"}, "local"},
		{"after quoted lookalike", "print '{ local x=1 }'\n{ local x=1 }\n", []string{"2:13"}, "local"},
		{"after comment lookalike", "# { local x=1 }\n{ local x=1 }\n", []string{"2:13"}, "local"},
		{"declaration word as argument", "{ print local x=1 }\n{ local x=1 }\n", []string{"2:13"}, "local"},
		{"continued line before brace", "{ local x=1 \\\n}\n", []string{"2:1"}, "local"},
		{"continued line inside clause", "{ local \\\n  x=1 }\n", []string{"2:7"}, "local"},
		{"negated", "{ ! local x=1 }\n", []string{"1:15"}, "local"},
		{"timed", "{ time local x=1 }\n", []string{"1:18"}, "local"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := []byte(test.src)
			_, firstErr := parseTree(src, test.name+".zsh")
			var parseErr syntax.ParseError
			if !errors.As(firstErr, &parseErr) || !strings.HasSuffix(parseErr.Text, unmatchedBraceClose) {
				t.Fatalf("parseTree() error = %v, want an unmatched brace error", firstErr)
			}

			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			tree := file.AST()
			assertLiteralsMatchSource(t, tree, test.src)

			blocks := declarationBlocks(tree)
			if len(blocks) != len(test.braces) {
				t.Fatalf("found %d declaration blocks, want %d", len(blocks), len(test.braces))
			}
			for index, block := range blocks {
				if got := block.Rbrace.String(); got != test.braces[index] {
					t.Errorf("block %d closes at %s, want %s", index, got, test.braces[index])
				}
				if test.src[block.Rbrace.Offset()] != '}' {
					t.Errorf("block %d Rbrace offset %d is not a brace in the source", index, block.Rbrace.Offset())
				}
				last := block.Stmts[len(block.Stmts)-1]
				if last.Semicolon.IsValid() {
					t.Errorf("block %d last statement keeps a separator at %s", index, last.Semicolon)
				}
				if got, want := last.End().Offset(), last.Cmd.End().Offset(); got != want {
					t.Errorf("block %d last statement ends at %d, want the command end %d", index, got, want)
				}
				if last.End().Offset() >= block.Rbrace.Offset() {
					t.Errorf("block %d last statement ends at %d, past the brace at %d", index, last.End().Offset(), block.Rbrace.Offset())
				}
			}
			first := blocks[0].Stmts[len(blocks[0].Stmts)-1]
			cmd := first.Cmd
			if timed, ok := cmd.(*syntax.TimeClause); ok {
				cmd = timed.Stmt.Cmd
			}
			var word string
			switch cmd := cmd.(type) {
			case *syntax.DeclClause:
				word = cmd.Variant.Value
			case *syntax.LetClause:
				word = "let"
			}
			if word != test.lastWord {
				t.Errorf("first declaration word = %q, want %q", word, test.lastWord)
			}
		})
	}
}

// The adapter must not move any other node: the tree of the separator-less
// block matches the tree of the same source with an explicit `;` except for
// the statement separator itself.
func TestParseDeclarationBraceCloseMatchesExplicitSeparator(t *testing.T) {
	const src = "f() {\n  print a\n  local x=1 }\nprint b\n"
	const twin = "f() {\n  print a\n  local x=1;}\nprint b\n"
	file, err := Parse(strings.NewReader(src), "implicit.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	twinFile, err := Parse(strings.NewReader(twin), "explicit.zsh")
	if err != nil {
		t.Fatalf("Parse(twin) error: %v", err)
	}
	// Every node starts at the same position, and every node except the
	// separator-less statement ends at the same position.
	var got, want []string
	collect := func(tree *syntax.File, into *[]string) {
		syntax.Walk(tree, func(node syntax.Node) bool {
			if node == nil {
				return true
			}
			span := node.Pos().String()
			if _, isStmt := node.(*syntax.Stmt); !isStmt {
				span += " " + node.End().String()
			}
			*into = append(*into, span)
			return true
		})
	}
	collect(file.AST(), &got)
	collect(twinFile.AST(), &want)
	if len(got) != len(want) {
		t.Fatalf("node count = %d, want %d", len(got), len(want))
	}
	for index := range got {
		if got[index] != want[index] {
			t.Errorf("node %d spans %s, want %s", index, got[index], want[index])
		}
	}
	body := file.AST().Stmts[0].Cmd.(*syntax.FuncDecl).Body.Cmd.(*syntax.Block)
	twinBody := twinFile.AST().Stmts[0].Cmd.(*syntax.FuncDecl).Body.Cmd.(*syntax.Block)
	last, twinLast := body.Stmts[len(body.Stmts)-1], twinBody.Stmts[len(twinBody.Stmts)-1]
	if last.Semicolon.IsValid() || !twinLast.Semicolon.IsValid() {
		t.Fatalf("separators = %s and %s, want only the explicit twin to keep one", last.Semicolon, twinLast.Semicolon)
	}
	if got, want := last.End().String(), "3:12"; got != want {
		t.Fatalf("implicit statement ends at %s, want %s", got, want)
	}
	if got, want := twinLast.End().String(), "3:13"; got != want {
		t.Fatalf("explicit statement ends at %s, want %s", got, want)
	}
}

func TestParseDeclarationBraceCloseRejectsInvalidSources(t *testing.T) {
	const unmatchedEOF = "reached EOF without matching `{` with `}`"
	tests := []struct {
		fixture string
		text    string
		line    uint
		col     uint
	}{
		{"invalid-231-double-brace-close.txt", unmatchedEOF, 1, 1},
		{"invalid-231-comment-glued-to-brace.txt", unmatchedEOF, 1, 1},
		{"invalid-231-brace-glued-to-assignment.txt", unmatchedEOF, 1, 1},
		{"invalid-231-redirect-glued-to-brace.txt", unmatchedEOF, 1, 1},
		{"invalid-231-word-glued-to-brace.txt", unmatchedEOF, 1, 1},
		{"invalid-231-unclosed-block.txt", unmatchedEOF, 1, 1},
		{"invalid-231-brace-glued-to-continued-value.txt", unmatchedEOF, 1, 1},
		{"invalid-231-separator-glued-to-comment.txt", unmatchedEOF, 1, 1},
		{"invalid-231-brace-in-substitution-value.txt", "`}` can only be used to close a block", 1, 19},
		// The first brace is a candidate, so the retry runs and reports the
		// stray brace at its own position instead of the unclosed block.
		{"invalid-231-stray-brace-after-block.txt", "`}` can only be used to close a block", 1, 15},
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

func TestParseDeclarationBraceCloseLeavesOtherErrorsUntouched(t *testing.T) {
	src := []byte("print x }\n")
	_, firstErr := parseTree(src, "other.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted a stray brace")
	}
	calls := 0
	_, err := parseDeclarationBraceCloseWithParser(src, "other.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
		calls++
		return nil, nil
	})
	if err != firstErr {
		t.Fatalf("error = %v, want the incoming error %v", err, firstErr)
	}
	if calls != 0 {
		t.Fatalf("parser called %d times, want 0", calls)
	}
}

func TestParseDeclarationBraceCloseFailsClosedWithoutRestoredBlock(t *testing.T) {
	src := []byte("{ local x=1 }\n")
	_, firstErr := parseTree(src, "closed.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the source")
	}
	// A retry whose tree does not hold the closed block at the expected
	// brace must return the original error rather than the foreign tree.
	_, err := parseDeclarationBraceCloseWithParser(src, "closed.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
		return parseTree([]byte("print x\n"), "closed.zsh")
	})
	if err != firstErr {
		t.Fatalf("error = %v, want the incoming error %v", err, firstErr)
	}
	// A retry that still fails reports its own error so later adapters and
	// the caller see the real position.
	retryErr := errors.New("retry failed")
	_, err = parseDeclarationBraceCloseWithParser(src, "closed.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
		return nil, retryErr
	})
	if err != retryErr {
		t.Fatalf("error = %v, want the retry error", err)
	}
}

func TestFindDeclarationBraceCloseSkipsInactiveBraces(t *testing.T) {
	tests := []struct {
		name string
		src  string
		ok   bool
		// space and brace are byte offsets when ok.
		space int
		brace int
	}{
		{"plain", "{ local x=1 }\n", true, 11, 12},
		{"brace glued to value", "{ local x=1}\n", false, 0, 0},
		{"brace followed by word", "{ local x=1 }else\n", false, 0, 0},
		{"brace followed by comment", "{ local x=1 }# c\n", false, 0, 0},
		{"double brace", "{ local x=1 }}\n", false, 0, 0},
		{"brace in single quotes", "{ local x='a }' }\n", true, 15, 16},
		{"brace in double quotes", "{ local x=\"a }\" }\n", true, 15, 16},
		{"brace in ansi-c quotes", "{ local x=$'a }' }\n", true, 16, 17},
		{"escaped brace", "{ local x=\\} }\n", true, 12, 13},
		{"brace in expansion", "{ local x=${y} }\n", true, 14, 15},
		{"brace in heredoc body", "{ local x=1 <<EOF\n}\nEOF\n}\n", false, 0, 0},
		{"separators in substitution", "{ local x=$(a | b; c) }\n", true, 21, 22},
		{"separators in legacy substitution", "{ local x=`a | b; c` }\n", true, 20, 21},
		{"unterminated substitution", "{ local x=$(a }\n", false, 0, 0},
		{"declaration as argument", "{ print local x=1 }\n", false, 0, 0},
		{"declaration in arithmetic", "{ (( local = 1 )) }\n", false, 0, 0},
		{"brace before seed", "{ local x=1 }\n{ print y\n", false, 0, 0},
		{"after do", "for f in a; do local x=$f }\n", true, 25, 26},
		{"after then", "if true; then local x=1 }\n", true, 23, 24},
		{"after negation", "{ ! local x=1 }\n", true, 13, 14},
		{"after time", "{ time local x=1 }\n", true, 16, 17},
		{"continued line before brace", "{ local x=1 \\\n}\n", true, 12, 14},
		{"brace glued to continued value", "{ local x=1\\\n}\n", false, 0, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seed := 0
			if test.name == "brace before seed" {
				seed = strings.LastIndex(test.src, "{")
			}
			space, brace, ok := findDeclarationBraceClose([]byte(test.src), seed)
			if ok != test.ok {
				t.Fatalf("ok = %t, want %t", ok, test.ok)
			}
			if !ok {
				return
			}
			if space != test.space || brace != test.brace {
				t.Fatalf("offsets = %d, %d, want %d, %d", space, brace, test.space, test.brace)
			}
			if test.src[brace] != '}' {
				t.Fatalf("byte at %d is %q, want a brace", brace, test.src[brace])
			}
		})
	}
}
