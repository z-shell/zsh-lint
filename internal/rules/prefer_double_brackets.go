package rules

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// PreferDoubleBrackets flags uses of [ ] or test in favor of Zsh's [[ ]].
type PreferDoubleBrackets struct{}

func (r PreferDoubleBrackets) ID() diag.RuleID {
	return "style/prefer-double-brackets"
}

func (r PreferDoubleBrackets) Name() string {
	return "Prefer [[ ]] over [ ] or test"
}

func (r PreferDoubleBrackets) Analyze(ctx *analyzer.Context, node syntax.Node) {
	call, ok := node.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return
	}

	word := call.Args[0]
	if len(word.Parts) == 1 {
		if lit, ok := word.Parts[0].(*syntax.Lit); ok {
			if lit.Value == "[" || lit.Value == "test" {
				ctx.Report(call.Pos(), call.End(), r.ID(), diag.Warning, "Prefer Zsh's [[ ... ]] over [ ... ] or test for conditions")
			}
		}
	}
}
