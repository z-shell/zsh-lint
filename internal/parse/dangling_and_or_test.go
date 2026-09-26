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
		// #466: a `\` line continuation or a comment between the operator and
		// the closer is list trivia, not a right operand.
		{"continuation before else", "if true; then\nprint a || \\\nelse\nprint b\nfi\n"},
		{"continuation before elif", "if true; then print a && \\\nelif true; then :; fi\n"},
		{"continuation before fi", "if true; then print a || \\\nfi\n"},
		{"continuation before done", "while false; do print a || \\\ndone\n"},
		{"continuation before brace", "{ print a || \\\n}\n"},
		{"continuation before then", "if print a || \\\nthen print b; fi\n"},
		{"continuation before paren", "( print a || \\\n)\n"},
		{"continuation before case arm end", "case x in x) print a || \\\n;; esac\n"},
		{"continuation at end of input", "print a || \\\n"},
		{"continuation glued to operator", "if true; then print a ||\\\nelse print b; fi\n"},
		{"continuation then blank line", "if true; then print a || \\\n\nelse print b; fi\n"},
		{"two continuations", "{ print a && \\\n\\\n\\\n}\n"},
		{"continuation in substitution", "x=$(print a || \\\n)\n"},
		{"comment before else", "if true; then print a || # c\nelse print b; fi\n"},
		{"comment glued to operator", "if true; then print a ||#c\nelse print b; fi\n"},
		{"comment line before else", "if true; then print a ||\n# c ||\nelse print b; fi\n"},
		{"comment before brace", "{ print a || # c\n}\n"},
		{"continuation then comment line", "{ print a || \\\n# c\n}\n"},
		{"comment ending in backslash", "if true; then print a || # c \\\nelse print b; fi\n"},
		{"length expansion before operator", "if true; then print ${#x} || \\\nelse print b; fi\n"},
		{"quoted newline before operator", "if true; then print \"a\nb\" || \\\nelse print b; fi\n"},
		{"quoted hash line before operator", "if true; then print 'a\n# x' ||\nelse print b; fi\n"},
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
		// #466: the new trivia must not let an invalid operand through.
		{"continuation then pipe", "print a || \\\n| print b\n"},
		{"continuation then operator", "print a || \\\n&& print b\n"},
		{"continuation then background", "print a && \\\n&\n"},
		{"continuation then operator before closer", "{ print a && \\\n&& }\n"},
		{"dangling pipe continuation before else", "if true; then print a | \\\nelse print b; fi\n"},
		{"dangling pipe comment before brace", "{ print a | # c\n}\n"},
		{"dangling pipe continuation before fi", "print a | \\\nfi\n"},
		{"lone continuation before stray closer", "print a ||\n\\\n}\n"},
		{"triple bar", "if true; then print a ||| \\\nelse print b; fi\n"},
		{"brace-form if broken by continuation", "if true { print a || \\\n} else { print b }\n"},
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
		// #466: a continuation joins the next line, so a statement there is the
		// operand; so is a `#` that does not start a word.
		{"continuation then statement", "print a && \\\nprint b\n"},
		{"continuation then comment then statement", "print a && \\\n# c\nprint b\n"},
		{"glued hash is not a comment", "h() { print a && print b#c\n}\n"},
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
		{"continuation then end", " \\\n", true},
		{"continuation then closer", " \\\n}\n", true},
		{"continuation then else", " \\\nelse\n", true},
		{"continuation then statement", " \\\nprint b\n", false},
		{"continuation then pipe", " \\\n| b\n", false},
		{"escaped backslash is a word", " \\\\\n}\n", false},
		{"escaped blank is a word", " \\ \n}\n", false},
		{"background at end of input", "\n&", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := danglingOperatorEndsList([]byte(tc.rest), 0); got != tc.want {
				t.Fatalf("danglingOperatorEndsList(%q) = %v, want %v", tc.rest, got, tc.want)
			}
		})
	}
}

// The closer path walks back from the reported closer, so exercise it with the
// seed the parser would report, including the edges of the source (#466).
func TestFindDanglingAndOrBefore(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		closer string // seed is the offset of its last occurrence; "" is end of input
		want   int    // operator offset, or -1 for no candidate
	}{
		{"blank line", "{ a &&\n\n}", "}", 4},
		{"continuation", "{ a || \\\n}", "}", 4},
		{"continuation glued", "{ a ||\\\n}", "}", 4},
		{"two continuations", "{ a && \\\n\\\n}", "}", 4},
		{"trailing comment", "{ a || # c\n}", "}", 4},
		{"glued comment", "{ a ||#c\n}", "}", 4},
		{"comment line", "{ a &&\n  # c ||\n}", "}", 4},
		{"comment line after continuation", "{ a && \\\n# c\n}", "}", 4},
		{"operator at start of source", "&& \\\n}", "}", 0},
		{"source starts with a newline", "\n}", "}", -1},
		{"closer at start of source", "}", "}", -1},
		{"end of input", "a ||", "", 2},
		{"end of input after continuation", "a || \\\n", "", 2},
		{"operand", "{ a && b\n}", "}", -1},
		{"operand after continuation", "{ a && \\\nb\n}", "}", -1},
		{"pipe", "{ a | \\\n}", "}", -1},
		{"glued hash is a word", "{ a && b#c\n}", "}", -1},
		{"expansion hash is a word", "{ a && ${#x}\n}", "}", -1},
		{"quoted hash is not a comment", "{ a && \"x #y\"\n}", "}", -1},
		{"escaped backslash is a word", "{ a && \\\\\n}", "}", -1},
		{"quoted hash then operator", "{ a \"x #y\" ||\n}", "}", 11},
		{"multi-line quote with a hash line", "{ a 'x\n# y' ||\n}", "}", 12},
		{"operator before comment holding an operator", "{ a && # c ||\n}", "}", 4},
		// Only what follows the operator is checked here. A missing left
		// operand is the parser's own error, and the reject row
		// "continuation then operator before closer" proves it survives.
		{"second operator", "{ a && \\\n&& }", "}", 9},
		// Zsh reads `\r` as a word byte (#333), so it is an operand, not trivia.
		{"carriage return is a word", "{ a &&\r\n}", "}", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seed := len(tc.src)
			if tc.closer != "" {
				seed = strings.LastIndex(tc.src, tc.closer)
			}
			got, ok := findDanglingAndOrBefore([]byte(tc.src), seed)
			if !ok {
				got = -1
			}
			if got != tc.want {
				t.Fatalf("findDanglingAndOrBefore(%q, %d) = %d, want %d", tc.src, seed, got, tc.want)
			}
		})
	}
	if _, ok := findDanglingAndOrBefore([]byte("a ||"), 5); ok {
		t.Fatal("a seed past the end of the source must be refused")
	}
}

func TestFirstCommentStart(t *testing.T) {
	cases := []struct {
		line string
		want int
	}{
		{"# c", 0},
		{"a # c", 2},
		{"a	# c", 2},
		{"a;# c", 2},
		{"a &&# c", 4},
		{"a ||# c", 4},
		{"a#b", -1},
		{"${#x}", -1},
		{"$#", -1},
		{"(#i)x", -1},
		{"a", -1},
		{"", -1},
		{"a#b # c", 4},
	}
	for _, tc := range cases {
		if got := firstCommentStart([]byte(tc.line)); got != tc.want {
			t.Errorf("firstCommentStart(%q) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestIsAndOrOperatorAt(t *testing.T) {
	cases := []struct {
		src  string
		i    int
		want bool
	}{
		{"&&", 0, true},
		{"||", 0, true},
		{"a &&", 2, true},
		{"a &&", 3, false},
		{"a &", 2, false},
		{"&|", 0, false},
		{"|&", 0, false},
		{"&&", -1, false},
		{"&&", -2, false},
	}
	for _, tc := range cases {
		if got := isAndOrOperatorAt([]byte(tc.src), tc.i); got != tc.want {
			t.Errorf("isAndOrOperatorAt(%q, %d) = %v, want %v", tc.src, tc.i, got, tc.want)
		}
	}
}

func TestSkipTriviaBackward(t *testing.T) {
	cases := []struct {
		src  string
		end  int
		want int
	}{
		{"a  \n}", 4, 1},
		{"a \\\n}", 4, 1},
		{"\n}", 1, 0},
		{"\\\n}", 2, 0},
		{"a\\\\\n}", 4, 3},
		{"a\\\\\\\n}", 5, 3},
		{"", 0, 0},
	}
	for _, tc := range cases {
		if got := skipTriviaBackward([]byte(tc.src), tc.end); got != tc.want {
			t.Errorf("skipTriviaBackward(%q, %d) = %d, want %d", tc.src, tc.end, got, tc.want)
		}
	}
}
