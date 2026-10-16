package parse

import (
	"os"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

func TestDynamicArithmeticTargets(t *testing.T) {
	src := "(( ${target}[i + 1] = 2 ))\n(( ++a_$suffix ))\n"
	ResetStats()
	file, err := Parse(strings.NewReader(src), "dynamic.zsh")
	if err != nil {
		t.Fatal(err)
	}
	if got := ReadStats(); got != (Stats{TreeParses: 1}) {
		t.Fatalf("dynamic arithmetic needed an adapter: %+v", got)
	}
	assign := file.AST().Stmts[0].Cmd.(*syntax.ArithmCmd).X.(*syntax.BinaryArithm)
	target := assign.X.(*syntax.Word).Parts[0].(*syntax.ParamExp)
	if target.ArithmName == nil || target.Param != nil || target.Index == nil || target.Dollar.IsValid() {
		t.Fatalf("dynamic subscript changed into a parameter value: %#v", target)
	}
	name := target.ArithmName.Parts[0].(*syntax.ParamExp)
	if name.Param.Value != "target" || name.Pos().Offset() != 3 || target.End().Offset() != uint(strings.Index(src, " =")) {
		t.Fatal("expanded name or indexed target lost or moved")
	}
	index, ok := target.Index.(*syntax.BinaryArithm)
	if !ok || index.Op != syntax.Add || index.Pos().Offset() != uint(strings.Index(src, "i +")) {
		t.Fatalf("index is not typed arithmetic: %#v", target.Index)
	}
}

func TestDynamicArithmeticUnclosed(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-612-dynamic-arithmetic-unclosed.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(strings.NewReader(string(src)), "invalid.zsh"); err == nil || !strings.Contains(err.Error(), "without matching") {
		t.Fatalf("unclosed arithmetic accepted or wrong error: %v", err)
	}
}
