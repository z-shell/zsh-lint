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

func TestRuleSetSelection(t *testing.T) {
	defaultIDs := []diag.RuleID{
		"quoting/unquoted-var",
		"style/backquotes",
		"style/prefer-double-brackets",
		"security/eval",
		"style/function-decl",
		"compat/special-param-shadow",
		"plugin/function-scoped-options",
		"plugin/zero-handling",
		"plugin/unload-function",
		"plugin/fpath-hygiene",
	}
	profileIDs := append(append([]diag.RuleID(nil), defaultIDs...), "plugin/function-namespace")

	if got := ruleIDs(Default()); !equalRuleIDs(got, defaultIDs) {
		t.Fatalf("Default IDs = %v, want %v", got, defaultIDs)
	}
	profile, err := ForProfile(ProjectProfileV1)
	if err != nil {
		t.Fatalf("ForProfile(%q): %v", ProjectProfileV1, err)
	}
	if got := ruleIDs(profile); !equalRuleIDs(got, profileIDs) {
		t.Fatalf("profile IDs = %v, want %v", got, profileIDs)
	}
	assertUniqueRuleIDs(t, profile)

	if _, err := ForProfile(Profile("z-shell/project@2")); err == nil {
		t.Fatal("ForProfile(unknown) succeeded, want error")
	}

	profile[0] = FunctionNamespace{}
	again, err := ForProfile(CurrentProjectProfile)
	if err != nil {
		t.Fatalf("ForProfile(CurrentProjectProfile): %v", err)
	}
	if got := again[0].ID(); got != "quoting/unquoted-var" {
		t.Fatalf("profile selection reused caller-mutated slice: first ID = %q", got)
	}
}

func ruleIDs(rules []analyzer.Rule) []diag.RuleID {
	ids := make([]diag.RuleID, len(rules))
	for index, rule := range rules {
		ids[index] = rule.ID()
	}
	return ids
}

func equalRuleIDs(got, want []diag.RuleID) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func assertUniqueRuleIDs(t *testing.T, rules []analyzer.Rule) {
	t.Helper()
	seen := make(map[diag.RuleID]bool, len(rules))
	for _, rule := range rules {
		if seen[rule.ID()] {
			t.Fatalf("duplicate rule ID %q", rule.ID())
		}
		seen[rule.ID()] = true
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
