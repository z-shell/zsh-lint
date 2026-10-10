package syntax

import (
	"bytes"
	"strings"
	"testing"
)

// Zsh expands arithmetic words before resolving names (zsh-lint #612).
// https://zsh.sourceforge.io/Doc/Release/Arithmetic-Evaluation.html#Arithmetic-Evaluation
func TestZshDynamicArithmetic(t *testing.T) {
	for _, expr := range []string{
		`${target} = 2`, `${target}[1] = 2`, `${target}[i + 1] += 2`,
		`$target = 2`, `${target}[1] + 2`,
		`a_$suffix[1] = 2`,
		`"${target}"[1] = 2`,
		`++a_$suffix`, `--${target}`, `a_$suffix++`, `${target}[1]--`,
		`${${(%):-$1%$m(l.x.y)}[-1]} = m`,
	} {
		t.Run(expr, func(t *testing.T) {
			src := "(( " + expr + " ))\nprint after\n"
			f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatal(err)
			}
			if len(f.Stmts) != 2 || f.Stmts[1].Pos().Offset() != uint(strings.Index(src, "print")) {
				t.Fatal("following statement lost or moved")
			}
			cmd, ok := f.Stmts[0].Cmd.(*ArithmCmd)
			if !ok || cmd.X.Pos().Offset() != 3 || cmd.X.End().Offset() != uint(3+len(expr)) {
				t.Fatalf("arithmetic expression or positions changed: %#v", f.Stmts[0].Cmd)
			}
			names := 0
			Walk(cmd.X, func(n Node) bool {
				if pe, ok := n.(*ParamExp); ok && pe.Dollar.IsValid() {
					names++
				}
				return true
			})
			if names == 0 {
				t.Fatal("dynamic name lost to walker")
			}
			var printed bytes.Buffer
			if err := NewPrinter().Print(&printed, f); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(printed.String(), expr) {
				t.Fatalf("arithmetic source changed: %s", &printed)
			}
			if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(printed.String()), ""); err != nil {
				t.Fatalf("printed source rejected: %v", err)
			}
		})
	}
}

func TestZshDynamicArithmeticDialectBoundary(t *testing.T) {
	for _, lang := range []LangVariant{LangBash, LangBats, LangMirBSDKorn} {
		for _, expr := range []string{`${target} = 2`, `${target}[1] = 2`, `++a_$suffix`, `a_$suffix++`} {
			if _, err := NewParser(Variant(lang)).Parse(strings.NewReader("(( "+expr+" ))"), ""); err == nil {
				t.Fatalf("%s accepted dynamic target %q", lang, expr)
			}
		}
	}
	for _, expr := range []string{`1 = 2`, `1++`, `++`, `${target}[1`, `${target} =`, `++${target} +`, `${target}[1 +] = 2`} {
		if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader("(( "+expr+" ))"), ""); err == nil {
			t.Fatalf("invalid expression accepted: %q", expr)
		}
	}
}

func TestZshDynamicArithmeticSubstitution(t *testing.T) {
	for _, src := range []string{`print $(( ${target} = 2 ))`, `print $(( $(print target) = 2 ))`, `print $(( ++a_$suffix ))`} {
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
		if err != nil {
			t.Fatal(err)
		}
		call := f.Stmts[0].Cmd.(*CallExpr)
		if _, ok := call.Args[1].Parts[0].(*ArithmExp); !ok {
			t.Fatalf("arithmetic substitution changed kind: %#v", call.Args[1])
		}
	}
}
