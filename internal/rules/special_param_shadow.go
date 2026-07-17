package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"mvdan.cc/sh/v3/syntax"
)

// SpecialParamShadow reports declarations that shadow parameters set by the
// shell.
//
// ID: `compat/special-param-shadow`
//
// Name: Shadowing special shell parameters
//
// Summary: Reports `local`, `typeset`, `declare`, or `readonly` declarations
// of a curated set of shell-set parameters (for example `ZSH_VERSION`,
// `OSTYPE`, and `pipestatus`); explicit non-local `-g` declarations (including
// `readonly -g`), the `export` builtin, and `typeset`-family
// query/display/function modes (`-p`/`+p`, `-m`/`+m`, and `-f`/`+f`) are
// excluded, and reads are never reported.
//
// Why: The Zsh manual's zshparam "Parameters Set By The Shell" section
// documents these as shell-provided state. In a function, `readonly` is
// `typeset -r` and creates a local binding unless `-g` explicitly selects
// non-local behavior. Version probes such as `is-at-least $ZSH_VERSION` are
// pervasive in plugin code, so a `local ZSH_VERSION=...` in a caller silently
// feeds the override to all nested code for the lifetime of the scope. At top
// level, the same declaration form clobbers the shell-managed outer binding
// instead of creating a temporary local. Unlike ordinary shadowing, the reader
// has no declaration of the original to look up -- the shell set it. See
// https://zsh.sourceforge.io/Doc/Release/Parameters.html#Parameters-Set-By-The-Shell.
//
// Bad:
//
//	compile_zsh() {
//	  local ZSH_VERSION="$1"
//	}
//
// Good:
//
//	compile_zsh() {
//	  local target_zsh_version="$1"
//	}
//
// Severity: Warning. The pattern is functional but misleading and can change
// what nested code observes; deliberate compatibility shims are realistic, so
// it is suppressible rather than an error.
//
// False positives: Deliberate compatibility shims or test harnesses that fake
// `ZSH_VERSION` or `OSTYPE` for downstream code are the rule's target behavior
// made intentional; suppress them with a reason. The rule flags only a curated
// allowlist of read-mostly shell-set parameters and stays silent for reads and
// the conventionally mutated `path`, `fpath`, `PATH`, `REPLY`, and `match`.
// The shell-special `status` and `pipestatus` cases remain included because
// `local -h status=...` and `local -h pipestatus=(...)` can replace their
// special behavior with ordinary local bindings.
//
// Suppression: Use
// `# zsh-lint disable=compat/special-param-shadow -- <reason>` on the finding
// line or immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: Issue #64 records `zd/docker/utils.zsh:78`:
// `local ZSH_VERSION="$1"` inside a helper that takes a target version as its
// first argument. Deferred issue #72 tracks bare assignments in function scope
// and `integer`/`float` declarations; both are out of scope for this rule
// version.
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
