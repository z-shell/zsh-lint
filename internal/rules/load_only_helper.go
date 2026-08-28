package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"mvdan.cc/sh/v3/syntax"
)

// LoadOnlyHelper reports private setup-shaped functions that are called only
// during project loading but remain defined afterward.
//
// ID: `plugin/load-only-helper`
//
// Name: Remove load-only helper functions
//
// Summary: Reports a provably load-only private helper that remains defined
// after initialization.
//
// Why: Every persistent function expands the plugin's process-wide public or
// private shell surface and its unload obligations. Standard 2 requires a
// plugin to remove helpers that exist only to initialize persistent state.
// See https://wiki.zshell.dev/community/zsh_plugin_standard#lifecycle-and-resource-ownership.
//
// Bad:
//
//	_example_setup() { typeset -g _example_ready=1; }
//	_example_setup
//
// Good:
//
//	_example_setup() { typeset -g _example_ready=1; }
//	_example_setup
//	unfunction _example_setup
//
// Severity: Hint. A retained helper is namespace and lifecycle debt, not a
// direct runtime failure.
//
// False positives: The rule is intentionally conservative. It requires a
// private project prefix, a setup/init/load/register/bootstrap role word, a
// load-scope call, no call from a persistent function, and no exact load-scope
// `unfunction`. Helpers whose future use cannot be disproved remain silent.
//
// Suppression: Use
// `# zsh-lint disable=plugin/load-only-helper -- <reason>` on the declaration
// line or immediately before it.
//
// Corpus evidence: The repository fixtures model load-only initialization;
// issue #187 adds this rule before wider organization migration so new helpers
// do not become persistent debt.
type LoadOnlyHelper struct{}

func (LoadOnlyHelper) ID() diag.RuleID { return "plugin/load-only-helper" }

func (LoadOnlyHelper) Name() string { return "Remove load-only helper functions" }

func (LoadOnlyHelper) Analyze(_ *analyzer.Context, _ syntax.Node) {}

func (rule LoadOnlyHelper) AnalyzeProject(ctx *analyzer.ProjectContext) {
	type declaration struct {
		input analyzer.ProjectInput
		name  *syntax.Lit
	}
	declarations := make(map[string]declaration)
	loadCalls := make(map[string]bool)
	persistentCalls := make(map[string]bool)
	unfunctions := make(map[string]bool)
	identifier := ""

	for _, input := range ctx.Inputs {
		if !portablePluginSource(input.Source) {
			continue
		}
		identifier = input.Source.ProjectIdentifier
		syntax.Walk(input.File.AST(), func(node syntax.Node) bool {
			switch value := node.(type) {
			case *syntax.FuncDecl:
				names := functionDeclarationNames(value)
				if len(names) == 0 {
					return true
				}
				for _, name := range names {
					declarations[name.Value] = declaration{input: input, name: name}
				}
				syntax.Walk(value.Body, func(bodyNode syntax.Node) bool {
					call, ok := bodyNode.(*syntax.CallExpr)
					if !ok {
						return true
					}
					if name, ok := calledFunctionName(call); ok {
						persistentCalls[name] = true
					}
					return true
				})
				return false
			case *syntax.CallExpr:
				name, ok := calledFunctionName(value)
				if !ok {
					return true
				}
				if name == "unfunction" {
					for _, argument := range value.Args[1:] {
						if literal, ok := literalWord(argument); ok {
							unfunctions[literal] = true
						}
					}
					return true
				}
				loadCalls[name] = true
			}
			return true
		})
	}

	privatePrefix := "_" + projectconfig.ShellPrefix(identifier) + "_"
	for name, declared := range declarations {
		if !strings.HasPrefix(name, privatePrefix) || !loadOnlyRoleName(name) ||
			!loadCalls[name] || persistentCalls[name] || unfunctions[name] {
			continue
		}
		ctx.Report(
			declared.input,
			declared.name.Pos(),
			declared.name.End(),
			rule.ID(),
			diag.Hint,
			"Private load-only helper '"+name+"' remains defined; unfunction it after initialization or use a localized anonymous loader",
		)
	}
}

func calledFunctionName(call *syntax.CallExpr) (string, bool) {
	if call == nil || len(call.Args) == 0 {
		return "", false
	}
	return literalWord(call.Args[0])
}

func loadOnlyRoleName(name string) bool {
	for _, role := range []string{"setup", "init", "load", "register", "bootstrap"} {
		if strings.HasSuffix(name, "_"+role) || strings.Contains(name, "_"+role+"_") {
			return true
		}
	}
	return false
}
