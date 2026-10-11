package syntax

import (
	"bytes"
	"strings"
	"testing"
)

// The length prefix measures the result of the expansion, including its suffix.
// https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
func TestZshLengthOperators(t *testing.T) {
	for _, expr := range []string{
		`${#x:t}`, `${#x:h}`, `${#x:h:t}`, `${#x:h2}`, `${#x:t2}`,
		`${#BUFFER:$highlight_start_index}`, `${#x:1}`, `${#x:1:2}`,
		`${#*:#0}`, `${#@:#0}`, `${#${x}:-}`, `${#$(print x):-}`,
		`${#a[@]:#x}`, `${#a//x/y}`, `${#a:-y}`, `${#a:+y}`,
		`${#a:=y}`, `${#a#x}`, `${#a%y}`,
	} {
		t.Run(expr, func(t *testing.T) {
			src := "print " + expr + "\nprint after\n"
			f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatal(err)
			}
			pe := f.Stmts[0].Cmd.(*CallExpr).Args[1].Parts[0].(*ParamExp)
			if !pe.Length || pe.Pos().Offset() != 6 || pe.End().Offset() != uint(6+len(expr)) {
				t.Fatalf("length or original positions lost: %#v", pe)
			}
			if len(f.Stmts) != 2 || f.Stmts[1].Pos().Offset() != uint(strings.Index(src, "print after")) {
				t.Fatal("following statement lost or moved")
			}
			if strings.Contains(expr, ":t") || strings.Contains(expr, ":h") {
				if len(pe.Modifiers) == 0 || pe.Modifiers[0].ValuePos.Offset() != uint(strings.Index(src, ":")+1) {
					t.Fatalf("modifier or its original position lost: %#v", pe.Modifiers)
				}
			}
			if expr == `${#BUFFER:$highlight_start_index}` {
				if pe.Slice == nil || pe.Slice.Offset.(*Word).Parts[0].(*ParamExp).Param.Value != "highlight_start_index" {
					t.Fatal("slice offset expansion is not a typed child")
				}
			}
			if strings.Contains(expr, "${#${") && pe.NestedParam.(*ParamExp).Param.Value != "x" {
				t.Fatal("nested parameter lost")
			}
			var printed bytes.Buffer
			if err := NewPrinter().Print(&printed, f); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(printed.String(), expr) {
				t.Fatalf("printed expression changed: %s", &printed)
			}
			if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(printed.String()), ""); err != nil {
				t.Fatalf("printed source rejected: %v", err)
			}
		})
	}
}

func TestZshLengthOperatorBoundaries(t *testing.T) {
	for _, lang := range []LangVariant{LangBash, LangBats, LangMirBSDKorn, LangPOSIX} {
		for _, expr := range []string{`${#x:t}`, `${#x:1}`, `${#*:#0}`, `${#a:-y}`} {
			if _, err := NewParser(Variant(lang)).Parse(strings.NewReader("print "+expr+"\n"), ""); err == nil {
				t.Fatalf("%s accepted Zsh length suffix %q", lang, expr)
			}
		}
	}
	for _, expr := range []string{`${#x:x}`, `${#x:ht}`, `${#x:hx}`, `${#x:h:t:x}`, `${#x:h:}`, `${#x:1:-1}`, `${#x:t`, `${#a[@]:#x`, `${+a:#x}`, `${#"$(print x)":-}`, `${#"${x}":h}`} {
		if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader("print "+expr+"\n"), ""); err == nil {
			t.Fatalf("malformed or unsupported expression accepted: %q", expr)
		}
	}
}
