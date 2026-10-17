package parse

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

func TestDeclarationBraceCloseRetainsLegacyNameref(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-649-nameref-literal-brace.txt")
	if err != nil {
		t.Fatal(err)
	}
	// Extending the declaration stop to nameref would accept this source,
	// although Zsh reads its assignment-looking argument as an ordinary word.
	assertParseErrorAt(t, src, "reached EOF without matching `{` with `}`", 1, 1)
	ResetStats()
	if _, err := Parse(strings.NewReader("{ nameref x=y }\n"), "legacy.zsh"); err != nil {
		t.Fatal(err)
	}
	if got := ReadStats(); got != (Stats{TreeParses: 2, MaxAdapterDepth: 1}) {
		t.Fatalf("legacy adapter coverage lost: %+v", got)
	}
}

func TestDeclarationAlternateBodyPreservesSource(t *testing.T) {
	for _, body := range []string{
		"if [[ -n x ]] { local u=1 }",
		"for i in 1; { local u=1 }",
	} {
		t.Run(body, func(t *testing.T) {
			src := "# zsh-lint disable=all\n" + body + " # trailing\nprint after\n"
			ResetStats()
			file, err := Parse(strings.NewReader(src), "issue-649.zsh")
			if err != nil {
				t.Fatal(err)
			}
			if got := ReadStats(); got != (Stats{TreeParses: 1}) {
				t.Fatalf("declaration body needed a retry: %+v", got)
			}
			if strings.Join(file.Lines(), "\n")+"\n" != src {
				t.Fatal("original suppression lines changed")
			}
			tree := file.AST()
			if len(tree.Stmts) != 2 || tree.Stmts[1].Pos().String() != "3:1" {
				t.Fatal("following statement lost or moved")
			}
			var stmts []*syntax.Stmt
			var open, close syntax.Pos
			switch cmd := tree.Stmts[0].Cmd.(type) {
			case *syntax.IfClause:
				stmts, open, close = cmd.Then, cmd.ThenPos, cmd.FiPos
			case *syntax.ForClause:
				stmts, open, close = cmd.Do, cmd.DoPos, cmd.DonePos
			default:
				t.Fatalf("alternate body lost its typed owner: %T", cmd)
			}
			if len(stmts) != 1 || src[open.Offset()] != '{' || src[close.Offset()] != '}' || close.Offset() != uint(strings.Index(src, "}")) {
				t.Fatal("body statements or brace positions changed")
			}
			decl, ok := stmts[0].Cmd.(*syntax.DeclClause)
			if !ok || decl.Variant.Value != "local" || len(decl.Args) != 1 || decl.Args[0].Name.Value != "u" {
				t.Fatalf("body declaration changed: %#v", stmts[0].Cmd)
			}
			if stmts[0].Semicolon.IsValid() || stmts[0].End().Offset() != uint(strings.Index(src, "u=1")+3) {
				t.Fatal("statement gained a separator or swallowed its brace")
			}
			assertLiteralsMatchSource(t, tree, src)
			var comments []string
			syntax.Walk(tree, func(n syntax.Node) bool {
				if c, ok := n.(*syntax.Comment); ok {
					comments = append(comments, c.Pos().String()+c.Text)
				}
				return true
			})
			want := fmt.Sprintf("[1:1 zsh-lint disable=all 2:%d trailing]", len(body)+2)
			if fmt.Sprint(comments) != want {
				t.Fatalf("comments = %v, want %s", comments, want)
			}
		})
	}
}

// The fork's declaration bodies must compose with every adapter in both
// orders, including braces read inside another adapter's retry.
func TestDeclarationBraceCloseComposesWithAdapters(t *testing.T) {
	for name, snippet := range adapterSnippets {
		for _, declaration := range []string{
			"{ typeset -g C=1 }",
			"if [[ -n x ]] { local u=1 }",
			"for i in 1; { let u=1 }",
		} {
			for _, src := range []string{declaration + "\n" + snippet.source, snippet.source + "\n" + declaration} {
				t.Run(name+src, func(t *testing.T) {
					if err := parseString(t, src+"\n"); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}
