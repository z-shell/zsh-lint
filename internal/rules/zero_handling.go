package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"mvdan.cc/sh/v3/syntax"
)

// ZeroHandling reports uninitialized top-level usage of `$0` in plugin entrypoints.
//
// ID: `plugin/zero-handling`
//
// Name: Zero-handling idiom in plugin entrypoint
//
// Summary: Reports direct uses of `$0` at the top level of configured plugin
// and Zi annex sourced-library entrypoints. Configured analysis also reports
// assignment to special parameter `0`, because sourced entrypoints must not
// replace caller state. Unconfigured analysis retains the legacy path heuristic.
//
// Why: When a Zsh plugin is sourced, positional parameter `$0` evaluates to the
// name of the shell (`zsh` or `-zsh`) rather than the path of the sourced script,
// unless `FUNCTION_ARGZERO` is active. Configured plugin entrypoints must
// resolve the source path with prompt expansion `${(%):-%N}` or
// `${(%):-%x}` (optionally with `$ZERO` fallback) and pass it into a
// localized scope instead of assigning to caller-visible special parameter
// `0`.
// See https://wiki.zshell.dev/community/zsh_plugin_standard#zero-handling.
//
// Bad:
//
//	fpath+=( "${0:h}/functions" )
//
// Good (configured sourced entrypoint):
//
//	() {
//	  builtin emulate -L zsh
//	  local -r source_path=${1:a}
//	  local -r plugin_dir=${source_path:h}
//	  fpath+=( "${plugin_dir}/functions" )
//	} "${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}"
//
// Severity: Warning. Deriving paths from uninitialized `$0` can load the
// wrong path; assigning to `0` can replace caller state or fail when
// `POSIX_ARGZERO` makes it read-only.
//
// False positives: Scripts intended solely for direct execution (not sourcing)
// or functions where `$0` refers to the function name. Suppress with a reason.
//
// Suppression: Use
// `# zsh-lint disable=plugin/zero-handling -- <reason>` on the finding line or
// immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: z-a-meta-plugins, zsh-fancy-completions, and zsh-eza use
// the caller-preserving anonymous-function argument pattern.
type ZeroHandling struct{}

func (ZeroHandling) ID() diag.RuleID {
	return "plugin/zero-handling"
}

func (ZeroHandling) Name() string {
	return "Zero-handling idiom in plugin entrypoint"
}

func (rule ZeroHandling) Analyze(ctx *analyzer.Context, node syntax.Node) {
	file, ok := node.(*syntax.File)
	if !ok || !sourcedPluginRuleApplies(ctx) {
		return
	}

	zeroInitialized := false

	for _, stmt := range file.Stmts {
		if stmt == nil {
			continue
		}

		if ctx.Source.Configured() && reportConfiguredZeroAssignment(ctx, stmt, rule.ID()) {
			// Avoid cascading path-use findings after the primary caller-state
			// violation on the assignment itself.
			zeroInitialized = true
			continue
		}

		// Preserve the legacy unconfigured initialization contract.
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

func reportConfiguredZeroAssignment(ctx *analyzer.Context, stmt *syntax.Stmt, ruleID diag.RuleID) bool {
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok {
		return false
	}
	reported := false
	for _, assign := range call.Assigns {
		if assign != nil && assign.Name != nil && assign.Name.Value == "0" {
			ctx.Report(assign.Pos(), assign.End(), ruleID, diag.Warning,
				"Do not assign to special parameter '0' in a sourced entrypoint; pass the resolved source path into a localized anonymous function")
			reported = true
		}
	}
	for _, arg := range call.Args {
		if isZeroAssignmentWord(arg) {
			ctx.Report(arg.Pos(), arg.End(), ruleID, diag.Warning,
				"Do not assign to special parameter '0' in a sourced entrypoint; pass the resolved source path into a localized anonymous function")
			reported = true
		}
	}
	return reported
}

func sourcedPluginRuleApplies(ctx *analyzer.Context) bool {
	if ctx.Source.Configured() {
		return configuredPluginSource(ctx.Source, projectconfig.ProfileSourcedLibrary)
	}
	return !hasFunctionsPathSegment(ctx.FilePath)
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
			if lit.Value == "%N" || lit.Value == "%x" {
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
