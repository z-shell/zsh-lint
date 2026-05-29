package rules

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// UnquotedVar checks for variable expansions that are not enclosed in double quotes.
// Zsh does not perform word splitting by default, but quoting is still recommended
// for safety and portability (e.g. against empty values).
type UnquotedVar struct{}

func (r UnquotedVar) ID() diag.RuleID {
	return "quoting/unquoted-var"
}

func (r UnquotedVar) Name() string {
	return "Unquoted variable expansion"
}

func (r UnquotedVar) Analyze(ctx *analyzer.Context, node syntax.Node) {
	word, ok := node.(*syntax.Word)
	if !ok {
		return
	}

	for _, part := range word.Parts {
		if param, ok := part.(*syntax.ParamExp); ok {
			// A ParamExp directly inside a Word's Parts means it is unquoted.
			// (If it were quoted, it would be inside a *syntax.DblQuoted part).
			ctx.Report(param.Pos(), param.End(), r.ID(), diag.Warning, "Variable expansion should be double-quoted")
		}
	}
}
