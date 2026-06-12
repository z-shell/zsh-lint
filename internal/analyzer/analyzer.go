package analyzer

import (
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/suppress"
	"mvdan.cc/sh/v3/syntax"
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

// Analyze runs the semantic analyzer on the parsed file. Rules that implement
// ScopeAwareRule opt into a scope-resolution pass before rule evaluation.
func (a *Analyzer) Analyze(file *parse.File, path string) diag.Diagnostics {
	ctx := NewContext(file, path)
	ast := file.AST()

	// Pass 1: Scope Resolution (Indexer), only when a rule consumes it.
	if ast != nil && needsScope(a.rules) {
		ctx.Scope.Index(ast)
	}

	// Pass 2: Rule Evaluation (Linter)
	// Traverse the AST and feed each node to the registered rules.
	if ast != nil {
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

	// Pass 3: inline suppression (docs/project/suppression.md). Applying it
	// here keeps human and JSON output consistent: suppressed findings are
	// dropped and meta/* findings appended before the single final sort.
	if ast != nil {
		known := make(map[diag.RuleID]bool, len(a.rules))
		for _, rule := range a.rules {
			known[rule.ID()] = true
		}
		ctx.Diagnostics = suppress.Apply(suppress.Collect(file), ctx.Diagnostics, known, path)
	}

	ctx.Diagnostics.Sort()
	return ctx.Diagnostics
}

func needsScope(rules []Rule) bool {
	for _, rule := range rules {
		if aware, ok := rule.(ScopeAwareRule); ok && aware.NeedsScope() {
			return true
		}
	}
	return false
}
