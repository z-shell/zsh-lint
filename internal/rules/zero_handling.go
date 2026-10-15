package rules

import (
	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// ZeroHandling reports caller-dependent top-level usage of `$0` in configured plugin entrypoints.
//
// ID: `plugin/zero-handling`
//
// Name: Zero-handling idiom in plugin entrypoint
//
// Summary: Reports direct uses of `$0` at the top level of configured plugin
// and Zi annex sourced-library entrypoints. Configured analysis also reports
// assignment to special parameter `0`, because sourced entrypoints must not
// replace caller state. Unconfigured sources remain silent: their execution
// profile and the caller's option state are unknown.
// The `${0:#$ZSH_ARGZERO}` filter is accepted.
//
// Why: Native Zsh enables `FUNCTION_ARGZERO` by default, temporarily setting
// `$0` to the sourced filename or function name on entry. A caller that disables
// it can leave `$0` holding the shell or caller name instead; the option state
// at entry cannot be inferred from the source alone. See
// https://zsh.sourceforge.io/Doc/Release/Options.html#Scripts-and-Functions.
// Configured plugin entrypoints must
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
// False positives: A native sourced script may legitimately use `$0` as its
// filename, and an autoload body may use it as its function name. Those uses
// are not plugin-entrypoint contract violations without an explicit configured
// sourced-library profile. Function definitions and other configured profiles
// remain silent.
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
	if !ok || !configuredPluginSource(ctx.Source, projectconfig.ProfileSourcedLibrary) {
		return
	}

	analyzeZeroStatements(ctx, file.Stmts, rule.ID(), false)
}

func analyzeZeroStatements(ctx *analyzer.Context, stmts []*syntax.Stmt, ruleID diag.RuleID, zeroInitialized bool) bool {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}

		// Compound-command redirections expand before their bodies execute.
		switch stmt.Cmd.(type) {
		case *syntax.Block, *syntax.IfClause:
			if !zeroInitialized {
				for _, redir := range stmt.Redirs {
					if redir != nil {
						checkUninitializedZero(ctx, redir, ruleID)
					}
				}
			}
		}
		initialized := zeroInitialized
		switch command := stmt.Cmd.(type) {
		case *syntax.Block:
			initialized = analyzeZeroStatements(ctx, command.Stmts, ruleID, zeroInitialized)
		case *syntax.IfClause:
			initialized = analyzeZeroIf(ctx, command, ruleID, zeroInitialized)
		default:
			if ctx.Source.Configured() && reportConfiguredZeroAssignment(ctx, stmt, ruleID) {
				// Avoid cascading path-use findings after the primary caller-state
				// violation on the assignment itself.
				initialized = true
			} else if isZeroInitializationStatement(stmt) {
				// Preserve the legacy unconfigured initialization contract.
				initialized = true
			} else if !zeroInitialized {
				checkUninitializedZero(ctx, stmt, ruleID)
			}
		}
		if !hasUnsafeStatementEffect(stmt) {
			zeroInitialized = initialized
		}
	}
	return zeroInitialized
}

func analyzeZeroIf(ctx *analyzer.Context, clause *syntax.IfClause, ruleID diag.RuleID, zeroInitialized bool) bool {
	zeroInitialized = analyzeZeroStatements(ctx, clause.Cond, ruleID, zeroInitialized)
	thenInitialized := analyzeZeroStatements(ctx, clause.Then, ruleID, zeroInitialized)
	if len(clause.Cond) == 0 {
		return thenInitialized // else has no condition
	}
	if clause.Else == nil {
		// A missing else leaves the incoming state possible after the if.
		return zeroInitialized
	}
	elseInitialized := analyzeZeroIf(ctx, clause.Else, ruleID, zeroInitialized)
	return thenInitialized && elseInitialized
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

	// 1. Direct assignment: 0=...
	if call, ok := stmt.Cmd.(*syntax.CallExpr); ok {
		for _, assign := range call.Assigns {
			if assign.Name != nil && assign.Name.Value == "0" {
				if hasPromptExpansionOrZeroVar(assign.Value) {
					return true
				}
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

func checkUninitializedZero(ctx *analyzer.Context, node syntax.Node, ruleID diag.RuleID) {
	if node == nil {
		return
	}

	syntax.Walk(node, func(n syntax.Node) bool {
		// Stop traversing into nested function declarations
		if _, ok := n.(*syntax.FuncDecl); ok {
			return false
		}

		if pe, ok := n.(*syntax.ParamExp); ok {
			if pe.Param != nil && pe.Param.Value == "0" {
				if isArgzeroFilter(pe) {
					return true
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

func isArgzeroFilter(pe *syntax.ParamExp) bool {
	return pe.Flags == nil && pe.Index == nil && !pe.Length &&
		pe.Exp != nil && pe.Exp.Op == syntax.MatchEmpty &&
		wordIsExactParameter(pe.Exp.Word, "ZSH_ARGZERO")
}
