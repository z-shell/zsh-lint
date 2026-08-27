package rules

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"mvdan.cc/sh/v3/syntax"
)

// FunctionNamespace reports persistent plugin function names outside the
// configured stable project namespace.
//
// ID: `plugin/function-namespace`
//
// Name: Project-owned function namespace
//
// Summary: Reports named functions in configured plugin and Zi annex source
// files, plus effective function names derived from configured autoload file
// basenames, when they do not use an explicitly declared project namespace.
//
// Why: The Zsh Plugin Standard requires persistent shell-visible names to use
// a stable project identifier. Namespacing prevents collisions when unrelated
// plugins share one shell process. Native completions keep Zsh's `_command`
// convention, and the established `.`, `→`, `+`, `/`, and `@` role prefixes
// remain valid before the project namespace. The portable `_example_callback`
// form is also accepted. See
// https://wiki.zshell.dev/community/zsh_plugin_standard#names-and-persistent-state
// and
// https://wiki.zshell.dev/community/zsh_plugin_standard#standard-function-name-space-prefixes.
//
// Bad:
//
//	refresh() { builtin emulate -L zsh; }
//
// Good:
//
//	example_refresh() { builtin emulate -L zsh; }
//	_example_precmd() { builtin emulate -L zsh; }
//
// Severity: Hint. A mismatched namespace is an interoperability and hygiene
// concern, not invalid Zsh syntax.
//
// False positives: The rule runs only with version 1 project configuration,
// a plugin or Zi annex project kind, a sourced-library or autoload-function
// source profile, and at least one explicit function namespace. Completion
// basenames use `_command` only when the source role is `completion`.
// Repository names and directory names are never inferred as namespaces.
//
// Suppression: Use
// `# zsh-lint disable=plugin/function-namespace -- <reason>` on a declaration
// line or immediately before it. For an autoload basename finding, put the
// standalone directive in the file header before any source code.
//
// Corpus evidence: `zsh-eza/functions/.zsh-eza` and the
// `z-a-meta-plugins/functions/.za-meta-plugins-*` files use their configured
// namespaces. `zsh-fancy-completions/lib/state.zsh` uses its explicit `zfc`
// namespace. Its five legacy dot-prefixed autoload basenames (`.complete_menu`,
// `.completion-prediction`, `.expand-or-complete-with-dots`, `.force_rehash`,
// and `.man_glob`) are classified as true-positive Hint findings. Advisory
// rollout does not require renaming those established external interfaces.
type FunctionNamespace struct{}

func (FunctionNamespace) ID() diag.RuleID { return "plugin/function-namespace" }

func (FunctionNamespace) Name() string { return "Project-owned function namespace" }

func (rule FunctionNamespace) Analyze(ctx *analyzer.Context, node syntax.Node) {
	if !functionNamespaceEnabled(ctx.Source) || ctx.Source.Profile != projectconfig.ProfileSourcedLibrary {
		return
	}
	declaration, ok := node.(*syntax.FuncDecl)
	if !ok || declaration == nil {
		return
	}
	for _, name := range functionDeclarationNames(declaration) {
		if namespacedFunction(name.Value, ctx.Source.FunctionNamespaces) {
			continue
		}
		ctx.Report(
			name.Pos(),
			name.End(),
			rule.ID(),
			diag.Hint,
			functionNamespaceMessage("Function", name.Value, ctx.Source.FunctionNamespaces),
		)
	}
}

// AnalyzeFile checks the effective function name supplied by an autoload file
// basename. It deliberately reports an unpositioned diagnostic because no AST
// token represents that name.
func (rule FunctionNamespace) AnalyzeFile(ctx *analyzer.Context) {
	if !functionNamespaceEnabled(ctx.Source) || ctx.Source.Profile != projectconfig.ProfileAutoloadFunction {
		return
	}
	name := filepath.Base(filepath.Clean(ctx.FilePath))
	if ctx.Source.Role == projectconfig.RoleCompletion {
		if strings.HasPrefix(name, "_") && name != "_" {
			return
		}
		ctx.Report(
			syntax.Pos{},
			syntax.Pos{},
			rule.ID(),
			diag.Hint,
			fmt.Sprintf("Autoload completion name %q must use the _command form", name),
		)
		return
	}
	if namespacedFunction(name, ctx.Source.FunctionNamespaces) {
		return
	}
	ctx.Report(
		syntax.Pos{},
		syntax.Pos{},
		rule.ID(),
		diag.Hint,
		functionNamespaceMessage("Autoload function", name, ctx.Source.FunctionNamespaces),
	)
}

func functionNamespaceEnabled(source projectconfig.SourceContext) bool {
	if source.ConfigVersion != projectconfig.CurrentVersion || len(source.FunctionNamespaces) == 0 {
		return false
	}
	return source.ProjectKind == projectconfig.KindPlugin || source.ProjectKind == projectconfig.KindZiAnnex
}

func functionDeclarationNames(declaration *syntax.FuncDecl) []*syntax.Lit {
	if declaration.Name != nil {
		return []*syntax.Lit{declaration.Name}
	}
	return declaration.Names
}

func namespacedFunction(name string, namespaces []string) bool {
	if hasFunctionNamespace(name, namespaces) {
		return true
	}
	for _, prefix := range []string{".", "→", "+", "/", "@", "_"} {
		if strings.HasPrefix(name, prefix) {
			return hasFunctionNamespace(strings.TrimPrefix(name, prefix), namespaces)
		}
	}
	return false
}

func hasFunctionNamespace(name string, namespaces []string) bool {
	for _, namespace := range namespaces {
		if strings.HasPrefix(name, namespace) {
			return true
		}
	}
	return false
}

func functionNamespaceMessage(kind, name string, namespaces []string) string {
	quoted := make([]string, len(namespaces))
	for index, namespace := range namespaces {
		quoted[index] = strconv.Quote(namespace)
	}
	return fmt.Sprintf(
		"%s name %q must use a configured project namespace (%s) after any recognized role prefix",
		kind,
		name,
		strings.Join(quoted, ", "),
	)
}
