package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func TestBackquotes(t *testing.T) {
	src := `
echo $(pwd)
echo ` + "`pwd`" + `
`
	f, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	analyzerInst := analyzer.New(Backquotes{})
	diags := analyzerInst.Analyze(f, "test.zsh")

	if len(diags) != 1 {
		t.Errorf("expected 1 diagnostic, got %d", len(diags))
		for _, d := range diags {
			t.Logf("diag: %v", d.Message)
		}
	} else if diags[0].RuleID != "style/backquotes" {
		t.Errorf("expected rule ID style/backquotes, got %s", diags[0].RuleID)
	}
}
