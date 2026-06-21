package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"mvdan.cc/sh/v3/syntax"
)

// FunctionScopedOptions reports autoloadable function files that execute logic
// before localizing inherited shell options.
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
