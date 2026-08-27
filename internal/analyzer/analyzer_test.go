package analyzer_test

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
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

type sourceRule struct {
	source projectconfig.SourceContext
}

func (r *sourceRule) ID() diag.RuleID { return "test/source" }
func (r *sourceRule) Name() string    { return "Source Rule" }
func (r *sourceRule) Analyze(ctx *analyzer.Context, _ syntax.Node) {
	r.source = ctx.Source.Clone()
}

type fileRule struct {
	calls int
}

func (r *fileRule) ID() diag.RuleID                            { return "test/file" }
func (r *fileRule) Name() string                               { return "File Rule" }
func (r *fileRule) Analyze(_ *analyzer.Context, _ syntax.Node) {}
func (r *fileRule) AnalyzeFile(ctx *analyzer.Context) {
	r.calls++
	ctx.Report(syntax.Pos{}, syntax.Pos{}, r.ID(), diag.Hint, "file finding")
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

func TestAnalyzerSuppliesSourceContext(t *testing.T) {
	file, err := parse.Parse(strings.NewReader("print ok\n"), "functions/example-run")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	want := projectconfig.SourceContext{
		ConfigVersion:      projectconfig.CurrentVersion,
		ProjectKind:        projectconfig.KindPlugin,
		MinimumZsh:         "5.8",
		FunctionNamespaces: []string{"example"},
		Profile:            projectconfig.ProfileAutoloadFunction,
		SourceRoot:         "functions",
	}
	rule := &sourceRule{}
	analyzer.New(rule).AnalyzeSource(file, "functions/example-run", want)
	if rule.source.ConfigVersion != want.ConfigVersion || rule.source.ProjectKind != want.ProjectKind ||
		rule.source.MinimumZsh != want.MinimumZsh ||
		rule.source.Profile != want.Profile || rule.source.SourceRoot != want.SourceRoot ||
		len(rule.source.FunctionNamespaces) != 1 || rule.source.FunctionNamespaces[0] != "example" {
		t.Errorf("source context = %+v, want %+v", rule.source, want)
	}

	want.FunctionNamespaces[0] = "mutated"
	if rule.source.FunctionNamespaces[0] != "example" {
		t.Errorf("analyzer retained caller-owned namespace storage: %q", rule.source.FunctionNamespaces)
	}
}

func TestAnalyzerLegacyEntryPointHasUnconfiguredSource(t *testing.T) {
	file, err := parse.Parse(strings.NewReader("print ok\n"), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rule := &sourceRule{}
	analyzer.New(rule).Analyze(file, "test.zsh")
	if rule.source.Configured() {
		t.Errorf("legacy Analyze() source = %+v, want unconfigured context", rule.source)
	}
}

func TestAnalyzerRunsFileRuleOnce(t *testing.T) {
	file, err := parse.Parse(strings.NewReader("print one\nprint two\n"), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rule := &fileRule{}
	diagnostics := analyzer.New(rule).Analyze(file, "test.zsh")
	if rule.calls != 1 {
		t.Errorf("AnalyzeFile calls = %d, want 1", rule.calls)
	}
	if len(diagnostics) != 1 || diagnostics[0].Range.IsValid() {
		t.Errorf("file diagnostics = %+v, want one unpositioned finding", diagnostics)
	}
}

func TestAnalyzerPositionsAfterNestedConditionalAlternation(t *testing.T) {
	const code = "badcmd before\nline=foo\n[[ $line == ((a|b)|c) ]]\nbadcmd after\n"
	file, err := parse.Parse(strings.NewReader(code), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	diags := analyzer.New(&dummyRule{}).Analyze(file, "test.zsh")
	if len(diags) != 2 {
		t.Fatalf("diagnostic count = %d, want 2: %+v", len(diags), diags)
	}
	want := []struct {
		line   int
		column int
		offset int
	}{
		{line: 1, column: 1, offset: 0},
		{line: 4, column: 1, offset: 48},
	}
	if offset := strings.Index(code, "badcmd after"); offset != want[1].offset {
		t.Fatalf("badcmd after offset = %d, want %d", offset, want[1].offset)
	}
	for i, diagnostic := range diags {
		if diagnostic.Range.Start.Line != want[i].line ||
			diagnostic.Range.Start.Column != want[i].column ||
			diagnostic.Range.Start.Offset != want[i].offset {
			t.Errorf("diagnostic[%d] start = %+v, want line=%d column=%d offset=%d",
				i, diagnostic.Range.Start, want[i].line, want[i].column, want[i].offset)
		}
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

func TestAnalyzerSuppressionAfterNestedConditionalAlternation(t *testing.T) {
	const code = "line=foo\n[[ $line == ((a|b)|c) ]]\neval $x # zsh-lint disable=security/eval -- static table\n"
	file, err := parse.Parse(strings.NewReader(code), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	diags := analyzer.New(rules.Default()...).Analyze(file, "test.zsh")
	if got := findByID(diags, "security/eval"); len(got) != 0 {
		t.Errorf("security/eval finding survived suppression: %+v", got)
	}
	if got := findByID(diags, "quoting/unquoted-var"); len(got) != 1 {
		t.Errorf("quoting/unquoted-var count = %d, want 1: %+v", len(got), got)
	}
	if got := findByID(diags, "meta/unused-suppression"); len(got) != 0 {
		t.Errorf("used suppression reported stale: %+v", got)
	}
}

func TestAnalyzerDiagnosticsAfterLegacyBacktickIsland(t *testing.T) {
	tests := []struct {
		name string
		code string
		want []struct {
			ruleID diag.RuleID
			offset int
			line   int
			column int
		}
	}{
		{
			name: "suppression on next line",
			code: "print `[[ $line == ((a|b)|c) ]]`\n" +
				"eval $x # zsh-lint disable=security/eval -- static table\n",
			want: []struct {
				ruleID diag.RuleID
				offset int
				line   int
				column int
			}{
				{ruleID: "style/backquotes", offset: 6, line: 1, column: 7},
				{ruleID: "quoting/unquoted-var", offset: 38, line: 2, column: 6},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.code), "legacy-analyzer.zsh")
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			diagnostics := analyzer.New(rules.Default()...).Analyze(file, "legacy-analyzer.zsh")
			if got := findByID(diagnostics, "security/eval"); len(got) != 0 {
				t.Errorf("security/eval finding survived suppression: %+v", got)
			}
			if got := findByID(diagnostics, "meta/unused-suppression"); len(got) != 0 {
				t.Errorf("used suppression reported stale: %+v", got)
			}
			if len(diagnostics) != len(test.want) {
				t.Fatalf("diagnostic count = %d, want %d: %+v", len(diagnostics), len(test.want), diagnostics)
			}
			for index, want := range test.want {
				got := diagnostics[index]
				if got.RuleID != want.ruleID || got.Range.Start.Offset != want.offset ||
					got.Range.Start.Line != want.line || got.Range.Start.Column != want.column {
					t.Errorf(
						"diagnostic[%d] = rule %q offset %d line %d column %d, want %q/%d/%d/%d",
						index,
						got.RuleID,
						got.Range.Start.Offset,
						got.Range.Start.Line,
						got.Range.Start.Column,
						want.ruleID,
						want.offset,
						want.line,
						want.column,
					)
				}
			}
		})
	}
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
