package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// zshmisc spells a function definition as `word ... () [ term ] { list }`, so
// the separator before the body is optional in the `()` spelling exactly as it
// is after the `function` keyword. #213 adapted the keyword form; this covers
// the `()` form, including the multi-name head from #304.
func TestParenSemicolonBodyParses(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		names []string
	}{
		{"single name", "a () ; { : }\n", []string{"a"}},
		{"two names", "a b () ; { : }\n", []string{"a", "b"}},
		{"three names", "a b c () ; { : }\n", []string{"a", "b", "c"}},
		{"separator then newline", "a () ;\n{ : }\n", []string{"a"}},
		{"body with statements", "a () ; { print one; print two }\n", []string{"a"}},
		{"keyword form still works", "function a; { : }\n", []string{"a"}},
		{"keyword with parens", "function a () ; { : }\n", []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			decls := multiNameFuncDecls(tree)
			if len(decls) != 1 {
				t.Fatalf("got %d function declarations, want 1", len(decls))
			}
			got := funcDeclNames(decls[0])
			if len(got) != len(tc.names) {
				t.Fatalf("got names %v, want %v", got, tc.names)
			}
			for i, want := range tc.names {
				if got[i] != want {
					t.Errorf("name %d: got %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

// The masked separator must not move any later byte: a diagnostic points at the
// original source, so the body has to keep its real offset.
func TestParenSemicolonBodyKeepsPositions(t *testing.T) {
	src := "a () ; { print body }\n"
	tree, err := Parse(strings.NewReader(src), "t.zsh")
	if err != nil {
		t.Fatalf("valid Zsh rejected: %v", err)
	}
	var found bool
	syntax.Walk(tree.AST(), func(node syntax.Node) bool {
		lit, ok := node.(*syntax.Lit)
		if !ok || lit.Value != "body" {
			return true
		}
		found = true
		if got := int(lit.ValuePos.Offset()); got != strings.Index(src, "body") {
			t.Errorf("body literal at offset %d, want %d", got, strings.Index(src, "body"))
		}
		return false
	})
	if !found {
		t.Fatal("body literal not present in the tree")
	}
}

// The `()` head is recognised by shape rather than a keyword, so a command that
// merely resembles one must not be claimed. Each source is valid Zsh that means
// something other than a function definition.
func TestParenSemicolonBodyLeavesOtherCommands(t *testing.T) {
	cases := []struct{ name, src string }{
		{"arithmetic command", "(( 1 + 1 )) ; { : }\n"},
		{"subshell", "( print a ) ; { : }\n"},
		{"assignment", "x=1 ; { : }\n"},
		{"array assignment", "x=(1 2) ; { : }\n"},
		{"pipeline", "a | b ; { : }\n"},
		{"redirection", "a > /dev/null ; { : }\n"},
		{"glob qualifier", "print *(.) ; { : }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(tc.src), "t.zsh"); err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
		})
	}
}

// Native Zsh rejects these, so the adapter must not rescue them.
func TestParenSemicolonBodyRejectsInvalid(t *testing.T) {
	cases := []struct{ name, src string }{
		{"separator at end of file", "a () ;\n"},
		{"double separator", "a () ; ;\n"},
		{"separator then keyword", "a () ; then\n"},
		{"unclosed body", "a () ; { :\n"},
		{"ampersand separator", "a () & { : }\n"},
		{"keyword head", "if () ; { : }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(tc.src), "t.zsh"); err == nil {
				t.Fatalf("invalid Zsh accepted: %q", tc.src)
			}
		})
	}
}

// Native Zsh rejects these, so the adapter must not rescue them. They live as
// .txt fixtures so the repository-wide `zsh -n` gate never sees them.
func TestParenSemicolonBodyInvalidFixtures(t *testing.T) {
	for _, name := range []string{
		"testdata/invalid-307-paren-semicolon-eof.txt",
		"testdata/invalid-307-paren-double-separator.txt",
	} {
		t.Run(name, func(t *testing.T) {
			fixture, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if _, err := Parse(strings.NewReader(string(fixture)), "invalid-307.zsh"); err == nil {
				t.Fatalf("Parse() unexpectedly accepted %s", name)
			}
		})
	}
}

// parenHeadAt is the whole recognition gate, so test it directly rather than
// only through the parser, where another adapter could mask a wrong answer.
func TestParenHeadAt(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"a ()", true},
		{"a b ()", true},
		{"a b c ()", true},
		{"a()", true},
		{"()", false},
		{"( print a )", false},
		{"a | b ()", false},
		{"x=1 ()", false},
		{"a > f ()", false},
		{"a ; ()", false},
		{"a", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			if got := parenHeadAt([]byte(tc.src), 0); got != tc.want {
				t.Errorf("parenHeadAt(%q) = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}
