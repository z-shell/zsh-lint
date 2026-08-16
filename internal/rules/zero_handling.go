package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"mvdan.cc/sh/v3/syntax"
)

// ZeroHandling reports uninitialized top-level usage of `$0` in plugin entrypoints.
//
// ID: `plugin/zero-handling`
//
// Name: Zero-handling idiom in plugin entrypoint
//
// Summary: Reports direct uses of `$0` at the top level of plugin scripts
// before `$0` has been initialized using prompt expansions (`${(%):-%N}` or
// `${(%):-%x}`).
//
// Why: When a Zsh plugin is sourced, positional parameter `$0` evaluates to the
// name of the shell (`zsh` or `-zsh`) rather than the path of the sourced script,
// unless `FUNCTION_ARGZERO` is active. Plugin entrypoints must initialize `$0`
// using prompt expansion `${(%):-%N}` or `${(%):-%x}` (optionally with `$ZERO`
// fallback) before using `$0` to derive directories or autoload paths.
// See https://wiki.zshell.dev/community/zsh_plugin_standard#zero-handling.
//
// Bad:
//
//	fpath+=( "${0:h}/functions" )
//
// Good:
//
//	0="${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}"
//	0="${${(M)0:#/*}:-$PWD/$0}"
//	fpath+=( "${0:h}/functions" )
//
// Severity: Warning. Deriving paths from uninitialized `$0` in a sourced plugin
// leads to incorrect directory paths or runtime loading failures.
//
// False positives: Scripts intended solely for direct execution (not sourcing)
// or functions where `$0` refers to the function name. Suppress with a reason.
//
// Suppression: Use
// `# zsh-lint disable=plugin/zero-handling -- <reason>` on the finding line or
// immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: `z-a-meta-plugins/z-a-meta-plugins.plugin.zsh:7` uses the
// compliant `0="${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}"` idiom before
// referencing `${0:h}` on line 12.
type ZeroHandling struct{}

func (ZeroHandling) ID() diag.RuleID {
	return "plugin/zero-handling"
}

func (ZeroHandling) Name() string {
	return "Zero-handling idiom in plugin entrypoint"
}

func (rule ZeroHandling) Analyze(ctx *analyzer.Context, node syntax.Node) {
	file, ok := node.(*syntax.File)
	if !ok || hasFunctionsPathSegment(ctx.FilePath) {
		return
	}

	zeroInitialized := false

	for _, stmt := range file.Stmts {
		if stmt == nil {
			continue
		}

		// Check if this statement initializes 0 with prompt expansion or $ZERO
		if isZeroInitializationStatement(stmt) {
			zeroInitialized = true
			continue
		}

		// Inspect statement for top-level $0 references (excluding function definitions)
		if !zeroInitialized {
			checkUninitializedZeroInStmt(ctx, stmt, rule.ID())
		}
	}
}

func isZeroInitializationStatement(stmt *syntax.Stmt) bool {
	if stmt == nil {
		return false
	}

	// 1. Direct assignment: 0=... (parsed as CallExpr word with "0=" prefix or Assign)
	if call, ok := stmt.Cmd.(*syntax.CallExpr); ok {
		for _, assign := range call.Assigns {
			if assign.Name != nil && assign.Name.Value == "0" {
				if hasPromptExpansionOrZeroVar(assign.Value) {
					return true
				}
			}
		}
		for _, arg := range call.Args {
			if arg != nil && isZeroAssignmentWord(arg) {
				return true
			}
		}
	}

	// 2. Declaration: typeset/local/declare ... _SOURCE=${(%):-%N}
	if decl, ok := stmt.Cmd.(*syntax.DeclClause); ok {
		for _, assign := range decl.Args {
			if assign != nil && hasPromptExpansionOrZeroVar(assign.Value) {
				return true
			}
		}
	}

	return false
}

func isZeroAssignmentWord(word *syntax.Word) bool {
	if word == nil || len(word.Parts) == 0 {
		return false
	}
	if lit, ok := word.Parts[0].(*syntax.Lit); ok {
		if strings.HasPrefix(lit.Value, "0=") {
			return true
		}
	}
	return false
}

func hasPromptExpansionOrZeroVar(word *syntax.Word) bool {
	return hasPromptExpansionOrZeroInWord(word)
}

func hasPromptExpansionOrZeroInWord(word *syntax.Word) bool {
	if word == nil {
		return false
	}
	found := false
	syntax.Walk(word, func(n syntax.Node) bool {
		if pe, ok := n.(*syntax.ParamExp); ok {
			if pe.Param != nil && pe.Param.Value == "ZERO" {
				found = true
				return false
			}
		}
		if lit, ok := n.(*syntax.Lit); ok {
			if strings.Contains(lit.Value, "%N") || strings.Contains(lit.Value, "%x") || strings.Contains(lit.Value, "ZERO") {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func checkUninitializedZeroInStmt(ctx *analyzer.Context, stmt *syntax.Stmt, ruleID diag.RuleID) {
	if stmt == nil {
		return
	}

	// Skip function declarations (where $0 refers to function name)
	if _, ok := stmt.Cmd.(*syntax.FuncDecl); ok {
		return
	}

	syntax.Walk(stmt, func(n syntax.Node) bool {
		// Stop traversing into nested function declarations
		if _, ok := n.(*syntax.FuncDecl); ok {
			return false
		}

		if pe, ok := n.(*syntax.ParamExp); ok {
			if pe.Param != nil && pe.Param.Value == "0" {
				// If this is the initial 0= assignment itself, do not report it
				if isInitializingParamExp(stmt, pe) {
					return false
				}
				ctx.Report(
					pe.Pos(),
					pe.End(),
					ruleID,
					diag.Warning,
					"Initialize '$0' with '${(%):-%N}' or '${(%):-%x}' prompt expansion before deriving paths from '$0'",
				)
			}
		}
		return true
	})
}

func isInitializingParamExp(stmt *syntax.Stmt, pe *syntax.ParamExp) bool {
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok {
		return false
	}
	for _, assign := range call.Assigns {
		if assign.Name != nil && assign.Name.Value == "0" {
			if assign.Value != nil && hasPromptExpansionOrZeroInWord(assign.Value) {
				return true
			}
		}
	}
	if len(call.Args) > 0 {
		firstArg := call.Args[0]
		if len(firstArg.Parts) > 0 {
			if lit, ok := firstArg.Parts[0].(*syntax.Lit); ok && strings.HasPrefix(lit.Value, "0=") {
				if hasPromptExpansionOrZeroInWord(firstArg) {
					return true
				}
			}
		}
	}
	return false
}
