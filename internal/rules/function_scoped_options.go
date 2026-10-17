package rules

import (
	"path"
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// FunctionScopedOptions reports autoloadable function files that execute logic
// before localizing inherited shell options.
//
// ID: `plugin/function-scoped-options`
//
// Name: Function files should scope shell options
//
// Summary: For configured plugin and Zi annex projects, reports autoload
// function sources when neither `builtin emulate -L zsh` nor
// `setopt local_options` appears before the first non-guard top-level
// statement. Unconfigured analysis retains the legacy `functions` path
// heuristic.
//
// Why: Zsh functions inherit the caller's option state. The Zsh manual
// documents `LOCAL_OPTIONS` as restoring options on function return, while
// `emulate -L` both selects Zsh emulation and makes option changes local.
// Without either form, options such as `SH_WORD_SPLIT`, `KSH_ARRAYS`, or
// `NO_EXTENDED_GLOB` can silently change function behavior.
// See https://zsh.sourceforge.io/Doc/Release/Options.html#index-LOCAL_005fOPTIONS
// and
// https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html#index-emulate.
// Autoload file bodies and single same-name definitions follow
// https://zsh.sourceforge.io/Doc/Release/Functions.html#Autoloading-Functions.
// First-line `#compdef` callbacks with underscore names run in the completion
// system's option environment; see
// https://zsh.sourceforge.io/Doc/Release/Completion-System.html#Autoloaded-files.
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
// scope. A single same-name wrapper is checked inside its body; bare autoload
// bodies still need scoping. Underscore names alone and `#autoload` tags do
// not exempt files. A contiguous prefix of recognized `condition || return`
// or single-return `if` guards is accepted before scoping. With `emulate -L zsh`,
// the exact trailing `${=${options[xtrace]:#off}:+-o xtrace}` idiom is accepted;
// other dynamic arguments remain findings.
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
	if !ok || !functionScopedOptionsApplies(ctx) || isCompletionCallback(ctx) {
		return
	}

	stmts := file.Stmts
	// Zsh also accepts an autoload file containing only a definition of the
	// function being loaded. Inspect that body, not the declaration itself.
	// Initialization followed by a definition is a different autoload form.
	if len(stmts) == 1 && !hasUnsafeGuardEffect(stmts[0]) {
		if fn, ok := stmts[0].Cmd.(*syntax.FuncDecl); ok && fn.Name != nil &&
			fn.Name.Value == path.Base(strings.ReplaceAll(ctx.FilePath, `\`, "/")) &&
			len(fn.Names) == 0 && fn.Body != nil && !hasUnsafeGuardEffect(fn.Body) {
			if block, ok := fn.Body.Cmd.(*syntax.Block); ok {
				stmts = block.Stmts
			}
		}
	}

	for _, stmt := range stmts {
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

func isCompletionCallback(ctx *analyzer.Context) bool {
	// An underscore alone does not imply compsys execution. A first-line
	// #compdef tag registers a completion callback, whose entry point supplies
	// the option environment. #autoload helpers have no such guarantee.
	if !strings.HasPrefix(path.Base(strings.ReplaceAll(ctx.FilePath, `\`, "/")), "_") || len(ctx.File.Lines()) == 0 {
		return false
	}
	line := ctx.File.Lines()[0]
	fields := strings.Fields(line)
	return strings.HasPrefix(line, "#compdef") && len(fields) > 1 && fields[0] == "#compdef"
}

func functionScopedOptionsApplies(ctx *analyzer.Context) bool {
	if ctx.Source.Configured() {
		return configuredPluginSource(ctx.Source, projectconfig.ProfileAutoloadFunction)
	}
	return hasFunctionsPathSegment(ctx.FilePath)
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

	command, args := effectiveCommand(call)

	switch command {
	case "emulate":
		return isLocalZshEmulate(args)
	case "setopt":
		return enablesLocalOptions(args)
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

	// exec return replaces the shell instead of returning from the function.
	resolved := resolveCommand(call)
	if resolved.exec || resolved.external || resolved.name != "return" {
		return false
	}
	args := resolved.args
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
	var values []string
	shellSeen := false
	preservesXtrace := false
	for i, arg := range args {
		value, ok := literalWord(arg)
		if !ok {
			// Accept only this fixed expansion, which yields either no words or
			// exactly "-o xtrace". Never accept arbitrary dynamic flags, nor an
			// expansion where a preceding -o/+o needs an option-name argument.
			if !shellSeen || i != len(args)-1 || (len(values) > 0 && (values[len(values)-1] == "-o" || values[len(values)-1] == "+o")) || !isXtracePreservingArgument(arg) {
				return false
			}
			preservesXtrace = true
			continue
		}
		values = append(values, value)
		shellSeen = shellSeen || value == "zsh"
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
		if !strings.HasPrefix(value, "-") || (preservesXtrace && strings.Trim(value[1:], "LR") != "") {
			return false
		}
		if strings.Contains(value[1:], "L") {
			local = true
		}
	}
	return false
}

func isXtracePreservingArgument(word *syntax.Word) bool {
	if word == nil || len(word.Parts) != 1 {
		return false
	}
	if _, ok := word.Parts[0].(*syntax.ParamExp); !ok {
		return false
	}
	var text strings.Builder
	if err := syntax.NewPrinter().Print(&text, word); err != nil {
		return false
	}
	return text.String() == "${=${options[xtrace]:#off}:+-o xtrace}"
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
