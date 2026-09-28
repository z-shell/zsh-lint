package syntax

import (
	"strings"
	"testing"
)

// TestZshBraceElse checks the tree and printed form of the brace form of
// `else` (zsh-lint #541). IfClause.ThenPos is empty for an `else`, as its
// documentation says; the printer relies on that to print `else` rather than
// an `elif` with an empty condition, which Zsh deparses as an elif.
func TestZshBraceElse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src, want string
	}{
		{
			"if [[ -n x ]] { print a } else { print b }\n",
			"if [[ -n x ]]; then print a; else print b; fi\n",
		},
		{
			"if [[ -n x ]] { print a } elif [[ -n y ]] { print c } else { print b }\n",
			"if [[ -n x ]]; then print a; elif [[ -n y ]]; then print c; else print b; fi\n",
		},
		// The `{` on its own line leaves a blank line where it stood, as
		// the printer keeps source line gaps; Zsh reads it the same.
		{
			"if [[ -n x ]] {\n\tprint a\n} else\n{\n\tprint b\n}\n",
			"if [[ -n x ]]; then\n\tprint a\nelse\n\n\tprint b\nfi\n",
		},
		// A classic `elif` may have an empty condition; it keeps its `then`,
		// and upstream prints the empty condition as `elif; then`, which
		// Zsh reads as the same program.
		{
			"if false; then :; elif then print x; fi\n",
			"if false; then :; elif; then print x; fi\n",
		},
	}
	p := NewParser(Variant(LangZsh))
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			f, err := p.Parse(strings.NewReader(tc.src), "")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.src, err)
			}
			ic := f.Stmts[0].Cmd.(*IfClause)
			last := ic
			for last.Else != nil {
				last = last.Else
			}
			isElse := strings.Contains(tc.src, "else")
			if isElse && last.ThenPos.IsValid() {
				t.Errorf("else clause has ThenPos %s; want it empty, as for a classic else", last.ThenPos)
			}
			if isElse && string(tc.src[last.Position.Offset():last.Position.Offset()+4]) != "else" {
				t.Errorf("else clause Position %s does not point at `else`", last.Position)
			}
			if !isElse && !last.ThenPos.IsValid() {
				t.Errorf("elif clause lost its ThenPos")
			}
			var sb strings.Builder
			if err := NewPrinter().Print(&sb, f); err != nil {
				t.Fatalf("Print(%q) failed: %v", tc.src, err)
			}
			if got := sb.String(); got != tc.want {
				t.Errorf("printed %q, want %q", got, tc.want)
			}
		})
	}
}
