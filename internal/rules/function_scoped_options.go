package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"mvdan.cc/sh/v3/syntax"
)

// FunctionScopedOptions reports autoloadable function files that execute logic
// before localizing inherited shell options.
//
// ID: `plugin/function-scoped-options`
//
// Name: Function files should scope shell options
//
// Summary: Reports executable files beneath a `functions` directory when
// neither `builtin emulate -L zsh` nor `setopt local_options` appears before
// the first non-guard top-level statement.
//
// Why: Zsh functions inherit the caller's option state. The Zsh manual
// documents `LOCAL_OPTIONS` as restoring options on function return, while
// `emulate -L` both selects Zsh emulation and makes option changes local.
// Without either form, options such as `SH_WORD_SPLIT`, `KSH_ARRAYS`, or
// `NO_EXTENDED_GLOB` can silently change function behavior.
// See https://zsh.sourceforge.io/Doc/Release/Options.html#index-LOCAL_005fOPTIONS
// and
// https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html#index-emulate.
//
// Bad:
//
//	local -a matches
//	matches=( $~pattern )
//
// Good:
//
//	builtin emulate -L zsh
//	local -a matches
//	matches=( $~pattern )
//
// Severity: Hint. Missing option scoping enables latent caller-dependent bugs,
// but some helper functions intentionally inspect or mutate caller option
// state.
//
// False positives: Functions that intentionally share caller option state and
// trivial single-builtin functions remain findings by design; suppress them
// with a reason. Files outside a complete `functions` path segment are out of
// scope. A contiguous prefix of recognized `condition || return` or
// single-return `if` guards is accepted before scoping.
//
// Suppression: Use
// `# zsh-lint disable=plugin/function-scoped-options -- <reason>` on the
// finding line or immediately before the next non-comment, non-blank source
// line.
//
// Corpus evidence: Issue #63 reported missing scoping across z-a-meta-plugins
// and zsh-fancy-completions function files. The analyzer currently flags
// `zsh-fancy-completions/functions/.completion-prediction` and
// `zsh-fancy-completions/functions/.force_rehash`. The originally cited
// `z-a-meta-plugins/functions/.za-meta-plugins-meta-cmd-help-handler` is a
// fully commented-out stub and is correctly silent. Compliant
// `setopt local_options` and `builtin emulate -L zsh` examples exist in the
// same repositories.
type FunctionScopedOptions struct{}

func (FunctionScopedOptions) ID() diag.RuleID {
	return "plugin/function-scoped-options"
}

func (FunctionScopedOptions) Name() string {
	return "Function files should scope shell options"
}

func (rule FunctionScopedOptions) Analyze(ctx *analyzer.Context, node syntax.Node) {
	file, ok := node.(*syntax.File)
	if !ok || !hasFunctionsPathSegment(ctx.FilePath) {
		return
	}

	for _, stmt := range file.Stmts {
		if stmt == nil {
			continue
		}
		if isOptionScopingStatement(stmt) {
			return
		}
		if isLeadingReturnGuard(stmt) {
			continue
		}
		ctx.Report(
			stmt.Pos(),
			stmt.End(),
			rule.ID(),
			diag.Hint,
			"Scope function options with 'builtin emulate -L zsh' or 'setopt local_options' before executable logic",
		)
		return
	}
}

func hasFunctionsPathSegment(path string) bool {
	normalized := strings.ReplaceAll(path, `\`, "/")
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "functions" {
			return true
		}
	}
	return false
}

func isOptionScopingStatement(stmt *syntax.Stmt) bool {
	if stmt == nil || hasUnsafeStatementEffect(stmt) {
		return false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) != 0 {
		return false
	}

	commandIndex, command, ok := literalCommand(call)
	if !ok {
		return false
	}

	switch command {
	case "emulate":
		return isLocalZshEmulate(call.Args[commandIndex+1:])
	case "setopt":
		return enablesLocalOptions(call.Args[commandIndex+1:])
	default:
		return false
	}
}

func hasUnsafeStatementEffect(stmt *syntax.Stmt) bool {
	return stmt.Background ||
		stmt.Coprocess ||
		stmt.Disown
}

func isLeadingReturnGuard(stmt *syntax.Stmt) bool {
	if stmt == nil || hasUnsafeGuardEffect(stmt) {
		return false
	}

	switch command := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		return command.Op == syntax.OrStmt && isLiteralReturnStatement(command.Y)
	case *syntax.IfClause:
		return command.Else == nil &&
			len(command.Cond) > 0 &&
			len(command.Then) == 1 &&
			isLiteralReturnStatement(command.Then[0])
	default:
		return false
	}
}

func isLiteralReturnStatement(stmt *syntax.Stmt) bool {
	if stmt == nil || hasUnsafeGuardEffect(stmt) {
		return false
	}

	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) != 0 {
		return false
	}

	commandIndex, command, ok := literalCommand(call)
	if !ok || command != "return" {
		return false
	}
	args := call.Args[commandIndex+1:]
	// Zsh return accepts at most one status, optionally after "--". Treating
	// extra literal words as a guard is unsafe because return reports an error
	// and execution continues in the function.
	if len(args) > 2 {
		return false
	}
	if len(args) == 2 {
		optionTerminator, ok := literalWord(args[0])
		if !ok || optionTerminator != "--" {
			return false
		}
	}
	for _, arg := range args {
		if _, ok := literalWord(arg); !ok {
			return false
		}
	}
	return true
}

func hasUnsafeGuardEffect(stmt *syntax.Stmt) bool {
	return stmt.Negated ||
		hasUnsafeStatementEffect(stmt) ||
		len(stmt.Redirs) != 0
}

func literalCommand(call *syntax.CallExpr) (int, string, bool) {
	if call == nil || len(call.Args) == 0 {
		return 0, "", false
	}

	command, ok := literalWord(call.Args[0])
	if !ok {
		return 0, "", false
	}
	if command != "builtin" {
		return 0, command, true
	}
	if len(call.Args) < 2 {
		return 0, "", false
	}

	command, ok = literalWord(call.Args[1])
	return 1, command, ok
}

func literalWord(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) != 1 {
		return "", false
	}
	literal, ok := word.Parts[0].(*syntax.Lit)
	if !ok {
		return "", false
	}
	return literal.Value, true
}

func isLocalZshEmulate(args []*syntax.Word) bool {
	values := make([]string, len(args))
	for i, arg := range args {
		value, ok := literalWord(arg)
		if !ok {
			return false
		}
		values[i] = value
	}

	for _, value := range values {
		if strings.HasPrefix(value, "-") && strings.Contains(value[1:], "c") {
			return false
		}
	}

	local := false
	for _, value := range values {
		if value == "zsh" {
			return local
		}
		if !strings.HasPrefix(value, "-") {
			return false
		}
		if strings.Contains(value[1:], "L") {
			local = true
		}
	}
	return false
}

func enablesLocalOptions(args []*syntax.Word) bool {
	values := make([]string, len(args))
	for i, arg := range args {
		value, ok := literalWord(arg)
		if !ok {
			return false
		}
		values[i] = value
	}

	var localOptionsEnabled *bool
	for i := 0; i < len(values); i++ {
		value := values[i]
		if (value == "+o" || value == "-o") && i+1 < len(values) {
			i++
			switch normalizeOptionName(values[i]) {
			case "localoptions":
				enabled := value == "-o"
				localOptionsEnabled = &enabled
			case "nolocaloptions":
				enabled := value == "+o"
				localOptionsEnabled = &enabled
			}
			continue
		}
		switch normalizeOptionName(value) {
		case "localoptions":
			enabled := true
			localOptionsEnabled = &enabled
		case "nolocaloptions":
			enabled := false
			localOptionsEnabled = &enabled
		}
	}
	return localOptionsEnabled != nil && *localOptionsEnabled
}

func normalizeOptionName(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "_", ""))
}
