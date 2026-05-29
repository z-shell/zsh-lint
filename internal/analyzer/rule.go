package analyzer

import (
	"github.com/z-shell/zsh-lint/internal/diag"
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
