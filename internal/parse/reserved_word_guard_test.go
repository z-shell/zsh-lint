package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// The parser fork reads foreach in command position as a ForClause (#214).
// Each row is `zsh -f -n` valid.
func TestParseForeachLoops(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"foreach end form", "foreach v ($a)\n  cmd $v\nend\n"},
		{"foreach nested end form", "if true; then\n  foreach v ($a)\n    cmd $v\n  end\nfi\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			var found []*syntax.ForClause
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if fc, ok := node.(*syntax.ForClause); ok {
					found = append(found, fc)
				}
				return true
			})
			if len(found) != 1 {
				t.Fatalf("found %d ForClause nodes, want 1", len(found))
			}
		})
	}
}

// The guard keys on the command name only: a quoted word, an argument, or a
// function named after the reserved word is not a loop.
func TestParseKeepsOrdinaryUsesOfLoopWords(t *testing.T) {
	for _, src := range []string{
		"print repeat foreach\n",
		"\\repeat 3\n",
		"x=repeat\n",
		"command repeat\n",
	} {
		if _, err := Parse(strings.NewReader(src), "ordinary.zsh"); err != nil {
			t.Errorf("Parse(%q) error = %v, want nil", src, err)
		}
	}
}
