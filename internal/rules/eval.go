package rules

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// EvalUsage reports calls to the `eval` shell builtin.
//
// ID: `security/eval`
//
// Name: Use of eval
//
// Summary: Reports commands whose literal command name is `eval`.
//
// Why: The Zsh manual documents that `eval` reads its arguments as shell input
// and executes the resulting commands in the current shell process. Dynamic or
// untrusted text can therefore become shell syntax rather than inert data.
// See https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html#index-eval.
//
// Bad:
//
//	eval "print -r -- $user_input"
//
// Good:
//
//	print -r -- "$user_input"
//
// Severity: Info. Re-evaluating dynamic input is risky, but deliberate uses
// such as trusted code generation and compatibility shims are common enough
// that the pattern is not automatically a bug.
//
// False positives: Static, maintainer-controlled command strings and deliberate
// shell-language adapters may require `eval`. Keep the finding visible unless
// the trust boundary is documented next to the call.
//
// Suppression: Use
// `# zsh-lint disable=security/eval -- <reason>` on the finding line or
// immediately before the next non-comment, non-blank source line. A reason is
// strongly recommended for this security-category rule.
//
// Corpus evidence: The June 12, 2026 LangZsh clean-baseline run produced zero
// findings from this rule across the 11 parseable corpus files. This
// grandfathered rule therefore has no positive corpus citation yet.
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
