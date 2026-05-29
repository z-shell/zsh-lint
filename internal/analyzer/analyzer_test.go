package analyzer_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
)

type dummyRule struct{}

func (r *dummyRule) ID() diag.RuleID { return "test/dummy" }
func (r *dummyRule) Name() string    { return "Dummy Rule" }
func (r *dummyRule) Analyze(ctx *analyzer.Context, node syntax.Node) {
	if call, ok := node.(*syntax.CallExpr); ok {
		if len(call.Args) > 0 && len(call.Args[0].Parts) > 0 {
			if word, ok := call.Args[0].Parts[0].(*syntax.Lit); ok && word.Value == "badcmd" {
				ctx.Report(call.Pos(), call.End(), r.ID(), diag.SeverityWarning, "Found badcmd")
			}
		}
	}
}

func TestAnalyzer(t *testing.T) {
	code := "echo ok\nbadcmd fail\n"
	file, err := parse.Parse(strings.NewReader(code), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	eng := analyzer.New(&dummyRule{})
	diags := eng.Analyze(file, "test.zsh")

	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	d := diags[0]
	if d.RuleID != "test/dummy" {
		t.Errorf("expected test/dummy, got %s", d.RuleID)
	}
	if d.Message != "Found badcmd" {
		t.Errorf("expected Found badcmd, got %s", d.Message)
	}
	if d.Range.Start.Line != 2 {
		t.Errorf("expected line 2, got %d", d.Range.Start.Line)
	}
}
