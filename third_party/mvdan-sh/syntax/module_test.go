package syntax_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"mvdan.cc/sh/v3/syntax"
	"mvdan.cc/sh/v3/syntax/typedjson"
)

// Issue #484. Native verdicts use newline-terminated files and zsh -f -n.
// Module names need not exist until evaluation.
func TestZshModuleConditions(t *testing.T) {
	for _, src := range []string{
		"[[ -n\na ]]",
		"[[ (-e ) ]]",
		"[[ -prefix - ]]",
		"[[ -prefix -x ]]",
		"[[ ! -prefix - ]]",
		"[[ -prefix a ]]",
		"[[ -after a b ]]",
		"[[ -between a b c ]]",
		"[[ -foo a b c d ]]",
		"[[ -foo a ]] && print y",
		"[[ a -foo b ]]",
		"[[ -prefix - && -n x ]]",
		"[[ -prefix - || -z x ]]",
		"[[ ( -prefix - ) ]]",
		"[[ -n a b ]]",
		"[[ -z ]]",
		"[[ -n ]]",
		"[[ -f a b ]]",
		"[[ -prefix a b c ]]",
		"[[ -- a ]]",
		"[[ -1 a ]]",
		"[[ -a-b c ]]",
		"[[ -prefix $x ]]",
		"[[ -prefix \"-\" ]]",
		"[[ -foo a -bar b ]]",
		"[[ -foo a && b ]]",
		"[[ -foo ]]",
		"[[ -prefix ]]",
		"[[ a -nt b ]]",
		"[[ -z a ]]",
		"[[ a == b ]]",
		"[[ -n == x ]]",
		// `-NAME OP` with nothing after the operator is a one-operand test
		// on the operator's text.
		"[[ -foo == ]]",
		"[[ -foo -nt ]]",
		"[[ -prefix a == b ]]",
		"[[ -prefix a ! b ]]",
		"[[ -prefix a ( b ) ]]",
		"[[ -prefix a\nb ]]",
		"[[ -prefix\na ]]",
		"[[ -z && a ]]",
		"[[ -foo a ( b ) ]]",
		"[[ -foo a (b|c) d ]]",
		"[[ -foo a ! b ]]",
		"[[ -foo a { ]]",
		"[[ -foo a b { ]]",
		"[[ -foo a(b) ]]",
		"[[ -e \"(a)\" ]]",
		"[[ -foo a b c d e f ]]",
		// A glob group in an operand nests bare parentheses and numeric
		// globs as Zsh's word lexer does.
		"[[ -foo a ( b (c) ) ]]",
		"[[ -foo a ( b ((c)) ) ]]",
		"[[ -foo a(b(c)d) e ]]",
		"[[ -foo x y a(b(c)d) ]]",
		"[[ a -foo b(c(d)e) ]]",
		"[[ -n a(b(c)d) ]]",
		"[[ -foo a ( b <1-5> ) ]]",
		"[[ -foo a ( b <-> ) ]]",
		"[[ -foo a ( b <(c) ) ]]",
		"[[ -foo a ( b >(c) ) ]]",
		"[[ -foo a ( b \"(\" (c) ) ]]",
		"[[ -foo a ( b \\( (c) ) ]]",
		"[[ -foo a ( b $(c) (d) ) ]]",
		"[[ -foo a ( b || c ) ]]",
		"[[ -foo a ( b\n) ]]",
		// A nested command substitution lexes with its own rules and hands
		// the condition's back when it closes.
		"[[ -foo $(x) ( b (c) ) ]]",
		"[[ -foo a $(x)(b(c)d) ]]",
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
			var encoded bytes.Buffer
			if err := typedjson.Encode(&encoded, f); err != nil {
				t.Fatal(err)
			}
			decoded, err := typedjson.Decode(&encoded)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(f, decoded) {
				t.Fatal("typedjson changed the tree")
			}
		})
	}
}

func TestZshModuleConditionsReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"[[ a -foo ]] ]]", "1:6: module condition requires a right operand"},
		{"[[ ! a -foo ]] ]]", "1:8: module condition requires a right operand"},
		{"[[ ( a -foo ]] ) ]]", "1:8: module condition requires a right operand"},
		{"[[ a -foo ]] && a ]]", "1:6: module condition requires a right operand"},
		{"[[ a -foo ]] || a ]]", "1:6: module condition requires a right operand"},
		{"[[ a && a -foo ]] ]]", "1:11: module condition requires a right operand"},
		{"[[ -prefix ! ]]", "1:12: not a valid test operator: `!`"},
		{"[[ -prefix ! a ]]", "1:12: not a valid test operator: `!`"},
		{"[[ -prefix ( a ) ]]", "1:12: a condition operand cannot start with `(`"},
		{"[[ -N ! ]]", "1:7: not a valid test operator: `!`"},
		{"[[ -R ! a ]]", "1:7: not a valid test operator: `!`"},
		{"[[ -n ! a ]]", "1:7: not a valid test operator: `!`"},
		{"[[ -f ! a ]]", "1:7: not a valid test operator: `!`"},
		{"[[ -z ! a ]]", "1:7: not a valid test operator: `!`"},
		{"[[ a -foo b c ]]", "1:13: not a valid test operator: `c`"},
		{"[[ a -eq b c ]]", "1:12: not a valid test operator: `c`"},
		{"[[ a == b c ]]", "1:11: not a valid test operator: `c`"},
		{"[[ - a ]]", "1:6: not a valid test operator: `a`"},
		{"[[ -prefix ( ]]", "1:12: reached EOF without matching `(` with `)`"},
		// Zsh reads a `(` after the name, or after the third word, as a token.
		{"[[ -n (a) ]]", "1:7: a condition operand cannot start with `(`"},
		{"[[ -foo (a|b) ]]", "1:9: a condition operand cannot start with `(`"},
		{"[[ -foo a b ( c ) ]]", "1:13: not a valid test operator: `(`"},
		// Past the third word a `!` ends the operand list.
		{"[[ -foo a b ! ]]", "1:13: not a valid test operator: `!`"},
		{"[[ -foo a b c ! ]]", "1:15: not a valid test operator: `!`"},
		// A `}` ends the list and is then an error, as in Zsh.
		{"[[ -foo a } ]]", "1:11: not a valid test operator: `}`"},
		{"[[ -foo a b } ]]", "1:13: not a valid test operator: `}`"},
		{"[[ -prefix ) ]]", "1:1: reached `)` without matching `[[` with `]]`"},
		{"[[ -prefix ]] ]]", "1:15: statements must be separated by &, ; or a newline"},
		{"[[ -prefix < ]]", "1:12: `<` must be followed by a word"},
		{"[[ a -foo ]]", "1:6: module condition requires a right operand"},
		{"[[ -prefix a < b ]]", "1:14: expected `&&`, `||` or `]]` after complex expr"},
		{"[[ a -foo b -bar c ]]", "1:13: not a valid test operator: `-bar`"},
		{"[[ -prefix a > b ]]", "1:14: expected `&&`, `||` or `]]` after complex expr"},
		// A bare `(` inside a group nests, so an unclosed one leaves the
		// group open; `;`, `&` and a `<` or `>` that is not a process
		// substitution or a numeric glob end the word inside it.
		{"[[ -foo a ( b (c) ]]", "1:11: reached EOF without matching `(` with `)`"},
		{"[[ -prefix ~ ( -x || (a) - ]]", "1:14: reached EOF without matching `(` with `)`"},
		{"[[ a -foo b(c(d)e ]]", "1:12: reached EOF without matching `(` with `)`"},
		{"[[ -n a(b(c)d ]]", "1:8: reached EOF without matching `(` with `)`"},
		{"[[ -foo a ( b ; c ) ]]", "1:15: a condition glob group cannot contain `;`"},
		{"[[ -foo a ( b & c ) ]]", "1:15: a condition glob group cannot contain `&`"},
		{"[[ -foo a ( b > c ) ]]", "1:15: a condition glob group cannot contain `>`"},
		{"[[ -foo a ( b\n; c ) ]]", "2:1: a condition glob group cannot contain `;`"},
		{"[[ -n a(b;c) ]]", "1:10: a condition glob group cannot contain `;`"},
		{"[[ -foo a ( b <5> ) ]]", "1:17: a condition glob group cannot contain `>`"},
		{"[[ -foo a ( b <x> ) ]]", "1:16: a condition glob group cannot contain `x`"},
		{"[[ -foo a ( b <1--2> ) ]]", "1:18: a condition glob group cannot contain `-`"},
		{"[[ -foo a ( b <1-2 ) ]]", "1:19: a condition glob group cannot contain ` `"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

// Explicit dialect assertions: error-table rows with empty expectations skip.
func TestModuleConditionsDialectGate(t *testing.T) {
	for _, lang := range []syntax.LangVariant{syntax.LangBash, syntax.LangMirBSDKorn} {
		for _, src := range []string{"[[ -prefix - ]]", "[[ a -foo b ]]", "[[ -n a b ]]", "[[ -z ]]", "[[ -n ]]", "[[ -foo == ]]"} {
			_, err := syntax.NewParser(syntax.Variant(lang)).Parse(strings.NewReader(src+"\n"), "")
			if err == nil {
				t.Errorf("%s accepted %q", lang, src)
			}
		}
	}
}

// POSIX treats [[ as an ordinary command, not as conditional syntax.
func TestModuleConditionsPOSIXUnchanged(t *testing.T) {
	const src = "[[ -prefix - ]]\n"
	f, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatal(err)
	}
	call, ok := f.Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) != 4 {
		t.Fatalf("POSIX command changed: %#v", f.Stmts[0].Cmd)
	}
	for i, want := range []string{"[[", "-prefix", "-", "]]"} {
		if call.Args[i].Lit() != want {
			t.Errorf("argument %d = %q, want %q", i, call.Args[i].Lit(), want)
		}
	}
}

func TestModuleConditionTree(t *testing.T) {
	for _, tc := range []struct {
		src, name string
		infix     bool
		args      []string
	}{
		{"[[ -prefix $x ]]", "-prefix", false, []string{"$x"}},
		{"[[ -foo a -bar b ]]", "-foo", false, []string{"a", "-bar", "b"}},
		{"[[ $x -foo $y ]]", "-foo", true, []string{"$x", "$y"}},
		{"[[ -n a b ]]", "-n", false, []string{"a", "b"}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			m, ok := f.Stmts[0].Cmd.(*syntax.TestClause).X.(*syntax.ModuleTest)
			if !ok {
				t.Fatalf("expression = %T", f.Stmts[0].Cmd.(*syntax.TestClause).X)
			}
			if m.Name.Lit() != tc.name || m.Infix != tc.infix || len(m.Args) != len(tc.args) {
				t.Fatalf("module = %#v", m)
			}
			if m.Pos().Offset() != 3 || m.End().Offset() != uint(len(tc.src)-3) || m.Name.Pos().Offset() != uint(strings.Index(tc.src, tc.name)) {
				t.Fatalf("incorrect positions: %v, %v, %v", m.Pos(), m.End(), m.Name.Pos())
			}
			for i, arg := range m.Args {
				if got := tc.src[arg.Pos().Offset():arg.End().Offset()]; got != tc.args[i] {
					t.Fatalf("arg %d = %q, want %q", i, got, tc.args[i])
				}
			}
			var words []string
			syntax.Walk(m, func(n syntax.Node) bool {
				if w, ok := n.(*syntax.Word); ok {
					words = append(words, tc.src[w.Pos().Offset():w.End().Offset()])
				}
				return true
			})
			want := append([]string{tc.name}, tc.args...)
			if tc.infix {
				want = []string{tc.args[0], tc.name, tc.args[1]}
			}
			if !reflect.DeepEqual(words, want) {
				t.Fatalf("walk = %q, want %q", words, want)
			}
		})
	}
}

func TestModuleConditionKeepsExistingTrees(t *testing.T) {
	for _, src := range []string{"[[ -foo ]]", "[[ -prefix ]]", "[[ a -nt b ]]", "[[ -z a ]]", "[[ a == b ]]", "[[ -1 -eq -1 ]]"} {
		zsh, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), "")
		if err != nil {
			t.Fatal(err)
		}
		bash, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src+"\n"), "")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(zsh, bash) {
			t.Fatalf("existing tree changed: %s", src)
		}
	}
	for _, src := range []string{"[[ -z ]]", "[[ -n ]]"} {
		f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), "")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := f.Stmts[0].Cmd.(*syntax.TestClause).X.(*syntax.Word); !ok {
			t.Fatalf("%s is not a string test", src)
		}
	}
}

// A ModuleTest built by hand or decoded from JSON may lack the operands the
// parser always supplies; Pos, End, Walk and the printer must not panic on it.
func TestZshModuleConditionDegenerate(t *testing.T) {
	name := &syntax.Word{Parts: []syntax.WordPart{&syntax.Lit{Value: "-foo"}}}
	one := &syntax.Word{Parts: []syntax.WordPart{&syntax.Lit{Value: "a"}}}
	for _, m := range []*syntax.ModuleTest{
		{Name: name},
		{Name: name, Infix: true},
		{Name: name, Infix: true, Args: []*syntax.Word{one}},
	} {
		_ = m.Pos()
		_ = m.End()
		syntax.Walk(m, func(syntax.Node) bool { return true })
		f := &syntax.File{Stmts: []*syntax.Stmt{{Cmd: &syntax.TestClause{X: m}}}}
		var out bytes.Buffer
		if err := syntax.NewPrinter().Print(&out, f); err != nil {
			t.Fatal(err)
		}
	}
}

// Issue #512: a lone `-` condition, or `!` or `(` after `<` or `>`, must be rejected in Zsh.
func TestZshIssue512Rejects(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"[[ - ]]", "1:4: condition expected: -"},
		{"[[ ! - ]]", "1:6: condition expected: -"},
		{"[[ ( - ) ]]", "1:6: condition expected: -"},
		{"[[ -n a && - ]]", "1:12: condition expected: -"},
		// A lone `-` before a connective.
		{"[[ - && x ]]", "1:4: condition expected: -"},
		{"[[ - || x ]]", "1:4: condition expected: -"},
		{"[[ x < ! ]]", "1:8: not a valid test operator: `!`"},
		{"[[ x > ! ]]", "1:8: not a valid test operator: `!`"},
		{"[[ x < (a) ]]", "1:8: a condition operand cannot start with `(`"},
		{"[[ x < ( || ) ]]", "1:8: a condition operand cannot start with `(`"},
		{"[[ x < ! y ]]", "1:8: not a valid test operator: `!`"},
		{"[[ x < (a)b ]]", "1:8: a condition operand cannot start with `(`"},
		{"[[ x < ( ]]", "1:8: reached EOF without matching `(` with `)`"},
		{"[[ x <\n! ]]", "2:1: not a valid test operator: `!`"},
		{"[[ x >\n! ]]", "2:1: not a valid test operator: `!`"},
		// `]]` is never the operand of `<` or `>`, on the same line or the
		// next; the error names the operator.
		{"[[ x <\n]]", "1:6: `<` must be followed by a word"},
		{"[[ x >\n]]", "1:6: `>` must be followed by a word"},
		{"[[ x < ]]", "1:6: `<` must be followed by a word"},
		{"[[ x < ]] ]]", "1:6: `<` must be followed by a word"},
		{"[[ x <\n]] ]]", "1:6: `<` must be followed by a word"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

// Issue #512: control rows must continue parsing under Zsh.
func TestZshIssue512Controls(t *testing.T) {
	for _, src := range []string{
		"[[ -- ]]",
		"[[ x < y ]]",
		"[[ x < \"!\" ]]",
		"[[ x == ! ]]",
		"[[ x < !y ]]",
		"[[ x < a(b) ]]",
		"[[ x > '(' ]]",
		"[[ ! x < y ]]",
		"[[ x < y && ! z ]]",
		"[[ x <\ny ]]",
		"[[ x >\ny ]]",
		"[[ x < ]]x ]]",
		"[[ x < ']]' ]]",
		"[[ x <\n\\]] ]]",
	} {
		t.Run(src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), "")
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
		})
	}
}

// Issue #512: Bash behavior must remain unchanged.
func TestZshIssue512BashUnchanged(t *testing.T) {
	for _, src := range []string{
		"[[ - ]]",
		"[[ ! - ]]",
		"[[ ( - ) ]]",
		"[[ -n a && - ]]",
		"[[ - && x ]]",
		"[[ - || x ]]",
		"[[ x < ! ]]",
		"[[ x > ! ]]",
		"[[ -- ]]",
		"[[ x < y ]]",
		"[[ x < \"!\" ]]",
		"[[ x == ! ]]",
		"[[ x < !y ]]",
		"[[ x > '(' ]]",
		"[[ ! x < y ]]",
		"[[ x < y && ! z ]]",
	} {
		t.Run(src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src+"\n"), "")
			if err != nil {
				t.Fatalf("Bash unexpectedly failed to parse %s: %v", src, err)
			}
		})
	}
	for _, tc := range []struct{ src, err string }{
		{"[[ x < a(b) ]]", "1:9: not a valid test operator: `(`"},
		{"[[ x < (a) ]]", "1:6: `<` must be followed by a word"},
		{"[[ x < ( || ) ]]", "1:6: `<` must be followed by a word"},
		{"[[ x < ( ]]", "1:6: `<` must be followed by a word"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}
