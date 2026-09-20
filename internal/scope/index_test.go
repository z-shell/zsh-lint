package scope_test

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/scope"
	"mvdan.cc/sh/v3/syntax"
)

func TestIndexer(t *testing.T) {
	code := `
global_var="test"
export exported_var=1
alias ll="ls -la"

my_func() {
	local local_var="safe"
	undeclared_local="unsafe"
}
`
	file, err := parse.Parse(strings.NewReader(code), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	sm := scope.NewMap()
	sm.Index(file.AST())

	// Test Globals
	globals := []string{"global_var", "exported_var", "ll", "my_func", "undeclared_local"}
	for _, g := range globals {
		if _, ok := sm.Globals[g]; !ok {
			t.Errorf("expected %q to be recorded as a global", g)
		}
	}

	// Test Export Flag
	if !sm.Globals["exported_var"].Exported {
		t.Errorf("expected exported_var to have Exported=true")
	}

	// Test Function Locals
	var funcDecl *syntax.FuncDecl
	for _, l := range sm.Locals {
		if _, ok := l["local_var"]; ok {
			funcDecl = sm.Globals["my_func"].Node.(*syntax.FuncDecl)
			break
		}
	}

	if funcDecl == nil {
		t.Fatalf("local_var not found in any function context")
	}

	if !sm.IsDeclared("local_var", funcDecl) {
		t.Errorf("expected local_var to be declared in function context")
	}

	if sm.IsDeclared("local_var", nil) { // nil means global context
		t.Errorf("expected local_var NOT to be declared in global context")
	}
}

// TestIndexerAliasFlags verifies that alias option flags (e.g. -g, -s) are not
// recorded as aliases named after the flag; only real name=value definitions are.
func TestIndexerAliasFlags(t *testing.T) {
	code := `
alias -g G="grep"
alias -s html=cat
alias plain="ls"
`
	file, err := parse.Parse(strings.NewReader(code), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	sm := scope.NewMap()
	sm.Index(file.AST())

	for _, want := range []string{"G", "html", "plain"} {
		if _, ok := sm.Globals[want]; !ok {
			t.Errorf("expected alias %q to be recorded", want)
		}
	}
	for _, flag := range []string{"-g", "-s"} {
		if _, ok := sm.Globals[flag]; ok {
			t.Errorf("option flag %q must not be recorded as an alias", flag)
		}
	}
}

func TestIndexerMultiNameFunction(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "function keyword",
			src:  "function a b { : }",
		},
		{
			name: "parentheses syntax",
			src:  "a b () { : }",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(tc.src), "test.zsh")
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			sm := scope.NewMap()
			sm.Index(file.AST())

			for _, name := range []string{"a", "b"} {
				sym, ok := sm.Globals[name]
				if !ok {
					t.Fatalf("expected %q to be recorded in Globals", name)
				}
				if sym.Kind != scope.KindFunction {
					t.Errorf("expected %q Kind to be KindFunction, got %v", name, sym.Kind)
				}
			}

			symA := sm.Globals["a"]
			symB := sm.Globals["b"]
			declA, okA := symA.Node.(*syntax.FuncDecl)
			declB, okB := symB.Node.(*syntax.FuncDecl)
			if !okA || !okB {
				t.Fatalf("expected both symbols to have *syntax.FuncDecl Node")
			}
			if declA != declB {
				t.Errorf("expected both symbols to reference the same *syntax.FuncDecl node, got %p vs %p", declA, declB)
			}
		})
	}
}

func TestIndexerAnonymousFunction(t *testing.T) {
	code := `() { local v=1 }`
	file, err := parse.Parse(strings.NewReader(code), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	sm := scope.NewMap()
	sm.Index(file.AST())

	for name, sym := range sm.Globals {
		if sym.Kind == scope.KindFunction {
			t.Errorf("unexpected KindFunction symbol in Globals: %q", name)
		}
	}
	if _, ok := sm.Globals[""]; ok {
		t.Errorf("Globals must not contain an empty-name key")
	}

	anonDecl := firstFuncDecl(t, file.AST())

	locals, ok := sm.Locals[anonDecl]
	if !ok {
		t.Fatalf("expected anonymous function to have entry in Locals")
	}
	if _, ok := locals["v"]; !ok {
		t.Errorf("expected Locals[decl] to contain %q", "v")
	}

	if !sm.IsDeclared("v", anonDecl) {
		t.Errorf("expected IsDeclared(\"v\", decl) to be true")
	}

	if sm.IsDeclared("v", nil) {
		t.Errorf("expected IsDeclared(\"v\", nil) to be false")
	}
}

func TestFunctionNames(t *testing.T) {
	t.Run("nil decl", func(t *testing.T) {
		if got := scope.FunctionNames(nil); got != nil {
			t.Errorf("expected nil for nil declaration, got %v", got)
		}
	})

	t.Run("single-name decl", func(t *testing.T) {
		file, err := parse.Parse(strings.NewReader("single() { : }"), "test.zsh")
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		decl := firstFuncDecl(t, file.AST())
		got := scope.FunctionNames(decl)
		if len(got) != 1 || got[0] != decl.Name {
			t.Errorf("expected exactly decl.Name, got %v", got)
		}
	})

	t.Run("multi-name decl", func(t *testing.T) {
		file, err := parse.Parse(strings.NewReader("function a b { : }"), "test.zsh")
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		decl := firstFuncDecl(t, file.AST())
		got := scope.FunctionNames(decl)
		if len(got) != len(decl.Names) {
			t.Fatalf("expected %d names, got %d", len(decl.Names), len(got))
		}
		for i := range got {
			if got[i] != decl.Names[i] {
				t.Errorf("name %d mismatch: got %v, want %v", i, got[i], decl.Names[i])
			}
		}
	})

	t.Run("anonymous decl", func(t *testing.T) {
		file, err := parse.Parse(strings.NewReader("() { : }"), "test.zsh")
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		decl := firstFuncDecl(t, file.AST())
		got := scope.FunctionNames(decl)
		if len(got) != 0 {
			t.Errorf("expected empty slice for anonymous function, got %v", got)
		}
	})
}

// firstFuncDecl returns the first function declaration under root, so a
// test can key into Map.Locals with the same node the indexer walked.
func firstFuncDecl(t *testing.T, root syntax.Node) *syntax.FuncDecl {
	t.Helper()
	var decl *syntax.FuncDecl
	syntax.Walk(root, func(n syntax.Node) bool {
		if fn, ok := n.(*syntax.FuncDecl); ok {
			decl = fn
			return false
		}
		return true
	})
	if decl == nil {
		t.Fatalf("no *syntax.FuncDecl found in tree")
	}
	return decl
}
