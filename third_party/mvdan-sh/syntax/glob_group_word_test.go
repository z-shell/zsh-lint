package syntax_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #511. Zsh lexes a glob group as part of its word (gettokstr in
// Src/lex.c), so `;`, `&`, and a `<` or `>` that starts neither a process
// substitution nor a numeric glob end the word inside the group. Native
// verdicts use newline-terminated files and zsh -f -n.
func TestZshGlobGroupWordEndsReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"[[ x == a(b;c) ]]", "1:12: not a valid test operator: `;`"},
		{"[[ x == a(b&c) ]]", "1:12: not a valid test operator: `&`"},
		{"[[ x == a(b>c) ]]", "1:12: expected `&&`, `||` or `]]` after complex expr"},
		{"[[ x == a(b<c) ]]", "1:12: expected `&&`, `||` or `]]` after complex expr"},
		{"print a(b;c)", "1:12: a command can only contain words and redirects; encountered `)`"},
		{"print a(b&c)", "1:12: a command can only contain words and redirects; encountered `)`"},
		{"print a(b>c)", "1:12: a command can only contain words and redirects; encountered `)`"},
		{"print a(<x>)", "1:11: `>` must be followed by a word"},
		{"x=a(b;c)", "1:8: a command can only contain words and redirects; encountered `)`"},
		// A `&&` is a connective, so the group's `)` is reported where it
		// stands, not at the `[[`.
		{"[[ x == a(b&&c) ]]", "1:15: `)` matches no `(`: the word ended inside the glob group at 1:10"},
		{"[[ x == (a&&b) ]]", "1:14: `)` matches no `(`: the word ended inside the glob group at 1:9"},
		{"[[ ( x == a(b&&c) ) ]]", "1:19: `)` matches no `(`: the word ended inside the glob group at 1:12"},
		// Numeric globs: only a complete `<m-n>` stays in the word.
		{"print a(<1-2)", "1:13: a command can only contain words and redirects; encountered `)`"},
		{"print a(<1-2>>)", "1:14: `>` must be followed by a word"},
		{"print a(<a-b>)", "1:13: `>` must be followed by a word"},
		{"print a(<+1-2>)", "1:14: `>` must be followed by a word"},
		{"print a(<1>)", "1:11: `>` must be followed by a word"},
		// Only a `)` reports the broken group; at EOF the `[[` is unclosed.
		{"[[ x == a(b&&c", "1:1: reached EOF without matching `[[` with `]]`"},
		// A broken group outside a substitution does not leak into a `[[`
		// inside it.
		{"print a(b;c $([[ y ) ]]))", "1:15: reached `)` without matching `[[` with `]]`"},
		// Glob qualifiers, case patterns, for lists and arrays lex alike.
		{"print *(e:x;y:)", "1:15: a command can only contain words and redirects; encountered `)`"},
		{"case x in a(b;c)) ;; esac", "1:14: case patterns must be separated with `|`"},
		{"for i in a(b;c); do :; done", "1:15: a command can only contain words and redirects; encountered `)`"},
		{"f=( a(b;c) )", "1:8: array element values must be words"},
		{"print a(b|c)d(e;f)", "1:18: a command can only contain words and redirects; encountered `)`"},
		{"(print a(b;c))", "1:14: statements must be separated by &, ; or a newline"},
		{"print a(b\\\nc;d)", "2:4: a command can only contain words and redirects; encountered `)`"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

// Valid Zsh that must parse and print back to the same tree. The first rows
// were already accepted; the rest were rejected before #511.
func TestZshGlobGroupWordEndsAccept(t *testing.T) {
	for _, src := range []string{
		"[[ x == a(b|c) ]]",
		"print a(b|c)",
		"print a(b||c)",
		"print a(<1-2>)",
		"print a(<1->)",
		"print a(<->)",
		"print a(<-1>)",
		"print a(<1-2>|x)",
		"print a(b<1-2>c)",
		"print a(\"b;c\")",
		"print a('b;c')",
		"print a(b\\;c)",
		"print a($(b;c))",
		"print a(b)(c)",
		"print *(e'x;y')",
		"print ${x:-a(b;c)}",
		"print a(${x:-)})",
		"print a(${x:-\"(\"})",
		// Process substitutions stay in the word.
		"print a(<(x))",
		"print a(>(x))",
		"print a(b<(x))",
		// A `(` inside a parameter expansion opens nothing.
		"print a(${x:-(})",
		"print a(\"${x:-(}\")",
		"print a(${x:-${y:-(}})",
		"print a(${(s:(:)x})",
	} {
		t.Run(src, func(t *testing.T) {
			f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			var printed bytes.Buffer
			if err := syntax.NewPrinter().Print(&printed, f); err != nil {
				t.Fatal(err)
			}
			again, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(&printed, "")
			if err != nil {
				t.Fatalf("printed source does not parse: %v", err)
			}
			if diff := cmp.Diff(f, again, cmpopts.IgnoreTypes(syntax.Pos{})); diff != "" {
				t.Fatalf("printing changed the tree (-want +got):\n%s", diff)
			}
		})
	}
}

// Inside a command substitution the word ends at the `;` too, which Zsh
// accepts when the `)` after it closes the substitution: it runs `print a(b`
// and then `c`, and the last `)` is literal text after the substitution
// (zsh 5.9.2 prints `bad pattern: a(b` and then `)`). The printer writes the
// `;` as a newline, so these rows check the tree without printing. The first
// row moved here from TestZshModuleConditions.
func TestZshGlobGroupWordEndsInSubstitution(t *testing.T) {
	for _, src := range []string{
		"[[ -foo a ${x:-$(print a(b;c))} ]]",
		"print ${x:-$(print a(b;c))}",
		"print \"${x:-$(print a(b;c))}\"",
		"x=\"$(print a(b;c))\"",
	} {
		t.Run(src, func(t *testing.T) {
			f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			var subst *syntax.CmdSubst
			syntax.Walk(f, func(n syntax.Node) bool {
				if cs, ok := n.(*syntax.CmdSubst); ok && subst == nil {
					subst = cs
				}
				return true
			})
			if subst == nil {
				t.Fatal("no command substitution")
			}
			var got []string
			for _, stmt := range subst.Stmts {
				call, ok := stmt.Cmd.(*syntax.CallExpr)
				if !ok {
					t.Fatalf("statement is %T", stmt.Cmd)
				}
				var words []string
				for _, w := range call.Args {
					words = append(words, src[w.Pos().Offset():w.End().Offset()])
				}
				got = append(got, strings.Join(words, " "))
			}
			if want := []string{"print a(b", "c"}; !cmp.Equal(got, want) {
				t.Fatalf("statements = %q, want %q", got, want)
			}
			if after := src[subst.End().Offset():]; !strings.HasPrefix(after, ")") {
				t.Fatalf("text after the substitution = %q, want a literal `)`", after)
			}
		})
	}
}

// A word that ends inside its group leaves the group open, and a blank inside
// the group stays in the word, as it does in Zsh (pct in gettokstr). The
// printer re-spaces these, so the rows check the broken word directly.
func TestZshGlobGroupWordEndsOpenGroup(t *testing.T) {
	for _, tc := range []struct{ src, word string }{
		{"(print a(b;c)", "a(b"},
		{"[[ x == a(b&&c ]]", "a(b"},
		{"[[ x == a(b && c ]]", "a(b "},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			syntax.Walk(f, func(n syntax.Node) bool {
				if w, ok := n.(*syntax.Word); ok && tc.src[w.Pos().Offset():w.End().Offset()] == tc.word {
					found = true
				}
				return true
			})
			if !found {
				t.Fatalf("no word %q in the tree", tc.word)
			}
		})
	}
}

// A numeric glob longer than the parser's read buffer is read rather than
// peeked. Valid shapes of any length parse; a shape that fails past the
// buffer is an error at the failing byte, since its `<` can no longer be
// left unread. Each rejected source is also a native parse error.
func TestZshGlobGroupLongNumericGlob(t *testing.T) {
	digits := strings.Repeat("1", 1100)
	for _, src := range []string{
		"print a(<" + digits + "-2>)",
		"print a(<" + digits + "->)",
		"print a(<" + digits + "-2>x)",
		"print a(<-" + digits + ">)",
		"print a(b<" + strings.Repeat("1", 5000) + "-2>c)",
	} {
		t.Run(src[:12], func(t *testing.T) {
			f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			var printed bytes.Buffer
			if err := syntax.NewPrinter().Print(&printed, f); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSuffix(printed.String(), "\n"); got != src {
				t.Fatalf("printed %d bytes, want the %d-byte source", len(got), len(src))
			}
		})
	}
	for _, tc := range []struct{ tail, err string }{
		{"-2)", "1:1112: a numeric glob cannot contain `)`"},
		{"x)", "1:1110: a numeric glob cannot contain `x`"},
		{"--2>)", "1:1111: a numeric glob cannot contain `-`"},
		{">)", "1:1110: a numeric glob cannot contain `>`"},
		{"-2 ; print b)", "1:1112: a numeric glob cannot contain ` `"},
	} {
		t.Run(tc.tail, func(t *testing.T) {
			src := "print a(<" + digits + tc.tail
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}
