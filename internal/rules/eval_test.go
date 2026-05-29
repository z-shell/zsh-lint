package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func TestEvalUsage(t *testing.T) {
	src := `
eval "echo $foo"
echo "eval"
`
	f, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	analyzerInst := analyzer.New(EvalUsage{})
	diags := analyzerInst.Analyze(f, "test.zsh")

	if len(diags) != 1 {
		t.Errorf("expected 1 diagnostic, got %d", len(diags))
		for _, d := range diags {
			t.Logf("diag: %v", d.Message)
		}
	} else if diags[0].RuleID != "security/eval" {
		t.Errorf("expected rule ID security/eval, got %s", diags[0].RuleID)
	}
}
