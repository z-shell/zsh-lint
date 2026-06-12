package analyzer_test

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/rules"
	"mvdan.cc/sh/v3/syntax"
)

type dummyRule struct{}

func (r *dummyRule) ID() diag.RuleID { return "test/dummy" }
func (r *dummyRule) Name() string    { return "Dummy Rule" }
func (r *dummyRule) Analyze(ctx *analyzer.Context, node syntax.Node) {
	if call, ok := node.(*syntax.CallExpr); ok {
		if len(call.Args) > 0 && len(call.Args[0].Parts) > 0 {
			if word, ok := call.Args[0].Parts[0].(*syntax.Lit); ok && word.Value == "badcmd" {
				ctx.Report(call.Pos(), call.End(), r.ID(), diag.Warning, "Found badcmd")
			}
		}
	}
}

type scopeRule struct {
	sawGlobal bool
}

func (r *scopeRule) ID() diag.RuleID { return "test/scope" }
func (r *scopeRule) Name() string    { return "Scope Rule" }
func (r *scopeRule) NeedsScope() bool {
	return true
}
func (r *scopeRule) Analyze(ctx *analyzer.Context, _ syntax.Node) {
	r.sawGlobal = ctx.Scope.IsDeclared("global_var", nil)
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

func TestAnalyzerIndexesScopeForOptInRule(t *testing.T) {
	file, err := parse.Parse(strings.NewReader("global_var=value\n"), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rule := &scopeRule{}
	analyzer.New(rule).Analyze(file, "test.zsh")

	if !rule.sawGlobal {
		t.Fatal("scope-aware rule did not receive the declaration index")
	}
}

func findByID(ds diag.Diagnostics, id diag.RuleID) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, d := range ds {
		if d.RuleID == id {
			out = append(out, d)
		}
	}
	return out
}

func TestAnalyzerAppliesSuppression(t *testing.T) {
	code := "eval $x # zsh-lint disable=security/eval -- static table\n"
	file, err := parse.Parse(strings.NewReader(code), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	eng := analyzer.New(rules.Default()...)
	diags := eng.Analyze(file, "test.zsh")

	if got := findByID(diags, "security/eval"); len(got) != 0 {
		t.Errorf("security/eval finding survived its suppression: %+v", got)
	}
	if got := findByID(diags, "quoting/unquoted-var"); len(got) != 1 {
		t.Errorf("expected quoting/unquoted-var on the same line to be unaffected, got %d findings", len(got))
	}
	if got := findByID(diags, "meta/unused-suppression"); len(got) != 0 {
		t.Errorf("used suppression reported stale: %+v", got)
	}
}

func TestAnalyzerReportsStaleSuppression(t *testing.T) {
	code := "print ok # zsh-lint disable=security/eval\n"
	file, err := parse.Parse(strings.NewReader(code), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	diags := analyzer.New(rules.Default()...).Analyze(file, "test.zsh")
	got := findByID(diags, "meta/unused-suppression")
	if len(got) != 1 {
		t.Fatalf("expected one meta/unused-suppression, got %v", diags)
	}
	if got[0].Severity != diag.Info {
		t.Errorf("stale suppression severity = %v, want Info", got[0].Severity)
	}
}

func TestAnalyzerReportsMalformedSuppression(t *testing.T) {
	code := "print ok # zsh-lint enable=security/eval\n"
	file, err := parse.Parse(strings.NewReader(code), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	diags := analyzer.New(rules.Default()...).Analyze(file, "test.zsh")
	got := findByID(diags, "meta/malformed-suppression")
	if len(got) != 1 {
		t.Fatalf("expected one meta/malformed-suppression, got %v", diags)
	}
	if got[0].Severity != diag.Warning {
		t.Errorf("malformed suppression severity = %v, want Warning", got[0].Severity)
	}
}
