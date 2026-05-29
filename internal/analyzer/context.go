package analyzer

import (
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/scope"
	"mvdan.cc/sh/v3/syntax"
)

// Context provides state and utilities for rules during AST traversal.
type Context struct {
	File        *parse.File
	FilePath    string
	Diagnostics diag.Diagnostics
	// Scope tracking per ADR 0011
	Scope *scope.Map
}

// NewContext creates a new analysis context for a file.
func NewContext(file *parse.File, path string) *Context {
	return &Context{
		File:     file,
		FilePath: path,
		Scope:    scope.NewMap(),
	}
}

// Report adds a finding to the context's diagnostic list.
func (c *Context) Report(pos, end syntax.Pos, ruleID diag.RuleID, sev diag.Severity, msg string) {
	d := diag.Diagnostic{
		RuleID:   ruleID,
		Severity: sev,
		Message:  msg,
		File:     c.FilePath,
	}

	if pos.IsValid() {
		d.Range.Start = diag.Position{
			Line:   int(pos.Line()),
			Column: int(pos.Col()),
			Offset: int(pos.Offset()),
		}
	}
	if end.IsValid() {
		d.Range.End = diag.Position{
			Line:   int(end.Line()),
			Column: int(end.Col()),
			Offset: int(end.Offset()),
		}
	}

	c.Diagnostics = append(c.Diagnostics, d)
}
