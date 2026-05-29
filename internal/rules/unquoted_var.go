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
	// Only inspect command words: the command name and its arguments. An
	// unquoted expansion there is subject to word splitting and globbing. This
	// deliberately excludes assignment right-hand sides (e.g. A=$BAZ), where
	// expansion is safe in both Zsh and Bash.
	call, ok := node.(*syntax.CallExpr)
	if !ok {
		return
	}
	for _, word := range call.Args {
		for _, part := range word.Parts {
			if param, ok := part.(*syntax.ParamExp); ok {
				// A ParamExp directly inside a Word's Parts is unquoted; a quoted
				// expansion would sit inside a *syntax.DblQuoted part instead.
				ctx.Report(param.Pos(), param.End(), r.ID(), diag.Warning, "Variable expansion should be double-quoted")
			}
		}
	}
}
