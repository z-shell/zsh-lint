package analyzer

import (
	"mvdan.cc/sh/v3/syntax"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
)

// Analyzer orchestrates the two-pass semantic analysis of a shell script.
type Analyzer struct {
	rules []Rule
}

// New creates a new Analyzer with the given rules.
func New(rules ...Rule) *Analyzer {
	return &Analyzer{
		rules: rules,
	}
}

// Analyze runs the semantic analyzer on the parsed file.
// Per ADR 0011, this runs in two passes: Scope Resolution, then Rule Evaluation.
func (a *Analyzer) Analyze(file *parse.File, path string) diag.Diagnostics {
	ctx := NewContext(file, path)

	// Pass 1: Scope Resolution (Indexer)
	if ast := file.AST(); ast != nil {
		ctx.Scope.Index(ast)
	}

	// Pass 2: Rule Evaluation (Linter)
	// Traverse the AST and feed each node to the registered rules.
	if ast := file.AST(); ast != nil {
		syntax.Walk(ast, func(node syntax.Node) bool {
			if node == nil {
				return true
			}
			for _, rule := range a.rules {
				rule.Analyze(ctx, node)
			}
			return true
		})
	}

	ctx.Diagnostics.Sort()
	return ctx.Diagnostics
}
