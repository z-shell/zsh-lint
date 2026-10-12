package syntax

import (
	"bytes"
	"strings"
	"testing"
	"testing/iotest"
)

// A colon before replacement requires a match of the complete parameter value.
// https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
func TestZshFullStringReplacement(t *testing.T) {
	for _, expr := range []string{
		`${x:/a/b}`, `${x:/a}`, `${x:/a/}`, `${x:/*/b}`, `${x://a/b}`,
		`${x:/a\/*/b}`, `${x:/$pattern/$replacement}`,
		`${x:/a/${y:-b}}`, `${${x}:/a/b}`, `${x[1]:/a/b}`,
		`${#x:/a/b}`, `${(U)x:/a/b}`, `${x:/(#b)(*)/${match[1]}}`,
		`${x:/}`, `${x://}`, `${x:/$(print a)/$(print b)}`,
	} {
		t.Run(expr, func(t *testing.T) {
			src := "print " + expr + "\nprint after\n"
			for _, slow := range []bool{false, true} {
				input := strings.NewReader(src)
				var f *File
				var err error
				p := NewParser(Variant(LangZsh))
				if slow {
					f, err = p.Parse(iotest.OneByteReader(input), "")
				} else {
					f, err = p.Parse(input, "")
				}
				if err != nil {
					t.Fatal(err)
				}
				pe := f.Stmts[0].Cmd.(*CallExpr).Args[1].Parts[0].(*ParamExp)
				if pe.Repl == nil || !pe.Repl.Whole || pe.Repl.All != strings.Contains(expr, "://") || pe.Slice != nil || pe.Pos().Offset() != 6 || pe.End().Offset() != uint(6+len(expr)) {
					t.Fatalf("replacement or original positions lost: %#v", pe)
				}
				if len(f.Stmts) != 2 || f.Stmts[1].Pos().Offset() != uint(strings.Index(src, "print after")) {
					t.Fatal("following statement lost or moved")
				}
				var printed bytes.Buffer
				if err := NewPrinter().Print(&printed, f); err != nil {
					t.Fatal(err)
				}
				// An omitted replacement delimiter is printed explicitly.
				want := expr
				if expr == `${x:/a}` {
					want = `${x:/a/}`
				} else if expr == `${x:/}` {
					want = `${x://}`
				} else if expr == `${x://}` {
					want = `${x:///}`
				}
				if !strings.Contains(printed.String(), want) {
					t.Fatalf("printed replacement changed: %s", &printed)
				}
				if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(printed.String()), ""); err != nil {
					t.Fatalf("printed source rejected: %v", err)
				}
			}
		})
	}
}

func TestZshFullStringReplacementBoundaries(t *testing.T) {
	for _, lang := range []LangVariant{LangBash, LangBats, LangMirBSDKorn, LangPOSIX} {
		if _, err := NewParser(Variant(lang)).Parse(strings.NewReader("print ${x:/a/b}\n"), ""); err == nil {
			t.Fatalf("%s accepted Zsh full-string replacement", lang)
		}
	}
	for _, expr := range []string{`${x:/a/b`, `${x:/a/${y}`, `${x:/a/$(}`} {
		if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader("print "+expr+"\n"), ""); err == nil {
			t.Fatalf("malformed expression accepted: %q", expr)
		}
	}
	for _, expr := range []string{`${x/a/b}`, `${x//a/b}`, `${x:1:2}`, `${x:-b}`, `${x:t}`} {
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader("print "+expr+"\n"), "")
		if err != nil {
			t.Fatal(err)
		}
		pe := f.Stmts[0].Cmd.(*CallExpr).Args[1].Parts[0].(*ParamExp)
		if pe.Repl != nil && pe.Repl.Whole {
			t.Fatalf("ordinary operator became a full-string replacement: %s", expr)
		}
	}
}
