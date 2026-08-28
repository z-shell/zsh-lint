package rules

import (
	"fmt"
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"mvdan.cc/sh/v3/syntax"
)

// SharedPluginsRegistry rejects portable plugin writes to the ownerless shared
// Plugins parameter.
//
// ID: `plugin/shared-plugins-registry`
//
// Name: No shared Plugins registry mutation
//
// Summary: Reports declarations, assignments, and indexed writes to the
// ownerless global `Plugins` parameter in configured portable plugin and Zi
// annex source.
//
// Why: Zsh Plugin Standard 2 requires every persistent parameter to have one
// project owner. A shared global registry has no portable ownership or unload
// contract. Plugin managers can maintain manager-owned registries outside the
// portable plugin source.
// See https://wiki.zshell.dev/community/zsh_plugin_standard#names-and-persistent-state.
//
// Bad:
//
//	typeset -gA Plugins
//	Plugins[example]=$PWD
//
// Good:
//
//	typeset -gA _example_state
//	_example_state[plugin_dir]=$PWD
//
// Severity: Warning. The shared write creates cross-plugin state with no
// stable owner, but managers may provide a similarly named private registry
// outside portable source.
//
// False positives: None in a Standard 2 portable source mapping. Manager-only
// integration belongs in a separately classified source and remains silent.
//
// Suppression: Use
// `# zsh-lint disable=plugin/shared-plugins-registry -- <reason>` on the
// finding line or immediately before it.
//
// Corpus evidence: z-a-meta-plugins#54, zsh-fancy-completions#59, and
// zsh-eza#119 track removal of current configured-corpus writes.
type SharedPluginsRegistry struct{}

func (SharedPluginsRegistry) ID() diag.RuleID { return "plugin/shared-plugins-registry" }

func (SharedPluginsRegistry) Name() string { return "No shared Plugins registry mutation" }

func (rule SharedPluginsRegistry) Analyze(ctx *analyzer.Context, node syntax.Node) {
	file, ok := node.(*syntax.File)
	if !ok || !portablePluginSource(ctx.Source) {
		return
	}

	reported := make(map[*syntax.Assign]bool)
	report := func(assign *syntax.Assign) {
		if assign == nil || reported[assign] {
			return
		}
		name, pos, end, ok := declaredParameter(assign)
		if !ok || name != "Plugins" {
			return
		}
		reported[assign] = true
		ctx.Report(
			pos,
			end,
			rule.ID(),
			diag.Warning,
			"Portable plugin source must not create or mutate the shared 'Plugins' registry; use project-owned private state",
		)
	}

	for _, statement := range file.Stmts {
		syntax.Walk(statement, func(candidate syntax.Node) bool {
			switch value := candidate.(type) {
			case *syntax.FuncDecl:
				rule.analyzeFunction(value, report)
				return false
			case *syntax.DeclClause:
				if value == nil || value.Variant == nil {
					return false
				}
				scan := scanDeclarationOptions(value.Variant.Value, value.Args)
				for _, assign := range value.Args[scan.firstOperand:] {
					report(assign)
				}
				return false
			case *syntax.CallExpr:
				if len(value.Args) == 0 {
					for _, assign := range value.Assigns {
						report(assign)
					}
				}
			}
			return true
		})
	}
}

func (rule SharedPluginsRegistry) analyzeFunction(function *syntax.FuncDecl, report func(*syntax.Assign)) {
	if function == nil || function.Body == nil {
		return
	}

	hasLocalPlugins := false
	syntax.Walk(function.Body, func(candidate syntax.Node) bool {
		switch value := candidate.(type) {
		case *syntax.FuncDecl:
			return false
		case *syntax.DeclClause:
			if value == nil || value.Variant == nil {
				return false
			}
			scan := scanDeclarationOptions(value.Variant.Value, value.Args)
			for _, assign := range value.Args[scan.firstOperand:] {
				name, _, _, ok := declaredParameter(assign)
				if !ok || name != "Plugins" {
					continue
				}
				if scan.mode == declarationModeGlobal {
					report(assign)
				} else {
					hasLocalPlugins = true
				}
			}
			return false
		}
		return true
	})

	if hasLocalPlugins {
		return
	}
	syntax.Walk(function.Body, func(candidate syntax.Node) bool {
		switch value := candidate.(type) {
		case *syntax.FuncDecl, *syntax.DeclClause:
			return false
		case *syntax.CallExpr:
			if len(value.Args) == 0 {
				for _, assign := range value.Assigns {
					report(assign)
				}
			}
		}
		return true
	})
}

// PersistentParameterNamespace reports persistent parameters outside the one
// private project namespace declared by schema version 2.
//
// ID: `plugin/persistent-parameter-namespace`
//
// Name: Private project-owned persistent parameter namespace
//
// Summary: Reports persistent parameters that do not use the configured
// project's `_project_name_...` private namespace.
//
// Why: Shell parameters share one process-wide namespace. Standard 2 routes
// public configuration through `:project-identifier:config` zstyles and
// reserves global parameters for necessary private typed state with one clear
// owner.
// See https://wiki.zshell.dev/community/zsh_plugin_standard#configuration-contract.
//
// Bad:
//
//	typeset -g FEATURE_ENABLED=1
//
// Good:
//
//	typeset -gi _example_feature_enabled=1
//	zstyle ':example:config' feature-enabled yes
//
// Severity: Warning. An unowned global can collide with another plugin or user
// parameter and makes cleanup ambiguous.
//
// False positives: The standard process-wide parameters `path`, `PATH`,
// `fpath`, `FPATH`, and `module_path` are exempt because portable plugins can
// own and restore entries in those shared ordered collections. Dynamic local
// parameters inside functions remain silent unless explicitly declared `-g`.
//
// Suppression: Use
// `# zsh-lint disable=plugin/persistent-parameter-namespace -- <reason>` on
// the finding line or immediately before it.
//
// Corpus evidence: z-a-meta-plugins#54, zsh-fancy-completions#59, and
// zsh-eza#119 track current unnamespaced configured-corpus parameters.
type PersistentParameterNamespace struct{}

func (PersistentParameterNamespace) ID() diag.RuleID {
	return "plugin/persistent-parameter-namespace"
}

func (PersistentParameterNamespace) Name() string {
	return "Private project-owned persistent parameter namespace"
}

func (rule PersistentParameterNamespace) Analyze(ctx *analyzer.Context, node syntax.Node) {
	file, ok := node.(*syntax.File)
	if !ok || !portablePluginSource(ctx.Source) {
		return
	}

	privatePrefix := "_" + projectconfig.ShellPrefix(ctx.Source.ProjectIdentifier) + "_"
	reported := make(map[*syntax.Assign]bool)
	report := func(assign *syntax.Assign) {
		if assign == nil || reported[assign] {
			return
		}
		name, pos, end, ok := declaredParameter(assign)
		if !ok || name == "Plugins" || exemptPersistentParameter(name) || strings.HasPrefix(name, privatePrefix) {
			return
		}
		reported[assign] = true
		ctx.Report(
			pos,
			end,
			rule.ID(),
			diag.Warning,
			fmt.Sprintf(
				"Persistent parameter %q must use private prefix %q; expose ordinary configuration through zstyle context %q",
				name,
				privatePrefix,
				":"+ctx.Source.ProjectIdentifier+":config",
			),
		)
	}

	// An explicit -g declaration is persistent even inside a function.
	syntax.Walk(file, func(candidate syntax.Node) bool {
		declaration, ok := candidate.(*syntax.DeclClause)
		if !ok || declaration == nil || declaration.Variant == nil {
			return true
		}
		scan := scanDeclarationOptions(declaration.Variant.Value, declaration.Args)
		if scan.mode != declarationModeGlobal {
			return true
		}
		for _, assign := range declaration.Args[scan.firstOperand:] {
			report(assign)
		}
		return true
	})

	// Declarations and assignments in the file load scope are persistent by
	// default. Function bodies have a separate dynamic scope and are skipped
	// unless the explicit-global pass above already proved persistence.
	for _, statement := range file.Stmts {
		syntax.Walk(statement, func(candidate syntax.Node) bool {
			if _, ok := candidate.(*syntax.FuncDecl); ok {
				return false
			}
			switch value := candidate.(type) {
			case *syntax.DeclClause:
				if value == nil || value.Variant == nil {
					return true
				}
				scan := scanDeclarationOptions(value.Variant.Value, value.Args)
				if scan.mode != declarationModeLocal {
					return true
				}
				for _, assign := range value.Args[scan.firstOperand:] {
					report(assign)
				}
			case *syntax.CallExpr:
				if len(value.Args) != 0 {
					return true
				}
				for _, assign := range value.Assigns {
					report(assign)
				}
			}
			return true
		})
	}
}

func portablePluginSource(source projectconfig.SourceContext) bool {
	if source.ConfigVersion != projectconfig.CurrentVersion || source.ProjectIdentifier == "" {
		return false
	}
	if source.Profile != projectconfig.ProfileSourcedLibrary {
		return false
	}
	return source.ProjectKind == projectconfig.KindPlugin || source.ProjectKind == projectconfig.KindZiAnnex
}

func declaredParameter(assign *syntax.Assign) (string, syntax.Pos, syntax.Pos, bool) {
	if assign == nil {
		return "", syntax.Pos{}, syntax.Pos{}, false
	}
	if assign.Name != nil {
		return assign.Name.Value, assign.Name.Pos(), assign.Name.End(), true
	}
	value, ok := staticDeclarationWord(assign.Value)
	if !ok {
		return "", syntax.Pos{}, syntax.Pos{}, false
	}
	name, _, _ := strings.Cut(value, "=")
	name = strings.TrimSuffix(name, "+")
	if name == "" || strings.ContainsAny(name, "[]") {
		return "", syntax.Pos{}, syntax.Pos{}, false
	}
	return name, assign.Pos(), assign.End(), true
}

func exemptPersistentParameter(name string) bool {
	switch name {
	case "fpath", "FPATH", "path", "PATH", "module_path":
		return true
	default:
		return false
	}
}
