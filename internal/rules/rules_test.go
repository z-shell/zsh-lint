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
		want diag.Severity
	}{
		{UnquotedVar{}, "echo $x\n", diag.Warning},
		{EvalUsage{}, "eval $cmd\n", diag.Info},
		{Backquotes{}, "echo `pwd`\n", diag.Hint},
		{FuncDeclStyle{}, "function f() { :; }\n", diag.Hint},
		{PreferDoubleBrackets{}, "if [ -f x ]; then :; fi\n", diag.Hint},
	}
	for _, tt := range tests {
		t.Run(string(tt.rule.ID()), func(t *testing.T) {
			f, err := parse.Parse(strings.NewReader(tt.src), "test.zsh")
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			diags := analyzer.New(tt.rule).Analyze(f, "test.zsh")
			if len(diags) == 0 {
				t.Fatalf("rule %s produced no finding on %q", tt.rule.ID(), tt.src)
			}
			if got := diags[0].Severity; got != tt.want {
				t.Errorf("rule %s severity = %v, want %v", tt.rule.ID(), got, tt.want)
			}
		})
	}
}
