package analyzer

import (
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
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
	return a.AnalyzeSource(file, path, projectconfig.SourceContext{})
}

// AnalyzeSource runs the semantic analyzer with resolved project and
// execution-profile metadata. Analyze remains the compatibility entry point
// for callers that do not use project configuration.
func (a *Analyzer) AnalyzeSource(file *parse.File, path string, source projectconfig.SourceContext) diag.Diagnostics {
	if source.Configured() {
		return a.AnalyzeProject([]ProjectInput{{File: file, Path: path, Source: source}})
	}
	return a.analyzeSource(file, path, source, true)
}

func (a *Analyzer) analyzeSource(file *parse.File, path string, source projectconfig.SourceContext, applySuppressions bool) diag.Diagnostics {
	ctx := NewContextWithSource(file, path, source)
	ast := file.AST()

	// File-level evaluation runs once and may report an unpositioned finding.
	for _, rule := range a.rules {
		if fileRule, ok := rule.(FileRule); ok {
			fileRule.AnalyzeFile(ctx)
		}
	}

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
	if ast != nil && applySuppressions {
		known := make(map[diag.RuleID]bool, len(a.rules))
		for _, rule := range a.rules {
			known[rule.ID()] = true
		}
		ctx.Diagnostics = suppress.Apply(suppress.Collect(file), ctx.Diagnostics, known, path)
	}

	ctx.Diagnostics.Sort()
	return ctx.Diagnostics
}

// AnalyzeProject runs per-file analysis and then project rules over the
// complete explicit configured input set.
func (a *Analyzer) AnalyzeProject(inputs []ProjectInput) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	for _, input := range inputs {
		diagnostics = append(diagnostics, a.analyzeSource(input.File, input.Path, input.Source, false)...)
	}
	project := &ProjectContext{Inputs: append([]ProjectInput(nil), inputs...)}
	for _, rule := range a.rules {
		if projectRule, ok := rule.(ProjectRule); ok {
			projectRule.AnalyzeProject(project)
		}
	}
	known := make(map[diag.RuleID]bool, len(a.rules))
	for _, rule := range a.rules {
		known[rule.ID()] = true
	}
	for _, input := range inputs {
		var attributed diag.Diagnostics
		var remaining diag.Diagnostics
		for _, diagnostic := range diagnostics {
			if diagnostic.File == input.Path {
				attributed = append(attributed, diagnostic)
			} else {
				remaining = append(remaining, diagnostic)
			}
		}
		for _, diagnostic := range project.Diagnostics {
			if diagnostic.File == input.Path {
				attributed = append(attributed, diagnostic)
			}
		}
		diagnostics = append(remaining, suppress.Apply(
			suppress.Collect(input.File),
			attributed,
			known,
			input.Path,
		)...)
	}
	diagnostics.Sort()
	return diagnostics
}

func needsScope(rules []Rule) bool {
	for _, rule := range rules {
		if aware, ok := rule.(ScopeAwareRule); ok && aware.NeedsScope() {
			return true
		}
	}
	return false
}
