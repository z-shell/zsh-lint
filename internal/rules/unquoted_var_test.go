package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func TestUnquotedVar(t *testing.T) {
	src := `
echo $foo
echo "$bar"
A=$BAZ
`
	f, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rule := UnquotedVar{}
	analyzerInst := analyzer.New(rule)
	diags := analyzerInst.Analyze(f, "test.zsh")

	if len(diags) != 2 {
		t.Errorf("expected 2 diagnostics, got %d", len(diags))
		for _, d := range diags {
			t.Logf("diag: %v", d)
		}
	}
}
