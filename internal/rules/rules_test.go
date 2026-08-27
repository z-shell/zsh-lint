package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
)

// TestRuleSeverities pins each default rule to the severity mapping in
// docs/project/rule-policy.md: hint for style/idiom preferences, info for
// risky-but-sometimes-intentional patterns (the policy's example is
// security/eval), warning for likely-bug patterns.
func TestRuleSeverities(t *testing.T) {
	tests := []struct {
		rule analyzer.Rule
		src  string
		path string
		want diag.Severity
	}{
		{UnquotedVar{}, "echo $x\n", "test.zsh", diag.Warning},
		{EvalUsage{}, "eval $cmd\n", "test.zsh", diag.Info},
		{Backquotes{}, "echo `pwd`\n", "test.zsh", diag.Hint},
		{FuncDeclStyle{}, "function f() { :; }\n", "test.zsh", diag.Hint},
		{PreferDoubleBrackets{}, "if [ -f x ]; then :; fi\n", "test.zsh", diag.Hint},
		{FunctionScopedOptions{}, "rehash\n", "functions/handler", diag.Hint},
		{SpecialParamShadow{}, "local ZSH_VERSION=1\n", "test.zsh", diag.Warning},
		{ZeroHandling{}, "fpath+=( \"${0:h}/functions\" )\n", "plugin.zsh", diag.Warning},
		{UnloadFunction{}, "add-zsh-hook precmd _hook\n", "plugin.zsh", diag.Hint},
		{FpathHygiene{}, "fpath=( \"${0:h}/functions\" )\n", "plugin.zsh", diag.Warning},
	}
	for _, tt := range tests {
		t.Run(string(tt.rule.ID()), func(t *testing.T) {
			f, err := parse.Parse(strings.NewReader(tt.src), tt.path)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			diags := analyzer.New(tt.rule).Analyze(f, tt.path)
			if len(diags) == 0 {
				t.Fatalf("rule %s produced no finding on %q", tt.rule.ID(), tt.src)
			}
			if got := diags[0].Severity; got != tt.want {
				t.Errorf("rule %s severity = %v, want %v", tt.rule.ID(), got, tt.want)
			}
		})
	}
}

func TestDefaultIncludesFunctionScopedOptions(t *testing.T) {
	for _, rule := range Default() {
		if rule.ID() == "plugin/function-scoped-options" {
			return
		}
	}
	t.Fatal("Default rules do not include plugin/function-scoped-options")
}

func TestDefaultIncludesSpecialParamShadow(t *testing.T) {
	for _, rule := range Default() {
		if rule.ID() == "compat/special-param-shadow" {
			return
		}
	}
	t.Fatal("Default rules do not include compat/special-param-shadow")
}

func TestDefaultIncludesZeroHandling(t *testing.T) {
	for _, rule := range Default() {
		if rule.ID() == "plugin/zero-handling" {
			return
		}
	}
	t.Fatal("Default rules do not include plugin/zero-handling")
}

func TestDefaultIncludesUnloadFunction(t *testing.T) {
	for _, rule := range Default() {
		if rule.ID() == "plugin/unload-function" {
			return
		}
	}
	t.Fatal("Default rules do not include plugin/unload-function")
}

func TestDefaultIncludesFpathHygiene(t *testing.T) {
	for _, rule := range Default() {
		if rule.ID() == "plugin/fpath-hygiene" {
			return
		}
	}
	t.Fatal("Default rules do not include plugin/fpath-hygiene")
}

func TestFunctionNamespaceIsOptIn(t *testing.T) {
	for _, rule := range Default() {
		if rule.ID() == "plugin/function-namespace" {
			t.Fatal("Default includes opt-in plugin/function-namespace")
		}
	}
	profile := ProjectProfile(1)
	if len(profile) != 1 || profile[0].ID() != "plugin/function-namespace" {
		t.Fatalf("ProjectProfile(1) = %v, want plugin/function-namespace", profile)
	}
	if profile := ProjectProfile(2); len(profile) != 0 {
		t.Fatalf("ProjectProfile(2) = %v, want no rules", profile)
	}
}

func TestFunctionNamespaceSeverity(t *testing.T) {
	file, err := parse.Parse(strings.NewReader("refresh() { :; }\n"), "plugin.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	diagnostics := analyzer.New(FunctionNamespace{}).AnalyzeSource(
		file,
		"plugin.zsh",
		sourcedPlugin("example"),
	)
	if len(diagnostics) != 1 || diagnostics[0].Severity != diag.Hint {
		t.Fatalf("diagnostics = %+v, want one hint", diagnostics)
	}
}
