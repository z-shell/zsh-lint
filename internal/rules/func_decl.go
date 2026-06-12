package rules

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// FuncDeclStyle checks for mixing the 'function' keyword with '()' in declarations.
type FuncDeclStyle struct{}

func (r FuncDeclStyle) ID() diag.RuleID {
	return "style/function-decl"
}

func (r FuncDeclStyle) Name() string {
	return "Function declaration style"
}

func (r FuncDeclStyle) Analyze(ctx *analyzer.Context, node syntax.Node) {
	decl, ok := node.(*syntax.FuncDecl)
	if !ok {
		return
	}

	// Mixing 'function foo' and 'foo()' -> 'function foo()'
	if decl.RsrvWord && decl.Parens {
		ctx.Report(decl.Pos(), decl.End(), r.ID(), diag.Hint, "Avoid mixing 'function' keyword with '()' for function declarations; use 'foo()' or 'function foo'")
	}
}
