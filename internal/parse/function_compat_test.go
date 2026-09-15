package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseFunctionSemicolonBody(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantName string
	}{
		{
			name:     "plain function with semicolon",
			src:      "function f; { print hi }\n",
			wantName: "f",
		},
		{
			name:     "function with parens and semicolon",
			src:      "function f () ; { print hi }\n",
			wantName: "f",
		},
		{
			name:     "function with semicolon and newline",
			src:      "function f;\n{\n  print hi\n}\n",
			wantName: "f",
		},
		{
			name:     "multi-name function with semicolon",
			src:      "function a b; { print hi }\n",
			wantName: "a",
		},
		{
			name:     "multiple function definitions with semicolons",
			src:      "function f; { print first }\nfunction g; { print second }\n",
			wantName: "f",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), tc.name+".zsh")
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tc.src, err)
			}
			if len(file.AST().Stmts) == 0 {
				t.Fatal("len(Stmts) = 0, want at least 1")
			}
			decl, ok := file.AST().Stmts[0].Cmd.(*syntax.FuncDecl)
			if !ok {
				t.Fatalf("Cmd is not *syntax.FuncDecl: %T", file.AST().Stmts[0].Cmd)
			}
			name := ""
			if decl.Name != nil {
				name = decl.Name.Value
			} else if len(decl.Names) > 0 {
				name = decl.Names[0].Value
			}
			if name != tc.wantName {
				t.Errorf("function name = %q, want %q", name, tc.wantName)
			}
			if decl.Body == nil {
				t.Error("decl.Body = nil, want block body")
			}
		})
	}
}

func TestParseFunctionSemicolonBodyPreservesPositions(t *testing.T) {
	const src = "function my_func; { print hi }\n"
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	decl := file.AST().Stmts[0].Cmd.(*syntax.FuncDecl)
	if decl.Pos().Col() != 1 {
		t.Errorf("decl.Pos().Col() = %d, want 1", decl.Pos().Col())
	}
	if decl.Name.Pos().Col() != 10 {
		t.Errorf("decl.Name.Pos().Col() = %d, want 10", decl.Name.Pos().Col())
	}
	if decl.Body.Pos().Col() != 19 {
		t.Errorf("decl.Body.Pos().Col() = %d, want 19 (original opening brace)", decl.Body.Pos().Col())
	}
}
