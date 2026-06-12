package rules

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// EvalUsage checks for the use of the 'eval' command.
type EvalUsage struct{}

func (r EvalUsage) ID() diag.RuleID {
	return "security/eval"
}

func (r EvalUsage) Name() string {
	return "Use of eval"
}

func (r EvalUsage) Analyze(ctx *analyzer.Context, node syntax.Node) {
	call, ok := node.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return
	}

	word := call.Args[0]
	if len(word.Parts) == 1 {
		if lit, ok := word.Parts[0].(*syntax.Lit); ok {
			if lit.Value == "eval" {
				ctx.Report(call.Pos(), call.End(), r.ID(), diag.Info, "Use of 'eval' can be dangerous; ensure inputs are properly sanitized")
			}
		}
	}
}
