package rules

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// FuncDeclStyle reports function declarations that combine `function` and `()`.
//
// ID: `style/function-decl`
//
// Name: Function declaration style
//
// Summary: Reports declarations written as `function name()` instead of
// choosing either `name()` or `function name`.
//
// Why: The Zsh manual's Complex Commands grammar documents the sh-compatible
// `word ()` form and the Zsh `function word` form as alternatives. Combining
// both spellings is accepted but redundant, so choosing one form communicates
// the intended style more clearly.
// See https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands.
//
// Bad:
//
//	function render() { print ok; }
//
// Good:
//
//	render() { print ok; }
//
// Severity: Hint. The mixed declaration is valid Zsh and the suggested change
// is stylistic.
//
// False positives: Generated code or a project-wide convention may
// intentionally use the mixed spelling even though Zsh does not require it.
//
// Suppression: Use
// `# zsh-lint disable=style/function-decl -- <reason>` on the finding line or
// immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: The June 12, 2026 LangZsh clean-baseline run produced zero
// findings from this rule across the 11 parseable corpus files. This
// grandfathered rule therefore has no positive corpus citation yet.
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
