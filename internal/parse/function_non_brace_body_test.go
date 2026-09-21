package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// zshmisc spells a function body as a `list`, and a brace group is only one way
// to write one. Native Zsh defines `a` with `print z` as its body for
// `a () ; print z`, so the front end must accept a body that is any statement.
func TestFunctionNonBraceBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"paren plain command", "a () ; print z\n"},
		{"paren if", "a () ; if true; then print z; fi\n"},
		{"paren while", "a () ; while false; do :; done\n"},
		{"paren for", "a () ; for i in 1; do print z; done\n"},
		{"paren case", "a () ; case x in y) : ;; esac\n"},
		{"paren subshell", "a () ; ( print z )\n"},
		{"paren assignment", "a () ; x=1\n"},
		{"paren body on next line", "a () ;\nprint z\n"},
		{"paren multi name", "a b () ; print z\n"},
		{"keyword plain command", "function a; print z\n"},
		{"keyword if", "function a; if true; then print z; fi\n"},
		{"keyword while", "function a; while false; do :; done\n"},
		{"keyword subshell", "function a; ( print z )\n"},
		{"keyword tab separator", "function a;\tprint z\n"},
		{"keyword already parenthesized", "function a () ; print z\n"},
		{"keyword comment before body", "function a; # c\nprint z\n"},
		{"keyword punctuated name", "function _my-fn; print z\n"},
		{"keyword two definitions", "function a; print y\nfunction b; print z\n"},
		{"brace body still works", "a () ; { print z }\n"},
		{"keyword brace body still works", "function a; { print z }\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
		})
	}
}

// The adapter must not rescue what Zsh itself refuses. A separator with no body
// after it is not a definition, and neither is one followed by a reserved word
// that can only close or continue an enclosing construct.
func TestFunctionNonBraceBodyRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"nothing after separator", "a () ;\n"},
		{"second separator", "a () ; ;\n"},
		{"then", "a () ; then\n"},
		{"else", "a () ; else\n"},
		{"elif", "a () ; elif\n"},
		{"fi", "a () ; fi\n"},
		{"do", "a () ; do\n"},
		{"done", "a () ; done\n"},
		{"esac", "a () ; esac\n"},
		{"always", "a () ; always\n"},
		{"closing brace", "a () ; }\n"},
		{"closing paren", "a () ; )\n"},
		{"pipe", "a () ; |\n"},
		{"background", "a () ; &\n"},
		{"keyword then", "function a; then\n"},
		{"keyword done", "function a; done\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseWithAdapters([]byte(tc.src), "t.zsh"); err == nil {
				t.Fatalf("invalid Zsh accepted: %q", tc.src)
			}
		})
	}
}

// The keyword spelling is rewritten to carry `()`, which costs two bytes. They
// are taken from the separator and the blank after it, so every later byte must
// still sit at its original offset or diagnostics would point at the wrong
// place in a file that now parses.
func TestFunctionNonBraceBodyKeepsPositions(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"keyword", "function a; print zulu\n"},
		{"paren", "a () ; print zulu\n"},
		{"keyword compound", "function a; if true; then print zulu; fi\n"},
		{"keyword tab", "function a;\tprint zulu\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			want := strings.Index(tc.src, "zulu")
			var found bool
			syntax.Walk(f.AST(), func(n syntax.Node) bool {
				lit, ok := n.(*syntax.Lit)
				if !ok || lit.Value != "zulu" {
					return true
				}
				found = true
				if got := int(lit.ValuePos.Offset()); got != want {
					t.Errorf("body word at offset %d, want %d", got, want)
				}
				return true
			})
			if !found {
				t.Fatal("body word not present in the tree")
			}
		})
	}
}

// The body belongs to the function, so it must not also run at top level. A
// rewrite that dropped the definition would still parse and would still put the
// word at the right offset, so assert the declaration and its name directly.
func TestFunctionNonBraceBodyBuildsDeclaration(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		names []string
	}{
		{"keyword", "function a; print z\n", []string{"a"}},
		{"paren", "a () ; print z\n", []string{"a"}},
		{"paren multi name", "a b () ; print z\n", []string{"a", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(strings.NewReader(tc.src), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			var got []string
			syntax.Walk(f.AST(), func(n syntax.Node) bool {
				if decl, ok := n.(*syntax.FuncDecl); ok {
					got = append(got, funcDeclNames(decl)...)
				}
				return true
			})
			if strings.Join(got, ",") != strings.Join(tc.names, ",") {
				t.Fatalf("declared %v, want %v", got, tc.names)
			}
		})
	}
}

// opensFunctionBody decides the whole widening, so test it directly: a gate that
// accepted everything would make the adapter rescue sources Zsh rejects.
func TestOpensFunctionBody(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"print z", true},
		{"if true; then :; fi", true},
		{"{ : }", true},
		{"( : )", true},
		{"x=1", true},
		{"function b; :", true},
		{"", false},
		{";", false},
		{"&", false},
		{"|", false},
		{")", false},
		{"}", false},
		{"then", false},
		{"else", false},
		{"elif", false},
		{"fi", false},
		{"do", false},
		{"done", false},
		{"esac", false},
		{"always", false},
		// A reserved word is only refused as a whole word: these are commands.
		{"doit", true},
		{"finish", true},
		{"thenceforth", true},
	} {
		if got := opensFunctionBody([]byte(tc.src), 0); got != tc.want {
			t.Errorf("opensFunctionBody(%q) = %v, want %v", tc.src, got, tc.want)
		}
	}
}

// functionKeywordNameEnd picks the two bytes the `()` overwrites. A wrong offset
// would corrupt the head rather than fix it, so pin the exact site it reports.
func TestFunctionKeywordNameEnd(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"keyword head", "function a; print z\n", 10},
		{"longer name", "function abcd; print z\n", 13},
		{"punctuated name", "function _my-fn; print z\n", 15},
		// A blank before the separator is spent instead of the separator, so
		// the `()` still lands directly after the name.
		{"blank before separator", "function a ; print z\n", 10},
		{"tab before separator", "function a\t; print z\n", 10},
		// The `()` spelling needs no rewrite; it already parses.
		{"already parenthesized", "function a (); print z\n", 0},
		{"no keyword", "a; print z\n", 0},
		{"paren spelling", "a () ; print z\n", 0},
		// A separator at end of input has no body and no byte to spend; the
		// bounds test here is what keeps the read in range.
		{"separator at end of input", "function a;", 0},
		// `function` is the keyword, so a head with no name word after it is
		// not a definition this rewrite can complete.
		{"keyword without a name", "function ;x\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			semi := strings.IndexByte(tc.src, ';')
			if got := functionKeywordNameEnd([]byte(tc.src), semi); got != tc.want {
				t.Errorf("functionKeywordNameEnd(%q) = %d, want %d", tc.src, got, tc.want)
			}
		})
	}
}

// Sources native Zsh rejects, kept as .txt so the repository-wide `zsh -n` gate
// never sees them.
func TestFunctionNonBraceBodyInvalidFixtures(t *testing.T) {
	for _, name := range []string{
		"invalid-346-no-body.txt",
		"invalid-346-closing-keyword.txt",
	} {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if _, err := parseWithAdapters(src, name); err == nil {
				t.Fatalf("invalid Zsh accepted: %q", src)
			}
		})
	}
}
