package analyzer

import (
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"mvdan.cc/sh/v3/syntax"
)

// Rule defines the interface for a zsh-lint static analysis rule.
type Rule interface {
	// ID returns the stable identifier for the rule (e.g. "quoting/unquoted-var")
	ID() diag.RuleID
	// Name returns a human-readable name for the rule
	Name() string
	// Analyze evaluates a syntax node and reports findings to the context.
	Analyze(ctx *Context, node syntax.Node)
}

// FileRule is implemented by rules that report file-level findings which do
// not belong to one AST node, such as contracts derived from an autoload file
// name. AnalyzeFile is called once per parsed file before node traversal.
type FileRule interface {
	AnalyzeFile(ctx *Context)
}

// ScopeAwareRule is implemented by rules that need the declaration index.
// Most rules inspect syntax only, so the analyzer skips the indexing pass
// unless at least one registered rule opts in.
type ScopeAwareRule interface {
	NeedsScope() bool
}

// ProjectInput is one parsed, explicitly configured source in a project
// analysis invocation.
type ProjectInput struct {
	File   *parse.File
	Path   string
	Source projectconfig.SourceContext
}

// ProjectRule evaluates invariants that require more than one configured
// source file. Per-file Analyze methods remain responsible for local findings.
type ProjectRule interface {
	AnalyzeProject(ctx *ProjectContext)
}
