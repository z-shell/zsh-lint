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

	// Only the unquoted command argument `$foo` (line 2) is flagged. The quoted
	// `"$bar"` is safe, and the assignment RHS `A=$BAZ` is safe (no word
	// splitting on assignment values), so neither is reported.
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	if diags[0].RuleID != rule.ID() {
		t.Errorf("expected rule ID %q, got %q", rule.ID(), diags[0].RuleID)
	}
	if diags[0].Range.Start.Line != 2 {
		t.Errorf("expected diagnostic on line 2 (echo $foo), got line %d", diags[0].Range.Start.Line)
	}
}
