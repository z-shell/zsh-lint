package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
)

// UnquotedVar reports direct, unquoted parameter expansions in command words.
//
// ID: `quoting/unquoted-var`
//
// Name: Unquoted variable expansion
//
// Summary: In configured projects, reports standalone unquoted parameter
// expansions in command arguments that may lose an empty value, with
// element-preserving advice for declared arrays.
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
// Configuration: The rule reports only for sources with project configuration.
// Without it, one file cannot show whether a value may be empty, and sampled
// precision was 1 in 34 on organization code and 0 in 20 on third-party code
// (#614, #705).
//
// False positives: Explicit native-Zsh splitting forms such as `${=words}`,
// flag-guided array or field splitting, and glob substitution `${~pattern}`
// (whose value must stay unquoted to match as a pattern) are excluded. Compound
// words, command names, and arguments of `:` are also excluded. Expansions that
// can never be empty are skipped: `${x:+word}` and `${x+word}`, which elide on
// purpose and would pass an empty argument if quoted; `${x:-word}` and
// `${x:=word}` with a non-empty literal word; `${x:?word}`, which fails instead
// of expanding empty; names declared integer or float earlier in the same
// function or file scope, unless that scope unsets them; and the read-only
// numeric parameters `PPID`, `ZSH_SUBSHELL` and `TTYIDLE`. Code may
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
	if !ok || !ctx.Source.Configured() {
		return
	}
	// The : exemption also covers a name after the command modifier: its
	// arguments are expanded only for their side effects either way. An option
	// argument such as NAME in exec -a NAME is checked for every command: an
	// empty unquoted NAME makes exec fail: "exec requires a command to execute".
	resolved := resolveCommand(call)
	words := append([]*syntax.Word(nil), resolved.optionArgs...)
	if resolved.name != ":" {
		words = append(words, resolved.args...)
	}
	for _, word := range words {
		// Only a standalone expansion can elide the whole argument. A quoted
		// expansion is nested in DblQuoted rather than directly in Word.Parts.
		if word == nil || len(word.Parts) != 1 {
			continue
		}
		param, ok := word.Parts[0].(*syntax.ParamExp)
		if !ok || shouldSkipUnquotedParam(param) {
			continue
		}
		if param.Param != nil && ctx.Scope != nil && ctx.Scope.IsDeclaredNumeric(param.Param.Value, param.Pos()) {
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
		case "?", "$", "!", "#", "LINENO", "HISTCMD", "SECONDS", "RANDOM", "EPOCHSECONDS", "EPOCHREALTIME",
			"PPID", "ZSH_SUBSHELL", "TTYIDLE":
			return true
		}
	}

	// 6. Expansion operators whose result is never an empty argument: ${x:+w}
	// and ${x+w} elide on purpose, so quoting them would pass an empty word;
	// ${x:?w} fails rather than expanding empty; ${x:-w} and ${x:=w} with a
	// non-empty literal w fall back to that text.
	if param.Exp != nil {
		switch param.Exp.Op {
		case syntax.AlternateUnset, syntax.AlternateUnsetOrNull, syntax.ErrorUnsetOrNull:
			return true
		case syntax.DefaultUnsetOrNull, syntax.AssignUnsetOrNull:
			if nonEmptyLiteralWord(param.Exp.Word) {
				return true
			}
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

// nonEmptyLiteralWord reports a word made only of literal or quoted literal
// text that is not empty.
func nonEmptyLiteralWord(word *syntax.Word) bool {
	if word == nil {
		return false
	}
	text := ""
	for _, part := range word.Parts {
		switch x := part.(type) {
		case *syntax.Lit:
			text += x.Value
		case *syntax.SglQuoted:
			text += x.Value
		case *syntax.DblQuoted:
			for _, inner := range x.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return false
				}
				text += lit.Value
			}
		default:
			return false
		}
	}
	return text != ""
}
