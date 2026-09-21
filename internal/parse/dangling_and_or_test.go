package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Native Zsh accepts `&&` and `||` as the last token of a list: the operator has
// no right operand, and the left one runs exactly as a bare statement would.
// Verified by execution, not by reading the grammar — `print a &&` prints `a`.
func TestDanglingAndOrParses(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"while condition", "x=0\nwhile (( x )) &&\n"},
		{"while condition or", "x=0\nwhile (( x )) ||\n"},
		{"until condition", "x=0\nuntil (( x )) &&\n"},
		{"top level", "print a &&\n"},
		{"top level or", "print a ||\n"},
		{"before semicolon", "print a &&\n;\n"},
		{"function body", "h() { print a &&\n}\n"},
		{"subshell", "( print a &&\n)\n"},
		{"then branch", "if true; then\nprint a &&\nfi\n"},
		{"loop body", "while (( 0 )); do\nprint a &&\ndone\n"},
		{"case arm", "case x in y) print a &&\n;; esac\n"},
		{"two in one file", "print a &&\nprint b &&\n"},
		{"nested closers", "h() {\nif true; then\nprint a &&\nfi\n}\n"},
		{"closer same line brace", "h() { print a && }\n"},
		{"closer same line done", "while true; do print a && done\n"},
		{"closer same line esac", "case x in y) print a && ;; esac\n"},
		{"elif follows", "if true; then\nprint a &&\nelif false; then\nprint b\nfi\n"},
		{"else follows", "if true; then\nprint a &&\nelse\nprint b\nfi\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
		})
	}
}

// The adapter must not widen the grammar. Every case here is rejected by native
// Zsh, and several produce exactly the same parser error text as the valid
// shapes above, so only the bytes after the operator can tell them apart.
func TestDanglingAndOrRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"dangling pipe", "x=0\nwhile (( x )) |\n"},
		{"if without then", "x=0\nif (( x )) &&\n"},
		{"operator after operator", "print a &&\n&&\n"},
		{"or after and", "print a &&\n||\n"},
		{"operator then pipe", "print a && | b\n"},
		{"operator blank then operator", "print a &&\n\n&&\n"},
		{"leading operator", "&& print a\n"},
		{"closer without opener", "print a &&\n}\n"},
		{"fi without if", "print a &&\nfi\n"},
		{"done without loop", "print a &&\ndone\n"},
		{"stray closer", "print a\n}\n"},
		{"unclosed if", "if true; then\nprint a\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err == nil {
				t.Fatalf("invalid Zsh accepted: %q", tc.src)
			}
		})
	}
}

// A `&&` with a real right operand must be left alone, including when that
// operand is on the following line. Masking one of these would delete an
// operator and silently change what the script means.
func TestDanglingAndOrLeavesCompleteOperators(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"same line", "print a && print b\n"},
		{"next line", "print a &&\nprint b\n"},
		{"blank line between", "print a &&\n\nprint b\n"},
		{"comment between", "print a &&\n# note\nprint b\n"},
		{"operand before closer", "h() { print a && print b\n}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := parseWithAdapters([]byte(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			// The operator survives: printing the tree still contains `&&`.
			var rendered strings.Builder
			if err := syntax.NewPrinter().Print(&rendered, tree); err != nil {
				t.Fatalf("print: %v", err)
			}
			if !strings.Contains(rendered.String(), "&&") {
				t.Fatalf("operator was masked away: %q", rendered.String())
			}
		})
	}
}

// The scanner decides by what follows the operator, so exercise it directly.
func TestDanglingOperatorEndsList(t *testing.T) {
	cases := []struct {
		name string
		rest string
		want bool
	}{
		{"end of input", "", true},
		{"newline then end", "\n", true},
		{"semicolon", "\n;\n", true},
		{"close brace", "\n}\n", true},
		{"close paren", "\n)\n", true},
		{"done", "\ndone\n", true},
		{"fi", "\nfi\n", true},
		{"esac", "\n;; esac\n", true},
		{"background", "\n&\n", true},
		{"another and", "\n&&\n", false},
		{"another or", "\n||\n", false},
		{"pipe", "\n|\n", false},
		{"statement", "\nprint b\n", false},
		{"statement same line", " print b\n", false},
		{"comment then statement", "\n# c\nprint b\n", false},
		{"word starting with done", "\ndoner\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := danglingOperatorEndsList([]byte(tc.rest), 0); got != tc.want {
				t.Fatalf("danglingOperatorEndsList(%q) = %v, want %v", tc.rest, got, tc.want)
			}
		})
	}
}
