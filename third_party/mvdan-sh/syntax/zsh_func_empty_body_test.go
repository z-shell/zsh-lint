package syntax

import (
	"strings"
	"testing"
)

func TestZshFunctionEmptyBodyValidRows(t *testing.T) {
	t.Parallel()

	validRows := []string{
		"function a",
		"function a b c",
		"function f print hi",
		"function a;",
		"function a b;",
		"function a b c;\n\n",
		"function",
		"function ;",
		"function a ()",
		"function a () ;",
		"{ function a; }",
		"{ function a }",
		"x=$(function a)",
		"(function a)",
		"if true; then function a; fi",
		"a() { function b; }",
		"f () ; function a",
		"function a; function b",
		"function a { }\nfunction b",
		"function a; function b; print z\na; b",
		"a () ; b () ; print z\na; b",
		// A case terminator closes a bodyless definition in a case item.
		"case x in x) function a ;; esac",
		"case x in x) function a ;& y) : ;; esac",
		// A comment after the separator does not become the body; the
		// closer after it ends the definition.
		"{ function a; # c\n}",
		"{ function a # c\n}",
		"function a;\n# c\n",
		"(function a; # c\n)",
	}

	p := NewParser(Variant(LangZsh))
	for _, src := range validRows {
		t.Run(src, func(t *testing.T) {
			f, err := p.Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", src, err)
			}
			// Verify printer roundtrip for bodyless definitions
			if len(f.Stmts) > 0 {
				if fn, ok := f.Stmts[0].Cmd.(*FuncDecl); ok && fn.Body == nil {
					var sb strings.Builder
					if err := NewPrinter().Print(&sb, f); err != nil {
						t.Fatalf("Print(%q) failed: %v", src, err)
					}
					f2, err := p.Parse(strings.NewReader(sb.String()), "")
					if err != nil {
						t.Fatalf("Re-parse of printed %q (printed: %q) failed: %v", src, sb.String(), err)
					}
					fn2, ok := f2.Stmts[0].Cmd.(*FuncDecl)
					if !ok || fn2.Body != nil {
						t.Fatalf("Re-parsed AST is not bodyless FuncDecl: %T", f2.Stmts[0].Cmd)
					}
				}
			}
		})
	}
}

func TestZshFunctionEmptyBodyMustRejectRows(t *testing.T) {
	t.Parallel()

	mustReject := []struct {
		src     string
		wantErr string
	}{
		{"a () ;", "1:1: `foo()` must be followed by a statement"},
		{"function a;;", "1:1: `foo()` must be followed by a statement"},
		{"function a &", "1:1: `foo()` must be followed by a statement"},
		{"function a; }", "1:13: `}` can only be used to close a block"},
		{"function a\n)", "2:1: statements must be separated by &, ; or a newline"},
		{"a () { : }\na ()", "2:1: `foo()` must be followed by a statement"},
	}

	p := NewParser(Variant(LangZsh))
	for _, tc := range mustReject {
		t.Run(tc.src, func(t *testing.T) {
			_, err := p.Parse(strings.NewReader(tc.src), "")
			if err == nil {
				t.Fatalf("Parse(%q) unexpectedly succeeded", tc.src)
			}
			if err.Error() != tc.wantErr {
				t.Errorf("Parse(%q) error mismatch:\n got:  %s\n want: %s", tc.src, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestZshFunctionEmptyBodyBashGated(t *testing.T) {
	t.Parallel()

	// Proves that bodyless function definitions are rejected in Bash
	bashInputs := []struct {
		src     string
		wantErr string
	}{
		{"function a;", "1:1: `foo()` must be followed by a statement"},
		{"function a", "1:1: `foo()` must be followed by a statement"},
		{"function a ()", "1:1: `foo()` must be followed by a statement"},
		{"function a () ;", "1:1: `foo()` must be followed by a statement"},
		{"function", "1:1: `function` must be followed by a name"},
	}

	p := NewParser(Variant(LangBash))
	for _, tc := range bashInputs {
		t.Run(tc.src, func(t *testing.T) {
			_, err := p.Parse(strings.NewReader(tc.src), "")
			if err == nil {
				t.Fatalf("Parse(%q) unexpectedly succeeded in LangBash", tc.src)
			}
			if err.Error() != tc.wantErr {
				t.Errorf("Parse(%q) error in LangBash:\n got:  %s\n want: %s", tc.src, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestZshFunctionEmptyBodyTreeShape(t *testing.T) {
	t.Parallel()

	p := NewParser(Variant(LangZsh))

	// Tree shape test asserting Body == nil for `function a`
	{
		f, err := p.Parse(strings.NewReader("function a"), "")
		if err != nil {
			t.Fatalf("Parse(function a) failed: %v", err)
		}
		if len(f.Stmts) != 1 {
			t.Fatalf("expected 1 statement, got %d", len(f.Stmts))
		}
		fn, ok := f.Stmts[0].Cmd.(*FuncDecl)
		if !ok {
			t.Fatalf("expected *FuncDecl, got %T", f.Stmts[0].Cmd)
		}
		if fn.Body != nil {
			t.Errorf("expected Body == nil, got %v", fn.Body)
		}
		if fn.Name == nil || fn.Name.Value != "a" {
			t.Errorf("expected Name 'a', got %v", fn.Name)
		}
		// End() should be at end of last name
		if fn.End() != fn.Name.End() {
			t.Errorf("expected End() == %v, got %v", fn.Name.End(), fn.End())
		}
	}

	// End() of a bodyless definition: past the "()" when Parens is set, else
	// at the end of the last name.
	for _, tc := range []struct {
		src  string
		want string
	}{
		{"function a ()", "1:14"},
		{"function a b c", "1:15"},
		{"function a b c ()", "1:18"},
		{"function", "1:9"},
	} {
		f, err := p.Parse(strings.NewReader(tc.src), "")
		if err != nil {
			t.Fatalf("Parse(%q) failed: %v", tc.src, err)
		}
		fn, ok := f.Stmts[0].Cmd.(*FuncDecl)
		if !ok || fn.Body != nil {
			t.Fatalf("Parse(%q): want a bodyless *FuncDecl, got %T", tc.src, f.Stmts[0].Cmd)
		}
		if got := fn.End().String(); got != tc.want {
			t.Errorf("Parse(%q): End() = %s, want %s", tc.src, got, tc.want)
		}
	}

	// Tree shape test asserting non-nil nested FuncDecl body for `function a; function b; print z`
	{
		f, err := p.Parse(strings.NewReader("function a; function b; print z"), "")
		if err != nil {
			t.Fatalf("Parse(function a; function b; print z) failed: %v", err)
		}
		if len(f.Stmts) != 1 {
			t.Fatalf("expected 1 outer statement, got %d", len(f.Stmts))
		}
		fnA, ok := f.Stmts[0].Cmd.(*FuncDecl)
		if !ok {
			t.Fatalf("expected outer *FuncDecl, got %T", f.Stmts[0].Cmd)
		}
		if fnA.Name == nil || fnA.Name.Value != "a" {
			t.Errorf("expected outer func name 'a', got %v", fnA.Name)
		}
		if fnA.Body == nil {
			t.Fatalf("expected outer fnA.Body != nil, got nil")
		}
		fnB, ok := fnA.Body.Cmd.(*FuncDecl)
		if !ok {
			t.Fatalf("expected fnA.Body.Cmd to be inner *FuncDecl, got %T", fnA.Body.Cmd)
		}
		if fnB.Name == nil || fnB.Name.Value != "b" {
			t.Errorf("expected inner func name 'b', got %v", fnB.Name)
		}
		if fnB.Body == nil {
			t.Fatalf("expected inner fnB.Body != nil, got nil")
		}
		call, ok := fnB.Body.Cmd.(*CallExpr)
		if !ok {
			t.Fatalf("expected inner fnB.Body.Cmd to be *CallExpr, got %T", fnB.Body.Cmd)
		}
		if len(call.Args) < 2 || call.Args[0].Lit() != "print" || call.Args[1].Lit() != "z" {
			t.Errorf("expected 'print z', got %v", call.Args)
		}
	}
}
