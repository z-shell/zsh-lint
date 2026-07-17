package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"mvdan.cc/sh/v3/expand"
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
// query/display/function modes (`-p`/`+p`, `+m`, and `-f`/`+f`) are excluded.
// Pattern declarations with `-m` are reported only when an effective `+g`
// makes matching non-local parameters local. The rule also stays silent when
// dynamic option words or the ambient `GLOBAL_EXPORT` option make declaration
// scope uncertain. Reads are never reported.
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
	if declarationModeFor(decl.Variant.Value, decl.Args) != declarationModeLocal {
		return
	}
	tieSeparator := tiedParameterSeparator(decl.Args)
	for _, assign := range decl.Args {
		if assign == nil || assign == tieSeparator {
			continue
		}

		var name string
		start, end := assign.Pos(), assign.End()
		if assign.Name != nil {
			name = assign.Name.Value
			start, end = assign.Name.Pos(), assign.Name.End()
		} else if value, ok := staticDeclarationWord(assign.Value); ok {
			var hasValue bool
			name, _, hasValue = strings.Cut(value, "=")
			if hasValue {
				name = strings.TrimSuffix(name, "+")
			}
		}
		if _, shadowed := shadowedSpecialParams[name]; !shadowed {
			continue
		}
		ctx.Report(
			start,
			end,
			rule.ID(),
			diag.Warning,
			"Declaring shell-set parameter '"+name+
				"' can override shell-managed state in this scope and nested code; use a different parameter name",
		)
	}
}

// tiedParameterSeparator returns the optional third operand of typeset -T.
// Zsh declares only the first two operands as the tied scalar and array; the
// third is separator data even when its text is a valid parameter name. The
// option scan stops at the same boundaries as declarationModeFor so an
// option-looking separator is never reinterpreted as an option.
func tiedParameterSeparator(args []*syntax.Assign) *syntax.Assign {
	tied := false
	firstOperand := len(args)
	for i, assign := range args {
		if assign == nil {
			continue
		}
		if assign.Name != nil {
			firstOperand = i
			break
		}
		value, ok := staticDeclarationWord(assign.Value)
		if !ok {
			return nil
		}
		if value == "-" || value == "--" {
			firstOperand = i + 1
			break
		}
		if len(value) < 2 || !strings.ContainsRune("-+", rune(value[0])) || value[1] == value[0] {
			firstOperand = i
			break
		}

		enabled := value[0] == '-'
		for _, option := range value[1:] {
			if option == 'T' {
				tied = enabled
			}
		}
	}
	if !tied {
		return nil
	}

	operand := 0
	for _, assign := range args[firstOperand:] {
		if assign == nil {
			continue
		}
		operand++
		if operand == 3 {
			return assign
		}
	}
	return nil
}

type declarationMode uint8

const (
	declarationModeUnknown declarationMode = iota
	declarationModeLocal
	declarationModeGlobal
	declarationModeNonDeclaration
)

// declarationModeFor classifies the effective options after Zsh removes
// quoting. Option processing stops at the first operand, --, or the historical
// single - terminator. A dynamic naked argument in the option region makes the
// command's declaration mode unknowable, so callers must stay silent rather
// than guess. For typeset-family declarations other than local, any observed
// -x without an explicit g decision depends on the ambient GLOBAL_EXPORT option
// and is likewise unknown; a later +x does not undo -x's implied -g. Pattern
// mode creates local parameters only when -m is combined with an effective
// +g; the final explicit g state therefore controls whether it declares in the
// current scope.
func declarationModeFor(variant string, args []*syntax.Assign) declarationMode {
	var global, globalExplicit, pattern, patternExplicit, sawExport bool
	for _, assign := range args {
		if assign == nil {
			continue
		}
		if assign.Name != nil {
			break
		}
		value, ok := staticDeclarationWord(assign.Value)
		if !ok {
			return declarationModeUnknown
		}
		if value == "-" || value == "--" {
			break
		}
		if len(value) < 2 || !strings.ContainsRune("-+", rune(value[0])) || value[1] == value[0] {
			break
		}
		// A retained backslash denotes an invalid option character, not quoting
		// around a later valid character. The builtin rejects such a word.
		if strings.ContainsRune(value, '\\') {
			return declarationModeUnknown
		}
		if strings.ContainsAny(value[1:], "pf") {
			return declarationModeNonDeclaration
		}

		enabled := value[0] == '-'
		for _, option := range value[1:] {
			switch option {
			case 'g':
				globalExplicit = true
				global = enabled
			case 'm':
				patternExplicit = true
				pattern = enabled
			case 'x':
				if enabled {
					sawExport = true
				}
			}
		}
	}

	if patternExplicit && (!pattern || !globalExplicit || global) {
		return declarationModeNonDeclaration
	}
	if globalExplicit {
		if global {
			return declarationModeGlobal
		}
		return declarationModeLocal
	}
	if variant != "local" && sawExport {
		return declarationModeUnknown
	}
	return declarationModeLocal
}

// staticDeclarationWord reconstructs the limited word forms whose values are
// known after Zsh removes quoting, including ANSI-C single-quoted escapes. Do
// not use literalWord here: declaration options may be split across literal and
// quoted AST parts. Any expansion makes the option dynamic and must leave
// declaration mode unknown.
func staticDeclarationWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", false
	}

	var value strings.Builder
	for _, part := range word.Parts {
		switch part := part.(type) {
		case *syntax.Lit:
			value.WriteString(removeUnquotedBackslashEscapes(part.Value))
		case *syntax.SglQuoted:
			partValue := part.Value
			if part.Dollar {
				var err error
				partValue, _, err = expand.Format(nil, partValue, nil)
				if err != nil {
					return "", false
				}
				partValue, _, _ = strings.Cut(partValue, "\x00")
			}
			value.WriteString(partValue)
		case *syntax.DblQuoted:
			for _, quotedPart := range part.Parts {
				literal, ok := quotedPart.(*syntax.Lit)
				if !ok {
					return "", false
				}
				value.WriteString(removeDoubleQuotedBackslashEscapes(literal.Value))
			}
		default:
			return "", false
		}
	}
	return value.String(), true
}

// removeUnquotedBackslashEscapes applies quote removal to static unquoted
// literals. Outside quotes, a backslash quotes the following character; an
// escaped newline is removed as a line continuation.
func removeUnquotedBackslashEscapes(value string) string {
	if !strings.ContainsRune(value, '\\') {
		return value
	}

	var unquoted strings.Builder
	unquoted.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 == len(value) {
			unquoted.WriteByte(value[i])
			continue
		}
		i++
		if value[i] != '\n' {
			unquoted.WriteByte(value[i])
		}
	}
	return unquoted.String()
}

// removeDoubleQuotedBackslashEscapes preserves backslashes before ordinary
// characters. Within double quotes, only shell-special characters and newline
// can be quoted with a backslash.
func removeDoubleQuotedBackslashEscapes(value string) string {
	if !strings.ContainsRune(value, '\\') {
		return value
	}

	var unquoted strings.Builder
	unquoted.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 == len(value) || !strings.ContainsRune("$`\"\\\n", rune(value[i+1])) {
			unquoted.WriteByte(value[i])
			continue
		}
		i++
		if value[i] != '\n' {
			unquoted.WriteByte(value[i])
		}
	}
	return unquoted.String()
}
