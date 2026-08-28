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
// basenames, when they do not use the declared project identifier.
//
// Why: The Zsh Plugin Standard requires persistent shell-visible names to use
// a stable project identifier. Namespacing prevents collisions when unrelated
// plugins share one shell process. Native completions keep Zsh's `_command`
// convention. Public functions use `example_...`; private functions use
// `_example_...`. Symbolic role prefixes are deliberately not accepted by the
// version 2 project profile. See
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
// False positives: The rule runs only with version 2 project configuration,
// a plugin or Zi annex project kind, a sourced-library or autoload-function
// source profile, and one explicit project identifier. Completion
// basenames use `_command` only when the source role is `completion`.
// Repository names and directory names are never inferred as identifiers.
//
// Suppression: Use
// `# zsh-lint disable=plugin/function-namespace -- <reason>` on a declaration
// line or immediately before it. For an autoload basename finding, put the
// standalone directive in the file header before any source code.
//
// Corpus evidence: The configured organization corpus records nonconforming
// symbolic and multi-prefix names as issue-backed migration findings until
// each owning project adopts the version 2 portable contract.
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
		if namespacedFunction(name.Value, ctx.Source.ProjectIdentifier) {
			continue
		}
		ctx.Report(
			name.Pos(),
			name.End(),
			rule.ID(),
			diag.Hint,
			functionNamespaceMessage("Function", name.Value, ctx.Source.ProjectIdentifier),
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
	if namespacedFunction(name, ctx.Source.ProjectIdentifier) {
		return
	}
	ctx.Report(
		syntax.Pos{},
		syntax.Pos{},
		rule.ID(),
		diag.Hint,
		functionNamespaceMessage("Autoload function", name, ctx.Source.ProjectIdentifier),
	)
}

func functionNamespaceEnabled(source projectconfig.SourceContext) bool {
	if source.ConfigVersion != projectconfig.CurrentVersion || source.ProjectIdentifier == "" {
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

func namespacedFunction(name, identifier string) bool {
	prefix := projectconfig.ShellPrefix(identifier) + "_"
	return strings.HasPrefix(name, prefix) || strings.HasPrefix(name, "_"+prefix)
}

func functionNamespaceMessage(kind, name, identifier string) string {
	prefix := projectconfig.ShellPrefix(identifier) + "_"
	return fmt.Sprintf(
		"%s name %q must use public prefix %s or private prefix %s",
		kind,
		name,
		strconv.Quote(prefix),
		strconv.Quote("_"+prefix),
	)
}
