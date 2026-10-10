package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestArithmeticCommandInSubstitution(t *testing.T) {
	const src = `output="$((( ${+flags[nocd]} == 0 )) && builtin cd -q "$dir"; eval ${flags[eval]#!})"`
	f, err := Parse(strings.NewReader(src), "annex.zsh")
	if err != nil {
		t.Fatal(err)
	}
	word := f.tree.Stmts[0].Cmd.(*syntax.CallExpr).Assigns[0].Value
	cs := word.Parts[0].(*syntax.DblQuoted).Parts[0].(*syntax.CmdSubst)
	and := cs.Stmts[0].Cmd.(*syntax.BinaryCmd)
	if and.Op != syntax.AndStmt {
		t.Fatalf("want && sublist, got %v", and.Op)
	}
	if _, ok := and.X.Cmd.(*syntax.ArithmCmd); !ok {
		t.Fatalf("want arithmetic command, got %T", and.X.Cmd)
	}
	if len(cs.Stmts) != 2 || cs.Stmts[1].Cmd.(*syntax.CallExpr).Args[0].Lit() != "eval" {
		t.Fatal("command substitution lost its eval command")
	}
	if cs.Left.Offset() != uint(strings.Index(src, "$(")) || cs.Right.Offset() != uint(strings.LastIndex(src, ")")) {
		t.Fatal("command substitution positions moved")
	}
}

func TestFlagPatternRepairRetainsAssignment(t *testing.T) {
	const src = `m[(i)a[bc]]=1 other=2 print ok`
	f, err := Parse(strings.NewReader(src), "assignment.zsh")
	if err != nil {
		t.Fatal(err)
	}
	call := f.tree.Stmts[0].Cmd.(*syntax.CallExpr)
	if len(call.Assigns) != 2 || call.Assigns[0].Name.Value != "m" || call.Assigns[1].Name.Value != "other" {
		t.Fatal("flag-pattern repair lost an assignment")
	}
}
