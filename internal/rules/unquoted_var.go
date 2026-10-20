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
// `${x:=word}` with a literal-only default, or safe literal text alongside
// plain short named expansions, optionally double-quoted, when neither the
// outer name nor referenced names are known arrays and no expansion-changing
// setopt is visible; `${x:?word}`, which fails instead
// of expanding empty; names declared integer or float earlier in the same
// function or file scope, unless that scope unsets them; and the read-only
// numeric parameters `PPID`, `ZSH_SUBSHELL` and `TTYIDLE`. Code may
// intentionally omit an empty argument or expand a value guaranteed to be
// non-empty; suppress those cases with a reason. Single-element subscripts use
// scalar advice; ranges, reverse and chained subscripts retain their selections
// rather than recommending all elements. Array advice uses only earlier
// declarations in the same lexical function or file scope, without resolving
// dynamic caller scope or declarations in other files. Non-empty default
// inference assumes native option defaults for parameters not known as arrays;
// it does not model caller option state. A declaration or unset
// inside a command substitution counts only within that substitution, which
// runs in a subshell.
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
		if !ok || shouldSkipUnquotedParam(ctx, param) {
			continue
		}
		if param.Param != nil && ctx.Scope != nil && ctx.Scope.IsDeclaredNumeric(param.Param.Value, param.Pos()) {
			continue
		}
		message := "Variable expansion should be double-quoted"
		if param.Param != nil && ctx.Scope != nil && ctx.Scope.IsDeclaredArray(param.Param.Value, param.Pos()) {
			message = unquotedArrayMessage(param)
		}
		ctx.Report(param.Pos(), param.End(), r.ID(), diag.Info, message)
	}
}

func unquotedArrayMessage(param *syntax.ParamExp) string {
	// Never replace a selection with all elements (#747). Complex selectors
	// can yield a scalar or an array, so preserve them without guessing a rewrite.
	if len(param.LaterIndexes) > 0 {
		return "Quote this expansion without replacing its subscripts; use (@) to preserve selected array elements"
	}
	if param.Index != nil {
		switch index := param.Index.(type) {
		case *syntax.FlagsArithm:
			return "Quote this expansion without replacing its subscripts; use (@) to preserve selected array elements"
		case *syntax.BinaryArithm:
			if index.Op == syntax.Comma {
				return "Quote this expansion without replacing its subscripts; use (@) to preserve selected array elements"
			}
		case *syntax.Word:
			if len(index.Parts) == 1 {
				if lit, ok := index.Parts[0].(*syntax.Lit); ok && (lit.Value == "@" || lit.Value == "*") {
					return "Array expansion should preserve elements with \"${" + param.Param.Value + "[@]}\""
				}
			}
		}
		return "Variable expansion should be double-quoted"
	}
	return "Array expansion should preserve elements with \"${" + param.Param.Value + "[@]}\""
}

func shouldSkipUnquotedParam(ctx *analyzer.Context, param *syntax.ParamExp) bool {
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
	// literal-only w keep their existing exemption. The additional matcher
	// accepts only a bounded literal-and-reference grammar, not arbitrary words.
	if param.Exp != nil {
		switch param.Exp.Op {
		case syntax.AlternateUnset, syntax.AlternateUnsetOrNull, syntax.ErrorUnsetOrNull:
			return true
		case syntax.DefaultUnsetOrNull, syntax.AssignUnsetOrNull:
			if nonEmptyStaticWord(param.Exp.Word) ||
				param.Param != nil && syntax.ValidName(param.Param.Value) && param.Index == nil &&
					param.Flags == nil && len(param.Modifiers) == 0 &&
					param.RcExpand == syntax.OptUnset && param.Split == syntax.OptUnset && param.GlobSubst == syntax.OptUnset &&
					(ctx.Scope == nil || !ctx.Scope.IsDeclaredArray(param.Param.Value, param.Pos())) &&
					plainLiteralDefault(ctx, param.Exp.Word) && !hasDefaultExpansionOptions(ctx) {
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

// plainLiteralDefault recognizes a small grammar, not general word
// non-emptiness: safe literal parts plus short named references, optionally
// enclosed together in one pair of double quotes (#746).
func plainLiteralDefault(ctx *analyzer.Context, word *syntax.Word) bool {
	if word == nil {
		return false
	}
	parts, quoted := word.Parts, false
	if len(parts) == 1 {
		if q, ok := parts[0].(*syntax.DblQuoted); ok && !q.Dollar {
			parts, quoted = q.Parts, true
		}
	}
	literal := false
	for _, part := range parts {
		switch x := part.(type) {
		case *syntax.Lit:
			if strings.Contains(x.Value, "\\") {
				return false
			}
			if !quoted && strings.Trim(x.Value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./:-") != "" {
				return false
			}
			literal = literal || x.Value != ""
		case *syntax.ParamExp:
			if !x.Short || x.Param == nil || !syntax.ValidName(x.Param.Value) || x.Index != nil ||
				x.RcExpand != syntax.OptUnset || x.Split != syntax.OptUnset || x.GlobSubst != syntax.OptUnset ||
				ctx.Scope != nil && ctx.Scope.IsDeclaredArray(x.Param.Value, x.Pos()) {
				return false
			}
		default:
			return false
		}
	}
	return literal
}

// Any visible setopt enabling expansion-changing options defeats this narrow
// proof, even after the use or in a different function. Dynamic names and
// short option bundles are unknown, not evidence that native defaults hold.
func hasDefaultExpansionOptions(ctx *analyzer.Context) bool {
	if ctx.File == nil {
		return true
	}
	unsafe := false
	syntax.Walk(ctx.File.AST(), func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || unsafe {
			return !unsafe
		}
		name, args := effectiveCommand(call)
		if name != "setopt" {
			return true
		}
		for _, arg := range args {
			value, static := staticDeclarationWord(arg)
			value = strings.ToLower(strings.ReplaceAll(value, "_", ""))
			if !static || value != "--" && (strings.HasPrefix(value, "-") ||
				strings.ContainsAny(value, "*?[]") || value == "rcexpandparam" || value == "globsubst") {
				unsafe = true
				return false
			}
		}
		return true
	})
	return unsafe
}

// nonEmptyStaticWord preserves the pre-existing literal-only exemption. The
// new mixed-word inference is deliberately narrower around transformations.
func nonEmptyStaticWord(word *syntax.Word) bool {
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
