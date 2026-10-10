package syntax

import (
	"strings"
	"testing"
)

// Numeric parameter assignments and integer/float declaration words are Zsh
// assignment syntax, not ordinary command arguments (zsh-lint #612).
// https://zsh.sourceforge.io/Doc/Release/Parameters.html
// https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html
func TestZshNumericAssignments(t *testing.T) {
	for _, tc := range []struct {
		src, name string
		append    bool
		index     bool
		args      int
	}{
		{src: `0=${ZERO:-${(%):-%N}}`, name: "0"},
		{src: `1=$value`, name: "1"},
		{src: `12="a b"`, name: "12"},
		{src: `0=`, name: "0"},
		{src: `1+=suffix`, name: "1", append: true},
		{src: `1[1]=x`, name: "1", index: true},
		{src: `0=source print ok`, name: "0", args: 2},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(tc.src+"\nprint after\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			call, ok := f.Stmts[0].Cmd.(*CallExpr)
			if !ok || len(call.Assigns) != 1 || len(call.Args) != tc.args {
				t.Fatalf("want one assignment and %d args, got %#v", tc.args, f.Stmts[0].Cmd)
			}
			as := call.Assigns[0]
			if as.Name.Value != tc.name || as.Name.Pos().Offset() != 0 || as.Name.End().Offset() != uint(len(tc.name)) || as.Append != tc.append || (as.Index != nil) != tc.index {
				t.Fatalf("assignment name, positions or kind changed: %#v", as)
			}
			end := len(tc.src)
			if tc.args > 0 {
				end = strings.Index(tc.src, " print")
			}
			if as.End().Offset() != uint(end) {
				t.Fatalf("assignment end = %v", as.End())
			}
			if len(f.Stmts) != 2 || f.Stmts[1].Pos().Offset() != uint(len(tc.src)+1) {
				t.Fatal("following statement lost or moved")
			}
		})
	}
	for _, src := range []string{`print 0=$value`, `"0=source"`, `0\=source`} {
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
		if err != nil {
			t.Fatal(err)
		}
		if len(f.Stmts[0].Cmd.(*CallExpr).Assigns) != 0 {
			t.Fatalf("ordinary word %q became an assignment", src)
		}
	}
}

func TestZshIntegerFloatDeclarations(t *testing.T) {
	for _, name := range []string{"integer", "float"} {
		for _, tail := range []string{` n=$value`, ` -g n=1 m=2`, ` n=(1 2)`, ` n`, ` 0=1`} {
			src := name + tail
			t.Run(src, func(t *testing.T) {
				f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
				if err != nil {
					t.Fatal(err)
				}
				decl, ok := f.Stmts[0].Cmd.(*DeclClause)
				if !ok || decl.Variant.Value != name || len(decl.Args) == 0 {
					t.Fatalf("want %s declaration, got %#v", name, f.Stmts[0].Cmd)
				}
				as := decl.Args[len(decl.Args)-1]
				if as.Name == nil || as.Name.Pos().Offset() != uint(strings.LastIndex(src, as.Name.Value)) {
					t.Fatalf("declaration name lost or moved: %#v", as)
				}
				if strings.Contains(tail, "=(") && (as.Array == nil || len(as.Array.Elems) != 2) {
					t.Fatal("array declaration lost its elements")
				}
				if strings.Contains(tail, "$value") {
					pe, ok := as.Value.Parts[0].(*ParamExp)
					if !ok || pe.Param.Value != "value" || pe.Pos().Offset() != uint(strings.Index(src, "$value")) {
						t.Fatal("declaration value lost or moved")
					}
				}
			})
		}
		for _, tail := range []string{`() { :; }`, ` () { :; }`} {
			f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(name+tail), "")
			if err != nil {
				t.Fatal(err)
			}
			fn, ok := f.Stmts[0].Cmd.(*FuncDecl)
			if !ok || fn.Name.Value != name {
				t.Fatalf("function named %s became a declaration", name)
			}
			if fn.RsrvWord || !fn.Parens {
				t.Fatalf("function %q: keyword or parentheses metadata changed", name+tail)
			}
		}
		for _, src := range []string{`"` + name + `" n=$value`, `\` + name + ` n=$value`, `builtin ` + name + ` n=$value`, `print ` + name + ` n=$value`} {
			f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := f.Stmts[0].Cmd.(*CallExpr); !ok {
				t.Fatalf("ordinary call %q became a declaration", src)
			}
		}
	}
}

func TestZshNumericDeclarationBraceTermination(t *testing.T) {
	for _, name := range []string{"integer", "float"} {
		for _, src := range []string{"{ " + name + " n=1 }", `: "$({ ` + name + ` n=1 })"`, "{ " + name + " }", name + ` "}"`, name + ` \}`} {
			f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatalf("%q: %v", src, err)
			}
			count := 0
			Walk(f, func(n Node) bool {
				if d, ok := n.(*DeclClause); ok {
					count++
					if d.Variant.Value != name {
						t.Fatalf("%q: declaration name lost", src)
					}
				}
				return true
			})
			if count != 1 {
				t.Fatalf("%q: got %d declarations, want one", src, count)
			}
		}
		for _, src := range []string{name + " }", name + " n=(one", name + " n=$(if true; then)"} {
			if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), ""); err == nil {
				t.Fatalf("invalid declaration %q accepted", src)
			}
		}
	}
}

func TestZshAssignmentsDeclarationsDialectBoundary(t *testing.T) {
	for _, lang := range []LangVariant{LangPOSIX, LangBash, LangBats, LangMirBSDKorn} {
		for _, src := range []string{`0=$value`, `1+=suffix`, `integer n=$value`, `float n=$value`} {
			f, err := NewParser(Variant(lang)).Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatalf("%s %q: %v", lang, src, err)
			}
			call, ok := f.Stmts[0].Cmd.(*CallExpr)
			if !ok || len(call.Assigns) != 0 {
				t.Fatalf("%s %q: non-Zsh tree changed", lang, src)
			}
		}
	}
}
