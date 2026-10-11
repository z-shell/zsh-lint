package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseGluedBraceGroups(t *testing.T) {
	const src = "# before\n{print ${value:-hi} $(print inner)}\nprint after\n"
	ResetStats()
	file, err := Parse(strings.NewReader(src), "glued.zsh")
	if err != nil {
		t.Fatal(err)
	}
	if got := ReadStats(); got != (Stats{TreeParses: 1}) {
		t.Fatalf("glued groups need no retry: %+v", got)
	}
	tree := file.AST()
	assertLiteralsMatchSource(t, tree, src)
	block, ok := tree.Stmts[0].Cmd.(*syntax.Block)
	if !ok || block.Lbrace.String() != "2:1" || block.Rbrace.String() != "2:35" || len(block.Stmts) != 1 {
		t.Fatalf("group structure or positions changed: %#v", tree.Stmts[0].Cmd)
	}
	if len(tree.Stmts[0].Comments) != 1 || tree.Stmts[0].Comments[0].Text != " before" || tree.Stmts[1].Pos().String() != "3:1" {
		t.Fatal("comment or following statement lost")
	}
	params, subs, calls := 0, 0, 0
	syntax.Walk(tree, func(node syntax.Node) bool {
		switch node.(type) {
		case *syntax.ParamExp:
			params++
		case *syntax.CmdSubst:
			subs++
		case *syntax.CallExpr:
			calls++
		}
		return true
	})
	if params != 1 || subs != 1 || calls != 3 {
		t.Fatalf("typed children hidden from Walk: params=%d subs=%d calls=%d", params, subs, calls)
	}
	invalid, err := os.ReadFile("testdata/invalid-276-unclosed-glued-group.txt")
	if err != nil {
		t.Fatal(err)
	}
	assertParseErrorAt(t, invalid, "reached EOF without matching `{` with `}`", 3, 1)
}
