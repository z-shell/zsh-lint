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
