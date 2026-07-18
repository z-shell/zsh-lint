package rules

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// UnquotedVar reports direct, unquoted parameter expansions in command words.
//
// ID: `quoting/unquoted-var`
//
// Name: Unquoted variable expansion
//
// Summary: Reports parameter expansions in command names or arguments that are
// not enclosed in double quotes.
//
// Why: The Zsh manual's Parameter Expansion section explains that unquoted
// parameters are not split on whitespace by default, unlike in sh, but null
// words are still elided; enabling `SH_WORD_SPLIT` also makes unquoted values
// subject to field splitting. Double quotes preserve an empty scalar as an
// argument and keep the expansion single-word under either option state.
// See https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion.
//
// Bad:
//
//	print -r -- $value
//
// Good:
//
//	print -r -- "$value"
//
// Severity: Warning. Losing an empty argument or inheriting `SH_WORD_SPLIT`
// can change command behavior, while intentional elision remains realistic.
//
// False positives: Code may intentionally omit an empty argument, deliberately
// rely on `SH_WORD_SPLIT`, or expand a value guaranteed to be non-empty. Those
// cases should use a reasoned suppression rather than weakening unrelated
// diagnostics.
//
// Suppression: Use
// `# zsh-lint disable=quoting/unquoted-var -- <reason>` on the finding line or
// immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: The June 12, 2026 LangZsh clean-baseline run produced zero
// findings from this rule across the 11 parseable corpus files. This
// grandfathered rule therefore has no positive corpus citation yet.
type UnquotedVar struct{}

func (r UnquotedVar) ID() diag.RuleID {
	return "quoting/unquoted-var"
}

func (r UnquotedVar) Name() string {
	return "Unquoted variable expansion"
}

func (r UnquotedVar) Analyze(ctx *analyzer.Context, node syntax.Node) {
	// Only inspect command words: the command name and its arguments. Empty
	// unquoted expansions can be elided there, and SH_WORD_SPLIT can split
	// their values into multiple arguments. This deliberately excludes
	// assignment right-hand sides (e.g. A=$BAZ), where the rule's
	// argument-vector preservation rationale does not apply.
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
