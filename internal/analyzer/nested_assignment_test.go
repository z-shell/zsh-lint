package analyzer_test

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/rules"
)

// #735 must restore semantic children, not only accept the assignment.
func TestNestedAssignmentTargetAnalysis(t *testing.T) {
	const assignment = `a[${b[(ie)${c[$(eval "$code")]}]}]=()` + "\n"
	file, err := parse.Parse(strings.NewReader(assignment), "test.zsh")
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := analyzer.New(&expansionRule{}, rules.EvalUsage{}).Analyze(file, "test.zsh")
	var expansions, evals int
	for _, d := range diagnostics {
		switch d.RuleID {
		case "test/expansion":
			expansions++
		case "security/eval":
			evals++
			start := strings.Index(assignment, `eval "$code"`)
			if d.Range.Start.Offset != start || d.Range.End.Offset != start+len(`eval "$code"`) {
				t.Fatalf("eval source range changed: %v", d.Range)
			}
		}
	}
	if expansions != 3 || evals != 1 {
		t.Fatalf("want b, c, code and one eval; got %v", diagnostics)
	}
	suppressed, err := parse.Parse(strings.NewReader("# zsh-lint disable=security/eval -- trusted fixture\n"+assignment), "test.zsh")
	if err != nil {
		t.Fatal(err)
	}
	if got := analyzer.New(rules.EvalUsage{}).Analyze(suppressed, "test.zsh"); len(got) != 0 {
		t.Fatalf("preceding suppression lost: %v", got)
	}
}
