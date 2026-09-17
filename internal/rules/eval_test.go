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

// The count of a repeat loop is not a command: `repeat eval print hi` runs
// `print hi` as many times as the parameter `eval` counts.
func TestEvalUsageIgnoresRepeatCount(t *testing.T) {
	src := "repeat eval print hi\nrepeat 2 eval \"$cmd\"\n"
	f, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	diags := analyzer.New(EvalUsage{}).Analyze(f, "test.zsh")

	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	if got := diags[0].Range.Start; got.Line != 2 || got.Column != 10 {
		t.Errorf("expected the eval in the loop body at 2:10, got %d:%d", got.Line, got.Column)
	}
}
