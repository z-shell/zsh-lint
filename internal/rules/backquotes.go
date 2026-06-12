package rules

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// Backquotes checks for deprecated backtick command substitutions.
type Backquotes struct{}

func (r Backquotes) ID() diag.RuleID {
	return "style/backquotes"
}

func (r Backquotes) Name() string {
	return "Deprecated backtick command substitution"
}

func (r Backquotes) Analyze(ctx *analyzer.Context, node syntax.Node) {
	if cs, ok := node.(*syntax.CmdSubst); ok && cs.Backquotes {
		ctx.Report(cs.Pos(), cs.End(), r.ID(), diag.Hint, "Use $(...) instead of deprecated `...` for command substitution")
	}
}
