package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #397. In a command or array word, a bare `(` inside a glob group
// nests, as gettokstr counts it in pct (Src/lex.c), so only its matching `)`
// closes it. The inner `)` used to close the outer group, and the rest of
// the word was lexed as a new token. Native verdicts use newline-terminated
// files and zsh -f -n.
func TestZshNestedGlobGroupWordAccept(t *testing.T) {
	for _, tc := range []struct{ src, lit string }{
		{"f=( (a|(b|c)) )", "(a|(b|c))"},
		{"f=( (a|.(b|c)) )", "(a|.(b|c))"},
		{"f=( x(a|(b|c)) )", "(a|(b|c))"},
		{"print (a|.(b|c))", "(a|.(b|c))"},
		{"f=( *~(a|.(b|c))/* )", "(a|.(b|c))"},
		{"files=( (#i)**/*.(zip|rar)~(*/*|.(_backup|git))/*(-.DN) )", "(*/*|.(_backup|git))"},
		{"print a(b(c)d)", "(b(c)d)"},
		{"f=( a(b(c)d) )", "(b(c)d)"},
		{"print (a|(b))", "(a|(b))"},
		{"print a(b(c|d)|(e))", "(b(c|d)|(e))"},
		{"print a(b(c)\nd)", "(b(c)\nd)"},
		{"x=a(b(c))", "(b(c))"},
		{"case x in a(b(c))) ;; esac", "(b(c))"},
		// Quoting and substitutions inside the nested group still nest as
		// they do in the outer one.
		{"print a(b(\")\"))", "(b(\")\"))"},
		{"print a(b(${x:-)}))", "(b(${x:-)}))"},
		{"print a(b($(c)))", "(b($(c)))"},
		{"print a(b(<(c)))", "(b(<(c)))"},
		{"print a(b(<1-2>))", "(b(<1-2>))"},
		// Blanks inside a group are part of the word.
		{"print a(b(c d))", "(b(c d))"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			syntax.Walk(f, func(n syntax.Node) bool {
				if l, ok := n.(*syntax.Lit); ok && l.Value == tc.lit {
					found = true
				}
				return true
			})
			if !found {
				t.Fatalf("no literal %q in the tree", tc.lit)
			}
		})
	}
}

func TestZshNestedGlobGroupWordReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		// The issue's false accept: the nested group is left open.
		{"f=( (a|(b|c) )", "1:3: reached EOF without matching `(` with `)`"},
		// A group left open to the end of input passes zsh -f -n, which
		// does not match the pattern, and fails when run with `bad
		// pattern`. The parser rejects it as it rejects `print a(b` on
		// main; the survey records the class.
		{"print a(b(c)", "1:8: reached EOF without matching `(` with `)`"},
		// One `)` too many still closes the word early.
		{"print a(b(c)))", "1:14: a command can only contain words and redirects; encountered `)`"},
		// `;`, `&` and a bare `>` end the word inside a nested group too.
		{"print a(b(c;d))", "1:14: a command can only contain words and redirects; encountered `)`"},
		{"print a(b(c)&)", "1:14: `)` can only be used to close a subshell"},
		{"print a(b(c)>f)", "1:15: a command can only contain words and redirects; encountered `)`"},
		// `()` is a token of its own (#522).
		{"print a(b(()))", "1:11: a command can only contain words and redirects; encountered `(`"},
		// A substitution body in a nested group is parsed (#519).
		{"print a(b($(done)))", "1:13: `done` can only be used to end a loop"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}
