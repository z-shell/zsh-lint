package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"mvdan.cc/sh/v3/syntax"
)

// FpathHygiene checks `fpath` modifications in plugin scripts for safety and best practices.
//
// ID: `plugin/fpath-hygiene`
//
// Name: fpath manipulation hygiene in plugin entrypoints
//
// Summary: Detects destructive assignments to `fpath` that overwrite existing
// autoload paths, additions of non-function directories (`bin/`, `tests/`), and
// hardcoded user paths.
//
// Why: Sourced Zsh plugins must not destructively replace `fpath` (which removes
// standard system and user function directories), add hardcoded user machine paths,
// or add non-function directories (`bin/`, `tests/`) that can trigger completion
// security audit warnings (`compaudit`) or namespace collisions. Plugins should
// append or prepend relative `functions/` or `completions/` directories.
// See https://wiki.zshell.dev/community/zsh_plugin_standard#completions-and-compinit-ownership.
//
// Bad:
//
//	# Destructively overwrites existing autoload paths
//	fpath=( "${0:h}/functions" )
//
//	# Adds binary directory to fpath
//	fpath+=( "${0:h}/bin" )
//
// Good:
//
//	fpath+=( "${0:h}/functions" "${0:h}/completions" )
//
//	# Prepending while preserving existing paths
//	fpath=( "${0:h}/functions" $fpath )
//
// Severity: Warning for destructive `fpath` overwrites or additions of `bin`/`tests`
// directories to `fpath`.
//
// False positives: Hermetic test runners or standalone shell bootstrap scripts
// that deliberately isolate `fpath`. Suppress with a reason.
//
// Suppression: Use
// `# zsh-lint disable=plugin/fpath-hygiene -- <reason>` on the finding line or
// immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: `z-a-meta-plugins/z-a-meta-plugins.plugin.zsh:12` uses
// compliant `fpath+=( "${0:h}/functions" )`.
type FpathHygiene struct{}

func (FpathHygiene) ID() diag.RuleID {
	return "plugin/fpath-hygiene"
}

func (FpathHygiene) Name() string {
	return "fpath manipulation hygiene in plugin entrypoints"
}

func (rule FpathHygiene) Analyze(ctx *analyzer.Context, node syntax.Node) {
	call, ok := node.(*syntax.CallExpr)
	if !ok {
		return
	}

	for _, assign := range call.Assigns {
		if assign == nil || assign.Name == nil {
			continue
		}
		varName := assign.Name.Value
		if varName != "fpath" && varName != "FPATH" {
			continue
		}

		// 1. Check for destructive overwrite: fpath=( ... ) without preserving $fpath
		if !assign.Append && assign.Array != nil {
			if !arrayContainsFpathReference(assign.Array) {
				ctx.Report(
					assign.Pos(),
					assign.End(),
					rule.ID(),
					diag.Warning,
					"Destructive assignment to 'fpath' removes existing autoload paths; use 'fpath+=( ... )' or include '$fpath'",
				)
			}
		}

		// 2. Check for invalid or suspicious directory additions
		if assign.Array != nil {
			for _, elem := range assign.Array.Elems {
				if elem != nil && elem.Value != nil {
					checkFpathElement(ctx, elem.Value, rule.ID())
				}
			}
		} else if assign.Value != nil {
			checkFpathElement(ctx, assign.Value, rule.ID())
		}
	}
}

func arrayContainsFpathReference(array *syntax.ArrayExpr) bool {
	if array == nil {
		return false
	}
	found := false
	syntax.Walk(array, func(n syntax.Node) bool {
		if pe, ok := n.(*syntax.ParamExp); ok && pe.Param != nil {
			if pe.Param.Value == "fpath" || pe.Param.Value == "FPATH" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func checkFpathElement(ctx *analyzer.Context, word *syntax.Word, ruleID diag.RuleID) {
	if word == nil {
		return
	}
	var sb strings.Builder
	syntax.Walk(word, func(n syntax.Node) bool {
		if lit, ok := n.(*syntax.Lit); ok {
			sb.WriteString(lit.Value)
		}
		return true
	})
	pathStr := sb.String()

	if strings.HasSuffix(pathStr, "/bin") || strings.HasSuffix(pathStr, "/tests") || strings.HasSuffix(pathStr, "/test") {
		ctx.Report(
			word.Pos(),
			word.End(),
			ruleID,
			diag.Warning,
			"Do not add non-function directories ('bin', 'tests') to 'fpath'",
		)
		return
	}

	if strings.HasPrefix(pathStr, "/home/") || strings.HasPrefix(pathStr, "/Users/") {
		ctx.Report(
			word.Pos(),
			word.End(),
			ruleID,
			diag.Warning,
			"Do not add hardcoded user machine paths to 'fpath'",
		)
		return
	}
}
