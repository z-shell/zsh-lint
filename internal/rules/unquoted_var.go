package rules

import (
	"strings"

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
// Summary: Reports standalone unquoted parameter expansions in command arguments
// that may lose an empty value, with element-preserving advice for declared arrays.
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
// Severity: Info. Losing an empty argument can change command behavior, but
// intentional elision and values guaranteed to be non-empty are common.
//
// False positives: Explicit native-Zsh splitting forms such as `${=words}`,
// flag-guided array or field splitting, and glob substitution `${~pattern}`
// (whose value must stay unquoted to match as a pattern) are excluded. Compound
// words, command names, and arguments of `:` are also excluded. Code may
// intentionally omit an empty argument or expand a value guaranteed to be
// non-empty; suppress those cases with a reason. Array advice uses only earlier
// declarations in the same lexical function or file scope, without resolving
// dynamic caller scope or declarations in other files.
//
// Suppression: Use
// `# zsh-lint disable=quoting/unquoted-var -- <reason>` on the finding line or
// immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: Sourced scripts and plugin entrypoints frequently use
// quoted parameters for scalar configuration and path derivation.
type UnquotedVar struct{}

func (r UnquotedVar) ID() diag.RuleID {
	return "quoting/unquoted-var"
}

func (r UnquotedVar) Name() string {
	return "Unquoted variable expansion"
}

func (UnquotedVar) NeedsScope() bool { return true }

func (r UnquotedVar) Analyze(ctx *analyzer.Context, node syntax.Node) {
	call, ok := node.(*syntax.CallExpr)
	if !ok {
		return
	}
	name, args := effectiveCommand(call)
	if name == ":" {
		return
	}
	for _, word := range args {
		// Only a standalone expansion can elide the whole argument. A quoted
		// expansion is nested in DblQuoted rather than directly in Word.Parts.
		if word == nil || len(word.Parts) != 1 {
			continue
		}
		param, ok := word.Parts[0].(*syntax.ParamExp)
		if !ok || shouldSkipUnquotedParam(param) {
			continue
		}
		message := "Variable expansion should be double-quoted"
		if param.Param != nil && ctx.Scope != nil && ctx.Scope.IsDeclaredArray(param.Param.Value, param.Pos()) {
			message = "Array expansion should preserve elements with \"${" + param.Param.Value + "[@]}\""
		}
		ctx.Report(param.Pos(), param.End(), r.ID(), diag.Info, message)
	}
}

func shouldSkipUnquotedParam(param *syntax.ParamExp) bool {
	if param == nil {
		return true
	}

	// 1. Length (${#var}), Width (${%var}), or IsSet (${+var}) expansions
	// return numeric integers (0, 1, or string length) that cannot split or elide.
	if param.Length || param.Width || param.IsSet {
		return true
	}

	// 2. Special parameters guaranteed to evaluate to numeric integers
	if param.Param != nil {
		switch param.Param.Value {
		case "?", "$", "!", "#", "LINENO", "HISTCMD", "SECONDS", "RANDOM", "EPOCHSECONDS", "EPOCHREALTIME":
			return true
		}
	}

	// 3. Explicit Zsh parameter expansion flags controlling splitting or array behavior
	if param.Flags != nil {
		flagVal := param.Flags.Value
		// Check for intentional splitting/array flags: @ (array), = (word split), f (lines), s (split), z (words)
		if strings.ContainsAny(flagVal, "@=fszw~") {
			return true
		}
	}

	// 4. ${=spec} explicitly requests SH_WORD_SPLIT behavior in native Zsh.
	// mvdan/sh v3.14 carries the prefix as a typed field; ${==spec} sets it to
	// OptOff, which disables splitting and therefore retains the ordinary
	// unquoted-empty-elision risk covered by this rule.
	if param.Split == syntax.OptOn {
		return true
	}

	// 5. ${~spec} asks Zsh to treat the value as a glob pattern. Quoting it
	// would defeat the expansion, so the unquoted form is the only correct one.
	if param.GlobSubst == syntax.OptOn {
		return true
	}

	return false
}
