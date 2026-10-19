package syntax

import (
	"bytes"
	"strings"
	"testing"
)

// A nested flagged subscript must not consume the assignment target's closing
// bracket (zsh-lint #735).
// https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Element-Assignment
func TestZshNestedAssignmentTarget(t *testing.T) {
	const target = `a[${b[(ie)${c[d]}]}]`
	for _, rhs := range []string{`()`, `(one two)`, `value`, `"$value"`} {
		src := target + "=" + rhs + "\nprint after\n"
		t.Run(rhs, func(t *testing.T) {
			f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatal(err)
			}
			call, ok := f.Stmts[0].Cmd.(*CallExpr)
			if !ok || len(call.Assigns) != 1 || len(call.Args) != 0 {
				t.Fatalf("want one assignment, got %#v", f.Stmts[0].Cmd)
			}
			as := call.Assigns[0]
			if as.Name.Value != "a" || as.Index.Pos().Offset() != 2 || as.Index.End().Offset() != uint(len(target)-1) {
				t.Fatalf("assignment index or positions changed: %#v", as)
			}
			var names []string
			Walk(as.Index, func(n Node) bool {
				if pe, ok := n.(*ParamExp); ok {
					names = append(names, pe.Param.Value)
					start, end := pe.Pos().Offset(), pe.End().Offset()
					if start >= end || end > uint(len(src)) || !strings.HasPrefix(src[start:end], "${"+pe.Param.Value+"[") {
						t.Fatalf("expansion %s lost source range: %v to %v", pe.Param.Value, pe.Pos(), pe.End())
					}
				}
				return true
			})
			if strings.Join(names, ",") != "b,c" {
				t.Fatalf("typed nested children = %v, want b,c", names)
			}
			if len(f.Stmts) != 2 || f.Stmts[1].Pos().Offset() != uint(strings.Index(src, "print after")) {
				t.Fatal("following statement lost or moved")
			}
			var printed bytes.Buffer
			if err := NewPrinter().Print(&printed, f); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(printed.String(), target+"="+rhs+"\n") {
				t.Fatalf("assignment changed when printed: %s", &printed)
			}
			if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(printed.String()), ""); err != nil {
				t.Fatalf("printed source rejected: %v", err)
			}
		})
	}
}
