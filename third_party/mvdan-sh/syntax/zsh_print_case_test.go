package syntax

import (
	"strings"
	"testing"
)

// TestZshPrintCasePatternBrace checks that a case pattern whose alternative
// ends in an unquoted literal `}` prints with its optional opening
// parenthesis (zsh-lint #541). Without it Zsh reads that `}` as the end of a
// brace group: `case x in a}) : ;; esac` is a parse error, and so is
// `case x in b|a}) ...`. A `}` that closes an expansion or is quoted does not
// need it.
func TestZshPrintCasePatternBrace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src, want string
	}{
		{"case x in (a}) : ;; esac\n", "case x in (a}) : ;; esac\n"},
		{"case x in (}) : ;; esac\n", "case x in (}) : ;; esac\n"},
		{"case x in (b|a}) : ;; esac\n", "case x in (b | a}) : ;; esac\n"},
		{"case x in (a}|b) : ;; esac\n", "case x in (a} | b) : ;; esac\n"},
		// Each item is judged on its own patterns.
		{"case x in y) : ;; (a}) : ;; esac\n", "case x in y) : ;; (a}) : ;; esac\n"},
		// No parenthesis is needed, so none is added.
		{"case x in (a) : ;; esac\n", "case x in a) : ;; esac\n"},
		{"case x in (a}b) : ;; esac\n", "case x in a}b) : ;; esac\n"},
		{"case x in ('a}') : ;; esac\n", "case x in 'a}') : ;; esac\n"},
		{"case x in (a\\}) : ;; esac\n", "case x in a\\}) : ;; esac\n"},
		{"case x in (${x}) : ;; esac\n", "case x in ${x}) : ;; esac\n"},
		// Upstream already keeps the parenthesis for `esac`.
		{"case x in (esac) : ;; esac\n", "case x in (esac) : ;; esac\n"},
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
