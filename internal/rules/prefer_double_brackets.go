package rules

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// PreferDoubleBrackets reports `[` and `test` conditional commands.
//
// ID: `style/prefer-double-brackets`
//
// Name: Prefer [[ ]] over [ ] or test
//
// Summary: Reports literal `[` and `test` command names in favor of Zsh's
// `[[ ... ]]` compound command.
//
// Why: The Zsh manual's Conditional Expressions section documents that
// expansions inside `[[ ... ]]` are constrained to one word and do not perform
// filename generation. With `[` or `test`, normal command-line globbing can
// produce multiple words and confuse the test command's syntax.
// See https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html.
//
// Bad:
//
//	if [ "$name" = *.zsh ]; then print match; fi
//
// Good:
//
//	if [[ $name = *.zsh ]]; then print match; fi
//
// Severity: Hint. `[` and `test` remain valid commands; the rule recommends
// the safer and more expressive Zsh-native conditional syntax.
//
// False positives: Scripts intentionally kept portable across POSIX shells, or
// code that explicitly needs `test` command semantics, should retain the
// portable form and document the choice.
//
// Suppression: Use
// `# zsh-lint disable=style/prefer-double-brackets -- <reason>` on the finding
// line or immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: The June 12, 2026 LangZsh clean-baseline run produced zero
// findings from this rule across the 11 parseable corpus files. This
// grandfathered rule therefore has no positive corpus citation yet.
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
				ctx.Report(call.Pos(), call.End(), r.ID(), diag.Hint, "Prefer Zsh's [[ ... ]] over [ ... ] or test for conditions")
			}
		}
	}
}
