package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func TestFuncDeclStyle(t *testing.T) {
	src := `
foo() { echo 1; }
function bar { echo 2; }
function baz() { echo 3; }
`
	f, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	analyzerInst := analyzer.New(FuncDeclStyle{})
	diags := analyzerInst.Analyze(f, "test.zsh")

	if len(diags) != 1 {
		t.Errorf("expected 1 diagnostic, got %d", len(diags))
		for _, d := range diags {
			t.Logf("diag: %v", d.Message)
		}
	} else if diags[0].Range.Start.Line != 4 {
		t.Errorf("expected diagnostic on line 4, got %d", diags[0].Range.Start.Line)
	}
}
