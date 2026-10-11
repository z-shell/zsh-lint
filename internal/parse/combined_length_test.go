package parse

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
	"mvdan.cc/sh/v3/syntax/typedjson"
)

func TestCombinedLengthUsesTypedChildrenWithoutRetry(t *testing.T) {
	src := "print ${#x:h:t} ${#BUFFER:$offset} ${#*:#0} ${#${x}:-}\nprint after\n"
	ResetStats()
	f, err := Parse(strings.NewReader(src), "length.zsh")
	if err != nil {
		t.Fatal(err)
	}
	if got := ReadStats(); got != (Stats{TreeParses: 1}) {
		t.Fatalf("combined length required a retry: %+v", got)
	}
	args := f.AST().Stmts[0].Cmd.(*syntax.CallExpr).Args
	mod := args[1].Parts[0].(*syntax.ParamExp)
	slice := args[2].Parts[0].(*syntax.ParamExp)
	special := args[3].Parts[0].(*syntax.ParamExp)
	nested := args[4].Parts[0].(*syntax.ParamExp)
	if !mod.Length || len(mod.Modifiers) != 2 || mod.Modifiers[0].Value != "h" || mod.Modifiers[1].Value != "t" ||
		!slice.Length || slice.Slice == nil || !special.Length || special.Param.Value != "*" || special.Exp.Op != syntax.MatchEmpty ||
		!nested.Length || nested.NestedParam.(*syntax.ParamExp).Param.Value != "x" || nested.Exp.Op != syntax.DefaultUnsetOrNull {
		t.Fatal("length prefix or suffix meanings lost")
	}
	var names []string
	syntax.Walk(f.AST(), func(n syntax.Node) bool {
		if pe, ok := n.(*syntax.ParamExp); ok && pe.Param != nil {
			names = append(names, pe.Param.Value)
		}
		return true
	})
	if !reflect.DeepEqual(names, []string{"x", "BUFFER", "offset", "*", "x"}) {
		t.Fatalf("nested expansions hidden from traversal: %v", names)
	}
	if f.AST().Stmts[1].Pos().Offset() != uint(strings.Index(src, "print after")) {
		t.Fatal("following statement moved")
	}
	var encoded bytes.Buffer
	if err := typedjson.Encode(&encoded, f.AST()); err != nil {
		t.Fatal(err)
	}
	decoded, err := typedjson.Decode(&encoded)
	if err != nil || !reflect.DeepEqual(decoded, f.AST()) {
		t.Fatalf("typed JSON lost the combined expansion tree: %v", err)
	}
}
