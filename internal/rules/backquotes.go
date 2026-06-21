package rules

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// Backquotes reports grave-accent command substitutions.
//
// ID: `style/backquotes`
//
// Name: Prefer dollar-parenthesis command substitution
//
// Summary: Reports the grave-accent form of command substitution in favor of
// `$()`.
//
// Why: The Zsh manual's Command Substitution section documents both `$()` and
// grave accents as supported forms. This rule prefers `$()` as the clearer,
// readily nestable modern idiom; it does not claim grave accents are invalid
// Zsh syntax.
// See https://zsh.sourceforge.io/Doc/Release/Expansion.html#Command-Substitution.
//
// Bad:
//
//	current=`pwd`
//
// Good:
//
//	current=$(pwd)
//
// Severity: Hint. The forms are semantically supported; this is an idiom and
// readability preference rather than a correctness diagnostic.
//
// False positives: A project may intentionally preserve historical style or
// mirror code shared with an environment where that spelling is required.
//
// Suppression: Use
// `# zsh-lint disable=style/backquotes -- <reason>` on the finding line or
// immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: The June 12, 2026 LangZsh clean-baseline run produced zero
// findings from this rule across the 11 parseable corpus files. This
// grandfathered rule therefore has no positive corpus citation yet.
type Backquotes struct{}

func (r Backquotes) ID() diag.RuleID {
	return "style/backquotes"
}

func (r Backquotes) Name() string {
	return "Prefer dollar-parenthesis command substitution"
}

func (r Backquotes) Analyze(ctx *analyzer.Context, node syntax.Node) {
	if cs, ok := node.(*syntax.CmdSubst); ok && cs.Backquotes {
		ctx.Report(cs.Pos(), cs.End(), r.ID(), diag.Hint, "Prefer $(...) over grave accents for command substitution")
	}
}
