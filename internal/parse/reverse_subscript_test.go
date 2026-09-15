package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #209: a reverse-subscript pattern containing `/` must parse as a
// flagged subscript whose pattern is the whole word, not as arithmetic. The
// ok-* corpus fixture only proves the absence of an error; this pins the tree.
func TestParseReverseSubscriptPatternWithSlash(t *testing.T) {
	tests := []struct {
		name        string
		src         string
		wantPattern string
	}{
		{"plain parameter", "x=( ${fpath[(R)$PLUGIN_DIR/*]} )\n", "$PLUGIN_DIR/*"},
		{"modifier parameter", "x=( ${fpath[(R)${PLUGIN_DIR:A}/*]} )\n", "${PLUGIN_DIR:A}/*"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			var param *syntax.ParamExp
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if p, ok := node.(*syntax.ParamExp); ok && param == nil {
					param = p
				}
				return param == nil
			})
			if param == nil {
				t.Fatal("no ParamExp in tree")
			}
			flagged, ok := param.Index.(*syntax.FlagsArithm)
			if !ok {
				t.Fatalf("Index is %T, want *syntax.FlagsArithm", param.Index)
			}
			if flagged.Flags == nil || flagged.Flags.Value != "R" {
				t.Errorf("Flags = %v, want R", flagged.Flags)
			}
			word, ok := flagged.X.(*syntax.Word)
			if !ok {
				t.Fatalf("X is %T, want *syntax.Word", flagged.X)
			}
			if got := word.Lit(); got != test.wantPattern {
				t.Errorf("pattern = %q, want %q", got, test.wantPattern)
			}
		})
	}
}
