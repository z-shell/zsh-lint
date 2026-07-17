package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"mvdan.cc/sh/v3/syntax"
)

// SpecialParamShadow reports declarations that shadow parameters set by the
// shell.
type SpecialParamShadow struct{}

func (SpecialParamShadow) ID() diag.RuleID {
	return "compat/special-param-shadow"
}

func (SpecialParamShadow) Name() string {
	return "Shadowing special shell parameters"
}

// shadowedSpecialParams is the curated set of shell-set parameters whose
// declaration can override state the shell maintains in the current scope and
// nested code. Names are matched exactly and case-sensitively.
var shadowedSpecialParams = map[string]struct{}{
	"ZSH_VERSION":    {},
	"ZSH_PATCHLEVEL": {},
	"ZSH_NAME":       {},
	"ZSH_ARGZERO":    {},
	"OSTYPE":         {},
	"MACHTYPE":       {},
	"VENDOR":         {},
	"pipestatus":     {},
	"status":         {},
}

// shadowingDecls are typeset-family builtins whose declaration forms can
// override shell-set parameters. export is intentionally excluded.
var shadowingDecls = map[string]struct{}{
	"local":    {},
	"typeset":  {},
	"declare":  {},
	"readonly": {},
}

func (rule SpecialParamShadow) Analyze(ctx *analyzer.Context, node syntax.Node) {
	decl, ok := node.(*syntax.DeclClause)
	if !ok || decl.Variant == nil {
		return
	}
	if _, ok := shadowingDecls[decl.Variant.Value]; !ok {
		return
	}
	if declaresGlobal(decl.Args) || usesNonDeclarationMode(decl.Args) {
		return
	}
	for _, assign := range decl.Args {
		if assign == nil || assign.Name == nil {
			continue
		}
		if _, shadowed := shadowedSpecialParams[assign.Name.Value]; !shadowed {
			continue
		}
		ctx.Report(
			assign.Name.Pos(),
			assign.Name.End(),
			rule.ID(),
			diag.Warning,
			"Declaring shell-set parameter '"+assign.Name.Value+
				"' can override shell-managed state in this scope and nested code; use a different parameter name",
		)
	}
}

// declaresGlobal reports whether a declaration carries a -g switch, which makes
// the declaration explicitly non-local and may target an enclosing binding.
// Flag arguments are the args without a name (for example -g or -ga).
func declaresGlobal(args []*syntax.Assign) bool {
	return hasShortFlag(args, "-", "g")
}

// usesNonDeclarationMode reports whether a typeset-family invocation selects
// a query, pattern, or function mode instead of declaring its named arguments.
// Zsh accepts both signs and grouped short flags for these modes.
func usesNonDeclarationMode(args []*syntax.Assign) bool {
	return hasShortFlag(args, "-+", "pmf")
}

// hasShortFlag reports whether a literal flag argument starts with an allowed
// sign and contains any requested option letter. Flag arguments are the args
// without a name (for example -g, -ap, or +m).
func hasShortFlag(args []*syntax.Assign, signs, letters string) bool {
	for _, assign := range args {
		if assign == nil || assign.Name != nil {
			continue
		}
		value, ok := literalWord(assign.Value)
		if !ok || len(value) < 2 || !strings.ContainsRune(signs, rune(value[0])) || value[1] == value[0] {
			continue
		}
		if strings.ContainsAny(value[1:], letters) {
			return true
		}
	}
	return false
}
