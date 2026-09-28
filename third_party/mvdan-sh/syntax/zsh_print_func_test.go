package syntax

import (
	"strings"
	"testing"
)

// TestZshPrintFunctionKeywordBody checks that a `function` keyword
// definition whose body is not a `{ }` group prints with `()` (zsh-lint
// #541). Without them Zsh reads every word up to a `{` as another function
// name, so `function f print hi` defines three functions with empty bodies.
func TestZshPrintFunctionKeywordBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src, want string
	}{
		{"function f () print hi\n", "function f() print hi\n"},
		{"function f () (( 1 ))\n", "function f() ((1))\n"},
		{"function f () ( print sub )\n", "function f() (print sub)\n"},
		// A body on the next line is the next command, not `()`.
		{"function f\nprint hi\n", "function f() print hi\n"},
		{"function a\nfunction b c\nprint after\n", "function a() function b c() print after\n"},
		// A brace body needs no `()`, and a bodyless definition keeps none.
		{"function f { print hi }\n", "function f { print hi; }\n"},
		{"function f\n", "function f\n"},
		{"function f () { print hi }\n", "function f() { print hi; }\n"},
	}
	p := NewParser(Variant(LangZsh))
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			f, err := p.Parse(strings.NewReader(tc.src), "")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.src, err)
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
