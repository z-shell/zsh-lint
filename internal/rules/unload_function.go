package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"mvdan.cc/sh/v3/syntax"
)

// UnloadFunction checks plugin entrypoints for lifecycle unload conventions.
//
// ID: `plugin/unload-function`
//
// Name: Unload function convention and hygiene
//
// Summary: Checks configured plugin and Zi annex sourced-library entrypoints
// that register persistent shell hooks or widgets for a namespaced unload
// function (`*_plugin_unload`), and checks that defined unload functions
// cleanly unfunction themselves upon completion. This remains a per-file
// check: project-wide registration and cleanup matching requires aggregation.
// Unconfigured analysis retains the legacy path heuristic.
//
// Why: The Zsh Plugin Standard specifies that plugins with persistent side
// effects (hooks via `add-zsh-hook`, line-editor widgets via
// `add-zle-hook-widget` or `zle -N`) should provide a namespaced
// `*_plugin_unload` function so plugin managers and users can cleanly unload the
// plugin without restarting the shell. The unload function must explicitly
// remove only its own resources and unfunction itself upon completion.
// See https://wiki.zshell.dev/community/zsh_plugin_standard#lifecycle-and-resource-ownership.
//
// Bad:
//
//	# Plugin installs hook but provides no unload function
//	add-zsh-hook precmd _my_precmd
//
// Good:
//
//	add-zsh-hook precmd _my_precmd
//
//	my_plugin_unload() {
//	  emulate -L zsh
//	  autoload -Uz add-zsh-hook
//	  add-zsh-hook -d precmd _my_precmd
//	  unfunction _my_precmd my_plugin_unload
//	}
//
// Severity: Hint. Missing unload functions or self-unfunction in unload definitions
// are lifecycle recommendations. Indiscriminate function wiping is a Warning.
//
// False positives: Plugins intended only for static, once-per-session loading
// without unload support. Suppress with a reason.
//
// Suppression: Use
// `# zsh-lint disable=plugin/unload-function -- <reason>` on the finding line or
// immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: `zsh-fancy-completions/lib/state.zsh:72` implements
// `zsh-fancy-completions_plugin_unload` with full resource restoration and
// self-unfunction.
type UnloadFunction struct{}

func (UnloadFunction) ID() diag.RuleID {
	return "plugin/unload-function"
}

func (UnloadFunction) Name() string {
	return "Unload function convention and hygiene"
}

func (rule UnloadFunction) Analyze(ctx *analyzer.Context, node syntax.Node) {
	file, ok := node.(*syntax.File)
	if !ok || !sourcedPluginRuleApplies(ctx) {
		return
	}

	var hookRegistrations []*syntax.CallExpr
	var unloadFuncs []*syntax.FuncDecl

	// Traverse AST to collect hook registrations and unload function declarations
	syntax.Walk(file, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.FuncDecl:
			if x.Name != nil && strings.HasSuffix(x.Name.Value, "_plugin_unload") {
				unloadFuncs = append(unloadFuncs, x)
			}
		case *syntax.CallExpr:
			if isHookRegistrationCall(x) {
				hookRegistrations = append(hookRegistrations, x)
			}
		}
		return true
	})

	// Check if hooks are registered without any unload function in the file
	if len(hookRegistrations) > 0 && len(unloadFuncs) == 0 {
		for _, call := range hookRegistrations {
			ctx.Report(
				call.Pos(),
				call.End(),
				rule.ID(),
				diag.Hint,
				"Plugin registers persistent hooks or widgets but defines no '<name>_plugin_unload' function",
			)
		}
	}

	// Inspect hygiene of defined unload functions
	for _, fn := range unloadFuncs {
		checkUnloadFunctionHygiene(ctx, fn, rule.ID())
	}
}

func isHookRegistrationCall(call *syntax.CallExpr) bool {
	if call == nil || len(call.Args) == 0 {
		return false
	}
	cmdName := getWordLiteral(call.Args[0])
	if cmdName == "add-zsh-hook" || cmdName == "add-zle-hook-widget" {
		// Ignore deregistration (-d / -D)
		for _, arg := range call.Args[1:] {
			lit := getWordLiteral(arg)
			if lit == "-d" || lit == "-D" {
				return false
			}
		}
		return true
	}
	if cmdName == "zle" {
		hasNew := false
		for _, arg := range call.Args[1:] {
			switch getWordLiteral(arg) {
			case "-D":
				return false
			case "-N":
				hasNew = true
			}
		}
		return hasNew
	}
	return false
}

func checkUnloadFunctionHygiene(ctx *analyzer.Context, fn *syntax.FuncDecl, ruleID diag.RuleID) {
	if fn == nil || fn.Name == nil {
		return
	}
	fnName := fn.Name.Value
	selfUnfunctionFound := false

	syntax.Walk(fn.Body, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		cmdName := getWordLiteral(call.Args[0])
		if cmdName == "unfunction" {
			for _, arg := range call.Args[1:] {
				if isSelfUnfunctionArg(arg, fnName) {
					selfUnfunctionFound = true
				}
				if isIndiscriminateFunctionsWipe(arg) {
					ctx.Report(
						call.Pos(),
						call.End(),
						ruleID,
						diag.Warning,
						"Do not wipe global functions indiscriminately; remove only plugin-owned functions explicitly",
					)
				}
			}
		}
		return true
	})

	if !selfUnfunctionFound {
		ctx.Report(
			fn.Pos(),
			fn.End(),
			ruleID,
			diag.Hint,
			"Unload function '"+fnName+"' should unfunction itself upon completion",
		)
	}
}

func isSelfUnfunctionArg(w *syntax.Word, fnName string) bool {
	if w == nil {
		return false
	}
	argLit := getWordLiteral(w)
	if argLit == fnName {
		return true
	}
	return wordIsExactParameter(w, "0") || wordIsExactParameter(w, fnName)
}

func wordIsExactParameter(w *syntax.Word, name string) bool {
	if w == nil || len(w.Parts) != 1 {
		return false
	}
	part := w.Parts[0]
	if quoted, ok := part.(*syntax.DblQuoted); ok {
		if len(quoted.Parts) != 1 {
			return false
		}
		part = quoted.Parts[0]
	}
	param, ok := part.(*syntax.ParamExp)
	return ok && param.Param != nil && param.Param.Value == name &&
		param.Flags == nil && !param.Excl && !param.Length && !param.Width &&
		!param.IsSet && param.NestedParam == nil && param.Index == nil &&
		len(param.Modifiers) == 0 && param.Slice == nil && param.Repl == nil &&
		param.Names == 0 && param.Exp == nil
}

func isIndiscriminateFunctionsWipe(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	found := false
	syntax.Walk(w, func(n syntax.Node) bool {
		if pe, ok := n.(*syntax.ParamExp); ok {
			if pe.Param != nil && pe.Param.Value == "functions" {
				found = true
				return false
			}
		}
		if lit, ok := n.(*syntax.Lit); ok {
			if strings.Contains(lit.Value, "functions") && (strings.Contains(lit.Value, "(k)") || strings.Contains(lit.Value, "(kv)")) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func getWordLiteral(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range w.Parts {
		if lit, ok := part.(*syntax.Lit); ok {
			sb.WriteString(lit.Value)
		}
	}
	return sb.String()
}
