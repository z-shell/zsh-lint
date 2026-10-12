package parse

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
	"mvdan.cc/sh/v3/syntax/typedjson"
)

func TestFullStringReplacementUsesTypedChildrenWithoutRetry(t *testing.T) {
	src := "print ${value:/$pattern/${replacement:-$(print b)}}\nprint after\n"
	ResetStats()
	f, err := Parse(strings.NewReader(src), "replacement.zsh")
	if err != nil {
		t.Fatal(err)
	}
	if got := ReadStats(); got != (Stats{TreeParses: 1}) {
		t.Fatalf("replacement required a retry: %+v", got)
	}
	pe := f.AST().Stmts[0].Cmd.(*syntax.CallExpr).Args[1].Parts[0].(*syntax.ParamExp)
	if pe.Repl == nil || !pe.Repl.Whole || pe.Repl.All || pe.Slice != nil || pe.Exp != nil || len(pe.Modifiers) != 0 {
		t.Fatalf("replacement meaning lost: %#v", pe)
	}
	for _, child := range []syntax.Node{pe.Repl.Orig, pe.Repl.With} {
		text := src[child.Pos().Offset():child.End().Offset()]
		if text != "$pattern" && text != "${replacement:-$(print b)}" {
			t.Fatalf("replacement child moved: %q", text)
		}
	}
	var names []string
	commands := 0
	syntax.Walk(f.AST(), func(n syntax.Node) bool {
		if pe, ok := n.(*syntax.ParamExp); ok && pe.Param != nil {
			names = append(names, pe.Param.Value)
		}
		if _, ok := n.(*syntax.CmdSubst); ok {
			commands++
		}
		return true
	})
	if !reflect.DeepEqual(names, []string{"value", "pattern", "replacement"}) || commands != 1 {
		t.Fatalf("replacement children hidden: %v, %d substitutions", names, commands)
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
		t.Fatalf("typed JSON lost the replacement tree: %v", err)
	}
}
