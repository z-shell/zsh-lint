package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestNumericDeclarationsBeforeBraceClose(t *testing.T) {
	for _, word := range []string{"integer", "float"} {
		src := "{ " + word + " n=$value }\nprint after\n"
		ResetStats()
		file, err := Parse(strings.NewReader(src), "numeric.zsh")
		if err != nil {
			t.Fatal(err)
		}
		if got := ReadStats(); got != (Stats{TreeParses: 1}) {
			t.Fatalf("numeric declaration needed an adapter: %+v", got)
		}
		tree := file.AST()
		block, ok := tree.Stmts[0].Cmd.(*syntax.Block)
		if !ok || len(block.Stmts) != 1 {
			t.Fatalf("block lost: %#v", tree.Stmts[0].Cmd)
		}
		stmt := block.Stmts[0]
		decl, ok := stmt.Cmd.(*syntax.DeclClause)
		if !ok || decl.Variant.Value != word || len(decl.Args) != 1 || decl.Args[0].Name.Value != "n" {
			t.Fatalf("want %s declaration with one name, got %#v", word, stmt.Cmd)
		}
		pe, ok := decl.Args[0].Value.Parts[0].(*syntax.ParamExp)
		if !ok || pe.Param.Value != "value" || pe.Pos().Offset() != uint(strings.Index(src, "$value")) {
			t.Fatal("declaration expansion lost or moved")
		}
		if stmt.Semicolon.IsValid() || block.Rbrace.Offset() != uint(strings.Index(src, "}")) || len(tree.Stmts) != 2 || tree.Stmts[1].Pos().Offset() != uint(strings.Index(src, "print")) {
			t.Fatal("brace adaptation moved a position or retained a synthetic separator")
		}
	}
}
